package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// AdminHandler 管理员处理器
type AdminHandler struct {
	userService    *service.UserService
	orderService   *service.OrderService
	statsService   *service.StatsService
	planService    *service.PlanService
	forwardService *service.PanelForwardService
}

type compatResetFlowRequest struct {
	ID   uint `json:"id" binding:"required"`
	Type int  `json:"type" binding:"required"`
}

// NewAdminHandler 创建管理员处理器
func NewAdminHandler() *AdminHandler {
	return &AdminHandler{
		userService:    service.NewUserService(),
		orderService:   service.NewOrderService(),
		statsService:   service.NewStatsService(),
		planService:    service.NewPlanService(),
		forwardService: service.NewPanelForwardService(database.Get()),
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
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /admin/users [post]
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

// GetUserList godoc
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
// @Success 200 {object} map[string]interface{}
// @Router /admin/users [get]
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

// GetUser godoc
// @Summary 获取用户详情
// @Description 管理员获取用户详细信息
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Router /admin/users/{id} [get]
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

// UpdateUser godoc
// @Summary 更新用户信息
// @Description 管理员更新用户信息，支持修改邮箱、密码、余额、套餐等
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Param request body map[string]interface{} true "用户更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/users/{id} [put]
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

	// 如果更新了套餐，且没有显式提供其他套餐相关字段，则从套餐自动同步
	if req.PlanID != nil {
		updates["plan_id"] = *req.PlanID
		plan, err := h.planService.Get(*req.PlanID)
		if err == nil {
			updates["group_id"] = plan.GroupID
			if req.TransferEnable == nil {
				updates["transfer_enable"] = plan.TransferEnable * 1073741824
			}
			if req.SpeedLimit == nil && plan.SpeedLimit != nil {
				updates["speed_limit"] = *plan.SpeedLimit
			}
			if req.DeviceLimit == nil && plan.DeviceLimit != nil {
				updates["device_limit"] = *plan.DeviceLimit
			}
			// 通常手动修改套餐 ID 时，如果没传过期时间，可能需要根据逻辑处理
			// 但这里我们尊重 req.ExpiredAt 的显式设置（或不设置保持原样）
		}
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

// BanUser godoc
// @Summary 封禁用户
// @Description 管理员封禁指定用户
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/users/{id}/ban [post]
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

// UnbanUser godoc
// @Summary 解封用户
// @Description 管理员解封指定用户
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/users/{id}/unban [post]
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

// ResetUserTraffic godoc
// @Summary 重置用户流量
// @Description 管理员重置指定用户的已使用流量
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/users/{id}/reset-traffic [post]
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

func (h *AdminHandler) ResetCompatFlow(c *gin.Context) {
	var req compatResetFlowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		compatError(c, "参数错误")
		return
	}

	var err error
	switch req.Type {
	case 1:
		err = h.userService.ResetTraffic(req.ID)
	case 2:
		err = h.forwardService.ResetUserTunnelTraffic(req.ID)
	default:
		compatError(c, "type must be 1 or 2")
		return
	}
	if err != nil {
		compatError(c, err.Error())
		return
	}

	compatSuccess(c, nil)
}

func compatSuccess(c *gin.Context, data interface{}) {
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

// DeleteUser godoc
// @Summary 删除用户
// @Description 管理员删除指定用户
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/users/{id} [delete]
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

// ====== 套餐管理 ======

// CreatePlan godoc
// @Summary 创建套餐
// @Description 管理员创建新套餐
// @Tags 管理端-套餐
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.Plan true "套餐信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/plans [post]
func (h *AdminHandler) CreatePlan(c *gin.Context) {
	var p model.Plan
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}
	if err := h.planService.Create(&p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建失败", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "创建成功", "data": p})
}

// GetPlans godoc
// @Summary 获取套餐列表
// @Description 管理员获取所有套餐列表
// @Tags 管理端-套餐
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/plans [get]
func (h *AdminHandler) GetPlans(c *gin.Context) {
	list, err := h.planService.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取失败", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// GetPlan godoc
// @Summary 获取套餐详情
// @Description 管理员获取指定套餐的详细信息
// @Tags 管理端-套餐
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "套餐ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/plans/{id} [get]
func (h *AdminHandler) GetPlan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID 无效"})
		return
	}
	p, err := h.planService.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "套餐不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": p})
}

// UpdatePlan godoc
// @Summary 更新套餐
// @Description 管理员更新指定套餐信息
// @Tags 管理端-套餐
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "套餐ID"
// @Param request body model.Plan true "套餐信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/plans/{id} [put]
func (h *AdminHandler) UpdatePlan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID 无效"})
		return
	}
	var p model.Plan
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}
	p.ID = uint(id)
	if err := h.planService.Update(&p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "更新成功", "data": p})
}

// DeletePlan godoc
// @Summary 删除套餐
// @Description 管理员删除指定套餐
// @Tags 管理端-套餐
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "套餐ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/plans/{id} [delete]
func (h *AdminHandler) DeletePlan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID 无效"})
		return
	}
	if err := h.planService.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除失败", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// AssignPlanToUser godoc
// @Summary 分配套餐给用户
// @Description 管理员将指定套餐分配给用户
// @Tags 管理端-套餐
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "套餐ID"
// @Param request body map[string]interface{} true "分配请求 {user_id: uint, expire_at: int64}"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/plans/{id}/assign [post]
func (h *AdminHandler) AssignPlanToUser(c *gin.Context) {
	planID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "套餐ID 无效"})
		return
	}
	var req struct {
		UserID   uint   `json:"user_id" binding:"required"`
		ExpireAt *int64 `json:"expire_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}
	if err := h.planService.AssignToUser(uint(planID), req.UserID, req.ExpireAt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "分配失败", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "分配成功"})
}

// GetUserStats godoc
// @Summary 获取用户统计
// @Description 管理员获取用户统计数据，包括总用户数、活跃用户等
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/users/stats [get]
func (h *AdminHandler) GetUserStats(c *gin.Context) {
	stats, err := h.userService.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取统计失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// ====== 订单管理 ======

// GetOrderList godoc
// @Summary 获取订单列表
// @Description 管理员获取订单列表，支持分页和筛选
// @Tags 管理端-订单
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param trade_no query string false "订单号"
// @Param email query string false "用户邮箱"
// @Param status query int false "订单状态"
// @Param type query int false "订单类型"
// @Param user_id query int false "用户ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/orders [get]
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

// GetOrder godoc
// @Summary 获取订单详情
// @Description 管理员获取指定订单的详细信息
// @Tags 管理端-订单
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "订单ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/orders/{id} [get]
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

// UpdateOrderStatus godoc
// @Summary 更新订单状态
// @Description 管理员更新指定订单的状态
// @Tags 管理端-订单
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "订单ID"
// @Param request body map[string]interface{} true "状态请求 {status: int}"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/orders/{id}/status [put]
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

// MarkOrderPaid godoc
// @Summary 标记订单已支付
// @Description 管理员手动标记订单为已支付并开通服务
// @Tags 管理端-订单
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "订单ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/orders/{id}/paid [post]
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

// CancelOrder godoc
// @Summary 取消订单
// @Description 管理员取消指定订单
// @Tags 管理端-订单
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "订单ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/orders/{id}/cancel [post]
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

// GetOrderStats godoc
// @Summary 获取订单统计
// @Description 管理员获取订单统计数据
// @Tags 管理端-订单
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/orders/stats [get]
func (h *AdminHandler) GetOrderStats(c *gin.Context) {
	stats, err := h.orderService.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取统计失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// ====== 仪表盘 ======

// GetDashboard godoc
// @Summary 获取仪表盘数据
// @Description 管理员获取仪表盘统计数据，包括用户数、订单数、收入等
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param refresh query bool false "是否强制刷新缓存"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/dashboard [get]
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
