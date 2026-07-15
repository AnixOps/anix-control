package grpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/v3/api/grpc/agent/v1"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	AgentProtocolVersion                 = "anix.agent.v1"
	defaultAgentHeartbeatIntervalSeconds = uint32(20)
)

type agentOperationKey struct {
	nodeID      uint32
	operationID string
}

// DesiredOperationDispatcher lets durable runtime workers push desired state
// without depending on the gRPC stream implementation.
type DesiredOperationDispatcher interface {
	DispatchOperation(context.Context, uint32, *agentv1pb.DesiredOperation) (*agentv1pb.OperationAck, error)
}

// ObservedStateHandler adapts terminal Agent state back into durable job state.
type ObservedStateHandler func(nodeID uint32, observed *agentv1pb.ObservedState)

// AgentControlConnection is the current bidirectional control stream for a node.
type AgentControlConnection struct {
	NodeID       uint32
	SessionID    string
	AgentVersion string
	InstanceID   string
	Capabilities []*agentv1pb.Capability
	ConnectedAt  time.Time
	LastSeen     time.Time
	DesiredRev   uint64
	ObservedRev  uint64
	stream       agentv1pb.AgentControlService_ControlStreamServer
	sendMu       sync.Mutex
	stateMu      sync.RWMutex
}

func (c *AgentControlConnection) send(message *agentv1pb.ControlToAgent) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.stream.Send(message)
}

func (c *AgentControlConnection) touch(observedRevision uint64) {
	c.stateMu.Lock()
	c.LastSeen = time.Now()
	if observedRevision > c.ObservedRev {
		c.ObservedRev = observedRevision
	}
	c.stateMu.Unlock()
}

// AgentControlSnapshot is a read-only view used by control-plane services.
type AgentControlSnapshot struct {
	NodeID       uint32    `json:"node_id"`
	SessionID    string    `json:"session_id"`
	AgentVersion string    `json:"agent_version"`
	InstanceID   string    `json:"instance_id"`
	Capabilities []string  `json:"capabilities"`
	ConnectedAt  time.Time `json:"connected_at"`
	LastSeen     time.Time `json:"last_seen"`
	DesiredRev   uint64    `json:"desired_revision"`
	ObservedRev  uint64    `json:"observed_revision"`
}

// AgentControlManager owns live streams and correlates desired operations with ACKs.
type AgentControlManager struct {
	mu               sync.RWMutex
	connections      map[uint32]*AgentControlConnection
	desiredRevision  map[uint32]uint64
	desired          map[uint32]map[string]*agentv1pb.DesiredOperation
	observed         map[uint32]*agentv1pb.ObservedState
	pending          map[agentOperationKey]chan *agentv1pb.OperationAck
	observedHandlers []ObservedStateHandler
}

func NewAgentControlManager() *AgentControlManager {
	return &AgentControlManager{
		connections:     make(map[uint32]*AgentControlConnection),
		desiredRevision: make(map[uint32]uint64),
		desired:         make(map[uint32]map[string]*agentv1pb.DesiredOperation),
		observed:        make(map[uint32]*agentv1pb.ObservedState),
		pending:         make(map[agentOperationKey]chan *agentv1pb.OperationAck),
	}
}

func (m *AgentControlManager) AddObservedStateHandler(handler ObservedStateHandler) {
	if handler == nil {
		return
	}
	m.mu.Lock()
	m.observedHandlers = append(m.observedHandlers, handler)
	m.mu.Unlock()
}

func (m *AgentControlManager) register(connection *AgentControlConnection) {
	m.mu.Lock()
	if current := m.connections[connection.NodeID]; current != nil && current != connection {
		m.failPendingLocked(connection.NodeID)
	}
	connection.stateMu.Lock()
	connection.DesiredRev = m.desiredRevision[connection.NodeID]
	connection.stateMu.Unlock()
	m.connections[connection.NodeID] = connection
	m.mu.Unlock()
}

func (m *AgentControlManager) reconcileDesiredRevision(nodeID uint32, observedRevision uint64) uint64 {
	m.mu.Lock()
	if observedRevision > m.desiredRevision[nodeID] {
		m.desiredRevision[nodeID] = observedRevision
	}
	desiredRevision := m.desiredRevision[nodeID]
	m.mu.Unlock()
	return desiredRevision
}

func (m *AgentControlManager) unregister(connection *AgentControlConnection) {
	m.mu.Lock()
	if current := m.connections[connection.NodeID]; current == connection {
		delete(m.connections, connection.NodeID)
		m.failPendingLocked(connection.NodeID)
	}
	m.mu.Unlock()
}

func (m *AgentControlManager) failPendingLocked(nodeID uint32) {
	for key, waiter := range m.pending {
		if key.nodeID == nodeID {
			delete(m.pending, key)
			close(waiter)
		}
	}
}

func (m *AgentControlManager) isCurrent(connection *AgentControlConnection) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connections[connection.NodeID] == connection
}

func (m *AgentControlManager) DesiredRevision(nodeID uint32) uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.desiredRevision[nodeID]
}

func (m *AgentControlManager) Connection(nodeID uint32) (AgentControlSnapshot, bool) {
	m.mu.RLock()
	connection, ok := m.connections[nodeID]
	m.mu.RUnlock()
	if !ok {
		return AgentControlSnapshot{}, false
	}

	connection.stateMu.RLock()
	defer connection.stateMu.RUnlock()
	capabilities := make([]string, 0, len(connection.Capabilities))
	for _, capability := range connection.Capabilities {
		if capability != nil && capability.Name != "" {
			capabilities = append(capabilities, capability.Name)
		}
	}
	return AgentControlSnapshot{
		NodeID:       connection.NodeID,
		SessionID:    connection.SessionID,
		AgentVersion: connection.AgentVersion,
		InstanceID:   connection.InstanceID,
		Capabilities: capabilities,
		ConnectedAt:  connection.ConnectedAt,
		LastSeen:     connection.LastSeen,
		DesiredRev:   connection.DesiredRev,
		ObservedRev:  connection.ObservedRev,
	}, true
}

func (m *AgentControlManager) ObservedState(nodeID uint32) (*agentv1pb.ObservedState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	observed, ok := m.observed[nodeID]
	if !ok {
		return nil, false
	}
	return proto.Clone(observed).(*agentv1pb.ObservedState), true
}

// DispatchOperation pushes a desired operation and waits only for receipt ACK.
// Runtime completion is reported separately through ObservedState.
func (m *AgentControlManager) DispatchOperation(ctx context.Context, nodeID uint32, operation *agentv1pb.DesiredOperation) (*agentv1pb.OperationAck, error) {
	if operation == nil {
		return nil, fmt.Errorf("desired operation is nil")
	}
	if strings.TrimSpace(operation.OperationId) == "" {
		return nil, fmt.Errorf("operation_id is required")
	}
	if strings.TrimSpace(operation.Kind) == "" {
		return nil, fmt.Errorf("operation kind is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	cloned := proto.Clone(operation).(*agentv1pb.DesiredOperation)
	key := agentOperationKey{nodeID: nodeID, operationID: cloned.OperationId}
	waiter := make(chan *agentv1pb.OperationAck, 1)

	var connection *AgentControlConnection
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m.mu.RLock()
		connection = m.connections[nodeID]
		m.mu.RUnlock()
		if connection == nil {
			return nil, fmt.Errorf("agent node %d is not connected", nodeID)
		}

		// The replay path takes sendMu before publishing the connection. Taking
		// the same lock before mutating desired state keeps a newly dispatched
		// operation out of the replay snapshot and preserves wire ordering.
		connection.sendMu.Lock()
		if err := ctx.Err(); err != nil {
			connection.sendMu.Unlock()
			return nil, err
		}
		m.mu.Lock()
		if m.connections[nodeID] != connection {
			m.mu.Unlock()
			connection.sendMu.Unlock()
			continue
		}
		if !connectionSupportsOperation(connection, cloned.Kind) {
			m.mu.Unlock()
			connection.sendMu.Unlock()
			return nil, fmt.Errorf("agent node %d does not advertise capability %q", nodeID, cloned.Kind)
		}
		if _, exists := m.pending[key]; exists {
			m.mu.Unlock()
			connection.sendMu.Unlock()
			return nil, fmt.Errorf("operation %q is already pending", cloned.OperationId)
		}
		if existing := m.desired[nodeID][cloned.OperationId]; existing != nil {
			m.mu.Unlock()
			connection.sendMu.Unlock()
			return nil, fmt.Errorf("operation %q is already desired", cloned.OperationId)
		}
		if cloned.Revision == 0 {
			cloned.Revision = m.desiredRevision[nodeID] + 1
		} else if cloned.Revision <= m.desiredRevision[nodeID] {
			m.mu.Unlock()
			connection.sendMu.Unlock()
			return nil, fmt.Errorf("revision %d is not newer than %d", cloned.Revision, m.desiredRevision[nodeID])
		}
		m.desiredRevision[nodeID] = cloned.Revision
		if m.desired[nodeID] == nil {
			m.desired[nodeID] = make(map[string]*agentv1pb.DesiredOperation)
		}
		m.desired[nodeID][cloned.OperationId] = proto.Clone(cloned).(*agentv1pb.DesiredOperation)
		connection.stateMu.Lock()
		connection.DesiredRev = cloned.Revision
		connection.stateMu.Unlock()
		m.pending[key] = waiter
		m.mu.Unlock()
		break
	}

	requestID := newAgentControlID("request")
	message := &agentv1pb.ControlToAgent{
		RequestId:    requestID,
		NodeId:       nodeID,
		Revision:     cloned.Revision,
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.ControlToAgent_DesiredOperation{
			DesiredOperation: cloned,
		},
	}
	if err := connection.stream.Send(message); err != nil {
		connection.sendMu.Unlock()
		m.removePending(key, waiter)
		return nil, fmt.Errorf("send desired operation: %w", err)
	}
	connection.sendMu.Unlock()

	select {
	case ack, ok := <-waiter:
		if !ok || ack == nil {
			return nil, fmt.Errorf("agent connection closed before operation ACK")
		}
		return ack, nil
	case <-ctx.Done():
		m.removePending(key, waiter)
		return nil, ctx.Err()
	}
}

func (m *AgentControlManager) removePending(key agentOperationKey, waiter chan *agentv1pb.OperationAck) {
	m.mu.Lock()
	if current := m.pending[key]; current == waiter {
		delete(m.pending, key)
	}
	m.mu.Unlock()
}

func (m *AgentControlManager) resolveAck(connection *AgentControlConnection, ack *agentv1pb.OperationAck) error {
	if ack == nil || strings.TrimSpace(ack.OperationId) == "" {
		return status.Error(codes.InvalidArgument, "operation ACK operation_id is required")
	}
	if ack.SessionId != connection.SessionID {
		return status.Error(codes.FailedPrecondition, "operation ACK session_id does not match")
	}
	if ack.Revision == 0 {
		return status.Error(codes.InvalidArgument, "operation ACK revision is required")
	}

	key := agentOperationKey{nodeID: connection.NodeID, operationID: ack.OperationId}
	m.mu.Lock()
	if m.connections[connection.NodeID] != connection {
		m.mu.Unlock()
		return status.Error(codes.Aborted, "operation ACK belongs to a replaced agent session")
	}
	desired := m.desired[connection.NodeID][ack.OperationId]
	if desired == nil {
		m.mu.Unlock()
		return status.Error(codes.FailedPrecondition, "operation ACK does not match a desired operation")
	}
	if desired.Revision != ack.Revision {
		m.mu.Unlock()
		return status.Errorf(codes.FailedPrecondition, "operation ACK revision %d does not match desired revision %d", ack.Revision, desired.Revision)
	}
	waiter := m.pending[key]
	if waiter != nil {
		delete(m.pending, key)
	}
	if !ack.Accepted {
		m.removeDesiredLocked(connection.NodeID, ack.OperationId)
	}
	m.mu.Unlock()
	if waiter != nil {
		waiter <- proto.Clone(ack).(*agentv1pb.OperationAck)
	}
	return nil
}

func (m *AgentControlManager) recordObserved(connection *AgentControlConnection, observed *agentv1pb.ObservedState) error {
	if observed == nil || strings.TrimSpace(observed.OperationId) == "" {
		return status.Error(codes.InvalidArgument, "observed state operation_id is required")
	}
	if observed.SessionId != connection.SessionID {
		return status.Error(codes.FailedPrecondition, "observed state session_id does not match")
	}
	if observed.Revision == 0 {
		return status.Error(codes.InvalidArgument, "observed state revision is required")
	}

	m.mu.Lock()
	if m.connections[connection.NodeID] != connection {
		m.mu.Unlock()
		return status.Error(codes.Aborted, "observed state belongs to a replaced agent session")
	}
	desired := m.desired[connection.NodeID][observed.OperationId]
	if desired == nil {
		m.mu.Unlock()
		return status.Error(codes.FailedPrecondition, "observed state does not match a desired operation")
	}
	if desired.Revision != observed.Revision {
		m.mu.Unlock()
		return status.Errorf(codes.FailedPrecondition, "observed state revision %d does not match desired revision %d", observed.Revision, desired.Revision)
	}

	current := m.observed[connection.NodeID]
	accepted := current == nil || observed.Revision >= current.Revision
	if accepted {
		m.observed[connection.NodeID] = proto.Clone(observed).(*agentv1pb.ObservedState)
	}
	if isTerminalObservedPhase(observed.Phase) {
		m.removeDesiredLocked(connection.NodeID, observed.OperationId)
	}
	var handlers []ObservedStateHandler
	if accepted {
		handlers = append(handlers, m.observedHandlers...)
	}
	m.mu.Unlock()
	connection.touch(observed.Revision)
	for _, handler := range handlers {
		handler(connection.NodeID, proto.Clone(observed).(*agentv1pb.ObservedState))
	}
	return nil
}

func (m *AgentControlManager) removeDesiredLocked(nodeID uint32, operationID string) {
	operations := m.desired[nodeID]
	if operations == nil {
		return
	}
	delete(operations, operationID)
	if len(operations) == 0 {
		delete(m.desired, nodeID)
	}
}

func (m *AgentControlManager) registerAndReplay(connection *AgentControlConnection) error {
	connection.sendMu.Lock()
	defer connection.sendMu.Unlock()

	m.register(connection)
	if err := m.replayDesiredLocked(connection); err != nil {
		m.unregister(connection)
		return err
	}
	return nil
}

func (m *AgentControlManager) replayDesiredLocked(connection *AgentControlConnection) error {
	m.mu.RLock()
	operations := make([]*agentv1pb.DesiredOperation, 0, len(m.desired[connection.NodeID]))
	for _, operation := range m.desired[connection.NodeID] {
		operations = append(operations, proto.Clone(operation).(*agentv1pb.DesiredOperation))
	}
	m.mu.RUnlock()

	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Revision < operations[j].Revision
	})
	for _, operation := range operations {
		if err := connection.stream.Send(&agentv1pb.ControlToAgent{
			RequestId:    newAgentControlID("replay"),
			NodeId:       connection.NodeID,
			Revision:     operation.Revision,
			SentAtUnixMs: time.Now().UnixMilli(),
			Payload: &agentv1pb.ControlToAgent_DesiredOperation{
				DesiredOperation: operation,
			},
		}); err != nil {
			return fmt.Errorf("replay desired operation %q: %w", operation.OperationId, err)
		}
	}
	return nil
}

func isTerminalObservedPhase(phase agentv1pb.ObservedPhase) bool {
	switch phase {
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
		agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED,
		agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED:
		return true
	default:
		return false
	}
}

var agentControlManager = NewAgentControlManager()

func GetAgentControlManager() *AgentControlManager {
	return agentControlManager
}

// AgentControlGRPCServer implements the Agent-first control stream.
type AgentControlGRPCServer struct {
	agentv1pb.UnimplementedAgentControlServiceServer
	manager                  *AgentControlManager
	nodeService              *service.NodeService
	heartbeatIntervalSeconds uint32
}

func NewAgentControlGRPCServer(manager *AgentControlManager) *AgentControlGRPCServer {
	if manager == nil {
		manager = GetAgentControlManager()
	}
	return &AgentControlGRPCServer{
		manager:                  manager,
		nodeService:              service.NewNodeService(),
		heartbeatIntervalSeconds: defaultAgentHeartbeatIntervalSeconds,
	}
}

func (s *AgentControlGRPCServer) ControlStream(stream agentv1pb.AgentControlService_ControlStreamServer) error {
	nodeID, err := authenticatedStreamNodeID(stream.Context())
	if err != nil {
		return err
	}

	first, err := stream.Recv()
	if err != nil {
		return err
	}
	hello := first.GetHello()
	if hello == nil {
		return status.Error(codes.FailedPrecondition, "hello must be the first control message")
	}
	if first.NodeId != nodeID {
		return status.Error(codes.PermissionDenied, "hello node_id does not match authenticated node")
	}
	if strings.TrimSpace(first.RequestId) == "" {
		return status.Error(codes.InvalidArgument, "hello request_id is required")
	}
	if hello.Protocol != AgentProtocolVersion {
		return status.Errorf(codes.FailedPrecondition, "unsupported agent protocol %q", hello.Protocol)
	}
	if strings.TrimSpace(hello.InstanceId) == "" {
		return status.Error(codes.InvalidArgument, "hello instance_id is required")
	}
	if strings.TrimSpace(hello.AgentVersion) == "" {
		return status.Error(codes.InvalidArgument, "hello agent_version is required")
	}
	if len(hello.Capabilities) == 0 {
		return status.Error(codes.InvalidArgument, "hello capabilities are required")
	}
	for _, capability := range hello.Capabilities {
		if capability == nil || strings.TrimSpace(capability.Name) == "" {
			return status.Error(codes.InvalidArgument, "capability name is required")
		}
	}

	now := time.Now()
	desiredRevision := s.manager.reconcileDesiredRevision(nodeID, first.Revision)
	connection := &AgentControlConnection{
		NodeID:       nodeID,
		SessionID:    newAgentControlID("session"),
		AgentVersion: hello.AgentVersion,
		InstanceID:   hello.InstanceId,
		Capabilities: cloneCapabilities(hello.Capabilities),
		ConnectedAt:  now,
		LastSeen:     now,
		DesiredRev:   desiredRevision,
		ObservedRev:  first.Revision,
		stream:       stream,
	}

	if err := s.nodeService.UpdateLastCheckAt(uint(nodeID)); err != nil {
		slog.Warn("failed to persist agent hello heartbeat", "component", "agent-control", "node_id", nodeID, "error", err)
	}

	if err := connection.send(&agentv1pb.ControlToAgent{
		RequestId:    first.RequestId,
		NodeId:       nodeID,
		Revision:     desiredRevision,
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.ControlToAgent_HelloAck{
			HelloAck: &agentv1pb.HelloAck{
				SessionId:                connection.SessionID,
				ServerTimeUnixMs:         time.Now().UnixMilli(),
				HeartbeatIntervalSeconds: s.heartbeatIntervalSeconds,
				DesiredRevision:          desiredRevision,
			},
		},
	}); err != nil {
		return err
	}
	if err := s.manager.registerAndReplay(connection); err != nil {
		return err
	}
	defer s.manager.unregister(connection)

	for {
		message, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if !s.manager.isCurrent(connection) {
			return status.Error(codes.Aborted, "agent stream replaced by a newer session")
		}
		if message.NodeId != nodeID {
			return status.Error(codes.PermissionDenied, "message node_id does not match authenticated node")
		}
		if strings.TrimSpace(message.RequestId) == "" {
			return status.Error(codes.InvalidArgument, "control message request_id is required")
		}
		connection.touch(message.Revision)

		switch payload := message.Payload.(type) {
		case *agentv1pb.AgentToControl_Heartbeat:
			if payload.Heartbeat.SessionId != connection.SessionID {
				return status.Error(codes.FailedPrecondition, "heartbeat session_id does not match")
			}
			connection.touch(payload.Heartbeat.ObservedRevision)
			if err := s.nodeService.UpdateLastCheckAt(uint(nodeID)); err != nil {
				slog.Warn("failed to persist agent heartbeat", "component", "agent-control", "node_id", nodeID, "error", err)
			}
			if err := connection.send(&agentv1pb.ControlToAgent{
				RequestId:    message.RequestId,
				NodeId:       nodeID,
				Revision:     s.manager.DesiredRevision(nodeID),
				SentAtUnixMs: time.Now().UnixMilli(),
				Payload: &agentv1pb.ControlToAgent_HeartbeatAck{
					HeartbeatAck: &agentv1pb.HeartbeatAck{
						SessionId:        connection.SessionID,
						ServerTimeUnixMs: time.Now().UnixMilli(),
						DesiredRevision:  s.manager.DesiredRevision(nodeID),
					},
				},
			}); err != nil {
				return err
			}
		case *agentv1pb.AgentToControl_OperationAck:
			if payload.OperationAck == nil || payload.OperationAck.Revision != message.Revision {
				return status.Error(codes.InvalidArgument, "operation ACK envelope revision does not match payload")
			}
			if err := s.manager.resolveAck(connection, payload.OperationAck); err != nil {
				return err
			}
		case *agentv1pb.AgentToControl_ObservedState:
			if payload.ObservedState == nil || payload.ObservedState.Revision != message.Revision {
				return status.Error(codes.InvalidArgument, "observed state envelope revision does not match payload")
			}
			if err := s.manager.recordObserved(connection, payload.ObservedState); err != nil {
				return err
			}
		case *agentv1pb.AgentToControl_Hello:
			return status.Error(codes.InvalidArgument, "hello may only be sent once")
		default:
			return status.Error(codes.InvalidArgument, "control message payload is required")
		}
	}
}

func authenticatedStreamNodeID(ctx context.Context) (uint32, error) {
	authed, authErr := authenticateNode(ctx)
	if !authed {
		if authErr == "" {
			authErr = "node api key is required"
		}
		return 0, status.Error(codes.Unauthenticated, authErr)
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0, status.Error(codes.Unauthenticated, "missing node metadata")
	}
	values := md.Get("x-node-id")
	if len(values) == 0 {
		return 0, status.Error(codes.Unauthenticated, "missing x-node-id")
	}
	nodeID, err := strconv.ParseUint(values[0], 10, 32)
	if err != nil || nodeID == 0 {
		return 0, status.Error(codes.Unauthenticated, "invalid x-node-id")
	}
	keys := md.Get("x-api-key")
	if len(keys) == 0 || strings.TrimSpace(keys[0]) == "" {
		return 0, status.Error(codes.Unauthenticated, "node api key is required")
	}
	node, err := service.NewNodeService().GetNodeByAPIKey(keys[0])
	if err != nil {
		return 0, status.Error(codes.Unauthenticated, "authenticated node no longer exists")
	}
	if node.ID != uint(nodeID) {
		return 0, status.Error(codes.Unauthenticated, "node api key does not match x-node-id")
	}
	if node.Status == model.NodeStatusDisabled {
		return 0, status.Error(codes.PermissionDenied, "node is disabled")
	}
	return uint32(nodeID), nil
}

func cloneCapabilities(capabilities []*agentv1pb.Capability) []*agentv1pb.Capability {
	cloned := make([]*agentv1pb.Capability, 0, len(capabilities))
	for _, capability := range capabilities {
		if capability != nil {
			cloned = append(cloned, proto.Clone(capability).(*agentv1pb.Capability))
		}
	}
	return cloned
}

func connectionSupportsOperation(connection *AgentControlConnection, kind string) bool {
	for _, capability := range connection.Capabilities {
		if capability != nil && capability.Name == kind {
			return true
		}
	}
	return false
}

func newAgentControlID(prefix string) string {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(random)
}
