package grpc

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/v3/api/grpc/agent/v1"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/service"
	"github.com/glebarez/sqlite"
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
