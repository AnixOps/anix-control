package grpc

import (
	"context"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// NodeConnection 节点连接信息
type NodeConnection struct {
	NodeID      uint32
	LastSeen    time.Time
	RemoteAddr  string
	Connection  interface{} // 可以是具体的流对象
}

// NodeConnectionManager 节点连接管理器
type NodeConnectionManager struct {
	mu          sync.RWMutex
	connections map[uint32]*NodeConnection
	configVer   map[uint32]int64 // 节点配置版本
}

// NewNodeConnectionManager 创建连接管理器
func NewNodeConnectionManager() *NodeConnectionManager {
	return &NodeConnectionManager{
		connections: make(map[uint32]*NodeConnection),
		configVer:   make(map[uint32]int64),
	}
}

// Register 注册节点连接
func (m *NodeConnectionManager) Register(nodeID uint32, addr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connections[nodeID] = &NodeConnection{
		NodeID:     nodeID,
		LastSeen:   time.Now(),
		RemoteAddr: addr,
	}
}

// Unregister 注销节点连接
func (m *NodeConnectionManager) Unregister(nodeID uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.connections, nodeID)
}

// UpdateLastSeen 更新最后活跃时间
func (m *NodeConnectionManager) UpdateLastSeen(nodeID uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if conn, ok := m.connections[nodeID]; ok {
		conn.LastSeen = time.Now()
	}
}

// GetConnection 获取节点连接
func (m *NodeConnectionManager) GetConnection(nodeID uint32) (*NodeConnection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.connections[nodeID]
	return conn, ok
}

// GetActiveNodes 获取所有活跃节点
func (m *NodeConnectionManager) GetActiveNodes() []uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	nodes := make([]uint32, 0, len(m.connections))
	for id := range m.connections {
		nodes = append(nodes, id)
	}
	return nodes
}

// UpdateConfigVersion 更新配置版本
func (m *NodeConnectionManager) UpdateConfigVersion(nodeID uint32, version int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configVer[nodeID] = version
}

// GetConfigVersion 获取配置版本
func (m *NodeConnectionManager) GetConfigVersion(nodeID uint32) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configVer[nodeID]
}

// IsConfigChanged 检查配置是否变更
func (m *NodeConnectionManager) IsConfigChanged(nodeID uint32, currentVer int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lastVer, ok := m.configVer[nodeID]
	if !ok {
		return true // 首次连接，需要推送
	}
	return currentVer > lastVer
}

// 全局连接管理器
var connectionManager = NewNodeConnectionManager()

// GetConnectionManager 获取连接管理器
func GetConnectionManager() *NodeConnectionManager {
	return connectionManager
}

// AuthInterceptor 认证拦截器
func AuthInterceptor(apiToken string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 健康检查不需要认证
		if strings.Contains(info.FullMethod, "HealthService") {
			return handler(ctx, req)
		}

		// 从 metadata 获取 token
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		tokens := md.Get("authorization")
		if len(tokens) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization token")
		}

		token := tokens[0]
		if strings.HasPrefix(token, "Bearer ") {
			token = strings.TrimPrefix(token, "Bearer ")
		}

		// 验证 token
		if apiToken != "" && token != apiToken {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		return handler(ctx, req)
	}
}

// StreamAuthInterceptor 流式认证拦截器
func StreamAuthInterceptor(apiToken string) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// 健康检查不需要认证
		if strings.Contains(info.FullMethod, "HealthService") {
			return handler(srv, ss)
		}

		// 从 metadata 获取 token
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			return status.Error(codes.Unauthenticated, "missing metadata")
		}

		tokens := md.Get("authorization")
		if len(tokens) == 0 {
			return status.Error(codes.Unauthenticated, "missing authorization token")
		}

		token := tokens[0]
		if strings.HasPrefix(token, "Bearer ") {
			token = strings.TrimPrefix(token, "Bearer ")
		}

		// 验证 token
		if apiToken != "" && token != apiToken {
			return status.Error(codes.Unauthenticated, "invalid token")
		}

		return handler(srv, ss)
	}
}

// LoggingInterceptor 日志拦截器
func LoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// 获取客户端地址
		clientAddr := "unknown"
		if p, ok := peer.FromContext(ctx); ok {
			clientAddr = p.Addr.String()
		}

		resp, err := handler(ctx, req)

		// 记录请求
		duration := time.Since(start)
		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}

		// 使用标准日志输出
		println("[GRPC]", info.FullMethod, "from", clientAddr, "duration", duration.String(), "code", code.String())

		return resp, err
	}
}

// StreamLoggingInterceptor 流式日志拦截器
func StreamLoggingInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()

		// 获取客户端地址
		clientAddr := "unknown"
		if p, ok := peer.FromContext(ss.Context()); ok {
			clientAddr = p.Addr.String()
		}

		err := handler(srv, ss)

		// 记录请求
		duration := time.Since(start)
		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}

		println("[GRPC STREAM]", info.FullMethod, "from", clientAddr, "duration", duration.String(), "code", code.String())

		return err
	}
}

// GetPeerAddr 从上下文获取客户端地址
func GetPeerAddr(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "unknown"
	}
	return p.Addr.String()
}

// GetNodeIDFromContext 从上下文获取节点ID（需要先在拦截器中设置）
func GetNodeIDFromContext(ctx context.Context) uint32 {
	nodeID, ok := ctx.Value(nodeIDKey{}).(uint32)
	if !ok {
		return 0
	}
	return nodeID
}

// SetNodeIDToContext 设置节点ID到上下文
func SetNodeIDToContext(ctx context.Context, nodeID uint32) context.Context {
	return context.WithValue(ctx, nodeIDKey{}, nodeID)
}

type nodeIDKey struct{}