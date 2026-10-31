package networkmodel

import (
	"context"
	"errors"
	"net/netip"
	"regexp"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// AuditActionNodeSettings is the operation log action of a device's
// network settings write.
const AuditActionNodeSettings = "network_node_settings"

// interfacePattern is a Linux interface name.
var interfacePattern = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,15}$`)

// NodeSettings are a device's own network settings (model.NodeNetwork):
// the interface the network model's automatic egresses of the device leave
// by, with its source address and gateway per family, and the port its
// automatic hop ingress listens on (0: AutoHopPort). Empty fields keep the
// default route.
type NodeSettings struct {
	Interface string `json:"interface"`
	SourceV4  string `json:"source_v4"`
	GatewayV4 string `json:"gateway_v4"`
	SourceV6  string `json:"source_v6"`
	GatewayV6 string `json:"gateway_v6"`
	HopPort   uint32 `json:"hop_port"`
}

// NodeSettings answers node's network settings; zero when it has none.
func (s *Service) NodeSettings(ctx context.Context, node string) (NodeSettings, error) {
	if _, err := s.requireNode(s.DB.WithContext(ctx), node); err != nil {
		return NodeSettings{}, err
	}
	var row model.NodeNetwork
	err := s.DB.WithContext(ctx).Where("node_ref = ?", node).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return NodeSettings{}, nil
	}
	if err != nil {
		return NodeSettings{}, err
	}
	return settingsOf(row), nil
}

// SetNodeSettings stores node's network settings, audited, and replans:
// the automatic parts of the routes through the device follow them.
func (s *Service) SetNodeSettings(ctx context.Context, node string, settings NodeSettings, actor Actor) (NodeSettings, error) {
	settings, err := checkSettings(settings)
	if err != nil {
		return NodeSettings{}, err
	}
	var changed []string
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ref, err := s.requireNode(tx, node)
		if err != nil {
			return err
		}
		_, doc, err := lockModel(tx, s.now())
		if err != nil {
			return err
		}
		row := model.NodeNetwork{NodeRef: ref, Interface: settings.Interface, SourceV4: settings.SourceV4, GatewayV4: settings.GatewayV4,
			SourceV6: settings.SourceV6, GatewayV6: settings.GatewayV6, HopPort: settings.HopPort, UpdatedAt: s.now()}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if changed, err = s.replan(tx, doc); err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionNodeSettings, map[string]any{
			"node": ref, "interface": row.Interface, "source_v4": row.SourceV4, "gateway_v4": row.GatewayV4,
			"source_v6": row.SourceV6, "gateway_v6": row.GatewayV6, "hop_port": row.HopPort,
		})
	})
	if err != nil {
		return NodeSettings{}, err
	}
	notify(changed)
	return settings, nil
}

// requireNode answers node's canonical reference, or ErrUnknownNode.
func (s *Service) requireNode(db *gorm.DB, node string) (string, error) {
	ref, err := agentcontrol.ParseAgentNode(node)
	if err != nil || ref.ID == 0 {
		return "", ErrUnknownNode
	}
	var count int64
	switch ref.Kind {
	case agentcontrol.NodeKindProxy:
		err = db.Model(&model.Node{}).Where("id = ?", ref.ID).Count(&count).Error
	case agentcontrol.NodeKindForward:
		err = db.Model(&model.ForwardNode{}).Where("id = ?", ref.ID).Count(&count).Error
	}
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "", ErrUnknownNode
	}
	return ref.String(), nil
}

func settingsOf(row model.NodeNetwork) NodeSettings {
	return NodeSettings{Interface: row.Interface, SourceV4: row.SourceV4, GatewayV4: row.GatewayV4,
		SourceV6: row.SourceV6, GatewayV6: row.GatewayV6, HopPort: row.HopPort}
}

// checkSettings answers settings trimmed, or an *Invalid: the interface is
// a Linux interface name, each source and gateway an address of its
// family, a source needs the interface, and a gateway its source.
func checkSettings(in NodeSettings) (NodeSettings, error) {
	in.Interface = strings.TrimSpace(in.Interface)
	var v violations
	if in.Interface != "" && !interfacePattern.MatchString(in.Interface) {
		v.add("interface", "an interface name of 1-15 letters, digits, '.', '_', '@' or '-'")
	}
	address := func(field string, value *string, v6 bool) {
		*value = strings.TrimSpace(*value)
		if *value == "" {
			return
		}
		ip, err := netip.ParseAddr(*value)
		if err != nil || ip.Unmap().Is6() != v6 || ip.Zone() != "" {
			family := "IPv4"
			if v6 {
				family = "IPv6"
			}
			v.add(field, "an %s address", family)
			return
		}
		*value = ip.Unmap().String()
	}
	address("source_v4", &in.SourceV4, false)
	address("gateway_v4", &in.GatewayV4, false)
	address("source_v6", &in.SourceV6, true)
	address("gateway_v6", &in.GatewayV6, true)
	if in.Interface == "" && (in.SourceV4 != "" || in.SourceV6 != "") {
		v.add("interface", "a source address leaves by an interface: name it")
	}
	if in.GatewayV4 != "" && in.SourceV4 == "" {
		v.add("source_v4", "a gateway needs the interface's source address")
	}
	if in.GatewayV6 != "" && in.SourceV6 == "" {
		v.add("source_v6", "a gateway needs the interface's source address")
	}
	if in.HopPort > 65535 {
		v.add("hop_port", "a port of 1-65535, or 0 for %d", AutoHopPort)
	}
	return in, v.err()
}
