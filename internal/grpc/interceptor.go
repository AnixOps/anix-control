package grpc

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/utils"
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
	Connection any // 可以是具体的流对象
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

// SetNodeConfigVersion 记录节点已推送的配置版本
func (m *NodeConnectionManager) SetNodeConfigVersion(nodeID uint32, version int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configVer[nodeID] = version
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
//
// authed is false with a nil error when the call carries no node key. A
// disabled node is refused (PermissionDenied), as the HTTP node API and the
// Agent control stream refuse it.
func authenticateNode(ctx context.Context) (nodeID uint32, authed bool, err error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0, false, nil
	}
	keys := md.Get("x-api-key")
	if len(keys) == 0 || keys[0] == "" {
		return 0, false, nil
	}
	ids := md.Get("x-node-id")
	if len(ids) == 0 {
		return 0, false, status.Error(codes.Unauthenticated, "missing x-node-id")
	}
	claimed, parseErr := strconv.ParseUint(ids[0], 10, 32)
	if parseErr != nil || claimed == 0 {
		return 0, false, status.Error(codes.Unauthenticated, "invalid x-node-id")
	}

	node, lookupErr := service.NewNodeService().GetNodeByAPIKey(keys[0])
	if lookupErr != nil {
		return 0, false, status.Error(codes.Unauthenticated, "invalid node api key")
	}
	if uint64(node.ID) != claimed {
		return 0, false, status.Error(codes.Unauthenticated, "node api key does not match x-node-id")
	}
	if node.Status == model.NodeStatusDisabled {
		return 0, false, status.Error(codes.PermissionDenied, "node is disabled")
	}
	return uint32(claimed), true, nil
}

// recordV2boardSighting records a node the v2board services authenticated
// in the transport inventory. agent_control.mtls never applies to these
// services: third-party node software shares them.
func recordV2boardSighting(ctx context.Context, nodeID uint32, identity string) {
	agenttransport.Seen(ctx, agenttransport.Sighting{
		Node:      agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: nodeID},
		Transport: model.AgentTransportV2boardGRPC, Identity: identity,
	})
}

// AuthInterceptor 认证拦截器: 先校验节点自身 x-api-key, 否则回退到全局
// API Token / JWT 两种认证方式 (管理端/兼容旧调用)
func AuthInterceptor(apiToken, jwtSecret string) grpc.UnaryServerInterceptor {
	return AuthInterceptorWithAgents(apiToken, jwtSecret, nil)
}

// AuthInterceptorWithAgents is AuthInterceptor on a listener that accepts
// agent client certificates. The Agent services authenticate in their
// handlers. On the v2board services a client certificate, when presented,
// must be a proxy node's agent certificate and authenticates the call; the
// request's node_id must name that node. Without a certificate the legacy
// credentials apply, whatever agent_control.mtls says: third-party node
// software keeps using them.
func AuthInterceptorWithAgents(apiToken, jwtSecret string, agents *AgentAuthenticator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// 健康检查和节点注册不需要认证 (注册时节点还没有 api key)
		if strings.Contains(info.FullMethod, "HealthService") || strings.HasSuffix(info.FullMethod, "/Register") ||
			strings.HasPrefix(info.FullMethod, agentServicePrefix) {
			return handler(ctx, req)
		}

		if principal, ok, err := v2boardCertificatePrincipal(ctx, agents); err != nil {
			setRefusalTrailer(err, func(md metadata.MD) { _ = grpc.SetTrailer(ctx, md) })
			return nil, err
		} else if ok {
			recordV2boardSighting(ctx, principal.Node.ID, principal.identity())
			return handler(withNodeCaller(ctx, principal.Node.ID), req)
		}

		if nodeID, authed, err := authenticateNode(ctx); err != nil {
			return nil, err
		} else if authed {
			recordV2boardSighting(ctx, nodeID, agentstreams.IdentityAPIKey)
			return handler(withNodeCaller(ctx, nodeID), req)
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
		ctx = withAdminCaller(ctx)

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
	return StreamAuthInterceptorWithAgents(apiToken, jwtSecret, nil)
}

// StreamAuthInterceptorWithAgents is StreamAuthInterceptor on a listener
// that accepts agent client certificates; see AuthInterceptorWithAgents.
// Every message of a certificate-authenticated v2board stream must carry
// the certificate's node_id.
func StreamAuthInterceptorWithAgents(apiToken, jwtSecret string, agents *AgentAuthenticator) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// 健康检查不需要认证; the Agent services authenticate in their
		// handlers (certificate or node API key, per agent_control.mtls).
		if strings.Contains(info.FullMethod, "HealthService") || strings.HasPrefix(info.FullMethod, agentServicePrefix) {
			return handler(srv, ss)
		}

		if principal, ok, err := v2boardCertificatePrincipal(ss.Context(), agents); err != nil {
			setRefusalTrailer(err, ss.SetTrailer)
			return err
		} else if ok {
			recordV2boardSighting(ss.Context(), principal.Node.ID, principal.identity())
			return handler(srv, &streamWithContext{ServerStream: ss, ctx: withNodeCaller(ss.Context(), principal.Node.ID)})
		}

		if nodeID, authed, err := authenticateNode(ss.Context()); err != nil {
			return err
		} else if authed {
			recordV2boardSighting(ss.Context(), nodeID, agentstreams.IdentityAPIKey)
			return handler(srv, &streamWithContext{ServerStream: ss, ctx: withNodeCaller(ss.Context(), nodeID)})
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

		wrapped := &streamWithContext{
			ServerStream: ss,
			ctx:          withAdminCaller(ss.Context()),
		}
		// 如果 JWT 解析成功，将用户信息放入流上下文
		if claims != nil {
			wrapped.ctx = SetUserIDToContext(wrapped.ctx, claims.UserID)
			wrapped.ctx = SetUserEmailToContext(wrapped.ctx, claims.Email)
			wrapped.ctx = SetUserAdminToContext(wrapped.ctx, claims.IsAdmin)
		}
		return handler(srv, wrapped)
	}
}

// v2boardCertificatePrincipal authenticates a v2board call by client
// certificate: only a proxy node's agent certificate is accepted there.
func v2boardCertificatePrincipal(ctx context.Context, agents *AgentAuthenticator) (agentPrincipal, bool, error) {
	principal, ok, err := agents.certificatePrincipal(ctx)
	if err != nil || !ok {
		return agentPrincipal{}, false, err
	}
	if principal.Node.Kind != agentcontrol.NodeKindProxy {
		return agentPrincipal{}, false, refuseAgent(agentcontrol.ErrorCodeCertWrongNode, codes.PermissionDenied, "only proxy node certificates may call the v2board services")
	}
	return principal, true, nil
}

// validateToken 验证 token，支持 JWT 和 API Token 两种方式
// 返回: (是否认证通过, JWT claims(如果不是JWT则为nil), 错误信息)
//
// Both authenticate an administrator, who may act for any node: the
// global grpc.api_token, or the JWT of an administrator (a user's login
// JWT is refused). With neither configured every token is refused, and
// only node API keys authenticate.
func validateToken(token, apiToken, jwtSecret string) (bool, *utils.Claims, string) {
	// 优先尝试 JWT 验证（如果 token 看起来像 JWT 且配置了 secret）
	if jwtSecret != "" && strings.Contains(token, ".") {
		verifier := authn.Verifier{Secret: func() string { return jwtSecret }, Revocations: authn.DefaultStore(), IdentityKeys: authn.DefaultIdentityKeys()}
		claims, err := verifier.Verify(context.Background(), token)
		if err == nil {
			if !claims.IsAdmin {
				return false, nil, "an administrator token is required"
			}
			return true, claims, ""
		}
		// JWT 验证失败，如果同时配置了 API Token 则回退
		if apiToken == "" {
			return false, nil, "invalid or expired JWT token"
		}
	}

	// API Token 验证
	if apiToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(apiToken)) == 1 {
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
