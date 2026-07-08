package grpc

import (
	"context"
	"log/slog"
	"time"

	pb "github.com/anixops/v2board/api/grpc/v2boardpb"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ConfigSyncGRPCServer 配置同步 gRPC 服务实现
type ConfigSyncGRPCServer struct {
	pb.UnimplementedConfigSyncServiceServer
	nodeService *service.NodeService
	userService *service.UserService
}

// NewConfigSyncGRPCServer 创建配置同步 gRPC 服务
func NewConfigSyncGRPCServer() *ConfigSyncGRPCServer {
	return &ConfigSyncGRPCServer{
		nodeService: service.NewNodeService(),
		userService: service.NewUserService(),
	}
}

// SyncConfig 请求配置同步 - 根据 lastSyncTime 判断是否有变更
func (s *ConfigSyncGRPCServer) SyncConfig(ctx context.Context, req *pb.ConfigSyncRequest) (*pb.ConfigSyncResponse, error) {
	node, err := s.nodeService.GetNode(uint(req.NodeId))
	if err != nil {
		return nil, status.Error(codes.NotFound, "node not found")
	}

	syncTime := time.Now().Unix()
	hasChanges := node.UpdatedAt.Unix() > req.LastSyncTime

	resp := &pb.ConfigSyncResponse{
		HasChanges: hasChanges,
		SyncTime:   syncTime,
	}

	if hasChanges {
		config, err := s.buildNodeConfigResponse(node)
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to build node config")
		}
		resp.Config = config

		users, err := s.buildUserList(node.GroupID)
		if err != nil {
			slog.Warn("failed to get users for config sync", "component", "grpc", "method", "SyncConfig", "node_id", req.NodeId, "error", err)
		} else {
			resp.Users = users
		}
	}

	return resp, nil
}

// FullSync 全量同步 - 节点启动时获取完整配置
func (s *ConfigSyncGRPCServer) FullSync(ctx context.Context, req *pb.ConfigSyncRequest) (*pb.ConfigSyncResponse, error) {
	node, err := s.nodeService.GetNode(uint(req.NodeId))
	if err != nil {
		return nil, status.Error(codes.NotFound, "node not found")
	}

	syncTime := time.Now().Unix()

	// 构建完整节点配置
	config, err := s.buildNodeConfigResponse(node)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to build node config")
	}

	// 获取完整用户列表
	users, err := s.buildUserList(node.GroupID)
	if err != nil {
		slog.Warn("failed to get users for full sync", "component", "grpc", "method", "FullSync", "node_id", req.NodeId, "error", err)
		users = []*pb.UserInfo{}
	}

	return &pb.ConfigSyncResponse{
		HasChanges: true,
		Config:     config,
		Users:      users,
		SyncTime:   syncTime,
	}, nil
}

// ConfigChanges 双向流：配置变更实时推送
func (s *ConfigSyncGRPCServer) ConfigChanges(stream pb.ConfigSyncService_ConfigChangesServer) error {
	mgr := GetConnectionManager()

	for {
		notification, err := stream.Recv()
		if err != nil {
			return err
		}

		// 注册节点连接（首次收到通知时）
		if notification.NodeId > 0 {
			nodeID := notification.NodeId
			mgr.Register(nodeID, GetPeerAddr(stream.Context()))
			mgr.SetNodeConfigVersion(nodeID, time.Now().Unix())
			slog.Info("node registered via config sync stream", "component", "grpc", "method", "ConfigChanges", "node_id", nodeID)
		}

		// 确认接收
		if err := stream.Send(&pb.StatusResponse{
			Success: true,
			Message: "config change notification received",
			Code:    200,
		}); err != nil {
			return err
		}

		// 记录变更日志
		changeType := notification.Type.String()
		slog.Info("config change received", "component", "grpc", "method", "ConfigChanges", "type", changeType, "node_id", notification.NodeId, "timestamp", notification.Timestamp)
	}
}

// buildNodeConfigResponse 构建节点配置响应
func (s *ConfigSyncGRPCServer) buildNodeConfigResponse(node *model.Node) (*pb.NodeConfigResponse, error) {
	protocols, _ := s.nodeService.GetProtocols(node.ID)

	if len(protocols) > 0 {
		// 用共享构建器填充完整协议配置, 与 HTTP/订阅端一致
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

// buildUserList 构建用户列表
func (s *ConfigSyncGRPCServer) buildUserList(groupID *uint) ([]*pb.UserInfo, error) {
	users, err := s.userService.GetActiveUsersForNode(groupID)
	if err != nil {
		return nil, err
	}

	userInfos := make([]*pb.UserInfo, 0, len(users))
	for _, user := range users {
		info, err := userInfoFromModel(user)
		if err != nil {
			return nil, err
		}
		userInfos = append(userInfos, info)
	}

	return userInfos, nil
}

// notifyConfigChange 通知指定节点配置变更（供外部调用）
func (s *ConfigSyncGRPCServer) notifyConfigChange(nodeID uint32, changeType pb.ConfigChangeNotification_ChangeType) error {
	mgr := GetConnectionManager()

	// 通过连接管理器的 channel 通知节点
	mgr.NotifyConfigChange(nodeID)

	slog.Info("node notified of config change", "component", "grpc", "method", "ConfigChanges", "node_id", nodeID, "change_type", changeType.String())
	return nil
}
