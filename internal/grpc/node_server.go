package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"time"

	pb "github.com/anixops/v2board/api/grpc/v2boardpb"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NodeGRPCServer 节点 gRPC 服务实现
type NodeGRPCServer struct {
	pb.UnimplementedNodeServiceServer
	nodeService *service.NodeService
	userService *service.UserService
}

// NewNodeGRPCServer 创建节点 gRPC 服务
func NewNodeGRPCServer() *NodeGRPCServer {
	return &NodeGRPCServer{
		nodeService: service.NewNodeService(),
		userService: service.NewUserService(),
	}
}

// Register 节点注册
func (s *NodeGRPCServer) Register(ctx context.Context, req *pb.NodeRegisterRequest) (*pb.NodeRegisterResponse, error) {
	// 构建注册请求
	registerReq := &model.NodeRegisterRequest{
		AuthKey:       req.AuthKey,
		Name:          req.Name,
		Host:          req.Host,
		Port:          int(req.Port),
		ServerVersion: req.ServerVersion,
		ServerOS:      req.ServerOs,
	}

	// 获取客户端 IP
	clientIP := "unknown"
	if p, ok := peerFromContext(ctx); ok {
		clientIP = p
	}

	// 执行注册
	resp, err := s.nodeService.RegisterNode(registerReq, clientIP)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	nodeID, err := uintToUint32("node id", resp.NodeID)
	if err != nil {
		return nil, status.Error(codes.OutOfRange, err.Error())
	}

	return &pb.NodeRegisterResponse{
		NodeId:  nodeID,
		ApiKey:  resp.APIKey,
		Secret:  resp.Secret,
		Message: resp.Message,
	}, nil
}

// GetConfig 获取节点配置
func (s *NodeGRPCServer) GetConfig(ctx context.Context, req *pb.NodeConfigRequest) (*pb.NodeConfigResponse, error) {
	if err := s.nodeService.UpdateLastCheckAt(uint(req.NodeId)); err != nil {
		slog.Warn("failed to update node heartbeat before config fetch", "component", "grpc", "method", "GetConfig", "node_id", req.NodeId, "error", err)
	}

	node, err := s.nodeService.GetNode(uint(req.NodeId))
	if err != nil {
		return nil, status.Error(codes.NotFound, "node not found")
	}

	// 获取协议配置
	protocols, _ := s.nodeService.GetProtocols(uint(req.NodeId))

	if len(protocols) > 0 {
		// 用共享构建器填充完整协议配置 (cipher / server_key / flow / tls_settings 等)
		resp, err := fillNodeConfigResponse(node, &protocols[0])
		if err != nil {
			return nil, status.Error(codes.OutOfRange, err.Error())
		}
		return resp, nil
	}

	serverPort, err := intToInt32("node port", node.Port)
	if err != nil {
		return nil, status.Error(codes.OutOfRange, err.Error())
	}

	// 无协议配置时的默认响应
	return &pb.NodeConfigResponse{
		NodeType:    "vless",
		Type:        "vless",
		Host:        node.Host,
		ServerPort:  serverPort,
		ServerName:  node.Host,
		Network:     "tcp",
		SendThrough: "0.0.0.0",
		BaseConfig: &pb.BaseConfig{
			PushInterval: 60,
			PullInterval: 60,
		},
	}, nil
}

// ReportStatus 上报节点状态
func (s *NodeGRPCServer) ReportStatus(ctx context.Context, req *pb.NodeStatusRequest) (*pb.StatusResponse, error) {
	heartbeatReq := &model.NodeHeartbeatRequest{
		CPUUsage:    req.CpuUsage,
		MemoryUsage: req.MemoryUsage,
		DiskUsage:   req.DiskUsage,
		Uptime:      req.Uptime,
		OnlineUsers: int(req.OnlineUsers),
		Upload:      req.Upload,
		Download:    req.Download,
	}

	if err := s.nodeService.Heartbeat(uint(req.NodeId), heartbeatReq); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.StatusResponse{
		Success: true,
		Message: "status updated",
		Code:    200,
	}, nil
}

// StatusStream 双向流：节点状态实时通信
func (s *NodeGRPCServer) StatusStream(stream pb.NodeService_StatusStreamServer) error {
	var nodeID uint32
	mgr := GetConnectionManager()

	// 获取客户端地址
	clientAddr := GetPeerAddr(stream.Context())

	defer func() {
		if nodeID > 0 {
			mgr.Unregister(nodeID)
			slog.Info("node gRPC disconnected", "component", "grpc", "method", "StatusStream", "node_id", nodeID)
		}
	}()

	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}

		// 首次连接时记录节点ID
		if nodeID == 0 {
			nodeID = req.NodeId
			mgr.Register(nodeID, clientAddr)
			slog.Info("node gRPC connected", "component", "grpc", "method", "StatusStream", "node_id", nodeID, "addr", clientAddr)
		}

		// 更新活跃时间
		mgr.UpdateLastSeen(nodeID)

		// 处理状态上报
		heartbeatReq := &model.NodeHeartbeatRequest{
			CPUUsage:    req.CpuUsage,
			MemoryUsage: req.MemoryUsage,
			DiskUsage:   req.DiskUsage,
			Uptime:      req.Uptime,
			OnlineUsers: int(req.OnlineUsers),
			Upload:      req.Upload,
			Download:    req.Download,
		}

		if err := s.nodeService.Heartbeat(uint(req.NodeId), heartbeatReq); err != nil {
			slog.Warn("failed to update node status", "component", "grpc", "method", "StatusStream", "node_id", req.NodeId, "error", err)
			continue
		}

		// 检查是否有配置变更需要推送
		configResp, err := s.checkConfigChanges(req.NodeId)
		if err == nil && configResp != nil {
			if err := stream.Send(configResp); err != nil {
				slog.Warn("failed to send config update", "component", "grpc", "method", "StatusStream", "node_id", req.NodeId, "error", err)
			}
		}
	}
}

// checkConfigChanges 检查配置变更：以节点 UpdatedAt 作为配置版本，
// 与连接管理器记录的已推送版本比对，发现更新则构建配置并推进版本号。
func (s *NodeGRPCServer) checkConfigChanges(nodeID uint32) (*pb.NodeConfigResponse, error) {
	node, err := s.nodeService.GetNode(uint(nodeID))
	if err != nil {
		return nil, err
	}

	currentVer := node.UpdatedAt.Unix()
	mgr := GetConnectionManager()
	if !mgr.IsConfigChanged(nodeID, currentVer) {
		return nil, nil
	}

	resp, err := s.buildConfigResponse(node)
	if err != nil {
		return nil, err
	}
	// 记录已推送版本，避免同一配置被重复推送。
	mgr.SetNodeConfigVersion(nodeID, currentVer)
	return resp, nil
}

// buildConfigResponse 根据节点及其协议构建下发配置。
func (s *NodeGRPCServer) buildConfigResponse(node *model.Node) (*pb.NodeConfigResponse, error) {
	protocols, _ := s.nodeService.GetProtocols(node.ID)
	if len(protocols) > 0 {
		return fillNodeConfigResponse(node, &protocols[0])
	}

	serverPort, err := intToInt32("node port", node.Port)
	if err != nil {
		return nil, err
	}

	return &pb.NodeConfigResponse{
		NodeType:    "vless",
		Type:        "vless",
		Host:        node.Host,
		ServerPort:  serverPort,
		ServerName:  node.Host,
		Network:     "tcp",
		SendThrough: "0.0.0.0",
		BaseConfig: &pb.BaseConfig{
			PushInterval: 60,
			PullInterval: 60,
		},
	}, nil
}

// peerFromContext 从上下文获取客户端地址 (已弃用，使用 GetPeerAddr)
func peerFromContext(ctx context.Context) (string, bool) {
	addr := GetPeerAddr(ctx)
	return addr, addr != "unknown"
}

// UserGRPCServer 用户 gRPC 服务实现
type UserGRPCServer struct {
	pb.UnimplementedUserServiceServer
	userService *service.UserService
	nodeService *service.NodeService
}

// NewUserGRPCServer 创建用户 gRPC 服务
func NewUserGRPCServer() *UserGRPCServer {
	return &UserGRPCServer{
		userService: service.NewUserService(),
		nodeService: service.NewNodeService(),
	}
}

// GetUsers 获取用户列表
func (s *UserGRPCServer) GetUsers(ctx context.Context, req *pb.UserListRequest) (*pb.UserListResponse, error) {
	_ = s.nodeService.UpdateLastCheckAt(uint(req.NodeId))

	// 获取节点信息以确定分组
	node, err := s.nodeService.GetNode(uint(req.NodeId))
	if err != nil {
		return nil, status.Error(codes.NotFound, "node not found")
	}

	// 获取活跃用户
	users, err := s.userService.GetActiveUsersForNode(node.GroupID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get users")
	}

	wireGuardExtras, err := s.buildWireGuardUserExtras(uint(req.NodeId), users)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to build wireguard users")
	}

	// 转换为 proto 格式
	userInfos := make([]*pb.UserInfo, 0, len(users))
	for _, user := range users {
		info, err := userInfoFromModel(user)
		if err != nil {
			return nil, status.Error(codes.OutOfRange, err.Error())
		}
		if extra, ok := wireGuardExtras[user.ID]; ok {
			info.Extra = extra
		}
		userInfos = append(userInfos, info)
	}

	return &pb.UserListResponse{
		Users:     userInfos,
		Total:     int64(len(userInfos)),
		UpdatedAt: time.Now().Unix(),
	}, nil
}

func (s *UserGRPCServer) buildWireGuardUserExtras(nodeID uint, users []*model.User) (map[uint]map[string]string, error) {
	if nodeID == 0 || len(users) == 0 {
		return nil, nil
	}
	protocols, err := s.nodeService.GetProtocols(nodeID)
	if err != nil || len(protocols) == 0 || protocols[0].Type != model.ProtocolWireGuard {
		return nil, err
	}
	return service.NewSubscriptionService().BuildWireGuardRuntimeUserExtras(&protocols[0], users)
}

// UserChanges 双向流：用户变更实时推送
func (s *UserGRPCServer) UserChanges(stream pb.UserService_UserChangesServer) error {
	for {
		notification, err := stream.Recv()
		if err != nil {
			return err
		}

		// 确认接收
		if err := stream.Send(&pb.StatusResponse{
			Success: true,
			Message: "notification received",
			Code:    200,
		}); err != nil {
			return err
		}

		// 记录用户变更
		slog.Info("user change received", "component", "grpc", "method", "UserChanges", "type", notification.Type, "user_id", notification.User.GetId())
	}
}

// TrafficGRPCServer 流量 gRPC 服务实现
type TrafficGRPCServer struct {
	pb.UnimplementedTrafficServiceServer
	userService   *service.UserService
	serverService *service.ServerService
	nodeService   *service.NodeService
}

// NewTrafficGRPCServer 创建流量 gRPC 服务
func NewTrafficGRPCServer() *TrafficGRPCServer {
	return &TrafficGRPCServer{
		userService:   service.NewUserService(),
		serverService: service.NewServerService(),
		nodeService:   service.NewNodeService(),
	}
}

// ReportTraffic 批量上报流量
func (s *TrafficGRPCServer) ReportTraffic(ctx context.Context, req *pb.TrafficReportRequest) (*pb.TrafficReportResponse, error) {
	// 更新节点心跳
	if err := s.nodeService.UpdateLastCheckAt(uint(req.NodeId)); err != nil {
		slog.Warn("failed to update node heartbeat before traffic report", "component", "grpc", "method", "ReportTraffic", "node_id", req.NodeId, "error", err)
	}

	// 转换流量数据
	traffics := make(map[uint][2]int64)
	for userID, data := range req.Traffics {
		if data == nil {
			continue
		}
		if err := service.ValidateTrafficDelta(data.Upload, data.Download); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		traffics[uint(userID)] = [2]int64{data.Upload, data.Download}
	}

	// 获取流量倍率
	rate := 1.0
	node, err := s.nodeService.GetNode(uint(req.NodeId))
	if err == nil {
		rate = node.Rate
	}

	// 按倍率计算实际流量
	userTraffics := make(map[uint][2]int64)
	for userID, traffic := range traffics {
		userTraffics[userID] = [2]int64{
			int64(float64(traffic[0]) * rate),
			int64(float64(traffic[1]) * rate),
		}
	}

	// 记录流量日志 (原始字节 + 倍率), 用于今日流量等基于时间的统计, 与 REST 上报路径保持一致
	if err := s.serverService.BatchRecordTrafficLog(model.ServerType("node"), uint(req.NodeId), traffics, rate); err != nil {
		slog.Warn("failed to record traffic log", "component", "grpc", "method", "ReportTraffic", "node_id", req.NodeId, "error", err)
	}

	var totalUpload, totalDownload int64
	for _, traffic := range userTraffics {
		totalUpload += traffic[0]
		totalDownload += traffic[1]
	}
	// 同步回写节点总流量，保证面板节点统计和流量监控读取的是同一口径。
	if err := s.nodeService.AccumulateTrafficOnly(uint(req.NodeId), totalUpload, totalDownload); err != nil {
		slog.Warn("failed to accumulate node traffic", "component", "grpc", "method", "ReportTraffic", "node_id", req.NodeId, "error", err)
	}

	// 统计表写入：日统计按当前整日分桶，月统计按当前月第一天分桶。
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).Unix()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Unix()
	if err := s.serverService.RecordServerStat(model.ServerType("node"), uint(req.NodeId), totalUpload, totalDownload, "d", dayStart); err != nil {
		slog.Warn("failed to record daily server stat", "component", "grpc", "method", "ReportTraffic", "node_id", req.NodeId, "error", err)
	}
	if err := s.serverService.RecordServerStat(model.ServerType("node"), uint(req.NodeId), totalUpload, totalDownload, "m", monthStart); err != nil {
		slog.Warn("failed to record monthly server stat", "component", "grpc", "method", "ReportTraffic", "node_id", req.NodeId, "error", err)
	}

	// 批量更新用户流量
	if err := s.userService.BatchUpdateTraffic(userTraffics); err != nil {
		return nil, status.Error(codes.Internal, "failed to update traffic")
	}

	return &pb.TrafficReportResponse{
		Success: true,
		Message: "traffic updated",
	}, nil
}

// ReportOnline 上报在线状态
func (s *TrafficGRPCServer) ReportOnline(ctx context.Context, req *pb.OnlineReportRequest) (*pb.StatusResponse, error) {
	// 更新节点心跳
	if err := s.nodeService.UpdateLastCheckAt(uint(req.NodeId)); err != nil {
		slog.Warn("failed to update node heartbeat before online report", "component", "grpc", "method", "ReportOnline", "node_id", req.NodeId, "error", err)
	}

	// 转换在线数据
	userIPs := make(map[uint][]string)
	for userID, data := range req.Online {
		userIPs[uint(userID)] = data.Ips
	}

	// 更新在线状态
	if err := s.serverService.UpdateOnlineStatus("", uint(req.NodeId), userIPs); err != nil {
		return nil, status.Error(codes.Internal, "failed to update online status")
	}

	return &pb.StatusResponse{
		Success: true,
		Message: "online status updated",
		Code:    200,
	}, nil
}

// TrafficStream 双向流：实时流量上报
func (s *TrafficGRPCServer) TrafficStream(stream pb.TrafficService_TrafficStreamServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}

		// 处理流量上报
		resp, err := s.ReportTraffic(stream.Context(), req)
		if err != nil {
			slog.Warn("failed to process traffic", "component", "grpc", "method", "TrafficStream", "node_id", req.NodeId, "error", err)
			continue
		}

		// 发送响应
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

// OnlineStream 双向流：实时在线状态
func (s *TrafficGRPCServer) OnlineStream(stream pb.TrafficService_OnlineStreamServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}

		// 处理在线状态上报
		resp, err := s.ReportOnline(stream.Context(), req)
		if err != nil {
			slog.Warn("failed to process online status", "component", "grpc", "method", "OnlineStream", "node_id", req.NodeId, "error", err)
			continue
		}

		// 发送响应
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

// HealthGRPCServer 健康检查 gRPC 服务实现
type HealthGRPCServer struct {
	pb.UnimplementedHealthServiceServer
}

// NewHealthGRPCServer 创建健康检查 gRPC 服务
func NewHealthGRPCServer() *HealthGRPCServer {
	return &HealthGRPCServer{}
}

// Check 简单健康检查
func (s *HealthGRPCServer) Check(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Status:        pb.HealthCheckResponse_SERVING,
		ServerVersion: "2.0.2-test.1",
		Timestamp:     time.Now().Unix(),
	}, nil
}

// Watch 双向流：持续健康检查
func (s *HealthGRPCServer) Watch(stream pb.HealthService_WatchServer) error {
	for {
		_, err := stream.Recv()
		if err != nil {
			return err
		}

		resp := &pb.HealthCheckResponse{
			Status:        pb.HealthCheckResponse_SERVING,
			ServerVersion: "2.0.2-test.1",
			Timestamp:     time.Now().Unix(),
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

// 辅助函数
func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
