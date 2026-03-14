package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// PaymentGatewayHandler 支付网关处理器
type PaymentGatewayHandler struct {
	gatewayService *service.PaymentGatewayService
}

// NewPaymentGatewayHandler 创建处理器
func NewPaymentGatewayHandler() *PaymentGatewayHandler {
	return &PaymentGatewayHandler{
		gatewayService: service.NewPaymentGatewayService(database.Get()),
	}
}

// ListGateways godoc
// @Summary 获取支付网关列表
// @Description 管理员获取所有支付网关列表
// @Tags 管理端-支付
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/payment/gateways [get]
func (h *PaymentGatewayHandler) ListGateways(c *gin.Context) {
	gateways, err := h.gatewayService.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gateways})
}

// CreateGateway godoc
// @Summary 创建支付网关
// @Description 管理员创建新的支付网关
// @Tags 管理端-支付
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateGatewayRequest true "网关信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/payment/gateways [post]
func (h *PaymentGatewayHandler) CreateGateway(c *gin.Context) {
	var req CreateGatewayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	gateway := &model.PaymentGateway{
		Name:        req.Name,
		Type:        req.Type,
		Enabled:     false,
		Icon:        req.Icon,
		Config:      req.Config,
		FeeRate:     req.FeeRate,
		FeeFixed:    req.FeeFixed,
		MinAmount:   req.MinAmount,
		MaxAmount:   req.MaxAmount,
		Sort:        req.Sort,
		Description: req.Description,
	}

	if err := h.gatewayService.Create(gateway); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gateway})
}

// UpdateGateway godoc
// @Summary 更新支付网关
// @Description 管理员更新指定支付网关的信息
// @Tags 管理端-支付
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "网关ID"
// @Param request body UpdateGatewayRequest true "网关更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/payment/gateways/{id} [put]
func (h *PaymentGatewayHandler) UpdateGateway(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	gateway, err := h.gatewayService.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "gateway not found"})
		return
	}

	var req UpdateGatewayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		gateway.Name = req.Name
	}
	if req.Icon != "" {
		gateway.Icon = req.Icon
	}
	if req.Config != "" {
		gateway.Config = req.Config
	}
	if req.FeeRate >= 0 {
		gateway.FeeRate = req.FeeRate
	}
	if req.FeeFixed >= 0 {
		gateway.FeeFixed = req.FeeFixed
	}
	if req.MinAmount > 0 {
		gateway.MinAmount = req.MinAmount
	}
	if req.MaxAmount > 0 {
		gateway.MaxAmount = req.MaxAmount
	}
	if req.Sort >= 0 {
		gateway.Sort = req.Sort
	}
	if req.Description != "" {
		gateway.Description = req.Description
	}

	if err := h.gatewayService.Update(gateway); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gateway})
}

// DeleteGateway godoc
// @Summary 删除支付网关
// @Description 管理员删除指定支付网关
// @Tags 管理端-支付
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "网关ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/payment/gateways/{id} [delete]
func (h *PaymentGatewayHandler) DeleteGateway(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.gatewayService.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ToggleGateway godoc
// @Summary 切换网关状态
// @Description 管理员启用或禁用指定支付网关
// @Tags 管理端-支付
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "网关ID"
// @Param request body ToggleRequest true "状态请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/payment/gateways/{id}/toggle [post]
func (h *PaymentGatewayHandler) ToggleGateway(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req ToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.gatewayService.Toggle(uint(id), req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// GetPaymentStats godoc
// @Summary 获取支付统计
// @Description 管理员获取支付统计数据，支持日期范围筛选
// @Tags 管理端-支付
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param start query string false "开始日期 (格式: 2006-01-02)"
// @Param end query string false "结束日期 (格式: 2006-01-02)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/payment/stats [get]
func (h *PaymentGatewayHandler) GetPaymentStats(c *gin.Context) {
	startStr := c.DefaultQuery("start", time.Now().AddDate(0, 0, -30).Format("2006-01-02"))
	endStr := c.DefaultQuery("end", time.Now().Format("2006-01-02"))

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start date"})
		return
	}

	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end date"})
		return
	}

	stats, err := h.gatewayService.GetStats(start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// ListPaymentRecords godoc
// @Summary 获取支付记录列表
// @Description 管理员获取支付记录列表，支持分页和状态筛选
// @Tags 管理端-支付
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param status query int false "支付状态"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/payment/records [get]
func (h *PaymentGatewayHandler) ListPaymentRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	var status *int
	if s := c.Query("status"); s != "" {
		st, _ := strconv.Atoi(s)
		status = &st
	}

	records, total, err := h.gatewayService.ListRecords(page, pageSize, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      records,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ========== 用户接口 ==========

// GetChannels godoc
// @Summary 获取支付渠道列表
// @Description 用户获取可用的支付渠道列表
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/payment/channels [get]
func (h *PaymentGatewayHandler) GetChannels(c *gin.Context) {
	channels, err := h.gatewayService.GetChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": channels})
}

// CreatePayment godoc
// @Summary 创建支付订单
// @Description 用户创建支付订单
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreatePaymentRequest true "支付请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/payment/create [post]
func (h *PaymentGatewayHandler) CreatePayment(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req CreatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取网关
	gateway, err := h.gatewayService.GetByID(req.GatewayID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway"})
		return
	}

	if !gateway.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway is disabled"})
		return
	}

	// 验证金额
	if err := h.gatewayService.ValidateAmount(gateway, req.Amount); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 计算手续费
	feeAmount := h.gatewayService.CalculateFee(gateway, req.Amount)
	actualAmount := req.Amount + feeAmount

	// 创建支付记录
	record := &model.PaymentRecord{
		TradeNo:      h.gatewayService.GenerateTradeNo(),
		GatewayID:    gateway.ID,
		GatewayType:  gateway.Type,
		UserID:       userID,
		Amount:       req.Amount,
		FeeAmount:    feeAmount,
		ActualAmount: actualAmount,
		Currency:     "CNY",
		Status:       model.PaymentStatusPending,
		ClientIP:     c.ClientIP(),
	}

	if req.OrderID != nil {
		record.OrderID = req.OrderID
	}

	if err := h.gatewayService.CreateRecord(record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"trade_no":      record.TradeNo,
			"amount":        record.Amount,
			"fee_amount":    record.FeeAmount,
			"actual_amount": record.ActualAmount,
			"pay_url":       "",
			"qrcode":        "",
		},
	})
}

// GetPaymentStatus godoc
// @Summary 查询支付状态
// @Description 用户查询支付订单状态
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param trade_no path string true "订单号"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /user/payment/status/{trade_no} [get]
func (h *PaymentGatewayHandler) GetPaymentStatus(c *gin.Context) {
	tradeNo := c.Param("trade_no")

	record, err := h.gatewayService.GetRecordByTradeNo(tradeNo)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "record not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"trade_no":      record.TradeNo,
			"amount":        record.Amount,
			"actual_amount": record.ActualAmount,
			"status":        record.Status,
			"paid_at":       record.PaidAt,
		},
	})
}

// GetUserRecords godoc
// @Summary 获取用户支付记录
// @Description 用户获取自己的支付记录列表
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/payment/records [get]
func (h *PaymentGatewayHandler) GetUserRecords(c *gin.Context) {
	userID := c.GetUint("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	records, total, err := h.gatewayService.GetUserRecords(userID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      records,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ========== 支付回调 ==========

// PaymentCallback godoc
// @Summary 支付回调
// @Description 接收第三方支付平台的回调通知
// @Tags 支付回调
// @Accept json
// @Produce plain
// @Param type path string true "支付类型 (alipay/wechat/epay等)"
// @Success 200 {string} string "success"
// @Failure 400 {string} string "fail"
// @Router /payment/callback/{type} [post]
func (h *PaymentGatewayHandler) PaymentCallback(c *gin.Context) {
	gatewayType := c.Param("type")

	body, err := c.GetRawData()
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}

	// EPay回调处理
	if gatewayType == model.PaymentGatewayEPay {
		tradeNo := c.Query("out_trade_no")
		gatewayTradeNo := c.Query("trade_no")

		if err := h.gatewayService.MarkAsPaid(tradeNo, gatewayTradeNo, string(body)); err != nil {
			c.String(http.StatusBadRequest, "fail")
			return
		}

		c.String(http.StatusOK, "success")
		return
	}

	c.String(http.StatusOK, "success")
}

// ========== 请求结构体 ==========

// CreateGatewayRequest 创建网关请求
type CreateGatewayRequest struct {
	Name        string  `json:"name" binding:"required"`
	Type        string  `json:"type" binding:"required,oneof=alipay wechat stripe usdt epay"`
	Icon        string  `json:"icon"`
	Config      string  `json:"config"`
	FeeRate     float64 `json:"fee_rate"`
	FeeFixed    float64 `json:"fee_fixed"`
	MinAmount   float64 `json:"min_amount"`
	MaxAmount   float64 `json:"max_amount"`
	Sort        int     `json:"sort"`
	Description string  `json:"description"`
}

// UpdateGatewayRequest 更新网关请求
type UpdateGatewayRequest struct {
	Name        string  `json:"name"`
	Icon        string  `json:"icon"`
	Config      string  `json:"config"`
	FeeRate     float64 `json:"fee_rate"`
	FeeFixed    float64 `json:"fee_fixed"`
	MinAmount   float64 `json:"min_amount"`
	MaxAmount   float64 `json:"max_amount"`
	Sort        int     `json:"sort"`
	Description string  `json:"description"`
}

// CreatePaymentRequest 创建支付请求
type CreatePaymentRequest struct {
	GatewayID uint    `json:"gateway_id" binding:"required"`
	Amount    float64 `json:"amount" binding:"required,min=1"`
	OrderID   *uint   `json:"order_id"`
}

// ToggleRequest 切换状态请求
type ToggleRequest struct {
	Enabled bool `json:"enabled"`
}