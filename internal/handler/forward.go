package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/gost"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// ForwardHandler 转发处理器
type ForwardHandler struct {
	nodeService  *service.ForwardNodeService
	ruleService  *service.ForwardRuleService
	panelService *service.PanelForwardService
	gostManager  *gost.Manager
}

// NewForwardHandler 创建处理器
func NewForwardHandler() *ForwardHandler {
	db := database.Get()
	nodeService := service.NewForwardNodeService(db)
	return &ForwardHandler{
		nodeService:  nodeService,
		ruleService:  service.NewForwardRuleService(db, nodeService),
		panelService: service.NewPanelForwardService(db),
		gostManager:  gost.NewManager(db),
	}
}

// ========== 中转节点管理 ==========

// ListNodes godoc
// @Summary 获取中转节点列表
// @Description 管理员获取中转节点列表，支持分页和类型筛选
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param type query string false "节点类型 (relay/exit)"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/forward/nodes [get]
func (h *ForwardHandler) ListNodes(c *gin.Context) {
	nodeType := c.Query("type")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	var status *int
	if rawStatus := c.Query("status"); rawStatus != "" {
		parsedStatus, err := strconv.Atoi(rawStatus)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		status = &parsedStatus
	}

	nodes, total, err := h.nodeService.List(nodeType, status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"list":      nodes,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
		"list":      nodes,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// CreateNode godoc
// @Summary 创建中转节点
// @Description 管理员创建新的中转节点
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateNodeRequest true "节点信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/forward/nodes [post]
func (h *ForwardHandler) CreateNode(c *gin.Context) {
	var req CreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	node := &model.ForwardNode{
		Name:      req.Name,
		Type:      req.Type,
		Host:      req.Host,
		Port:      req.Port,
		APIPort:   req.APIPort,
		Region:    req.Region,
		ISP:       req.ISP,
		Bandwidth: req.Bandwidth,
		Weight:    req.Weight,
		MaxConn:   req.MaxConn,
		Enabled:   true,
	}

	if req.APIToken != "" {
		node.APIToken = req.APIToken
	} else {
		node.APIToken = h.nodeService.GenerateAPIToken()
	}

	if err := h.nodeService.Create(node); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": node})
}

// GetNode godoc
// @Summary 获取中转节点详情
// @Description 管理员获取指定中转节点的详细信息
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/forward/nodes/{id} [get]
func (h *ForwardHandler) GetNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	node, err := h.nodeService.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": node})
}

// UpdateNode godoc
// @Summary 更新中转节点
// @Description 管理员更新指定中转节点的信息
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Param request body UpdateNodeRequest true "节点更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/forward/nodes/{id} [put]
func (h *ForwardHandler) UpdateNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	node, err := h.nodeService.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	var req UpdateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新字段
	if req.Name != "" {
		node.Name = req.Name
	}
	if req.Type != "" {
		node.Type = req.Type
	}
	if req.Host != "" {
		node.Host = req.Host
	}
	if req.Port > 0 {
		node.Port = req.Port
	}
	if req.Region != "" {
		node.Region = req.Region
	}
	if req.ISP != "" {
		node.ISP = req.ISP
	}
	if req.Bandwidth > 0 {
		node.Bandwidth = req.Bandwidth
	}
	if req.Weight > 0 {
		node.Weight = req.Weight
	}
	if req.MaxConn > 0 {
		node.MaxConn = req.MaxConn
	}
	if req.Enabled != nil {
		node.Enabled = *req.Enabled
	}

	if err := h.nodeService.Update(node); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": node})
}

// DeleteNode godoc
// @Summary 删除中转节点
// @Description 管理员删除指定中转节点
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/forward/nodes/{id} [delete]
func (h *ForwardHandler) DeleteNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.nodeService.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// CheckNode godoc
// @Summary 健康检查节点
// @Description 管理员对指定中转节点进行健康检查
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/forward/nodes/{id}/check [post]
func (h *ForwardHandler) CheckNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	result, err := h.nodeService.HealthCheck(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// ToggleNode godoc
// @Summary 切换节点状态
// @Description 管理员启用或禁用指定中转节点
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Param request body ToggleRequest true "状态请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/forward/nodes/{id}/toggle [post]
func (h *ForwardHandler) ToggleNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req ToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	node, err := h.nodeService.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	if req.Enabled {
		node.Enabled = true
		node.Status = model.ForwardNodeStatusOnline
	} else {
		node.Enabled = false
		node.Status = model.ForwardNodeStatusOffline
	}

	if err := h.nodeService.Update(node); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// ========== 转发规则管理 ==========

// ListRules godoc
// @Summary 获取转发规则列表
// @Description 管理员获取转发规则列表，支持分页和用户筛选
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param user_id query int false "用户ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/forward/rules [get]
func (h *ForwardHandler) ListRules(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	var userID *uint
	if uid := c.Query("user_id"); uid != "" {
		id, _ := strconv.ParseUint(uid, 10, 32)
		uidUint := uint(id)
		userID = &uidUint
	}

	rules, total, err := h.ruleService.List(page, pageSize, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"list":      rules,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
		"list":      rules,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// CreateRule godoc
// @Summary 创建转发规则
// @Description 管理员创建新的转发规则
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateRuleRequest true "规则信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/forward/rules [post]
func (h *ForwardHandler) CreateRule(c *gin.Context) {
	var req CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rule := &model.ForwardRule{
		Name:         req.Name,
		Enabled:      true,
		RelayNodeID:  req.RelayNodeID,
		ListenPort:   req.ListenPort,
		Protocol:     req.Protocol,
		ExitNodeID:   req.ExitNodeID,
		TargetHost:   req.TargetHost,
		TargetPort:   req.TargetPort,
		UserID:       req.UserID,
		UserGroupID:  req.UserGroupID,
		SpeedLimit:   req.SpeedLimit,
		TrafficLimit: req.TrafficLimit,
		ExpireTime:   req.ExpireTime,
		Remark:       req.Remark,
	}

	if rule.Protocol == "" {
		rule.Protocol = "tcp"
	}

	if err := h.ruleService.Create(rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rule})
}

// GetRule godoc
// @Summary 获取转发规则详情
// @Description 管理员获取指定转发规则的详细信息
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "规则ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/forward/rules/{id} [get]
func (h *ForwardHandler) GetRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	rule, err := h.ruleService.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rule})
}

// UpdateRule godoc
// @Summary 更新转发规则
// @Description 管理员更新指定转发规则的信息
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "规则ID"
// @Param request body UpdateRuleRequest true "规则更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/forward/rules/{id} [put]
func (h *ForwardHandler) UpdateRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	rule, err := h.ruleService.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}

	var req UpdateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新字段
	if req.Name != "" {
		rule.Name = req.Name
	}
	if req.RelayNodeID > 0 {
		rule.RelayNodeID = req.RelayNodeID
	}
	if req.ExitNodeID > 0 {
		rule.ExitNodeID = req.ExitNodeID
	}
	if req.ListenPort > 0 {
		rule.ListenPort = req.ListenPort
	}
	if req.Protocol != "" {
		rule.Protocol = req.Protocol
	}
	if req.TargetHost != "" {
		rule.TargetHost = req.TargetHost
	}
	if req.TargetPort > 0 {
		rule.TargetPort = req.TargetPort
	}
	if req.SpeedLimit != nil {
		rule.SpeedLimit = req.SpeedLimit
	}
	if req.TrafficLimit != nil {
		rule.TrafficLimit = req.TrafficLimit
	}
	if req.ExpireTime != nil {
		rule.ExpireTime = req.ExpireTime
	}
	if req.Remark != "" {
		rule.Remark = req.Remark
	}

	if err := h.ruleService.Update(rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rule})
}

// DeleteRule godoc
// @Summary 删除转发规则
// @Description 管理员删除指定转发规则
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "规则ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/forward/rules/{id} [delete]
func (h *ForwardHandler) DeleteRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.ruleService.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ToggleRule godoc
// @Summary 切换规则状态
// @Description 管理员启用或禁用指定转发规则
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "规则ID"
// @Param request body ToggleRequest true "状态请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/forward/rules/{id}/toggle [post]
func (h *ForwardHandler) ToggleRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req ToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.ruleService.Toggle(uint(id), req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// ========== 统计 ==========

// GetForwardStats godoc
// @Summary 获取转发统计
// @Description 管理员获取转发统计数据，包括节点数、在线数、流量等
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /admin/forward/stats [get]
func (h *ForwardHandler) GetForwardStats(c *gin.Context) {
	// 获取中转节点统计
	relayNodes, _ := h.nodeService.GetByType(model.ForwardNodeTypeRelay)
	exitNodes, _ := h.nodeService.GetByType(model.ForwardNodeTypeExit)

	var totalUpload, totalDownload int64
	var onlineRelay, onlineExit int

	for _, n := range relayNodes {
		totalUpload += n.TotalUpload
		totalDownload += n.TotalDownload
		if n.Status == model.ForwardNodeStatusOnline {
			onlineRelay++
		}
	}

	for _, n := range exitNodes {
		totalUpload += n.TotalUpload
		totalDownload += n.TotalDownload
		if n.Status == model.ForwardNodeStatusOnline {
			onlineExit++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"relay_nodes":    len(relayNodes),
		"exit_nodes":     len(exitNodes),
		"online_relay":   onlineRelay,
		"online_exit":    onlineExit,
		"total_upload":   totalUpload,
		"total_download": totalDownload,
	})
}

// ========== 用户接口 ==========

// GetUserRules godoc
// @Summary 获取用户的转发规则
// @Description 用户获取自己的转发规则列表
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/forward/rules [get]
func (h *ForwardHandler) GetUserRules(c *gin.Context) {
	userID := c.GetUint("user_id")

	rules, err := h.ruleService.GetUserRules(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rules})
}

// CreateUserRule godoc
// @Summary 用户创建转发规则
// @Description 用户创建自己的转发规则
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body service.CreateRuleRequest true "规则信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /user/forward/rules [post]
func (h *ForwardHandler) CreateUserRule(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req service.CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rule, err := h.ruleService.CreateRuleForUser(userID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rule})
}

// ========== 请求结构体 ==========

// CreateNodeRequest 创建节点请求
type CreateNodeRequest struct {
	Name      string `json:"name" binding:"required"`
	Type      string `json:"type" binding:"required,oneof=relay exit"`
	Host      string `json:"host" binding:"required"`
	Port      int    `json:"port" binding:"required,min=1,max=65535"`
	APIPort   int    `json:"api_port"`
	APIToken  string `json:"api_token"`
	Region    string `json:"region"`
	ISP       string `json:"isp"`
	Bandwidth int64  `json:"bandwidth"`
	Weight    int    `json:"weight"`
	MaxConn   int    `json:"max_conn"`
}

// UpdateNodeRequest 更新节点请求
type UpdateNodeRequest struct {
	Name      string `json:"name"`
	Type      string `json:"type" binding:"omitempty,oneof=relay exit"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Region    string `json:"region"`
	ISP       string `json:"isp"`
	Bandwidth int64  `json:"bandwidth"`
	Weight    int    `json:"weight"`
	MaxConn   int    `json:"max_conn"`
	Enabled   *bool  `json:"enabled"`
}

// CreateRuleRequest 创建规则请求
type CreateRuleRequest struct {
	Name         string     `json:"name" binding:"required"`
	RelayNodeID  uint       `json:"relay_node_id" binding:"required"`
	ListenPort   int        `json:"listen_port" binding:"required,min=1,max=65535"`
	Protocol     string     `json:"protocol" binding:"omitempty,oneof=tcp udp both"`
	ExitNodeID   uint       `json:"exit_node_id" binding:"required"`
	TargetHost   string     `json:"target_host" binding:"required"`
	TargetPort   int        `json:"target_port" binding:"required,min=1,max=65535"`
	UserID       *uint      `json:"user_id"`
	UserGroupID  *uint      `json:"user_group_id"`
	SpeedLimit   *int64     `json:"speed_limit"`
	TrafficLimit *int64     `json:"traffic_limit"`
	ExpireTime   *time.Time `json:"expire_time"`
	Remark       string     `json:"remark"`
}

// UpdateRuleRequest 更新规则请求
type UpdateRuleRequest struct {
	Name         string     `json:"name"`
	RelayNodeID  uint       `json:"relay_node_id"`
	ExitNodeID   uint       `json:"exit_node_id"`
	ListenPort   int        `json:"listen_port"`
	Protocol     string     `json:"protocol"`
	TargetHost   string     `json:"target_host"`
	TargetPort   int        `json:"target_port"`
	SpeedLimit   *int64     `json:"speed_limit"`
	TrafficLimit *int64     `json:"traffic_limit"`
	ExpireTime   *time.Time `json:"expire_time"`
	Remark       string     `json:"remark"`
}

// TestGostConnectionRequest 测试 gost 连接请求
type TestGostConnectionRequest struct {
	Host     string `json:"host" binding:"required"`
	APIPort  int    `json:"api_port" binding:"required"`
	APIToken string `json:"api_token"`
}

// TestGostConnection godoc
// @Summary 测试 gost 节点连接
// @Description 测试与 gost 节点的 API 连接是否正常
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body TestGostConnectionRequest true "连接信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /admin/forward/test-connection [post]
func (h *ForwardHandler) TestGostConnection(c *gin.Context) {
	var req TestGostConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	client := gost.NewClient(&gost.Config{
		Host:     fmt.Sprintf("http://%s:%d", req.Host, req.APIPort),
		APIToken: req.APIToken,
	})

	ctx := context.Background()
	if err := client.HealthCheck(ctx); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 获取服务列表验证
	services, err := client.GetServices(ctx)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "API connected but failed to get services: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "Connection successful",
		"service_count": services.Count,
	})
}

// SyncNodeStats godoc
// @Summary 同步节点统计数据
// @Description 从 gost 节点同步流量和连接统计数据到面板
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /admin/forward/nodes/{id}/sync-stats [post]
func (h *ForwardHandler) SyncNodeStats(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	ctx := context.Background()
	stats, err := h.gostManager.GetNodeStats(ctx, uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Stats synced",
		"stats":   stats,
	})
}
