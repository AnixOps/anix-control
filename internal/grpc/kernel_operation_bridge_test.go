package grpc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/v3/api/grpc/agent/v1"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type kernelOperationStreamStub struct {
	snapshot      AgentControlSnapshot
	connected     bool
	operations    []*agentv1pb.DesiredOperation
	cancellations []string
	handlers      []ObservedStateHandler
	err           error
	cancelErr     error
	reject        bool
	dispatched    chan struct{}
	dispatchOnce  sync.Once
}

func (s *kernelOperationStreamStub) CancelOperation(_ context.Context, _ uint32, operationID string, _ uint64) error {
	if s.cancelErr != nil {
		return s.cancelErr
	}
	s.cancellations = append(s.cancellations, operationID)
	return nil
}

func (s *kernelOperationStreamStub) Connection(uint32) (AgentControlSnapshot, bool) {
	return s.snapshot, s.connected
}

func (s *kernelOperationStreamStub) DispatchOperation(_ context.Context, _ uint32, operation *agentv1pb.DesiredOperation) (*agentv1pb.OperationAck, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.operations = append(s.operations, operation)
	if s.dispatched != nil {
		s.dispatchOnce.Do(func() { close(s.dispatched) })
	}
	if s.reject {
		return &agentv1pb.OperationAck{OperationId: operation.OperationId, Accepted: false, Error: "policy rejected operation"}, nil
	}
	return &agentv1pb.OperationAck{OperationId: operation.OperationId, Accepted: true, SessionId: s.snapshot.SessionID, Revision: operation.Revision}, nil
}

func (s *kernelOperationStreamStub) AddObservedStateHandler(handler ObservedStateHandler) {
	s.handlers = append(s.handlers, handler)
}

func (s *kernelOperationStreamStub) observe(nodeID uint32, observed *agentv1pb.ObservedState) {
	for _, handler := range s.handlers {
		handler(nodeID, observed)
	}
}

func newKernelOperationBridgeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, service.EnsureKernelSchema(db))
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	return db
}

func readControlOperationEnvelopeGolden(t *testing.T) []byte {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "contracts", "agent", "v1", "operation-envelope-golden.json"))
	require.NoError(t, err)
	return bytes.TrimSpace(raw)
}

func seedAgentPluginRelease(t *testing.T, db *gorm.DB, pluginID string) {
	t.Helper()
	manifest := service.PluginManifest{
		ID: pluginID, Name: pluginID, Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginRelease{
		PluginID: pluginID, Version: manifest.Version, APIVersion: manifest.APIVersion,
		ManifestJSON: string(canonical), ArtifactSHA256: manifest.ArtifactSHA256, Signature: "test",
	}).Error)
}

func TestKernelOperationDesiredMatchesAgentEnvelopeGolden(t *testing.T) {
	nodeID := uint(42)
	deadline := time.UnixMilli(1800000000000)
	operation := model.KernelOperation{
		ID:              "6d1e2a5b-2f43-41a7-a4d3-19f3f93f8c3a",
		IdempotencyKey:  "node-42-config-7",
		EnvelopeVersion: service.KernelOperationEnvelopeVersion,
		SessionID:       "agent-session-golden",
		NodeID:          &nodeID,
		PluginID:        "wireguard",
		TargetVersion:   "1.0.0",
		Kind:            "plugin.configure",
		Revision:        7,
		ConfigJSON:      `{"endpoint":"edge.example","listen_port":51820,"private_key_ref":"secret/wg-42"}`,
		ConfigHash:      "58d0423286a2a03720f1f59fe7d8cc8acb0b7c997a0cda4c464b2a3b12044319",
		State:           "dispatching",
		DeadlineAt:      &deadline,
	}

	desired, err := kernelOperationDesired(operation)
	require.NoError(t, err)
	require.Equal(t, operation.ID, desired.OperationId)
	require.Equal(t, uint64(7), desired.Revision)
	require.Equal(t, deadline.UnixMilli(), desired.DeadlineUnixMs)
	require.Equal(t, string(readControlOperationEnvelopeGolden(t)), string(bytes.TrimSpace(desired.PayloadJson)))
}

func TestKernelOperationBridgeConversionBoundaries(t *testing.T) {
	nodeID, err := kernelAgentNodeID(42)
	require.NoError(t, err)
	require.Equal(t, uint32(42), nodeID)
	if ^uint(0) > uint(math.MaxUint32) {
		_, err = kernelAgentNodeID(uint(math.MaxUint32) + 1)
		require.Error(t, err)
	}

	revision, err := kernelAgentRevision(7)
	require.NoError(t, err)
	require.Equal(t, uint64(7), revision)
	_, err = kernelAgentRevision(0)
	require.Error(t, err)

	observed, ok := kernelObservedRevision(uint64(math.MaxInt64))
	require.True(t, ok)
	require.Equal(t, int64(math.MaxInt64), observed)
	_, ok = kernelObservedRevision(math.MaxUint64)
	require.False(t, ok)
}

func TestKernelOperationDesiredRejectsInvalidStoredRows(t *testing.T) {
	nodeID := uint(1)
	deadline := time.Now().Add(time.Minute)
	base := model.KernelOperation{
		ID: "invalid-stored-operation", IdempotencyKey: "invalid-stored-operation", EnvelopeVersion: service.KernelOperationEnvelopeVersion,
		NodeID: &nodeID, PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.configure", Revision: 1,
		ConfigJSON: `{}`, DeadlineAt: &deadline,
	}

	missingNode := base
	missingNode.NodeID = nil
	_, err := kernelOperationDesired(missingNode)
	require.Error(t, err)

	badEnvelope := base
	badEnvelope.EnvelopeVersion = "unsupported/v1"
	_, err = kernelOperationDesired(badEnvelope)
	require.Error(t, err)

	nonCanonical := base
	nonCanonical.ConfigJSON = ` {}`
	_, err = kernelOperationDesired(nonCanonical)
	require.Error(t, err)

	badRevision := base
	badRevision.Revision = -1
	_, err = kernelOperationDesired(badRevision)
	require.Error(t, err)
}

func TestKernelObservedStateMapsEveryPhase(t *testing.T) {
	tests := []struct {
		phase   agentv1pb.ObservedPhase
		message string
		state   string
		error   string
	}{
		{agentv1pb.ObservedPhase_OBSERVED_PHASE_ACCEPTED, "accepted", "running", "accepted"},
		{agentv1pb.ObservedPhase_OBSERVED_PHASE_APPLYING, "applying", "running", "applying"},
		{agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, "ignored", "succeeded", ""},
		{agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED, "newer revision", "superseded", "newer revision"},
		{agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED, "", "failed", "Agent reported operation failure"},
		{agentv1pb.ObservedPhase_OBSERVED_PHASE_UNSPECIFIED, "waiting", "running", "waiting"},
	}
	for _, test := range tests {
		state, message := kernelObservedState(&agentv1pb.ObservedState{Phase: test.phase, Message: test.message})
		require.Equal(t, test.state, state)
		require.Equal(t, test.error, message)
	}
}

func TestTerminalKernelOperationStateIncludesLegacyCompleted(t *testing.T) {
	for _, state := range []string{"succeeded", "completed", "failed", "superseded", "cancelled", "timed_out"} {
		require.True(t, isTerminalKernelOperationState(state), state)
	}
	require.False(t, isTerminalKernelOperationState("running"))
}

func TestKernelOperationBridgePersistsDispatchFailureAndAgentRejection(t *testing.T) {
	tests := []struct {
		name          string
		stream        *kernelOperationStreamStub
		expectedState string
		expectedError string
	}{
		{name: "transport failure", stream: &kernelOperationStreamStub{err: errors.New("transport unavailable")}, expectedState: "dispatching", expectedError: "transport unavailable"},
		{name: "agent rejection", stream: &kernelOperationStreamStub{reject: true}, expectedState: "failed", expectedError: "policy rejected operation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newKernelOperationBridgeDB(t)
			node := model.Node{Name: test.name, Host: "127.0.0.10"}
			require.NoError(t, db.Create(&node).Error)
			seedAgentPluginRelease(t, db, "wireguard")
			deadline := time.Now().Add(time.Minute)
			operation, _, err := service.CreateKernelOperation(db, model.KernelOperation{
				ID: uuid.NewString(), IdempotencyKey: "dispatch-key-" + strings.ReplaceAll(test.name, " ", "-"), NodeID: &node.ID,
				PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.health", ConfigJSON: `{}`, DeadlineAt: &deadline,
			})
			require.NoError(t, err)
			test.stream.connected = true
			test.stream.snapshot = AgentControlSnapshot{NodeID: uint32(node.ID), SessionID: "dispatch-session"}
			bridge, err := NewKernelOperationBridge(db, test.stream)
			require.NoError(t, err)
			count, err := bridge.RunOnce(context.Background())
			require.NoError(t, err)
			require.Zero(t, count)
			var stored model.KernelOperation
			require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
			require.Equal(t, test.expectedState, stored.State)
			require.Equal(t, test.expectedError, stored.LastError)
		})
	}
}

func TestKernelOperationBridgeFailsInvalidNodeIDBeforeDispatch(t *testing.T) {
	if ^uint(0) <= uint(math.MaxUint32) {
		t.Skip("uint node ids cannot exceed Agent uint32 range on this platform")
	}
	db := newKernelOperationBridgeDB(t)
	seedAgentPluginRelease(t, db, "wireguard")
	nodeID := uint(math.MaxUint32) + 1
	deadline := time.Now().Add(time.Minute)
	digest := sha256.Sum256([]byte("{}"))
	operation := model.KernelOperation{
		ID: uuid.NewString(), IdempotencyKey: "invalid-node-dispatch", EnvelopeVersion: service.KernelOperationEnvelopeVersion,
		NodeID: &nodeID, PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.health", Revision: 1,
		ConfigJSON: "{}", ConfigHash: hex.EncodeToString(digest[:]), State: "pending", DeadlineAt: &deadline,
	}
	require.NoError(t, db.Create(&operation).Error)
	require.NoError(t, db.Create(&model.NodeOperationRevision{
		NodeID: uint(math.MaxUint32) + 1, DesiredRevision: 1,
	}).Error)
	stream := &kernelOperationStreamStub{connected: true, snapshot: AgentControlSnapshot{NodeID: 1, SessionID: "invalid-node-session"}}
	bridge, err := NewKernelOperationBridge(db, stream)
	require.NoError(t, err)
	count, err := bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
	var stored model.KernelOperation
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.Equal(t, "failed", stored.State)
	require.Contains(t, stored.LastError, "exceeds Agent uint32 range")
	require.Empty(t, stream.operations)
}

func TestKernelOperationBridgeRecordsCancellationDeliveryFailure(t *testing.T) {
	db := newKernelOperationBridgeDB(t)
	node := model.Node{Name: "cancel-error-node", Host: "127.0.0.12", APIKey: "cancel-error-key"}
	require.NoError(t, db.Create(&node).Error)
	seedAgentPluginRelease(t, db, "wireguard")
	deadline := time.Now().Add(time.Minute)
	operation, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: uuid.NewString(), IdempotencyKey: "cancel-delivery-failure", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.health", ConfigJSON: `{}`, DeadlineAt: &deadline,
	})
	require.NoError(t, err)
	dispatchedAt := time.Now().Add(-time.Second)
	require.NoError(t, db.Model(operation).Updates(map[string]any{
		"state": "cancel_requested", "session_id": "cancel-error-session", "dispatched_at": dispatchedAt,
	}).Error)
	stream := &kernelOperationStreamStub{
		connected: true, snapshot: AgentControlSnapshot{NodeID: uint32(node.ID), SessionID: "cancel-error-session"},
		cancelErr: errors.New("agent stream unavailable"),
	}
	bridge, err := NewKernelOperationBridge(db, stream)
	require.NoError(t, err)
	count, err := bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
	var stored model.KernelOperation
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.Equal(t, "cancel_requested", stored.State)
	require.Nil(t, stored.CancelDispatchedAt)
	require.Equal(t, "cancellation delivery failed: agent stream unavailable", stored.LastError)
}

func TestKernelOperationBridgeIgnoresInvalidObservedState(t *testing.T) {
	db := newKernelOperationBridgeDB(t)
	node := model.Node{Name: "invalid-observed-node", Host: "127.0.0.13"}
	require.NoError(t, db.Create(&node).Error)
	seedAgentPluginRelease(t, db, "wireguard")
	deadline := time.Now().Add(time.Minute)
	operation, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: uuid.NewString(), IdempotencyKey: "invalid-observed", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.health", ConfigJSON: `{}`, DeadlineAt: &deadline,
	})
	require.NoError(t, err)
	stream := &kernelOperationStreamStub{}
	bridge, err := NewKernelOperationBridge(db, stream)
	require.NoError(t, err)
	bridge.recordObserved(uint32(node.ID), nil)
	bridge.recordObserved(uint32(node.ID), &agentv1pb.ObservedState{Revision: uint64(operation.Revision)})
	bridge.recordObserved(uint32(node.ID), &agentv1pb.ObservedState{OperationId: operation.ID, Revision: math.MaxUint64})
	var stored model.KernelOperation
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.Equal(t, "pending", stored.State)
	require.Nil(t, stored.ObservedAt)
}

func TestKernelOperationBridgeStartDispatchesUntilCancelled(t *testing.T) {
	db := newKernelOperationBridgeDB(t)
	node := model.Node{Name: "start-node", Host: "127.0.0.11"}
	require.NoError(t, db.Create(&node).Error)
	seedAgentPluginRelease(t, db, "wireguard")
	deadline := time.Now().Add(time.Minute)
	_, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: uuid.NewString(), IdempotencyKey: "start-operation", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.health", ConfigJSON: `{}`, DeadlineAt: &deadline,
	})
	require.NoError(t, err)
	dispatched := make(chan struct{})
	stream := &kernelOperationStreamStub{
		connected: true, snapshot: AgentControlSnapshot{NodeID: uint32(node.ID), SessionID: "start-session"}, dispatched: dispatched,
	}
	bridge, err := NewKernelOperationBridge(db, stream)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	bridge.Start(ctx, time.Hour, func(err error) { t.Errorf("unexpected bridge error: %v", err) })
	select {
	case <-dispatched:
	case <-time.After(5 * time.Second):
		t.Fatal("bridge did not dispatch the pending operation")
	}
	cancel()
}

func TestServerExposesAgentControlManager(t *testing.T) {
	server := NewServer(nil)
	require.Same(t, GetAgentControlManager(), server.GetAgentControlManager())
}

func TestKernelOperationBridgeDispatchesVersionedEnvelopeAndPersistsObservedState(t *testing.T) {
	db := newKernelOperationBridgeDB(t)
	node := model.Node{Name: "bridge-node", Host: "127.0.0.1"}
	require.NoError(t, db.Create(&node).Error)
	seedAgentPluginRelease(t, db, "wireguard")
	deadline := time.Now().Add(time.Minute)
	operation, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: "28bbf69e-c713-469d-bdfc-f06c24cb68e5", IdempotencyKey: "bridge-config-1", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.configure",
		ConfigJSON: ` { "endpoint": "example.test", "port": 51820 } `, DeadlineAt: &deadline,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), operation.Revision)

	stream := &kernelOperationStreamStub{connected: true, snapshot: AgentControlSnapshot{NodeID: uint32(node.ID), SessionID: "agent-session-1"}}
	bridge, err := NewKernelOperationBridge(db, stream)
	require.NoError(t, err)
	count, err := bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Len(t, stream.operations, 1)

	desired := stream.operations[0]
	require.Equal(t, operation.ID, desired.OperationId)
	require.Equal(t, uint64(1), desired.Revision)
	var envelope struct {
		Version    string          `json:"version"`
		SessionID  string          `json:"session_id"`
		Config     json.RawMessage `json:"config"`
		ConfigHash string          `json:"config_hash"`
	}
	require.NoError(t, json.Unmarshal(desired.PayloadJson, &envelope))
	require.Equal(t, service.KernelOperationEnvelopeVersion, envelope.Version)
	require.Equal(t, "agent-session-1", envelope.SessionID)
	require.JSONEq(t, `{"endpoint":"example.test","port":51820}`, string(envelope.Config))
	require.Equal(t, operation.ConfigHash, envelope.ConfigHash)

	var stored model.KernelOperation
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.Equal(t, "running", stored.State)
	require.Equal(t, "agent-session-1", stored.SessionID)
	require.NotNil(t, stored.DispatchedAt)
	require.NotNil(t, stored.AcknowledgedAt)

	stream.observe(uint32(node.ID), &agentv1pb.ObservedState{
		OperationId: operation.ID, Revision: desired.Revision,
		Phase:     agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
		StateJson: []byte(`{"health":"ok"}`), SessionId: "agent-session-1",
	})
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.Equal(t, "succeeded", stored.State)
	require.JSONEq(t, `{"health":"ok"}`, stored.ResultJSON)
	require.NotNil(t, stored.ObservedAt)

	var cursor model.NodeOperationRevision
	require.NoError(t, db.First(&cursor, "node_id = ?", node.ID).Error)
	require.Equal(t, int64(1), cursor.DesiredRevision)
	require.Equal(t, int64(1), cursor.ObservedRevision)
}

func TestKernelOperationBridgeAndTopologyExecutorCompletePluginStep(t *testing.T) {
	db := newKernelOperationBridgeDB(t)
	node := model.Node{Name: "topology-bridge-node", Host: "127.0.0.9", APIKey: "topology-bridge-key"}
	require.NoError(t, db.Create(&node).Error)
	seedAgentPluginRelease(t, db, "wireguard")
	topology := model.Topology{Name: "topology-bridge", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	revision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "draft", ContentHash: strings.Repeat("a", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&revision).Error)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "forward", PluginID: "wireguard", Role: "entry", DesiredVersion: "1.0.0", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&model.TopologyVertex{
		RevisionID: revision.ID, Key: "entry", Kind: "plugin", NodeID: &node.ID, PluginID: "wireguard", Role: "entry", ConfigJSON: `{"listen_port":51820}`,
	}).Error)
	deployment, _, err := service.PlanTopologyDeployment(db, service.TopologyDeploymentPlanInput{TopologyID: topology.ID, RevisionID: revision.ID, ActorID: 1})
	require.NoError(t, err)
	_, err = service.RequestTopologyDeploymentApply(db, deployment.ID, time.Now())
	require.NoError(t, err)
	executor, err := service.NewTopologyDeploymentExecutor(db)
	require.NoError(t, err)
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)

	stream := &kernelOperationStreamStub{connected: true, snapshot: AgentControlSnapshot{NodeID: uint32(node.ID), SessionID: "topology-agent-session"}}
	bridge, err := NewKernelOperationBridge(db, stream)
	require.NoError(t, err)
	_, err = bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Len(t, stream.operations, 1)
	configure := stream.operations[0]
	var configureOperation model.KernelOperation
	require.NoError(t, db.First(&configureOperation, "id = ?", configure.OperationId).Error)
	require.NotNil(t, configureOperation.TopologyDeploymentID)
	require.NotNil(t, configureOperation.TopologyStepID)
	require.Equal(t, revision.Revision, configureOperation.TopologyRevision)
	var envelope struct {
		Config json.RawMessage `json:"config"`
	}
	require.NoError(t, json.Unmarshal(configure.PayloadJson, &envelope))
	require.JSONEq(t, `{"listen_port":51820}`, string(envelope.Config))
	stream.observe(uint32(node.ID), &agentv1pb.ObservedState{
		OperationId: configure.OperationId, Revision: configure.Revision, Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
		StateJson: []byte(`{"health":"ok"}`), SessionId: "topology-agent-session",
	})

	// The bridge persists the operation result but does not prematurely mark
	// the node topology-succeeded before the subsequent enable operation.
	var observedCount int64
	require.NoError(t, db.Model(&model.TopologyObservedState{}).Where("deployment_id = ?", deployment.ID).Count(&observedCount).Error)
	require.Zero(t, observedCount)
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	_, err = bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Len(t, stream.operations, 2)
	enable := stream.operations[1]
	require.Equal(t, "plugin.enable", enable.Kind)
	stream.observe(uint32(node.ID), &agentv1pb.ObservedState{
		OperationId: enable.OperationId, Revision: enable.Revision, Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
		StateJson: []byte(`{"health":"ok"}`), SessionId: "topology-agent-session",
	})
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	status, err := service.GetTopologyDeploymentStatus(db, deployment.ID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", status.Deployment.State)
	require.Len(t, status.Observed, 1)
	require.Equal(t, "succeeded", status.Observed[0].State)
}

func TestKernelOperationBridgeLeavesPendingWorkUntilAgentConnects(t *testing.T) {
	db := newKernelOperationBridgeDB(t)
	node := model.Node{Name: "offline-node", Host: "127.0.0.2"}
	require.NoError(t, db.Create(&node).Error)
	seedAgentPluginRelease(t, db, "wireguard")
	deadline := time.Now().Add(time.Minute)
	operation, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: "e5d2649b-d96b-41e8-befe-35d6a5f978fc", IdempotencyKey: "offline-config-1", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.health", ConfigJSON: `{}`, DeadlineAt: &deadline,
	})
	require.NoError(t, err)

	stream := &kernelOperationStreamStub{}
	bridge, err := NewKernelOperationBridge(db, stream)
	require.NoError(t, err)
	count, err := bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
	require.Empty(t, stream.operations)

	var stored model.KernelOperation
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.Equal(t, "pending", stored.State)
	require.Empty(t, stored.SessionID)
}

func TestKernelOperationBridgeReplaysRunningOperationAfterControlRestart(t *testing.T) {
	db := newKernelOperationBridgeDB(t)
	node := model.Node{Name: "restart-node", Host: "127.0.0.3", APIKey: "restart-node-key"}
	require.NoError(t, db.Create(&node).Error)
	seedAgentPluginRelease(t, db, "wireguard")
	deadline := time.Now().Add(time.Minute)
	operation, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: "466090fc-1510-4ca9-9ea8-61fc4f14039f", IdempotencyKey: "restart-running-1", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.health", ConfigJSON: `{}`, DeadlineAt: &deadline,
	})
	require.NoError(t, err)

	firstStream := &kernelOperationStreamStub{connected: true, snapshot: AgentControlSnapshot{NodeID: uint32(node.ID), SessionID: "control-before-restart"}}
	firstBridge, err := NewKernelOperationBridge(db, firstStream)
	require.NoError(t, err)
	count, err := firstBridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)

	secondStream := &kernelOperationStreamStub{connected: true, snapshot: AgentControlSnapshot{NodeID: uint32(node.ID), SessionID: "control-after-restart"}}
	restartedBridge, err := NewKernelOperationBridge(db, secondStream)
	require.NoError(t, err)
	count, err = restartedBridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Len(t, secondStream.operations, 1)
	require.Equal(t, operation.ID, secondStream.operations[0].OperationId)

	var stored model.KernelOperation
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.Equal(t, "running", stored.State)
	require.Equal(t, "control-after-restart", stored.SessionID)
	restartedBridge.recordObserved(uint32(node.ID), &agentv1pb.ObservedState{
		OperationId: operation.ID, Revision: uint64(operation.Revision),
		Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, SessionId: "control-after-restart",
	})
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.Equal(t, "succeeded", stored.State)
}

func TestKernelOperationBridgeDeliversCancellationAndKeepsTerminalStateMonotonic(t *testing.T) {
	db := newKernelOperationBridgeDB(t)
	node := model.Node{Name: "cancel-node", Host: "127.0.0.4", APIKey: "cancel-node-key"}
	require.NoError(t, db.Create(&node).Error)
	seedAgentPluginRelease(t, db, "wireguard")
	deadline := time.Now().Add(time.Minute)
	operation, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: "9d45c832-24cf-45a2-8d30-c622207b0403", IdempotencyKey: "cancel-running-1", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.health", ConfigJSON: `{}`, DeadlineAt: &deadline,
	})
	require.NoError(t, err)
	stream := &kernelOperationStreamStub{connected: true, snapshot: AgentControlSnapshot{NodeID: uint32(node.ID), SessionID: "cancel-session"}}
	bridge, err := NewKernelOperationBridge(db, stream)
	require.NoError(t, err)
	count, err := bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)

	cancelled, err := service.CancelKernelOperation(db, operation.ID, time.Now())
	require.NoError(t, err)
	require.Equal(t, "cancel_requested", cancelled.State)
	count, err = bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
	require.Equal(t, []string{operation.ID}, stream.cancellations)

	var stored model.KernelOperation
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.NotNil(t, stored.CancelDispatchedAt)
	bridge.recordObserved(uint32(node.ID), &agentv1pb.ObservedState{
		OperationId: operation.ID, Revision: uint64(operation.Revision),
		Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_APPLYING, SessionId: "cancel-session",
	})
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.Equal(t, "cancel_requested", stored.State)
	bridge.recordObserved(uint32(node.ID), &agentv1pb.ObservedState{
		OperationId: operation.ID, Revision: uint64(operation.Revision),
		Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, SessionId: "cancel-session",
	})
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.Equal(t, "cancelled", stored.State)

	bridge.recordObserved(uint32(node.ID), &agentv1pb.ObservedState{
		OperationId: operation.ID, Revision: uint64(operation.Revision),
		Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_APPLYING, SessionId: "cancel-session",
	})
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.Equal(t, "cancelled", stored.State)
}
