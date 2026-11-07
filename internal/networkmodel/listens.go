package networkmodel

import (
	"fmt"
	"net/netip"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// listener is a port a node listens on and who listens there.
type listener struct {
	node, address string
	port          uint32
	owner         string
}

// overlaps says whether two listeners on one node take the same socket:
// the same port, and the same address or either unspecified. The protocol
// does not matter, as in planned.take.
func (l listener) overlaps(other listener) bool {
	if l.node != other.node || l.port != other.port {
		return false
	}
	a, errA := netip.ParseAddr(l.address)
	b, errB := netip.ParseAddr(other.address)
	if l.address == "" || other.address == "" || (errA == nil && a.IsUnspecified()) || (errB == nil && b.IsUnspecified()) {
		return true
	}
	if errA == nil && errB == nil {
		return a.Unmap() == b.Unmap()
	}
	return l.address == other.address
}

// ListenConflicts answers, for the forwards with the given ids (as the
// sources answer them, after the write in tx), every listener on the same
// node that another owner holds: a network-model ingress, another forward,
// reverse proxy or kernel forward hop, a node protocol's port or an egress
// relay's port. Paused forwards are left out on both sides, as the planner
// leaves them out.
func (s *Service) ListenConflicts(tx *gorm.DB, ids ...string) ([]string, error) {
	_, doc, err := loadDocument(tx)
	if err != nil {
		return nil, err
	}
	full, forwards, err := s.completed(tx, doc)
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	var mine, others []listener
	for _, forward := range forwards {
		if forward.Paused {
			continue
		}
		list := &others
		if wanted[forward.ID] {
			list = &mine
		}
		owner := "forward " + forward.ID
		if forward.ReverseProxy != nil {
			owner = "reverse proxy " + forward.ID
		}
		for _, entry := range forward.Entries {
			*list = append(*list, listener{entry, forward.Listen.Address, forward.Listen.Port, owner})
		}
		for _, hop := range forward.Kernel {
			for _, node := range hop.Nodes {
				*list = append(*list, listener{node, hop.Listen.Address, hop.Listen.Port, "kernel forward " + forward.ID})
			}
		}
	}
	if len(mine) == 0 {
		return nil, nil
	}
	nodes := map[string]string{}
	for _, line := range full.Lines {
		nodes[line.ID] = line.Node
	}
	for _, ingress := range full.Ingresses {
		if !ingress.Disabled {
			others = append(others, listener{nodes[ingress.Line], ingress.Listen.Address, ingress.Listen.Port, "ingress " + ingress.ID})
		}
	}
	var protocols []model.NodeProtocol
	if err := tx.Select("id", "node_id", "port").Where("enable = ?", 1).Find(&protocols).Error; err != nil {
		return nil, err
	}
	for _, protocol := range protocols {
		others = append(others, listener{proxyRef(protocol.NodeID), "", uint32(protocol.Port), fmt.Sprintf("protocol %d", protocol.ID)}) // #nosec G115 -- ports are 16-bit.
	}
	var relays []model.EgressRoute
	if err := tx.Select("id", "exit_node_id", "relay_port").Where("enabled = ?", true).Find(&relays).Error; err != nil {
		return nil, err
	}
	for _, relay := range relays {
		others = append(others, listener{proxyRef(relay.ExitNodeID), "", uint32(relay.RelayPort), fmt.Sprintf("egress relay %d", relay.ID)}) // #nosec G115 -- ports are 16-bit.
	}
	var conflicts []string
	for _, l := range mine {
		for _, other := range others {
			if l.overlaps(other) {
				conflicts = append(conflicts, fmt.Sprintf("listen %s on %s is taken by %s", listenKey(l.address, l.port), l.node, other.owner))
			}
		}
	}
	return conflicts, nil
}
