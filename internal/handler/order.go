package handler

import (
	"errors"
	"log"
	"strconv"

	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

// OrderHandler 用户端订单处理器
type OrderHandler struct {
	orderService *service.OrderService
}

// NewOrderHandler 创建订单处理器
func NewOrderHandler() *OrderHandler {
	return &OrderHandler{
		orderService: service.NewOrderService(),
	}
}

// GetOrders 获取用户自己的订单: each with its plan's id and name, none with
// the buyer. The page size is clamped as on the administrator's list.
func (h *OrderHandler) GetOrders(c *gin.Context) {
	userID := c.GetUint("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	page, pageSize = ClampPagination(page, pageSize)

	res, err := h.orderService.GetUserOrders(userID, page, pageSize)
	if err != nil {
		log.Printf("user order list failed: %v", err)
		panelError(c, "获取订单列表失败")
		return
	}

	panelSuccess(c, res)
}

// SaveOrder 保存订单 (下单)
func (h *OrderHandler) SaveOrder(c *gin.Context) {
	userID := c.GetUint("user_id")
	var req struct {
		PlanID   uint   `json:"plan_id" binding:"required,gt=0"`
		Period   string `json:"period" binding:"required,min=1"`
		CouponID *uint  `json:"coupon_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	order, err := h.orderService.Create(service.CreateOrderParams{
		UserID:   userID,
		PlanID:   req.PlanID,
		Period:   req.Period,
		CouponID: req.CouponID,
	})

	if err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, order)
}

// GetOrderDetail 获取订单详情: the caller's own order with its plan's id and
// name. Another user's order is "not found", as an unknown one.
func (h *OrderHandler) GetOrderDetail(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "ID无效")
		return
	}

	order, err := h.orderService.GetUserView(userID, uint(id))
	if err != nil {
		if !errors.Is(err, service.ErrOrderNotFound) {
			log.Printf("user order detail lookup failed: %v", err)
		}
		panelError(c, "订单不存在")
		return
	}

	panelSuccess(c, order)
}
