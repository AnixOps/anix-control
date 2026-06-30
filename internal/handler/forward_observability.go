package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// parseUnixMilliQuery parses a unix-milli query param, returning fallback when absent/invalid.
func parseUnixMilliQuery(c *gin.Context, key string, fallback time.Time) time.Time {
	raw := c.Query(key)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return time.UnixMilli(parsed)
}

// ListObservabilityTargets GET /admin/forward/observability/targets
func (h *ForwardHandler) ListObservabilityTargets(c *gin.Context) {
	items, err := h.obsService.ListTargets()
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, gin.H{"list": items, "total": len(items)})
}

// GetObservabilityTrend GET /admin/forward/observability/trend?targetKey=&from=&to=
func (h *ForwardHandler) GetObservabilityTrend(c *gin.Context) {
	now := time.Now()
	to := parseUnixMilliQuery(c, "to", now)
	from := parseUnixMilliQuery(c, "from", to.Add(-time.Hour))

	trend, err := h.obsService.GetLatencyTrend(c.Query("targetKey"), from, to)
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, trend)
}

// GetObservabilityTopology GET /admin/forward/observability/topology
func (h *ForwardHandler) GetObservabilityTopology(c *gin.Context) {
	topo, err := h.obsService.GetTopology()
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, topo)
}

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
