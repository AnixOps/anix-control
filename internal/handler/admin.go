package handler

import (
	"net/http"
	"strconv"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// AdminHandler 管理员处理器
type AdminHandler struct {
	userService  *service.UserService
	orderService *service.OrderService
	statsService *service.StatsService
}

// NewAdminHandler 创建管理员处理器
func NewAdminHandler() *AdminHandler {
	return &AdminHandler{
		userService:  service.NewUserService(),
		orderService: service.NewOrderService(),
		statsService: service.NewStatsService(),
	}
}

// ====== 用户管理 ======

// CreateUser 管理员创建用户
func (h *AdminHandler) CreateUser(c *gin.Context) {
	var req model.AdminCreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	db := database.GetDB()

	// 检查邮箱是否已存在
	var existingUser model.User
	if err := db.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "该邮箱已被注册"})
		return
	}

	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "密码加密失败"})
		return
	}

	// 创建用户
	user := &model.User{
		Email:    req.Email,
		Password: string(hashedPassword),
		UUID:     uuid.New().String(),
		Token:    uuid.New().String(),
		IsAdmin:  req.IsAdmin,
		Banned:   0,
	}

	if err := db.Create(user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建用户失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "用户创建成功",
		"data":    user,
	})
}

// GetUserList 获取用户列表
func (h *AdminHandler) GetUserList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	email := c.Query("email")
	status := c.Query("status")

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
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取用户列表失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetUser 获取用户详情
func (h *AdminHandler) GetUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的用户ID"})
		return
	}

	user, err := h.userService.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": user})
}

// UpdateUser 更新用户信息
func (h *AdminHandler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的用户ID"})
		return
	}

	var req struct {
		Email          *string `json:"email"`
		Password       *string `json:"password"`
		Balance        *int64  `json:"balance"`
		PlanID         *uint   `json:"plan_id"`
		ExpiredAt      *int64  `json:"expired_at"`
		TransferEnable *int64  `json:"transfer_enable"`
		SpeedLimit     *int64  `json:"speed_limit"`
		DeviceLimit    *int    `json:"device_limit"`
		Banned         *int    `json:"banned"`
		IsAdmin        *int    `json:"is_admin"`
		RemarkContent  *string `json:"remark_content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.Password != nil && *req.Password != "" {
		// TODO: 密码加密
		updates["password"] = *req.Password
	}
	if req.Balance != nil {
		updates["balance"] = *req.Balance
	}
	if req.PlanID != nil {
		updates["plan_id"] = *req.PlanID
	}
	if req.ExpiredAt != nil {
		updates["expired_at"] = *req.ExpiredAt
	}
	if req.TransferEnable != nil {
		updates["transfer_enable"] = *req.TransferEnable
	}
	if req.SpeedLimit != nil {
		updates["speed_limit"] = *req.SpeedLimit
	}
	if req.DeviceLimit != nil {
		updates["device_limit"] = *req.DeviceLimit
	}
	if req.Banned != nil {
		updates["banned"] = *req.Banned
	}
	if req.IsAdmin != nil {
		updates["is_admin"] = *req.IsAdmin
	}
	if req.RemarkContent != nil {
		updates["remark_content"] = *req.RemarkContent
	}

	if err := h.userService.Update(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// BanUser 封禁用户
func (h *AdminHandler) BanUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的用户ID"})
		return
	}

	if err := h.userService.Ban(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "封禁失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "封禁成功"})
}

// UnbanUser 解封用户
func (h *AdminHandler) UnbanUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的用户ID"})
		return
	}

	if err := h.userService.Unban(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "解封失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "解封成功"})
}

// ResetUserTraffic 重置用户流量
func (h *AdminHandler) ResetUserTraffic(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的用户ID"})
		return
	}

	if err := h.userService.ResetTraffic(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "重置失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "流量重置成功"})
}

// DeleteUser 删除用户
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的用户ID"})
		return
	}

	if err := h.userService.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// GetUserStats 获取用户统计
func (h *AdminHandler) GetUserStats(c *gin.Context) {
	stats, err := h.userService.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取统计失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// ====== 订单管理 ======

// GetOrderList 获取订单列表
func (h *AdminHandler) GetOrderList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	tradeNo := c.Query("trade_no")
	email := c.Query("email")

	var status *int
	if s := c.Query("status"); s != "" {
		statusVal, _ := strconv.Atoi(s)
		status = &statusVal
	}

	var orderType *int
	if t := c.Query("type"); t != "" {
		typeVal, _ := strconv.Atoi(t)
		orderType = &typeVal
	}

	var userID *uint
	if uid := c.Query("user_id"); uid != "" {
		id, _ := strconv.ParseUint(uid, 10, 32)
		userIDVal := uint(id)
		userID = &userIDVal
	}

	result, err := h.orderService.GetList(service.OrderListParams{
		Page:     page,
		PageSize: pageSize,
		UserID:   userID,
		Status:   status,
		Type:     orderType,
		TradeNo:  tradeNo,
		Email:    email,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取订单列表失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetOrder 获取订单详情
func (h *AdminHandler) GetOrder(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的订单ID"})
		return
	}

	order, err := h.orderService.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": order})
}

// UpdateOrderStatus 更新订单状态
func (h *AdminHandler) UpdateOrderStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的订单ID"})
		return
	}

	var req struct {
		Status int `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	if err := h.orderService.UpdateStatus(uint(id), req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// MarkOrderPaid 标记订单已支付 (手动开通)
func (h *AdminHandler) MarkOrderPaid(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的订单ID"})
		return
	}

	// 先标记为已支付
	if err := h.orderService.UpdateStatus(uint(id), 1); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败", "error": err.Error()})
		return
	}

	// 然后完成订单
	if err := h.orderService.Complete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "订单完成失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "订单已开通"})
}

// CancelOrder 取消订单
func (h *AdminHandler) CancelOrder(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的订单ID"})
		return
	}

	if err := h.orderService.Cancel(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "取消失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "取消成功"})
}

// GetOrderStats 获取订单统计
func (h *AdminHandler) GetOrderStats(c *gin.Context) {
	stats, err := h.orderService.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取统计失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// ====== 仪表盘 ======

// GetDashboard 获取仪表盘数据 (优先从缓存读取)
func (h *AdminHandler) GetDashboard(c *gin.Context) {
	// 检查是否强制刷新
	forceRefresh := c.Query("refresh") == "true"

	stats, err := h.statsService.GetDashboardStats(forceRefresh)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取统计失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": stats,
	})
}
