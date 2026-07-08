package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/anixops/v2board/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

var (
	agentWSReadTimeout  = 60 * time.Second
	agentWSWriteTimeout = 10 * time.Second
)

// AgentHandler Agent 绠＄悊 API
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

// AgentConnection Agent 杩炴帴淇℃伅
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
	errAgentNodeNotFound = errors.New("node not found")
	errAgentInvalidToken = errors.New("invalid token")
)

// hashString hashes a token with SHA-256, matching the api_key_hash contract
// used by the UniProxy node-auth middleware and node registration.
func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// NewAgentHandler 鍒涘缓 Handler
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
func (h *AgentHandler) verifyForwardNodeToken(nodeID uint, token string) (isForwardNode bool, err error) {
	var fwd model.ForwardNode
	if fwdErr := h.db.First(&fwd, nodeID).Error; fwdErr == nil {
		if fwd.APIToken != token {
			return true, errAgentInvalidToken
		}
		return true, nil
	}

	var node model.Node
	if nodeErr := h.db.First(&node, nodeID).Error; nodeErr != nil {
		return false, errAgentNodeNotFound
	}

	tokenHash := hashString(token)
	valid := node.APIKeyHash == tokenHash || (node.APIKeyHash == "" && node.APIKey == token)
	if !valid {
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

// touchNodeOnline marks a node online in the table that owns it.
func (h *AgentHandler) touchNodeOnline(nodeID uint, isForwardNode bool) {
	if isForwardNode {
		h.db.Model(&model.ForwardNode{}).Where("id = ?", nodeID).Updates(map[string]any{
			"status":     model.ForwardNodeStatusOnline,
			"last_check": time.Now(),
		})
		return
	}
	h.db.Model(&model.Node{}).Where("id = ?", nodeID).Updates(map[string]any{
		"status":        model.NodeStatusOnline,
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

// ========== Agent 璁よ瘉鍜屾敞鍐?==========

// AgentRegister godoc
// @Summary Agent 娉ㄥ唽
// @Description Agent 鍚姩鏃舵敞鍐屽埌闈㈡澘
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentRegisterRequest true "娉ㄥ唽淇℃伅"
// @Success 200 {object} map[string]any
// @Router /api/v2/agent/register [post]
func (h *AgentHandler) AgentRegister(c *gin.Context) {
	var req AgentRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 楠岃瘉 Token
	isForwardNode, err := h.verifyForwardNodeToken(req.NodeID, req.Token)
	if err != nil {
		if errors.Is(err, errAgentInvalidToken) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 璁板綍杩炴帴
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

// AgentRegisterRequest 娉ㄥ唽璇锋眰
type AgentRegisterRequest struct {
	NodeID       uint           `json:"node_id" binding:"required,gt=0"`
	Token        string         `json:"token" binding:"required,min=1"`
	Version      string         `json:"version" binding:"omitempty,max=64"`
	System       map[string]any `json:"system"`
	Capabilities []string       `json:"capabilities"`
}

// ========== Agent 蹇冭烦 ==========

// AgentHeartbeat godoc
// @Summary Agent 蹇冭烦
// @Description Agent 瀹氭湡鍙戦€佸績璺?// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentHeartbeatRequest true "蹇冭烦淇℃伅"
// @Success 200 {object} map[string]any
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
	NodeID    uint           `json:"node_id" binding:"required,gt=0"`
	Status    string         `json:"status" binding:"omitempty,max=32"`
	Resources map[string]any `json:"resources"`
}

// ========== 浠诲姟绠＄悊 ==========

// AgentGetTasks godoc
// @Summary 鑾峰彇寰呮墽琛屼换鍔?// @Description Agent 杞鑾峰彇寰呮墽琛岀殑浠诲姟
// @Tags Agent
// @Produce json
// @Param node_id query int true "鑺傜偣 ID"
// @Success 200 {object} map[string]any
// @Router /api/v2/agent/tasks [get]
func (h *AgentHandler) AgentGetTasks(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Query("node_id"), 10, 32)

	tasks := []AgentTask{}

	// 取出该节点尚未送达 (status=pending) 的白名单诊断任务，HTTP 轮询作为
	// WebSocket 推送的降级路径。这些任务持久化在 DB 里，重启不丢。取出后
	// 原子标记 dispatched 防止并发重复下发。
	if h.diagnosticSvc != nil {
		diagTasks, err := h.diagnosticSvc.PullPendingTasks(uint(nodeID))
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
				Type:   "diagnostic",
				Action: diagTasks[i].Action,
				Params: params,
			})
		}
	}

	// 追加持久化的 clean_agent bridge 任务：这些任务存在 DB 里 (重启不丢)，
	// 只发给目标节点 (node_id 过滤)，取出后原子标记 dispatched 防止并发重复下发。
	tasks = append(tasks, h.pullBridgeTasks(uint(nodeID))...)

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

// AgentTask 浠诲姟瀹氫箟
type AgentTask struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Action  string         `json:"action"`
	Params  map[string]any `json:"params"`
	Timeout int            `json:"timeout"`
}

// AgentReportResult godoc
// @Summary 涓婃姤浠诲姟缁撴灉
// @Description Agent 涓婃姤浠诲姟鎵ц缁撴灉
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentTaskResult true "浠诲姟缁撴灉"
// @Success 200 {object} map[string]any
// @Router /api/v2/agent/result [post]
func (h *AgentHandler) AgentReportResult(c *gin.Context) {
	var result AgentTaskResult
	if err := c.ShouldBindJSON(&result); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 若该 task_id 命中持久化的 clean_agent bridge 映射，则回写 runtime job 与 forward 状态。
	// 幂等：重复上报同一已完成任务直接返回 200，不二次污染终态。
	if handled, done, err := h.completeBridgeResult(result); err != nil {
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
		if err := h.diagnosticSvc.CompleteTask(result.TaskID, result.NodeID, result.Action, result.Success, result.Output, result.Error, result.Duration); err != nil {
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

	done, err := svc.CompleteJob(result.TaskID, result.Success, result.Output, result.Error)
	if err != nil {
		return true, false, err
	}
	return true, done, nil
}

// AgentTaskResult 浠诲姟缁撴灉
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

// ========== 鐩戞帶鏁版嵁 ==========

// AgentMonitor godoc
// @Summary 涓婃姤鐩戞帶鏁版嵁
// @Description Agent 涓婃姤绯荤粺鐩戞帶鏁版嵁
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentMonitorRequest true "鐩戞帶鏁版嵁"
// @Success 200 {object} map[string]any
// @Router /api/v2/agent/monitor [post]
func (h *AgentHandler) AgentMonitor(c *gin.Context) {
	var req AgentMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 保存该节点最新监控快照 (内存实时态)，供面板拉取展示。
	// 历史趋势的时序库持久化 (InfluxDB/Prometheus) 由部署侧按需接入。
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
	NodeID uint           `json:"node_id" binding:"required"`
	System map[string]any `json:"system"`
}

// AgentMonitorSnapshot stores the latest monitor payload pushed by an agent node.
type AgentMonitorSnapshot struct {
	NodeID    uint           `json:"node_id"`
	System    map[string]any `json:"system"`
	UpdatedAt time.Time      `json:"updated_at"`
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
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("[WARN] agent ws: failed to close connection: %v", err)
		}
	}()
	if err := prepareAgentWebSocket(conn); err != nil {
		log.Printf("[WARN] agent ws: failed to prepare connection: %v", err)
		return
	}

	// 绛夊緟璁よ瘉娑堟伅
	msg, err := readAgentWebSocketMessage(conn)
	if err != nil {
		return
	}

	var authMsg struct {
		Type   string `json:"type"`
		NodeID uint   `json:"node_id"`
		Token  string `json:"token"`
	}
	if err := json.Unmarshal(msg, &authMsg); err != nil || authMsg.Type != "auth" {
		agentConn := &AgentConnection{WsConn: conn}
		_ = writeAgentWebSocketJSON(agentConn, map[string]string{"error": "invalid auth"})
		return
	}

	// 楠岃瘉 Token
	var node model.ForwardNode
	if err := h.db.First(&node, authMsg.NodeID).Error; err != nil {
		agentConn := &AgentConnection{WsConn: conn}
		_ = writeAgentWebSocketJSON(agentConn, map[string]string{"error": "node not found"})
		return
	}
	if node.APIToken != authMsg.Token {
		agentConn := &AgentConnection{WsConn: conn}
		_ = writeAgentWebSocketJSON(agentConn, map[string]string{"error": "invalid token"})
		return
	}

	// 璁板綍 WebSocket 杩炴帴
	agentConn := &AgentConnection{
		NodeID:   authMsg.NodeID,
		LastSeen: time.Now(),
		WsConn:   conn,
	}
	h.connections.Store(authMsg.NodeID, agentConn)

	_ = writeAgentWebSocketJSON(agentConn, map[string]string{"type": "auth", "message": "connected"})

	// 澶勭悊娑堟伅寰幆
	for {
		msg, err := readAgentWebSocketMessage(conn)
		if err != nil {
			break
		}

		// Parse and dispatch WebSocket message types:
		//   - "heartbeat": update LastSeen timestamp
		//   - "log": forward log entries to the logging system
		//   - "status_change": notify about service state changes
		//   - "error": record and alert error events
		var m map[string]any
		if err := json.Unmarshal(msg, &m); err != nil {
			log.Printf("[WARN] agent ws: failed to parse message: %v", err)
			continue
		}
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

// ========== 绠＄悊鎺ュ彛 ==========

// CreateTask godoc
// @Summary 鍒涘缓浠诲姟
// @Description 绠＄悊鍛樺悜鑺傜偣涓嬪彂浠诲姟
// @Tags 绠＄悊绔?Agent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateTaskRequest true "浠诲姟淇℃伅"
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

	// 只接受白名单诊断动作，params 会被归一化（多余字段丢弃，数值裁剪）。
	normalizedParams, err := service.ValidateAgentDiagnosticTask(req.Action, req.Params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task := AgentTask{
		ID:      generateTaskID(),
		Type:    req.Type,
		Action:  req.Action,
		Params:  normalizedParams,
		Timeout: req.Timeout,
	}

	if h.diagnosticSvc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "diagnostic task service unavailable"})
		return
	}
	taskRow, err := h.diagnosticSvc.CreateTask(task.ID, req.NodeID, req.Action, normalizedParams)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	messageID, ack, dispatchErr := h.dispatchWithAckRetry(agentConn, "task.assign", map[string]any{
		"task": task,
	}, true)
	if dispatchErr != nil {
		if fallbackErr := h.sendLegacyTask(agentConn, task); fallbackErr != nil {
			_ = h.diagnosticSvc.MarkStatus(task.ID, model.AgentDiagnosticTaskStatusFailed)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":        "send failed",
				"task_id":      task.ID,
				"message_id":   messageID,
				"dispatch_err": dispatchErr.Error(),
				"data":         taskRow,
			})
			return
		}

		_ = h.diagnosticSvc.MarkStatus(task.ID, model.AgentDiagnosticTaskStatusDispatched)
		taskRow, _ = h.diagnosticSvc.GetTask(task.ID)

		c.JSON(http.StatusOK, gin.H{
			"message":      "task sent with legacy fallback",
			"task_id":      task.ID,
			"node_id":      req.NodeID,
			"success":      true,
			"output":       "task dispatched (legacy fallback)",
			"duration_ms":  int64(0),
			"message_id":   messageID,
			"ack_received": false,
			"dispatch_err": dispatchErr.Error(),
			"data":         taskRow,
		})
		return
	}

	_ = h.diagnosticSvc.MarkStatus(task.ID, model.AgentDiagnosticTaskStatusDispatched)
	taskRow, _ = h.diagnosticSvc.GetTask(task.ID)

	c.JSON(http.StatusOK, gin.H{
		"message":      "task sent",
		"task_id":      task.ID,
		"node_id":      req.NodeID,
		"success":      true,
		"output":       "task dispatched",
		"duration_ms":  int64(0),
		"message_id":   messageID,
		"ack_received": true,
		"ack":          ack,
		"data":         taskRow,
	})
}

// CreateTaskRequest 鍒涘缓浠诲姟璇锋眰
type CreateTaskRequest struct {
	NodeID  uint           `json:"node_id" binding:"required"`
	Type    string         `json:"type" binding:"required"`
	Action  string         `json:"action" binding:"required"`
	Params  map[string]any `json:"params"`
	Timeout int            `json:"timeout"`
}

// ListAgents godoc
// @Summary 鑾峰彇鍦ㄧ嚎 Agent 鍒楄〃
// @Description 绠＄悊鍛樿幏鍙栨墍鏈夊湪绾跨殑 Agent
// @Tags 绠＄悊绔?Agent
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
// @Summary 鎵ц鍛戒护
// @Description 绠＄悊鍛樺湪鑺傜偣涓婃墽琛屽懡浠?// @Tags 绠＄悊绔?Agent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body ExecuteCommandRequest true "鍛戒护淇℃伅"
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
		Type:    "diagnostic",
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

// ========== 杞彂瑙勫垯鍚屾 ==========

// AgentGetForwardRules godoc
// @Summary 鑾峰彇杞彂瑙勫垯
// @Description Agent 鑾峰彇璇ヨ妭鐐圭殑杞彂瑙勫垯
// @Tags Agent
// @Produce json
// @Param node_id query int true "鑺傜偣 ID"
// @Success 200 {object} map[string]any
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
