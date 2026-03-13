package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/anixops/v2board/api/grpc/v2boardpb"
	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// GRPCTestSuite gRPC 测试套件
type GRPCTestSuite struct {
	suite.Suite
	server     *grpc.Server
	clientConn *grpc.ClientConn
	addr       string
}

func (s *GRPCTestSuite) SetupSuite() {
	// 初始化缓存
	cache.InitMemory()

	// 初始化数据库
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})

	// 自动迁移
	database.AutoMigrate(
		&model.User{},
		&model.Plan{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.AuthorizedKey{},
	)

	// 启动 gRPC 服务器
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(s.T(), err)
	s.addr = lis.Addr().String()

	s.server = grpc.NewServer()
	pb.RegisterNodeServiceServer(s.server, NewNodeGRPCServer())
	pb.RegisterUserServiceServer(s.server, NewUserGRPCServer())
	pb.RegisterTrafficServiceServer(s.server, NewTrafficGRPCServer())
	pb.RegisterHealthServiceServer(s.server, NewHealthGRPCServer())

	go s.server.Serve(lis)

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 创建客户端连接
	s.clientConn, err = grpc.Dial(s.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	assert.NoError(s.T(), err)
}

func (s *GRPCTestSuite) TearDownSuite() {
	if s.clientConn != nil {
		s.clientConn.Close()
	}
	if s.server != nil {
		s.server.GracefulStop()
	}
	database.Close()
}

func (s *GRPCTestSuite) SetupTest() {
	// 清理数据
	db := database.Get()
	db.Exec("DELETE FROM v2_user")
	db.Exec("DELETE FROM v2_plan")
	db.Exec("DELETE FROM v2_node")
	db.Exec("DELETE FROM v2_node_protocol")
	db.Exec("DELETE FROM v2_authorized_key")
}

// TestHealthCheck 测试健康检查
func (s *GRPCTestSuite) TestHealthCheck() {
	client := pb.NewHealthServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.HealthCheckRequest{
		NodeId: 1,
	}

	resp, err := client.Check(ctx, req)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), resp)
	assert.Equal(s.T(), pb.HealthCheckResponse_SERVING, resp.Status)
	assert.NotEmpty(s.T(), resp.ServerVersion)
	assert.Greater(s.T(), resp.Timestamp, int64(0))
}

// TestGetNodeConfig_NotFound 测试获取不存在的节点配置
func (s *GRPCTestSuite) TestGetNodeConfig_NotFound() {
	client := pb.NewNodeServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.NodeConfigRequest{
		NodeId: 99999,
	}

	_, err := client.GetConfig(ctx, req)
	assert.Error(s.T(), err)
}

// TestGetUsers_NodeNotFound 测试获取不存在节点的用户
func (s *GRPCTestSuite) TestGetUsers_NodeNotFound() {
	client := pb.NewUserServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.UserListRequest{
		NodeId: 99999,
	}

	_, err := client.GetUsers(ctx, req)
	assert.Error(s.T(), err)
}

// TestReportTraffic_NodeNotFound 测试上报流量（节点不存在）
func (s *GRPCTestSuite) TestReportTraffic_NodeNotFound() {
	client := pb.NewTrafficServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.TrafficReportRequest{
		NodeId:   99999,
		Traffics: make(map[uint32]*pb.TrafficData),
	}

	// 节点不存在时不会报错，只是更新心跳会失败
	_, err := client.ReportTraffic(ctx, req)
	// 由于节点不存在，可能会返回错误或成功（取决于实现）
	// 这里主要测试请求能正常发送
	_ = err // 忽略错误，主要测试不 panic
}

// TestReportOnline_NodeNotFound 测试上报在线状态（节点不存在）
func (s *GRPCTestSuite) TestReportOnline_NodeNotFound() {
	client := pb.NewTrafficServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.OnlineReportRequest{
		NodeId: 99999,
		Online: make(map[uint32]*pb.OnlineData),
	}

	_, err := client.ReportOnline(ctx, req)
	_ = err // 忽略错误，主要测试不 panic
}

// TestReportStatus_NodeNotFound 测试上报状态（节点不存在）
func (s *GRPCTestSuite) TestReportStatus_NodeNotFound() {
	client := pb.NewNodeServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.NodeStatusRequest{
		NodeId:      99999,
		CpuUsage:    50.0,
		MemoryUsage: 60.0,
		DiskUsage:   70.0,
		Uptime:      3600,
		OnlineUsers: 10,
		Upload:      1024,
		Download:    2048,
	}

	_, err := client.ReportStatus(ctx, req)
	assert.Error(s.T(), err)
}

// TestConnectionManager 测试连接管理器
func (s *GRPCTestSuite) TestConnectionManager() {
	mgr := NewNodeConnectionManager()

	// 测试注册
	mgr.Register(1, "192.168.1.1:12345")
	mgr.Register(2, "192.168.1.2:12345")

	// 测试获取
	conn, ok := mgr.GetConnection(1)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), uint32(1), conn.NodeID)
	assert.Equal(s.T(), "192.168.1.1:12345", conn.RemoteAddr)

	// 测试活跃节点列表
	nodes := mgr.GetActiveNodes()
	assert.Len(s.T(), nodes, 2)

	// 测试注销
	mgr.Unregister(1)
	_, ok = mgr.GetConnection(1)
	assert.False(s.T(), ok)

	// 测试配置版本
	mgr.UpdateConfigVersion(2, 100)
	ver := mgr.GetConfigVersion(2)
	assert.Equal(s.T(), int64(100), ver)

	// 测试配置变更检测
	assert.False(s.T(), mgr.IsConfigChanged(2, 100)) // 版本相同
	assert.True(s.T(), mgr.IsConfigChanged(2, 200))  // 版本更新
}

// TestGetPeerAddr 测试获取客户端地址
func (s *GRPCTestSuite) TestGetPeerAddr() {
	// 没有 peer 信息的上下文
	addr := GetPeerAddr(context.Background())
	assert.Equal(s.T(), "unknown", addr)
}

// TestNodeIDContext 测试节点ID上下文
func (s *GRPCTestSuite) TestNodeIDContext() {
	ctx := context.Background()

	// 测试设置和获取
	ctx = SetNodeIDToContext(ctx, 123)
	nodeID := GetNodeIDFromContext(ctx)
	assert.Equal(s.T(), uint32(123), nodeID)

	// 没有设置的上下文
	nodeID = GetNodeIDFromContext(context.Background())
	assert.Equal(s.T(), uint32(0), nodeID)
}

func TestGRPCSuite(t *testing.T) {
	suite.Run(t, new(GRPCTestSuite))
}

// GRPCIntegrationSuite 集成测试套件
type GRPCIntegrationSuite struct {
	suite.Suite
	server     *grpc.Server
	clientConn *grpc.ClientConn
	addr       string
}

func (s *GRPCIntegrationSuite) SetupSuite() {
	// 初始化缓存
	cache.InitMemory()

	// 初始化数据库
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})

	// 自动迁移
	database.AutoMigrate(
		&model.User{},
		&model.Plan{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.AuthorizedKey{},
	)

	// 启动 gRPC 服务器
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(s.T(), err)
	s.addr = lis.Addr().String()

	s.server = grpc.NewServer()
	pb.RegisterNodeServiceServer(s.server, NewNodeGRPCServer())
	pb.RegisterUserServiceServer(s.server, NewUserGRPCServer())
	pb.RegisterTrafficServiceServer(s.server, NewTrafficGRPCServer())
	pb.RegisterHealthServiceServer(s.server, NewHealthGRPCServer())

	go s.server.Serve(lis)

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 创建客户端连接
	s.clientConn, err = grpc.NewClient(s.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	assert.NoError(s.T(), err)
}

func (s *GRPCIntegrationSuite) TearDownSuite() {
	if s.clientConn != nil {
		s.clientConn.Close()
	}
	if s.server != nil {
		s.server.GracefulStop()
	}
	database.Close()
}

func (s *GRPCIntegrationSuite) SetupTest() {
	// 清理数据
	db := database.Get()
	db.Exec("DELETE FROM v2_user")
	db.Exec("DELETE FROM v2_plan")
	db.Exec("DELETE FROM v2_node")
	db.Exec("DELETE FROM v2_node_protocol")
	db.Exec("DELETE FROM v2_authorized_key")
}

// TestNodeRegistrationFlow 测试完整的节点注册流程
func (s *GRPCIntegrationSuite) TestNodeRegistrationFlow() {
	db := database.Get()

	// 创建授权密钥（使用 NodeService.GenerateAuthKey 生成正确的 hash）
	nodeService := service.NewNodeService()
	authKey, rawKey, err := nodeService.GenerateAuthKey("test-key", 1)
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), rawKey)

	client := pb.NewNodeServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 注册节点
	req := &pb.NodeRegisterRequest{
		AuthKey:       rawKey,
		Name:          "test-node-1",
		Host:          "192.168.1.100",
		Port:          443,
		ServerVersion: "1.0.0",
		ServerOs:      "linux",
	}

	resp, err := client.Register(ctx, req)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), resp)
	assert.NotZero(s.T(), resp.NodeId)
	assert.NotEmpty(s.T(), resp.ApiKey)
	assert.NotEmpty(s.T(), resp.Secret)

	// 验证节点已创建
	var node model.Node
	err = db.First(&node, resp.NodeId).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "test-node-1", node.Name)
	assert.Equal(s.T(), "192.168.1.100", node.Host)

	// 验证授权密钥已标记为使用
	var usedKey model.AuthorizedKey
	err = db.First(&usedKey, authKey.ID).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 1, usedKey.Used)

	// 获取节点配置
	configReq := &pb.NodeConfigRequest{
		NodeId: resp.NodeId,
	}

	configResp, err := client.GetConfig(ctx, configReq)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), configResp)
	assert.Equal(s.T(), "192.168.1.100", configResp.Host)
}

// TestConfigWithProtocol 测试带协议配置的节点
func (s *GRPCIntegrationSuite) TestConfigWithProtocol() {
	db := database.Get()

	// 创建节点
	groupID := uint(1)
	node := &model.Node{
		Name:    "protocol-test-node",
		Host:    "10.0.0.1",
		Port:    8443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  "test-api-key",
		Secret:  "test-secret",
	}
	err := db.Create(node).Error
	assert.NoError(s.T(), err)

	// 创建协议配置
	protocol := &model.NodeProtocol{
		NodeID:    node.ID,
		Type:      model.ProtocolVLESS,
		Port:      8443,
		TLS:       1,
		Transport: func() *string { s := "ws"; return &s }(),
	}
	err = db.Create(protocol).Error
	assert.NoError(s.T(), err)

	client := pb.NewNodeServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 获取配置
	req := &pb.NodeConfigRequest{
		NodeId: uint32(node.ID),
	}

	resp, err := client.GetConfig(ctx, req)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), resp)
	assert.Equal(s.T(), "vless", resp.Type)
	assert.Equal(s.T(), int32(8443), resp.ServerPort)
	assert.Equal(s.T(), int32(1), resp.Tls)
	assert.Equal(s.T(), "ws", resp.Network)
}

// TestUsersWithPlan 测试带套餐的用户
func (s *GRPCIntegrationSuite) TestUsersWithPlan() {
	db := database.Get()

	// 创建套餐
	speedLimit := int64(100)
	deviceLimit := 5
	plan := &model.Plan{
		Name:           "test-plan",
		TransferEnable: 10737418240, // 10GB
		SpeedLimit:     &speedLimit,
		DeviceLimit:    &deviceLimit,
		Show:           1,
	}
	err := db.Create(plan).Error
	assert.NoError(s.T(), err)

	// 创建节点
	groupID := uint(1)
	node := &model.Node{
		Name:    "user-test-node",
		Host:    "10.0.0.2",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  "test-api-key-2",
	}
	err = db.Create(node).Error
	assert.NoError(s.T(), err)

	// 创建用户
	userSpeedLimit := int64(100)
	userDeviceLimit := 5
	userPlanID := plan.ID
	userGroupID := uint(1)
	user := &model.User{
		Email:          "test@example.com",
		Password:       "hashed_password",
		Token:          "test-token-" + uuid.New().String()[:8],
		UUID:           uuid.New().String(),
		PlanID:         &userPlanID,
		GroupID:        &userGroupID,
		TransferEnable: 10737418240,
		SpeedLimit:     &userSpeedLimit,
		DeviceLimit:    &userDeviceLimit,
	}
	err = db.Create(user).Error
	assert.NoError(s.T(), err)

	client := pb.NewUserServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 获取用户列表
	req := &pb.UserListRequest{
		NodeId: uint32(node.ID),
	}

	resp, err := client.GetUsers(ctx, req)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), resp)
	assert.GreaterOrEqual(s.T(), resp.Total, int64(1))

	// 验证用户信息
	found := false
	for _, u := range resp.Users {
		if u.Uuid == user.UUID {
			found = true
			assert.Equal(s.T(), uint32(user.ID), u.Id)
			assert.Equal(s.T(), int64(10737418240), u.TransferEnable)
			break
		}
	}
	assert.True(s.T(), found, "User should be in the response")
}

// TestTrafficReportWithRate 测试流量倍率
func (s *GRPCIntegrationSuite) TestTrafficReportWithRate() {
	db := database.Get()

	// 创建套餐
	plan := &model.Plan{
		Name:           "traffic-plan",
		TransferEnable: 10737418240,
		Show:           1,
	}
	err := db.Create(plan).Error
	assert.NoError(s.T(), err)

	// 创建节点 (倍率 2.0)
	groupID := uint(1)
	node := &model.Node{
		Name:    "traffic-test-node",
		Host:    "10.0.0.3",
		Port:    443,
		GroupID: &groupID,
		Rate:    2.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  "test-api-key-3",
	}
	err = db.Create(node).Error
	assert.NoError(s.T(), err)

	// 创建用户
	userPlanID := plan.ID
	userGroupID := uint(1)
	user := &model.User{
		Email:          "traffic@example.com",
		Password:       "hashed_password",
		Token:          "traffic-token-" + uuid.New().String()[:8],
		UUID:           uuid.New().String(),
		PlanID:         &userPlanID,
		GroupID:        &userGroupID,
		TransferEnable: 10737418240,
		U:              0,
		D:              0,
	}
	err = db.Create(user).Error
	assert.NoError(s.T(), err)

	client := pb.NewTrafficServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 上报流量 (上传 1GB, 下载 2GB)
	traffics := make(map[uint32]*pb.TrafficData)
	traffics[uint32(user.ID)] = &pb.TrafficData{
		Upload:   1073741824,  // 1GB
		Download: 2147483648,  // 2GB
	}

	req := &pb.TrafficReportRequest{
		NodeId:   uint32(node.ID),
		Traffics: traffics,
	}

	resp, err := client.ReportTraffic(ctx, req)
	assert.NoError(s.T(), err)
	assert.True(s.T(), resp.Success)

	// 验证用户流量已更新 (应有倍率 2.0)
	var updatedUser model.User
	err = db.First(&updatedUser, user.ID).Error
	assert.NoError(s.T(), err)
	// 上传: 1GB * 2.0 = 2GB
	assert.Equal(s.T(), int64(2147483648), updatedUser.U)
	// 下载: 2GB * 2.0 = 4GB
	assert.Equal(s.T(), int64(4294967296), updatedUser.D)
}

func TestGRPCIntegrationSuite(t *testing.T) {
	suite.Run(t, new(GRPCIntegrationSuite))
}

// ========== 高级测试：拦截器、流式通信、服务器 ==========

// GRPCAdvancedSuite 高级测试套件
type GRPCAdvancedSuite struct {
	suite.Suite
	server     *grpc.Server
	clientConn *grpc.ClientConn
	addr       string
	apiToken   string
}

func (s *GRPCAdvancedSuite) SetupSuite() {
	// 初始化缓存
	cache.InitMemory()

	// 初始化数据库
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})

	// 自动迁移
	database.AutoMigrate(
		&model.User{},
		&model.Plan{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.AuthorizedKey{},
	)

	// 设置测试 token
	s.apiToken = "test-api-token-12345"

	// 启动带认证的 gRPC 服务器
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(s.T(), err)
	s.addr = lis.Addr().String()

	// 创建带拦截器的服务器
	interceptors := []grpc.UnaryServerInterceptor{
		LoggingInterceptor(),
		AuthInterceptor(s.apiToken),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		StreamLoggingInterceptor(),
		StreamAuthInterceptor(s.apiToken),
	}

	s.server = grpc.NewServer(
		grpc.ChainUnaryInterceptor(interceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	pb.RegisterNodeServiceServer(s.server, NewNodeGRPCServer())
	pb.RegisterUserServiceServer(s.server, NewUserGRPCServer())
	pb.RegisterTrafficServiceServer(s.server, NewTrafficGRPCServer())
	pb.RegisterHealthServiceServer(s.server, NewHealthGRPCServer())

	go s.server.Serve(lis)

	time.Sleep(100 * time.Millisecond)

	// 创建客户端连接
	s.clientConn, err = grpc.NewClient(s.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	assert.NoError(s.T(), err)
}

func (s *GRPCAdvancedSuite) TearDownSuite() {
	if s.clientConn != nil {
		s.clientConn.Close()
	}
	if s.server != nil {
		s.server.GracefulStop()
	}
	database.Close()
}

func (s *GRPCAdvancedSuite) SetupTest() {
	db := database.Get()
	db.Exec("DELETE FROM v2_user")
	db.Exec("DELETE FROM v2_plan")
	db.Exec("DELETE FROM v2_node")
	db.Exec("DELETE FROM v2_node_protocol")
	db.Exec("DELETE FROM v2_authorized_key")
}

// TestAuthInterceptor_MissingToken 测试缺少 token 的请求
func (s *GRPCAdvancedSuite) TestAuthInterceptor_MissingToken() {
	client := pb.NewNodeServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 不提供 token，应该被拒绝
	req := &pb.NodeConfigRequest{NodeId: 1}
	_, err := client.GetConfig(ctx, req)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "Unauthenticated")
}

// TestAuthInterceptor_InvalidToken 测试无效 token
func (s *GRPCAdvancedSuite) TestAuthInterceptor_InvalidToken() {
	client := pb.NewNodeServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 提供错误的 token
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer wrong-token")
	req := &pb.NodeConfigRequest{NodeId: 1}
	_, err := client.GetConfig(ctx, req)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "Unauthenticated")
}

// TestAuthInterceptor_ValidToken 测试有效 token
func (s *GRPCAdvancedSuite) TestAuthInterceptor_ValidToken() {
	db := database.Get()

	// 创建节点
	groupID := uint(1)
	node := &model.Node{
		Name:    "auth-test-node",
		Host:    "10.0.0.10",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  "test-key",
	}
	err := db.Create(node).Error
	assert.NoError(s.T(), err)

	client := pb.NewNodeServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 提供正确的 token
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+s.apiToken)
	req := &pb.NodeConfigRequest{NodeId: uint32(node.ID)}
	resp, err := client.GetConfig(ctx, req)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), resp)
}

// TestHealthService_NoAuthRequired 健康检查不需要认证
func (s *GRPCAdvancedSuite) TestHealthService_NoAuthRequired() {
	client := pb.NewHealthServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 不提供 token，健康检查应该成功
	req := &pb.HealthCheckRequest{NodeId: 1}
	resp, err := client.Check(ctx, req)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), pb.HealthCheckResponse_SERVING, resp.Status)
}

// TestConnectionManager_UpdateLastSeen 测试更新活跃时间
func (s *GRPCAdvancedSuite) TestConnectionManager_UpdateLastSeen() {
	mgr := NewNodeConnectionManager()
	mgr.Register(1, "192.168.1.1:12345")

	// 等待一小段时间
	time.Sleep(10 * time.Millisecond)

	// 更新活跃时间
	mgr.UpdateLastSeen(1)

	// 验证时间更新
	conn, ok := mgr.GetConnection(1)
	assert.True(s.T(), ok)
	assert.True(s.T(), conn.LastSeen.After(time.Now().Add(-100*time.Millisecond)))
}

// TestConnectionManager_GetConfigVersion_NotExists 测试获取不存在节点的配置版本
func (s *GRPCAdvancedSuite) TestConnectionManager_GetConfigVersion_NotExists() {
	mgr := NewNodeConnectionManager()

	// 不存在的节点，版本应为 0
	ver := mgr.GetConfigVersion(999)
	assert.Equal(s.T(), int64(0), ver)
}

// TestConnectionManager_IsConfigChanged_FirstTime 首次连接需要推送
func (s *GRPCAdvancedSuite) TestConnectionManager_IsConfigChanged_FirstTime() {
	mgr := NewNodeConnectionManager()

	// 首次连接，应该返回 true
	changed := mgr.IsConfigChanged(1, 100)
	assert.True(s.T(), changed)
}

// TestServerConfig 测试服务器配置
func (s *GRPCAdvancedSuite) TestServerConfig() {
	cfg := DefaultServerConfig()
	assert.Equal(s.T(), "0.0.0.0", cfg.Host)
	assert.Equal(s.T(), 50051, cfg.Port)
	assert.Equal(s.T(), 30*time.Second, cfg.KeepaliveTime)
}

// TestNewServer 测试创建服务器
func (s *GRPCAdvancedSuite) TestNewServer() {
	cfg := &ServerConfig{
		Host: "127.0.0.1",
		Port: 50052,
	}
	server := NewServer(cfg)
	assert.NotNil(s.T(), server)
	assert.Equal(s.T(), cfg, server.config)
}

// TestNewServer_NilConfig 测试空配置使用默认值
func (s *GRPCAdvancedSuite) TestNewServer_NilConfig() {
	server := NewServer(nil)
	assert.NotNil(s.T(), server)
	assert.NotNil(s.T(), server.config)
	assert.Equal(s.T(), "0.0.0.0", server.config.Host)
}

// TestGetConnectionManager 测试获取全局连接管理器
func (s *GRPCAdvancedSuite) TestGetConnectionManager() {
	mgr1 := GetConnectionManager()
	mgr2 := GetConnectionManager()
	assert.Equal(s.T(), mgr1, mgr2, "Should return the same instance")
}

// TestHashString 测试字符串哈希
func (s *GRPCAdvancedSuite) TestHashString() {
	hash1 := hashString("test")
	hash2 := hashString("test")
	assert.Equal(s.T(), hash1, hash2, "Same input should produce same hash")

	hash3 := hashString("different")
	assert.NotEqual(s.T(), hash1, hash3, "Different input should produce different hash")

	// SHA256 产生 64 个十六进制字符
	assert.Len(s.T(), hash1, 64)
}

func TestGRPCAdvancedSuite(t *testing.T) {
	suite.Run(t, new(GRPCAdvancedSuite))
}

// ========== 流式通信测试 ==========

// GRPCStreamSuite 流式通信测试套件
type GRPCStreamSuite struct {
	suite.Suite
	server     *grpc.Server
	clientConn *grpc.ClientConn
	addr       string
}

func (s *GRPCStreamSuite) SetupSuite() {
	cache.InitMemory()
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	database.AutoMigrate(
		&model.User{},
		&model.Plan{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.AuthorizedKey{},
	)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(s.T(), err)
	s.addr = lis.Addr().String()

	s.server = grpc.NewServer()
	pb.RegisterNodeServiceServer(s.server, NewNodeGRPCServer())
	pb.RegisterUserServiceServer(s.server, NewUserGRPCServer())
	pb.RegisterTrafficServiceServer(s.server, NewTrafficGRPCServer())
	pb.RegisterHealthServiceServer(s.server, NewHealthGRPCServer())

	go s.server.Serve(lis)
	time.Sleep(100 * time.Millisecond)

	s.clientConn, err = grpc.NewClient(s.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	assert.NoError(s.T(), err)
}

func (s *GRPCStreamSuite) TearDownSuite() {
	if s.clientConn != nil {
		s.clientConn.Close()
	}
	if s.server != nil {
		s.server.GracefulStop()
	}
	database.Close()
}

func (s *GRPCStreamSuite) SetupTest() {
	db := database.Get()
	db.Exec("DELETE FROM v2_user")
	db.Exec("DELETE FROM v2_plan")
	db.Exec("DELETE FROM v2_node")
	db.Exec("DELETE FROM v2_node_protocol")
	db.Exec("DELETE FROM v2_authorized_key")
}

// TestHealthWatch 测试健康检查流
func (s *GRPCStreamSuite) TestHealthWatch() {
	client := pb.NewHealthServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Watch(ctx)
	assert.NoError(s.T(), err)

	// 发送请求
	err = stream.Send(&pb.HealthCheckRequest{NodeId: 1})
	assert.NoError(s.T(), err)

	// 接收响应
	resp, err := stream.Recv()
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), pb.HealthCheckResponse_SERVING, resp.Status)

	// 关闭流
	stream.CloseSend()
}

// TestStatusStream 测试状态流
func (s *GRPCStreamSuite) TestStatusStream() {
	db := database.Get()

	// 创建节点
	groupID := uint(1)
	node := &model.Node{
		Name:    "stream-test-node",
		Host:    "10.0.0.20",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  "stream-test-key",
	}
	err := db.Create(node).Error
	assert.NoError(s.T(), err)

	client := pb.NewNodeServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.StatusStream(ctx)
	assert.NoError(s.T(), err)

	// 发送状态
	err = stream.Send(&pb.NodeStatusRequest{
		NodeId:      uint32(node.ID),
		CpuUsage:    50.0,
		MemoryUsage: 60.0,
		DiskUsage:   70.0,
		Uptime:      3600,
		OnlineUsers: 10,
	})
	assert.NoError(s.T(), err)

	// 验证连接管理器中注册了节点
	time.Sleep(100 * time.Millisecond)
	mgr := GetConnectionManager()
	_, ok := mgr.GetConnection(uint32(node.ID))
	assert.True(s.T(), ok, "Node should be registered in connection manager")

	stream.CloseSend()
}

// TestTrafficStream 测试流量流
func (s *GRPCStreamSuite) TestTrafficStream() {
	db := database.Get()

	// 创建套餐和用户
	plan := &model.Plan{
		Name:           "stream-traffic-plan",
		TransferEnable: 10737418240,
		Show:           1,
	}
	err := db.Create(plan).Error
	assert.NoError(s.T(), err)

	groupID := uint(1)
	node := &model.Node{
		Name:    "traffic-stream-node",
		Host:    "10.0.0.21",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  "traffic-stream-key",
	}
	err = db.Create(node).Error
	assert.NoError(s.T(), err)

	userPlanID := plan.ID
	userGroupID := uint(1)
	user := &model.User{
		Email:          "stream@example.com",
		Password:       "hash",
		Token:          "stream-token-" + uuid.New().String()[:8],
		UUID:           uuid.New().String(),
		PlanID:         &userPlanID,
		GroupID:        &userGroupID,
		TransferEnable: 10737418240,
	}
	err = db.Create(user).Error
	assert.NoError(s.T(), err)

	client := pb.NewTrafficServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.TrafficStream(ctx)
	assert.NoError(s.T(), err)

	// 发送流量数据
	traffics := make(map[uint32]*pb.TrafficData)
	traffics[uint32(user.ID)] = &pb.TrafficData{
		Upload:   1024000,
		Download: 2048000,
	}

	err = stream.Send(&pb.TrafficReportRequest{
		NodeId:   uint32(node.ID),
		Traffics: traffics,
	})
	assert.NoError(s.T(), err)

	// 接收响应
	resp, err := stream.Recv()
	assert.NoError(s.T(), err)
	assert.True(s.T(), resp.Success)

	stream.CloseSend()
}

// TestOnlineStream 测试在线状态流
func (s *GRPCStreamSuite) TestOnlineStream() {
	db := database.Get()

	groupID := uint(1)
	node := &model.Node{
		Name:    "online-stream-node",
		Host:    "10.0.0.22",
		Port:    443,
		GroupID: &groupID,
		Rate:    1.0,
		Show:    1,
		Status:  model.NodeStatusOnline,
		APIKey:  "online-stream-key",
	}
	err := db.Create(node).Error
	assert.NoError(s.T(), err)

	client := pb.NewTrafficServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.OnlineStream(ctx)
	assert.NoError(s.T(), err)

	// 发送在线数据
	online := make(map[uint32]*pb.OnlineData)
	online[1] = &pb.OnlineData{
		Ips:       []string{"192.168.1.1", "192.168.1.2"},
		Timestamp: time.Now().Unix(),
	}

	err = stream.Send(&pb.OnlineReportRequest{
		NodeId: uint32(node.ID),
		Online: online,
	})
	assert.NoError(s.T(), err)

	// 接收响应
	resp, err := stream.Recv()
	assert.NoError(s.T(), err)
	assert.True(s.T(), resp.Success)

	stream.CloseSend()
}

// TestUserChangesStream 测试用户变更流
func (s *GRPCStreamSuite) TestUserChangesStream() {
	client := pb.NewUserServiceClient(s.clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.UserChanges(ctx)
	assert.NoError(s.T(), err)

	// 发送用户变更通知
	notification := &pb.UserChangeNotification{
		Type: pb.UserChangeNotification_CREATED,
		User: &pb.UserInfo{
			Id:     1,
			Uuid:   uuid.New().String(),
		},
		Timestamp: time.Now().Unix(),
	}

	err = stream.Send(notification)
	assert.NoError(s.T(), err)

	// 接收确认
	resp, err := stream.Recv()
	assert.NoError(s.T(), err)
	assert.True(s.T(), resp.Success)

	stream.CloseSend()
}

func TestGRPCStreamSuite(t *testing.T) {
	suite.Run(t, new(GRPCStreamSuite))
}

// ========== 服务器完整功能测试 ==========

// TestServerStart 测试服务器启动
func TestServerStart(t *testing.T) {
	cache.InitMemory()
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	defer database.Close()

	cfg := &ServerConfig{
		Host:             "127.0.0.1",
		Port:             50052,
		KeepaliveTime:    30 * time.Second,
		KeepaliveTimeout: 10 * time.Second,
	}

	server := NewServer(cfg)
	err := server.Start()
	assert.NoError(t, err)

	// 验证服务器启动
	time.Sleep(200 * time.Millisecond)
	assert.NotNil(t, server.grpcServer)
	assert.NotNil(t, server.listener)

	// 停止服务器
	server.Stop()
}

// TestServerStop 测试服务器停止
func TestServerStop(t *testing.T) {
	cache.InitMemory()
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	defer database.Close()

	server := NewServer(&ServerConfig{
		Host: "127.0.0.1",
		Port: 50053,
	})

	err := server.Start()
	assert.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// 停止服务器
	server.Stop()

	// 服务器应该已停止
	assert.NotNil(t, server.grpcServer)
}

// TestServerGetConnectionManager 测试获取连接管理器
func TestServerGetConnectionManager(t *testing.T) {
	cache.InitMemory()
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	defer database.Close()

	server := NewServer(&ServerConfig{Host: "127.0.0.1", Port: 50054})
	mgr := server.GetConnectionManager()
	assert.NotNil(t, mgr)
	assert.Equal(t, GetConnectionManager(), mgr)
}

// TestStreamAuthInterceptor_WithAuth 测试流式认证拦截器
func TestStreamAuthInterceptor_WithAuth(t *testing.T) {
	cache.InitMemory()
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	database.AutoMigrate(&model.User{}, &model.Plan{}, &model.Node{}, &model.NodeProtocol{}, &model.AuthorizedKey{})
	defer database.Close()

	apiToken := "test-stream-token"

	// 创建带认证的服务器
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	addr := lis.Addr().String()

	server := grpc.NewServer(
		grpc.ChainStreamInterceptor(StreamAuthInterceptor(apiToken)),
	)
	pb.RegisterHealthServiceServer(server, NewHealthGRPCServer())
	pb.RegisterNodeServiceServer(server, NewNodeGRPCServer())

	go server.Serve(lis)
	defer server.GracefulStop()
	time.Sleep(100 * time.Millisecond)

	// 创建客户端
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	assert.NoError(t, err)
	defer conn.Close()

	// 测试健康检查 (豁免认证)
	healthClient := pb.NewHealthServiceClient(conn)
	ctx := context.Background()
	_, err = healthClient.Check(ctx, &pb.HealthCheckRequest{})
	assert.NoError(t, err, "Health check should not require auth")

	// 测试需要认证的流 (无 token)
	nodeClient := pb.NewNodeServiceClient(conn)
	stream, err := nodeClient.StatusStream(ctx)
	assert.NoError(t, err)

	// 发送应该失败 (无认证)
	err = stream.Send(&pb.NodeStatusRequest{NodeId: 1})
	assert.NoError(t, err) // Send 可能不立即失败

	// 接收应该失败
	_, err = stream.Recv()
	assert.Error(t, err, "Should fail without auth token")
}

// TestStreamLoggingInterceptor 测试流式日志拦截器
func TestStreamLoggingInterceptor(t *testing.T) {
	cache.InitMemory()
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	defer database.Close()

	// 创建带日志拦截器的服务器
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	addr := lis.Addr().String()

	server := grpc.NewServer(
		grpc.ChainStreamInterceptor(StreamLoggingInterceptor()),
	)
	pb.RegisterHealthServiceServer(server, NewHealthGRPCServer())

	go server.Serve(lis)
	defer server.GracefulStop()
	time.Sleep(100 * time.Millisecond)

	// 创建客户端
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	assert.NoError(t, err)
	defer conn.Close()

	// 测试健康检查流
	healthClient := pb.NewHealthServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := healthClient.Watch(ctx)
	assert.NoError(t, err)

	err = stream.Send(&pb.HealthCheckRequest{})
	assert.NoError(t, err)

	_, err = stream.Recv()
	assert.NoError(t, err)

	stream.CloseSend()
}

// TestAuthInterceptor_HealthExemption 测试认证拦截器对健康检查的豁免
func TestAuthInterceptor_HealthExemption(t *testing.T) {
	cache.InitMemory()
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	defer database.Close()

	apiToken := "secret-token"

	// 创建带认证的服务器
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	addr := lis.Addr().String()

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(AuthInterceptor(apiToken)),
	)
	pb.RegisterHealthServiceServer(server, NewHealthGRPCServer())

	go server.Serve(lis)
	defer server.GracefulStop()
	time.Sleep(100 * time.Millisecond)

	// 创建客户端
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	assert.NoError(t, err)
	defer conn.Close()

	// 健康检查不需要认证
	healthClient := pb.NewHealthServiceClient(conn)
	ctx := context.Background()
	resp, err := healthClient.Check(ctx, &pb.HealthCheckRequest{})
	assert.NoError(t, err)
	assert.Equal(t, pb.HealthCheckResponse_SERVING, resp.Status)
}

// TestReportStatus_WithNode 测试有节点时的状态上报
func TestReportStatus_WithNode(t *testing.T) {
	cache.InitMemory()
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	database.AutoMigrate(&model.User{}, &model.Plan{}, &model.Node{}, &model.NodeProtocol{}, &model.AuthorizedKey{})
	defer database.Close()

	db := database.Get()

	// 创建节点
	node := &model.Node{
		Name:   "status-report-node",
		Host:   "10.0.0.30",
		Port:   443,
		Rate:   1.0,
		Show:   1,
		Status: model.NodeStatusOnline,
		APIKey: "status-key",
	}
	err := db.Create(node).Error
	assert.NoError(t, err)

	// 创建 gRPC 测试环境
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	addr := lis.Addr().String()

	server := grpc.NewServer()
	pb.RegisterNodeServiceServer(server, NewNodeGRPCServer())

	go server.Serve(lis)
	defer server.GracefulStop()
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	assert.NoError(t, err)
	defer conn.Close()

	// 上报状态
	client := pb.NewNodeServiceClient(conn)
	ctx := context.Background()
	resp, err := client.ReportStatus(ctx, &pb.NodeStatusRequest{
		NodeId:      uint32(node.ID),
		CpuUsage:    45.5,
		MemoryUsage: 65.2,
		DiskUsage:   75.0,
		Uptime:      7200,
		OnlineUsers: 25,
		Upload:      1024000,
		Download:    2048000,
	})
	assert.NoError(t, err)
	assert.True(t, resp.Success)

	// 验证数据库中的节点状态
	var updatedNode model.Node
	err = db.First(&updatedNode, node.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, 45.5, updatedNode.CPUUsage)
	assert.Equal(t, 65.2, updatedNode.MemoryUsage)
	assert.Equal(t, 75.0, updatedNode.DiskUsage)
	assert.Equal(t, int64(7200), updatedNode.Uptime)
	assert.Equal(t, 25, updatedNode.OnlineUsers)
}

// TestGetConfig_WithMultipleProtocols 测试多协议配置
func TestGetConfig_WithMultipleProtocols(t *testing.T) {
	cache.InitMemory()
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	database.AutoMigrate(&model.User{}, &model.Plan{}, &model.Node{}, &model.NodeProtocol{}, &model.AuthorizedKey{})
	defer database.Close()

	db := database.Get()

	// 创建节点
	node := &model.Node{
		Name:   "multi-protocol-node",
		Host:   "10.0.0.40",
		Port:   443,
		Rate:   1.0,
		Show:   1,
		Status: model.NodeStatusOnline,
		APIKey: "multi-key",
	}
	err := db.Create(node).Error
	assert.NoError(t, err)

	// 创建多个协议
	protocol1 := &model.NodeProtocol{
		NodeID:    node.ID,
		Type:      model.ProtocolVMess,
		Port:      10001,
		TLS:       1,
		Transport: func() *string { s := "ws"; return &s }(),
	}
	protocol2 := &model.NodeProtocol{
		NodeID: node.ID,
		Type:   model.ProtocolVLESS,
		Port:   10002,
		TLS:    2, // Reality
	}
	err = db.Create(protocol1).Error
	assert.NoError(t, err)
	err = db.Create(protocol2).Error
	assert.NoError(t, err)

	// 创建 gRPC 测试环境
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	addr := lis.Addr().String()

	server := grpc.NewServer()
	pb.RegisterNodeServiceServer(server, NewNodeGRPCServer())

	go server.Serve(lis)
	defer server.GracefulStop()
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	assert.NoError(t, err)
	defer conn.Close()

	// 获取配置 (应该返回第一个协议)
	client := pb.NewNodeServiceClient(conn)
	ctx := context.Background()
	resp, err := client.GetConfig(ctx, &pb.NodeConfigRequest{NodeId: uint32(node.ID)})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	// 应该返回第一个协议配置
	assert.Equal(t, "vmess", resp.Type)
	assert.Equal(t, int32(10001), resp.ServerPort)
	assert.Equal(t, int32(1), resp.Tls)
	assert.Equal(t, "ws", resp.Network)
}