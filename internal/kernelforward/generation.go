package kernelforward

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/sdk/forward/planner"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// Generation recovery (forward-sdk.md section 8.2, PROTOCOL.md
// "Forwarding"). An Agent applies only a newer generation than the one it
// holds, and refuses the generation it holds with another state_hash. After
// Control's database is reset or restored from a backup, a node's stored
// generation can be lower than the one its Agent holds, and every state
// Control sends would be ignored. The Agent reports the generation and
// state_hash it holds (NodeForwardReport), so Control recovers from the
// report alone; Agents need no change:
//
//   - A node's report is ahead of its stored state when its generation is
//     higher, or equal with another non-empty state_hash.
//   - Control then moves the node's stored generation to the reported one
//     when the state_hash is the same (the node already runs that state),
//     else to the reported one plus one, keeping the node's hops, and
//     pushes the node's configuration. RecordReport does this as soon as
//     it stores such a report; every plan stamps from the larger of the
//     stored and the reported generation too, which covers a node without
//     a stored state yet.
//   - ResetNode is the operator's escape hatch: it moves the node's
//     generation above both and pushes it.

// ErrNoState: the node has no stored forwarding state (no accepted plan
// covered it yet).
var ErrNoState = errors.New("forward node has no stored forwarding state")

// Recovery reasons (anixops_forward_generation_recoveries_total).
const (
	RecoveryReport   = "report"
	RecoveryPlan     = "plan"
	RecoveryOperator = "operator"
)

var recoveries = map[string]*atomic.Uint64{
	RecoveryReport: {}, RecoveryPlan: {}, RecoveryOperator: {},
}

func countRecovery(reason string) {
	if counter, ok := recoveries[reason]; ok {
		counter.Add(1)
	}
}

// reportAhead tells whether a node's reported generation is ahead of its
// stored one. An empty reported state_hash compares generations only.
func reportAhead(stored, reported planner.Generation) bool {
	switch {
	case reported.Generation > stored.Generation:
		return true
	case reported.Generation == stored.Generation && reported.Generation > 0:
		return reported.StateHash != "" && reported.StateHash != stored.StateHash
	default:
		return false
	}
}

// recovered answers the generation a node's state moves to when its report
// is ahead: the reported one for the same state_hash, else one above it.
func recovered(stored, reported planner.Generation) uint64 {
	if reported.StateHash != "" && reported.StateHash == stored.StateHash {
		return reported.Generation
	}
	return reported.Generation + 1
}

// loadReported answers the generation and state_hash of each node's latest
// report.
func loadReported(tx *gorm.DB) (map[string]planner.Generation, error) {
	var rows []model.KernelForwardNodeReport
	if err := tx.Select("node_ref", "generation", "state_hash").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: load node reports: %w", err)
	}
	out := make(map[string]planner.Generation, len(rows))
	for _, row := range rows {
		out[row.NodeRef] = planner.Generation{Generation: row.Generation, StateHash: row.StateHash}
	}
	return out, nil
}

// restamp moves a node's stored state to generation, re-encoding the state
// with it: the configuration carries the state's encoding.
func restamp(tx *gorm.DB, row model.KernelForwardNodeState, generation uint64) error {
	state, found, err := loadState(tx, row.NodeRef)
	if err != nil {
		return err
	}
	if !found {
		return ErrNoState
	}
	state.Generation = generation
	encoded, err := jsonWrite.Marshal(state)
	if err != nil {
		return fmt.Errorf("kernel forward: encode state of %s: %w", row.NodeRef, err)
	}
	if err := tx.Model(&model.KernelForwardNodeState{}).Where("node_ref = ?", row.NodeRef).Updates(map[string]any{
		"generation": generation, "state_json": string(encoded),
	}).Error; err != nil {
		return fmt.Errorf("kernel forward: store state of %s: %w", row.NodeRef, err)
	}
	return nil
}

func loadStateRow(tx *gorm.DB, nodeRef string) (model.KernelForwardNodeState, bool, error) {
	var rows []model.KernelForwardNodeState
	if err := tx.Select("node_ref", "generation", "state_hash").Where("node_ref = ?", nodeRef).Limit(1).Find(&rows).Error; err != nil {
		return model.KernelForwardNodeState{}, false, fmt.Errorf("kernel forward: load state of %s: %w", nodeRef, err)
	}
	if len(rows) == 0 {
		return model.KernelForwardNodeState{}, false, nil
	}
	return rows[0], true, nil
}

func logRecovery(reason, nodeRef string, from, reported, to uint64) {
	slog.Warn("forward generation recovered: the node holds a generation Control did not stamp (a reset or restored database)",
		"component", "kernel-forward", "reason", reason, "node", nodeRef,
		"stored_generation", from, "reported_generation", reported, "generation", to)
}

// recoverGeneration moves a node's stored generation past its latest
// report when the report is ahead, under the plan lock, and pushes the
// node. It answers whether the generation moved.
func (s *Service) recoverGeneration(ctx context.Context, nodeRef string) (bool, error) {
	db, err := s.db(ctx)
	if err != nil {
		return false, err
	}
	var moved bool
	var from, reported, to uint64
	err = db.Transaction(func(tx *gorm.DB) error {
		if _, err := lockPlan(tx, s.now()); err != nil {
			return err
		}
		row, found, err := loadStateRow(tx, nodeRef)
		if err != nil || !found {
			// No stored state: the next accepted plan stamps above the
			// report (writeStates).
			return err
		}
		reports, err := loadReported(tx.Where("node_ref = ?", nodeRef))
		if err != nil {
			return err
		}
		stored := planner.Generation{Generation: row.Generation, StateHash: row.StateHash}
		report, ok := reports[nodeRef]
		if !ok || !reportAhead(stored, report) {
			return nil
		}
		from, reported, to = row.Generation, report.Generation, recovered(stored, report)
		if err := restamp(tx, row, to); err != nil {
			return err
		}
		moved = true
		return nil
	})
	if err != nil || !moved {
		return false, err
	}
	countRecovery(RecoveryReport)
	logRecovery(RecoveryReport, nodeRef, from, reported, to)
	notify([]string{nodeRef})
	return true, nil
}

// NodeReset is what ResetNode did.
type NodeReset struct {
	NodeRef string `json:"node_ref"`
	// Previous is the stored generation before; Reported the latest
	// report's (0 before the node's first).
	Previous   uint64 `json:"previous_generation"`
	Reported   uint64 `json:"reported_generation"`
	Generation uint64 `json:"generation"`
	StateHash  string `json:"state_hash"`
}

// ResetNode moves a node's forwarding generation above both its stored one
// and the one its latest report holds, keeping its hops, and pushes the
// node's configuration to its session in this process (another process's
// sessions get it with their next configuration refresh, within a minute).
// It is the operator's command (anix-control forward reset-node), never
// served on ForwardControl. ErrNoState when the node has no stored
// state.
func (s *Service) ResetNode(ctx context.Context, nodeRef string) (NodeReset, error) {
	node, err := agentcontrol.ParseAgentNode(nodeRef)
	if err != nil || (node.Kind != agentcontrol.NodeKindForward && node.Kind != agentcontrol.NodeKindProxy) {
		return NodeReset{}, fmt.Errorf("%w: node_ref %q is not a proxy-<id> or forward-<id> node", ErrInvalidRequest, nodeRef)
	}
	nodeRef = node.String()
	db, err := s.db(ctx)
	if err != nil {
		return NodeReset{}, err
	}
	var reset NodeReset
	err = db.Transaction(func(tx *gorm.DB) error {
		if _, err := lockPlan(tx, s.now()); err != nil {
			return err
		}
		row, found, err := loadStateRow(tx, nodeRef)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: %s", ErrNoState, nodeRef)
		}
		reports, err := loadReported(tx.Where("node_ref = ?", nodeRef))
		if err != nil {
			return err
		}
		reported := reports[nodeRef].Generation
		reset = NodeReset{NodeRef: nodeRef, Previous: row.Generation, Reported: reported, StateHash: row.StateHash}
		reset.Generation = max(row.Generation, reported) + 1
		return restamp(tx, row, reset.Generation)
	})
	if err != nil {
		return NodeReset{}, err
	}
	countRecovery(RecoveryOperator)
	logRecovery(RecoveryOperator, nodeRef, reset.Previous, reset.Reported, reset.Generation)
	notify([]string{nodeRef})
	return reset, nil
}
