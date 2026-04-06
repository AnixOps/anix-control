package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

type panelForwardIDRequest struct {
	ID uint `json:"id" binding:"required"`
}

type panelForwardDiagnoseRequest struct {
	ForwardID uint `json:"forwardId" binding:"required"`
}

type panelForwardOrderRequest struct {
	Forwards []service.PanelForwardOrderUpdate `json:"forwards" binding:"required"`
}

type panelUserTunnelIDRequest struct {
	ID uint `json:"id" binding:"required"`
}

func (h *ForwardHandler) ListPanelForwards(c *gin.Context) {
	items, err := h.panelService.ListForwards(c.GetUint("user_id"), c.GetBool("is_admin"))
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, items)
}

func (h *ForwardHandler) ListPanelRuntimeJobs(c *gin.Context) {
	limit := 50
	if raw := c.DefaultQuery("limit", "50"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	var status *int
	if raw := c.Query("status"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			status = &parsed
		}
	}

	var forwardID *uint
	if raw := c.Query("forward_id"); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 32); err == nil {
			id := uint(parsed)
			forwardID = &id
		}
	}

	jobs, err := h.panelService.ListRuntimeJobs(service.PanelRuntimeJobFilter{
		Backend:   c.Query("backend"),
		Status:    status,
		ForwardID: forwardID,
		Limit:     limit,
	})
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, gin.H{
		"list":  jobs,
		"total": len(jobs),
	})
}

func (h *ForwardHandler) ListPanelTunnels(c *gin.Context) {
	items, err := h.panelService.ListTunnels(c.GetUint("user_id"), c.GetBool("is_admin"))
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, items)
}

func (h *ForwardHandler) CreatePanelForward(c *gin.Context) {
	var req service.PanelForwardInput
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	item, err := h.panelService.CreateForward(c.GetUint("user_id"), c.GetBool("is_admin"), req)
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, item)
}

func (h *ForwardHandler) UpdatePanelForward(c *gin.Context) {
	var req service.PanelForwardUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	item, err := h.panelService.UpdateForward(c.GetUint("user_id"), c.GetBool("is_admin"), req)
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, item)
}

func (h *ForwardHandler) DeletePanelForward(c *gin.Context) {
	var req panelForwardIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if err := h.panelService.DeleteForward(c.GetUint("user_id"), c.GetBool("is_admin"), req.ID, false); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, true)
}

func (h *ForwardHandler) ForceDeletePanelForward(c *gin.Context) {
	var req panelForwardIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if err := h.panelService.DeleteForward(c.GetUint("user_id"), c.GetBool("is_admin"), req.ID, true); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, true)
}

func (h *ForwardHandler) PausePanelForward(c *gin.Context) {
	var req panelForwardIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if err := h.panelService.SetForwardStatus(c.GetUint("user_id"), c.GetBool("is_admin"), req.ID, 0); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, true)
}

func (h *ForwardHandler) ResumePanelForward(c *gin.Context) {
	var req panelForwardIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if err := h.panelService.SetForwardStatus(c.GetUint("user_id"), c.GetBool("is_admin"), req.ID, 1); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, true)
}

func (h *ForwardHandler) DiagnosePanelForward(c *gin.Context) {
	var req panelForwardDiagnoseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	report, err := h.panelService.DiagnoseForward(c.GetUint("user_id"), c.GetBool("is_admin"), req.ForwardID)
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, report)
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

func (h *ForwardHandler) RemovePanelUserTunnel(c *gin.Context) {
	var req panelUserTunnelIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if err := h.panelService.RemoveUserTunnel(req.ID); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, "用户隧道权限删除成功")
}

func (h *ForwardHandler) UpdatePanelUserTunnel(c *gin.Context) {
	var req service.PanelUserTunnelUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if err := h.panelService.UpdateUserTunnel(req); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, "用户隧道权限更新成功")
}

func panelSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "操作成功",
		"ts":   time.Now().UnixMilli(),
		"data": data,
	})
}

func panelError(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{
		"code": -1,
		"msg":  msg,
		"ts":   time.Now().UnixMilli(),
		"data": nil,
	})
}
