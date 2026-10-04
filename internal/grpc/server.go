package grpc

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/panicrecovery"
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
	// AgentPKI verifies agent client certificates and serves
	// AgentEnrollment; nil when the built-in module PKI is off. Client
	// certificates need TLS (TLSCertFile).
	AgentPKI *agentpki.Service
	// AgentMTLS is agent_control.mtls: off, optional, preferred or
	// required (the configuration default from 4.2).
	AgentMTLS string
	// AgentLegacySunset is agent_control.legacy_sunset, announced to legacy
	// agents in preferred mode; zero announces none.
	AgentLegacySunset time.Time
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

// agentAuthenticator returns how agents authenticate on this listener.
func (s *Server) agentAuthenticator() *AgentAuthenticator {
	return &AgentAuthenticator{PKI: s.config.AgentPKI, Mode: s.config.AgentMTLS, Sunset: s.config.AgentLegacySunset}
}

// tlsConfig is the listener's TLS configuration. With the agent PKI the
// server asks for a client certificate but never requires one in the
// handshake: agents enroll without one, and legacy agents have none. The
// Agent services and the interceptors verify a presented certificate
// against the agent trust bundle, which changes with CA rotation. In
// agent_control.mtls: off no client certificate is requested.
func (s *Server) tlsConfig(cert tls.Certificate) *tls.Config {
	config := &tls.Config{Certificates: []tls.Certificate{cert}}
	if s.agentAuthenticator().pki() != nil {
		config.ClientAuth = tls.RequestClientCert
	}
	return config
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

	interceptors, streamInterceptors := s.interceptorChains()

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
		opts = append(opts, grpc.Creds(credentials.NewTLS(s.tlsConfig(cert))))
	}

	// 创建 gRPC 服务器
	s.grpcServer = grpc.NewServer(opts...)

	// 注册服务
	pb.RegisterNodeServiceServer(s.grpcServer, NewNodeGRPCServer())
	RegisterNodeLogServiceServer(s.grpcServer, NewNodeLogGRPCServer())
	pb.RegisterUserServiceServer(s.grpcServer, NewUserGRPCServer())
	pb.RegisterTrafficServiceServer(s.grpcServer, NewTrafficGRPCServer())
	pb.RegisterHealthServiceServer(s.grpcServer, NewHealthGRPCServer())
	agents := s.agentAuthenticator()
	// Proxy node operations take their revisions from the durable per-node
	// cursor that plugin operations use; forward nodes keep the in-memory
	// counter (their ids overlap proxy node ids).
	GetAgentControlManager().UseRevisionStore(NewDatabaseRevisionStore(databaseForAgentChecks))
	agentv1pb.RegisterAgentControlServiceServer(s.grpcServer, NewAgentControlGRPCServer(nil).WithAuthenticator(agents))
	agentv1pb.RegisterAgentEnrollmentServer(s.grpcServer, NewAgentEnrollmentGRPCServer(agents))
	agentv1pb.RegisterAgentArtifactsServer(s.grpcServer, NewAgentArtifactsGRPCServer(agents))

	// 启动服务器
	go func() {
		slog.Info("gRPC server listening", "component", "grpc", "addr", addr)
		if err := s.grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC server error", "component", "grpc", "error", err)
		}
	}()

	return nil
}

// Addr returns the address the started listener is bound to, for a
// configured port of 0.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Stop 停止服务器
func (s *Server) Stop() {
	if s.grpcServer != nil {
		slog.Info("gRPC server stopping", "component", "grpc")
		s.grpcServer.GracefulStop()
		slog.Info("gRPC server stopped", "component", "grpc")
	}
}

// AgentPKI returns the agent PKI the listener verifies client certificates
// with, nil when the built-in module PKI is off.
func (s *Server) AgentPKI() *agentpki.Service {
	return s.config.AgentPKI
}

// GetAgentControlManager returns the Agent-first desired/observed connection manager.
func (s *Server) GetAgentControlManager() *AgentControlManager {
	return GetAgentControlManager()
}

// interceptorChains returns the node-facing server's unary and stream
// interceptor chains, outermost first.
func (s *Server) interceptorChains() ([]grpc.UnaryServerInterceptor, []grpc.StreamServerInterceptor) {
	// 添加拦截器. Panic recovery is outermost so a panic in a handler or in a
	// later interceptor becomes codes.Internal instead of killing the kernel.
	interceptors := []grpc.UnaryServerInterceptor{
		panicrecovery.UnaryServerInterceptor(),
		LoggingInterceptor(),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		panicrecovery.StreamServerInterceptor(),
		StreamLoggingInterceptor(),
	}

	// 认证拦截器始终启用: 每个节点自带的 x-api-key/x-node-id 必须校验通过,
	// 不能因为没配置全局 api_token/JWT 就完全跳过认证 (那样任何人接上
	// gRPC 端口都能冒充任意 node_id 上报数据)。api_token/JWT 仅用于给
	// 没有节点 key 的旧版调用方或管理端做兼容回退。
	agents := s.agentAuthenticator()
	interceptors = append(interceptors, AuthInterceptorWithAgents(s.config.APIToken, s.config.JWTSecret, agents))
	streamInterceptors = append(streamInterceptors, StreamAuthInterceptorWithAgents(s.config.APIToken, s.config.JWTSecret, agents))
	return interceptors, streamInterceptors
}
