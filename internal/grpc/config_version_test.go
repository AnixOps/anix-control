package grpc

import (
	"context"
	"math"
	"net"
	"testing"
	"time"

	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// uniqueTestKey generates a unique API key for test nodes.
func uniqueTestKey(prefix string) string {
	return prefix + "-" + uuid.New().String()[:8]
}

// ---------------------------------------------------------------------------
// Config version edge-case tests (unit-level, no gRPC server needed)
// ---------------------------------------------------------------------------

// TestConfigVersion_FirstConnection_NoVersionSet verifies that a node
// without any recorded version always reports a config change.
func TestConfigVersion_FirstConnection_NoVersionSet(t *testing.T) {
	mgr := NewNodeConnectionManager()
	nodeID := uint32(1)

	// No version set at all
	assert.True(t, mgr.IsConfigChanged(nodeID, 0),
		"first check with no version set should return true")

	// Even if the caller claims version 999, first connection should still push
	assert.True(t, mgr.IsConfigChanged(nodeID, 999),
		"first check should return true regardless of caller version")
}

// TestConfigVersion_SameVersion_NoChange verifies that when the caller's
// version equals the server's tracked version, no change is reported.
func TestConfigVersion_SameVersion_NoChange(t *testing.T) {
	mgr := NewNodeConnectionManager()
	nodeID := uint32(2)
	mgr.SetNodeConfigVersion(nodeID, 42)

	assert.False(t, mgr.IsConfigChanged(nodeID, 42),
		"same version should not trigger a change")
}

// TestConfigVersion_Decrease_NoChange verifies that when the caller's
// version is lower than the server's tracked version (i.e., a decrease),
// no change is reported. This prevents stale configs from triggering
// unnecessary pushes.
func TestConfigVersion_Decrease_NoChange(t *testing.T) {
	mgr := NewNodeConnectionManager()
	nodeID := uint32(3)
	mgr.SetNodeConfigVersion(nodeID, 100)

	// Caller has an older version (50) than server (100)
	assert.False(t, mgr.IsConfigChanged(nodeID, 50),
		"decreased version should not trigger a change")

	// Caller version 0 (reset) while server is at 100
	assert.False(t, mgr.IsConfigChanged(nodeID, 0),
		"version 0 against server 100 should not trigger a change")
}

// TestConfigVersion_Int64Overflow_EdgeCase verifies behavior when
// versions approach or wrap around int64 boundaries.
func TestConfigVersion_Int64Overflow_EdgeCase(t *testing.T) {
	mgr := NewNodeConnectionManager()
	nodeID := uint32(4)

	// Set version to max int64
	mgr.SetNodeConfigVersion(nodeID, math.MaxInt64)

	// At max, same version -> no change
	assert.False(t, mgr.IsConfigChanged(nodeID, math.MaxInt64),
		"max int64 same version should not change")

	// If a caller sends a smaller version (simulating overflow wrap to negative
	// or a reset), the server still sees it as a decrease -> no change
	assert.False(t, mgr.IsConfigChanged(nodeID, 0),
		"caller at 0 while server at MaxInt64 should not trigger change")

	// Now simulate a version "wrap": server records a lower version
	// (this is what happens when the version counter wraps around int64)
	mgr.SetNodeConfigVersion(nodeID, 0)

	// Caller at MaxInt64 while server at 0: caller is "newer" -> change
	assert.True(t, mgr.IsConfigChanged(nodeID, math.MaxInt64),
		"caller at MaxInt64 while server reset to 0 should trigger change")

	// Caller at 1 while server at 0: change
	assert.True(t, mgr.IsConfigChanged(nodeID, 1),
		"caller at 1 while server at 0 should trigger change")
}

// TestConfigVersion_ZeroVersion_Semantics verifies that version 0 is
// treated as a valid version (not as "unset").
func TestConfigVersion_ZeroVersion_Semantics(t *testing.T) {
	mgr := NewNodeConnectionManager()
	nodeID := uint32(5)

	// Before SetNodeConfigVersion: version is 0 (map default), but the
	// key doesn't exist, so IsConfigChanged returns true.
	assert.True(t, mgr.IsConfigChanged(nodeID, 0),
		"unset key should return true")

	// After explicitly setting to 0: the key now exists with value 0
	mgr.SetNodeConfigVersion(nodeID, 0)
	assert.False(t, mgr.IsConfigChanged(nodeID, 0),
		"explicitly set version 0 should not trigger change")

	// Caller at 1 > server 0: change
	assert.True(t, mgr.IsConfigChanged(nodeID, 1),
		"caller at 1 with server at 0 should trigger change")
}

// TestConfigVersion_NegativeVersion_Behavior verifies behavior with
// negative versions (possible after int64 overflow in Go).
func TestConfigVersion_NegativeVersion_Behavior(t *testing.T) {
	mgr := NewNodeConnectionManager()
	nodeID := uint32(6)

	mgr.SetNodeConfigVersion(nodeID, -100)

	// Caller at 0 > server at -100: change
	assert.True(t, mgr.IsConfigChanged(nodeID, 0),
		"caller at 0 vs server at -100 should trigger change")

	// Caller at -100 == server: no change
	assert.False(t, mgr.IsConfigChanged(nodeID, -100),
		"same negative version should not trigger change")

	// Caller at -200 < server at -100: no change
	assert.False(t, mgr.IsConfigChanged(nodeID, -200),
		"caller at -200 vs server at -100 should not trigger change")
}

// ---------------------------------------------------------------------------
// Multiple nodes with independent version tracking
// ---------------------------------------------------------------------------

// TestConfigVersion_MultipleNodes_IndependentTracking verifies that each
// node maintains its own version independently.
func TestConfigVersion_MultipleNodes_IndependentTracking(t *testing.T) {
	mgr := NewNodeConnectionManager()

	// Register three nodes with different versions
	mgr.Register(1, "10.0.0.1:1111")
	mgr.Register(2, "10.0.0.2:2222")
	mgr.Register(3, "10.0.0.3:3333")

	mgr.SetNodeConfigVersion(1, 10)
	mgr.SetNodeConfigVersion(2, 20)
	mgr.SetNodeConfigVersion(3, 30)

	// Each node reports its own version
	assert.Equal(t, int64(10), mgr.GetConfigVersion(1))
	assert.Equal(t, int64(20), mgr.GetConfigVersion(2))
	assert.Equal(t, int64(30), mgr.GetConfigVersion(3))

	// IsConfigChanged is independent per node
	assert.False(t, mgr.IsConfigChanged(1, 10))
	assert.True(t, mgr.IsConfigChanged(1, 15))

	assert.False(t, mgr.IsConfigChanged(2, 20))
	assert.True(t, mgr.IsConfigChanged(2, 25))

	assert.False(t, mgr.IsConfigChanged(3, 30))
	assert.True(t, mgr.IsConfigChanged(3, 35))

	// Update only node 2
	mgr.SetNodeConfigVersion(2, 50)

	// Node 1 and 3 unaffected
	assert.Equal(t, int64(10), mgr.GetConfigVersion(1))
	assert.Equal(t, int64(30), mgr.GetConfigVersion(3))
	assert.Equal(t, int64(50), mgr.GetConfigVersion(2))

	// Now node 1 at version 10 vs server at 10 -> no change
	assert.False(t, mgr.IsConfigChanged(1, 10))
	// But node 1 at version 15 vs server at 10 -> change
	assert.True(t, mgr.IsConfigChanged(1, 15))
}

// TestConfigVersion_MultipleNodes_ConcurrentUpdates verifies concurrent
// version updates across multiple nodes don't interfere.
func TestConfigVersion_MultipleNodes_ConcurrentUpdates(t *testing.T) {
	mgr := NewNodeConnectionManager()
	numNodes := 50

	// Register all nodes
	for i := 1; i <= numNodes; i++ {
		mgr.Register(uint32(i), "10.0.0.1:0")
		mgr.SetNodeConfigVersion(uint32(i), int64(i*100))
	}

	// Verify all versions
	for i := 1; i <= numNodes; i++ {
		assert.Equal(t, int64(i*100), mgr.GetConfigVersion(uint32(i)),
			"node %d version mismatch", i)
	}

	// Concurrently update half the nodes
	done := make(chan struct{})
	go func() {
		for i := 1; i <= numNodes; i += 2 {
			mgr.SetNodeConfigVersion(uint32(i), int64(i*100+1))
		}
		close(done)
	}()
	<-done

	// Odd nodes updated, even nodes unchanged
	for i := 1; i <= numNodes; i++ {
		expected := int64(i * 100)
		if i%2 == 1 {
			expected = int64(i*100 + 1)
		}
		assert.Equal(t, expected, mgr.GetConfigVersion(uint32(i)),
			"node %d version should be %d", i, expected)
	}
}

// ---------------------------------------------------------------------------
// UpdateConfigVersion vs SetNodeConfigVersion equivalence
// ---------------------------------------------------------------------------

// TestConfigVersion_UpdateVsSet_Equivalence verifies that UpdateConfigVersion
// and SetNodeConfigVersion are functionally equivalent (both write to configVer).
func TestConfigVersion_UpdateVsSet_Equivalence(t *testing.T) {
	mgr := NewNodeConnectionManager()
	nodeID := uint32(100)

	mgr.UpdateConfigVersion(nodeID, 42)
	assert.Equal(t, int64(42), mgr.GetConfigVersion(nodeID))

	mgr.SetNodeConfigVersion(nodeID, 99)
	assert.Equal(t, int64(99), mgr.GetConfigVersion(nodeID))

	// GetNodeConfigVersion should return the same value
	assert.Equal(t, int64(99), mgr.GetNodeConfigVersion(nodeID))
}

// ---------------------------------------------------------------------------
// Integration: version tracking through full StatusStream lifecycle
// ---------------------------------------------------------------------------

// TestConfigVersion_StatusStream_Lifecycle verifies config version tracking
// through the complete StatusStream lifecycle: register -> heartbeat ->
// config change -> version bump -> notification -> push.
func TestConfigVersion_StatusStream_Lifecycle(t *testing.T) {
	cache.InitMemory()
	requireInMemoryDatabase(t)
	t.Cleanup(func() { requireDatabaseClosed(t) })
	requireAutoMigrate(t,
		&model.User{},
		&model.Plan{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.AuthorizedKey{},
	)
	db := database.Get()
	db.Exec("DELETE FROM v2_authorized_key")
	db.Exec("DELETE FROM v2_node_protocol")
	db.Exec("DELETE FROM v2_node")
	db.Exec("DELETE FROM v2_plan")
	db.Exec("DELETE FROM v2_user")

	// Reset connection manager
	connectionManager = NewNodeConnectionManager()

	// Start a local gRPC server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()

	server := grpc.NewServer()
	pb.RegisterNodeServiceServer(server, NewNodeGRPCServer())
	pb.RegisterHealthServiceServer(server, NewHealthGRPCServer())

	serverErr := serveGRPCServerForTest(t, server, lis)
	t.Cleanup(func() { stopGRPCServerForTest(t, server, serverErr) })
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { requireClientConnClosed(t, conn) })

	// Create a node in the database
	groupID := uint(1)
	node := &model.Node{
		Name:    "lifecycle-node",
		Host:    "10.0.0.200",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  uniqueTestKey("lc"),
	}
	require.NoError(t, db.Create(node).Error)

	mgr := GetConnectionManager()

	// Phase 1: Open StatusStream, send initial heartbeat to register
	client := pb.NewNodeServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StatusStream(ctx)
	require.NoError(t, err)

	require.NoError(t, stream.Send(&pb.NodeStatusRequest{
		NodeId:      uint32(node.ID),
		CpuUsage:    30.0,
		MemoryUsage: 50.0,
		DiskUsage:   40.0,
		Uptime:      120,
		OnlineUsers: 5,
	}))
	time.Sleep(50 * time.Millisecond)

	// Verify node is registered
	_, ok := mgr.GetConnection(uint32(node.ID))
	require.True(t, ok, "node should be registered after heartbeat")

	// Phase 2: Simulate server setting a config version
	mgr.SetNodeConfigVersion(uint32(node.ID), 1)

	// Verify version is tracked
	ver := mgr.GetConfigVersion(uint32(node.ID))
	assert.Equal(t, int64(1), ver, "config version should be 1")

	// Phase 3: Simulate a config update on the server side
	// The admin updates the config, which bumps the version to 2
	mgr.UpdateConfigVersion(uint32(node.ID), 2)

	// The node still has version 1, so it should detect a change
	// (from the node's perspective: its last pushed version was 1, server is at 2)
	// Note: IsConfigChanged checks if caller's currentVer > server's lastVer.
	// Here the "caller" is the server checking if node needs update.
	// So if node reports version 1, and server is at 2:
	//   IsConfigChanged(nodeID, 1) -> 1 > 2? false -> no change from node's perspective
	// But the server can also check: IsConfigChanged(nodeID, 2) -> 2 > 2? false
	// The actual flow is: server calls IsConfigChanged with node's reported version.

	// Phase 4: Trigger config change notification
	mgr.NotifyConfigChange(uint32(node.ID))

	// Verify signal arrived in configChan
	connEntry, ok := mgr.GetConnection(uint32(node.ID))
	require.True(t, ok)

	select {
	case <-connEntry.configChan:
		// Config change signal received
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for config change signal")
	}

	// Phase 5: Node receives the config, applies it, and reports new version
	mgr.SetNodeConfigVersion(uint32(node.ID), 2)
	assert.Equal(t, int64(2), mgr.GetConfigVersion(uint32(node.ID)),
		"version should be updated to 2 after node acknowledges")

	// Verify no more signals in the channel
	select {
	case <-connEntry.configChan:
		t.Fatal("configChan should be empty after consuming the signal")
	default:
		// Expected: channel is empty
	}

	// Clean up
	requireCloseSend(t, stream)
}

// TestConfigVersion_StatusStream_MultipleChanges verifies that multiple
// sequential config changes are tracked correctly through the stream.
func TestConfigVersion_StatusStream_MultipleChanges(t *testing.T) {
	cache.InitMemory()
	requireInMemoryDatabase(t)
	t.Cleanup(func() { requireDatabaseClosed(t) })
	requireAutoMigrate(t,
		&model.Node{},
		&model.AuthorizedKey{},
	)
	db := database.Get()
	db.Exec("DELETE FROM v2_authorized_key")
	db.Exec("DELETE FROM v2_node")

	// Reset connection manager
	connectionManager = NewNodeConnectionManager()

	// Start a local gRPC server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()

	server := grpc.NewServer()
	pb.RegisterNodeServiceServer(server, NewNodeGRPCServer())
	pb.RegisterHealthServiceServer(server, NewHealthGRPCServer())

	serverErr := serveGRPCServerForTest(t, server, lis)
	t.Cleanup(func() { stopGRPCServerForTest(t, server, serverErr) })
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { requireClientConnClosed(t, conn) })

	// Create a node
	groupID := uint(1)
	node := &model.Node{
		Name:    "multi-change-node",
		Host:    "10.0.0.201",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  uniqueTestKey("mc"),
	}
	require.NoError(t, db.Create(node).Error)

	mgr := GetConnectionManager()

	client := pb.NewNodeServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StatusStream(ctx)
	require.NoError(t, err)

	// Register node
	require.NoError(t, stream.Send(&pb.NodeStatusRequest{
		NodeId: uint32(node.ID),
	}))
	time.Sleep(50 * time.Millisecond)

	mgr.SetNodeConfigVersion(uint32(node.ID), 1)

	// Send 5 config changes sequentially
	for v := int64(2); v <= 6; v++ {
		mgr.UpdateConfigVersion(uint32(node.ID), v)
		mgr.NotifyConfigChange(uint32(node.ID))

		// Consume the signal
		connEntry, ok := mgr.GetConnection(uint32(node.ID))
		require.True(t, ok)

		select {
		case <-connEntry.configChan:
			// Signal received
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for config change signal for version %d", v)
		}

		// Node acknowledges by setting its version
		mgr.SetNodeConfigVersion(uint32(node.ID), v)
		assert.Equal(t, v, mgr.GetConfigVersion(uint32(node.ID)),
			"version should be %d after acknowledgment", v)
	}

	requireCloseSend(t, stream)
}

// TestConfigVersion_Unregister_ClearsState verifies that unregistering a
// node removes its connection but preserves the config version (since
// configVer is a separate map from connections).
func TestConfigVersion_Unregister_PreservesVersion(t *testing.T) {
	mgr := NewNodeConnectionManager()
	nodeID := uint32(42)

	mgr.Register(nodeID, "10.0.0.1:1234")
	mgr.SetNodeConfigVersion(nodeID, 77)

	// Unregister
	mgr.Unregister(nodeID)

	// Connection should be gone
	_, ok := mgr.GetConnection(nodeID)
	assert.False(t, ok, "connection should be removed after unregister")

	// Config version should still be accessible (separate map)
	assert.Equal(t, int64(77), mgr.GetConfigVersion(nodeID),
		"config version should persist after unregister")
}

// TestConfigVersion_GetNodeConfigVersion_MatchesGetConfigVersion verifies
// that GetNodeConfigVersion returns the same value as GetConfigVersion.
func TestConfigVersion_GetNodeConfigVersion_MatchesGetConfigVersion(t *testing.T) {
	mgr := NewNodeConnectionManager()
	nodeID := uint32(55)

	// Before setting: both should return 0 (default)
	assert.Equal(t, int64(0), mgr.GetConfigVersion(nodeID))
	assert.Equal(t, int64(0), mgr.GetNodeConfigVersion(nodeID))

	mgr.SetNodeConfigVersion(nodeID, 123)

	assert.Equal(t, int64(123), mgr.GetConfigVersion(nodeID))
	assert.Equal(t, int64(123), mgr.GetNodeConfigVersion(nodeID))
}

// TestCheckConfigChanges_DetectsAndPushes verifies that checkConfigChanges
// pushes config on first check, suppresses repeats at the same version, and
// re-pushes after the node's UpdatedAt advances.
func TestCheckConfigChanges_DetectsAndPushes(t *testing.T) {
	cache.InitMemory()
	requireInMemoryDatabase(t)
	t.Cleanup(func() { requireDatabaseClosed(t) })
	requireAutoMigrate(t,
		&model.Node{},
		&model.NodeProtocol{},
		&model.AuthorizedKey{},
	)
	db := database.Get()
	db.Exec("DELETE FROM v2_authorized_key")
	db.Exec("DELETE FROM v2_node_protocol")
	db.Exec("DELETE FROM v2_node")

	connectionManager = NewNodeConnectionManager()

	groupID := uint(1)
	node := &model.Node{
		Name:    "cfg-change-node",
		Host:    "10.0.0.210",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  uniqueTestKey("cc"),
	}
	require.NoError(t, db.Create(node).Error)

	srv := NewNodeGRPCServer()
	nodeID := uint32(node.ID)

	// First check: no version recorded -> should push config.
	resp, err := srv.checkConfigChanges(nodeID)
	require.NoError(t, err)
	require.NotNil(t, resp, "first check should push config")
	assert.Equal(t, node.Host, resp.Host)

	// Second check at the same version -> no push.
	resp, err = srv.checkConfigChanges(nodeID)
	require.NoError(t, err)
	assert.Nil(t, resp, "same version should not push again")

	// Advance the node's UpdatedAt to simulate a config edit.
	newTime := node.UpdatedAt.Add(10 * time.Second)
	require.NoError(t, db.Model(node).Update("updated_at", newTime).Error)

	resp, err = srv.checkConfigChanges(nodeID)
	require.NoError(t, err)
	require.NotNil(t, resp, "advanced version should push config again")
}

// TestCheckConfigChanges_UnknownNode verifies an error is returned for a
// node that does not exist.
func TestCheckConfigChanges_UnknownNode(t *testing.T) {
	cache.InitMemory()
	requireInMemoryDatabase(t)
	t.Cleanup(func() { requireDatabaseClosed(t) })
	requireAutoMigrate(t, &model.Node{}, &model.AuthorizedKey{})

	connectionManager = NewNodeConnectionManager()
	srv := NewNodeGRPCServer()

	_, err := srv.checkConfigChanges(999999)
	assert.Error(t, err, "unknown node should return an error")
}
