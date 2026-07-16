package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/v3/api/grpc/agent/v1"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/service"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const kernelOperationDispatchRetryAfter = 30 * time.Second

const (
	maxKernelAgentNodeID      = uint64(^uint32(0))
	maxKernelObservedRevision = uint64(1<<63 - 1)
)

func kernelAgentNodeID(nodeID uint) (uint32, error) {
	if uint64(nodeID) > maxKernelAgentNodeID {
		return 0, fmt.Errorf("node id %d exceeds Agent uint32 range", nodeID)
	}
	return uint32(nodeID), nil // #nosec G115 -- the value is bounded above.
}

func kernelAgentRevision(revision int64) (uint64, error) {
	if revision <= 0 {
		return 0, fmt.Errorf("revision %d is not positive", revision)
	}
	return uint64(revision), nil // #nosec G115 -- non-positive values are rejected above.
}

func kernelObservedRevision(revision uint64) (int64, bool) {
	if revision > maxKernelObservedRevision {
		return 0, false
	}
	return int64(revision), true // #nosec G115 -- the value is bounded above.
}

// kernelOperationStream is deliberately narrower than AgentControlManager so
// durable-operation behavior can be exercised without a live gRPC listener.
type kernelOperationStream interface {
	Connection(uint32) (AgentControlSnapshot, bool)
	DispatchOperation(context.Context, uint32, *agentv1pb.DesiredOperation) (*agentv1pb.OperationAck, error)
	CancelOperation(context.Context, uint32, string, uint64) error
	AddObservedStateHandler(ObservedStateHandler)
}

// KernelOperationBridge joins durable Kernel operations to the ephemeral Agent
// stream. The database stays authoritative: the stream supplies only the
// active session and delivery transport.
type KernelOperationBridge struct {
	db          *gorm.DB
	stream      kernelOperationStream
	now         func() time.Time
	retryAfter  time.Duration
	recoverOnce sync.Once
	recoverErr  error
}

func NewKernelOperationBridge(db *gorm.DB, stream kernelOperationStream) (*KernelOperationBridge, error) {
	if db == nil {
		return nil, errors.New("kernel operation bridge requires a database")
	}
	if stream == nil {
		return nil, errors.New("kernel operation bridge requires an Agent control stream")
	}
	bridge := &KernelOperationBridge{
		db: db, stream: stream, now: time.Now, retryAfter: kernelOperationDispatchRetryAfter,
	}
	stream.AddObservedStateHandler(bridge.recordObserved)
	return bridge, nil
}

// RunOnce dispatches all pending node operations and recovers dispatch attempts
// whose Control process stopped before an ACK could be persisted.
func (b *KernelOperationBridge) RunOnce(ctx context.Context) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	b.recoverOnce.Do(func() {
		b.recoverErr = b.db.Model(&model.KernelOperation{}).
			Where("node_id IS NOT NULL AND state IN ?", []string{"dispatching", "running"}).
			Updates(map[string]any{
				"state": "pending", "session_id": "",
				"last_error": "Control restarted before terminal observation; replaying durable operation",
			}).Error
	})
	if b.recoverErr != nil {
		return 0, b.recoverErr
	}
	now := b.now()
	if _, err := service.ExpireKernelOperations(b.db, now); err != nil {
		return 0, err
	}
	if err := b.dispatchCancellations(ctx); err != nil {
		return 0, err
	}
	staleBefore := now.Add(-b.retryAfter)
	if err := b.db.Model(&model.KernelOperation{}).
		Where("state = ? AND dispatched_at IS NOT NULL AND dispatched_at <= ?", "dispatching", staleBefore).
		Updates(map[string]any{"state": "pending", "last_error": "dispatch acknowledgement was not recorded; retrying"}).Error; err != nil {
		return 0, err
	}

	var operations []model.KernelOperation
	if err := b.db.Where("node_id IS NOT NULL AND state = ?", "pending").Order("node_id, revision, created_at").Find(&operations).Error; err != nil {
		return 0, err
	}

	dispatched := 0
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return dispatched, err
		}
		ok, err := b.dispatchOne(ctx, operation)
		if err != nil {
			return dispatched, err
		}
		if ok {
			dispatched++
		}
	}
	return dispatched, nil
}

func (b *KernelOperationBridge) dispatchCancellations(ctx context.Context) error {
	var operations []model.KernelOperation
	if err := b.db.Where(
		"node_id IS NOT NULL AND state IN ? AND dispatched_at IS NOT NULL AND cancel_dispatched_at IS NULL",
		[]string{"cancel_requested", "timed_out"},
	).Order("created_at, id").Find(&operations).Error; err != nil {
		return err
	}
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if operation.NodeID == nil {
			continue
		}
		nodeID, err := kernelAgentNodeID(*operation.NodeID)
		if err != nil {
			_ = b.recordDispatchError(operation.ID, err.Error())
			continue
		}
		revision, err := kernelAgentRevision(operation.Revision)
		if err != nil {
			_ = b.recordDispatchError(operation.ID, err.Error())
			continue
		}
		if _, connected := b.stream.Connection(nodeID); !connected {
			continue
		}
		if err := b.stream.CancelOperation(ctx, nodeID, operation.ID, revision); err != nil {
			if err := b.db.Model(&model.KernelOperation{}).Where("id = ?", operation.ID).
				Update("last_error", "cancellation delivery failed: "+err.Error()).Error; err != nil {
				return err
			}
			continue
		}
		dispatchedAt := b.now()
		if err := b.db.Model(&model.KernelOperation{}).
			Where("id = ? AND cancel_dispatched_at IS NULL", operation.ID).
			Update("cancel_dispatched_at", dispatchedAt).Error; err != nil {
			return err
		}
	}
	return nil
}

// Start runs durable dispatch until ctx is cancelled. Callers supply logging so
// this package remains usable by tests and non-server control processes.
func (b *KernelOperationBridge) Start(ctx context.Context, interval time.Duration, reportError func(error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	go func() {
		run := func() {
			if _, err := b.RunOnce(ctx); err != nil && reportError != nil && !errors.Is(err, context.Canceled) {
				reportError(err)
			}
		}
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

func (b *KernelOperationBridge) dispatchOne(ctx context.Context, operation model.KernelOperation) (bool, error) {
	if operation.NodeID == nil {
		return false, nil
	}
	if !service.IsAgentPluginOperation(operation.Kind) {
		return false, b.failOperation(operation.ID, "operation kind is not dispatchable to the Agent Supervisor")
	}
	nodeID, err := kernelAgentNodeID(*operation.NodeID)
	if err != nil {
		return false, b.failOperation(operation.ID, err.Error())
	}
	snapshot, connected := b.stream.Connection(nodeID)
	if !connected || strings.TrimSpace(snapshot.SessionID) == "" {
		return false, nil
	}

	claimedAt := b.now()
	claim := b.db.Model(&model.KernelOperation{}).
		Where("id = ? AND state = ?", operation.ID, "pending").
		Updates(map[string]any{"state": "dispatching", "session_id": snapshot.SessionID, "dispatched_at": claimedAt, "last_error": ""})
	if claim.Error != nil {
		return false, claim.Error
	}
	if claim.RowsAffected == 0 {
		return false, nil
	}

	operation.SessionID = snapshot.SessionID
	operation.State = "dispatching"
	operation.DispatchedAt = &claimedAt
	desired, err := kernelOperationDesired(operation)
	if err != nil {
		return false, b.failOperation(operation.ID, err.Error())
	}
	deadlineCtx, cancel := context.WithDeadline(ctx, *operation.DeadlineAt)
	defer cancel()
	ack, err := b.stream.DispatchOperation(deadlineCtx, nodeID, desired)
	if err != nil {
		// Retain dispatching state: a reconnect replays the in-memory desired
		// operation, while a later worker run recovers an interrupted process.
		return false, b.recordDispatchError(operation.ID, err.Error())
	}
	if !ack.Accepted {
		return false, b.failOperation(operation.ID, strings.TrimSpace(ack.Error))
	}
	ackAt := b.now()
	return true, b.db.Model(&model.KernelOperation{}).
		Where("id = ? AND state = ?", operation.ID, "dispatching").
		Updates(map[string]any{"state": "running", "acknowledged_at": ackAt, "last_error": ""}).Error
}

func kernelOperationDesired(operation model.KernelOperation) (*agentv1pb.DesiredOperation, error) {
	if operation.NodeID == nil || operation.Revision <= 0 || operation.DeadlineAt == nil {
		return nil, errors.New("stored operation is missing node revision or deadline")
	}
	if operation.EnvelopeVersion != service.KernelOperationEnvelopeVersion {
		return nil, fmt.Errorf("unsupported stored operation envelope version %q", operation.EnvelopeVersion)
	}
	canonical, err := service.CanonicalKernelOperationConfig(operation.ConfigJSON)
	if err != nil {
		return nil, err
	}
	if canonical != operation.ConfigJSON {
		return nil, errors.New("stored operation config is not canonical")
	}
	revision, err := kernelAgentRevision(operation.Revision)
	if err != nil {
		return nil, err
	}
	envelope, err := json.Marshal(kernelOperationEnvelopePayload{
		Version: service.KernelOperationEnvelopeVersion, OperationID: operation.ID,
		IdempotencyKey: operation.IdempotencyKey, SessionID: operation.SessionID,
		Revision: revision, PluginID: operation.PluginID,
		TargetVersion: operation.TargetVersion, ConfigHash: operation.ConfigHash,
		Config: json.RawMessage(operation.ConfigJSON),
	})
	if err != nil {
		return nil, fmt.Errorf("encode operation envelope: %w", err)
	}
	return &agentv1pb.DesiredOperation{
		OperationId: operation.ID, Kind: operation.Kind, Revision: revision,
		PayloadJson: envelope, DeadlineUnixMs: operation.DeadlineAt.UnixMilli(),
	}, nil
}

func (b *KernelOperationBridge) recordDispatchError(operationID, message string) error {
	return b.db.Model(&model.KernelOperation{}).Where("id = ? AND state = ?", operationID, "dispatching").
		Updates(map[string]any{"last_error": strings.TrimSpace(message)}).Error
}

func (b *KernelOperationBridge) failOperation(operationID, message string) error {
	if strings.TrimSpace(message) == "" {
		message = "Agent rejected operation"
	}
	return b.db.Model(&model.KernelOperation{}).Where("id = ? AND state IN ?", operationID, []string{"pending", "dispatching", "running"}).
		Updates(map[string]any{"state": "failed", "last_error": message}).Error
}

func (b *KernelOperationBridge) recordObserved(nodeID uint32, observed *agentv1pb.ObservedState) {
	if observed == nil || strings.TrimSpace(observed.OperationId) == "" {
		return
	}
	observedRevision, validRevision := kernelObservedRevision(observed.Revision)
	if !validRevision {
		return
	}
	observedAt := b.now()
	if err := b.db.Transaction(func(tx *gorm.DB) error {
		var operation model.KernelOperation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
			&operation, "id = ? AND node_id = ? AND revision = ?", observed.OperationId, nodeID, observedRevision,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		state, message := kernelObservedState(observed)
		terminalObserved := isTerminalKernelObserved(observed.Phase)
		updates := map[string]any{}
		switch {
		case operation.State == "timed_out":
			// A late Agent result is evidence, not permission to reverse a
			// deadline decision made by the durable Control state machine.
		case operation.State == "cancel_requested":
			if terminalObserved {
				updates["state"] = "cancelled"
				updates["observed_at"] = observedAt
				if message == "" {
					message = "operation cancelled"
				}
				updates["last_error"] = message
			}
		case isTerminalKernelOperationState(operation.State):
			// Terminal database state is monotonic. Duplicate or reordered
			// observations cannot move it back to running or another outcome.
		default:
			updates["state"], updates["observed_at"], updates["last_error"] = state, observedAt, message
		}
		if len(updates) > 0 && len(observed.StateJson) > 0 {
			updates["result_json"] = string(observed.StateJson)
		}
		if len(updates) > 0 {
			if err := tx.Model(&operation).Updates(updates).Error; err != nil {
				return err
			}
		}
		cursor := model.NodeOperationRevision{NodeID: uint(nodeID)}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cursor).Error; err != nil {
			return err
		}
		var current model.NodeOperationRevision
		if err := tx.First(&current, "node_id = ?", nodeID).Error; err != nil {
			return err
		}
		if observedRevision > current.ObservedRevision {
			if err := tx.Model(&current).Update("observed_revision", observedRevision).Error; err != nil {
				return err
			}
		}
		// Topology deployment steps use ordinary Agent plugin operations so the
		// package receives exactly its signed config document. The deployment
		// executor owns topology aggregation after this operation row becomes
		// terminal; writing a topology success here would incorrectly mark a
		// node complete while another vertex on that node is still applying.
		if operation.TopologyDeploymentID != nil {
			return nil
		}
		// Keep the pre-executor topology.* envelope compatibility path for
		// persisted 3.1 preview rows. New deployments never place topology
		// metadata inside plugin config JSON.
		var topologyRef struct {
			DeploymentID    uint  `json:"deployment_id"`
			DesiredRevision int64 `json:"desired_revision"`
		}
		if strings.HasPrefix(operation.Kind, "topology.") && json.Unmarshal([]byte(operation.ConfigJSON), &topologyRef) == nil && topologyRef.DeploymentID != 0 {
			desiredRevision := topologyRef.DesiredRevision
			if desiredRevision == 0 {
				desiredRevision = operation.Revision
			}
			topologyState := "applying"
			switch observed.Phase {
			case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED:
				topologyState = "succeeded"
			case agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED:
				topologyState = "failed"
			case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED:
				topologyState = "rolled_back"
			}
			if _, _, err := service.ApplyTopologyObservedStateTx(tx, service.TopologyObservedStateUpdate{
				DeploymentID: topologyRef.DeploymentID, NodeID: uint(nodeID),
				DesiredRevision: desiredRevision, ObservedRevision: observedRevision,
				State: topologyState, HealthJSON: string(observed.StateJson), LastError: message,
				ObservedAt: observedAt,
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		// Stream correctness must not depend on a database callback. The durable
		// worker will reconcile nonterminal rows on its next pass.
		return
	}
}

func isTerminalKernelObserved(phase agentv1pb.ObservedPhase) bool {
	switch phase {
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
		agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED,
		agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED:
		return true
	default:
		return false
	}
}

func isTerminalKernelOperationState(state string) bool {
	switch state {
	case "succeeded", "failed", "superseded", "cancelled", "timed_out":
		return true
	default:
		return false
	}
}

func kernelObservedState(observed *agentv1pb.ObservedState) (string, string) {
	message := strings.TrimSpace(observed.Message)
	switch observed.Phase {
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_ACCEPTED, agentv1pb.ObservedPhase_OBSERVED_PHASE_APPLYING:
		return "running", message
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED:
		return "succeeded", ""
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED:
		return "superseded", message
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED:
		if message == "" {
			message = "Agent reported operation failure"
		}
		return "failed", message
	default:
		return "running", message
	}
}
