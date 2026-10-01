package kernelnodeops

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The agent session RPCs (node-ops-service.md section 3.1) answer the live
// sessions only the kernel holds, to holders of kernel.nodeops.agents.v1.
// A session is described without its credentials: never a key, a token or
// a certificate. What an agent reported about itself (system information,
// observed state) is scrubbed like a result.

// agents returns the sources this server reads: its own, else the
// defaults registered at startup.
func (h *hostServer) agents() AgentSources {
	if h.server.Agents != nil {
		return *h.server.Agents
	}
	return DefaultAgentSources()
}

func transportEnum(transport agentstreams.Transport) kernelnodeopsv1.AgentTransport {
	switch transport {
	case agentstreams.TransportControlStream:
		return kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_CONTROL_STREAM
	case agentstreams.TransportWebSocket:
		return kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_WEBSOCKET
	}
	return kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_UNSPECIFIED
}

func unixMillisOf(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}

// sessionProto renders a session for the contract, scrubbing what the
// agent reported.
func sessionProto(session agentstreams.Session) *kernelnodeopsv1.AgentSession {
	rendered := &kernelnodeopsv1.AgentSession{
		Node:              nodeRef(nodeKindByName(session.Node.Kind), uint64(session.Node.ID)),
		Transport:         transportEnum(session.Transport),
		SessionId:         session.SessionID,
		AgentVersion:      session.AgentVersion,
		InstanceId:        session.InstanceID,
		Capabilities:      append([]string(nil), session.Capabilities...),
		ConnectedAtUnixMs: unixMillisOf(session.ConnectedAt),
		LastSeenUnixMs:    unixMillisOf(session.LastSeen),
		DesiredRevision:   session.DesiredRevision,
		ObservedRevision:  session.ObservedRevision,
		Identity:          session.Identity,
	}
	if session.System != nil {
		if encoded, err := json.Marshal(session.System); err == nil {
			rendered.SystemJson = scrubBytes(encoded, nil)
		}
	}
	return rendered
}

func observedProto(observed *agentv1pb.ObservedState) *kernelnodeopsv1.ObservedOperation {
	if observed == nil {
		return nil
	}
	return &kernelnodeopsv1.ObservedOperation{
		AgentOperationId: observed.GetOperationId(), Revision: observed.GetRevision(), Phase: strings.TrimPrefix(observed.GetPhase().String(), "OBSERVED_PHASE_"),
		Message: scrubText(observed.GetMessage(), nil), StateJson: scrubBytes(observed.GetStateJson(), nil),
		ObservedAtUnixMs: observed.GetObservedAtUnixMs(), SessionId: observed.GetSessionId(),
	}
}

// ListAgentSessions answers the live sessions of one transport, or of
// both, ordered by node kind, node id and transport.
func (h *hostServer) ListAgentSessions(ctx context.Context, request *kernelnodeopsv1.ListAgentSessionsRequest) (*kernelnodeopsv1.ListAgentSessionsResponse, error) {
	if err := h.configured(); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_AGENTS); err != nil {
		return nil, err
	}
	transport := request.GetTransport()
	if _, known := kernelnodeopsv1.AgentTransport_name[int32(transport)]; !known {
		return nil, status.Error(codes.InvalidArgument, "transport is unknown")
	}
	sources := h.agents()
	var sessions []agentstreams.Session
	if sources.Streams != nil && transport != kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_WEBSOCKET {
		sessions = append(sessions, sources.Streams.Sessions()...)
	}
	if sources.WebSockets != nil && transport != kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_CONTROL_STREAM {
		sessions = append(sessions, sources.WebSockets.Sessions()...)
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		a, b := sessions[i], sessions[j]
		if a.Node.Kind != b.Node.Kind {
			return a.Node.Kind < b.Node.Kind
		}
		if a.Node.ID != b.Node.ID {
			return a.Node.ID < b.Node.ID
		}
		return a.Transport < b.Transport
	})
	response := &kernelnodeopsv1.ListAgentSessionsResponse{}
	for _, session := range sessions {
		response.Sessions = append(response.Sessions, sessionProto(session))
	}
	return response, nil
}

// proxyNodeOf checks a session request's proxy node id and that the node
// exists.
func (h *hostServer) proxyNodeOf(ctx context.Context, nodeID uint64) (agentcontrol.AgentNode, error) {
	if err := checkID(nodeID, "node_id"); err != nil {
		return agentcontrol.AgentNode{}, err
	}
	if err := nodeExists(h.db(ctx), nodeRef(kernelnodeopsv1.NodeKind_NODE_KIND_PROXY, nodeID)); err != nil {
		return agentcontrol.AgentNode{}, err
	}
	return proxyAgentNode(nodeID), nil
}

// GetAgentSession answers a proxy node's Agent Control session and the
// last state its agent reported.
func (h *hostServer) GetAgentSession(ctx context.Context, request *kernelnodeopsv1.GetAgentSessionRequest) (*kernelnodeopsv1.GetAgentSessionResponse, error) {
	if err := h.configured(); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_AGENTS); err != nil {
		return nil, err
	}
	node, err := h.proxyNodeOf(ctx, request.GetNodeId())
	if err != nil {
		return nil, err
	}
	response := &kernelnodeopsv1.GetAgentSessionResponse{}
	streams := h.agents().Streams
	if streams == nil {
		return response, nil
	}
	if session, ok := streams.Session(node); ok {
		response.Connected = true
		response.Session = sessionProto(session)
	}
	if observed, ok := streams.Observed(node); ok {
		response.Observed = observedProto(observed)
	}
	return response, nil
}

// GetAgentMonitor answers the last monitor snapshot a proxy node's
// WebSocket agent posted.
func (h *hostServer) GetAgentMonitor(ctx context.Context, request *kernelnodeopsv1.GetAgentMonitorRequest) (*kernelnodeopsv1.GetAgentMonitorResponse, error) {
	if err := h.configured(); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_AGENTS); err != nil {
		return nil, err
	}
	node, err := h.proxyNodeOf(ctx, request.GetNodeId())
	if err != nil {
		return nil, err
	}
	response := &kernelnodeopsv1.GetAgentMonitorResponse{}
	agents := h.agents().WebSockets
	if agents == nil {
		return response, nil
	}
	system, receivedAt, ok := agents.Monitor(uint(node.ID))
	if !ok {
		return response, nil
	}
	encoded, err := json.Marshal(system)
	if err != nil {
		return nil, failure("get agent monitor", err)
	}
	response.Found = true
	response.SnapshotJson = scrubBytes(encoded, nil)
	response.ReceivedAtUnixMs = unixMillisOf(receivedAt)
	return response, nil
}
