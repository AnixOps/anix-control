package kernelnodeops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
)

// AgentOperationRequest is one Agent Control operation to send on a node's
// stream, as POST /admin/nodes/:id/agent-control/operations takes it.
type AgentOperationRequest struct {
	// OperationID is the stream's operation id; one is generated when
	// empty.
	OperationID string
	// Kind is agent.ping, node.reload or users.reload.
	Kind        string
	PayloadJSON []byte
	// Timeout bounds the wait for the acknowledgement:
	// DefaultAgentAckTimeout when zero, at most MaxAgentAckTimeout.
	Timeout time.Duration
}

// AgentOperation is a dispatched Agent Control operation.
type AgentOperation struct {
	Desired *agentv1pb.DesiredOperation
	Ack     *agentv1pb.OperationAck
	stream  *streamDispatch
}

// Release stops following the operation's observed states.
func (o *AgentOperation) Release() {
	if o != nil && o.stream != nil {
		o.stream.release()
	}
}

// DispatchAgentOperation sends one bounded, whitelisted Agent Control
// operation on node's stream and waits for the agent's acknowledgement, as
// the legacy route does. The desired operation is returned with the error
// of a failed dispatch, so a caller can show what was sent; the revision
// the agent acknowledged is written into it.
func DispatchAgentOperation(ctx context.Context, streams agentstreams.Streams, node agentcontrol.AgentNode, request AgentOperationRequest) (*AgentOperation, error) {
	if streams == nil {
		return nil, errors.New("agent control manager is unavailable")
	}
	kind := strings.TrimSpace(request.Kind)
	if !agentOperationKinds[kind] {
		return nil, fmt.Errorf("unsupported Agent Control operation %q", kind)
	}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = DefaultAgentAckTimeout
	}
	if timeout > MaxAgentAckTimeout {
		timeout = MaxAgentAckTimeout
	}
	operationID := strings.TrimSpace(request.OperationID)
	if operationID == "" {
		operationID = NewTaskID()
	}
	desired := &agentv1pb.DesiredOperation{
		OperationId: operationID, Kind: kind, PayloadJson: request.PayloadJSON, DeadlineUnixMs: time.Now().Add(timeout).UnixMilli(),
	}
	operation := &AgentOperation{Desired: desired}
	operation.stream = dispatchOnStream(ctx, streams, node, desired, timeout)
	operation.Ack = operation.stream.Ack
	if operation.stream.Err != nil {
		return operation, operation.stream.Err
	}
	return operation, nil
}

// agentOperationExecutor serves agent.operation.
type agentOperationExecutor struct {
	sources SourcesFunc
}

func (x agentOperationExecutor) Execute(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetAgentControlOperation()
	node := proxyAgentNode(op.GetNodeId())
	streams := x.sources().Streams
	if streams == nil {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE, "this kernel holds no Agent Control streams", true)
	}
	if _, connected := streams.Session(node); !connected {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE, fmt.Sprintf("%s is not connected to Agent Control", node), true)
	}
	operation, err := DispatchAgentOperation(ctx, streams, node, AgentOperationRequest{
		OperationID: op.GetAgentOperationId(), Kind: op.GetKind(), PayloadJSON: op.GetPayloadJson(),
		Timeout: ackTimeout(op.GetTimeoutSeconds()),
	})
	if err != nil {
		if operation == nil {
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, err.Error(), false)
		}
		return streamFailure(err).WithChannel(kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL)
	}
	defer operation.Release()
	result := &kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_AgentOperation{AgentOperation: &kernelnodeopsv1.AgentOperationResult{
		AgentOperationId: operation.Desired.GetOperationId(), Revision: operation.Ack.GetRevision(), Ack: agentAck(operation.Ack),
	}}}
	if err := run.Accept(ctx, Acceptance{Channel: kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL, NodeRevision: operation.Ack.GetRevision()}); err != nil {
		return ended(err)
	}
	if !operation.Ack.GetAccepted() {
		return rejected(operation.Ack).WithResult(result)
	}
	observed, err := operation.stream.awaitTerminal(ctx, streams, node)
	if err != nil {
		return awaitFailure(err).WithResult(result)
	}
	return observedOutcome(observed, result)
}
