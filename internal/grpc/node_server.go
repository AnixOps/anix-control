package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
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

	return &pb.NodeRegisterResponse{
		NodeId:  uint32(resp.NodeID),
		ApiKey:  resp.APIKey,
		Secret:  resp.Secret,
		Message: resp.Message,
	}, nil
}

// GetConfig 获取节点配置
func (s *NodeGRPCServer) GetConfig(ctx context.Context, req *pb.NodeConfigRequest) (*pb.NodeConfigResponse, error) {
	node, err := s.nodeService.GetNode(uint(req.NodeId))
	if err != nil {
		return nil, status.Error(codes.NotFound, "node not found")
	}

	// 获取协议配置
	protocols, _ := s.nodeService.GetProtocols(uint(req.NodeId))

	// 构建配置响应
	resp := &pb.NodeConfigResponse{
		Host:        node.Host,
		ServerPort:  int32(node.Port),
		ServerName:  node.Host,
		SendThrough: "0.0.0.0",
		BaseConfig: &pb.BaseConfig{
			PushInterval: 60,
			PullInterval: 60,
		},
	}

	// 如果有协议配置
	if len(protocols) > 0 {
		protocol := protocols[0]
		resp.NodeType = string(protocol.Type)
		resp.Type = string(protocol.Type)
		resp.ServerPort = int32(protocol.Port)
		resp.Tls = int32(protocol.TLS)

		if protocol.Transport != nil {
			resp.Network = *protocol.Transport
		} else {
			resp.Network = "tcp"
		}
	} else {
		// 默认配置
		resp.NodeType = "vless"
		resp.Type = "vless"
		resp.Network = "tcp"
	}

	return resp, nil
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
			log.Printf("Node %d disconnected from gRPC stream", nodeID)
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
			log.Printf("Node %d connected via gRPC stream from %s", nodeID, clientAddr)
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
			log.Printf("Failed to update node status: %v", err)
			continue
		}

		// 检查是否有配置变更需要推送
		configResp, err := s.checkConfigChanges(req.NodeId)
		if err == nil && configResp != nil {
			if err := stream.Send(configResp); err != nil {
				log.Printf("Failed to send config update: %v", err)
			}
		}
	}
}

// checkConfigChanges 检查配置变更
func (s *NodeGRPCServer) checkConfigChanges(nodeID uint32) (*pb.NodeConfigResponse, error) {
	// TODO: 实现配置变更检测
	// 可以通过缓存或数据库版本号来判断是否需要推送
	return nil, nil
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

	// 转换为 proto 格式
	userInfos := make([]*pb.UserInfo, 0, len(users))
	for _, user := range users {
		speedLimit := user.GetSpeedLimit()
		deviceLimit := user.GetDeviceLimit()

		userInfos = append(userInfos, &pb.UserInfo{
			Id:             uint32(user.ID),
			Uuid:           user.UUID,
			SpeedLimit:     speedLimit,
			DeviceLimit:    int32(deviceLimit),
			TransferEnable: user.TransferEnable,
			UsedUpload:     user.U,
			UsedDownload:   user.D,
		})
	}

	return &pb.UserListResponse{
		Users:      userInfos,
		Total:      int64(len(userInfos)),
		UpdatedAt:  time.Now().Unix(),
	}, nil
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
		log.Printf("User change: type=%v, user_id=%v", notification.Type, notification.User.GetId())
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
	s.nodeService.UpdateLastCheckAt(uint(req.NodeId))

	// 转换流量数据
	traffics := make(map[uint][2]int64)
	for userID, data := range req.Traffics {
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
	s.nodeService.UpdateLastCheckAt(uint(req.NodeId))

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
			log.Printf("Failed to process traffic: %v", err)
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
			log.Printf("Failed to process online status: %v", err)
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
		ServerVersion: "2.0.0",
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
			ServerVersion: "2.0.0",
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