package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NodeTrafficHandler serves a proxy node's traffic series for the admin
// charts: hourly buckets from the raw traffic log and daily buckets from
// the daily server statistics. It reads the tables the traffic reports
// already write and keeps none of its own.
type NodeTrafficHandler struct {
	db func() *gorm.DB
}

// NewNodeTrafficHandler reads the kernel database on each request.
func NewNodeTrafficHandler() *NodeTrafficHandler {
	return &NodeTrafficHandler{db: database.Get}
}

type nodeTrafficPointJSON struct {
	StartUnixMs int64 `json:"start_unix_ms"`
	UpBytes     int64 `json:"up_bytes"`
	DownBytes   int64 `json:"down_bytes"`
}

// parseUnixMs reads an optional Unix millisecond query value.
func parseUnixMs(c *gin.Context, name string) (time.Time, bool) {
	raw := c.Query(name)
	if raw == "" {
		return time.Time{}, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", name+" must be a positive Unix time in milliseconds")
		return time.Time{}, false
	}
	return time.UnixMilli(value), true
}

// Get godoc
// @Summary Proxy node traffic series
// @Description One proxy node's traffic over time for the admin charts, ascending, buckets without traffic as zeros. granularity=hour (the default) sums the node's rows of the raw traffic log v2_server_log in UTC hours: the last 24 hours by default, at most 720 hourly buckets, and only as far back as the operator keeps the raw log (docs/reference/traffic-stats-operations.md). granularity=day reads the node's daily server statistics (v2_stat_server): the last 30 days by default, at most 366 daily buckets, aligned to the Control host's local midnight, kept regardless of the raw log's retention. since and until are Unix milliseconds (since inclusive, until exclusive); they are rounded out to whole buckets, and the answer's since_unix_ms and until_unix_ms are the window actually returned. Bytes are the panel's metered bytes: up and down with the node's traffic rate applied, as /api/v2/admin/traffic/hourly counts them, summed over every protocol the node reported under. This is the proxy node's user traffic; forwarding traffic is /api/v4/forward/stats and /api/v4/forward/routes/{id}/stats.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "proxy node id"
// @Param granularity query string false "hour (default) or day"
// @Param since query int false "window start, Unix milliseconds"
// @Param until query int false "window end (exclusive), Unix milliseconds"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/v4/kernel/nodes/{id}/traffic [get]
func (h *NodeTrafficHandler) Get(c *gin.Context) {
	nodeID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	granularity := c.DefaultQuery("granularity", service.NodeTrafficHour)
	since, ok := parseUnixMs(c, "since")
	if !ok {
		return
	}
	until, ok := parseUnixMs(c, "until")
	if !ok {
		return
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	series, err := service.NewStatsServiceOn(db.WithContext(c.Request.Context())).GetNodeTraffic(nodeID, granularity, since, until)
	switch {
	case errors.Is(err, service.ErrNodeTrafficWindow):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	case errors.Is(err, service.ErrNodeTrafficNode):
		kernelError(c, http.StatusNotFound, "not_found", "node not found")
		return
	case err != nil:
		kernelDBError(c, err)
		return
	}
	points := make([]nodeTrafficPointJSON, 0, len(series.Points))
	var totalUp, totalDown int64
	for _, point := range series.Points {
		points = append(points, nodeTrafficPointJSON{StartUnixMs: point.Start.UnixMilli(), UpBytes: point.Up, DownBytes: point.Down})
		totalUp += point.Up
		totalDown += point.Down
	}
	kernelData(c, http.StatusOK, gin.H{
		"node_id":       series.NodeID,
		"granularity":   series.Granularity,
		"since_unix_ms": series.Since.UnixMilli(),
		"until_unix_ms": series.Until.UnixMilli(),
		"points":        points,
		"total":         gin.H{"up_bytes": totalUp, "down_bytes": totalDown},
	})
}
