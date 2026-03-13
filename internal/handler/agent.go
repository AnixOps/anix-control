package handler

import (
	"encoding/json"
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

// AgentHandler Agent 管理 API
type AgentHandler struct {
	db          *gorm.DB
	connections sync.Map // nodeID -> *AgentConnection
	wsUpgrader  websocket.Upgrader
}

// AgentConnection Agent 连接信息
type AgentConnection struct {
	NodeID      uint
	LastSeen    time.Time
	Version     string
	SystemInfo  map[string]interface{}
	Capabilities []string
	WsConn      *websocket.Conn
}

// NewAgentHandler 创建 Handler
func NewAgentHandler() *AgentHandler {
	return &AgentHandler{
		db: database.Get(),
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
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
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/agent/register [post]
func (h *AgentHandler) AgentRegister(c *gin.Context) {
	var req AgentRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证 Token
	var node model.ForwardNode
	if err := h.db.First(&node, req.NodeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	if node.APIToken != req.Token {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	// 记录连接
	conn := &AgentConnection{
		NodeID:      req.NodeID,
		LastSeen:    time.Now(),
		Version:     req.Version,
		SystemInfo:  req.System,
		Capabilities: req.Capabilities,
	}
	h.connections.Store(req.NodeID, conn)

	// 更新节点状态
	h.db.Model(&node).Updates(map[string]interface{}{
		"status":     model.ForwardNodeStatusOnline,
		"last_check": time.Now(),
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "registered",
		"node_id": req.NodeID,
	})
}

// AgentRegisterRequest 注册请求
type AgentRegisterRequest struct {
	NodeID       uint                   `json:"node_id"`
	Token        string                 `json:"token"`
	Version      string                 `json:"version"`
	System       map[string]interface{} `json:"system"`
	Capabilities []string               `json:"capabilities"`
}

// ========== Agent 心跳 ==========

// AgentHeartbeat godoc
// @Summary Agent 心跳
// @Description Agent 定期发送心跳
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentHeartbeatRequest true "心跳信息"
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/agent/heartbeat [post]
func (h *AgentHandler) AgentHeartbeat(c *gin.Context) {
	var req AgentHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新连接时间
	if conn, ok := h.connections.Load(req.NodeID); ok {
		ac := conn.(*AgentConnection)
		ac.LastSeen = time.Now()
	}

	// 更新节点状态
	h.db.Model(&model.ForwardNode{}).Where("id = ?", req.NodeID).Updates(map[string]interface{}{
		"last_check": time.Now(),
		"status":     model.ForwardNodeStatusOnline,
	})

	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// AgentHeartbeatRequest 心跳请求
type AgentHeartbeatRequest struct {
	NodeID    uint                   `json:"node_id"`
	Status    string                 `json:"status"`
	Resources map[string]interface{} `json:"resources"`
}

// ========== 任务管理 ==========

// AgentGetTasks godoc
// @Summary 获取待执行任务
// @Description Agent 轮询获取待执行的任务
// @Tags Agent
// @Produce json
// @Param node_id query int true "节点 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/agent/tasks [get]
func (h *AgentHandler) AgentGetTasks(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Query("node_id"), 10, 32)
	_ = nodeID // TODO: 使用 nodeID 过滤任务

	// TODO: 从任务队列获取该节点的待执行任务
	// 这里简化实现，返回空任务列表
	tasks := []AgentTask{}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

// AgentTask 任务定义
type AgentTask struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Action   string                 `json:"action"`
	Params   map[string]interface{} `json:"params"`
	Timeout  int                    `json:"timeout"`
}

// AgentReportResult godoc
// @Summary 上报任务结果
// @Description Agent 上报任务执行结果
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentTaskResult true "任务结果"
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/agent/result [post]
func (h *AgentHandler) AgentReportResult(c *gin.Context) {
	var result AgentTaskResult
	if err := c.ShouldBindJSON(&result); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: 保存任务结果到数据库
	// TODO: 如果有回调，触发回调

	c.JSON(http.StatusOK, gin.H{"message": "received"})
}

// AgentTaskResult 任务结果
type AgentTaskResult struct {
	TaskID    string      `json:"task_id"`
	Success   bool        `json:"success"`
	Output    string      `json:"output"`
	Error     string      `json:"error,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Duration  int64       `json:"duration_ms"`
	Timestamp time.Time   `json:"timestamp"`
}

// ========== 监控数据 ==========

// AgentMonitor godoc
// @Summary 上报监控数据
// @Description Agent 上报系统监控数据
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentMonitorRequest true "监控数据"
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/agent/monitor [post]
func (h *AgentHandler) AgentMonitor(c *gin.Context) {
	var req AgentMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: 保存监控数据到时序数据库

	c.JSON(http.StatusOK, gin.H{"message": "received"})
}

// AgentMonitorRequest 监控请求
type AgentMonitorRequest struct {
	NodeID uint                   `json:"node_id"`
	System map[string]interface{} `json:"system"`
}

// ========== WebSocket 连接 ==========

// AgentWebSocket godoc
// @Summary WebSocket 连接
// @Description Agent 建立 WebSocket 长连接
// @Tags Agent
// @Success 101
// @Router /api/v2/agent/ws [get]
func (h *AgentHandler) AgentWebSocket(c *gin.Context) {
	conn, err := h.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 等待认证消息
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

	// 验证 Token
	var node model.ForwardNode
	if err := h.db.First(&node, authMsg.NodeID).Error; err != nil {
		conn.WriteJSON(map[string]string{"error": "node not found"})
		return
	}
	if node.APIToken != authMsg.Token {
		conn.WriteJSON(map[string]string{"error": "invalid token"})
		return
	}

	// 记录 WebSocket 连接
	agentConn := &AgentConnection{
		NodeID:   authMsg.NodeID,
		LastSeen: time.Now(),
		WsConn:   conn,
	}
	h.connections.Store(authMsg.NodeID, agentConn)

	conn.WriteJSON(map[string]string{"type": "auth", "message": "connected"})

	// 处理消息循环
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		// 处理消息
		var m map[string]interface{}
		json.Unmarshal(msg, &m)
		// TODO: 处理不同类型的消息
		_ = m
	}

	// 清理连接
	h.connections.Delete(authMsg.NodeID)
}

// ========== 管理接口 ==========

// CreateTask godoc
// @Summary 创建任务
// @Description 管理员向节点下发任务
// @Tags 管理端-Agent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateTaskRequest true "任务信息"
// @Success 200 {object} map[string]interface{}
// @Router /admin/agent/tasks [post]
func (h *AgentHandler) CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查节点是否在线
	conn, ok := h.connections.Load(req.NodeID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node offline"})
		return
	}

	agentConn := conn.(*AgentConnection)

	// 如果有 WebSocket 连接，直接发送
	if agentConn.WsConn != nil {
		err := agentConn.WsConn.WriteJSON(map[string]interface{}{
			"type": "task",
			"task": AgentTask{
				ID:      generateTaskID(),
				Type:    req.Type,
				Action:  req.Action,
				Params:  req.Params,
				Timeout: req.Timeout,
			},
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "send failed"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "task sent"})
}

// CreateTaskRequest 创建任务请求
type CreateTaskRequest struct {
	NodeID  uint                   `json:"node_id" binding:"required"`
	Type    string                 `json:"type" binding:"required"`
	Action  string                 `json:"action" binding:"required"`
	Params  map[string]interface{} `json:"params"`
	Timeout int                    `json:"timeout"`
}

// ListAgents godoc
// @Summary 获取在线 Agent 列表
// @Description 管理员获取所有在线的 Agent
// @Tags 管理端-Agent
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

	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

// ExecuteCommand godoc
// @Summary 执行命令
// @Description 管理员在节点上执行命令
// @Tags 管理端-Agent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body ExecuteCommandRequest true "命令信息"
// @Success 200 {object} map[string]interface{}
// @Router /admin/agent/execute [post]
func (h *AgentHandler) ExecuteCommand(c *gin.Context) {
	var req ExecuteCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 创建命令任务
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

	// 复用 CreateTask 逻辑
	c.Set("task_request", taskReq)
	h.CreateTask(c)
}

// ExecuteCommandRequest 执行命令请求
type ExecuteCommandRequest struct {
	NodeID  uint                   `json:"node_id" binding:"required"`
	Command string                 `json:"command" binding:"required"`
	Args    []string               `json:"args"`
	Env     map[string]interface{} `json:"env"`
	Timeout int                    `json:"timeout"`
}

// ========== 转发规则同步 ==========

// AgentGetForwardRules godoc
// @Summary 获取转发规则
// @Description Agent 获取该节点的转发规则
// @Tags Agent
// @Produce json
// @Param node_id query int true "节点 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/forward/agent/rules [get]
func (h *AgentHandler) AgentGetForwardRules(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Query("node_id"), 10, 32)

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

func generateTaskID() string {
	return fmt.Sprintf("task-%d", time.Now().UnixNano())
}