package handler

import (
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

type panelSpeedLimitDeleteRequest struct {
	ID uint `json:"id" binding:"required"`
}

type SpeedLimitHandler struct {
	service      *service.SpeedLimitService
	panelService *service.PanelForwardService
}

func NewSpeedLimitHandler() *SpeedLimitHandler {
	db := database.Get()
	return &SpeedLimitHandler{
		service:      service.NewSpeedLimitService(db),
		panelService: service.NewPanelForwardService(db),
	}
}

func (h *SpeedLimitHandler) CreatePanelSpeedLimit(c *gin.Context) {
	var req service.SpeedLimitInput
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if _, err := h.service.Create(req); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, nil)
}

func (h *SpeedLimitHandler) ListPanelSpeedLimits(c *gin.Context) {
	items, err := h.service.List()
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, items)
}

func (h *SpeedLimitHandler) UpdatePanelSpeedLimit(c *gin.Context) {
	var req service.SpeedLimitUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if _, err := h.service.Update(req); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, "限速规则更新成功")
}

func (h *SpeedLimitHandler) DeletePanelSpeedLimit(c *gin.Context) {
	var req panelSpeedLimitDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if err := h.service.Delete(req.ID); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, "限速规则删除成功")
}

func (h *SpeedLimitHandler) ListPanelSpeedLimitTunnels(c *gin.Context) {
	items, err := h.panelService.ListTunnels(c.GetUint("user_id"), true)
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, items)
}
