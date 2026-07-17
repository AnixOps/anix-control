package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/v4/api/grpc/agent/v1"
	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// ServerConfig gRPC 服务器配置
type ServerConfig struct {
	// 监听地址
	Host string
	// 监听端口
	Port int
	// API Token (可选，用于认证)
	APIToken string
	// JWT Secret (可选，用于 JWT 认证)
	JWTSecret string
	// TLS 证书/私钥路径 (可选，都为空时使用明文)
	TLSCertFile string
	TLSKeyFile  string
	// Keepalive 时间
	KeepaliveTime time.Duration
	// Keepalive 超时
	KeepaliveTimeout time.Duration
	// 最大连接空闲时间
	MaxConnectionIdle time.Duration
	// 最大连接年龄
	MaxConnectionAge time.Duration
}

// DefaultServerConfig 默认配置
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Host:              "0.0.0.0",
		Port:              50051,
		KeepaliveTime:     30 * time.Second,
		KeepaliveTimeout:  10 * time.Second,
		MaxConnectionIdle: 15 * time.Minute,
		MaxConnectionAge:  30 * time.Minute,
	}
}

// Server gRPC 服务器
type Server struct {
	config     *ServerConfig
	grpcServer *grpc.Server
	listener   net.Listener
}

// NewServer 创建 gRPC 服务器
func NewServer(cfg *ServerConfig) *Server {
	if cfg == nil {
		cfg = DefaultServerConfig()
	}
	return &Server{config: cfg}
}

// Start 启动服务器
func (s *Server) Start() error {
	if (s.config.TLSCertFile == "") != (s.config.TLSKeyFile == "") {
		return fmt.Errorf("gRPC TLS cert and key must be configured together")
	}

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = lis

	// 创建 gRPC 服务器选项
	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: s.config.MaxConnectionIdle,
			MaxConnectionAge:  s.config.MaxConnectionAge,
			Time:              s.config.KeepaliveTime,
			Timeout:           s.config.KeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	// 添加拦截器
	interceptors := []grpc.UnaryServerInterceptor{
		LoggingInterceptor(),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		StreamLoggingInterceptor(),
	}

	// 认证拦截器始终启用: 每个节点自带的 x-api-key/x-node-id 必须校验通过,
	// 不能因为没配置全局 api_token/JWT 就完全跳过认证 (那样任何人接上
	// gRPC 端口都能冒充任意 node_id 上报数据)。api_token/JWT 仅用于给
	// 没有节点 key 的旧版调用方或管理端做兼容回退。
	interceptors = append(interceptors, AuthInterceptor(s.config.APIToken, s.config.JWTSecret))
	streamInterceptors = append(streamInterceptors, StreamAuthInterceptor(s.config.APIToken, s.config.JWTSecret))

	// 链式拦截器
	if len(interceptors) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(interceptors...))
	}
	if len(streamInterceptors) > 0 {
		opts = append(opts, grpc.ChainStreamInterceptor(streamInterceptors...))
	}

	// TLS: 配置了证书/私钥时启用, 否则保持明文 (向后兼容)
	if s.config.TLSCertFile != "" && s.config.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.config.TLSCertFile, s.config.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("failed to load gRPC TLS cert/key: %w", err)
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
		})))
	}

	// 创建 gRPC 服务器
	s.grpcServer = grpc.NewServer(opts...)

	// 注册服务
	pb.RegisterNodeServiceServer(s.grpcServer, NewNodeGRPCServer())
	RegisterNodeLogServiceServer(s.grpcServer, NewNodeLogGRPCServer())
	pb.RegisterUserServiceServer(s.grpcServer, NewUserGRPCServer())
	pb.RegisterTrafficServiceServer(s.grpcServer, NewTrafficGRPCServer())
	pb.RegisterHealthServiceServer(s.grpcServer, NewHealthGRPCServer())
	agentv1pb.RegisterAgentControlServiceServer(s.grpcServer, NewAgentControlGRPCServer(nil))

	// 启动服务器
	go func() {
		slog.Info("gRPC server listening", "component", "grpc", "addr", addr)
		if err := s.grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC server error", "component", "grpc", "error", err)
		}
	}()

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() {
	if s.grpcServer != nil {
		slog.Info("gRPC server stopping", "component", "grpc")
		s.grpcServer.GracefulStop()
		slog.Info("gRPC server stopped", "component", "grpc")
	}
}

// GracefulShutdown 优雅关闭
func (s *Server) GracefulShutdown(ctx context.Context) error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		slog.Info("gRPC received signal", "component", "grpc", "signal", sig.String())
		s.Stop()
		return nil
	case <-ctx.Done():
		s.Stop()
		return ctx.Err()
	}
}

// GetConnectionManager 获取连接管理器
func (s *Server) GetConnectionManager() *NodeConnectionManager {
	return GetConnectionManager()
}

// GetAgentControlManager returns the Agent-first desired/observed connection manager.
func (s *Server) GetAgentControlManager() *AgentControlManager {
	return GetAgentControlManager()
}

// Run 运行服务器（阻塞）
func Run(cfg *ServerConfig) error {
	server := NewServer(cfg)
	if err := server.Start(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return server.GracefulShutdown(ctx)
}
