// Package reachprobe tells a node that is down from one that is unreachable
// from some regions only. The administrator marks proxy nodes as vantages
// of a region (domestic or overseas); every Interval the singleton worker
// has each connected vantage's Agent run reach.probe on the other nodes'
// public addresses and keeps the latest answer per vantage and target. A
// target's status compares the regions: reachable from one region but not
// from the other is region_unreachable.
//
// The probe only opens and closes TCP connections to the ports of the
// target's enabled protocols: no data, no user traffic.
package reachprobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The regions of a vantage.
const (
	RegionDomestic = "domestic"
	RegionOverseas = "overseas"
)

const (
	// Interval is the time between two rounds.
	Interval = 10 * time.Minute
	// Stale is the age after which an answer no longer counts.
	Stale = 3 * Interval
	// ConnectTimeout bounds each connection.
	ConnectTimeout = 3 * time.Second
	// maxPorts is the ports probed per target: its enabled protocols'.
	maxPorts = 3
	// batch is the targets of one probe (the Agent takes 64).
	batch = 64
)

// The statuses of a target.
const (
	StatusOK                  = "ok"                   // reachable from every region that answered
	StatusRegionUnreachable   = "region_unreachable"   // reachable from overseas, not from domestic
	StatusOverseasUnreachable = "overseas_unreachable" // reachable from domestic, not from overseas
	StatusUnreachable         = "unreachable"          // reachable from no region
	StatusUnknown             = "unknown"              // no fresh answer
)

// Prober runs reach.probe (kernelnodeops.ReachProbes).
type Prober interface {
	Capable(node agentcontrol.AgentNode) bool
	Probe(ctx context.Context, node agentcontrol.AgentNode, targets []kernelnodeops.ReachTarget, timeout time.Duration) (json.RawMessage, error)
}

// Service reads and writes the vantages and the answers.
type Service struct {
	DB     *gorm.DB
	Prober Prober
	// Now defaults to time.Now.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Vantage is a vantage as the API answers it.
type Vantage struct {
	NodeID  uint   `json:"node_id"`
	Region  string `json:"region"`
	Capable bool   `json:"capable"`
}

// Vantages answers the vantages, by node id.
func (s *Service) Vantages(ctx context.Context) ([]Vantage, error) {
	var rows []model.ReachVantage
	if err := s.DB.WithContext(ctx).Order("node_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	vantages := make([]Vantage, len(rows))
	for i, row := range rows {
		vantages[i] = Vantage{NodeID: row.NodeID, Region: row.Region}
		if s.Prober != nil {
			vantages[i].Capable = s.Prober.Capable(proxy(row.NodeID))
		}
	}
	return vantages, nil
}

// SetVantages replaces the vantages, and drops the answers of nodes that
// are no longer one.
func (s *Service) SetVantages(ctx context.Context, vantages []Vantage) error {
	seen := map[uint]bool{}
	for _, v := range vantages {
		if v.Region != RegionDomestic && v.Region != RegionOverseas {
			return fmt.Errorf("region must be %s or %s", RegionDomestic, RegionOverseas)
		}
		if seen[v.NodeID] {
			return fmt.Errorf("node %d is listed twice", v.NodeID)
		}
		seen[v.NodeID] = true
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := make([]uint, 0, len(vantages))
		for _, v := range vantages {
			ids = append(ids, v.NodeID)
		}
		var found int64
		if err := tx.Model(&model.Node{}).Where("id IN ?", append(ids, 0)).Count(&found).Error; err != nil {
			return err
		}
		if int(found) != len(ids) {
			return errors.New("a listed node does not exist")
		}
		if err := tx.Where("1 = 1").Delete(&model.ReachVantage{}).Error; err != nil {
			return err
		}
		if err := tx.Where("vantage_node_id NOT IN ?", append(ids, 0)).Delete(&model.ReachProbe{}).Error; err != nil {
			return err
		}
		now := s.now()
		for _, v := range vantages {
			if err := tx.Create(&model.ReachVantage{NodeID: v.NodeID, Region: v.Region, UpdatedAt: now}).Error; err != nil {
				return err
			}
			// A node that changed region starts over.
			if err := tx.Where("vantage_node_id = ? AND region <> ?", v.NodeID, v.Region).Delete(&model.ReachProbe{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// target is a node the vantages probe.
type target struct {
	nodeID uint
	host   string
	ports  []int
}

// targets are the nodes to probe: every node that is not disabled and not
// a relay child of another node (a domestic entry in front of an exit),
// at its reported address (server_ip, else the address it joined from,
// else its host), on its enabled protocols' ports (at most 3). Nodes
// without a public address or a port are left out.
func (s *Service) targets(ctx context.Context) ([]target, error) {
	var nodes []model.Node
	if err := s.DB.WithContext(ctx).Where("status <> ? AND parent_id IS NULL", model.NodeStatusDisabled).Order("id").Find(&nodes).Error; err != nil {
		return nil, err
	}
	var protocols []model.NodeProtocol
	if err := s.DB.WithContext(ctx).Where("enable = 1 AND port > 0").Order("port").Find(&protocols).Error; err != nil {
		return nil, err
	}
	ports := map[uint][]int{}
	for _, p := range protocols {
		list := ports[p.NodeID]
		if len(list) < maxPorts && !containsPort(list, p.Port) {
			ports[p.NodeID] = append(list, p.Port)
		}
	}
	var out []target
	for _, node := range nodes {
		host := address(node)
		if host == "" || len(ports[node.ID]) == 0 {
			continue
		}
		out = append(out, target{nodeID: node.ID, host: host, ports: ports[node.ID]})
	}
	return out, nil
}

func containsPort(list []int, port int) bool {
	for _, p := range list {
		if p == port {
			return true
		}
	}
	return false
}

// address is the node's public address or host name; "" when it has none.
func address(node model.Node) string {
	for _, raw := range []string{deref(node.ServerIP), node.LastPublicAddress, node.Host} {
		host := strings.TrimSpace(raw)
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if host == "" {
			continue
		}
		if addr, err := netip.ParseAddr(host); err == nil && (!addr.IsGlobalUnicast() || addr.IsPrivate()) {
			continue
		}
		return host
	}
	return ""
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func proxy(id uint) agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(id)} // #nosec G115 -- node ids are uint32 on the wire.
}

// probeAnswer is the part of the Agent's answer Control keeps.
type probeAnswer struct {
	Targets []struct {
		Host      string          `json:"host"`
		Reachable bool            `json:"reachable"`
		Ports     json.RawMessage `json:"ports,omitempty"`
		Address   string          `json:"address,omitempty"`
		Error     string          `json:"error,omitempty"`
	} `json:"targets"`
}

// RunVantage has one vantage probe every target but itself and stores the
// answers.
func (s *Service) RunVantage(ctx context.Context, vantage model.ReachVantage, targets []target) error {
	var list []target
	for _, t := range targets {
		if t.nodeID != vantage.NodeID {
			list = append(list, t)
		}
	}
	for start := 0; start < len(list); start += batch {
		chunk := list[start:min(start+batch, len(list))]
		request := make([]kernelnodeops.ReachTarget, len(chunk))
		for i, t := range chunk {
			request[i] = kernelnodeops.ReachTarget{Host: t.host, Ports: t.ports}
		}
		raw, err := s.Prober.Probe(ctx, proxy(vantage.NodeID), request, ConnectTimeout)
		if err != nil {
			return err
		}
		var answer probeAnswer
		if err := json.Unmarshal(raw, &answer); err != nil || len(answer.Targets) != len(chunk) {
			return fmt.Errorf("the Agent of node %d answered %d targets for %d", vantage.NodeID, len(answer.Targets), len(chunk))
		}
		now := s.now()
		rows := make([]model.ReachProbe, len(chunk))
		for i, t := range chunk {
			result, _ := json.Marshal(answer.Targets[i])
			rows[i] = model.ReachProbe{
				VantageNodeID: vantage.NodeID, TargetNodeID: t.nodeID, Region: vantage.Region,
				Reachable: answer.Targets[i].Reachable, Result: string(result), ProbedAt: now,
			}
		}
		err = s.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "vantage_node_id"}, {Name: "target_node_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"region", "reachable", "result", "probed_at"}),
		}).Create(&rows).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// RunRound has every connected vantage probe the targets, one after the
// other, and answers how many ran.
func (s *Service) RunRound(ctx context.Context) (int, error) {
	var vantages []model.ReachVantage
	if err := s.DB.WithContext(ctx).Order("node_id").Find(&vantages).Error; err != nil {
		return 0, err
	}
	if len(vantages) == 0 {
		return 0, nil
	}
	targets, err := s.targets(ctx)
	if err != nil {
		return 0, err
	}
	ran := 0
	for _, vantage := range vantages {
		if !s.Prober.Capable(proxy(vantage.NodeID)) {
			continue
		}
		if err := s.RunVantage(ctx, vantage, targets); err != nil {
			slog.Warn("reach probe failed", "vantage_node_id", vantage.NodeID, "err", err)
			continue
		}
		ran++
	}
	return ran, nil
}

// RegionView is one region's view of a target.
type RegionView struct {
	// Reachable is true when any fresh answer of the region reached it,
	// nil when the region has no fresh answer.
	Reachable *bool      `json:"reachable"`
	Vantages  int        `json:"vantages"`
	ProbedAt  *time.Time `json:"probed_at,omitempty"`
}

// NodeStatus is a target's status.
type NodeStatus struct {
	NodeID   uint       `json:"node_id"`
	Status   string     `json:"status"`
	Domestic RegionView `json:"domestic"`
	Overseas RegionView `json:"overseas"`
}

// Statuses answers the status of every node that has an answer, by id.
func (s *Service) Statuses(ctx context.Context) ([]NodeStatus, error) {
	var rows []model.ReachProbe
	if err := s.DB.WithContext(ctx).Where("probed_at >= ?", s.now().Add(-Stale)).Find(&rows).Error; err != nil {
		return nil, err
	}
	byNode := map[uint]*NodeStatus{}
	for _, row := range rows {
		status := byNode[row.TargetNodeID]
		if status == nil {
			status = &NodeStatus{NodeID: row.TargetNodeID}
			byNode[row.TargetNodeID] = status
		}
		view := &status.Domestic
		if row.Region == RegionOverseas {
			view = &status.Overseas
		}
		view.Vantages++
		reachable := row.Reachable || (view.Reachable != nil && *view.Reachable)
		view.Reachable = &reachable
		if view.ProbedAt == nil || row.ProbedAt.After(*view.ProbedAt) {
			at := row.ProbedAt.UTC()
			view.ProbedAt = &at
		}
	}
	out := make([]NodeStatus, 0, len(byNode))
	for _, status := range byNode {
		status.Status = verdict(status.Domestic.Reachable, status.Overseas.Reachable)
		out = append(out, *status)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out, nil
}

// verdict compares the regions.
func verdict(domestic, overseas *bool) string {
	switch {
	case domestic == nil && overseas == nil:
		return StatusUnknown
	case domestic != nil && overseas != nil && !*domestic && *overseas:
		return StatusRegionUnreachable
	case domestic != nil && overseas != nil && *domestic && !*overseas:
		return StatusOverseasUnreachable
	case (domestic == nil || !*domestic) && (overseas == nil || !*overseas):
		return StatusUnreachable
	}
	return StatusOK
}

// Worker runs a round every Interval in the singleton worker.
type Worker struct {
	Service *Service
}

// Run runs rounds until ctx ends.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(Interval)
	defer ticker.Stop()
	for {
		if _, err := w.Service.RunRound(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("reach probe round failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
