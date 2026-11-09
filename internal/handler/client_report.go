package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/clientreport"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ClientReportHandler takes AnixOps Net clients' failure reports on the
// subscription's own address (POST /{subscribe_path}/{token}/report,
// internal/clientreport) and answers them to administrators.
type ClientReportHandler struct {
	db      func() *gorm.DB
	limiter *clientreport.Limiter
	now     func() time.Time
	// servesHere is the subscription address's host rule (subscriptions
	// only on the subscription domains when so configured); nil serves
	// every host.
	servesHere func(*gin.Context) bool
}

// NewClientReportHandler reads the database on each request and answers
// reports on the hosts subscriptions answers on.
func NewClientReportHandler(subscriptions *SubscribeHandler) *ClientReportHandler {
	h := &ClientReportHandler{db: database.Get, limiter: clientreport.NewLimiter(), now: time.Now}
	if subscriptions != nil {
		h.servesHere = subscriptions.servesSubscriptionsHere
	}
	return h
}

// Report godoc
// @Summary Report connection outcomes (AnixOps Net)
// @Description A client's connection outcomes per node of its subscription over window_secs (60-86400): ok, fallback, timeout, refused, reset, after_handshake, keepalive_lost, other (each 0-1000000), stopped, clock_skew_secs; at most 200 nodes, unknown fields ignored, no destinations. The token is the subscription's, in the path. 204 on success, 404 for an unknown token, 400 out of bounds, 429 when the token reported within 10 minutes (drop the report).
// @Tags Subscription
// @Accept json
// @Param token path string true "subscription token"
// @Param request body clientreport.Report true "the report"
// @Success 204
// @Router /s/{token}/report [post]
func (h *ClientReportHandler) Report(c *gin.Context) {
	if h.servesHere != nil && !h.servesHere(c) {
		c.Status(http.StatusNotFound)
		return
	}
	token := c.Param("token")
	if i := strings.LastIndex(token, "."); i > 0 {
		token = token[:i]
	}
	if token == "" || len(token) > 128 {
		c.Status(http.StatusNotFound)
		return
	}
	db := h.db().WithContext(c.Request.Context())
	var users []model.User
	if err := db.Select("id").Where("token = ?", token).Limit(1).Find(&users).Error; err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	if len(users) == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, clientreport.MaxBodyBytes)
	var report clientreport.Report
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "the body is not a report"}})
		return
	}
	if err := report.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": err.Error()}})
		return
	}
	now := h.now().UTC()
	if !h.limiter.Allow(strconv.FormatUint(uint64(users[0].ID), 10), now) {
		c.Header("Retry-After", strconv.Itoa(int(clientreport.RateWindow.Seconds())))
		c.Status(http.StatusTooManyRequests)
		return
	}
	if err := clientreport.Record(c.Request.Context(), h.db(), users[0].ID, report, now); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

// Summary godoc
// @Summary Client failure reports by node
// @Description What AnixOps Net clients reported about each node over the last hours (default 24, at most 336): the outcomes by cause, how many subscribers reported it and gave up on it, outcomes by kind of network, the failure ratio; most failing first. Never a subscriber's identity.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param hours query int false "hours to look back"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/client-failures [get]
func (h *ClientReportHandler) Summary(c *gin.Context) {
	hours := 24
	if raw := c.Query("hours"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			kernelError(c, http.StatusBadRequest, "invalid_request", "hours must be a number")
			return
		}
		hours = n
	}
	nodes, err := clientreport.Summary(c.Request.Context(), h.db(), hours, h.now())
	if err != nil {
		if errors.Is(err, clientreport.ErrInvalid) {
			kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"hours": hours, "nodes": nodes})
}
