package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// NodeHandler 节点处理器
type NodeHandler struct {
	nodeService *service.NodeService
}

// NewNodeHandler 创建节点处理器
func NewNodeHandler() *NodeHandler {
	return &NodeHandler{
		nodeService: service.NewNodeService(),
	}
}

// ========== 节点自动注册 API (公开) ==========

// Register 节点自动注册
func (h *NodeHandler) Register(c *gin.Context) {
	var req model.NodeRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	// 获取客户端IP
	clientIP := c.ClientIP()
	if req.Host == "" {
		req.Host = clientIP
	}
	if req.Port == 0 {
		req.Port = 443
	}

	resp, err := h.nodeService.RegisterNode(&req, clientIP)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "注册成功",
		"data":    resp,
	})
}

// Heartbeat 节点心跳
func (h *NodeHandler) Heartbeat(c *gin.Context) {
	// 从中间件获取节点ID
	nodeID, exists := c.Get("node_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未授权"})
		return
	}

	var req model.NodeHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	if err := h.nodeService.Heartbeat(nodeID.(uint), &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "心跳失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// ========== 管理员节点管理 ==========

// GetNodes 获取节点列表
func (h *NodeHandler) GetNodes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	search := c.Query("search")

	var status *model.NodeStatus
	if s := c.Query("status"); s != "" {
		st, _ := strconv.Atoi(s)
		ns := model.NodeStatus(st)
		status = &ns
	}

	var groupID *uint
	if g := c.Query("group_id"); g != "" {
		gid, _ := strconv.ParseUint(g, 10, 32)
		gidVal := uint(gid)
		groupID = &gidVal
	}

	result, err := h.nodeService.GetNodes(service.NodeListParams{
		Page:     page,
		PageSize: pageSize,
		Status:   status,
		GroupID:  groupID,
		Search:   search,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取节点列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetNode 获取节点详情
func (h *NodeHandler) GetNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	node, err := h.nodeService.GetNode(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "节点不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": node})
}

// CreateNode 创建节点 (手动添加)
func (h *NodeHandler) CreateNode(c *gin.Context) {
	var node model.Node
	if err := c.ShouldBindJSON(&node); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	if err := h.nodeService.CreateNode(&node); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "节点创建成功",
		"data": gin.H{
			"node_id": node.ID,
			"api_key": node.APIKey,
			"secret":  node.Secret,
		},
	})
}

// UpdateNode 更新节点
func (h *NodeHandler) UpdateNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	// 禁止更新敏感字段
	delete(updates, "id")
	delete(updates, "api_key")
	delete(updates, "api_key_hash")
	delete(updates, "secret")

	if err := h.nodeService.UpdateNode(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteNode 删除节点
func (h *NodeHandler) DeleteNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	if err := h.nodeService.DeleteNode(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// GetNodeStats 获取节点统计
func (h *NodeHandler) GetNodeStats(c *gin.Context) {
	stats, err := h.nodeService.GetNodeStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取统计失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// ========== 高级配置 (RawConfig) ==========

// GetNodeRawConfig 获取节点原始配置
func (h *NodeHandler) GetNodeRawConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	node, err := h.nodeService.GetNode(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "节点不存在"})
		return
	}

	var config interface{}
	if node.RawConfig != nil && *node.RawConfig != "" {
		json.Unmarshal([]byte(*node.RawConfig), &config)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"node_id":    node.ID,
			"name":       node.Name,
			"raw_config": config,
		},
	})
}

// UpdateNodeRawConfig 更新节点原始配置 (高级模式)
func (h *NodeHandler) UpdateNodeRawConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	var req struct {
		RawConfig interface{} `json:"raw_config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	// 验证并序列化 JSON
	var rawConfigStr *string
	if req.RawConfig != nil {
		jsonBytes, err := json.Marshal(req.RawConfig)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "无效的 JSON 配置"})
			return
		}
		str := string(jsonBytes)
		rawConfigStr = &str
	}

	if err := h.nodeService.UpdateNode(uint(id), map[string]interface{}{
		"raw_config": rawConfigStr,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "配置更新成功"})
}

// ValidateRawConfig 验证原始配置 JSON
func (h *NodeHandler) ValidateRawConfig(c *gin.Context) {
	var req struct {
		RawConfig interface{} `json:"raw_config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	// 验证 JSON 结构
	jsonBytes, err := json.Marshal(req.RawConfig)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"valid":   false,
			"message": "无效的 JSON",
			"error":   err.Error(),
		})
		return
	}

	// 检查必要字段
	var config map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"valid":   false,
			"message": "配置必须是 JSON 对象",
		})
		return
	}

	warnings := []string{}
	if _, ok := config["server_port"]; !ok {
		warnings = append(warnings, "缺少 server_port 字段")
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":    true,
		"message":  "配置有效",
		"warnings": warnings,
		"size":     len(jsonBytes),
	})
}

// ========== 协议管理 ==========

// GetProtocols 获取节点的协议列表
func (h *NodeHandler) GetProtocols(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	protocols, err := h.nodeService.GetProtocols(uint(nodeID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取协议列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": protocols})
}

// CreateProtocol 创建协议
func (h *NodeHandler) CreateProtocol(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	var protocol model.NodeProtocol
	if err := c.ShouldBindJSON(&protocol); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	protocol.NodeID = uint(nodeID)

	if err := h.nodeService.CreateProtocol(&protocol); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "协议创建成功",
		"data":    protocol,
	})
}

// UpdateProtocol 更新协议
func (h *NodeHandler) UpdateProtocol(c *gin.Context) {
	protocolID, err := strconv.ParseUint(c.Param("protocol_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的协议ID"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	delete(updates, "id")
	delete(updates, "node_id")

	if err := h.nodeService.UpdateProtocol(uint(protocolID), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteProtocol 删除协议
func (h *NodeHandler) DeleteProtocol(c *gin.Context) {
	protocolID, err := strconv.ParseUint(c.Param("protocol_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的协议ID"})
		return
	}

	if err := h.nodeService.DeleteProtocol(uint(protocolID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// GetProtocolTemplates 获取协议模板
func (h *NodeHandler) GetProtocolTemplates(c *gin.Context) {
	templates := model.GetProtocolTemplates()
	c.JSON(http.StatusOK, gin.H{"data": templates})
}

// SyncProtocol 同步协议到节点
func (h *NodeHandler) SyncProtocol(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	if err := h.nodeService.SyncProtocolToNode(uint(nodeID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "同步失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "同步成功"})
}

// ========== 授权密钥管理 ==========

// GenerateAuthKey 生成授权密钥
func (h *NodeHandler) GenerateAuthKey(c *gin.Context) {
	var req struct {
		Name       string `json:"name"`
		ExpireDays int    `json:"expire_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	if req.Name == "" {
		req.Name = "授权密钥"
	}

	authKey, key, err := h.nodeService.GenerateAuthKey(req.Name, req.ExpireDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "生成失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "生成成功",
		"data": gin.H{
			"id":        authKey.ID,
			"name":      authKey.Name,
			"key":       key, // 只在创建时返回一次
			"expire_at": authKey.ExpireAt,
		},
	})
}

// GetAuthKeys 获取授权密钥列表
func (h *NodeHandler) GetAuthKeys(c *gin.Context) {
	keys, err := h.nodeService.GetAuthKeys()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": keys})
}

// DeleteAuthKey 删除授权密钥
func (h *NodeHandler) DeleteAuthKey(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的ID"})
		return
	}

	if err := h.nodeService.DeleteAuthKey(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}
