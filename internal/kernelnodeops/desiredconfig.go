package kernelnodeops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DesiredConfigFormat names the desired configuration document's schema
// (node-ops-service.md section 5.5): a proxy node's protocols as the
// kernel's builder renders them for the node, with its raw configuration
// and the UniProxy answers (legacy_pull); a forward node's legacy rules and
// tunnels.
const DesiredConfigFormat = "anixops.nodeconfig/v1"

// ErrNodeGone reports a node whose row no longer exists.
var ErrNodeGone = errors.New("node does not exist")

// DesiredConfig is a node's desired configuration as the kernel built it
// from the current rows. The document holds the node's runtime secrets; it
// is stored in the protected table and never answered.
type DesiredConfig struct {
	Node     agentcontrol.AgentNode
	Format   string
	Document map[string]any
	// JSON is the canonical document: sorted keys, no whitespace. Hash is
	// its SHA-256 (hex). The same configuration always has the same hash.
	JSON []byte
	Hash string
	// ExcludedProtocols counts the protocol rows left out because they
	// failed validation; zero while validation is report-only (section 3.8).
	ExcludedProtocols uint32
}

// BuildDesiredConfig builds node's desired configuration from its rows.
// ErrNodeGone when the node does not exist.
func BuildDesiredConfig(db *gorm.DB, node agentcontrol.AgentNode) (*DesiredConfig, error) {
	var document map[string]any
	var err error
	switch node.Kind {
	case agentcontrol.NodeKindProxy:
		document, err = buildProxyDesiredConfig(db, uint(node.ID))
	case agentcontrol.NodeKindForward:
		document, err = buildForwardDesiredConfig(db, uint(node.ID))
	default:
		return nil, fmt.Errorf("node kind %q has no desired configuration", node.Kind)
	}
	if err != nil {
		return nil, err
	}
	return newDesiredConfig(node, document)
}

func newDesiredConfig(node agentcontrol.AgentNode, document map[string]any) (*DesiredConfig, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode desired configuration: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return &DesiredConfig{Node: node, Format: DesiredConfigFormat, Document: document, JSON: encoded, Hash: hex.EncodeToString(sum[:])}, nil
}

// buildProxyDesiredConfig renders a proxy node: its enabled protocols
// through the kernel's builder (service.BuildNodeProtocolConfig, the one
// source UniProxy and the gRPC node service share) and its raw
// configuration.
func buildProxyDesiredConfig(db *gorm.DB, id uint) (map[string]any, error) {
	var nodes []model.Node
	if err := db.Where("id = ?", id).Limit(1).Find(&nodes).Error; err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, ErrNodeGone
	}
	node := nodes[0]
	var protocols []model.NodeProtocol
	if err := db.Where("node_id = ?", id).Order("sort ASC, id ASC").Find(&protocols).Error; err != nil {
		return nil, err
	}
	document := map[string]any{
		"kind": agentcontrol.NodeKindProxy,
		"node": map[string]any{
			"id": node.ID, "name": node.Name, "host": node.Host, "port": node.Port, "status": int(node.Status),
			"group_id": node.GroupID, "parent_id": node.ParentID,
		},
		"raw_config": nil,
	}
	// The raw configuration as the node's pull reads it: its secrets
	// through the node credential split, in the table's phase.
	nodesecrets.ResolveNodeRawConfig(db, &node)
	if node.RawConfig != nil && *node.RawConfig != "" {
		var raw map[string]any
		if err := json.Unmarshal([]byte(*node.RawConfig), &raw); err != nil || raw == nil {
			document["raw_config_error"] = "raw_config is not a JSON object"
		} else {
			document["raw_config"] = raw
		}
	}
	rendered := make([]map[string]any, 0, len(protocols))
	for i := range protocols {
		protocol := &protocols[i]
		if protocol.Enable != 1 {
			continue
		}
		rendered = append(rendered, map[string]any{
			"id": protocol.ID, "name": protocol.Name, "type": string(protocol.Type), "port": protocol.Port,
			"show": protocol.Show, "group_id": protocol.GroupID, "config": service.BuildNodeProtocolConfig(&node, protocol),
		})
	}
	document["protocols"] = rendered
	document["legacy_pull"] = buildLegacyPull(db, id, rendered)
	return document, nil
}

// buildLegacyPull renders what the node's legacy pull answers
// (service.BuildUniProxyNodeConfig, the UniProxy configuration): "default"
// for a pull that names no node type, and "types" for each node type the
// node serves (its enabled protocols' and the default's), by normalized
// type. A pull that would fail leaves its entry out. The Agent Control
// stream's snapshot thus carries exactly what UniProxy gives the node,
// and no secret beyond it.
func buildLegacyPull(db *gorm.DB, id uint, protocols []map[string]any) map[string]any {
	pull := map[string]any{}
	candidates := make([]string, 0, len(protocols)+1)
	if answer, err := service.BuildUniProxyNodeConfig(db, id, ""); err == nil {
		pull["default"] = answer
		if nodeType, ok := answer["node_type"].(string); ok {
			candidates = append(candidates, nodeType)
		}
	}
	for _, protocol := range protocols {
		if nodeType, ok := protocol["type"].(string); ok {
			candidates = append(candidates, nodeType)
		}
	}
	types := map[string]any{}
	for _, candidate := range candidates {
		nodeType := service.NormalizeNodeType(candidate)
		if _, done := types[nodeType]; done || nodeType == "" {
			continue
		}
		if answer, err := service.BuildUniProxyNodeConfig(db, id, nodeType); err == nil {
			types[nodeType] = answer
		}
	}
	pull["types"] = types
	return pull
}

// buildForwardDesiredConfig renders a forward node: the legacy rules it
// relays or exits, as GET /forward/agent/rules serves them to its agent,
// and the tunnels it belongs to.
func buildForwardDesiredConfig(db *gorm.DB, id uint) (map[string]any, error) {
	var nodes []model.ForwardNode
	if err := db.Where("id = ?", id).Limit(1).Find(&nodes).Error; err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, ErrNodeGone
	}
	node := nodes[0]
	var rules []model.ForwardRule
	if err := db.Where("relay_node_id = ? OR exit_node_id = ?", id, id).Order("id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	var tunnels []model.ForwardTunnel
	if err := db.Where("in_node_id = ? OR out_node_id = ?", id, id).Order("id ASC").Find(&tunnels).Error; err != nil {
		return nil, err
	}
	renderedRules := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		role := "exit"
		if rule.RelayNodeID == id {
			role = "relay"
		}
		renderedRules = append(renderedRules, map[string]any{
			"id": rule.ID, "enabled": rule.Enabled, "listen_port": rule.ListenPort, "protocol": rule.Protocol,
			"target_host": rule.TargetHost, "target_port": rule.TargetPort, "role": role,
		})
	}
	renderedTunnels := make([]map[string]any, 0, len(tunnels))
	for _, tunnel := range tunnels {
		role := "exit"
		if tunnel.InNodeID == id {
			role = "entry"
		}
		renderedTunnels = append(renderedTunnels, map[string]any{"id": tunnel.ID, "name": tunnel.Name, "role": role})
	}
	return map[string]any{
		"kind": agentcontrol.NodeKindForward,
		"node": map[string]any{
			"id": node.ID, "name": node.Name, "type": node.Type, "host": node.Host, "port": node.Port,
			"api_port": node.APIPort, "metrics_port": node.MetricsPort, "enabled": node.Enabled,
		},
		"legacy_rules": renderedRules,
		"tunnels":      renderedTunnels,
	}, nil
}

// storeAttempts bounds the retries of a store that raced another writer
// of the same node.
const storeAttempts = 5

// StoreDesiredConfig records cfg as its node's desired configuration. The
// revision grows by one when the hash changed and stays when it did not,
// so it is monotonic per node and the same configuration keeps its
// revision. changed reports whether the hash moved. Two writers of one node
// are serialized by the row's revision: a lost race is retried.
func StoreDesiredConfig(ctx context.Context, db *gorm.DB, cfg *DesiredConfig, now time.Time) (model.KernelNodeDesiredConfig, bool, error) {
	if cfg == nil {
		return model.KernelNodeDesiredConfig{}, false, errors.New("desired configuration is required")
	}
	now = now.UTC()
	for attempt := 0; attempt < storeAttempts; attempt++ {
		var rows []model.KernelNodeDesiredConfig
		if err := db.WithContext(ctx).Where("node_kind = ? AND node_id = ?", cfg.Node.Kind, cfg.Node.ID).Limit(1).Find(&rows).Error; err != nil {
			return model.KernelNodeDesiredConfig{}, false, err
		}
		if len(rows) == 0 {
			row := model.KernelNodeDesiredConfig{
				NodeKind: cfg.Node.Kind, NodeID: uint64(cfg.Node.ID), Revision: 1, ConfigHash: cfg.Hash, Format: cfg.Format,
				ConfigJSON: string(cfg.JSON), ExcludedProtocols: cfg.ExcludedProtocols, BuiltAt: now, CreatedAt: now, UpdatedAt: now,
			}
			result := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			if result.Error != nil {
				return model.KernelNodeDesiredConfig{}, false, result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			return row, true, nil
		}
		row := rows[0]
		if row.ConfigHash == cfg.Hash {
			if err := db.WithContext(ctx).Model(&model.KernelNodeDesiredConfig{}).Where("id = ?", row.ID).
				Updates(map[string]any{"built_at": now, "updated_at": now, "format": cfg.Format, "excluded_protocols": cfg.ExcludedProtocols}).Error; err != nil {
				return model.KernelNodeDesiredConfig{}, false, err
			}
			row.BuiltAt, row.UpdatedAt, row.Format, row.ExcludedProtocols = now, now, cfg.Format, cfg.ExcludedProtocols
			return row, false, nil
		}
		result := db.WithContext(ctx).Model(&model.KernelNodeDesiredConfig{}).Where("id = ? AND revision = ?", row.ID, row.Revision).
			Updates(map[string]any{
				"revision": row.Revision + 1, "config_hash": cfg.Hash, "format": cfg.Format, "config_json": string(cfg.JSON),
				"excluded_protocols": cfg.ExcludedProtocols, "built_at": now, "updated_at": now,
			})
		if result.Error != nil {
			return model.KernelNodeDesiredConfig{}, false, result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		row.Revision++
		row.ConfigHash, row.Format, row.ConfigJSON, row.ExcludedProtocols = cfg.Hash, cfg.Format, string(cfg.JSON), cfg.ExcludedProtocols
		row.BuiltAt, row.UpdatedAt = now, now
		return row, true, nil
	}
	return model.KernelNodeDesiredConfig{}, false, fmt.Errorf("desired configuration of %s: another writer kept winning", cfg.Node)
}

// LoadDesiredConfig returns the stored desired configuration of node.
func LoadDesiredConfig(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode) (model.KernelNodeDesiredConfig, bool, error) {
	var rows []model.KernelNodeDesiredConfig
	if err := db.WithContext(ctx).Where("node_kind = ? AND node_id = ?", node.Kind, node.ID).Limit(1).Find(&rows).Error; err != nil {
		return model.KernelNodeDesiredConfig{}, false, err
	}
	if len(rows) == 0 {
		return model.KernelNodeDesiredConfig{}, false, nil
	}
	return rows[0], true, nil
}
