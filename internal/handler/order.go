package handler

import (
	"net/http"
	"strconv"

	"github.com/anixops/v2board/internal/service"
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

// GetOrders 获取用户自己的订单
func (h *OrderHandler) GetOrders(c *gin.Context) {
	userID := c.GetUint("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	res, err := h.orderService.GetUserOrders(userID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取订单列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": res})
}

// SaveOrder 保存订单 (下单)
func (h *OrderHandler) SaveOrder(c *gin.Context) {
	userID := c.GetUint("user_id")
	var req struct {
		PlanID   uint   `json:"plan_id" binding:"required"`
		Period   string `json:"period" binding:"required"`
		CouponID *uint  `json:"coupon_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	order, err := h.orderService.Create(service.CreateOrderParams{
		UserID:   userID,
		PlanID:   req.PlanID,
		Period:   req.Period,
		CouponID: req.CouponID,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "订单已提交",
		"data":    order,
	})
}

// GetOrderDetail 获取订单详情
func (h *OrderHandler) GetOrderDetail(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID无效"})
		return
	}

	order, err := h.orderService.GetByID(uint(id))
	if err != nil || order.UserID != userID {
		c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": order})
}
