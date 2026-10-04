package forwardlegacy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Check paths.
const (
	PathAgent   = "agent"
	PathNodeX   = "nodex"
	PathAnsible = "ansible"
)

// PathNotNeeded is a path result for a path that had nothing to clean on
// the node.
const PathNotNeeded = "not_needed"

// NodeInfo is a forward node as a cleaner sees it.
type NodeInfo struct {
	ID   uint   `json:"id"`
	Ref  string `json:"node_ref"`
	Name string `json:"name"`
	Host string `json:"host"`
}

// PathResult is one path's result on one node.
type PathResult struct {
	Path string `json:"path"`
	// State is clean, dirty, unreachable or not_needed.
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
	// Items counts the legacy resources the path removed or checked.
	Items int `json:"items,omitempty"`
}

// Cleaner removes the old forward runtime of a node through one path
// (NodeX's HTTP API, the Ansible fallback) and verifies it is gone.
type Cleaner interface {
	CleanNode(ctx context.Context, node NodeInfo) PathResult
}

// NodeStatus is a forward node's standing in the upgrade.
type NodeStatus struct {
	NodeID uint   `json:"node_id"`
	Ref    string `json:"node_ref"`
	Name   string `json:"name"`
	// State is the node's standing: unchecked, clean, dirty, unreachable
	// or abandoned.
	State string `json:"state"`
	// CheckState is the latest check's own result.
	CheckState    string       `json:"check_state,omitempty"`
	Checks        []PathResult `json:"checks,omitempty"`
	Detail        string       `json:"detail,omitempty"`
	CheckedAt     *time.Time   `json:"checked_at,omitempty"`
	AbandonedAt   *time.Time   `json:"abandoned_at,omitempty"`
	AbandonedBy   string       `json:"abandoned_by,omitempty"`
	AbandonReason string       `json:"abandon_reason,omitempty"`
}

// Ready reports whether the node no longer holds up the drop.
func (s NodeStatus) Ready() bool { return s.State == StateClean || s.State == StateAbandoned }

// NodeRef is a forward node's reference (its Agent identity name).
func NodeRef(id uint) string { return "forward-" + strconv.FormatUint(uint64(id), 10) }

type inventoryNode struct {
	ID   uint
	Name string
	Host string
}

func listNodes(ctx context.Context, db *gorm.DB) ([]inventoryNode, error) {
	var nodes []inventoryNode
	if !db.Migrator().HasTable(&model.ForwardNode{}) {
		return nodes, nil
	}
	err := db.WithContext(ctx).Model(&model.ForwardNode{}).Select("id, name, host").Order("id").Scan(&nodes).Error
	return nodes, err
}

// ResolveNode finds a forward node by its reference (forward-<id>) or its
// name, which must then be unique.
func ResolveNode(ctx context.Context, db *gorm.DB, given string) (NodeInfo, error) {
	given = strings.TrimSpace(given)
	nodes, err := listNodes(ctx, db)
	if err != nil {
		return NodeInfo{}, err
	}
	var matches []inventoryNode
	for _, node := range nodes {
		if NodeRef(node.ID) == given || node.Name == given {
			matches = append(matches, node)
		}
	}
	switch len(matches) {
	case 0:
		return NodeInfo{}, fmt.Errorf("no forward node is named %q (give its name or forward-<id>)", given)
	case 1:
		return NodeInfo{ID: matches[0].ID, Ref: NodeRef(matches[0].ID), Name: matches[0].Name, Host: matches[0].Host}, nil
	}
	refs := make([]string, 0, len(matches))
	for _, match := range matches {
		refs = append(refs, NodeRef(match.ID))
	}
	return NodeInfo{}, fmt.Errorf("%d forward nodes are named %q (%s): give the node's forward-<id>", len(matches), given, strings.Join(refs, ", "))
}

// CheckOptions configure a check.
type CheckOptions struct {
	NodeX   Cleaner
	Ansible Cleaner
	// Nodes limits the check to these node names or references; empty
	// checks every forward node.
	Nodes []string
	Now   time.Time
}

// Check runs every path on the forward nodes, records each node's result
// and answers the nodes' standing.
func Check(ctx context.Context, db *gorm.DB, options CheckOptions) ([]NodeStatus, error) {
	if err := EnsureSchema(db); err != nil {
		return nil, err
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	nodes, err := listNodes(ctx, db)
	if err != nil {
		return nil, err
	}
	selected := nodes
	if len(options.Nodes) > 0 {
		selected = nil
		for _, given := range options.Nodes {
			info, err := ResolveNode(ctx, db, given)
			if err != nil {
				return nil, err
			}
			selected = append(selected, inventoryNode{ID: info.ID, Name: info.Name, Host: info.Host})
		}
	}
	agents, err := agentPaths(ctx, db, now)
	if err != nil {
		return nil, err
	}
	for _, node := range selected {
		info := NodeInfo{ID: node.ID, Ref: NodeRef(node.ID), Name: node.Name, Host: node.Host}
		results := []PathResult{agents(info)}
		for _, cleaner := range []struct {
			path    string
			cleaner Cleaner
		}{{PathNodeX, options.NodeX}, {PathAnsible, options.Ansible}} {
			if cleaner.cleaner == nil {
				continue
			}
			result := cleaner.cleaner.CleanNode(ctx, info)
			result.Path = cleaner.path
			results = append(results, result)
		}
		state, detail := combine(results)
		if err := recordCheck(ctx, db, info, state, detail, results, now); err != nil {
			return nil, err
		}
	}
	return Status(ctx, db)
}

// combine answers a node's state from its path results: any dirty path
// makes it dirty, else any unreachable one unreachable, else clean.
func combine(results []PathResult) (string, string) {
	var dirty, unreachable []string
	for _, result := range results {
		line := result.Path + ": " + result.Detail
		switch result.State {
		case StateDirty:
			dirty = append(dirty, line)
		case StateUnreachable:
			unreachable = append(unreachable, line)
		}
	}
	switch {
	case len(dirty) > 0:
		return StateDirty, strings.Join(append(dirty, unreachable...), "; ")
	case len(unreachable) > 0:
		return StateUnreachable, strings.Join(unreachable, "; ")
	}
	for _, result := range results {
		if result.State == StateClean {
			return StateClean, result.Path + ": " + result.Detail
		}
	}
	return StateClean, "no legacy forward runtime was placed on this node (no clean agent, no Agent channel to verify, and no flux forward, rule or Ansible machine references it)"
}

// agentPaths answers each node's Agent path result. An enrolled Agent was
// installed by install.sh, which removes the old runtime locally before it
// enrolls (O1): the tables inet v2b_forward, ip v2b_forward and
// ip anixops_forward and the clean agent's v2forward-agent unit. A node
// still on the clean agent, or on another legacy channel, cannot be
// verified.
func agentPaths(ctx context.Context, db *gorm.DB, now time.Time) (func(NodeInfo) PathResult, error) {
	inventory, err := agenttransport.Build(ctx, db, agenttransport.Policy{}, agenttransport.Options{Now: now})
	if err != nil {
		return nil, fmt.Errorf("read the Agent transports: %w", err)
	}
	byID := map[uint]agenttransport.NodeTransports{}
	for _, node := range inventory.Nodes {
		if node.Kind == agentcontrol.NodeKindForward {
			byID[node.ID] = node
		}
	}
	cleanAgents := map[uint]bool{}
	if db.Migrator().HasTable(&model.ForwardCleanAgent{}) {
		var ids []uint
		if err := db.WithContext(ctx).Model(&model.ForwardCleanAgent{}).Where("node_id IS NOT NULL").Pluck("node_id", &ids).Error; err != nil {
			return nil, err
		}
		for _, id := range ids {
			cleanAgents[id] = true
		}
	}
	return func(info NodeInfo) PathResult {
		result := PathResult{Path: PathAgent}
		node, known := byID[info.ID]
		enrolled := known && (node.Status == agenttransport.StatusMTLS ||
			(node.Status == agenttransport.StatusUnseen && node.Certificate != nil))
		switch {
		case enrolled:
			result.State = StateClean
			result.Detail = "the node runs the enrolled AnixOps Agent" + versionSuffix(node.AgentVersion) +
				"; its installer removed the legacy forward runtime (inet/ip v2b_forward, ip anixops_forward, v2forward-agent)"
		case known && node.Status == agenttransport.StatusLegacy:
			result.State = StateUnreachable
			result.Detail = "the node is still on a legacy Agent channel (" + node.Transport + "): install the new Agent with the node's install command, which removes the legacy runtime"
		case cleanAgents[info.ID]:
			result.State = StateUnreachable
			result.Detail = "the node had a clean agent and no enrolled Agent: install the new Agent, which removes the clean agent and its tables"
		default:
			result.State = PathNotNeeded
			result.Detail = "no Agent enrolled"
		}
		return result
	}, nil
}

func versionSuffix(version string) string {
	if strings.TrimSpace(version) == "" {
		return ""
	}
	return " " + version
}

func recordCheck(ctx context.Context, db *gorm.DB, info NodeInfo, state, detail string, results []PathResult, now time.Time) error {
	checks, err := json.Marshal(results)
	if err != nil {
		return err
	}
	checkedAt := now.UTC()
	row := model.ForwardLegacyNode{
		NodeID: info.ID, NodeRef: info.Ref, NodeName: info.Name, State: state, Checks: string(checks),
		Detail: detail, CheckedAt: &checkedAt, CreatedAt: checkedAt, UpdatedAt: checkedAt,
	}
	updates := map[string]any{
		"node_ref": info.Ref, "node_name": info.Name, "state": state, "checks": string(checks),
		"detail": detail, "checked_at": &checkedAt, "updated_at": checkedAt,
	}
	// A node found dirty is reachable again: its abandonment no longer
	// stands, it must be cleaned.
	if state == StateDirty {
		updates["abandoned_at"] = nil
		updates["abandoned_by"] = ""
		updates["abandon_reason"] = ""
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_id"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&row).Error
}

// Status answers every forward node's standing, unchecked nodes included.
func Status(ctx context.Context, db *gorm.DB) ([]NodeStatus, error) {
	nodes, err := listNodes(ctx, db)
	if err != nil {
		return nil, err
	}
	records := map[uint]model.ForwardLegacyNode{}
	if db.Migrator().HasTable(&model.ForwardLegacyNode{}) {
		var rows []model.ForwardLegacyNode
		if err := db.WithContext(ctx).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			records[row.NodeID] = row
		}
	}
	statuses := make([]NodeStatus, 0, len(nodes))
	for _, node := range nodes {
		status := NodeStatus{NodeID: node.ID, Ref: NodeRef(node.ID), Name: node.Name, State: StateUnchecked}
		if record, ok := records[node.ID]; ok {
			status.CheckState = record.State
			status.State = record.State
			status.Detail = record.Detail
			status.CheckedAt = record.CheckedAt
			_ = json.Unmarshal([]byte(record.Checks), &status.Checks)
			if record.AbandonedAt != nil && record.State != StateClean {
				status.State = StateAbandoned
				status.AbandonedAt, status.AbandonedBy, status.AbandonReason = record.AbandonedAt, record.AbandonedBy, record.AbandonReason
			}
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// ErrNotUnreachable refuses abandoning a node that is not unreachable.
var ErrNotUnreachable = errors.New("only a node whose latest check found it unreachable can be abandoned")

// Abandon marks an unreachable node as abandoned: the drop no longer waits
// for it. Its old runtime, if any, stays on the machine.
func Abandon(ctx context.Context, db *gorm.DB, given, reason, actor string, now time.Time) (NodeStatus, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return NodeStatus{}, errors.New("a reason is required (--reason)")
	}
	if err := EnsureSchema(db); err != nil {
		return NodeStatus{}, err
	}
	info, err := ResolveNode(ctx, db, given)
	if err != nil {
		return NodeStatus{}, err
	}
	var record model.ForwardLegacyNode
	err = db.WithContext(ctx).Where("node_id = ?", info.ID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return NodeStatus{}, fmt.Errorf("%s (%s) was never checked: run forward legacy check first: %w", info.Ref, info.Name, ErrNotUnreachable)
	}
	if err != nil {
		return NodeStatus{}, err
	}
	if record.State != StateUnreachable {
		return NodeStatus{}, fmt.Errorf("%s (%s) is %s: %w", info.Ref, info.Name, record.State, ErrNotUnreachable)
	}
	abandonedAt := now.UTC()
	if err := db.WithContext(ctx).Model(&model.ForwardLegacyNode{}).Where("id = ?", record.ID).Updates(map[string]any{
		"abandoned_at": &abandonedAt, "abandoned_by": actor, "abandon_reason": reason, "updated_at": abandonedAt,
	}).Error; err != nil {
		return NodeStatus{}, err
	}
	statuses, err := Status(ctx, db)
	if err != nil {
		return NodeStatus{}, err
	}
	for _, status := range statuses {
		if status.NodeID == info.ID {
			return status, nil
		}
	}
	return NodeStatus{}, fmt.Errorf("%s disappeared", info.Ref)
}
