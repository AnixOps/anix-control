package kernelnodeops

import (
	"context"
	"errors"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

// NodeReloadOperation is the Agent Control operation a node sync sends to
// an agent without config.v1: the agent re-reads its configuration over the
// legacy transport. An agent that negotiated config.v1 is pushed a
// ConfigSnapshot instead (A2-3).
const NodeReloadOperation = "node.reload"

// SyncNodeOptions adjust a node sync.
type SyncNodeOptions struct {
	// Force pushes even when the configuration did not change, as the
	// administrator's sync button does.
	Force bool
	// AckTimeout bounds the wait for the agent's acknowledgement;
	// DefaultAgentAckTimeout when zero.
	AckTimeout time.Duration
	// Now stamps the desired configuration; time.Now when nil.
	Now func() time.Time
}

// NodeSync is what a node sync did (node-ops-service.md section 5.5 and
// the node.sync kind of section 3.3).
type NodeSync struct {
	Node agentcontrol.AgentNode
	// Channel is AGENT_CONTROL when the node's agent holds a stream, else
	// LEGACY_PULL: the node's next periodic pull reads the configuration.
	Channel kernelnodeopsv1.Channel
	Config  *DesiredConfig
	// Stored is the desired configuration row after the sync; Changed is
	// true when its hash moved (and its revision grew).
	Stored  model.KernelNodeDesiredConfig
	Changed bool
	// Pushed is true when a node.reload or a ConfigSnapshot went out on
	// the stream: the configuration changed, the sync was forced, or (for
	// a snapshot) the agent had not applied the stored configuration.
	Pushed bool
	// Desired and Ack describe a node.reload push; Snapshot and SessionID
	// a ConfigSnapshot push (the agent negotiated config.v1). DispatchErr
	// is why either push failed.
	Desired     *agentv1pb.DesiredOperation
	Ack         *agentv1pb.OperationAck
	Snapshot    *agentv1pb.ConfigSnapshot
	SessionID   string
	DispatchErr error
	stream      *streamDispatch
	config      *configPush
}

// SyncNode rebuilds and stores a node's desired configuration from its
// rows, drops what the kernel caches about the node, and, when the node's
// agent holds an Agent Control stream, pushes the configuration: a
// ConfigSnapshot when the agent negotiated config.v1 (once the
// configuration changed, opts.Force is set, or the agent has not applied
// the stored configuration), else a node.reload through the existing
// control-stream operation path (once the configuration changed or
// opts.Force is set). Nodes on the legacy transports keep their stored
// configuration for their next pull. The legacy sync route and the
// node.sync executor call it. ErrNodeGone when the node does not exist.
func SyncNode(ctx context.Context, db *gorm.DB, streams agentstreams.Streams, node agentcontrol.AgentNode, opts SyncNodeOptions) (*NodeSync, error) {
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	cfg, err := BuildDesiredConfig(db, node)
	if err != nil {
		return nil, err
	}
	stored, changed, err := StoreDesiredConfig(ctx, db, cfg, now())
	if err != nil {
		return nil, err
	}
	sync := &NodeSync{Node: node, Channel: kernelnodeopsv1.Channel_CHANNEL_LEGACY_PULL, Config: cfg, Stored: stored, Changed: changed}
	if node.Kind == agentcontrol.NodeKindProxy {
		service.DropNodeCache(uint(node.ID))
	}
	if streams != nil {
		if _, connected := streams.Session(node); connected {
			sync.Channel = kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL
		}
	}
	if sync.Channel != kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL {
		if node.Kind == agentcontrol.NodeKindProxy {
			// The legacy pull: nothing is pushed; the kernel logs it as
			// before.
			_ = service.NewNodeService().SyncProtocolToNode(uint(node.ID))
		}
		return sync, nil
	}
	if configStreams, ok := streams.(agentstreams.ConfigStreams); ok && configStreams.ConfigNegotiated(node) {
		return sync, sync.pushSnapshot(ctx, db, configStreams, opts.Force)
	}
	if !changed && !opts.Force {
		return sync, nil
	}
	timeout := opts.AckTimeout
	if timeout <= 0 {
		timeout = DefaultAgentAckTimeout
	}
	sync.Pushed = true
	sync.Desired = &agentv1pb.DesiredOperation{
		OperationId: NewTaskID(), Kind: NodeReloadOperation, DeadlineUnixMs: now().Add(timeout).UnixMilli(),
	}
	sync.stream = dispatchOnStream(ctx, streams, node, sync.Desired, timeout)
	sync.Ack, sync.DispatchErr = sync.stream.Ack, sync.stream.Err
	return sync, nil
}

// pushSnapshot pushes the stored configuration as a ConfigSnapshot to an
// agent that negotiated config.v1, unless it is unchanged, not forced, and
// verified applied already.
func (s *NodeSync) pushSnapshot(ctx context.Context, db *gorm.DB, streams agentstreams.ConfigStreams, force bool) error {
	if !s.Changed && !force {
		applied, err := appliedDesired(ctx, db, s.Node, s.Stored)
		if err != nil {
			return err
		}
		if applied {
			return nil
		}
	}
	s.Pushed = true
	s.config = pushConfig(ctx, streams, s.Node, ConfigSnapshotOf(s.Stored))
	s.Snapshot, s.SessionID, s.DispatchErr = s.config.Snapshot, s.config.SessionID, s.config.Err
	return nil
}

// Release stops following the pushed operation's observed states, or the
// pushed snapshot's ConfigStatus; a caller that answers at the
// acknowledgement (or the send) calls it.
func (s *NodeSync) Release() {
	if s == nil {
		return
	}
	if s.stream != nil {
		s.stream.release()
	}
	if s.config != nil {
		s.config.release()
	}
}

// nodeSyncExecutor serves node.sync.
type nodeSyncExecutor struct {
	sources SourcesFunc
}

func (x nodeSyncExecutor) Execute(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetSyncNode()
	node := agentNodeOf(op.GetNode())
	streams := x.sources().Streams
	sync, err := SyncNode(ctx, run.DB(), streams, node, SyncNodeOptions{Force: op.GetForce(), Now: run.now})
	if err != nil {
		if errors.Is(err, ErrNodeGone) {
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, "the node no longer exists", false)
		}
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "the desired configuration could not be stored: "+err.Error(), true)
	}
	result := &kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_NodeSync{NodeSync: &kernelnodeopsv1.NodeSyncResult{
		Channel: sync.Channel, ConfigHash: sync.Stored.ConfigHash, ConfigRevision: sync.Stored.Revision,
		Changed: sync.Changed, ExcludedProtocols: sync.Stored.ExcludedProtocols,
	}}}
	if !sync.Pushed {
		if err := run.Accept(ctx, Acceptance{Channel: sync.Channel, Result: result}); err != nil {
			return ended(err)
		}
		return Succeeded(result)
	}
	defer sync.Release()
	if sync.DispatchErr != nil {
		return streamFailure(sync.DispatchErr).WithResult(result).WithChannel(sync.Channel)
	}
	if sync.Snapshot != nil {
		return awaitSnapshotStatus(ctx, run, sync, result)
	}
	nodeSync := result.GetNodeSync()
	nodeSync.AgentOperationId, nodeSync.Revision, nodeSync.Ack = sync.Desired.GetOperationId(), sync.Ack.GetRevision(), agentAck(sync.Ack)
	if err := run.Accept(ctx, Acceptance{Channel: sync.Channel, NodeRevision: sync.Ack.GetRevision(), Result: result}); err != nil {
		return ended(err)
	}
	if !sync.Ack.GetAccepted() {
		return rejected(sync.Ack).WithResult(result)
	}
	observed, err := sync.stream.awaitTerminal(ctx, streams, node)
	if err != nil {
		return awaitFailure(err).WithResult(result)
	}
	return observedOutcome(observed, result)
}

// awaitSnapshotStatus ends a node.sync that pushed a ConfigSnapshot on the
// agent's ConfigStatus: SUCCEEDED when the agent applied the snapshot's
// revision and hash, FAILED with the agent's error when it could not. The
// operation runs (node_revision is the configuration revision) until then
// or until its deadline. The result's ack is the status: accepted is
// applied, with the session and revision that answered.
func awaitSnapshotStatus(ctx context.Context, run *Run, sync *NodeSync, result *kernelnodeopsv1.OperationResult) Outcome {
	nodeSync := result.GetNodeSync()
	nodeSync.Revision, nodeSync.Snapshot = sync.Snapshot.GetConfigRevision(), true
	if err := run.Accept(ctx, Acceptance{Channel: sync.Channel, NodeRevision: sync.Snapshot.GetConfigRevision(), Result: result}); err != nil {
		return ended(err)
	}
	report, err := sync.config.awaitStatus(ctx)
	if err != nil {
		return awaitFailure(err).WithResult(result)
	}
	status := report.Status
	nodeSync.Ack = &kernelnodeopsv1.AgentAck{
		Accepted: status.GetApplied(), Error: status.GetError(), SessionId: report.SessionID, Revision: status.GetConfigRevision(),
	}
	if status.GetApplied() {
		return Succeeded(result)
	}
	message := status.GetError()
	if message == "" {
		message = "the agent could not apply the configuration"
	}
	if code := ConfigStatusErrorCode(status); code != "" {
		message = code + ": " + message
	}
	return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, message, true).WithResult(result)
}

// ended is the outcome when the operation ended (cancelled or timed out)
// before its executor could accept it: the engine keeps the ledger's state
// and records this as evidence.
func ended(err error) Outcome {
	if errors.Is(err, ErrOperationEnded) {
		return Cancelled("the operation ended before the agent's acknowledgement was recorded")
	}
	return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "the acknowledgement could not be recorded: "+err.Error(), true)
}
