package grpc

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// nodeListenerForTest is the node-facing listener with the production
// interceptor chains, two proxy nodes and their API keys.
type nodeListenerForTest struct {
	conn  *grpc.ClientConn
	nodeA *model.Node
	nodeB *model.Node
}

const (
	nodeKeyAForTest = "node-a-api-key"
	nodeKeyBForTest = "node-b-api-key"
)

func startNodeListenerForTest(t *testing.T, cfg *ServerConfig) *nodeListenerForTest {
	t.Helper()
	cache.InitMemory()
	requireInMemoryDatabase(t)
	t.Cleanup(func() { requireDatabaseClosed(t) })
	requireAutoMigrate(t, &model.User{}, &model.Plan{}, &model.Node{}, &model.NodeProtocol{}, &model.AuthorizedKey{},
		&model.TrafficLog{}, &model.OnlineLog{}, &model.StatUser{}, &model.StatServer{}, &model.NodeLog{})
	connectionManager = NewNodeConnectionManager()

	groupID := uint(1)
	newNode := func(name, key string) *model.Node {
		node := &model.Node{Name: name, Host: "10.0.0.1", Port: 443, GroupID: &groupID, Rate: 1, Show: 1,
			Status: model.NodeStatusOnline, APIKey: key, APIKeyHash: apiKeyHashForTest(key)}
		require.NoError(t, database.Get().Create(node).Error)
		require.NoError(t, database.Get().Create(&model.NodeProtocol{NodeID: node.ID, Name: name, Type: model.ProtocolVLESS,
			Port: 443, Enable: 1}).Error)
		return node
	}
	l := &nodeListenerForTest{nodeA: newNode("node-a", nodeKeyAForTest), nodeB: newNode("node-b", nodeKeyBForTest)}

	unary, stream := NewServer(cfg).interceptorChains()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(unary...), grpc.ChainStreamInterceptor(stream...))
	pb.RegisterNodeServiceServer(server, NewNodeGRPCServer())
	RegisterNodeLogServiceServer(server, NewNodeLogGRPCServer())
	pb.RegisterUserServiceServer(server, NewUserGRPCServer())
	pb.RegisterTrafficServiceServer(server, NewTrafficGRPCServer())
	pb.RegisterHealthServiceServer(server, NewHealthGRPCServer())
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("passthrough:///node-listener",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	l.conn = conn
	return l
}

func nodeKeyContextForTest(t *testing.T, nodeID uint, key string) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return metadata.AppendToOutgoingContext(ctx, "x-api-key", key, "x-node-id", strconv.FormatUint(uint64(nodeID), 10))
}

func bearerContextForTest(t *testing.T, token string) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

// nodeScopedCallsForTest calls every node-scoped RPC for nodeID and returns
// each call's error by name. A stream sends one message and receives once.
func nodeScopedCallsForTest(ctx context.Context, conn *grpc.ClientConn, nodeID uint32) map[string]error {
	nodes := pb.NewNodeServiceClient(conn)
	users := pb.NewUserServiceClient(conn)
	traffic := pb.NewTrafficServiceClient(conn)
	errs := map[string]error{}

	_, errs["GetConfig"] = nodes.GetConfig(ctx, &pb.NodeConfigRequest{NodeId: nodeID})
	_, errs["ReportStatus"] = nodes.ReportStatus(ctx, &pb.NodeStatusRequest{NodeId: nodeID, CpuUsage: 1})
	_, errs["GetUsers"] = users.GetUsers(ctx, &pb.UserListRequest{NodeId: nodeID})
	_, errs["ReportTraffic"] = traffic.ReportTraffic(ctx, &pb.TrafficReportRequest{NodeId: nodeID})
	_, errs["ReportOnline"] = traffic.ReportOnline(ctx, &pb.OnlineReportRequest{NodeId: nodeID})
	errs["ReportLogs"] = conn.Invoke(ctx, NodeLogService_ReportLogs_FullMethodName,
		nodeLogRequestForTest(nodeID, nodeLogEntryForTest("binding test", time.Now().Unix())), &pb.StatusResponse{})

	if stream, err := nodes.StatusStream(ctx); err != nil {
		errs["StatusStream"] = err
	} else {
		errs["StatusStream"] = firstStreamReplyErrForTest(stream, &pb.NodeStatusRequest{NodeId: nodeID})
	}
	if stream, err := traffic.TrafficStream(ctx); err != nil {
		errs["TrafficStream"] = err
	} else {
		errs["TrafficStream"] = firstStreamReplyErrForTest(stream, &pb.TrafficReportRequest{NodeId: nodeID})
	}
	if stream, err := traffic.OnlineStream(ctx); err != nil {
		errs["OnlineStream"] = err
	} else {
		errs["OnlineStream"] = firstStreamReplyErrForTest(stream, &pb.OnlineReportRequest{NodeId: nodeID})
	}
	return errs
}

// firstStreamReplyErrForTest sends request on stream and returns the error
// of its first reply. A stream the server refuses can end before the
// request is written: Send then returns io.EOF and the stream's status is
// only observable through RecvMsg.
func firstStreamReplyErrForTest[Req, Res any](stream grpc.BidiStreamingClient[Req, Res], request *Req) error {
	if err := stream.Send(request); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	_, err := stream.Recv()
	return err
}

func requireCodesForTest(t *testing.T, want codes.Code, errs map[string]error) {
	t.Helper()
	require.Len(t, errs, 9)
	for method, err := range errs {
		assert.Equal(t, want, status.Code(err), "%s: %v", method, err)
	}
}

// Without grpc.api_token (the default) the listener used to accept any
// authorization header: every RPC must now be refused without a node key.
func TestNodeListenerWithoutGlobalTokenRefusesCallersWithoutNodeKey(t *testing.T) {
	l := startNodeListenerForTest(t, &ServerConfig{})

	requireCodesForTest(t, codes.Unauthenticated, nodeScopedCallsForTest(bearerContextForTest(t, "anything"), l.conn, uint32(l.nodeA.ID)))
	requireCodesForTest(t, codes.Unauthenticated, nodeScopedCallsForTest(bearerContextForTest(t, ""), l.conn, uint32(l.nodeA.ID)))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	requireCodesForTest(t, codes.Unauthenticated, nodeScopedCallsForTest(ctx, l.conn, uint32(l.nodeA.ID)))

	// The health check and node registration stay unauthenticated.
	_, err := pb.NewHealthServiceClient(l.conn).Check(ctx, &pb.HealthCheckRequest{})
	require.NoError(t, err)
	_, err = pb.NewNodeServiceClient(l.conn).Register(ctx, &pb.NodeRegisterRequest{AuthKey: "unknown"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "Register reaches its handler: %v", err)
}

func TestNodeListenerRefusesNodeKeyOfAnotherXNodeID(t *testing.T) {
	l := startNodeListenerForTest(t, &ServerConfig{})

	requireCodesForTest(t, codes.Unauthenticated, nodeScopedCallsForTest(nodeKeyContextForTest(t, l.nodeB.ID, nodeKeyAForTest), l.conn, uint32(l.nodeB.ID)))
	requireCodesForTest(t, codes.Unauthenticated, nodeScopedCallsForTest(nodeKeyContextForTest(t, l.nodeA.ID, "not-a-node-key"), l.conn, uint32(l.nodeA.ID)))
}

// A node's key acts only for its node: node A cannot read node B's
// configuration or users, nor report for it.
func TestNodeListenerBindsRequestsToTheAuthenticatedNode(t *testing.T) {
	l := startNodeListenerForTest(t, &ServerConfig{})
	before := *l.nodeB

	requireCodesForTest(t, codes.PermissionDenied, nodeScopedCallsForTest(nodeKeyContextForTest(t, l.nodeA.ID, nodeKeyAForTest), l.conn, uint32(l.nodeB.ID)))

	var after model.Node
	require.NoError(t, database.Get().First(&after, l.nodeB.ID).Error)
	assert.Equal(t, before.LastCheckAt, after.LastCheckAt, "node B's heartbeat is untouched")
	assert.Zero(t, after.CPUUsage)
	var logs int64
	require.NoError(t, database.Get().Model(&model.NodeLog{}).Where("node_id = ?", l.nodeB.ID).Count(&logs).Error)
	assert.Zero(t, logs)

	// The same key acts for its own node.
	requireCodesForTest(t, codes.OK, nodeScopedCallsForTest(nodeKeyContextForTest(t, l.nodeA.ID, nodeKeyAForTest), l.conn, uint32(l.nodeA.ID)))
}

// A node's stream may not switch to another node after its first message.
func TestNodeListenerEndsStreamsThatSwitchNode(t *testing.T) {
	l := startNodeListenerForTest(t, &ServerConfig{})
	ctx := nodeKeyContextForTest(t, l.nodeA.ID, nodeKeyAForTest)

	traffic, err := pb.NewTrafficServiceClient(l.conn).TrafficStream(ctx)
	require.NoError(t, err)
	require.NoError(t, traffic.Send(&pb.TrafficReportRequest{NodeId: uint32(l.nodeA.ID)}))
	_, err = traffic.Recv()
	require.NoError(t, err)
	require.NoError(t, traffic.Send(&pb.TrafficReportRequest{NodeId: uint32(l.nodeB.ID)}))
	_, err = traffic.Recv()
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	statusStream, err := pb.NewNodeServiceClient(l.conn).StatusStream(ctx)
	require.NoError(t, err)
	require.NoError(t, statusStream.Send(&pb.NodeStatusRequest{NodeId: uint32(l.nodeA.ID)}))
	_, err = statusStream.Recv() // the first configuration push
	require.NoError(t, err)
	require.NoError(t, statusStream.Send(&pb.NodeStatusRequest{NodeId: uint32(l.nodeB.ID), CpuUsage: 99}))
	_, err = statusStream.Recv()
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	var nodeB model.Node
	require.NoError(t, database.Get().First(&nodeB, l.nodeB.ID).Error)
	assert.Zero(t, nodeB.CPUUsage)
}

// The global grpc.api_token is the administrator's: it acts for any node.
func TestNodeListenerGlobalAPITokenActsForAnyNode(t *testing.T) {
	const apiToken = "global-admin-token"
	l := startNodeListenerForTest(t, &ServerConfig{APIToken: apiToken})

	requireCodesForTest(t, codes.OK, nodeScopedCallsForTest(bearerContextForTest(t, apiToken), l.conn, uint32(l.nodeA.ID)))
	requireCodesForTest(t, codes.OK, nodeScopedCallsForTest(bearerContextForTest(t, apiToken), l.conn, uint32(l.nodeB.ID)))
	requireCodesForTest(t, codes.Unauthenticated, nodeScopedCallsForTest(bearerContextForTest(t, apiToken+"x"), l.conn, uint32(l.nodeB.ID)))
	// Node keys keep working next to it, bound to their node.
	requireCodesForTest(t, codes.PermissionDenied, nodeScopedCallsForTest(nodeKeyContextForTest(t, l.nodeA.ID, nodeKeyAForTest), l.conn, uint32(l.nodeB.ID)))
}

// With a JWT secret configured only an administrator's JWT authenticates:
// a user's login JWT never reads node configurations.
func TestNodeListenerJWTRequiresAnAdministrator(t *testing.T) {
	const secret = "grpc-listener-jwt-secret-with-enough-length"
	l := startNodeListenerForTest(t, &ServerConfig{JWTSecret: secret})
	userToken, err := utils.GenerateToken(8, "user@example.com", false, secret, 3600)
	require.NoError(t, err)
	adminToken, err := utils.GenerateToken(1, "admin@example.com", true, secret, 3600)
	require.NoError(t, err)

	requireCodesForTest(t, codes.Unauthenticated, nodeScopedCallsForTest(bearerContextForTest(t, userToken), l.conn, uint32(l.nodeB.ID)))
	requireCodesForTest(t, codes.OK, nodeScopedCallsForTest(bearerContextForTest(t, adminToken), l.conn, uint32(l.nodeB.ID)))
}

// A disabled node's key is refused, as the HTTP node API and the Agent
// control stream refuse it, and heartbeats never re-enable the node.
func TestNodeListenerDisabledNodeStaysDisabled(t *testing.T) {
	const apiToken = "global-admin-token"
	l := startNodeListenerForTest(t, &ServerConfig{APIToken: apiToken})
	db := database.Get()

	// The node's stream is open when an administrator disables the node.
	ctx := nodeKeyContextForTest(t, l.nodeA.ID, nodeKeyAForTest)
	statusStream, err := pb.NewNodeServiceClient(l.conn).StatusStream(ctx)
	require.NoError(t, err)
	require.NoError(t, statusStream.Send(&pb.NodeStatusRequest{NodeId: uint32(l.nodeA.ID)}))
	_, err = statusStream.Recv()
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", l.nodeA.ID).Updates(map[string]any{
		"status": model.NodeStatusDisabled, "last_check_at": 1}).Error)

	require.NoError(t, statusStream.Send(&pb.NodeStatusRequest{NodeId: uint32(l.nodeA.ID), CpuUsage: 12}))
	_, err = statusStream.Recv()
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "the stream ends instead of pushing configuration")
	var node model.Node
	require.NoError(t, db.First(&node, l.nodeA.ID).Error)
	assert.Equal(t, model.NodeStatusDisabled, node.Status)
	require.NotNil(t, node.LastCheckAt)
	assert.Greater(t, *node.LastCheckAt, int64(1), "the heartbeat still records the node's liveness")

	// Its key no longer gets configuration, users or anything else.
	requireCodesForTest(t, codes.PermissionDenied, nodeScopedCallsForTest(ctx, l.conn, uint32(l.nodeA.ID)))

	// An administrator's heartbeats for it leave it disabled too. Its
	// status stream ends: a disabled node is pushed no configuration.
	adminErrs := nodeScopedCallsForTest(bearerContextForTest(t, apiToken), l.conn, uint32(l.nodeA.ID))
	assert.Equal(t, codes.PermissionDenied, status.Code(adminErrs["StatusStream"]))
	delete(adminErrs, "StatusStream")
	for method, err := range adminErrs {
		assert.NoError(t, err, method)
	}
	require.NoError(t, db.First(&node, l.nodeA.ID).Error)
	assert.Equal(t, model.NodeStatusDisabled, node.Status)
}
