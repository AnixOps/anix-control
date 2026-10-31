package handler

// The administrator's order routes. They live apart from admin.go because they
// belong to the commercial set (config/commercial-boundary.json).

import (
	"errors"
	"strconv"

	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
)

// GetOrderList godoc
// Each order carries its plan's id and name and its buyer's id and e-mail
// (service.OrderView), never the rest of their rows.
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
// @Param sort query string false "排序字段: id, trade_no, status, type, total_amount, created_at, paid_at; 缺省为 created_at 倒序" Enums(id, trade_no, status, type, total_amount, created_at, paid_at)
// @Param order query string false "排序方向, 仅与 sort 同用" Enums(asc, desc) default(asc)
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/orders [get]
func (h *AdminHandler) GetOrderList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = ClampPagination(page, pageSize)
	tradeNo := c.Query("trade_no")
	email := c.Query("email")
	orderBy, err := v2compat.ParseListSort(c.Query("sort"), c.Query("order"), service.OrderSortColumns, service.OrderSortTiebreaker)
	if err != nil {
		panelError(c, err.Error())
		return
	}

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

	result, err := service.NewOrderService().GetList(service.OrderListParams{
		Page:     page,
		PageSize: pageSize,
		UserID:   userID,
		Status:   status,
		Type:     orderType,
		TradeNo:  tradeNo,
		Email:    email,
		OrderBy:  orderBy,
	})
	if err != nil {
		panelError(c, "获取订单列表失败: "+err.Error())
		return
	}

	panelSuccess(c, result)
}

// GetOrder godoc
// The order with its plan's id and name and its buyer's id and e-mail
// (service.OrderView).
// @Summary 获取订单详情
// @Description 管理员获取指定订单的详细信息
// @Tags 管理端-订单
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "订单ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /admin/orders/{id} [get]
func (h *AdminHandler) GetOrder(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的订单ID")
		return
	}

	order, err := service.NewOrderService().GetAdminView(uint(id))
	if err != nil {
		panelAdminOrderError(c, "获取订单失败", err)
		return
	}

	panelSuccess(c, order)
}

// UpdateOrderStatus godoc
// @Summary 更新订单状态
// @Description 管理员更新指定订单的状态
// @Tags 管理端-订单
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "订单ID"
// @Param request body map[string]any true "状态请求 {status: int}"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/orders/{id}/status [put]
func (h *AdminHandler) UpdateOrderStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的订单ID")
		return
	}

	var req struct {
		Status int `json:"status" binding:"required,oneof=0 1 2 3"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误: "+err.Error())
		return
	}

	if err := service.NewOrderService().UpdateStatus(uint(id), req.Status); err != nil {
		panelAdminOrderError(c, "更新失败", err)
		return
	}

	panelSuccess(c, gin.H{"message": "更新成功"})
}

// MarkOrderPaid godoc
// @Summary 标记订单已支付
// @Description 管理员手动标记订单为已支付并开通服务
// @Tags 管理端-订单
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "订单ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/orders/{id}/paid [post]
func (h *AdminHandler) MarkOrderPaid(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的订单ID")
		return
	}

	// 先标记为已支付
	if err := service.NewOrderService().UpdateStatus(uint(id), 1); err != nil {
		panelAdminOrderError(c, "更新失败", err)
		return
	}

	// 然后完成订单
	if err := service.NewOrderService().Complete(uint(id)); err != nil {
		panelAdminOrderError(c, "订单完成失败", err)
		return
	}

	panelSuccess(c, gin.H{"message": "订单已开通"})
}

// CancelOrder godoc
// @Summary 取消订单
// @Description 管理员取消指定订单
// @Tags 管理端-订单
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "订单ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/orders/{id}/cancel [post]
func (h *AdminHandler) CancelOrder(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的订单ID")
		return
	}

	if err := service.NewOrderService().Cancel(uint(id)); err != nil {
		panelAdminOrderError(c, "取消失败", err)
		return
	}

	panelSuccess(c, gin.H{"message": "取消成功"})
}

func panelAdminOrderError(c *gin.Context, fallback string, err error) {
	switch {
	case errors.Is(err, service.ErrOrderNotFound), errors.Is(err, service.ErrOrderPlanNotFound):
		panelError(c, err.Error())
	default:
		panelError(c, fallback+": "+err.Error())
	}
}

// GetOrderStats godoc
// @Summary 获取订单统计
// @Description 管理员获取订单统计数据
// @Tags 管理端-订单
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/orders/stats [get]
func (h *AdminHandler) GetOrderStats(c *gin.Context) {
	stats, err := service.NewOrderService().GetStats()
	if err != nil {
		panelError(c, "获取统计失败: "+err.Error())
		return
	}

	panelSuccess(c, stats)
}
