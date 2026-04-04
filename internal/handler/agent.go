package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

// AgentHandler Agent 绠＄悊 API
type AgentHandler struct {
	db          *gorm.DB
	connections sync.Map // nodeID -> *AgentConnection
	pendingAcks sync.Map // messageID -> *wsPendingAck
	taskResults sync.Map // taskID -> AgentTaskStatus
	monitorData sync.Map // nodeID -> AgentMonitorSnapshot
	wsUpgrader  websocket.Upgrader
	ackTimeout  time.Duration
	maxRetries  int
}

// AgentConnection Agent 杩炴帴淇℃伅
type AgentConnection struct {
	NodeID       uint
	LastSeen     time.Time
	Version      string
	SystemInfo   map[string]interface{}
	Capabilities []string
	WsConn       *websocket.Conn
	writeMu      sync.Mutex
}

type wsAuthInfo struct {
	NodeID       uint
	Version      string
	System       map[string]interface{}
	Capabilities []string
	FromHeaders  bool
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
	ID         string      `json:"id"`
	Type       string      `json:"type"`
	NodeID     uint        `json:"node_id"`
	Timestamp  int64       `json:"timestamp"`
	Payload    interface{} `json:"payload,omitempty"`
	RequireAck bool        `json:"require_ack,omitempty"`
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
	errAgentNodeNotFound = errors.New("node not found")
	errAgentInvalidToken = errors.New("invalid token")
)

// NewAgentHandler 鍒涘缓 Handler
func NewAgentHandler() *AgentHandler {
	return &AgentHandler{
		db: database.Get(),
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		ackTimeout: 3 * time.Second,
		maxRetries: 2,
	}
}

func (h *AgentHandler) verifyForwardNodeToken(nodeID uint, token string) error {
	var node model.ForwardNode
	if err := h.db.First(&node, nodeID).Error; err != nil {
		return errAgentNodeNotFound
	}
	if node.APIToken != token {
		return errAgentInvalidToken
	}
	return nil
}

func (h *AgentHandler) markNodeOnline(nodeID uint) {
	h.db.Model(&model.ForwardNode{}).Where("id = ?", nodeID).Updates(map[string]interface{}{
		"status":     model.ForwardNodeStatusOnline,
		"last_check": time.Now(),
	})
}

func (h *AgentHandler) updateConnectionLastSeen(nodeID uint) {
	if conn, ok := h.connections.Load(nodeID); ok {
		ac := conn.(*AgentConnection)
		ac.LastSeen = time.Now()
	}
	h.markNodeOnline(nodeID)
}

func (h *AgentHandler) authFromRequest(c *gin.Context) (*wsAuthInfo, bool, error) {
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
	if err := h.verifyForwardNodeToken(nodeID, token); err != nil {
		return nil, true, err
	}

	return &wsAuthInfo{
		NodeID:      nodeID,
		FromHeaders: true,
	}, true, nil
}

func (h *AgentHandler) authFromMessage(conn *websocket.Conn) (*wsAuthInfo, error) {
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var authMsg struct {
		Type         string                 `json:"type"`
		NodeID       uint                   `json:"node_id"`
		Token        string                 `json:"token"`
		Version      string                 `json:"version"`
		System       map[string]interface{} `json:"system"`
		Capabilities []string               `json:"capabilities"`
	}
	if err := json.Unmarshal(msg, &authMsg); err != nil || authMsg.Type != "auth" {
		return nil, fmt.Errorf("invalid auth")
	}
	if err := h.verifyForwardNodeToken(authMsg.NodeID, authMsg.Token); err != nil {
		return nil, err
	}

	return &wsAuthInfo{
		NodeID:       authMsg.NodeID,
		Version:      authMsg.Version,
		System:       authMsg.System,
		Capabilities: authMsg.Capabilities,
		FromHeaders:  false,
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
	if agentConn == nil || agentConn.WsConn == nil {
		return fmt.Errorf("ws connection not available")
	}
	agentConn.writeMu.Lock()
	defer agentConn.writeMu.Unlock()
	return agentConn.WsConn.WriteJSON(envelope)
}

func (h *AgentHandler) sendLegacyTask(agentConn *AgentConnection, task AgentTask) error {
	if agentConn == nil || agentConn.WsConn == nil {
		return fmt.Errorf("ws connection not available")
	}
	agentConn.writeMu.Lock()
	defer agentConn.writeMu.Unlock()
	return agentConn.WsConn.WriteJSON(map[string]interface{}{
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

func (h *AgentHandler) dispatchWithAckRetry(agentConn *AgentConnection, messageType string, payload interface{}, requireAck bool) (string, *wsAckPayload, error) {
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
	h.pendingAcks.Range(func(key, value interface{}) bool {
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
		h.markNodeOnline(agentConn.NodeID)
	}

	if envelope.RequireAck && envelope.ID != "" {
		h.sendAck(agentConn, envelope.ID, nil)
	}
}

// ========== Agent 璁よ瘉鍜屾敞鍐?==========

// AgentRegister godoc
// @Summary Agent 娉ㄥ唽
// @Description Agent 鍚姩鏃舵敞鍐屽埌闈㈡澘
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentRegisterRequest true "娉ㄥ唽淇℃伅"
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/agent/register [post]
func (h *AgentHandler) AgentRegister(c *gin.Context) {
	var req AgentRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 楠岃瘉 Token
	if err := h.verifyForwardNodeToken(req.NodeID, req.Token); err != nil {
		if errors.Is(err, errAgentInvalidToken) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 璁板綍杩炴帴
	conn := &AgentConnection{
		NodeID:       req.NodeID,
		LastSeen:     time.Now(),
		Version:      req.Version,
		SystemInfo:   req.System,
		Capabilities: req.Capabilities,
	}
	h.connections.Store(req.NodeID, conn)
	h.markNodeOnline(req.NodeID)

	// 鏇存柊鑺傜偣鐘舵€?	h.markNodeOnline(req.NodeID)

	c.JSON(http.StatusOK, gin.H{
		"message": "registered",
		"node_id": req.NodeID,
	})
}

// AgentRegisterRequest 娉ㄥ唽璇锋眰
type AgentRegisterRequest struct {
	NodeID       uint                   `json:"node_id"`
	Token        string                 `json:"token"`
	Version      string                 `json:"version"`
	System       map[string]interface{} `json:"system"`
	Capabilities []string               `json:"capabilities"`
}

// ========== Agent 蹇冭烦 ==========

// AgentHeartbeat godoc
// @Summary Agent 蹇冭烦
// @Description Agent 瀹氭湡鍙戦€佸績璺?// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentHeartbeatRequest true "蹇冭烦淇℃伅"
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/agent/heartbeat [post]
func (h *AgentHandler) AgentHeartbeat(c *gin.Context) {
	var req AgentHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 鏇存柊杩炴帴鏃堕棿
	h.updateConnectionLastSeen(req.NodeID)

	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// AgentHeartbeatRequest 蹇冭烦璇锋眰
type AgentHeartbeatRequest struct {
	NodeID    uint                   `json:"node_id"`
	Status    string                 `json:"status"`
	Resources map[string]interface{} `json:"resources"`
}

// ========== 浠诲姟绠＄悊 ==========

// AgentGetTasks godoc
// @Summary 鑾峰彇寰呮墽琛屼换鍔?// @Description Agent 杞鑾峰彇寰呮墽琛岀殑浠诲姟
// @Tags Agent
// @Produce json
// @Param node_id query int true "鑺傜偣 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/agent/tasks [get]
func (h *AgentHandler) AgentGetTasks(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Query("node_id"), 10, 32)
	_ = nodeID // TODO: 浣跨敤 nodeID 杩囨护浠诲姟

	// TODO: 浠庝换鍔￠槦鍒楄幏鍙栬鑺傜偣鐨勫緟鎵ц浠诲姟
	// 杩欓噷绠€鍖栧疄鐜帮紝杩斿洖绌轰换鍔″垪琛?	tasks := []AgentTask{}

	tasks := []AgentTask{}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

// AgentTask 浠诲姟瀹氫箟
type AgentTask struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Action  string                 `json:"action"`
	Params  map[string]interface{} `json:"params"`
	Timeout int                    `json:"timeout"`
}

// AgentTaskStatus stores the latest status/result for a dispatched task.
type AgentTaskStatus struct {
	TaskID      string                 `json:"task_id"`
	NodeID      uint                   `json:"node_id"`
	Type        string                 `json:"type,omitempty"`
	Action      string                 `json:"action,omitempty"`
	Params      map[string]interface{} `json:"params,omitempty"`
	Timeout     int                    `json:"timeout,omitempty"`
	Status      string                 `json:"status"`
	MessageID   string                 `json:"message_id,omitempty"`
	AckReceived bool                   `json:"ack_received"`
	Success     bool                   `json:"success"`
	Output      string                 `json:"output,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Data        interface{}            `json:"data,omitempty"`
	DurationMS  int64                  `json:"duration_ms,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// AgentReportResult godoc
// @Summary 涓婃姤浠诲姟缁撴灉
// @Description Agent 涓婃姤浠诲姟鎵ц缁撴灉
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentTaskResult true "浠诲姟缁撴灉"
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/agent/result [post]
func (h *AgentHandler) AgentReportResult(c *gin.Context) {
	var result AgentTaskResult
	if err := c.ShouldBindJSON(&result); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: 淇濆瓨浠诲姟缁撴灉鍒版暟鎹簱
	// TODO: 濡傛灉鏈夊洖璋冿紝瑙﹀彂鍥炶皟

	now := time.Now()
	snapshot, _ := h.taskResults.Load(result.TaskID)
	taskStatus, ok := snapshot.(AgentTaskStatus)
	if !ok {
		taskStatus = AgentTaskStatus{
			TaskID:  result.TaskID,
			Status:  "completed",
			Success: result.Success,
		}
	}

	if result.NodeID != 0 {
		taskStatus.NodeID = result.NodeID
	}
	if result.Type != "" {
		taskStatus.Type = result.Type
	}
	if result.Action != "" {
		taskStatus.Action = result.Action
	}

	taskStatus.Success = result.Success
	taskStatus.Output = result.Output
	taskStatus.Error = result.Error
	taskStatus.Data = result.Data
	taskStatus.DurationMS = result.Duration
	taskStatus.Status = "completed"
	taskStatus.UpdatedAt = now
	if result.Timestamp.IsZero() {
		taskStatus.Timestamp = now
	} else {
		taskStatus.Timestamp = result.Timestamp
	}

	h.taskResults.Store(result.TaskID, taskStatus)

	c.JSON(http.StatusOK, gin.H{
		"message": "received",
		"data":    taskStatus,
	})
}

// AgentTaskResult 浠诲姟缁撴灉
type AgentTaskResult struct {
	TaskID    string      `json:"task_id" binding:"required"`
	NodeID    uint        `json:"node_id,omitempty"`
	Type      string      `json:"type,omitempty"`
	Action    string      `json:"action,omitempty"`
	Success   bool        `json:"success"`
	Output    string      `json:"output"`
	Error     string      `json:"error,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Duration  int64       `json:"duration_ms"`
	Timestamp time.Time   `json:"timestamp"`
}

// ========== 鐩戞帶鏁版嵁 ==========

// AgentMonitor godoc
// @Summary 涓婃姤鐩戞帶鏁版嵁
// @Description Agent 涓婃姤绯荤粺鐩戞帶鏁版嵁
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentMonitorRequest true "鐩戞帶鏁版嵁"
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/agent/monitor [post]
func (h *AgentHandler) AgentMonitor(c *gin.Context) {
	var req AgentMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: 淇濆瓨鐩戞帶鏁版嵁鍒版椂搴忔暟鎹簱

	snapshot := AgentMonitorSnapshot{
		NodeID:    req.NodeID,
		System:    req.System,
		UpdatedAt: time.Now(),
	}
	h.monitorData.Store(req.NodeID, snapshot)
	h.updateConnectionLastSeen(req.NodeID)

	c.JSON(http.StatusOK, gin.H{
		"message": "received",
		"data":    snapshot,
	})
}

// AgentMonitorRequest 鐩戞帶璇锋眰
type AgentMonitorRequest struct {
	NodeID uint                   `json:"node_id" binding:"required"`
	System map[string]interface{} `json:"system"`
}

// AgentMonitorSnapshot stores the latest monitor payload pushed by an agent node.
type AgentMonitorSnapshot struct {
	NodeID    uint                   `json:"node_id"`
	System    map[string]interface{} `json:"system"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// ========== WebSocket 杩炴帴 ==========

// AgentWebSocket godoc
// @Summary WebSocket 杩炴帴
// @Description Agent 寤虹珛 WebSocket 闀胯繛鎺?// @Tags Agent
// @Success 101
// @Router /api/v2/agent/ws [get]
func (h *AgentHandler) AgentWebSocket(c *gin.Context) {
	conn, err := h.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 绛夊緟璁よ瘉娑堟伅
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return
	}

	var authMsg struct {
		Type   string `json:"type"`
		NodeID uint   `json:"node_id"`
		Token  string `json:"token"`
	}
	if err := json.Unmarshal(msg, &authMsg); err != nil || authMsg.Type != "auth" {
		conn.WriteJSON(map[string]string{"error": "invalid auth"})
		return
	}

	// 楠岃瘉 Token
	var node model.ForwardNode
	if err := h.db.First(&node, authMsg.NodeID).Error; err != nil {
		conn.WriteJSON(map[string]string{"error": "node not found"})
		return
	}
	if node.APIToken != authMsg.Token {
		conn.WriteJSON(map[string]string{"error": "invalid token"})
		return
	}

	// 璁板綍 WebSocket 杩炴帴
	agentConn := &AgentConnection{
		NodeID:   authMsg.NodeID,
		LastSeen: time.Now(),
		WsConn:   conn,
	}
	h.connections.Store(authMsg.NodeID, agentConn)

	conn.WriteJSON(map[string]string{"type": "auth", "message": "connected"})

	// 澶勭悊娑堟伅寰幆
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		// 澶勭悊娑堟伅
		var m map[string]interface{}
		json.Unmarshal(msg, &m)
		// TODO: 澶勭悊涓嶅悓绫诲瀷鐨勬秷鎭?		_ = m
	}

	// 娓呯悊杩炴帴
	h.connections.Delete(authMsg.NodeID)
}

// AgentWebSocketUnified supports both legacy message-auth and header/query-auth.
func (h *AgentHandler) AgentWebSocketUnified(c *gin.Context) {
	authInfo, hasHeaderAuth, err := h.authFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	conn, err := h.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	if !hasHeaderAuth {
		authInfo, err = h.authFromMessage(conn)
		if err != nil {
			_ = conn.WriteJSON(map[string]string{"error": err.Error()})
			return
		}
	}

	agentConn := &AgentConnection{
		NodeID:       authInfo.NodeID,
		LastSeen:     time.Now(),
		Version:      authInfo.Version,
		SystemInfo:   authInfo.System,
		Capabilities: authInfo.Capabilities,
		WsConn:       conn,
	}
	h.connections.Store(authInfo.NodeID, agentConn)
	h.markNodeOnline(authInfo.NodeID)
	defer h.connections.Delete(authInfo.NodeID)
	defer h.failPendingAcksForNode(authInfo.NodeID, "agent websocket closed")

	if !authInfo.FromHeaders {
		_ = conn.WriteJSON(map[string]string{"type": "auth", "message": "connected"})
	}

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		h.handleWebSocketMessage(agentConn, msg)
	}
}

// ========== 绠＄悊鎺ュ彛 ==========

// CreateTask godoc
// @Summary 鍒涘缓浠诲姟
// @Description 绠＄悊鍛樺悜鑺傜偣涓嬪彂浠诲姟
// @Tags 绠＄悊绔?Agent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateTaskRequest true "浠诲姟淇℃伅"
// @Success 200 {object} map[string]interface{}
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

	// 妫€鏌ヨ妭鐐规槸鍚﹀湪绾?
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

	task := AgentTask{
		ID:      generateTaskID(),
		Type:    req.Type,
		Action:  req.Action,
		Params:  req.Params,
		Timeout: req.Timeout,
	}

	taskStatus := AgentTaskStatus{
		TaskID:    task.ID,
		NodeID:    req.NodeID,
		Type:      req.Type,
		Action:    req.Action,
		Params:    req.Params,
		Timeout:   req.Timeout,
		Status:    "pending",
		Success:   false,
		Timestamp: time.Now(),
		UpdatedAt: time.Now(),
	}
	h.taskResults.Store(task.ID, taskStatus)

	messageID, ack, err := h.dispatchWithAckRetry(agentConn, "task.assign", map[string]interface{}{
		"task": task,
	}, true)
	if err != nil {
		if fallbackErr := h.sendLegacyTask(agentConn, task); fallbackErr != nil {
			taskStatus.Status = "failed"
			taskStatus.Error = "send failed: " + fallbackErr.Error()
			taskStatus.MessageID = messageID
			taskStatus.UpdatedAt = time.Now()
			h.taskResults.Store(task.ID, taskStatus)

			c.JSON(http.StatusInternalServerError, gin.H{
				"error":        "send failed",
				"task_id":      task.ID,
				"message_id":   messageID,
				"dispatch_err": err.Error(),
				"data":         taskStatus,
			})
			return
		}

		taskStatus.Status = "dispatched"
		taskStatus.MessageID = messageID
		taskStatus.AckReceived = false
		taskStatus.Success = true
		taskStatus.Output = "task dispatched (legacy fallback)"
		taskStatus.UpdatedAt = time.Now()
		h.taskResults.Store(task.ID, taskStatus)

		c.JSON(http.StatusOK, gin.H{
			"message":      "task sent with legacy fallback",
			"task_id":      task.ID,
			"node_id":      req.NodeID,
			"success":      true,
			"output":       taskStatus.Output,
			"duration_ms":  int64(0),
			"message_id":   messageID,
			"ack_received": false,
			"dispatch_err": err.Error(),
			"data":         taskStatus,
		})
		return
	}

	taskStatus.Status = "dispatched"
	taskStatus.MessageID = messageID
	taskStatus.AckReceived = true
	taskStatus.Success = true
	taskStatus.Output = "task dispatched"
	taskStatus.UpdatedAt = time.Now()
	h.taskResults.Store(task.ID, taskStatus)

	c.JSON(http.StatusOK, gin.H{
		"message":      "task sent",
		"task_id":      task.ID,
		"node_id":      req.NodeID,
		"success":      true,
		"output":       taskStatus.Output,
		"duration_ms":  int64(0),
		"message_id":   messageID,
		"ack_received": true,
		"ack":          ack,
		"data":         taskStatus,
	})
}

// CreateTaskRequest 鍒涘缓浠诲姟璇锋眰
type CreateTaskRequest struct {
	NodeID  uint                   `json:"node_id" binding:"required"`
	Type    string                 `json:"type" binding:"required"`
	Action  string                 `json:"action" binding:"required"`
	Params  map[string]interface{} `json:"params"`
	Timeout int                    `json:"timeout"`
}

// ListAgents godoc
// @Summary 鑾峰彇鍦ㄧ嚎 Agent 鍒楄〃
// @Description 绠＄悊鍛樿幏鍙栨墍鏈夊湪绾跨殑 Agent
// @Tags 绠＄悊绔?Agent
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /admin/agent/list [get]
func (h *AgentHandler) ListAgents(c *gin.Context) {
	var agents []map[string]interface{}

	h.connections.Range(func(key, value interface{}) bool {
		conn := value.(*AgentConnection)
		agents = append(agents, map[string]interface{}{
			"node_id":      conn.NodeID,
			"last_seen":    conn.LastSeen,
			"version":      conn.Version,
			"system":       conn.SystemInfo,
			"capabilities": conn.Capabilities,
			"online":       time.Since(conn.LastSeen) < 60*time.Second,
		})
		return true
	})

	c.JSON(http.StatusOK, gin.H{
		"agents": agents,
		"data":   gin.H{"agents": agents},
	})
}

// GetTaskResult godoc
// @Summary Get task execution result
// @Description Query the latest status/result for a task by task_id
// @Tags 管理端 Agent
// @Produce json
// @Security BearerAuth
// @Param task_id path string true "Task ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/agent/tasks/{task_id} [get]
func (h *AgentHandler) GetTaskResult(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	snapshot, ok := h.taskResults.Load(taskID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "task result not found"})
		return
	}

	taskStatus, ok := snapshot.(AgentTaskStatus)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid task result state"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": taskStatus})
}

// GetMonitor godoc
// @Summary Get latest monitor payload from agent node
// @Description Query monitor data by node_id
// @Tags 管理端 Agent
// @Produce json
// @Security BearerAuth
// @Param node_id query int true "Node ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
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

	c.JSON(http.StatusOK, gin.H{"data": monitor})
}

// ExecuteCommand godoc
// @Summary 鎵ц鍛戒护
// @Description 绠＄悊鍛樺湪鑺傜偣涓婃墽琛屽懡浠?// @Tags 绠＄悊绔?Agent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body ExecuteCommandRequest true "鍛戒护淇℃伅"
// @Success 200 {object} map[string]interface{}
// @Router /admin/agent/execute [post]
func (h *AgentHandler) ExecuteCommand(c *gin.Context) {
	var req ExecuteCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 鍒涘缓鍛戒护浠诲姟
	taskReq := CreateTaskRequest{
		NodeID: req.NodeID,
		Type:   "command",
		Action: req.Command,
		Params: map[string]interface{}{
			"args": req.Args,
			"env":  req.Env,
		},
		Timeout: req.Timeout,
	}

	// 澶嶇敤 CreateTask 閫昏緫
	c.Set("task_request", taskReq)
	h.CreateTask(c)
}

// ExecuteCommandRequest 鎵ц鍛戒护璇锋眰
type ExecuteCommandRequest struct {
	NodeID  uint                   `json:"node_id" binding:"required"`
	Command string                 `json:"command" binding:"required"`
	Args    []string               `json:"args"`
	Env     map[string]interface{} `json:"env"`
	Timeout int                    `json:"timeout"`
}

// ========== 杞彂瑙勫垯鍚屾 ==========

// AgentGetForwardRules godoc
// @Summary 鑾峰彇杞彂瑙勫垯
// @Description Agent 鑾峰彇璇ヨ妭鐐圭殑杞彂瑙勫垯
// @Tags Agent
// @Produce json
// @Param node_id query int true "鑺傜偣 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/forward/agent/rules [get]
func (h *AgentHandler) AgentGetForwardRules(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Query("node_id"), 10, 32)

	var rules []*model.ForwardRule
	h.db.Where("relay_node_id = ? OR exit_node_id = ?", nodeID, nodeID).
		Preload("RelayNode").
		Preload("ExitNode").
		Find(&rules)

	// 杞崲涓?Agent 闇€瑕佺殑鏍煎紡
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

// ForwardRuleForAgent 杞彂瑙勫垯锛圓gent 鏍煎紡锛?
type ForwardRuleForAgent struct {
	ID         uint   `json:"id"`
	Enabled    bool   `json:"enabled"`
	ListenPort int    `json:"listen_port"`
	Protocol   string `json:"protocol"`
	TargetHost string `json:"target_host"`
	TargetPort int    `json:"target_port"`
}

// ========== 杈呭姪鍑芥暟 ==========

func generateTaskID() string {
	return fmt.Sprintf("task-%d", time.Now().UnixNano())
}

func generateMessageID() string {
	return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}
