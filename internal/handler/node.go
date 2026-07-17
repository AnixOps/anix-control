package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NodeHandler 节点处理器
type NodeHandler struct {
	nodeService    *service.NodeService
	nodeLogService *service.NodeLogService
	agentControl   nodeAgentControl
}

// NewNodeHandler 创建节点处理器
func NewNodeHandler() *NodeHandler {
	return &NodeHandler{
		nodeService:    service.NewNodeService(),
		nodeLogService: service.NewNodeLogService(),
		agentControl:   defaultNodeAgentControl(),
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
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
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
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 500 {object} map[string]any
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
		if errors.Is(err, service.ErrNegativeTraffic) {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "心跳失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// RuntimeHealth godoc
// @Summary 上报节点运行时健康状态
// @Description 节点上报 WireGuard/GOST 等托管运行时的进程健康状态，不覆盖普通心跳状态
// @Tags 节点通信
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.NodeRuntimeHealthRequest true "运行时健康状态"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /node/runtime-health [post]
func (h *NodeHandler) RuntimeHealth(c *gin.Context) {
	nodeID, exists := c.Get("node_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未授权"})
		return
	}

	var req model.NodeRuntimeHealthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}
	if err := h.nodeService.UpdateRuntimeHealth(nodeID.(uint), req.Healthy, req.Error); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "运行时状态更新失败"})
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
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes [get]
func (h *NodeHandler) GetNodes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = ClampPagination(page, pageSize)
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

	panelSuccess(c, result)
}

// GetNode godoc
// @Summary 获取节点详情
// @Description 管理员获取指定节点的详细信息
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
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

	panelSuccess(c, node)
}

// GetNodeCredentials godoc
// @Summary 获取节点凭证
// @Description 管理员获取指定节点的 api_key / secret, 用于 AnixOps Agent 对接配置。
// @Description Node.APIKey/Secret 在普通序列化里是隐藏字段 (json:"-"), 此接口显式返回,
// @Description 仅限管理员, 供 Ansible 等部署工具自动拉取节点凭证。
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /admin/nodes/{id}/credentials [get]
func (h *NodeHandler) GetNodeCredentials(c *gin.Context) {
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

	panelSuccess(c, gin.H{
		"node_id": node.ID,
		"name":    node.Name,
		"host":    node.Host,
		"port":    node.Port,
		"api_key": node.APIKey,
		"secret":  node.Secret,
	})
}

// CreateNode godoc
// @Summary 创建节点
// @Description 管理员手动创建节点
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.Node true "节点信息"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
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

	panelSuccess(c, gin.H{
		"node_id": node.ID,
		"api_key": node.APIKey,
		"secret":  node.Secret,
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
// @Param request body map[string]any true "节点更新信息"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/{id} [put]
func (h *NodeHandler) UpdateNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	var updates map[string]any
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

	panelSuccess(c, gin.H{"message": "更新成功"})
}

// DeleteNode godoc
// @Summary 删除节点
// @Description 管理员删除指定节点
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
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

	panelSuccess(c, gin.H{"message": "删除成功"})
}

// GetNodeStats godoc
// @Summary 获取节点统计
// @Description 管理员获取节点统计数据
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/stats [get]
func (h *NodeHandler) GetNodeStats(c *gin.Context) {
	stats, err := h.nodeService.GetNodeStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取统计失败"})
		return
	}

	panelSuccess(c, stats)
}

// GetNodeLogs godoc
// @Summary 获取节点运行日志
// @Description 管理员获取指定节点通过 gRPC 上报的运行日志
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param level query string false "日志级别"
// @Param source query string false "日志来源"
// @Param search query string false "关键词"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/{id}/logs [get]
func (h *NodeHandler) GetNodeLogs(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = ClampPagination(page, pageSize)

	if _, err := h.nodeService.GetNode(uint(nodeID)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "节点不存在"})
		return
	}

	result, err := h.nodeLogService.GetLogs(service.NodeLogListParams{
		NodeID:   uint(nodeID),
		Page:     page,
		PageSize: pageSize,
		Level:    c.Query("level"),
		Source:   c.Query("source"),
		Search:   c.Query("search"),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取节点日志失败", "error": err.Error()})
		return
	}

	list := make([]gin.H, 0, len(result.List))
	for _, item := range result.List {
		var fields any
		if item.FieldsJSON != "" {
			_ = json.Unmarshal([]byte(item.FieldsJSON), &fields)
		}

		list = append(list, gin.H{
			"id":          item.ID,
			"node_id":     item.NodeID,
			"level":       item.Level,
			"source":      item.Source,
			"message":     item.Message,
			"trace_id":    item.TraceID,
			"fields":      fields,
			"fields_json": item.FieldsJSON,
			"logged_at":   item.LoggedAt,
			"created_at":  item.CreatedAt,
		})
	}

	panelSuccess(c, gin.H{
		"list":      list,
		"total":     result.Total,
		"page":      page,
		"page_size": pageSize,
	})
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
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
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

	var config any
	if node.RawConfig != nil && *node.RawConfig != "" {
		if err := json.Unmarshal([]byte(*node.RawConfig), &config); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "节点原始配置 JSON 无效"})
			return
		}
	}

	panelSuccess(c, gin.H{
		"node_id":    node.ID,
		"name":       node.Name,
		"raw_config": config,
	})
}

func decodeRawConfigObject(raw any) (map[string]any, []byte, error) {
	var (
		jsonBytes []byte
		err       error
	)
	if rawString, ok := raw.(string); ok {
		jsonBytes = []byte(rawString)
	} else {
		jsonBytes, err = json.Marshal(raw)
		if err != nil {
			return nil, nil, err
		}
	}

	var config map[string]any
	if err := json.Unmarshal(jsonBytes, &config); err != nil || config == nil {
		if err == nil {
			err = errors.New("raw config must be an object")
		}
		return nil, nil, err
	}
	normalized, err := json.Marshal(config)
	if err != nil {
		return nil, nil, err
	}
	return config, normalized, nil
}

// UpdateNodeRawConfig godoc
// @Summary 更新节点原始配置
// @Description 管理员更新指定节点的原始JSON配置（高级模式）
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Param request body map[string]any true "原始配置 {raw_config: object}"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/{id}/raw-config [put]
func (h *NodeHandler) UpdateNodeRawConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	var req struct {
		RawConfig any `json:"raw_config" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	// 验证并序列化 JSON
	var rawConfigStr *string
	if req.RawConfig != nil {
		config, jsonBytes, err := decodeRawConfigObject(req.RawConfig)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "原始配置必须是 JSON 对象"})
			return
		}
		if err := service.ValidateWireGuardRuntimeConfig(config); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "WireGuard 配置无效", "error": err.Error()})
			return
		}
		str := string(jsonBytes)
		rawConfigStr = &str
	}

	if err := h.nodeService.UpdateNode(uint(id), map[string]any{
		"raw_config": rawConfigStr,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败"})
		return
	}

	panelSuccess(c, gin.H{"message": "配置更新成功"})
}

// ValidateRawConfig godoc
// @Summary 验证原始配置
// @Description 管理员验证节点原始JSON配置的有效性
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]any true "原始配置 {raw_config: object}"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /admin/nodes/validate-config [post]
func (h *NodeHandler) ValidateRawConfig(c *gin.Context) {
	var req struct {
		RawConfig any `json:"raw_config" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	config, jsonBytes, err := decodeRawConfigObject(req.RawConfig)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"valid":   false,
			"message": "配置必须是 JSON 对象",
		})
		return
	}
	if err := service.ValidateWireGuardRuntimeConfig(config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"valid":   false,
			"message": "WireGuard 配置无效",
			"error":   err.Error(),
		})
		return
	}

	warnings := []string{}
	if _, ok := config["server_port"]; !ok {
		warnings = append(warnings, "缺少 server_port 字段")
	}

	panelSuccess(c, gin.H{
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
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
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

	panelSuccess(c, protocols)
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
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
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
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidNodeProtocol) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"message": "创建失败", "error": err.Error()})
		return
	}

	panelSuccess(c, protocol)
}

// UpdateProtocol godoc
// @Summary 更新节点协议
// @Description 管理员更新指定的节点协议配置
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param protocol_id path int true "协议ID"
// @Param request body map[string]any true "协议更新信息"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/protocols/{protocol_id} [put]
func (h *NodeHandler) UpdateProtocol(c *gin.Context) {
	protocolID, err := strconv.ParseUint(c.Param("protocol_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的协议ID"})
		return
	}

	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	delete(updates, "id")
	delete(updates, "node_id")

	if err := h.nodeService.UpdateProtocol(uint(protocolID), updates); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidNodeProtocol) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"message": "更新失败", "error": err.Error()})
		return
	}

	panelSuccess(c, gin.H{"message": "更新成功"})
}

// DeleteProtocol godoc
// @Summary 删除节点协议
// @Description 管理员删除指定的节点协议配置
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param protocol_id path int true "协议ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
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

	panelSuccess(c, gin.H{"message": "删除成功"})
}

// GetProtocolTemplates godoc
// @Summary 获取协议模板
// @Description 管理员获取可用的协议模板列表
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /admin/protocol-templates [get]
func (h *NodeHandler) GetProtocolTemplates(c *gin.Context) {
	templates := model.GetProtocolTemplates()
	panelSuccess(c, templates)
}

// GenerateWireGuardKeypair creates a WireGuard server keypair for the admin
// protocol form. The private key is returned only to the authenticated caller
// and is never stored by this endpoint.
func (h *NodeHandler) GenerateWireGuardKeypair(c *gin.Context) {
	privateKey, publicKey, err := service.GenerateWireGuardKeypair()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "生成 WireGuard 密钥失败"})
		return
	}
	panelSuccess(c, gin.H{"private_key": privateKey, "public_key": publicKey})
}

// SyncProtocol godoc
// @Summary 同步协议到节点
// @Description 管理员将协议配置同步到指定节点
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/{id}/sync [post]
func (h *NodeHandler) SyncProtocol(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	if _, err := h.nodeService.GetNode(uint(nodeID)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "节点不存在"})
		return
	}

	if h.agentControl != nil {
		if _, connected := h.agentControl.Connection(uint32(nodeID)); connected {
			ack, operation, dispatchErr := h.dispatchAgentControlOperation(
				c.Request.Context(),
				uint32(nodeID),
				"",
				"node.reload",
				nil,
				defaultAgentControlOperationTimeout,
			)
			if dispatchErr != nil {
				c.JSON(http.StatusBadGateway, gin.H{"message": "Agent Control 同步下发失败", "error": dispatchErr.Error()})
				return
			}
			panelSuccess(c, gin.H{
				"message":      "同步操作已由 AnixOps Agent 接收",
				"transport":    "agent-control-grpc",
				"operation_id": operation.OperationId,
				"revision":     operation.Revision,
				"ack":          ack,
			})
			return
		}
	}

	if err := h.nodeService.SyncProtocolToNode(uint(nodeID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "同步失败"})
		return
	}

	panelSuccess(c, gin.H{
		"message":   "节点未连接 Agent Control，将保留旧版周期拉取同步",
		"transport": "legacy-poll",
	})
}

// ========== 授权密钥管理 ==========

// GenerateAuthKey godoc
// @Summary 生成授权密钥
// @Description 管理员生成节点自动注册的授权密钥
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]any true "密钥请求 {name, expire_days}"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/auth-keys [post]
func (h *NodeHandler) GenerateAuthKey(c *gin.Context) {
	var req struct {
		Name       string `json:"name" binding:"required,min=1,max=255"`
		ExpireDays int    `json:"expire_days" binding:"gte=0"`
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

	panelSuccess(c, gin.H{
		"id":        authKey.ID,
		"name":      authKey.Name,
		"key":       key, // 只在创建时返回一次
		"expire_at": authKey.ExpireAt,
	})
}

// GetAuthKeys godoc
// @Summary 获取授权密钥列表
// @Description 管理员获取所有授权密钥列表
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/auth-keys [get]
func (h *NodeHandler) GetAuthKeys(c *gin.Context) {
	keys, err := h.nodeService.GetAuthKeys()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取失败"})
		return
	}

	panelSuccess(c, keys)
}

// DeleteAuthKey godoc
// @Summary 删除授权密钥
// @Description 管理员删除指定的授权密钥
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "密钥ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
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

	panelSuccess(c, gin.H{"message": "删除成功"})
}

// ========== 内部API (API Token认证) ==========

// InternalGenerateAuthKey godoc
// @Summary 内部生成授权密钥
// @Description 使用API Token认证生成节点自动注册的授权密钥（供Ansible等自动化工具使用）
// @Tags 内部API
// @Accept json
// @Produce json
// @Security ApiTokenAuth
// @Param request body map[string]any true "密钥请求 {name, expire_days, node_name}"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /internal/auth-keys [post]
func (h *NodeHandler) InternalGenerateAuthKey(c *gin.Context) {
	var req struct {
		Name       string `json:"name" binding:"min=1,max=255"`
		ExpireDays int    `json:"expire_days" binding:"gte=0"`
		NodeName   string `json:"node_name" binding:"omitempty,max=255"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	// 自动生成名称
	if req.Name == "" {
		if req.NodeName != "" {
			req.Name = "Ansible-" + req.NodeName
		} else {
			req.Name = "Ansible-Auto-Generated"
		}
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
			"key":       key,
			"expire_at": authKey.ExpireAt,
		},
	})
}
