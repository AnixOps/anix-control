package handler

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var monitorWSUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     utils.CheckWebSocketOrigin,
}

// MonitorWSMessage WebSocket 监控消息
type MonitorWSMessage struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	Data      any    `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
}

// NodeSnapshot 节点快照
type NodeSnapshot struct {
	ID            uint    `json:"id"`
	Name          string  `json:"name"`
	Host          string  `json:"host"`
	Status        string  `json:"status"`
	CPUUsage      float64 `json:"cpu_usage"`
	MemoryUsage   float64 `json:"memory_usage"`
	DiskUsage     float64 `json:"disk_usage"`
	OnlineUsers   int     `json:"online_users"`
	TotalUpload   int64   `json:"total_upload"`
	TotalDownload int64   `json:"total_download"`
	Uptime        int64   `json:"uptime"`
}

// MonitorOverview 监控概览
type MonitorOverview struct {
	TotalNodes    int64 `json:"total_nodes"`
	OnlineNodes   int64 `json:"online_nodes"`
	OfflineNodes  int64 `json:"offline_nodes"`
	PendingNodes  int64 `json:"pending_nodes"`
	TotalUpload   int64 `json:"total_upload"`
	TotalDownload int64 `json:"total_download"`
}

// MonitorSnapshot 初始快照
type MonitorSnapshot struct {
	Overview MonitorOverview `json:"overview"`
	Nodes    []NodeSnapshot  `json:"nodes"`
}

// MonitorWSHandler 处理 WebSocket 监控连接
type MonitorWSHandler struct {
	nodeService *service.NodeService
}

// NewMonitorWSHandler 创建 WebSocket 监控处理器
func NewMonitorWSHandler() *MonitorWSHandler {
	return &MonitorWSHandler{
		nodeService: service.NewNodeService(),
	}
}

// HandleMonitorWS 处理 WebSocket 监控连接
// @Summary WebSocket 实时监控
// @Description 实时推送节点监控数据
// @Tags 管理端-WebSocket
// @Produce json
// @Security BearerAuth
// @Success 101
// @Router /admin/ws/monitor [get]
func (h *MonitorWSHandler) HandleMonitorWS(c *gin.Context) {
	// 从 JWTAuth 中间件获取用户信息
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
		return
	}

	isAdminVal, _ := c.Get("is_admin")
	isAdmin, _ := isAdminVal.(bool)

	// 验证 token（WebSocket 连接也可能通过 query 参数传 token）
	token := c.Query("token")
	if token != "" {
		claims, err := utils.ParseToken(token)
		if err != nil || !claims.IsAdmin {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
			return
		}
		userID = claims.UserID
		isAdmin = claims.IsAdmin
	}

	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"message": "admin access required"})
		return
	}

	// 升级为 WebSocket
	conn, err := monitorWSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[MonitorWS] WebSocket upgrade failed: %v", err)
		return
	}

	log.Printf("[MonitorWS] Admin %d connected", userID)

	client := &monitorClient{
		conn:        conn,
		userID:      userID.(uint),
		send:        make(chan []byte, 256),
		stop:        make(chan struct{}),
		mu:          sync.Mutex{},
		nodeService: h.nodeService,
	}

	go client.writePump()
	go client.runUpdates()

	// 发送初始快照
	client.sendInitialSnapshot()

	// 保持连接，等待客户端断开
	client.readPump()
}

type monitorClient struct {
	conn        *websocket.Conn
	userID      uint
	send        chan []byte
	stop        chan struct{}
	mu          sync.Mutex
	nodeService *service.NodeService
}

// readPump 读取客户端消息
func (c *monitorClient) readPump() {
	defer func() {
		if err := c.conn.Close(); err != nil {
			log.Printf("[MonitorWS] Close failed for user_id=%d: %v", c.userID, err)
		}
		close(c.stop)
	}()

	if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		log.Printf("[MonitorWS] Set read deadline failed for user_id=%d: %v", c.userID, err)
		return
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("[MonitorWS] Read error: %v", err)
			return
		}
	}
}

// writePump 发送消息到客户端
func (c *monitorClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.mu.Lock()
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					log.Printf("[MonitorWS] Close frame failed for user_id=%d: %v", c.userID, err)
				}
				c.mu.Unlock()
				return
			}

			c.mu.Lock()
			if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				c.mu.Unlock()
				log.Printf("[MonitorWS] Set write deadline failed for user_id=%d: %v", c.userID, err)
				return
			}
			err := c.conn.WriteMessage(websocket.TextMessage, message)
			c.mu.Unlock()

			if err != nil {
				log.Printf("[MonitorWS] Write error: %v", err)
				return
			}

		case <-ticker.C:
			c.mu.Lock()
			if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				c.mu.Unlock()
				log.Printf("[MonitorWS] Set ping deadline failed for user_id=%d: %v", c.userID, err)
				return
			}
			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			c.mu.Unlock()

			if err != nil {
				return
			}
		}
	}
}

// runUpdates 定期发送节点更新
func (c *monitorClient) runUpdates() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var prevSnapshot *MonitorSnapshot

	for {
		select {
		case <-ticker.C:
			snapshot := c.buildSnapshot()
			if snapshot == nil {
				continue
			}

			if prevSnapshot == nil {
				// 首次更新，发送完整快照
				msg := MonitorWSMessage{
					Type:      "snapshot",
					Timestamp: time.Now().Unix(),
					Data:      snapshot,
				}
				c.enqueueMessage(msg)
				prevSnapshot = snapshot
				continue
			}

			// 检测变化，发送增量更新
			delta := buildDelta(prevSnapshot, snapshot)
			if delta != nil {
				msg := MonitorWSMessage{
					Type:      "delta",
					Timestamp: time.Now().Unix(),
					Data:      delta,
				}
				c.enqueueMessage(msg)
				prevSnapshot = snapshot
			}

		case <-c.stop:
			return
		}
	}
}

func (c *monitorClient) sendInitialSnapshot() {
	snapshot := c.buildSnapshot()
	if snapshot == nil {
		return
	}

	msg := MonitorWSMessage{
		Type:      "initial",
		Timestamp: time.Now().Unix(),
		Data:      snapshot,
	}
	c.enqueueMessage(msg)
}

func (c *monitorClient) enqueueMessage(msg MonitorWSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[MonitorWS] Marshal %s message failed for user_id=%d: %v", msg.Type, c.userID, err)
		return
	}

	select {
	case c.send <- data:
	default:
		log.Printf("[MonitorWS] Send queue full for user_id=%d; dropping %s message", c.userID, msg.Type)
	}
}

// buildSnapshot 构建完整监控快照
func (c *monitorClient) buildSnapshot() *MonitorSnapshot {
	overview, err := c.nodeService.GetNodeStats()
	if err != nil {
		log.Printf("[MonitorWS] Failed to get node stats: %v", err)
		return nil
	}

	nodesResult, err := c.nodeService.GetNodes(service.NodeListParams{
		Page:     1,
		PageSize: 1000,
	})
	if err != nil {
		log.Printf("[MonitorWS] Failed to get nodes: %v", err)
		return nil
	}

	snapshots := make([]NodeSnapshot, 0, len(nodesResult.List))
	for _, node := range nodesResult.List {
		status := "offline"
		if node.IsOnline() {
			status = "online"
		} else if node.Status == model.NodeStatusPending {
			status = "pending"
		}

		snapshots = append(snapshots, NodeSnapshot{
			ID:            node.ID,
			Name:          node.Name,
			Host:          node.Host,
			Status:        status,
			CPUUsage:      roundTo2(node.CPUUsage),
			MemoryUsage:   roundTo2(node.MemoryUsage),
			DiskUsage:     roundTo2(node.DiskUsage),
			OnlineUsers:   node.OnlineUsers,
			TotalUpload:   node.TotalUpload,
			TotalDownload: node.TotalDownload,
			Uptime:        node.Uptime,
		})
	}

	// Safe type assertions with fallback
	totalNodes := toInt64(overview["total"])
	onlineNodes := toInt64(overview["online"])
	offlineNodes := toInt64(overview["offline"])
	pendingNodes := toInt64(overview["pending"])
	totalUpload := toInt64(overview["up"])
	totalDownload := toInt64(overview["down"])

	return &MonitorSnapshot{
		Overview: MonitorOverview{
			TotalNodes:    totalNodes,
			OnlineNodes:   onlineNodes,
			OfflineNodes:  offlineNodes,
			PendingNodes:  pendingNodes,
			TotalUpload:   totalUpload,
			TotalDownload: totalDownload,
		},
		Nodes: snapshots,
	}
}

func toInt64(v any) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	default:
		return 0
	}
}

func roundTo2(v float64) float64 {
	return math.Round(v*100) / 100
}

// NodeDelta 节点变化
type NodeDelta struct {
	ID            uint     `json:"id"`
	Name          string   `json:"name"`
	Status        string   `json:"status,omitempty"`
	CPUUsage      float64  `json:"cpu_usage,omitempty"`
	MemoryUsage   float64  `json:"memory_usage,omitempty"`
	DiskUsage     float64  `json:"disk_usage,omitempty"`
	OnlineUsers   int      `json:"online_users,omitempty"`
	TotalUpload   int64    `json:"total_upload,omitempty"`
	TotalDownload int64    `json:"total_download,omitempty"`
	Uptime        int64    `json:"uptime,omitempty"`
	ChangedFields []string `json:"changed_fields"`
}

// DeltaData 增量数据
type DeltaData struct {
	Overview    *MonitorOverview `json:"overview,omitempty"`
	NodeUpdates []NodeDelta      `json:"node_updates"`
}

// buildDelta 构建增量更新
func buildDelta(prev, curr *MonitorSnapshot) *DeltaData {
	if prev == nil || curr == nil {
		return nil
	}

	// 检查概览是否变化
	overviewChanged := prev.Overview.TotalNodes != curr.Overview.TotalNodes ||
		prev.Overview.OnlineNodes != curr.Overview.OnlineNodes ||
		prev.Overview.OfflineNodes != curr.Overview.OfflineNodes ||
		prev.Overview.PendingNodes != curr.Overview.PendingNodes ||
		prev.Overview.TotalUpload != curr.Overview.TotalUpload ||
		prev.Overview.TotalDownload != curr.Overview.TotalDownload

	// 构建节点 ID 映射
	prevMap := make(map[uint]NodeSnapshot, len(prev.Nodes))
	for _, n := range prev.Nodes {
		prevMap[n.ID] = n
	}
	currMap := make(map[uint]NodeSnapshot, len(curr.Nodes))
	for _, n := range curr.Nodes {
		currMap[n.ID] = n
	}

	updates := make([]NodeDelta, 0)

	// 检查已存在节点的变化
	for id, currNode := range currMap {
		prevNode, exists := prevMap[id]
		if !exists {
			// 新节点
			updates = append(updates, NodeDelta{
				ID:            currNode.ID,
				Name:          currNode.Name,
				Status:        currNode.Status,
				CPUUsage:      currNode.CPUUsage,
				MemoryUsage:   currNode.MemoryUsage,
				DiskUsage:     currNode.DiskUsage,
				OnlineUsers:   currNode.OnlineUsers,
				TotalUpload:   currNode.TotalUpload,
				TotalDownload: currNode.TotalDownload,
				Uptime:        currNode.Uptime,
				ChangedFields: []string{"new"},
			})
			continue
		}

		delta := compareNodes(prevNode, currNode)
		if delta != nil {
			updates = append(updates, *delta)
		}
	}

	// 检查被移除的节点
	for id, prevNode := range prevMap {
		if _, exists := currMap[id]; !exists {
			updates = append(updates, NodeDelta{
				ID:            prevNode.ID,
				Name:          prevNode.Name,
				Status:        "removed",
				ChangedFields: []string{"removed"},
			})
		}
	}

	if len(updates) == 0 && !overviewChanged {
		return nil
	}

	deltaData := &DeltaData{
		NodeUpdates: updates,
	}

	if overviewChanged {
		deltaData.Overview = &curr.Overview
	}

	return deltaData
}

// compareNodes 比较两个节点快照，返回变化字段
func compareNodes(prev, curr NodeSnapshot) *NodeDelta {
	var changed []string

	if prev.Status != curr.Status {
		changed = append(changed, "status")
	}
	if prev.CPUUsage != curr.CPUUsage {
		changed = append(changed, "cpu_usage")
	}
	if prev.MemoryUsage != curr.MemoryUsage {
		changed = append(changed, "memory_usage")
	}
	if prev.DiskUsage != curr.DiskUsage {
		changed = append(changed, "disk_usage")
	}
	if prev.OnlineUsers != curr.OnlineUsers {
		changed = append(changed, "online_users")
	}
	if prev.TotalUpload != curr.TotalUpload {
		changed = append(changed, "total_upload")
	}
	if prev.TotalDownload != curr.TotalDownload {
		changed = append(changed, "total_download")
	}
	if prev.Uptime != curr.Uptime {
		changed = append(changed, "uptime")
	}

	if len(changed) == 0 {
		return nil
	}

	return &NodeDelta{
		ID:            curr.ID,
		Name:          curr.Name,
		Status:        curr.Status,
		CPUUsage:      curr.CPUUsage,
		MemoryUsage:   curr.MemoryUsage,
		DiskUsage:     curr.DiskUsage,
		OnlineUsers:   curr.OnlineUsers,
		TotalUpload:   curr.TotalUpload,
		TotalDownload: curr.TotalDownload,
		Uptime:        curr.Uptime,
		ChangedFields: changed,
	}
}
