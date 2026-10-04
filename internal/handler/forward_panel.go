package handler

import (
	"net/http"
	"time"

	"github.com/AnixOps/anix-control/sdk/v2compat"

	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

type panelForwardIDRequest struct {
	ID uint `json:"id" binding:"required"`
}

type panelForwardOrderRequest struct {
	Forwards []service.PanelForwardOrderUpdate `json:"forwards" binding:"required"`
}

func (h *ForwardHandler) ListPanelForwards(c *gin.Context) {
	items, err := h.panelService.ListForwards(c.GetUint("user_id"), c.GetBool("is_admin"))
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, items)
}

func (h *ForwardHandler) GetPanelRuntimeStatus(c *gin.Context) {
	status, err := h.panelService.GetRuntimeStatus(c.Request.Context())
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, status)
}

func (h *ForwardHandler) GetNodeXRuntimeStatus(c *gin.Context) {
	status, err := h.panelService.GetNodeXOperatorStatus(c.Request.Context())
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, status)
}

func (h *ForwardHandler) GetLocalRuntimeStatus(c *gin.Context) {
	status, err := h.panelService.GetLocalOperatorStatus(c.Request.Context())
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, status)
}

func (h *ForwardHandler) DiagnosePanelRuntime(c *gin.Context) {
	summary, err := h.panelService.DiagnoseRuntime(c.Request.Context())
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, summary)
}

func (h *ForwardHandler) DiagnoseNodeXRuntime(c *gin.Context) {
	summary, err := h.panelService.DiagnoseNodeXOperator(c.Request.Context())
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, summary)
}

func (h *ForwardHandler) DiagnoseLocalRuntime(c *gin.Context) {
	summary, err := h.panelService.DiagnoseLocalOperator(c.Request.Context())
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, summary)
}

func (h *ForwardHandler) ListPanelTunnels(c *gin.Context) {
	items, err := h.panelService.ListTunnels(c.GetUint("user_id"), c.GetBool("is_admin"))
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, items)
}

func (h *ForwardHandler) ListPanelAdminTunnels(c *gin.Context) {
	items, err := h.panelService.ListAdminTunnels()
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, items)
}

func (h *ForwardHandler) CreatePanelTunnel(c *gin.Context) {
	var req service.PanelTunnelInput
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	item, err := h.panelService.CreateTunnel(req)
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, item)
}

func (h *ForwardHandler) DeletePanelTunnel(c *gin.Context) {
	var req panelForwardIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if err := h.panelService.DeleteTunnel(req.ID); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, true)
}

func (h *ForwardHandler) UpdatePanelForwardOrder(c *gin.Context) {
	var req panelForwardOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if err := h.panelService.UpdateOrder(c.GetUint("user_id"), c.GetBool("is_admin"), req.Forwards); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, true)
}

func (h *ForwardHandler) AssignPanelUserTunnel(c *gin.Context) {
	var req service.PanelUserTunnelInput
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if err := h.panelService.AssignUserTunnel(req); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, "用户隧道权限分配成功")
}

func (h *ForwardHandler) ListPanelUserTunnels(c *gin.Context) {
	var req service.PanelUserTunnelQueryInput
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	items, err := h.panelService.ListUserTunnels(req)
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, items)
}

// panelSuccess and panelError write the panel envelope through pkg/v2compat,
// which package-native implementations use as well.
func panelSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, v2compat.PanelSuccess(data, time.Now()))
}

func panelError(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, v2compat.PanelError(msg, time.Now()))
}
