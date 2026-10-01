package grpc

import (
	"context"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// uniqueKey generates a unique API key for test nodes to avoid UNIQUE constraint failures.
func uniqueKey(prefix string) string {
	return prefix + "-" + uuid.New().String()[:8]
}

// StreamBidirectionalTestSuite tests for bidirectional gRPC stream patterns.
// Covers: StatusStream heartbeat/LastSeen, UserChanges notifications,
// ConfigChanges version-based sync, goroutine cleanup, concurrent
// NotifyConfigChange safety, and context cancellation.
type StreamBidirectionalTestSuite struct {
	suite.Suite
	server     *grpc.Server
	serverErr  <-chan error
	clientConn *grpc.ClientConn
	addr       string
}

func (s *StreamBidirectionalTestSuite) SetupSuite() {
	cache.InitMemory()
	requireInMemoryDatabase(s.T())
	requireAutoMigrate(s.T(),
		&model.User{},
		&model.Plan{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.AuthorizedKey{},
		&model.TrafficLog{},
		&model.OnlineLog{},
		&model.StatUser{},
		&model.StatServer{},
	)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(s.T(), err)
	s.addr = lis.Addr().String()

	s.server = grpc.NewServer(adminCallerServerOptionsForTest()...)
	pb.RegisterNodeServiceServer(s.server, NewNodeGRPCServer())
	pb.RegisterUserServiceServer(s.server, NewUserGRPCServer())
	pb.RegisterTrafficServiceServer(s.server, NewTrafficGRPCServer())
	pb.RegisterHealthServiceServer(s.server, NewHealthGRPCServer())

	s.serverErr = serveGRPCServerForTest(s.T(), s.server, lis)
	time.Sleep(100 * time.Millisecond)

	s.clientConn, err = grpc.NewClient(s.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(s.T(), err)

	// grpc.NewClient connects lazily. Establish the transport up front so the
	// goroutine-leak tests do not count the connection's own goroutines,
	// regardless of which test issues the first RPC.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = pb.NewHealthServiceClient(s.clientConn).Check(ctx, &pb.HealthCheckRequest{NodeId: 1})
	require.NoError(s.T(), err)
}

func (s *StreamBidirectionalTestSuite) TearDownSuite() {
	if s.clientConn != nil {
		requireClientConnClosed(s.T(), s.clientConn)
	}
	if s.server != nil {
		stopGRPCServerForTest(s.T(), s.server, s.serverErr)
	}
	requireDatabaseClosed(s.T())
}

func (s *StreamBidirectionalTestSuite) SetupTest() {
	// Ensure tables exist (re-migrate in case DB was reset by concurrent access)
	requireAutoMigrate(s.T(),
		&model.User{},
		&model.Plan{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.AuthorizedKey{},
		&model.TrafficLog{},
		&model.OnlineLog{},
		&model.StatUser{},
		&model.StatServer{},
	)
	db := database.Get()
	db.Exec("DELETE FROM v2_stat_server")
	db.Exec("DELETE FROM v2_stat_user")
	db.Exec("DELETE FROM v2_online_log")
	db.Exec("DELETE FROM v2_server_log")
	db.Exec("DELETE FROM v2_authorized_key")
	db.Exec("DELETE FROM v2_node_protocol")
	db.Exec("DELETE FROM v2_node")
	db.Exec("DELETE FROM v2_plan")
	db.Exec("DELETE FROM v2_user")
	// Reset connection manager for isolation
	connectionManager = NewNodeConnectionManager()
}

// ---------------------------------------------------------------------------
// 1. StatusStream: heartbeat updates LastSeen
// ---------------------------------------------------------------------------

// TestStatusStream_HeartbeatUpdatesLastSeen verifies that sending a heartbeat
// via StatusStream updates the node's LastSeen timestamp in the connection
// manager.
func (s *StreamBidirectionalTestSuite) TestStatusStream_HeartbeatUpdatesLastSeen() {
	db := database.Get()

	groupID := uint(1)
	node := &model.Node{
		Name:    "heartbeat-node",
		Host:    "10.0.0.100",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  uniqueKey("hk"),
	}
	require.NoError(s.T(), db.Create(node).Error)

	client := pb.NewNodeServiceClient(s.clientConn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StatusStream(ctx)
	require.NoError(s.T(), err)

	// Send initial heartbeat to register the node
	require.NoError(s.T(), stream.Send(&pb.NodeStatusRequest{
		NodeId:      uint32(node.ID),
		CpuUsage:    30.0,
		MemoryUsage: 50.0,
		DiskUsage:   40.0,
		Uptime:      120,
		OnlineUsers: 5,
	}))

	time.Sleep(50 * time.Millisecond)

	mgr := GetConnectionManager()
	conn, ok := mgr.connectionForTest(uint32(node.ID))
	require.True(s.T(), ok, "node should be registered after first heartbeat")
	firstSeen := conn.LastSeen

	// Wait a bit then send another heartbeat
	time.Sleep(100 * time.Millisecond)

	require.NoError(s.T(), stream.Send(&pb.NodeStatusRequest{
		NodeId:      uint32(node.ID),
		CpuUsage:    35.0,
		MemoryUsage: 55.0,
		DiskUsage:   40.0,
		Uptime:      220,
		OnlineUsers: 8,
	}))

	time.Sleep(50 * time.Millisecond)

	conn2, ok := mgr.connectionForTest(uint32(node.ID))
	require.True(s.T(), ok)
	assert.True(s.T(), conn2.LastSeen.After(firstSeen),
		"LastSeen should be updated after second heartbeat")

	requireCloseSend(s.T(), stream)
}

// TestStatusStream_MultipleHeartbeats verifies multiple sequential heartbeats
// all succeed and the node stays registered.
func (s *StreamBidirectionalTestSuite) TestStatusStream_MultipleHeartbeats() {
	db := database.Get()

	groupID := uint(1)
	node := &model.Node{
		Name:    "multi-hb-node",
		Host:    "10.0.0.101",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  uniqueKey("mhk"),
	}
	require.NoError(s.T(), db.Create(node).Error)

	client := pb.NewNodeServiceClient(s.clientConn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StatusStream(ctx)
	require.NoError(s.T(), err)

	// Send 5 heartbeats
	for i := 0; i < 5; i++ {
		require.NoError(s.T(), stream.Send(&pb.NodeStatusRequest{
			NodeId:      uint32(node.ID),
			CpuUsage:    float64(30 + i),
			MemoryUsage: 50.0,
			DiskUsage:   40.0,
			Uptime:      int64(120 + i*60),
			OnlineUsers: int32(5 + i),
		}))
		time.Sleep(20 * time.Millisecond)
	}

	mgr := GetConnectionManager()
	_, ok := mgr.connectionForTest(uint32(node.ID))
	assert.True(s.T(), ok, "node should remain registered after multiple heartbeats")

	requireCloseSend(s.T(), stream)
}

// ---------------------------------------------------------------------------
// 3. UserChanges: stream receives user change notifications
// ---------------------------------------------------------------------------

// TestUserChanges_StreamReceivesNotifications verifies that the UserChanges
// bidirectional stream correctly receives UserChangeNotification messages
// and responds with StatusResponse acknowledgements.
func (s *StreamBidirectionalTestSuite) TestUserChanges_StreamReceivesNotifications() {
	client := pb.NewUserServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.UserChanges(ctx)
	require.NoError(s.T(), err)

	// Send a CREATED notification
	require.NoError(s.T(), stream.Send(&pb.UserChangeNotification{
		Type: pb.UserChangeNotification_CREATED,
		User: &pb.UserInfo{
			Id:   1,
			Uuid: "test-user-uuid-001",
		},
		Timestamp: time.Now().Unix(),
	}))

	resp, err := stream.Recv()
	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Success)
	assert.Equal(s.T(), "notification received", resp.Message)

	// Send an UPDATED notification
	require.NoError(s.T(), stream.Send(&pb.UserChangeNotification{
		Type: pb.UserChangeNotification_UPDATED,
		User: &pb.UserInfo{
			Id:   1,
			Uuid: "test-user-uuid-001",
		},
		Timestamp: time.Now().Unix(),
	}))

	resp2, err := stream.Recv()
	require.NoError(s.T(), err)
	assert.True(s.T(), resp2.Success)

	requireCloseSend(s.T(), stream)
}

// TestUserChanges_StreamReceivesDeletedNotification verifies DELETED type.
func (s *StreamBidirectionalTestSuite) TestUserChanges_StreamReceivesDeletedNotification() {
	client := pb.NewUserServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.UserChanges(ctx)
	require.NoError(s.T(), err)

	require.NoError(s.T(), stream.Send(&pb.UserChangeNotification{
		Type: pb.UserChangeNotification_DELETED,
		User: &pb.UserInfo{
			Id:   42,
			Uuid: "deleted-user-uuid",
		},
		Timestamp: time.Now().Unix(),
	}))

	resp, err := stream.Recv()
	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Success)

	requireCloseSend(s.T(), stream)
}

// ---------------------------------------------------------------------------
// 4. ConfigChanges streaming: version-based incremental sync
// ---------------------------------------------------------------------------

// TestConfigChanges_VersionBasedIncrementalSync verifies that the
// ConfigChanges stream tracks configuration versions per node. On first
// registration a version is set; subsequent calls to SetNodeConfigVersion
// update it, and IsConfigChanged correctly reports whether a push is needed.
//
// Semantics: IsConfigChanged(nodeID, currentVer) compares the caller's
// currentVer against the server's tracked lastVer. It returns true when
// currentVer > lastVer, meaning the caller has a newer version than what the
// server last recorded (indicating the server needs to catch up).
func (s *StreamBidirectionalTestSuite) TestConfigChanges_VersionBasedIncrementalSync() {
	mgr := GetConnectionManager()
	nodeID := uint32(5001)

	// No version set yet: first check should indicate change needed
	assert.True(s.T(), mgr.IsConfigChanged(nodeID, 0),
		"unregistered node should always need config")

	// Register via stream (simulating the ConfigChanges flow)
	mgr.Register(nodeID, "10.0.0.1:9999")
	mgr.SetNodeConfigVersion(nodeID, 100)

	ver := mgr.configVersionForTest(nodeID)
	assert.Equal(s.T(), int64(100), ver)

	// currentVer == lastVer: no change
	assert.False(s.T(), mgr.IsConfigChanged(nodeID, 100))

	// currentVer < lastVer: no change (client has older version)
	assert.False(s.T(), mgr.IsConfigChanged(nodeID, 50))

	// currentVer > lastVer: change (client has newer version than server tracked)
	assert.True(s.T(), mgr.IsConfigChanged(nodeID, 200))

	// Update version on server side
	mgr.SetNodeConfigVersion(nodeID, 300)
	// Now server is at 300, client at 200: no change (200 < 300)
	assert.False(s.T(), mgr.IsConfigChanged(nodeID, 200))
	// Client at 300 matches server: no change
	assert.False(s.T(), mgr.IsConfigChanged(nodeID, 300))
	// Client at 400 exceeds server: change
	assert.True(s.T(), mgr.IsConfigChanged(nodeID, 400))

	mgr.Unregister(nodeID)
}

// TestConfigChanges_IncrementalVersionUpdates verifies that multiple
// config updates properly track the version.
func (s *StreamBidirectionalTestSuite) TestConfigChanges_IncrementalVersionUpdates() {
	mgr := GetConnectionManager()
	nodeID := uint32(5003)
	mgr.Register(nodeID, "10.0.0.1:8888")

	versions := []int64{10, 20, 30, 40}
	for _, v := range versions {
		mgr.SetNodeConfigVersion(nodeID, v)
		assert.Equal(s.T(), v, mgr.configVersionForTest(nodeID))
		// Client with older version: no change (currentVer < lastVer)
		assert.False(s.T(), mgr.IsConfigChanged(nodeID, v-5))
		// Client with same version: no change
		assert.False(s.T(), mgr.IsConfigChanged(nodeID, v))
		// Client with newer version: change
		assert.True(s.T(), mgr.IsConfigChanged(nodeID, v+5))
	}

	mgr.Unregister(nodeID)
}

// ---------------------------------------------------------------------------
// 5. Stream closes cleanly and goroutines exit (no goroutine leak)
// ---------------------------------------------------------------------------

// countGoroutines returns an approximate count of goroutines.
func countGoroutines() int {
	return runtime.NumGoroutine()
}

// TestStatusStream_CleanCloseNoGoroutineLeak verifies that opening and
// closing a StatusStream does not leak goroutines.
func (s *StreamBidirectionalTestSuite) TestStatusStream_CleanCloseNoGoroutineLeak() {
	db := database.Get()

	groupID := uint(1)
	node := &model.Node{
		Name:    "leak-test-node",
		Host:    "10.0.0.110",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  uniqueKey("lkt"),
	}
	require.NoError(s.T(), db.Create(node).Error)

	initialGoroutines := countGoroutines()

	// Open and close 3 streams
	for i := 0; i < 3; i++ {
		client := pb.NewNodeServiceClient(s.clientConn)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		stream, err := client.StatusStream(ctx)
		require.NoError(s.T(), err)

		require.NoError(s.T(), stream.Send(&pb.NodeStatusRequest{
			NodeId: uint32(node.ID),
		}))
		time.Sleep(20 * time.Millisecond)

		requireCloseSend(s.T(), stream)
		cancel()

		// Wait for server-side goroutine to clean up
		time.Sleep(100 * time.Millisecond)
	}

	// Give GC a moment
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := countGoroutines()
	// Allow some tolerance for internal goroutines
	assert.LessOrEqual(s.T(), finalGoroutines, initialGoroutines+2,
		"goroutine count should not increase significantly after stream close")
}

// TestHealthWatch_CleanClose verifies Health Watch stream closes cleanly.
func (s *StreamBidirectionalTestSuite) TestHealthWatch_CleanClose() {
	client := pb.NewHealthServiceClient(s.clientConn)

	initialGoroutines := countGoroutines()

	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		stream, err := client.Watch(ctx)
		require.NoError(s.T(), err)

		require.NoError(s.T(), stream.Send(&pb.HealthCheckRequest{NodeId: 1}))
		_, err = stream.Recv()
		require.NoError(s.T(), err)

		requireCloseSend(s.T(), stream)
		cancel()
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := countGoroutines()
	assert.LessOrEqual(s.T(), finalGoroutines, initialGoroutines+2,
		"health watch streams should not leak goroutines")
}

// TestTrafficStream_CleanClose verifies TrafficStream closes cleanly.
func (s *StreamBidirectionalTestSuite) TestTrafficStream_CleanClose() {
	db := database.Get()

	plan := &model.Plan{
		Name:           "traffic-close-plan",
		TransferEnable: 10737418240,
		Show:           1,
	}
	require.NoError(s.T(), db.Create(plan).Error)

	groupID := uint(1)
	node := &model.Node{
		Name:    "traffic-close-node",
		Host:    "10.0.0.111",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  uniqueKey("tck"),
	}
	require.NoError(s.T(), db.Create(node).Error)

	userPlanID := plan.ID
	userGroupID := uint(1)
	user := &model.User{
		Email:          "traffic-close@example.com",
		Password:       "hash",
		Token:          "tc-" + time.Now().String(),
		UUID:           "tc-uuid-001",
		PlanID:         &userPlanID,
		GroupID:        &userGroupID,
		TransferEnable: 10737418240,
	}
	require.NoError(s.T(), db.Create(user).Error)

	initialGoroutines := countGoroutines()

	client := pb.NewTrafficServiceClient(s.clientConn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	stream, err := client.TrafficStream(ctx)
	require.NoError(s.T(), err)

	traffics := map[uint32]*pb.TrafficData{
		uint32(user.ID): {Upload: 1024, Download: 2048},
	}
	require.NoError(s.T(), stream.Send(&pb.TrafficReportRequest{
		NodeId:   uint32(node.ID),
		Traffics: traffics,
	}))

	_, err = stream.Recv()
	require.NoError(s.T(), err)

	requireCloseSend(s.T(), stream)
	cancel()

	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := countGoroutines()
	assert.LessOrEqual(s.T(), finalGoroutines, initialGoroutines+2,
		"traffic stream should not leak goroutines")
}

// ---------------------------------------------------------------------------
// 7. Stream context cancellation stops goroutines
// ---------------------------------------------------------------------------

// TestStatusStream_ContextCancellation verifies that cancelling the
// context of a StatusStream causes the server-side handler to exit.
func (s *StreamBidirectionalTestSuite) TestStatusStream_ContextCancellation() {
	db := database.Get()

	groupID := uint(1)
	node := &model.Node{
		Name:    "cancel-node",
		Host:    "10.0.0.120",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  uniqueKey("ck"),
	}
	require.NoError(s.T(), db.Create(node).Error)

	client := pb.NewNodeServiceClient(s.clientConn)
	ctx, cancel := context.WithCancel(context.Background())

	stream, err := client.StatusStream(ctx)
	require.NoError(s.T(), err)

	// Register node
	require.NoError(s.T(), stream.Send(&pb.NodeStatusRequest{
		NodeId: uint32(node.ID),
	}))
	time.Sleep(50 * time.Millisecond)

	mgr := GetConnectionManager()
	_, ok := mgr.connectionForTest(uint32(node.ID))
	require.True(s.T(), ok, "node should be registered")

	// Cancel the context
	cancel()

	// Wait for server-side cleanup
	time.Sleep(200 * time.Millisecond)

	// Node should be unregistered (defer in StatusStream calls Unregister)
	_, ok = mgr.connectionForTest(uint32(node.ID))
	assert.False(s.T(), ok, "node should be unregistered after context cancellation")
}

// TestUserChanges_ContextCancellation verifies UserChanges stream exits
// when context is cancelled.
func (s *StreamBidirectionalTestSuite) TestUserChanges_ContextCancellation() {
	client := pb.NewUserServiceClient(s.clientConn)
	ctx, cancel := context.WithCancel(context.Background())

	stream, err := client.UserChanges(ctx)
	require.NoError(s.T(), err)

	// Send one notification
	require.NoError(s.T(), stream.Send(&pb.UserChangeNotification{
		Type:      pb.UserChangeNotification_CREATED,
		User:      &pb.UserInfo{Id: 1, Uuid: "cancel-test-uuid"},
		Timestamp: time.Now().Unix(),
	}))
	_, err = stream.Recv()
	require.NoError(s.T(), err)

	// Cancel context
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Subsequent send/recv should fail
	err = stream.Send(&pb.UserChangeNotification{
		Type:      pb.UserChangeNotification_UPDATED,
		User:      &pb.UserInfo{Id: 1, Uuid: "cancel-test-uuid"},
		Timestamp: time.Now().Unix(),
	})
	assert.Error(s.T(), err, "send after cancel should fail")
}

// TestTrafficStream_ContextCancellation verifies TrafficStream exits on cancel.
func (s *StreamBidirectionalTestSuite) TestTrafficStream_ContextCancellation() {
	db := database.Get()

	plan := &model.Plan{
		Name:           "traffic-cancel-plan",
		TransferEnable: 10737418240,
		Show:           1,
	}
	require.NoError(s.T(), db.Create(plan).Error)

	groupID := uint(1)
	node := &model.Node{
		Name:    "traffic-cancel-node",
		Host:    "10.0.0.121",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  uniqueKey("tsc"),
	}
	require.NoError(s.T(), db.Create(node).Error)

	userPlanID := plan.ID
	userGroupID := uint(1)
	user := &model.User{
		Email:          "traffic-cancel@example.com",
		Password:       "hash",
		Token:          "tsc-" + time.Now().String(),
		UUID:           "tsc-uuid-001",
		PlanID:         &userPlanID,
		GroupID:        &userGroupID,
		TransferEnable: 10737418240,
	}
	require.NoError(s.T(), db.Create(user).Error)

	client := pb.NewTrafficServiceClient(s.clientConn)
	ctx, cancel := context.WithCancel(context.Background())

	stream, err := client.TrafficStream(ctx)
	require.NoError(s.T(), err)

	traffics := map[uint32]*pb.TrafficData{
		uint32(user.ID): {Upload: 1024, Download: 2048},
	}
	require.NoError(s.T(), stream.Send(&pb.TrafficReportRequest{
		NodeId:   uint32(node.ID),
		Traffics: traffics,
	}))

	_, err = stream.Recv()
	require.NoError(s.T(), err)

	// Cancel context
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Subsequent send should fail
	err = stream.Send(&pb.TrafficReportRequest{
		NodeId:   uint32(node.ID),
		Traffics: traffics,
	})
	assert.Error(s.T(), err, "send after context cancel should fail")
}

// TestOnlineStream_ContextCancellation verifies OnlineStream exits on cancel.
func (s *StreamBidirectionalTestSuite) TestOnlineStream_ContextCancellation() {
	db := database.Get()

	groupID := uint(1)
	node := &model.Node{
		Name:    "online-cancel-node",
		Host:    "10.0.0.122",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  uniqueKey("osc"),
	}
	require.NoError(s.T(), db.Create(node).Error)

	client := pb.NewTrafficServiceClient(s.clientConn)
	ctx, cancel := context.WithCancel(context.Background())

	stream, err := client.OnlineStream(ctx)
	require.NoError(s.T(), err)

	online := map[uint32]*pb.OnlineData{
		1: {
			Ips:       []string{"192.0.2.10", "192.0.2.11"},
			Timestamp: time.Now().Unix(),
		},
	}
	require.NoError(s.T(), stream.Send(&pb.OnlineReportRequest{
		NodeId: uint32(node.ID),
		Online: online,
	}))

	_, err = stream.Recv()
	require.NoError(s.T(), err)

	cancel()
	time.Sleep(100 * time.Millisecond)

	err = stream.Send(&pb.OnlineReportRequest{
		NodeId: uint32(node.ID),
		Online: online,
	})
	assert.Error(s.T(), err, "send after context cancel should fail")
}

// ---------------------------------------------------------------------------
// 8. Connection manager edge cases
// ---------------------------------------------------------------------------

// TestConnectionManager_RegisterUnregisterRoundtrip verifies full lifecycle.
func (s *StreamBidirectionalTestSuite) TestConnectionManager_RegisterUnregisterRoundtrip() {
	mgr := NewNodeConnectionManager()

	// Register
	mgr.Register(1, "1.2.3.4:1234")
	conn, ok := mgr.connectionForTest(1)
	require.True(s.T(), ok)
	assert.Equal(s.T(), uint32(1), conn.NodeID)
	assert.Equal(s.T(), "1.2.3.4:1234", conn.RemoteAddr)
	assert.Len(s.T(), mgr.activeNodesForTest(), 1)

	// Unregister
	mgr.Unregister(1)
	_, ok = mgr.connectionForTest(1)
	assert.False(s.T(), ok)
	assert.Len(s.T(), mgr.activeNodesForTest(), 0)
}

// TestConnectionManager_UpdateLastSeen verifies the LastSeen field updates.
func (s *StreamBidirectionalTestSuite) TestConnectionManager_UpdateLastSeen() {
	mgr := NewNodeConnectionManager()
	mgr.Register(1, "1.2.3.4:1234")

	conn, _ := mgr.connectionForTest(1)
	before := conn.LastSeen

	time.Sleep(50 * time.Millisecond)
	mgr.UpdateLastSeen(1)

	after, _ := mgr.connectionForTest(1)
	assert.True(s.T(), after.LastSeen.After(before),
		"LastSeen should be updated")
}

// TestConnectionManager_ConfigVersionIndependence verifies config version
// tracking is independent of connection registration.
func (s *StreamBidirectionalTestSuite) TestConnectionManager_ConfigVersionIndependence() {
	mgr := NewNodeConnectionManager()

	// Set config version without registering connection
	mgr.SetNodeConfigVersion(999, 42)
	assert.Equal(s.T(), int64(42), mgr.configVersionForTest(999))

	// Now register the same node
	mgr.Register(999, "1.2.3.4:5678")
	// Config version should still be preserved
	assert.Equal(s.T(), int64(42), mgr.configVersionForTest(999))

	mgr.Unregister(999)
}

// TestConnectionManager_ConcurrentRegisterUnregister verifies no race
// conditions when registering and unregistering concurrently.
func (s *StreamBidirectionalTestSuite) TestConnectionManager_ConcurrentRegisterUnregister() {
	mgr := NewNodeConnectionManager()
	var wg sync.WaitGroup

	// Concurrently register many nodes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id uint32) {
			defer wg.Done()
			mgr.Register(id, "10.0.0.1:1234")
			mgr.UpdateLastSeen(id)
			mgr.SetNodeConfigVersion(id, int64(id))
		}(uint32(i))
	}
	wg.Wait()
	assert.Len(s.T(), mgr.activeNodesForTest(), 100)

	// Concurrently unregister
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id uint32) {
			defer wg.Done()
			mgr.Unregister(id)
		}(uint32(i))
	}
	wg.Wait()
	assert.Len(s.T(), mgr.activeNodesForTest(), 0)
}

// ---------------------------------------------------------------------------
// 10. Multiple streams concurrently
// ---------------------------------------------------------------------------

// TestMultipleConcurrentStreams verifies that multiple bidirectional streams
// can operate concurrently without interfering with each other.
func (s *StreamBidirectionalTestSuite) TestMultipleConcurrentStreams() {
	db := database.Get()

	// Create 3 nodes and track their IDs
	nodeIDs := make([]uint32, 3)
	for i := 0; i < 3; i++ {
		groupID := uint(i + 1)
		node := &model.Node{
			Name:    "concurrent-node",
			Host:    "10.0.0.140",
			Port:    443,
			GroupID: &groupID,
			Rate:    1.0,
			Show:    1,
			Status:  model.NodeStatusOnline,
			APIKey:  uniqueKey("cc"),
		}
		require.NoError(s.T(), db.Create(node).Error)
		nodeIDs[i] = uint32(node.ID)
	}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			nodeClient := pb.NewNodeServiceClient(s.clientConn)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			stream, err := nodeClient.StatusStream(ctx)
			if err != nil {
				return
			}

			// Use the actual database node ID
			_ = stream.Send(&pb.NodeStatusRequest{
				NodeId: nodeIDs[idx],
			})
			time.Sleep(50 * time.Millisecond)
			if err := stream.CloseSend(); err != nil {
				s.T().Errorf("close status stream: %v", err)
			}
		}(i)
	}

	// Should complete without deadlock
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		s.T().Fatal("concurrent streams caused a deadlock")
	}

	// Give server-side goroutines time to clean up before next test
	time.Sleep(200 * time.Millisecond)
}

// TestUserChanges_MultipleConcurrentStreams verifies concurrent
// UserChanges streams don't interfere.
func (s *StreamBidirectionalTestSuite) TestUserChanges_MultipleConcurrentStreams() {
	var wg sync.WaitGroup
	numStreams := 5

	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			client := pb.NewUserServiceClient(s.clientConn)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			stream, err := client.UserChanges(ctx)
			if err != nil {
				return
			}

			notification := &pb.UserChangeNotification{
				Type:      pb.UserChangeNotification_CREATED,
				User:      &pb.UserInfo{Id: uint32(idx), Uuid: "uuid"},
				Timestamp: time.Now().Unix(),
			}
			_ = stream.Send(notification)
			_, _ = stream.Recv()
			if err := stream.CloseSend(); err != nil {
				s.T().Errorf("close user changes stream: %v", err)
			}
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		s.T().Fatal("concurrent UserChanges streams caused a deadlock")
	}
}

func TestStreamBidirectionalTestSuite(t *testing.T) {
	suite.Run(t, new(StreamBidirectionalTestSuite))
}
