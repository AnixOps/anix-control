package grpc

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/service"
	"github.com/AnixOps/anix-control/v3/internal/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// NodeConnection 节点连接信息
type NodeConnection struct {
	NodeID     uint32
	LastSeen   time.Time
	RemoteAddr string
	Connection any           // 可以是具体的流对象
	configChan chan struct{} // buffered channel for config push signals
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
		configChan: make(chan struct{}, 1),
	}
}

// Unregister 注销节点连接
func (m *NodeConnectionManager) Unregister(nodeID uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if conn, ok := m.connections[nodeID]; ok && conn.configChan != nil {
		close(conn.configChan)
	}
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

// NotifyConfigChange 通知节点配置变更
func (m *NodeConnectionManager) NotifyConfigChange(nodeID uint32) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if conn, ok := m.connections[nodeID]; ok && conn.configChan != nil {
		select {
		case conn.configChan <- struct{}{}:
		default:
		}
	}
}

// SetNodeConfigVersion 记录节点已推送的配置版本
func (m *NodeConnectionManager) SetNodeConfigVersion(nodeID uint32, version int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configVer[nodeID] = version
}

// GetNodeConfigVersion 获取节点已推送的配置版本
func (m *NodeConnectionManager) GetNodeConfigVersion(nodeID uint32) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configVer[nodeID]
}

// 全局连接管理器
var connectionManager = NewNodeConnectionManager()

// GetConnectionManager 获取连接管理器
func GetConnectionManager() *NodeConnectionManager {
	return connectionManager
}

// authenticateNode 校验节点自带的 x-api-key/x-node-id metadata (V2bX 通过
// GRPCClient.withAuth 附带), 而不是全局共享的 api_token/JWT。每个节点用自己的
// APIKeyHash 校验, 且 x-node-id 必须与 key 对应的节点一致, 防止一个节点的
// key 被拿来冒充另一个 node_id。Register 方法没有 key (节点还没注册), 由
// 调用方跳过。
func authenticateNode(ctx context.Context) (authed bool, errMsg string) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false, ""
	}
	keys := md.Get("x-api-key")
	if len(keys) == 0 || keys[0] == "" {
		return false, ""
	}
	ids := md.Get("x-node-id")
	if len(ids) == 0 {
		return false, "missing x-node-id"
	}
	nodeID, err := strconv.ParseUint(ids[0], 10, 32)
	if err != nil {
		return false, "invalid x-node-id"
	}

	node, err := service.NewNodeService().GetNodeByAPIKey(keys[0])
	if err != nil {
		return false, "invalid node api key"
	}
	if uint64(node.ID) != nodeID {
		return false, "node api key does not match x-node-id"
	}
	return true, ""
}

// AuthInterceptor 认证拦截器: 先校验节点自身 x-api-key, 否则回退到全局
// API Token / JWT 两种认证方式 (管理端/兼容旧调用)
func AuthInterceptor(apiToken, jwtSecret string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// 健康检查和节点注册不需要认证 (注册时节点还没有 api key)
		if strings.Contains(info.FullMethod, "HealthService") || strings.HasSuffix(info.FullMethod, "/Register") {
			return handler(ctx, req)
		}

		if authed, errMsg := authenticateNode(ctx); authed {
			return handler(ctx, req)
		} else if errMsg != "" {
			return nil, status.Error(codes.Unauthenticated, errMsg)
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
		token, _ = strings.CutPrefix(token, "Bearer ")

		// 验证 token
		authed, claims, err := validateToken(token, apiToken, jwtSecret)
		if !authed {
			return nil, status.Error(codes.Unauthenticated, err)
		}

		// 如果 JWT 解析成功，将用户信息放入上下文
		if claims != nil {
			ctx = SetUserIDToContext(ctx, claims.UserID)
			ctx = SetUserEmailToContext(ctx, claims.Email)
			ctx = SetUserAdminToContext(ctx, claims.IsAdmin)
		}

		return handler(ctx, req)
	}
}

// StreamAuthInterceptor 流式认证拦截器: 先校验节点自身 x-api-key, 否则回退到
// 全局 API Token / JWT 两种认证方式 (管理端/兼容旧调用)
func StreamAuthInterceptor(apiToken, jwtSecret string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// 健康检查不需要认证
		if strings.Contains(info.FullMethod, "HealthService") {
			return handler(srv, ss)
		}

		if authed, errMsg := authenticateNode(ss.Context()); authed {
			return handler(srv, ss)
		} else if errMsg != "" {
			return status.Error(codes.Unauthenticated, errMsg)
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
		token, _ = strings.CutPrefix(token, "Bearer ")

		// 验证 token
		authed, claims, errStr := validateToken(token, apiToken, jwtSecret)
		if !authed {
			return status.Error(codes.Unauthenticated, errStr)
		}

		// 如果 JWT 解析成功，将用户信息放入流上下文
		if claims != nil {
			wrapped := &streamWithContext{
				ServerStream: ss,
				ctx:          ss.Context(),
			}
			wrapped.ctx = SetUserIDToContext(wrapped.ctx, claims.UserID)
			wrapped.ctx = SetUserEmailToContext(wrapped.ctx, claims.Email)
			wrapped.ctx = SetUserAdminToContext(wrapped.ctx, claims.IsAdmin)
			return handler(srv, wrapped)
		}

		return handler(srv, ss)
	}
}

// validateToken 验证 token，支持 JWT 和 API Token 两种方式
// 返回: (是否认证通过, JWT claims(如果不是JWT则为nil), 错误信息)
func validateToken(token, apiToken, jwtSecret string) (bool, *utils.Claims, string) {
	// 优先尝试 JWT 验证（如果 token 看起来像 JWT 且配置了 secret）
	if jwtSecret != "" && strings.Contains(token, ".") {
		claims, err := utils.ParseTokenWithSecret(token, jwtSecret)
		if err == nil {
			return true, claims, ""
		}
		// JWT 验证失败，如果同时配置了 API Token 则回退
		if apiToken == "" {
			return false, nil, "invalid or expired JWT token"
		}
	}

	// API Token 验证
	if apiToken != "" && token == apiToken {
		return true, nil, ""
	}

	if apiToken == "" && jwtSecret == "" {
		// 没有配置任何认证，放行
		return true, nil, ""
	}

	return false, nil, "invalid token"
}

// streamWithContext 包装 grpc.ServerStream 以便注入自定义 context
type streamWithContext struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *streamWithContext) Context() context.Context {
	return w.ctx
}

// LoggingInterceptor 日志拦截器
func LoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
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
		slog.Info("grpc unary request", "component", "grpc", "method", info.FullMethod, "addr", clientAddr, "duration", duration.Milliseconds(), "code", code.String())

		return resp, err
	}
}

// StreamLoggingInterceptor 流式日志拦截器
func StreamLoggingInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
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

		slog.Info("grpc stream request", "component", "grpc", "method", info.FullMethod, "addr", clientAddr, "duration", duration.Milliseconds(), "code", code.String())

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

type userIDKey struct{}

type userEmailKey struct{}

type userAdminKey struct{}

// SetUserIDToContext 设置用户ID到上下文
func SetUserIDToContext(ctx context.Context, userID uint) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

// GetUserIDFromContext 从上下文获取用户ID
func GetUserIDFromContext(ctx context.Context) (uint, bool) {
	userID, ok := ctx.Value(userIDKey{}).(uint)
	return userID, ok
}

// SetUserEmailToContext 设置用户邮箱到上下文
func SetUserEmailToContext(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, userEmailKey{}, email)
}

// GetUserEmailFromContext 从上下文获取用户邮箱
func GetUserEmailFromContext(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(userEmailKey{}).(string)
	return email, ok
}

// SetUserAdminToContext 设置用户管理员状态到上下文
func SetUserAdminToContext(ctx context.Context, isAdmin bool) context.Context {
	return context.WithValue(ctx, userAdminKey{}, isAdmin)
}

// GetUserAdminFromContext 从上下文获取用户管理员状态
func GetUserAdminFromContext(ctx context.Context) (bool, bool) {
	isAdmin, ok := ctx.Value(userAdminKey{}).(bool)
	return isAdmin, ok
}
