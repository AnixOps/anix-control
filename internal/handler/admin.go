package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// BuildInfo is set via ldflags at build time.
var (
	BuildVersion = "dev"
	BuildTime    = "unknown"
	BuildCode    = ""
	BuildCommit  = "unknown"
)

// AdminHandler 管理员处理器
type AdminHandler struct {
	userService  *service.UserService
	statsService *service.StatsService
	planService  *service.PlanService
}

type compatResetFlowRequest struct {
	ID   uint `json:"id" binding:"required"`
	Type int  `json:"type" binding:"required"`
}

// NewAdminHandler 创建管理员处理器
func NewAdminHandler() *AdminHandler {
	return &AdminHandler{
		userService:  service.NewUserService(),
		statsService: service.NewStatsService(),
		planService:  service.NewPlanService(),
	}
}

// ====== 用户管理 ======

// ResetCompatFlow is the Flux-style POST /api/v2/user/reset: type 1 zeroes a
// subscriber's traffic, under service.ForwardUserResetRequestID in the
// subscriber request ledger so that a retry applies once whichever side
// serves it (the forward package serves the route natively too); type 2
// zeroes a tunnel permission's traffic and that of its forwards.
func (h *AdminHandler) ResetCompatFlow(c *gin.Context) {
	var req compatResetFlowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		compatError(c, "参数错误")
		return
	}

	var err error
	switch req.Type {
	case 1:
		err = h.userService.ResetTraffic(req.ID, service.ForwardUserResetRequestID(req.ID, assignmentToken(c)))
	default:
		compatError(c, "type must be 1")
		return
	}
	if err != nil {
		compatError(c, err.Error())
		return
	}

	compatSuccess(c, nil)
}

func compatSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "操作成功",
		"ts":   time.Now().UnixMilli(),
		"data": data,
	})
}

func compatError(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{
		"code": -1,
		"msg":  msg,
		"ts":   time.Now().UnixMilli(),
		"data": nil,
	})
}

// assignmentToken identifies an administrator's request for
// service.AdminUserResetRequestID: its Idempotency-Key, else its request id (X-Request-ID or
// the one the kernel gave it), else a fresh value.
func assignmentToken(c *gin.Context) string {
	if key := strings.TrimSpace(c.GetHeader("Idempotency-Key")); key != "" {
		return key
	}
	if id := strings.TrimSpace(c.GetString("request_id")); id != "" {
		return id
	}
	if id := strings.TrimSpace(c.GetHeader("X-Request-ID")); id != "" {
		return id
	}
	return uuid.NewString()
}

// GET /api/v2/admin/system/info
func (h *AdminHandler) GetSystemInfo(c *gin.Context) {
	panelSuccess(c, gin.H{
		"version":    BuildVersion,
		"build_time": BuildTime,
		"build_code": BuildCode,
		"commit":     BuildCommit,
	})
}
