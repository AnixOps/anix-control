package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetObservabilityMultiIngress GET /admin/forward/observability/multi-ingress?targetId=
func (h *ForwardHandler) GetObservabilityMultiIngress(c *gin.Context) {
	var forwardID uint
	if raw := c.Query("targetId"); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 32); err == nil {
			forwardID = uint(parsed)
		}
	}

	rows, err := h.obsService.GetMultiIngressLatency(forwardID)
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, gin.H{"list": rows})
}
