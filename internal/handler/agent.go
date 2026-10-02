package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/agentws"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

// agentDiagnosticTaskType is the type of every task an administrator sends
// to an agent: a whitelisted diagnostic action (ValidateAgentDiagnosticTask).
const agentDiagnosticTaskType = kernelnodeops.AgentDiagnosticTaskType

var (
	agentWSReadTimeout  = 60 * time.Second
	agentWSWriteTimeout = 10 * time.Second
)

// AgentHandler Agent 管理 API
type AgentHandler struct {
	db            *gorm.DB
	connections   sync.Map // nodeID -> *AgentConnection
	pendingAcks   sync.Map // messageID -> *wsPendingAck
	monitorData   sync.Map // nodeID -> AgentMonitorSnapshot
	diagnosticSvc *service.AgentDiagnosticTaskService
	wsUpgrader    websocket.Upgrader
	ackTimeout    time.Duration
	maxRetries    int
}

// AgentConnection Agent 连接信息
type AgentConnection struct {
	NodeID        uint
	LastSeen      time.Time
	Version       string
	SystemInfo    map[string]any
	Capabilities  []string
	IsForwardNode bool
	WsConn        *websocket.Conn
	writeMu       sync.Mutex
}

type wsAuthInfo struct {
	NodeID        uint
	Version       string
	System        map[string]any
	Capabilities  []string
	FromHeaders   bool
	IsForwardNode bool
}

type wsInboundEnvelope struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	NodeID     uint            `json:"node_id"`
	Timestamp  int64           `json:"timestamp"`
	Payload    json.RawMessage `json:"payload"`
	RequireAck bool            `json:"require_ack,omitempty"`
}

type wsOutboundEnvelope struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	NodeID     uint   `json:"node_id"`
	Timestamp  int64  `json:"timestamp"`
	Payload    any    `json:"payload,omitempty"`
	RequireAck bool   `json:"require_ack,omitempty"`
}

type wsAckPayload struct {
	MessageID string `json:"msg_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

type wsPendingAck struct {
	NodeID uint
	Chan   chan *wsAckPayload
}

var (
	errAgentNodeNotFound    = errors.New("node not found")
	errAgentInvalidToken    = errors.New("invalid token")
	errAgentMissingIdentity = errors.New("missing node_id or token")
	errAgentNodeMismatch    = errors.New("node_id does not match the authenticated node")
	errAgentTaskNotFound    = errors.New("task not found")
)

// hashString hashes a token with SHA-256, matching the api_key_hash contract
// used by the UniProxy node-auth middleware and node registration.
func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// NewAgentHandler 创建 Handler
func NewAgentHandler() *AgentHandler {
	db := database.Get()
	return &AgentHandler{
		db:            db,
		diagnosticSvc: service.NewAgentDiagnosticTaskService(db),
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: utils.CheckWebSocketOrigin,
		},
		ackTimeout: 3 * time.Second,
		maxRetries: 2,
	}
}

// verifyForwardNodeToken authenticates an agent/node WebSocket or REST call.
// It first tries the forward-node table (relay nodes carry an APIToken); if the
// id is not a forward node it falls back to the proxy-node table (v2_node),
// matching the same api_key/api_key_hash contract used by the UniProxy
// middleware. The returned isForwardNode tells callers which table owns the
// node so online-status writes land in the right place.
//
// Both checks go through the node credential split, which applies each
// table's phase. An empty token, a tombstone and the placeholder never
// authenticate: a node whose key or token is empty would otherwise accept
// anyone.
func (h *AgentHandler) verifyForwardNodeToken(nodeID uint, token string) (isForwardNode bool, err error) {
	var fwd model.ForwardNode
	if fwdErr := h.db.First(&fwd, nodeID).Error; fwdErr == nil {
		if !nodesecrets.ForwardNodeTokenMatches(h.db, &fwd, token) {
			return true, errAgentInvalidToken
		}
		return true, nil
	}

	var node model.Node
	if nodeErr := h.db.First(&node, nodeID).Error; nodeErr != nil {
		return false, errAgentNodeNotFound
	}

	if !nodesecrets.NodeAPIKeyMatches(h.db, &node, token) {
		return false, errAgentInvalidToken
	}
	return false, nil
}

func prepareAgentWebSocket(conn *websocket.Conn) error {
	if conn == nil {
		return fmt.Errorf("ws connection not available")
	}
	conn.SetReadLimit(512 * 1024)
	if err := refreshAgentWebSocketReadDeadline(conn); err != nil {
		return err
	}
	conn.SetPongHandler(func(string) error {
		return refreshAgentWebSocketReadDeadline(conn)
	})
	return nil
}

func refreshAgentWebSocketReadDeadline(conn *websocket.Conn) error {
	return conn.SetReadDeadline(time.Now().Add(agentWSReadTimeout))
}

func readAgentWebSocketMessage(conn *websocket.Conn) ([]byte, error) {
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if err := refreshAgentWebSocketReadDeadline(conn); err != nil {
		return nil, err
	}
	return msg, nil
}

func writeAgentWebSocketJSON(agentConn *AgentConnection, payload any) error {
	if agentConn == nil || agentConn.WsConn == nil {
		return fmt.Errorf("ws connection not available")
	}
	agentConn.writeMu.Lock()
	defer agentConn.writeMu.Unlock()
	if err := agentConn.WsConn.SetWriteDeadline(time.Now().Add(agentWSWriteTimeout)); err != nil {
		return err
	}
	return agentConn.WsConn.WriteJSON(payload)
}

// touchNodeOnline marks a node online in the table that owns it; a
// disabled proxy node stays disabled.
func (h *AgentHandler) touchNodeOnline(nodeID uint, isForwardNode bool) {
	if isForwardNode {
		h.db.Model(&model.ForwardNode{}).Where("id = ?", nodeID).Updates(map[string]any{
			"status":     model.ForwardNodeStatusOnline,
			"last_check": time.Now(),
		})
		return
	}
	h.db.Model(&model.Node{}).Where("id = ?", nodeID).Updates(map[string]any{
		"status":        service.NodeHeartbeatStatus(),
		"last_check_at": time.Now().Unix(),
	})
}

func (h *AgentHandler) updateConnectionLastSeen(nodeID uint) {
	isForwardNode := true
	if conn, ok := h.connections.Load(nodeID); ok {
		ac := conn.(*AgentConnection)
		ac.LastSeen = time.Now()
		isForwardNode = ac.IsForwardNode
	}
	h.touchNodeOnline(nodeID, isForwardNode)
}

// markAgentSeen records a report of an authenticated agent: the last-seen
// time of its live connection, if any, and its node online in the table that
// authenticated it.
func (h *AgentHandler) markAgentSeen(nodeID uint, isForwardNode bool) {
	if conn, ok := h.connections.Load(nodeID); ok {
		conn.(*AgentConnection).LastSeen = time.Now()
	}
	h.touchNodeOnline(nodeID, isForwardNode)
}

// RequireAgentNode authenticates an agent's HTTP request in the kernel,
// before the package gateway, as the agent WebSocket's preflight does: the
// node id comes from X-Node-ID or the node_id query, the token from
// X-API-Key or the api_key or token query, and verifyForwardNodeToken checks
// it. The verified node travels to the legacy handler as the kernel's trusted
// agent identity, so the credential never reaches the package host.
//
// The heartbeat, task, result and monitor routes had no authentication: anyone
// could mark any node online, take and complete its queued tasks (forward
// runtime jobs included) or report its monitoring data.
func (h *AgentHandler) RequireAgentNode(c *gin.Context) {
	authInfo, hasRequestAuth, err := h.authFromRequest(c)
	if err == nil && (!hasRequestAuth || authInfo == nil || authInfo.NodeID == 0) {
		err = errAgentMissingIdentity
	}
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.Set(agentws.TrustedContextKey, true)
	c.Set(agentws.ForwardNodeContextKey, authInfo.IsForwardNode)
	c.Set("node_id", authInfo.NodeID)
	c.Next()
}

// authenticatedAgentNode returns the node an agent request is authenticated
// as: the kernel's trusted identity (RequireAgentNode, relayed by the package
// bridge), or the request's own credentials. Without one it answers 401 and
// returns false. A node id in a body is only ever checked against it.
func (h *AgentHandler) authenticatedAgentNode(c *gin.Context) (nodeID uint, isForwardNode bool, ok bool) {
	authInfo, hasRequestAuth, err := h.authFromRequest(c)
	if err == nil && (!hasRequestAuth || authInfo == nil || authInfo.NodeID == 0) {
		err = errAgentMissingIdentity
	}
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return 0, false, false
	}
	return authInfo.NodeID, authInfo.IsForwardNode, true
}

// PrepareWebSocketBridge authenticates optional request credentials before a
// package-host relay is opened. It retains only the verified node identity in
// Gin context; the credential itself never reaches the package host.
func (h *AgentHandler) PrepareWebSocketBridge(c *gin.Context) bool {
	if c == nil {
		return false
	}
	authInfo, hasRequestAuth, err := h.authFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return false
	}
	if !hasRequestAuth {
		return true
	}
	if authInfo == nil || authInfo.NodeID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid node_id"})
		return false
	}
	c.Set(agentws.TrustedContextKey, true)
	c.Set(agentws.ForwardNodeContextKey, authInfo.IsForwardNode)
	c.Set("node_id", authInfo.NodeID)
	return true
}

func (h *AgentHandler) authFromRequest(c *gin.Context) (*wsAuthInfo, bool, error) {
	if agentWebSocketContextBool(c, agentws.TrustedContextKey) {
		nodeID := agentWebSocketContextNodeID(c)
		if nodeID == 0 {
			return nil, true, fmt.Errorf("invalid trusted node_id")
		}
		return &wsAuthInfo{
			NodeID: nodeID, FromHeaders: true,
			IsForwardNode: agentWebSocketContextBool(c, agentws.ForwardNodeContextKey),
		}, true, nil
	}
	nodeIDStr := c.Query("node_id")
	if nodeIDStr == "" {
		nodeIDStr = c.GetHeader("X-Node-ID")
	}

	token := c.GetHeader("X-API-Key")
	if token == "" {
		token = c.Query("api_key")
	}
	if token == "" {
		token = c.Query("token")
	}

	if nodeIDStr == "" && token == "" {
		return nil, false, nil
	}
	if nodeIDStr == "" || token == "" {
		return nil, true, fmt.Errorf("missing node_id or token")
	}

	nodeID64, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		return nil, true, fmt.Errorf("invalid node_id")
	}

	nodeID := uint(nodeID64)
	isForwardNode, err := h.verifyForwardNodeToken(nodeID, token)
	if err != nil {
		return nil, true, err
	}

	return &wsAuthInfo{
		NodeID:        nodeID,
		FromHeaders:   true,
		IsForwardNode: isForwardNode,
	}, true, nil
}

func (h *AgentHandler) authFromMessage(conn *websocket.Conn) (*wsAuthInfo, error) {
	msg, err := readAgentWebSocketMessage(conn)
	if err != nil {
		return nil, err
	}

	var authMsg struct {
		Type         string         `json:"type"`
		NodeID       uint           `json:"node_id"`
		Token        string         `json:"token"`
		Version      string         `json:"version"`
		System       map[string]any `json:"system"`
		Capabilities []string       `json:"capabilities"`
	}
	if err := json.Unmarshal(msg, &authMsg); err != nil || authMsg.Type != "auth" {
		return nil, fmt.Errorf("invalid auth")
	}
	isForwardNode, err := h.verifyForwardNodeToken(authMsg.NodeID, authMsg.Token)
	if err != nil {
		return nil, err
	}

	return &wsAuthInfo{
		NodeID:        authMsg.NodeID,
		Version:       authMsg.Version,
		System:        authMsg.System,
		Capabilities:  authMsg.Capabilities,
		FromHeaders:   false,
		IsForwardNode: isForwardNode,
	}, nil
}

func normalizeInboundMessageType(messageType string) string {
	switch messageType {
	case "config.update":
		return "config_update"
	case "user.update":
		return "user_update"
	case "user.ban":
		return "user_ban"
	case "rule.update":
		return "rule_update"
	case "cert.update":
		return "cert_update"
	case "traffic.report":
		return "traffic_report"
	case "force.reload":
		return "force_reload"
	case "task.assign":
		return "task"
	case "task.result":
		return "task_result"
	default:
		return messageType
	}
}

func agentWebSocketContextBool(c *gin.Context, key string) bool {
	if c == nil {
		return false
	}
	value, ok := c.Get(key)
	return ok && value == true
}

func agentWebSocketContextNodeID(c *gin.Context) uint {
	if c == nil {
		return 0
	}
	value, ok := c.Get("node_id")
	if !ok {
		return 0
	}
	switch nodeID := value.(type) {
	case uint:
		return nodeID
	case uint32:
		return uint(nodeID)
	case int:
		if nodeID > 0 {
			return uint(nodeID)
		}
	case float64:
		if nodeID > 0 {
			return uint(nodeID)
		}
	}
	return 0
}

func parseAckPayload(raw []byte, payload json.RawMessage) (*wsAckPayload, bool) {
	type ackRaw struct {
		MsgID     string `json:"msg_id"`
		MessageID string `json:"message_id"`
		Success   bool   `json:"success"`
		Error     string `json:"error,omitempty"`
		Timestamp int64  `json:"timestamp"`
	}

	decode := func(data []byte) (*wsAckPayload, bool) {
		if len(data) == 0 {
			return nil, false
		}
		var a ackRaw
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, false
		}
		messageID := a.MsgID
		if messageID == "" {
			messageID = a.MessageID
		}
		if messageID == "" {
			return nil, false
		}
		return &wsAckPayload{
			MessageID: messageID,
			Success:   a.Success,
			Error:     a.Error,
			Timestamp: a.Timestamp,
		}, true
	}

	if ack, ok := decode(payload); ok {
		return ack, true
	}
	return decode(raw)
}

func (h *AgentHandler) resolveAck(ack *wsAckPayload) {
	if ack == nil || ack.MessageID == "" {
		return
	}

	pending, ok := h.pendingAcks.LoadAndDelete(ack.MessageID)
	if !ok {
		return
	}

	waiter := pending.(*wsPendingAck)
	select {
	case waiter.Chan <- ack:
	default:
	}
}

func (h *AgentHandler) sendEnvelope(agentConn *AgentConnection, envelope *wsOutboundEnvelope) error {
	return writeAgentWebSocketJSON(agentConn, envelope)
}

func (h *AgentHandler) sendLegacyTask(agentConn *AgentConnection, task AgentTask) error {
	return writeAgentWebSocketJSON(agentConn, map[string]any{
		"type": "task",
		"task": task,
	})
}

func (h *AgentHandler) sendAck(agentConn *AgentConnection, messageID string, err error) {
	if messageID == "" {
		return
	}

	payload := &wsAckPayload{
		MessageID: messageID,
		Success:   err == nil,
		Timestamp: time.Now().Unix(),
	}
	if err != nil {
		payload.Error = err.Error()
	}

	_ = h.sendEnvelope(agentConn, &wsOutboundEnvelope{
		ID:        generateMessageID(),
		Type:      "ack",
		NodeID:    agentConn.NodeID,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	})
}

func (h *AgentHandler) dispatchWithAckRetry(agentConn *AgentConnection, messageType string, payload any, requireAck bool) (string, *wsAckPayload, error) {
	messageID := generateMessageID()
	if !requireAck {
		return messageID, nil, h.sendEnvelope(agentConn, &wsOutboundEnvelope{
			ID:        messageID,
			Type:      messageType,
			NodeID:    agentConn.NodeID,
			Timestamp: time.Now().Unix(),
			Payload:   payload,
		})
	}

	attempts := h.maxRetries + 1
	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		waiter := &wsPendingAck{
			NodeID: agentConn.NodeID,
			Chan:   make(chan *wsAckPayload, 1),
		}
		h.pendingAcks.Store(messageID, waiter)

		sendErr := h.sendEnvelope(agentConn, &wsOutboundEnvelope{
			ID:         messageID,
			Type:       messageType,
			NodeID:     agentConn.NodeID,
			Timestamp:  time.Now().Unix(),
			Payload:    payload,
			RequireAck: true,
		})
		if sendErr != nil {
			h.pendingAcks.Delete(messageID)
			lastErr = sendErr
			continue
		}

		select {
		case ack := <-waiter.Chan:
			h.pendingAcks.Delete(messageID)
			if ack == nil {
				lastErr = fmt.Errorf("empty ack for message %s", messageID)
				continue
			}
			if !ack.Success {
				if ack.Error == "" {
					return messageID, ack, fmt.Errorf("agent nack for message %s", messageID)
				}
				return messageID, ack, fmt.Errorf("agent nack: %s", ack.Error)
			}
			return messageID, ack, nil
		case <-time.After(h.ackTimeout):
			h.pendingAcks.Delete(messageID)
			lastErr = fmt.Errorf("ack timeout after %s", h.ackTimeout)
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("send failed")
	}
	return messageID, nil, lastErr
}

func (h *AgentHandler) failPendingAcksForNode(nodeID uint, reason string) {
	h.pendingAcks.Range(func(key, value any) bool {
		waiter := value.(*wsPendingAck)
		if waiter.NodeID != nodeID {
			return true
		}

		messageID, _ := key.(string)
		h.pendingAcks.Delete(key)
		select {
		case waiter.Chan <- &wsAckPayload{
			MessageID: messageID,
			Success:   false,
			Error:     reason,
			Timestamp: time.Now().Unix(),
		}:
		default:
		}
		return true
	})
}

func (h *AgentHandler) handleWebSocketMessage(agentConn *AgentConnection, raw []byte) {
	if agentConn == nil {
		return
	}

	h.updateConnectionLastSeen(agentConn.NodeID)
	recordWebSocketSighting(context.Background(), agentConn)

	var envelope wsInboundEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return
	}

	if envelope.Type == "" {
		return
	}

	if envelope.NodeID != 0 && envelope.NodeID != agentConn.NodeID {
		return
	}

	envelope.Type = normalizeInboundMessageType(envelope.Type)

	if envelope.Type == "ack" {
		if ack, ok := parseAckPayload(raw, envelope.Payload); ok {
			h.resolveAck(ack)
		}
		return
	}

	switch envelope.Type {
	case "heartbeat", "pong", "traffic_report", "alert", "task_result":
		h.touchNodeOnline(agentConn.NodeID, agentConn.IsForwardNode)
	}

	if envelope.RequireAck && envelope.ID != "" {
		h.sendAck(agentConn, envelope.ID, nil)
	}
}

// ========== Agent 认证和注册 ==========

// AgentRegister godoc
// @Summary Agent 注册
// @Description Agent 启动时注册到面板
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentRegisterRequest true "注册信息"
// @Success 200 {object} map[string]any
// @Router /api/v2/agent/register [post]
func (h *AgentHandler) AgentRegister(c *gin.Context) {
	var req AgentRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证 Token
	isForwardNode, err := h.verifyForwardNodeToken(req.NodeID, req.Token)
	if err != nil {
		if errors.Is(err, errAgentInvalidToken) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 记录连接
	conn := &AgentConnection{
		NodeID:        req.NodeID,
		LastSeen:      time.Now(),
		Version:       req.Version,
		SystemInfo:    req.System,
		Capabilities:  req.Capabilities,
		IsForwardNode: isForwardNode,
	}
	h.connections.Store(req.NodeID, conn)
	h.touchNodeOnline(req.NodeID, isForwardNode)

	c.JSON(http.StatusOK, gin.H{
		"message": "registered",
		"node_id": req.NodeID,
	})
}

// AgentRegisterRequest 注册请求
type AgentRegisterRequest struct {
	NodeID       uint           `json:"node_id" binding:"required,gt=0"`
	Token        string         `json:"token" binding:"required,min=1"`
	Version      string         `json:"version" binding:"omitempty,max=64"`
	System       map[string]any `json:"system"`
	Capabilities []string       `json:"capabilities"`
}

// ========== Agent 心跳 ==========

// AgentHeartbeat godoc
// @Summary Agent 心跳
// @Description Agent 定期发送心跳
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentHeartbeatRequest true "心跳信息"
// @Success 200 {object} map[string]any
// @Router /api/v2/agent/heartbeat [post]
func (h *AgentHandler) AgentHeartbeat(c *gin.Context) {
	nodeID, isForwardNode, ok := h.authenticatedAgentNode(c)
	if !ok {
		return
	}
	var req AgentHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.NodeID != nodeID {
		c.JSON(http.StatusForbidden, gin.H{"error": errAgentNodeMismatch.Error()})
		return
	}

	// 更新连接时间
	h.markAgentSeen(nodeID, isForwardNode)

	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// AgentHeartbeatRequest 心跳请求
type AgentHeartbeatRequest struct {
	NodeID    uint           `json:"node_id" binding:"required,gt=0"`
	Status    string         `json:"status" binding:"omitempty,max=32"`
	Resources map[string]any `json:"resources"`
}

// ========== 任务管理 ==========

// AgentGetTasks godoc
// @Summary 获取待执行任务
// @Description Agent 轮询获取待执行的任务
// @Tags Agent
// @Produce json
// @Param node_id query int true "节点 ID"
// @Success 200 {object} map[string]any
// @Router /api/v2/agent/tasks [get]
func (h *AgentHandler) AgentGetTasks(c *gin.Context) {
	// Only the authenticated node's own tasks; a node_id query authenticates
	// with the token or is stripped by the gateway, it never selects a node.
	nodeID, _, ok := h.authenticatedAgentNode(c)
	if !ok {
		return
	}

	tasks := []AgentTask{}

	// 取出该节点尚未送达 (status=pending) 的白名单诊断任务，HTTP 轮询作为
	// WebSocket 推送的降级路径。这些任务持久化在 DB 里，重启不丢。取出后
	// 原子标记 dispatched 防止并发重复下发。
	if h.diagnosticSvc != nil {
		diagTasks, err := h.diagnosticSvc.PullPendingTasks(nodeID)
		if err != nil {
			log.Printf("[WARN] agent tasks: load diagnostic tasks for node %d failed: %v", nodeID, err)
		}
		for i := range diagTasks {
			params := map[string]any{}
			if trimmed := diagTasks[i].Params; trimmed != "" {
				if err := json.Unmarshal([]byte(trimmed), &params); err != nil {
					log.Printf("[WARN] agent tasks: diagnostic task %s params decode failed: %v", diagTasks[i].TaskID, err)
				}
			}
			tasks = append(tasks, AgentTask{
				ID:     diagTasks[i].TaskID,
				Type:   agentDiagnosticTaskType,
				Action: diagTasks[i].Action,
				Params: params,
			})
		}
	}

	// 追加持久化的 clean_agent bridge 任务：这些任务存在 DB 里 (重启不丢)，
	// 只发给目标节点 (node_id 过滤)，取出后原子标记 dispatched 防止并发重复下发。
	tasks = append(tasks, h.pullBridgeTasks(nodeID)...)

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

// pullBridgeTasks 取出该节点尚未下发的 clean_agent bridge 任务并原子标记为 dispatched。
func (h *AgentHandler) pullBridgeTasks(nodeID uint) []AgentTask {
	tasks := []AgentTask{}
	if h.db == nil || nodeID == 0 {
		return tasks
	}

	var mappings []model.ForwardAgentBridgeTask
	if err := h.db.
		Where("node_id = ? AND status = ?", nodeID, model.ForwardAgentBridgeTaskStatusPending).
		Order("id ASC").
		Find(&mappings).Error; err != nil {
		log.Printf("[WARN] agent tasks: load bridge tasks for node %d failed: %v", nodeID, err)
		return tasks
	}

	for idx := range mappings {
		mapping := mappings[idx]
		// 原子领取：仅当仍为 pending 时才转为 dispatched，避免并发重复下发。
		result := h.db.Model(&model.ForwardAgentBridgeTask{}).
			Where("id = ? AND status = ?", mapping.ID, model.ForwardAgentBridgeTaskStatusPending).
			Updates(map[string]any{
				"status":     model.ForwardAgentBridgeTaskStatusDispatched,
				"dispatched": true,
			})
		if result.Error != nil {
			log.Printf("[WARN] agent tasks: dispatch bridge task %s failed: %v", mapping.TaskID, result.Error)
			continue
		}
		if result.RowsAffected == 0 {
			continue
		}

		params := map[string]any{}
		if trimmed := mapping.Params; trimmed != "" {
			if err := json.Unmarshal([]byte(trimmed), &params); err != nil {
				log.Printf("[WARN] agent tasks: bridge task %s params decode failed: %v", mapping.TaskID, err)
			}
		}

		tasks = append(tasks, AgentTask{
			ID:     mapping.TaskID,
			Type:   mapping.Type,
			Action: mapping.Action,
			Params: params,
		})
	}

	return tasks
}

// AgentTask 任务定义
type AgentTask = agentstreams.DiagnosticTask

// AgentReportResult godoc
// @Summary 上报任务结果
// @Description Agent 上报任务执行结果
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentTaskResult true "任务结果"
// @Success 200 {object} map[string]any
// @Router /api/v2/agent/result [post]
func (h *AgentHandler) AgentReportResult(c *gin.Context) {
	nodeID, _, ok := h.authenticatedAgentNode(c)
	if !ok {
		return
	}
	var result AgentTaskResult
	if err := c.ShouldBindJSON(&result); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if result.NodeID != 0 && result.NodeID != nodeID {
		c.JSON(http.StatusForbidden, gin.H{"error": errAgentNodeMismatch.Error()})
		return
	}
	// A node reports only its own tasks.
	result.NodeID = nodeID

	// 若该 task_id 命中持久化的 clean_agent bridge 映射，则回写 runtime job 与 forward 状态。
	// 幂等：重复上报同一已完成任务直接返回 200，不二次污染终态。
	if handled, done, err := h.completeBridgeResult(result); errors.Is(err, errAgentTaskNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if handled {
		c.JSON(http.StatusOK, gin.H{
			"message":   "received",
			"bridged":   true,
			"duplicate": done,
		})
		return
	}

	// 否则按白名单诊断任务处理，结果落库（重启不丢）。
	var taskStatus *model.AgentDiagnosticTask
	if h.diagnosticSvc != nil {
		if err := h.diagnosticSvc.CompleteTask(result.TaskID, result.NodeID, result.Action, result.Success, result.Output, result.Error, result.Duration); errors.Is(err, service.ErrAgentDiagnosticTaskOfAnotherNode) {
			c.JSON(http.StatusNotFound, gin.H{"error": errAgentTaskNotFound.Error()})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		taskStatus, _ = h.diagnosticSvc.GetTask(result.TaskID)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "received",
		"data":    taskStatus,
	})
}

// completeBridgeResult 处理 clean_agent bridge 任务的结果回写。
// 返回 (handled, alreadyDone, err)：handled=false 表示该 task_id 不是 bridge 任务
// (走原内存路径)；handled=true 表示已回写 (或幂等命中已完成态)。
func (h *AgentHandler) completeBridgeResult(result AgentTaskResult) (bool, bool, error) {
	if h.db == nil {
		return false, false, nil
	}
	svc := service.NewForwardAgentBridgeService(h.db)
	mapping, err := svc.LookupBridgeTask(result.TaskID)
	if err != nil {
		return false, false, err
	}
	if mapping == nil {
		return false, false, nil
	}
	// Another node's forward job is not this node's to complete.
	if mapping.NodeID != result.NodeID {
		return false, false, errAgentTaskNotFound
	}

	done, err := svc.CompleteJob(result.TaskID, result.Success, result.Output, result.Error)
	if err != nil {
		return true, false, err
	}
	return true, done, nil
}

// AgentTaskResult 任务结果
type AgentTaskResult struct {
	TaskID    string    `json:"task_id" binding:"required"`
	NodeID    uint      `json:"node_id,omitempty"`
	Type      string    `json:"type,omitempty"`
	Action    string    `json:"action,omitempty"`
	Success   bool      `json:"success"`
	Output    string    `json:"output"`
	Error     string    `json:"error,omitempty"`
	Data      any       `json:"data,omitempty"`
	Duration  int64     `json:"duration_ms"`
	Timestamp time.Time `json:"timestamp"`
}

// ========== 监控数据 ==========

// AgentMonitor godoc
// @Summary 上报监控数据
// @Description Agent 上报系统监控数据
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentMonitorRequest true "监控数据"
// @Success 200 {object} map[string]any
// @Router /api/v2/agent/monitor [post]
func (h *AgentHandler) AgentMonitor(c *gin.Context) {
	nodeID, isForwardNode, ok := h.authenticatedAgentNode(c)
	if !ok {
		return
	}
	var req AgentMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// A node reports only its own monitoring data.
	if req.NodeID != nodeID {
		c.JSON(http.StatusForbidden, gin.H{"error": errAgentNodeMismatch.Error()})
		return
	}

	// 保存该节点最新监控快照 (内存实时态)，供面板拉取展示。
	// 历史趋势的时序库持久化 (InfluxDB/Prometheus) 由部署侧按需接入。
	snapshot := AgentMonitorSnapshot{
		NodeID:    req.NodeID,
		System:    req.System,
		UpdatedAt: time.Now(),
	}
	h.monitorData.Store(nodeID, snapshot)
	h.markAgentSeen(nodeID, isForwardNode)

	c.JSON(http.StatusOK, gin.H{
		"message": "received",
		"data":    snapshot,
	})
}

// AgentMonitorRequest 监控请求
type AgentMonitorRequest struct {
	NodeID uint           `json:"node_id" binding:"required"`
	System map[string]any `json:"system"`
}

// AgentMonitorSnapshot stores the latest monitor payload pushed by an agent node.
type AgentMonitorSnapshot struct {
	NodeID    uint           `json:"node_id"`
	System    map[string]any `json:"system"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ========== WebSocket 连接 ==========

// AgentWebSocketUnified supports both legacy message-auth and header/query-auth.
// @Summary WebSocket 连接
// @Description Agent 建立 WebSocket 长连接
// @Tags Agent
// @Success 101
// @Router /api/v2/agent/ws [get]
func (h *AgentHandler) AgentWebSocketUnified(c *gin.Context) {
	authInfo, hasHeaderAuth, err := h.authFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	conn, err := h.wsUpgrader.Upgrade(c.Writer, c.Request, legacyAgentUpgradeHeader(c.Writer.Header()))
	if err != nil {
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("[WARN] agent ws: failed to close connection: %v", err)
		}
	}()
	if err := prepareAgentWebSocket(conn); err != nil {
		log.Printf("[WARN] agent ws: failed to prepare connection: %v", err)
		return
	}

	if !hasHeaderAuth {
		authInfo, err = h.authFromMessage(conn)
		if err != nil {
			agentConn := &AgentConnection{WsConn: conn}
			_ = writeAgentWebSocketJSON(agentConn, map[string]string{"error": err.Error()})
			return
		}
	}

	agentConn := &AgentConnection{
		NodeID:        authInfo.NodeID,
		LastSeen:      time.Now(),
		Version:       authInfo.Version,
		SystemInfo:    authInfo.System,
		Capabilities:  authInfo.Capabilities,
		IsForwardNode: authInfo.IsForwardNode,
		WsConn:        conn,
	}
	h.connections.Store(authInfo.NodeID, agentConn)
	h.touchNodeOnline(authInfo.NodeID, authInfo.IsForwardNode)
	recordWebSocketSighting(c.Request.Context(), agentConn)
	defer h.connections.Delete(authInfo.NodeID)
	defer h.failPendingAcksForNode(authInfo.NodeID, "agent websocket closed")

	if !authInfo.FromHeaders {
		_ = writeAgentWebSocketJSON(agentConn, map[string]string{"type": "auth", "message": "connected"})
	}

	for {
		msg, err := readAgentWebSocketMessage(conn)
		if err != nil {
			break
		}
		h.handleWebSocketMessage(agentConn, msg)
	}
}

// legacyAgentUpgradeHeader carries the deprecation signals the legacy agent
// path middleware (agenttransport.LegacyHTTP) set into the WebSocket
// handshake answer, which the upgrader writes itself.
func legacyAgentUpgradeHeader(header http.Header) http.Header {
	var upgrade http.Header
	for _, name := range []string{"Deprecation", "Sunset", "Link"} {
		if values := header.Values(name); len(values) > 0 {
			if upgrade == nil {
				upgrade = http.Header{}
			}
			for _, value := range values {
				upgrade.Add(name, value)
			}
		}
	}
	return upgrade
}

// recordWebSocketSighting records an agent WebSocket session in the
// transport inventory, at its start and as it talks (the recorder writes at
// most once a minute).
func recordWebSocketSighting(ctx context.Context, agentConn *AgentConnection) {
	if agentConn == nil || agentConn.NodeID == 0 {
		return
	}
	kind := agentcontrol.NodeKindProxy
	if agentConn.IsForwardNode {
		kind = agentcontrol.NodeKindForward
	}
	agenttransport.Seen(ctx, agenttransport.Sighting{
		Node:      agentcontrol.AgentNode{Kind: kind, ID: uint32(agentConn.NodeID)}, // #nosec G115 -- node ids are uint32 on every agent channel.
		Transport: model.AgentTransportWebSocket, AgentVersion: agentConn.Version, Identity: agentstreams.IdentityAPIKey,
	})
}

// ========== 管理接口 ==========

// CreateTask godoc
// @Summary 创建任务
// @Description 管理员向节点下发任务
// @Tags 管理端 Agent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateTaskRequest true "任务信息"
// @Success 200 {object} map[string]any
// @Router /admin/agent/tasks [post]
func (h *AgentHandler) CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if taskReqValue, ok := c.Get("task_request"); ok {
		taskReq, ok := taskReqValue.(CreateTaskRequest)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_request payload"})
			return
		}
		req = taskReq
	} else {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	// The task goes to the agent with its type, so the type is fixed like
	// the action: only diagnostic tasks, whatever the body says.
	if strings.TrimSpace(req.Type) != agentDiagnosticTaskType {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("task type %q is not allowed: only %q tasks are sent to agents", req.Type, agentDiagnosticTaskType)})
		return
	}
	req.Type = agentDiagnosticTaskType

	// 检查节点是否在线
	conn, ok := h.connections.Load(req.NodeID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node offline"})
		return
	}

	agentConn := conn.(*AgentConnection)
	if agentConn.WsConn == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node websocket unavailable"})
		return
	}

	// The task is kernelnodeops.RunAgentDiagnostic, the function the
	// agent.diagnostic executor runs: only whitelisted actions, params
	// normalized, the row written before the dispatch. This route carries
	// it on the node's WebSocket, with the legacy task message as the
	// fallback.
	run, err := kernelnodeops.RunAgentDiagnostic(c.Request.Context(), h.db, kernelnodeops.AgentDiagnosticRequest{
		NodeID: req.NodeID, Action: req.Action, Params: req.Params, Timeout: req.Timeout,
	}, &webSocketDiagnosticTransport{handler: h, connection: agentConn})
	if err != nil {
		var refused *kernelnodeops.DiagnosticValidationError
		if errors.As(err, &refused) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	dispatch := run.Dispatch
	if dispatch.DispatchError != nil {
		if !dispatch.LegacyFallback {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":        "send failed",
				"task_id":      run.Task.ID,
				"message_id":   dispatch.MessageID,
				"dispatch_err": dispatch.DispatchError.Error(),
				"data":         run.Row,
			})
			return
		}

		panelSuccess(c, gin.H{
			"message":      "task sent with legacy fallback",
			"task_id":      run.Task.ID,
			"node_id":      req.NodeID,
			"success":      true,
			"output":       "task dispatched (legacy fallback)",
			"duration_ms":  int64(0),
			"message_id":   dispatch.MessageID,
			"ack_received": false,
			"dispatch_err": dispatch.DispatchError.Error(),
			"data":         run.Row,
		})
		return
	}

	panelSuccess(c, gin.H{
		"message":      "task sent",
		"task_id":      run.Task.ID,
		"node_id":      req.NodeID,
		"success":      true,
		"output":       "task dispatched",
		"duration_ms":  int64(0),
		"message_id":   dispatch.MessageID,
		"ack_received": true,
		"ack":          dispatch.RawAck,
		"data":         run.Row,
	})
}

// webSocketDiagnosticTransport carries a diagnostic task on a node's
// WebSocket: task.assign with an acknowledgement, then the legacy task
// message when the acknowledgement fails.
type webSocketDiagnosticTransport struct {
	handler    *AgentHandler
	connection *AgentConnection
}

func (t *webSocketDiagnosticTransport) DispatchDiagnostic(_ context.Context, task agentstreams.DiagnosticTask) agentstreams.DiagnosticDispatch {
	messageID, ack, err := t.handler.dispatchWithAckRetry(t.connection, "task.assign", map[string]any{"task": task}, true)
	dispatch := agentstreams.DiagnosticDispatch{MessageID: messageID}
	if err != nil {
		dispatch.DispatchError = err
		if ack != nil {
			dispatch.RawAck = ack
			dispatch.Ack = agentstreams.Ack{Accepted: ack.Success, Error: ack.Error, AcceptedAt: time.Unix(ack.Timestamp, 0)}
		}
		if fallbackErr := t.handler.sendLegacyTask(t.connection, task); fallbackErr != nil {
			dispatch.FallbackError = fallbackErr
			return dispatch
		}
		dispatch.LegacyFallback = true
		return dispatch
	}
	dispatch.AckReceived = true
	dispatch.RawAck = ack
	dispatch.Ack = agentstreams.Ack{Accepted: true, AcceptedAt: time.Unix(ack.Timestamp, 0)}
	return dispatch
}

// WebSockets is this handler's connected agents as the KernelNodeOps
// session RPCs and the agent.diagnostic executor read them
// (agentstreams.WebSockets).
func (h *AgentHandler) WebSockets() agentstreams.WebSockets {
	return agentWebSockets{handler: h}
}

type agentWebSockets struct {
	handler *AgentHandler
}

func (w agentWebSockets) Sessions() []agentstreams.Session {
	var sessions []agentstreams.Session
	w.handler.connections.Range(func(_, value any) bool {
		conn, ok := value.(*AgentConnection)
		if !ok {
			return true
		}
		kind := agentcontrol.NodeKindProxy
		if conn.IsForwardNode {
			kind = agentcontrol.NodeKindForward
		}
		sessions = append(sessions, agentstreams.Session{
			Node: agentcontrol.AgentNode{Kind: kind, ID: uint32(conn.NodeID)}, Transport: agentstreams.TransportWebSocket, // #nosec G115 -- node ids are 32-bit.
			AgentVersion: conn.Version, Capabilities: append([]string(nil), conn.Capabilities...), LastSeen: conn.LastSeen,
			System: conn.SystemInfo, Identity: agentstreams.IdentityAPIKey,
		})
		return true
	})
	return sessions
}

func (w agentWebSockets) Monitor(nodeID uint) (map[string]any, time.Time, bool) {
	value, ok := w.handler.monitorData.Load(nodeID)
	if !ok {
		return nil, time.Time{}, false
	}
	snapshot, ok := value.(AgentMonitorSnapshot)
	if !ok {
		return nil, time.Time{}, false
	}
	return snapshot.System, snapshot.UpdatedAt, true
}

func (w agentWebSockets) DiagnosticTransport(nodeID uint) (agentstreams.DiagnosticTransport, bool) {
	value, ok := w.handler.connections.Load(nodeID)
	if !ok {
		return nil, false
	}
	conn, ok := value.(*AgentConnection)
	if !ok || conn.WsConn == nil {
		return nil, false
	}
	return &webSocketDiagnosticTransport{handler: w.handler, connection: conn}, true
}

// CreateTaskRequest 创建任务请求
type CreateTaskRequest struct {
	NodeID  uint           `json:"node_id" binding:"required"`
	Type    string         `json:"type" binding:"required"`
	Action  string         `json:"action" binding:"required"`
	Params  map[string]any `json:"params"`
	Timeout int            `json:"timeout"`
}

// ListAgents godoc
// @Summary 获取在线 Agent 列表
// @Description 管理员获取所有在线的 Agent
// @Tags 管理端 Agent
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /admin/agent/list [get]
func (h *AgentHandler) ListAgents(c *gin.Context) {
	agents := make([]map[string]any, 0)

	h.connections.Range(func(key, value any) bool {
		conn := value.(*AgentConnection)
		agents = append(agents, map[string]any{
			"node_id":      conn.NodeID,
			"last_seen":    conn.LastSeen,
			"version":      conn.Version,
			"system":       conn.SystemInfo,
			"capabilities": conn.Capabilities,
			"online":       time.Since(conn.LastSeen) < 60*time.Second,
		})
		return true
	})

	panelSuccess(c, gin.H{"agents": agents})
}

// GetTaskResult godoc
// @Summary Get task execution result
// @Description Query the latest status/result for a task by task_id
// @Tags 管理端 Agent
// @Produce json
// @Security BearerAuth
// @Param task_id path string true "Task ID"
// @Success 200 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /admin/agent/tasks/{task_id} [get]
func (h *AgentHandler) GetTaskResult(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	if h.diagnosticSvc == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task result not found"})
		return
	}

	taskRow, err := h.diagnosticSvc.GetTask(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task result not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	panelSuccess(c, taskRow)
}

// ListDiagnosticTasks godoc
// @Summary List diagnostic task history
// @Description Query recent whitelisted diagnostic tasks, optionally filtered by node_id
// @Tags 管理端 Agent
// @Produce json
// @Security BearerAuth
// @Param node_id query int false "Node ID"
// @Param limit query int false "Limit"
// @Success 200 {object} map[string]any
// @Router /admin/agent/tasks [get]
func (h *AgentHandler) ListDiagnosticTasks(c *gin.Context) {
	nodeID, _ := strconv.Atoi(c.Query("node_id"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	if h.diagnosticSvc == nil {
		panelSuccess(c, []model.AgentDiagnosticTask{})
		return
	}

	tasks, err := h.diagnosticSvc.ListTasks(uint(nodeID), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	panelSuccess(c, tasks)
}

// GetMonitor godoc
// @Summary Get latest monitor payload from agent node
// @Description Query monitor data by node_id
// @Tags 管理端 Agent
// @Produce json
// @Security BearerAuth
// @Param node_id query int true "Node ID"
// @Success 200 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /admin/agent/monitor [get]
func (h *AgentHandler) GetMonitor(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Query("node_id"), 10, 32)
	if err != nil || nodeID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid node_id is required"})
		return
	}

	snapshot, ok := h.monitorData.Load(uint(nodeID))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "monitor data not found"})
		return
	}

	monitor, ok := snapshot.(AgentMonitorSnapshot)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid monitor state"})
		return
	}

	panelSuccess(c, monitor)
}

// ExecuteCommand godoc
// @Summary 执行命令
// @Description 管理员在节点上执行命令
// @Tags 管理端 Agent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body ExecuteCommandRequest true "命令信息"
// @Success 200 {object} map[string]any
// @Router /admin/agent/execute [post]
func (h *AgentHandler) ExecuteCommand(c *gin.Context) {
	var req ExecuteCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// ExecuteCommand 不再接受任意命令字符串，改为白名单诊断动作
	taskReq := CreateTaskRequest{
		NodeID:  req.NodeID,
		Type:    agentDiagnosticTaskType,
		Action:  req.Action,
		Params:  req.Params,
		Timeout: req.Timeout,
	}

	// 复用 CreateTask 逻辑
	c.Set("task_request", taskReq)
	h.CreateTask(c)
}

// ExecuteCommandRequest 白名单诊断动作请求
type ExecuteCommandRequest struct {
	NodeID  uint           `json:"node_id" binding:"required"`
	Action  string         `json:"action" binding:"required"`
	Params  map[string]any `json:"params"`
	Timeout int            `json:"timeout"`
}

// ========== 转发规则同步 ==========

// AgentGetForwardRules godoc
// @Summary 获取转发规则
// @Description Agent 获取该节点的转发规则
// @Tags Agent
// @Produce json
// @Param node_id query int true "节点 ID"
// @Param X-API-Key header string true "forward node API token"
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Router /api/v2/forward/agent/rules [get]
//
// Only a forward node reads its rules: the request names the node
// (node_id, or the X-Node-ID header) and carries its API token (the
// X-API-Key header, or api_key or token), as the agent's other calls do.
// The route is public, and the rules show where every user's traffic goes.
func (h *AgentHandler) AgentGetForwardRules(c *gin.Context) {
	nodeID, err := h.authenticateForwardNodeRequest(c)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, errAgentNotForwardNode) {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	var rules []*model.ForwardRule
	h.db.Where("relay_node_id = ? OR exit_node_id = ?", nodeID, nodeID).
		Preload("RelayNode").
		Preload("ExitNode").
		Find(&rules)

	// 转换为 Agent 需要的格式
	var agentRules []ForwardRuleForAgent
	for _, r := range rules {
		agentRules = append(agentRules, ForwardRuleForAgent{
			ID:         r.ID,
			Enabled:    r.Enabled,
			ListenPort: r.ListenPort,
			Protocol:   r.Protocol,
			TargetHost: r.TargetHost,
			TargetPort: r.TargetPort,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": agentRules})
}

// errAgentNotForwardNode refuses a proxy node's credentials where only a
// forward node may act.
var errAgentNotForwardNode = errors.New("not a forward node")

// authenticateForwardNodeRequest returns the forward node a REST request
// authenticates as: the node it names and whose API token it carries. An
// unknown node and a wrong token are the same error, so the answer does not
// tell which nodes exist.
func (h *AgentHandler) authenticateForwardNodeRequest(c *gin.Context) (uint, error) {
	rawNodeID := c.Query("node_id")
	if rawNodeID == "" {
		rawNodeID = c.GetHeader("X-Node-ID")
	}
	token := c.GetHeader("X-API-Key")
	if token == "" {
		token = c.Query("api_key")
	}
	if token == "" {
		token = c.Query("token")
	}
	nodeID, err := strconv.ParseUint(rawNodeID, 10, 32)
	if err != nil || nodeID == 0 || token == "" {
		return 0, errors.New("missing node_id or token")
	}
	isForwardNode, err := h.verifyForwardNodeToken(uint(nodeID), token)
	if err != nil {
		return 0, errAgentInvalidToken
	}
	if !isForwardNode {
		return 0, errAgentNotForwardNode
	}
	return uint(nodeID), nil
}

// ForwardRuleForAgent 转发规则（Agent 格式）
type ForwardRuleForAgent struct {
	ID         uint   `json:"id"`
	Enabled    bool   `json:"enabled"`
	ListenPort int    `json:"listen_port"`
	Protocol   string `json:"protocol"`
	TargetHost string `json:"target_host"`
	TargetPort int    `json:"target_port"`
}

// ========== 辅助函数 ==========

func generateMessageID() string {
	return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}
