package grpc

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-agent/sdk/api/grpc/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type agentControlTestEnvironment struct {
	node    model.Node
	apiKey  string
	manager *AgentControlManager
	conn    *grpc.ClientConn
	server  *grpc.Server
	errCh   <-chan error
}

type blockingAgentControlStream struct {
	grpc.ServerStream
	mu          sync.Mutex
	sent        []*agentv1pb.ControlToAgent
	firstSend   chan struct{}
	releaseSend chan struct{}
	once        sync.Once
}

func (s *blockingAgentControlStream) Send(message *agentv1pb.ControlToAgent) error {
	if desired := message.GetDesiredOperation(); desired != nil && desired.OperationId == "replay-1" {
		s.once.Do(func() { close(s.firstSend) })
		<-s.releaseSend
	}
	s.mu.Lock()
	s.sent = append(s.sent, message)
	s.mu.Unlock()
	return nil
}

func (s *blockingAgentControlStream) Recv() (*agentv1pb.AgentToControl, error) {
	return nil, io.EOF
}

func newAgentControlTestEnvironment(t *testing.T) *agentControlTestEnvironment {
	t.Helper()
	cache.InitMemory()
	requireInMemoryDatabase(t)
	requireAutoMigrate(t, &model.Node{}, &model.NodeServiceAssignment{}, &model.PluginTelemetryState{}, &model.NodePluginObservedState{}, &model.Plugin{}, &model.PluginRelease{}, &model.PluginTrustRoot{})

	apiKey := "agent-control-test-key"
	hash := sha256.Sum256([]byte(apiKey))
	node := model.Node{
		Name:       "agent-control-test-node",
		Host:       "127.0.0.1",
		APIKeyHash: hex.EncodeToString(hash[:]),
		Status:     model.NodeStatusOnline,
	}
	require.NoError(t, database.GetDB().Create(&node).Error)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := service.PluginManifest{
		ID: "machine-telemetry", Name: "Machine Telemetry", Version: "1.1.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("a", 64), Capabilities: []string{"telemetry.read"},
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	_, err = service.RegisterPluginRelease(database.GetDB(), string(canonical), signature, publicKey)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	manager := NewAgentControlManager()
	server := grpc.NewServer(grpc.ChainStreamInterceptor(StreamAuthInterceptor("", "")))
	agentv1pb.RegisterAgentControlServiceServer(server, NewAgentControlGRPCServer(manager))
	errCh := serveGRPCServerForTest(t, server, listener)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	environment := &agentControlTestEnvironment{
		node:    node,
		apiKey:  apiKey,
		manager: manager,
		conn:    conn,
		server:  server,
		errCh:   errCh,
	}
	t.Cleanup(func() {
		requireClientConnClosed(t, conn)
		stopGRPCServerForTest(t, server, errCh)
		requireDatabaseClosed(t)
	})
	return environment
}

func (e *agentControlTestEnvironment) authContext(parent context.Context) context.Context {
	return metadata.AppendToOutgoingContext(
		parent,
		"x-node-id", strconv.FormatUint(uint64(e.node.ID), 10),
		"x-api-key", e.apiKey,
	)
}

func validAgentHello(nodeID uint32) *agentv1pb.AgentToControl {
	return &agentv1pb.AgentToControl{
		RequestId:    "hello-request",
		NodeId:       nodeID,
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Hello{
			Hello: &agentv1pb.Hello{
				Protocol:     AgentProtocolVersion,
				AgentVersion: "test-agent-v1",
				InstanceId:   "test-instance",
				Capabilities: []*agentv1pb.Capability{{Name: "agent.ping", Version: "v1"}},
			},
		},
	}
}

func requireNftablesForwardObservedStateAssignment(t *testing.T, environment *agentControlTestEnvironment) service.PluginManifest {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := service.PluginManifest{
		ID: "nftables-forward", Name: "nftables Forward", Version: "1.2.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("b", 64), Capabilities: []string{"kernel.observed-state"},
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	_, err = service.RegisterPluginRelease(database.GetDB(), string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&model.NodeServiceAssignment{
		NodeID: environment.node.ID, ServiceScope: "forward", PluginID: manifest.ID, Role: "cn_dedicated_nftables", DesiredVersion: manifest.Version, Enabled: true,
	}).Error)
	return manifest
}

func validNftablesForwardRuntimeObservation(manifest service.PluginManifest, observedAt time.Time) *agentv1pb.PluginObservedState {
	return &agentv1pb.PluginObservedState{
		PluginId: manifest.ID, Version: manifest.Version, DesiredRevision: 7, ObservedRevision: 7,
		ConfigHash: strings.Repeat("c", 64), Health: "healthy", RulesetSha256: strings.Repeat("d", 64), ObservedAtUnixMs: observedAt.UnixMilli(),
		RuleCounters: []*agentv1pb.PluginRuleCounter{{RuleId: "tcp-443", Packets: 4, Bytes: 512}},
	}
}

func TestAgentControlStreamDispatchAckObservedAndHeartbeat(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	defer cancel()

	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(validAgentHello(uint32(environment.node.ID))))
	helloAckMessage, err := stream.Recv()
	require.NoError(t, err)
	helloAck := helloAckMessage.GetHelloAck()
	require.NotNil(t, helloAck)
	require.NotEmpty(t, helloAck.SessionId)

	snapshot, connected := environment.manager.Connection(uint32(environment.node.ID))
	require.True(t, connected)
	assert.Equal(t, "test-agent-v1", snapshot.AgentVersion)
	assert.Contains(t, snapshot.Capabilities, "agent.ping")

	observedHook := make(chan *agentv1pb.ObservedState, 1)
	environment.manager.AddObservedStateHandler(func(nodeID uint32, observed *agentv1pb.ObservedState) {
		if nodeID == uint32(environment.node.ID) {
			observedHook <- observed
		}
	})

	dispatchResult := make(chan *agentv1pb.OperationAck, 1)
	dispatchError := make(chan error, 1)
	go func() {
		ack, dispatchErr := environment.manager.DispatchOperation(ctx, uint32(environment.node.ID), &agentv1pb.DesiredOperation{
			OperationId: "operation-1",
			Kind:        "agent.ping",
		})
		if dispatchErr != nil {
			dispatchError <- dispatchErr
			return
		}
		dispatchResult <- ack
	}()

	desiredMessage, err := stream.Recv()
	require.NoError(t, err)
	desired := desiredMessage.GetDesiredOperation()
	require.NotNil(t, desired)
	assert.Equal(t, "operation-1", desired.OperationId)
	assert.Equal(t, uint64(1), desired.Revision)

	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId:    "operation-ack-request",
		NodeId:       uint32(environment.node.ID),
		Revision:     desired.Revision,
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_OperationAck{
			OperationAck: &agentv1pb.OperationAck{
				OperationId:      desired.OperationId,
				Accepted:         true,
				AcceptedAtUnixMs: time.Now().UnixMilli(),
				SessionId:        helloAck.SessionId,
				Revision:         desired.Revision,
			},
		},
	}))

	select {
	case dispatchErr := <-dispatchError:
		require.NoError(t, dispatchErr)
	case ack := <-dispatchResult:
		require.True(t, ack.Accepted)
	case <-ctx.Done():
		t.Fatal("timed out waiting for operation ACK")
	}

	observed := &agentv1pb.ObservedState{
		OperationId:      desired.OperationId,
		Revision:         desired.Revision,
		Phase:            agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
		StateJson:        []byte(`{"ok":true}`),
		ObservedAtUnixMs: time.Now().UnixMilli(),
		SessionId:        helloAck.SessionId,
	}
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId:    "observed-request",
		NodeId:       uint32(environment.node.ID),
		Revision:     desired.Revision,
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload:      &agentv1pb.AgentToControl_ObservedState{ObservedState: observed},
	}))

	select {
	case hooked := <-observedHook:
		assert.Equal(t, desired.OperationId, hooked.OperationId)
	case <-ctx.Done():
		t.Fatal("timed out waiting for observed state hook")
	}
	storedObserved, ok := environment.manager.ObservedState(uint32(environment.node.ID))
	require.True(t, ok)
	assert.Equal(t, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, storedObserved.Phase)

	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId:    "heartbeat-request",
		NodeId:       uint32(environment.node.ID),
		Revision:     desired.Revision,
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{
			Heartbeat: &agentv1pb.Heartbeat{
				SessionId:        helloAck.SessionId,
				ObservedRevision: desired.Revision,
			},
		},
	}))
	heartbeatAckMessage, err := stream.Recv()
	require.NoError(t, err)
	assert.Equal(t, "heartbeat-request", heartbeatAckMessage.RequestId)
	assert.Equal(t, desired.Revision, heartbeatAckMessage.GetHeartbeatAck().DesiredRevision)

	var refreshed model.Node
	require.NoError(t, database.GetDB().First(&refreshed, environment.node.ID).Error)
	require.NotNil(t, refreshed.LastCheckAt)
}

func TestAgentControlHeartbeatPersistsAssignedPluginTelemetry(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	require.NoError(t, database.GetDB().Create(&model.NodeServiceAssignment{
		NodeID: environment.node.ID, ServiceScope: "monitoring", PluginID: "machine-telemetry", Role: "collector", DesiredVersion: "1.1.0", Enabled: true,
	}).Error)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	defer cancel()
	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(validAgentHello(uint32(environment.node.ID))))
	helloAckMessage, err := stream.Recv()
	require.NoError(t, err)
	helloAck := helloAckMessage.GetHelloAck()
	require.NotNil(t, helloAck)

	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "telemetry-heartbeat", NodeId: uint32(environment.node.ID), SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{
			SessionId: helloAck.SessionId,
			Metrics: map[string]float64{
				"go_goroutines": 4,
				"plugin.machine-telemetry.cpu_usage_percent":          18.5,
				"plugin.machine-telemetry.memory_usage_percent":       35,
				"plugin.machine-telemetry.disk_usage_percent":         42,
				"plugin.machine-telemetry.uptime_seconds":             600,
				"plugin.machine-telemetry.sample_age_seconds":         1,
				"plugin.unassigned.telemetry_should_not_be_persisted": 99,
			},
		}},
	}))
	ack, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, ack.GetHeartbeatAck())

	var state model.PluginTelemetryState
	require.NoError(t, database.GetDB().First(&state, "node_id = ? AND plugin_id = ?", environment.node.ID, "machine-telemetry").Error)
	require.Contains(t, state.MetricsJSON, `"cpu_usage_percent":18.5`)
	var count int64
	require.NoError(t, database.GetDB().Model(&model.PluginTelemetryState{}).Where("plugin_id = ?", "unassigned").Count(&count).Error)
	require.Zero(t, count)
	var node model.Node
	require.NoError(t, database.GetDB().First(&node, environment.node.ID).Error)
	require.Equal(t, 18.5, node.CPUUsage)
	require.Equal(t, int64(600), node.Uptime)
}

func TestAgentControlHeartbeatPersistsSignedPluginRuntimeObservation(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	manifest := requireNftablesForwardObservedStateAssignment(t, environment)

	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	defer cancel()
	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(validAgentHello(uint32(environment.node.ID))))
	helloAckMessage, err := stream.Recv()
	require.NoError(t, err)
	helloAck := helloAckMessage.GetHelloAck()
	require.NotNil(t, helloAck)
	now := time.Now()
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "runtime-observation-heartbeat", NodeId: uint32(environment.node.ID), SentAtUnixMs: now.UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{
			SessionId:          helloAck.SessionId,
			PluginObservations: []*agentv1pb.PluginObservedState{validNftablesForwardRuntimeObservation(manifest, now)},
		}},
	}))
	ack, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, ack.GetHeartbeatAck())

	var state model.NodePluginObservedState
	require.NoError(t, database.GetDB().First(&state, "node_id = ? AND plugin_id = ?", environment.node.ID, manifest.ID).Error)
	require.Equal(t, manifest.Version, state.Version)
	require.Equal(t, strings.Repeat("c", 64), state.ConfigHash)
	require.Equal(t, "healthy", state.Health)
	require.JSONEq(t, `[{"rule_id":"tcp-443","packets":4,"bytes":512}]`, state.CountersJSON)
}

func TestAgentControlHeartbeatPersistsValidRuntimeObservationAlongsideMalformedEntries(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	manifest := requireNftablesForwardObservedStateAssignment(t, environment)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	defer cancel()
	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(validAgentHello(uint32(environment.node.ID))))
	helloAckMessage, err := stream.Recv()
	require.NoError(t, err)
	helloAck := helloAckMessage.GetHelloAck()
	require.NotNil(t, helloAck)

	now := time.Now()
	badConfig := validNftablesForwardRuntimeObservation(manifest, now)
	badConfig.ConfigHash = "not-a-sha256"
	badVersion := validNftablesForwardRuntimeObservation(manifest, now)
	badVersion.Version = "not-installed"
	badCounter := validNftablesForwardRuntimeObservation(manifest, now)
	badCounter.PluginId = "malformed-counter"
	badCounter.RuleCounters = []*agentv1pb.PluginRuleCounter{nil}
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "mixed-runtime-observation-heartbeat", NodeId: uint32(environment.node.ID), SentAtUnixMs: now.UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{
			SessionId: helloAck.SessionId,
			PluginObservations: []*agentv1pb.PluginObservedState{
				nil,
				badConfig,
				badVersion,
				badCounter,
				validNftablesForwardRuntimeObservation(manifest, now),
			},
		}},
	}))
	ack, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, ack.GetHeartbeatAck())

	var state model.NodePluginObservedState
	require.NoError(t, database.GetDB().First(&state, "node_id = ? AND plugin_id = ?", environment.node.ID, manifest.ID).Error)
	require.Equal(t, strings.Repeat("c", 64), state.ConfigHash)
	require.Equal(t, "healthy", state.Health)
}

func TestAgentControlHeartbeatPersistsValidRuntimeObservationAlongsideCounterOverflow(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	manifest := requireNftablesForwardObservedStateAssignment(t, environment)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	defer cancel()
	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(validAgentHello(uint32(environment.node.ID))))
	helloAckMessage, err := stream.Recv()
	require.NoError(t, err)
	helloAck := helloAckMessage.GetHelloAck()
	require.NotNil(t, helloAck)

	now := time.Now()
	overflow := validNftablesForwardRuntimeObservation(manifest, now)
	overflow.PluginId = "overflowed-counter"
	overflow.RuleCounters = make([]*agentv1pb.PluginRuleCounter, maxAgentHeartbeatRuleCounters+1)
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "overflow-runtime-observation-heartbeat", NodeId: uint32(environment.node.ID), SentAtUnixMs: now.UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{
			SessionId: helloAck.SessionId,
			PluginObservations: []*agentv1pb.PluginObservedState{
				overflow,
				validNftablesForwardRuntimeObservation(manifest, now),
			},
		}},
	}))
	ack, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, ack.GetHeartbeatAck())

	var state model.NodePluginObservedState
	require.NoError(t, database.GetDB().First(&state, "node_id = ? AND plugin_id = ?", environment.node.ID, manifest.ID).Error)
	require.Equal(t, "healthy", state.Health)
}

func TestAgentControlHeartbeatRejectsOversizedRuntimeObservationEnvelope(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	manifest := requireNftablesForwardObservedStateAssignment(t, environment)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	defer cancel()
	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(validAgentHello(uint32(environment.node.ID))))
	helloAckMessage, err := stream.Recv()
	require.NoError(t, err)
	helloAck := helloAckMessage.GetHelloAck()
	require.NotNil(t, helloAck)

	now := time.Now()
	observations := make([]*agentv1pb.PluginObservedState, maxAgentHeartbeatPluginObservations+1)
	for index := range observations {
		observations[index] = validNftablesForwardRuntimeObservation(manifest, now)
	}
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "oversized-runtime-observation-heartbeat", NodeId: uint32(environment.node.ID), SentAtUnixMs: now.UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{
			SessionId: helloAck.SessionId, PluginObservations: observations,
		}},
	}))
	ack, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, ack.GetHeartbeatAck())

	var count int64
	require.NoError(t, database.GetDB().Model(&model.NodePluginObservedState{}).
		Where("node_id = ? AND plugin_id = ?", environment.node.ID, manifest.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestAgentControlStreamReplaysUnobservedOperationAfterReconnect(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	firstCtx, cancelFirst := context.WithCancel(environment.authContext(context.Background()))
	firstStream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(firstCtx)
	require.NoError(t, err)
	require.NoError(t, firstStream.Send(validAgentHello(uint32(environment.node.ID))))
	_, err = firstStream.Recv()
	require.NoError(t, err)

	dispatchResult := make(chan error, 1)
	go func() {
		_, dispatchErr := environment.manager.DispatchOperation(context.Background(), uint32(environment.node.ID), &agentv1pb.DesiredOperation{
			OperationId: "operation-replay",
			Kind:        "agent.ping",
		})
		dispatchResult <- dispatchErr
	}()

	firstDesiredMessage, err := firstStream.Recv()
	require.NoError(t, err)
	firstDesired := firstDesiredMessage.GetDesiredOperation()
	require.NotNil(t, firstDesired)
	cancelFirst()

	select {
	case dispatchErr := <-dispatchResult:
		require.Error(t, dispatchErr)
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch did not unblock after the first stream closed")
	}

	secondCtx, cancelSecond := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	defer cancelSecond()
	secondStream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(secondCtx)
	require.NoError(t, err)
	secondHello := validAgentHello(uint32(environment.node.ID))
	secondHello.RequestId = "hello-reconnect"
	require.NoError(t, secondStream.Send(secondHello))
	secondHelloAckMessage, err := secondStream.Recv()
	require.NoError(t, err)
	secondHelloAck := secondHelloAckMessage.GetHelloAck()
	require.NotNil(t, secondHelloAck)

	replayedMessage, err := secondStream.Recv()
	require.NoError(t, err)
	replayed := replayedMessage.GetDesiredOperation()
	require.NotNil(t, replayed)
	assert.Equal(t, firstDesired.OperationId, replayed.OperationId)
	assert.Equal(t, firstDesired.Revision, replayed.Revision)

	require.NoError(t, secondStream.Send(&agentv1pb.AgentToControl{
		RequestId:    "replay-observed",
		NodeId:       uint32(environment.node.ID),
		Revision:     replayed.Revision,
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_ObservedState{ObservedState: &agentv1pb.ObservedState{
			OperationId:      replayed.OperationId,
			Revision:         replayed.Revision,
			Phase:            agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
			ObservedAtUnixMs: time.Now().UnixMilli(),
			SessionId:        secondHelloAck.SessionId,
		}},
	}))
	require.Eventually(t, func() bool {
		environment.manager.mu.RLock()
		defer environment.manager.mu.RUnlock()
		return len(environment.manager.desired[uint32(environment.node.ID)]) == 0
	}, time.Second, 10*time.Millisecond)
}

func TestAgentControlCancellationCanBeRehydratedAfterControlRestart(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	defer cancel()
	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	hello := validAgentHello(uint32(environment.node.ID))
	hello.GetHello().Capabilities = append(hello.GetHello().Capabilities, &agentv1pb.Capability{Name: "operation.cancel", Version: "v1"})
	require.NoError(t, stream.Send(hello))
	helloAckMessage, err := stream.Recv()
	require.NoError(t, err)
	helloAck := helloAckMessage.GetHelloAck()
	require.NotNil(t, helloAck)

	require.NoError(t, environment.manager.CancelOperation(ctx, uint32(environment.node.ID), "operation-from-old-control", 7))
	cancelMessage, err := stream.Recv()
	require.NoError(t, err)
	cancelOperation := cancelMessage.GetDesiredOperation()
	require.NotNil(t, cancelOperation)
	require.Equal(t, "operation-from-old-control", cancelOperation.OperationId)
	require.Equal(t, "operation.cancel", cancelOperation.Kind)
	require.Equal(t, uint64(7), cancelOperation.Revision)

	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "cancel-ack", NodeId: uint32(environment.node.ID), Revision: 7, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_OperationAck{OperationAck: &agentv1pb.OperationAck{
			OperationId: cancelOperation.OperationId, Accepted: true, SessionId: helloAck.SessionId, Revision: 7,
		}},
	}))
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "cancel-observed", NodeId: uint32(environment.node.ID), Revision: 7, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_ObservedState{ObservedState: &agentv1pb.ObservedState{
			OperationId: cancelOperation.OperationId, Revision: 7,
			Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED, Message: "operation cancelled", SessionId: helloAck.SessionId,
		}},
	}))
	require.Eventually(t, func() bool {
		environment.manager.mu.RLock()
		defer environment.manager.mu.RUnlock()
		return environment.manager.desired[uint32(environment.node.ID)][cancelOperation.OperationId] == nil
	}, time.Second, 10*time.Millisecond)
}

func TestAgentControlCancellationRequiresAdvertisedCapability(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	defer cancel()
	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(validAgentHello(uint32(environment.node.ID))))
	_, err = stream.Recv()
	require.NoError(t, err)

	err = environment.manager.CancelOperation(ctx, uint32(environment.node.ID), "unsupported-cancel", 1)
	require.ErrorContains(t, err, "does not advertise capability")
}

func TestAgentControlHelloReconcilesObservedRevisionBeforeDispatch(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	defer cancel()

	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	hello := validAgentHello(uint32(environment.node.ID))
	hello.Revision = 41
	require.NoError(t, stream.Send(hello))
	helloAckMessage, err := stream.Recv()
	require.NoError(t, err)
	helloAck := helloAckMessage.GetHelloAck()
	require.NotNil(t, helloAck)
	assert.Equal(t, uint64(41), helloAck.DesiredRevision)
	assert.Equal(t, uint64(41), environment.manager.DesiredRevision(uint32(environment.node.ID)))

	dispatchResult := make(chan error, 1)
	go func() {
		_, dispatchErr := environment.manager.DispatchOperation(ctx, uint32(environment.node.ID), &agentv1pb.DesiredOperation{
			OperationId: "operation-after-reconcile",
			Kind:        "agent.ping",
		})
		dispatchResult <- dispatchErr
	}()

	desiredMessage, err := stream.Recv()
	require.NoError(t, err)
	desired := desiredMessage.GetDesiredOperation()
	require.NotNil(t, desired)
	assert.Equal(t, uint64(42), desired.Revision)
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId:    "reconciled-operation-ack",
		NodeId:       uint32(environment.node.ID),
		Revision:     desired.Revision,
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_OperationAck{OperationAck: &agentv1pb.OperationAck{
			OperationId:      desired.OperationId,
			Accepted:         true,
			AcceptedAtUnixMs: time.Now().UnixMilli(),
			SessionId:        helloAck.SessionId,
			Revision:         desired.Revision,
		}},
	}))
	select {
	case dispatchErr := <-dispatchResult:
		require.NoError(t, dispatchErr)
	case <-ctx.Done():
		t.Fatal("timed out waiting for reconciled operation ACK")
	}
}

func TestAgentControlDispatchRejectsUnadvertisedCapability(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 5*time.Second)
	defer cancel()

	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(validAgentHello(uint32(environment.node.ID))))
	_, err = stream.Recv()
	require.NoError(t, err)

	_, err = environment.manager.DispatchOperation(ctx, uint32(environment.node.ID), &agentv1pb.DesiredOperation{
		OperationId: "unsupported-operation",
		Kind:        "node.reload",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `does not advertise capability "node.reload"`)
	assert.Zero(t, environment.manager.DesiredRevision(uint32(environment.node.ID)))
	environment.manager.mu.RLock()
	assert.Empty(t, environment.manager.desired[uint32(environment.node.ID)])
	environment.manager.mu.RUnlock()
}

func TestAgentControlRegisterAndReplaySerializesConcurrentDispatch(t *testing.T) {
	manager := NewAgentControlManager()
	nodeID := uint32(7)
	manager.desiredRevision[nodeID] = 1
	manager.desired[nodeID] = map[string]*agentv1pb.DesiredOperation{
		"replay-1": {OperationId: "replay-1", Kind: "agent.ping", Revision: 1},
	}
	stream := &blockingAgentControlStream{
		firstSend:   make(chan struct{}),
		releaseSend: make(chan struct{}),
	}
	connection := &AgentControlConnection{
		NodeID:       nodeID,
		SessionID:    "current-session",
		Capabilities: []*agentv1pb.Capability{{Name: "agent.ping"}},
		stream:       stream,
	}

	replayDone := make(chan error, 1)
	go func() { replayDone <- manager.registerAndReplay(connection) }()
	select {
	case <-stream.firstSend:
	case <-time.After(time.Second):
		t.Fatal("replay did not start")
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := manager.DispatchOperation(context.Background(), nodeID, &agentv1pb.DesiredOperation{
			OperationId: "live-2",
			Kind:        "agent.ping",
		})
		dispatchDone <- err
	}()
	select {
	case err := <-dispatchDone:
		t.Fatalf("dispatch bypassed replay batch lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	manager.mu.RLock()
	assert.Nil(t, manager.desired[nodeID]["live-2"])
	assert.Equal(t, uint64(1), manager.desiredRevision[nodeID])
	manager.mu.RUnlock()

	close(stream.releaseSend)
	require.NoError(t, <-replayDone)
	require.Eventually(t, func() bool {
		stream.mu.Lock()
		defer stream.mu.Unlock()
		return len(stream.sent) == 2
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, manager.resolveAck(connection, &agentv1pb.OperationAck{
		OperationId: "live-2",
		Accepted:    true,
		SessionId:   connection.SessionID,
		Revision:    2,
	}))
	require.NoError(t, <-dispatchDone)
	stream.mu.Lock()
	assert.Equal(t, "replay-1", stream.sent[0].GetDesiredOperation().OperationId)
	assert.Equal(t, "live-2", stream.sent[1].GetDesiredOperation().OperationId)
	stream.mu.Unlock()
}

func TestAgentControlReplayRebindsPluginEnvelopeToNewSession(t *testing.T) {
	manager := NewAgentControlManager()
	nodeID := uint32(8)
	config := json.RawMessage(`{"interval_seconds":30}`)
	digest := sha256.Sum256(config)
	payload, err := json.Marshal(kernelOperationEnvelopePayload{
		Version: service.KernelOperationEnvelopeVersion, OperationID: "plugin-replay", IdempotencyKey: "plugin-replay-key",
		SessionID: "replaced-session", Revision: 4, PluginID: "machine-telemetry", TargetVersion: "1.0.0",
		ConfigHash: hex.EncodeToString(digest[:]), Config: config,
	})
	require.NoError(t, err)
	manager.desiredRevision[nodeID] = 4
	manager.desired[nodeID] = map[string]*agentv1pb.DesiredOperation{
		"plugin-replay": {OperationId: "plugin-replay", Kind: "plugin.enable", Revision: 4, PayloadJson: payload},
	}
	stream := &blockingAgentControlStream{firstSend: make(chan struct{}), releaseSend: make(chan struct{})}
	connection := &AgentControlConnection{
		NodeID: nodeID, SessionID: "current-session", Capabilities: []*agentv1pb.Capability{{Name: "plugin.enable"}}, stream: stream,
	}

	require.NoError(t, manager.registerAndReplay(connection))
	stream.mu.Lock()
	require.Len(t, stream.sent, 1)
	replayed := stream.sent[0].GetDesiredOperation()
	stream.mu.Unlock()
	require.NotNil(t, replayed)
	var envelope kernelOperationEnvelopePayload
	require.NoError(t, json.Unmarshal(replayed.PayloadJson, &envelope))
	require.Equal(t, connection.SessionID, envelope.SessionID)
	require.Equal(t, "plugin-replay", envelope.OperationID)

	manager.mu.RLock()
	stored := manager.desired[nodeID]["plugin-replay"]
	manager.mu.RUnlock()
	require.NotNil(t, stored)
	require.NoError(t, json.Unmarshal(stored.PayloadJson, &envelope))
	require.Equal(t, connection.SessionID, envelope.SessionID)
}

func TestAgentControlManagerRejectsReplacedSessionReports(t *testing.T) {
	manager := NewAgentControlManager()
	nodeID := uint32(9)
	oldConnection := &AgentControlConnection{NodeID: nodeID, SessionID: "old-session"}
	currentConnection := &AgentControlConnection{NodeID: nodeID, SessionID: "current-session"}
	manager.register(oldConnection)
	manager.register(currentConnection)
	operation := &agentv1pb.DesiredOperation{OperationId: "operation-9", Kind: "agent.ping", Revision: 9}
	waiter := make(chan *agentv1pb.OperationAck, 1)
	key := agentOperationKey{nodeID: nodeID, operationID: operation.OperationId}
	manager.mu.Lock()
	manager.desiredRevision[nodeID] = operation.Revision
	manager.desired[nodeID] = map[string]*agentv1pb.DesiredOperation{operation.OperationId: operation}
	manager.pending[key] = waiter
	manager.mu.Unlock()

	err := manager.resolveAck(oldConnection, &agentv1pb.OperationAck{
		OperationId: operation.OperationId,
		Accepted:    false,
		SessionId:   oldConnection.SessionID,
		Revision:    operation.Revision,
	})
	assert.Equal(t, codes.Aborted, status.Code(err))
	err = manager.recordObserved(oldConnection, &agentv1pb.ObservedState{
		OperationId: operation.OperationId,
		Revision:    operation.Revision,
		Phase:       agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
		SessionId:   oldConnection.SessionID,
	})
	assert.Equal(t, codes.Aborted, status.Code(err))

	err = manager.resolveAck(currentConnection, &agentv1pb.OperationAck{
		OperationId: operation.OperationId,
		Accepted:    false,
		SessionId:   oldConnection.SessionID,
		Revision:    operation.Revision,
	})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	err = manager.resolveAck(currentConnection, &agentv1pb.OperationAck{
		OperationId: operation.OperationId,
		Accepted:    false,
		SessionId:   currentConnection.SessionID,
		Revision:    operation.Revision + 1,
	})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	err = manager.recordObserved(currentConnection, &agentv1pb.ObservedState{
		OperationId: operation.OperationId,
		Revision:    operation.Revision,
		Phase:       agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
		SessionId:   oldConnection.SessionID,
	})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	err = manager.recordObserved(currentConnection, &agentv1pb.ObservedState{
		OperationId: operation.OperationId,
		Revision:    operation.Revision + 1,
		Phase:       agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
		SessionId:   currentConnection.SessionID,
	})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))

	manager.mu.RLock()
	assert.Equal(t, waiter, manager.pending[key])
	assert.Same(t, operation, manager.desired[nodeID][operation.OperationId])
	_, observed := manager.observed[nodeID]
	manager.mu.RUnlock()
	assert.False(t, observed)
}

func TestAgentControlStreamRejectsDisabledNode(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	require.NoError(t, database.GetDB().Model(&model.Node{}).
		Where("id = ?", environment.node.ID).
		Update("status", model.NodeStatusDisabled).Error)

	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 2*time.Second)
	defer cancel()
	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	// The server rejects disabled nodes before reading the first application
	// message. A client Send may therefore race with the final status and return
	// io.EOF; Recv is the authoritative way to observe the gRPC status.
	_, err = stream.Recv()
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestAgentControlStreamRejectsMissingAndWrongNodeCredentials(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{name: "missing metadata", ctx: context.Background()},
		{name: "global token fallback is rejected", ctx: metadata.AppendToOutgoingContext(context.Background(),
			"x-node-id", strconv.FormatUint(uint64(environment.node.ID), 10),
			"authorization", "Bearer global-token",
		)},
		{name: "wrong api key", ctx: metadata.AppendToOutgoingContext(context.Background(),
			"x-node-id", strconv.FormatUint(uint64(environment.node.ID), 10),
			"x-api-key", "wrong-key",
		)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(test.ctx, 2*time.Second)
			defer cancel()
			stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
			require.NoError(t, err)
			_, err = stream.Recv()
			require.Error(t, err)
			assert.Equal(t, codes.Unauthenticated, status.Code(err))
		})
	}
}

func TestAgentControlStreamValidatesHelloIdentityAndFields(t *testing.T) {
	environment := newAgentControlTestEnvironment(t)
	tests := []struct {
		name string
		edit func(*agentv1pb.AgentToControl)
		code codes.Code
	}{
		{
			name: "node mismatch",
			edit: func(message *agentv1pb.AgentToControl) { message.NodeId++ },
			code: codes.PermissionDenied,
		},
		{
			name: "missing request id",
			edit: func(message *agentv1pb.AgentToControl) { message.RequestId = "" },
			code: codes.InvalidArgument,
		},
		{
			name: "missing capabilities",
			edit: func(message *agentv1pb.AgentToControl) { message.GetHello().Capabilities = nil },
			code: codes.InvalidArgument,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 2*time.Second)
			defer cancel()
			stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
			require.NoError(t, err)
			hello := validAgentHello(uint32(environment.node.ID))
			test.edit(hello)
			require.NoError(t, stream.Send(hello))
			_, err = stream.Recv()
			require.Error(t, err)
			assert.Equal(t, test.code, status.Code(err))
		})
	}
}
