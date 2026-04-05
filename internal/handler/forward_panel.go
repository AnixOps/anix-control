package handler

import (
	"net/http"
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

func (h *ForwardHandler) ListPanelForwards(c *gin.Context) {
	items, err := h.panelService.ListForwards(c.GetUint("user_id"), c.GetBool("is_admin"))
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, items)
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
		"code": 1,
		"msg":  msg,
		"ts":   time.Now().UnixMilli(),
	})
}
