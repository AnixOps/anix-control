package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/anixops/v2board/api/grpc/v2boardpb"
	"google.golang.org/grpc"
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
		Host:             "0.0.0.0",
		Port:             50051,
		KeepaliveTime:    30 * time.Second,
		KeepaliveTimeout: 10 * time.Second,
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

	// 如果配置了 API Token 或 JWT Secret，添加认证拦截器
	hasAuth := s.config.APIToken != "" || s.config.JWTSecret != ""
	if hasAuth {
		interceptors = append(interceptors, AuthInterceptor(s.config.APIToken, s.config.JWTSecret))
		streamInterceptors = append(streamInterceptors, StreamAuthInterceptor(s.config.APIToken, s.config.JWTSecret))
	}

	// 链式拦截器
	if len(interceptors) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(interceptors...))
	}
	if len(streamInterceptors) > 0 {
		opts = append(opts, grpc.ChainStreamInterceptor(streamInterceptors...))
	}

	// 创建 gRPC 服务器
	s.grpcServer = grpc.NewServer(opts...)

	// 注册服务
	pb.RegisterNodeServiceServer(s.grpcServer, NewNodeGRPCServer())
	pb.RegisterUserServiceServer(s.grpcServer, NewUserGRPCServer())
	pb.RegisterTrafficServiceServer(s.grpcServer, NewTrafficGRPCServer())
	pb.RegisterHealthServiceServer(s.grpcServer, NewHealthGRPCServer())

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