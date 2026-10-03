package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
)

// AgentStreams is the kernel's Agent Control streams by node kind
// (agentstreams.Streams): proxy nodes stream in one manager and forward
// nodes in another, since their ids overlap. KernelNodeOps dispatches node
// operations through it (node-ops-service.md section 3.8).
type AgentStreams struct {
	proxy   *AgentControlManager
	forward *AgentControlManager
}

// NewAgentStreams returns the streams over the two managers.
func NewAgentStreams(proxy, forward *AgentControlManager) *AgentStreams {
	return &AgentStreams{proxy: proxy, forward: forward}
}

var agentStreams = NewAgentStreams(agentControlManager, forwardAgentControlManager)

// GetAgentStreams returns the streams of the process's managers.
func GetAgentStreams() *AgentStreams { return agentStreams }

func (s *AgentStreams) manager(node agentcontrol.AgentNode) *AgentControlManager {
	switch node.Kind {
	case agentcontrol.NodeKindProxy:
		return s.proxy
	case agentcontrol.NodeKindForward:
		return s.forward
	}
	return nil
}

// Session returns the node's current stream session.
func (s *AgentStreams) Session(node agentcontrol.AgentNode) (agentstreams.Session, bool) {
	manager := s.manager(node)
	if manager == nil {
		return agentstreams.Session{}, false
	}
	manager.mu.RLock()
	connection := manager.connections[node.ID]
	manager.mu.RUnlock()
	if connection == nil {
		return agentstreams.Session{}, false
	}
	return connection.session(node.Kind), true
}

// Sessions lists every stream session of both kinds.
func (s *AgentStreams) Sessions() []agentstreams.Session {
	var sessions []agentstreams.Session
	for _, entry := range []struct {
		kind    string
		manager *AgentControlManager
	}{{agentcontrol.NodeKindProxy, s.proxy}, {agentcontrol.NodeKindForward, s.forward}} {
		if entry.manager == nil {
			continue
		}
		entry.manager.mu.RLock()
		connections := make([]*AgentControlConnection, 0, len(entry.manager.connections))
		for _, connection := range entry.manager.connections {
			connections = append(connections, connection)
		}
		entry.manager.mu.RUnlock()
		for _, connection := range connections {
			sessions = append(sessions, connection.session(entry.kind))
		}
	}
	return sessions
}

// session describes the connection without its credentials.
func (c *AgentControlConnection) session(kind string) agentstreams.Session {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	agentMetrics, agentMetricsAt := c.agentMetricsCopyLocked()
	capabilities := make([]string, 0, len(c.Capabilities))
	for _, capability := range c.Capabilities {
		if capability != nil && capability.Name != "" {
			capabilities = append(capabilities, capability.Name)
		}
	}
	return agentstreams.Session{
		Node: agentcontrol.AgentNode{Kind: kind, ID: c.NodeID}, Transport: agentstreams.TransportControlStream,
		SessionID: c.SessionID, AgentVersion: c.AgentVersion, InstanceID: c.InstanceID, Capabilities: capabilities,
		ConnectedAt: c.ConnectedAt, LastSeen: c.LastSeen, DesiredRevision: c.DesiredRev, ObservedRevision: c.ObservedRev,
		Identity: c.identity(), Authentication: c.principal.authentication(), Certificate: c.principal.certificate(),
		NegotiatedCapabilities: c.negotiatedCapabilities(), AgentMetrics: agentMetrics, AgentMetricsAt: agentMetricsAt,
	}
}

// Observed returns the last observed state the node's agent reported.
func (s *AgentStreams) Observed(node agentcontrol.AgentNode) (*agentv1pb.ObservedState, bool) {
	manager := s.manager(node)
	if manager == nil {
		return nil, false
	}
	return manager.ObservedState(node.ID)
}

// Dispatch sends operation on the node's stream and waits for the
// acknowledgement. Its errors wrap the agentstreams sentinels and keep the
// manager's text.
func (s *AgentStreams) Dispatch(ctx context.Context, node agentcontrol.AgentNode, operation *agentv1pb.DesiredOperation) (*agentv1pb.OperationAck, error) {
	manager := s.manager(node)
	if manager == nil {
		return nil, classified{sentinel: agentstreams.ErrNotConnected, cause: fmt.Errorf("agent node %s has no stream of its kind", node)}
	}
	ack, err := manager.DispatchOperation(ctx, node.ID, operation)
	if err != nil {
		return nil, classify(err)
	}
	return ack, nil
}

// Cancel asks the agent to stop an operation it holds.
func (s *AgentStreams) Cancel(ctx context.Context, node agentcontrol.AgentNode, operationID string, revision uint64) error {
	manager := s.manager(node)
	if manager == nil {
		return classified{sentinel: agentstreams.ErrNotConnected, cause: fmt.Errorf("agent node %s has no stream of its kind", node)}
	}
	if err := manager.CancelOperation(ctx, node.ID, operationID, revision); err != nil {
		return classify(err)
	}
	return nil
}

// OnObserved registers handler with both managers.
func (s *AgentStreams) OnObserved(handler agentstreams.ObservedHandler) {
	if handler == nil {
		return
	}
	for _, entry := range []struct {
		kind    string
		manager *AgentControlManager
	}{{agentcontrol.NodeKindProxy, s.proxy}, {agentcontrol.NodeKindForward, s.forward}} {
		if entry.manager == nil {
			continue
		}
		kind := entry.kind
		entry.manager.AddObservedStateHandler(func(nodeID uint32, observed *agentv1pb.ObservedState) {
			handler(agentcontrol.AgentNode{Kind: kind, ID: nodeID}, observed)
		})
	}
}

// classified is a manager error with the agentstreams sentinel it means.
// Its text is the manager's, unchanged: the legacy routes show it.
type classified struct {
	sentinel error
	cause    error
}

func (e classified) Error() string { return e.cause.Error() }
func (e classified) Unwrap() error { return e.cause }

// Is matches the sentinel and whatever the cause matches.
func (e classified) Is(target error) bool {
	return errors.Is(e.sentinel, target)
}

// classify maps a manager error to its sentinel by the text the manager
// writes; the text itself is kept.
func classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "is not connected"), strings.Contains(text, "connection changed"):
		return classified{sentinel: agentstreams.ErrNotConnected, cause: err}
	case strings.Contains(text, "does not advertise capability"):
		return classified{sentinel: agentstreams.ErrCapabilityMissing, cause: err}
	case strings.Contains(text, "closed before operation ACK"):
		return classified{sentinel: agentstreams.ErrSessionClosed, cause: err}
	case strings.Contains(text, "is already pending"), strings.Contains(text, "is already desired"):
		return classified{sentinel: agentstreams.ErrOperationPending, cause: err}
	}
	return err
}
