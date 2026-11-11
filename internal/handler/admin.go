package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/subscriber"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
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

// CreateUser godoc
// @Summary 创建用户
// @Description 管理员创建新用户
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.AdminCreateUserRequest true "用户信息"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /admin/users [post]
func (h *AdminHandler) CreateUser(c *gin.Context) {
	var req model.AdminCreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误: "+err.Error())
		return
	}

	db := database.GetDB()

	// 检查邮箱是否已存在
	var existingUser model.User
	if err := db.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		panelError(c, "该邮箱已被注册")
		return
	}

	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		panelError(c, "密码加密失败")
		return
	}

	// 创建用户
	user := &model.User{
		Email:         req.Email,
		Password:      string(hashedPassword),
		UUID:          uuid.New().String(),
		Token:         uuid.New().String(),
		IsAdmin:       req.IsAdmin,
		FlowResetTime: req.FlowResetTime,
		GroupID:       req.GroupID,
		Banned:        0,
	}

	if req.TransferEnable != nil {
		user.TransferEnable = *req.TransferEnable
	}
	if req.SpeedLimit != nil {
		user.SpeedLimit = req.SpeedLimit
	}
	if req.DeviceLimit != nil {
		user.DeviceLimit = req.DeviceLimit
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return subscriber.RecordChangesTx(tx, []uint{user.ID}, false, time.Now())
	}); err != nil {
		panelError(c, "创建用户失败: "+err.Error())
		return
	}

	panelSuccess(c, user)
}

// GetUserList godoc
// Each user carries the account and subscription summary
// (service.UserListItem), never the subscription token or proxy uuid, which
// the user detail answers for one user.
// @Summary 获取用户列表
// @Description 管理员获取用户列表
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param email query string false "邮箱搜索"
// @Param status query string false "状态筛选"
// @Param plan_id query int false "套餐ID筛选"
// @Param sort query string false "排序字段: id, email, created_at, expired_at, traffic, transfer_enable; 缺省为 created_at 倒序" Enums(id, email, created_at, expired_at, traffic, transfer_enable)
// @Param order query string false "排序方向, 仅与 sort 同用" Enums(asc, desc) default(asc)
// @Success 200 {object} map[string]any
// @Router /admin/users [get]
func (h *AdminHandler) GetUserList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = ClampPagination(page, pageSize)
	email := c.Query("email")
	status := c.Query("status")
	orderBy, err := v2compat.ParseListSort(c.Query("sort"), c.Query("order"), service.UserSortColumns, service.UserSortTiebreaker)
	if err != nil {
		panelError(c, err.Error())
		return
	}

	var planID *uint
	if pid := c.Query("plan_id"); pid != "" {
		id, _ := strconv.ParseUint(pid, 10, 32)
		planIDVal := uint(id)
		planID = &planIDVal
	}

	result, err := h.userService.GetList(service.UserListParams{
		Page:     page,
		PageSize: pageSize,
		Email:    email,
		PlanID:   planID,
		Status:   status,
		OrderBy:  orderBy,
	})
	if err != nil {
		panelError(c, "获取用户列表失败: "+err.Error())
		return
	}

	panelSuccess(c, result)
}

// GetUser godoc
// @Summary 获取用户详情
// @Description 管理员获取用户详细信息
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]any
// @Router /admin/users/{id} [get]
func (h *AdminHandler) GetUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的用户ID")
		return
	}

	user, err := h.userService.GetByID(uint(id))
	if err != nil {
		panelError(c, "用户不存在")
		return
	}

	panelSuccess(c, user)
}

// UpdateUser godoc
// @Summary 更新用户信息
// @Description 管理员更新用户信息，支持修改邮箱、密码、余额、套餐等
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Param request body map[string]any true "用户更新信息"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/users/{id} [put]
func (h *AdminHandler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的用户ID")
		return
	}

	var req struct {
		Email          *string `json:"email"`
		Password       *string `json:"password"`
		Balance        *int64  `json:"balance"`
		PlanID         *uint   `json:"plan_id" binding:"omitempty,gt=0"`
		GroupID        *uint   `json:"group_id"`
		ExpiredAt      *int64  `json:"expired_at"`
		TransferEnable *int64  `json:"transfer_enable"`
		SpeedLimit     *int64  `json:"speed_limit"`
		DeviceLimit    *int    `json:"device_limit"`
		FlowResetTime  *int64  `json:"flowResetTime"`
		Banned         *int    `json:"banned"`
		IsAdmin        *int    `json:"is_admin"`
		RemarkContent  *string `json:"remark_content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误: "+err.Error())
		return
	}

	updates := make(map[string]any)
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.Password != nil && *req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			panelError(c, "密码加密失败")
			return
		}
		updates["password"] = string(hash)
	}
	// 套餐、流量、限速等权益字段（更换套餐时从套餐同步）
	entitlements := service.SubscriberEntitlements{
		Balance: req.Balance, PlanID: req.PlanID, GroupID: req.GroupID, ExpiredAt: req.ExpiredAt,
		TransferEnable: req.TransferEnable, SpeedLimit: req.SpeedLimit, DeviceLimit: req.DeviceLimit,
		FlowResetTime: req.FlowResetTime, RemarkContent: req.RemarkContent,
	}
	for column, value := range entitlements.Updates(h.planService) {
		updates[column] = value
	}
	if req.Banned != nil {
		updates["banned"] = *req.Banned
	}
	if req.IsAdmin != nil {
		updates["is_admin"] = *req.IsAdmin
	}

	if err := h.userService.Update(uint(id), updates); err != nil {
		panelAdminUserError(c, "更新失败", err)
		return
	}

	panelSuccess(c, gin.H{"message": "更新成功"})
}

// BanUser godoc
// @Summary 封禁用户
// @Description 管理员封禁指定用户
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/users/{id}/ban [post]
func (h *AdminHandler) BanUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的用户ID")
		return
	}

	if err := h.userService.Ban(uint(id)); err != nil {
		panelAdminUserError(c, "封禁失败", err)
		return
	}

	panelSuccess(c, gin.H{"message": "封禁成功"})
}

// UnbanUser godoc
// @Summary 解封用户
// @Description 管理员解封指定用户
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/users/{id}/unban [post]
func (h *AdminHandler) UnbanUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的用户ID")
		return
	}

	if err := h.userService.Unban(uint(id)); err != nil {
		panelAdminUserError(c, "解封失败", err)
		return
	}

	panelSuccess(c, gin.H{"message": "解封成功"})
}

// ResetUserTraffic godoc
// @Summary 重置用户流量
// @Description 管理员重置指定用户的已使用流量
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/users/{id}/reset-traffic [post]
func (h *AdminHandler) ResetUserTraffic(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的用户ID")
		return
	}

	requestID := service.AdminUserResetRequestID(service.AdminUserResetTraffic, uint(id), assignmentToken(c))
	if err := h.userService.ResetTraffic(uint(id), requestID); err != nil {
		panelAdminUserError(c, "重置失败", err)
		return
	}

	panelSuccess(c, gin.H{"message": "流量重置成功"})
}

// ResetUserSubscribe godoc
// @Summary 重置用户订阅
// @Description 管理员为用户重新生成订阅 token, 旧订阅链接立即失效 (UUID 不变)
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/users/{id}/reset-subscribe [post]
func (h *AdminHandler) ResetUserSubscribe(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的用户ID")
		return
	}

	requestID := service.AdminUserResetRequestID(service.AdminUserResetSubscribe, uint(id), assignmentToken(c))
	newToken, err := h.userService.ResetToken(uint(id), requestID)
	if err != nil {
		panelAdminUserError(c, "重置订阅失败", err)
		return
	}

	panelSuccess(c, gin.H{"token": newToken})
}

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

func panelAdminUserError(c *gin.Context, fallback string, err error) {
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		panelError(c, err.Error())
	default:
		panelError(c, fallback+": "+err.Error())
	}
}

// DeleteUser godoc
// @Summary 删除用户
// @Description 管理员删除指定用户
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/users/{id} [delete]
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的用户ID")
		return
	}

	if err := h.userService.Delete(uint(id)); err != nil {
		panelAdminUserError(c, "删除失败", err)
		return
	}

	panelSuccess(c, gin.H{"message": "删除成功"})
}

// assignmentToken identifies an administrator's request for
// service.AdminUserResetRequestID and the
// subscription group request ids (service.SubscriptionGroupGrantRequestID
// and its kin): its Idempotency-Key, else its request id (X-Request-ID or
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

// GetUserStats godoc
// @Summary 获取用户统计
// @Description 管理员获取用户统计数据，包括总用户数、活跃用户等
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/users/stats [get]
func (h *AdminHandler) GetUserStats(c *gin.Context) {
	stats, err := h.userService.GetStats()
	if err != nil {
		panelError(c, "获取统计失败: "+err.Error())
		return
	}

	panelSuccess(c, stats)
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
