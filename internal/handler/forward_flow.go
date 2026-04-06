package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

type panelForwardTrafficRequest struct {
	Records []service.PanelForwardTrafficRecord `json:"records" binding:"required"`
}

type panelForwardTrafficSnapshotRequest struct {
	Records []service.PanelForwardTrafficSnapshot `json:"records" binding:"required"`
}

func (h *ForwardHandler) UploadPanelFlowData(c *gin.Context) {
	defer c.String(http.StatusOK, "ok")

	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil || len(rawBody) == 0 {
		return
	}

	var payload service.PanelForwardFlowData
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return
	}

	_ = h.panelService.UploadFluxForwardFlow(payload)
}

func (h *ForwardHandler) ReportPanelForwardTraffic(c *gin.Context) {
	var req panelForwardTrafficRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if err := h.panelService.RecordForwardTraffic(req.Records); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, true)
}

func (h *ForwardHandler) SnapshotPanelForwardTraffic(c *gin.Context) {
	var req panelForwardTrafficSnapshotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	if err := h.panelService.ApplyForwardTrafficSnapshots(req.Records); err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, true)
}
