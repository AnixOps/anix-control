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

// Register godoc
// @Summary 节点自动注册
// @Description 节点通过授权密钥自动注册到面板
// @Tags 节点通信
// @Accept json
// @Produce json
// @Param request body model.NodeRegisterRequest true "注册请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /node/register [post]
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

// Heartbeat godoc
// @Summary 节点心跳
// @Description 节点向面板发送心跳以保持在线状态
// @Tags 节点通信
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.NodeHeartbeatRequest true "心跳请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /node/heartbeat [post]
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

// GetNodes godoc
// @Summary 获取节点列表
// @Description 管理员获取节点列表，支持分页和筛选
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param search query string false "搜索关键词"
// @Param status query int false "节点状态"
// @Param group_id query int false "分组ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/nodes [get]
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

// GetNode godoc
// @Summary 获取节点详情
// @Description 管理员获取指定节点的详细信息
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/nodes/{id} [get]
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

// CreateNode godoc
// @Summary 创建节点
// @Description 管理员手动创建节点
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.Node true "节点信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/nodes [post]
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

// UpdateNode godoc
// @Summary 更新节点
// @Description 管理员更新指定节点的信息
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Param request body map[string]interface{} true "节点更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/nodes/{id} [put]
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

// DeleteNode godoc
// @Summary 删除节点
// @Description 管理员删除指定节点
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/nodes/{id} [delete]
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

// GetNodeStats godoc
// @Summary 获取节点统计
// @Description 管理员获取节点统计数据
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/nodes/stats [get]
func (h *NodeHandler) GetNodeStats(c *gin.Context) {
	stats, err := h.nodeService.GetNodeStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取统计失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// ========== 高级配置 (RawConfig) ==========

// GetNodeRawConfig godoc
// @Summary 获取节点原始配置
// @Description 管理员获取指定节点的原始JSON配置
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/nodes/{id}/raw-config [get]
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

// UpdateNodeRawConfig godoc
// @Summary 更新节点原始配置
// @Description 管理员更新指定节点的原始JSON配置（高级模式）
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Param request body map[string]interface{} true "原始配置 {raw_config: object}"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/nodes/{id}/raw-config [put]
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

// ValidateRawConfig godoc
// @Summary 验证原始配置
// @Description 管理员验证节点原始JSON配置的有效性
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]interface{} true "原始配置 {raw_config: object}"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /admin/nodes/validate-config [post]
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

// GetProtocols godoc
// @Summary 获取节点协议列表
// @Description 管理员获取指定节点的协议配置列表
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/nodes/{id}/protocols [get]
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

// CreateProtocol godoc
// @Summary 创建节点协议
// @Description 管理员为指定节点创建协议配置
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Param request body model.NodeProtocol true "协议配置"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/nodes/{id}/protocols [post]
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

// UpdateProtocol godoc
// @Summary 更新节点协议
// @Description 管理员更新指定的节点协议配置
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param protocol_id path int true "协议ID"
// @Param request body map[string]interface{} true "协议更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/nodes/protocols/{protocol_id} [put]
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

// DeleteProtocol godoc
// @Summary 删除节点协议
// @Description 管理员删除指定的节点协议配置
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param protocol_id path int true "协议ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/nodes/protocols/{protocol_id} [delete]
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

// GetProtocolTemplates godoc
// @Summary 获取协议模板
// @Description 管理员获取可用的协议模板列表
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /admin/nodes/protocol-templates [get]
func (h *NodeHandler) GetProtocolTemplates(c *gin.Context) {
	templates := model.GetProtocolTemplates()
	c.JSON(http.StatusOK, gin.H{"data": templates})
}

// SyncProtocol godoc
// @Summary 同步协议到节点
// @Description 管理员将协议配置同步到指定节点
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/nodes/{id}/sync [post]
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

// GenerateAuthKey godoc
// @Summary 生成授权密钥
// @Description 管理员生成节点自动注册的授权密钥
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]interface{} true "密钥请求 {name, expire_days}"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/auth-keys [post]
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

// GetAuthKeys godoc
// @Summary 获取授权密钥列表
// @Description 管理员获取所有授权密钥列表
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/auth-keys [get]
func (h *NodeHandler) GetAuthKeys(c *gin.Context) {
	keys, err := h.nodeService.GetAuthKeys()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": keys})
}

// DeleteAuthKey godoc
// @Summary 删除授权密钥
// @Description 管理员删除指定的授权密钥
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "密钥ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/auth-keys/{id} [delete]
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
