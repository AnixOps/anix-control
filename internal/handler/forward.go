package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/gost"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

// ForwardHandler 转发处理器
type ForwardHandler struct {
	nodeService  *service.ForwardNodeService
	ruleService  *service.ForwardRuleService
	panelService *service.PanelForwardService
	obsService   *service.ForwardObservabilityService
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
		obsService:   service.NewForwardObservabilityService(db),
		gostManager:  gost.NewManager(db),
	}
}

// ========== 中转节点管理 ==========

// ========== 转发规则管理 ==========

// ========== 统计 ==========

// GetForwardStats godoc
// @Summary 获取转发统计
// @Description 管理员获取转发统计数据，包括节点数、在线数、流量等
// @Tags 管理端-转发
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /admin/forward/stats [get]
func (h *ForwardHandler) GetForwardStats(c *gin.Context) {
	// 获取中转节点统计
	relayNodes, err := h.nodeService.GetByType(model.ForwardNodeTypeRelay)
	if err != nil {
		panelError(c, "failed to get relay nodes")
		return
	}
	exitNodes, err := h.nodeService.GetByType(model.ForwardNodeTypeExit)
	if err != nil {
		panelError(c, "failed to get exit nodes")
		return
	}

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

	panelSuccess(c, gin.H{
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
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /user/forward/rules [get]
func (h *ForwardHandler) GetUserRules(c *gin.Context) {
	userID := c.GetUint("user_id")

	rules, err := h.ruleService.GetUserRules(userID)
	if err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, rules)
}

// ========== 请求结构体 ==========

// CreateNodeRequest 创建节点请求
type CreateNodeRequest struct {
	Name        string `json:"name" binding:"required"`
	Type        string `json:"type" binding:"required,oneof=relay exit"`
	Host        string `json:"host" binding:"required"`
	Port        int    `json:"port" binding:"required,min=1,max=65535"`
	APIPort     int    `json:"api_port"`
	APIToken    string `json:"api_token"`
	MetricsPort int    `json:"metrics_port"`
	Region      string `json:"region"`
	ISP         string `json:"isp"`
	Bandwidth   int64  `json:"bandwidth"`
	Weight      int    `json:"weight"`
	MaxConn     int    `json:"max_conn"`
}

// UpdateNodeRequest 更新节点请求
type UpdateNodeRequest struct {
	Name        string `json:"name"`
	Type        string `json:"type" binding:"omitempty,oneof=relay exit"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	APIPort     int    `json:"api_port"`
	APIToken    string `json:"api_token"`
	MetricsPort int    `json:"metrics_port"`
	Region      string `json:"region"`
	ISP         string `json:"isp"`
	Bandwidth   int64  `json:"bandwidth"`
	Weight      int    `json:"weight"`
	MaxConn     int    `json:"max_conn"`
	Enabled     *bool  `json:"enabled"`
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
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /admin/forward/test-connection [post]
func (h *ForwardHandler) TestGostConnection(c *gin.Context) {
	var req TestGostConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, err.Error())
		return
	}

	client := gost.NewClient(&gost.Config{
		Host:     fmt.Sprintf("http://%s:%d", req.Host, req.APIPort),
		APIToken: req.APIToken,
	})

	ctx := context.Background()
	if err := client.HealthCheck(ctx); err != nil {
		panelSuccess(c, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 获取服务列表验证
	services, err := client.GetServices(ctx)
	if err != nil {
		panelSuccess(c, gin.H{
			"success": false,
			"message": "API connected but failed to get services: " + err.Error(),
		})
		return
	}

	panelSuccess(c, gin.H{
		"success":       true,
		"message":       "Connection successful",
		"service_count": services.Count,
	})
}
