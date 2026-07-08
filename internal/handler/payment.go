package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PaymentHandler 支付处理器
type PaymentHandler struct {
	gatewayService *service.PaymentGatewayService
}

// NewPaymentHandler 创建支付处理器
func NewPaymentHandler() *PaymentHandler {
	return &PaymentHandler{
		gatewayService: service.NewPaymentGatewayService(database.Get()),
	}
}

// webhookSecretFor 读取指定网关类型已启用配置中的 Webhook 密钥。
// 未配置网关或密钥为空时返回空串，调用方据此决定 fail-closed。
func (h *PaymentHandler) webhookSecretFor(gatewayType string) string {
	gateway, err := h.gatewayService.GetByType(gatewayType)
	if err != nil || gateway == nil {
		return ""
	}
	cfg, err := h.gatewayService.ParseConfig(gateway)
	if err != nil {
		return ""
	}
	switch c := cfg.(type) {
	case *model.StripeConfig:
		return c.WebhookSecret
	case *model.X402Config:
		return c.WebhookSecret
	}
	return ""
}

// verifyPayPalWebhook 调用 PayPal verify-webhook-signature API 校验回调真实性。
// 未配置 PayPal 网关 (ClientID/Secret/WebhookID 缺失) 时 fail-closed 返回错误。
func (h *PaymentHandler) verifyPayPalWebhook(c *gin.Context, body []byte) error {
	gateway, err := h.gatewayService.GetByType(model.PaymentGatewayPayPal)
	if err != nil || gateway == nil {
		return errWebhookSecret
	}
	cfg, err := h.gatewayService.ParseConfig(gateway)
	if err != nil {
		return err
	}
	ppCfg, ok := cfg.(*model.PayPalConfig)
	if !ok || ppCfg.ClientID == "" || ppCfg.ClientSecret == "" || ppCfg.WebhookID == "" {
		return errWebhookSecret
	}

	headers := paypalSignatureHeaders{
		AuthAlgo:         c.GetHeader("Paypal-Auth-Algo"),
		CertURL:          c.GetHeader("Paypal-Cert-Url"),
		TransmissionID:   c.GetHeader("Paypal-Transmission-Id"),
		TransmissionSig:  c.GetHeader("Paypal-Transmission-Sig"),
		TransmissionTime: c.GetHeader("Paypal-Transmission-Time"),
	}
	if headers.TransmissionID == "" || headers.TransmissionSig == "" {
		return errSignatureMissing
	}

	return verifyPayPalSignatureRemote(c.Request.Context(), ppCfg, headers, body)
}

// ====== X402 虚拟货币支付 (测试链) ======

// X402CreatePayment 创建 X402 支付
func (h *PaymentHandler) X402CreatePayment(c *gin.Context) {
	var req struct {
		OrderID uint   `json:"order_id" binding:"required"`
		Token   string `json:"token"`   // 支付代币 (ETH, USDT, USDC)
		Network string `json:"network"` // 网络 (sepolia, base-sepolia 等测试链)
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误: "+err.Error())
		return
	}

	// 1. 验证订单存在
	db := database.Get()
	var order model.Order
	if err := db.Where("id = ?", req.OrderID).First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			panelError(c, "订单不存在")
			return
		}
		log.Printf("x402 payment order lookup failed: %v", err)
		panelError(c, "数据库错误")
		return
	}

	if order.Status != 0 { // 0: pending
		panelError(c, "订单已支付或已取消")
		return
	}

	// 2. 生成支付地址 (模拟测试链地址)
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		log.Printf("x402 payment address generation failed: %v", err)
		panelError(c, "生成支付地址失败")
		return
	}
	walletAddr := fmt.Sprintf("0x%s", hex.EncodeToString(b))

	// 3. 计算支付金额 (将分转换为 ETH 单位)
	amountETH := float64(order.TotalAmount) / 100_000_000 // 简易转换

	// 4. 创建支付记录
	nonce := make([]byte, 4)
	if _, err := rand.Read(nonce); err != nil {
		log.Printf("x402 trade number generation failed: %v", err)
		panelError(c, "生成支付单号失败")
		return
	}
	tradeNo := fmt.Sprintf("X402%s%s", time.Now().Format("20060102150405"), hex.EncodeToString(nonce))

	paymentRecord := &model.PaymentRecord{
		TradeNo:       tradeNo,
		GatewayType:   model.PaymentMethodCrypto,
		Provider:      model.PaymentProviderX402,
		UserID:        order.UserID,
		Amount:        float64(order.TotalAmount) / 100, // 分转元
		ActualAmount:  amountETH,
		Currency:      req.Token,
		Status:        model.PaymentStatusPending,
		OrderID:       &req.OrderID,
		WalletAddress: &walletAddr,
		Network:       &req.Network,
	}

	if err := h.gatewayService.CreateRecord(paymentRecord); err != nil {
		log.Printf("x402 payment record creation failed: %v", err)
		panelError(c, "创建支付记录失败")
		return
	}

	expiresAt := time.Now().Add(30 * time.Minute).Unix()

	panelSuccess(c, gin.H{
		"payment_id":     paymentRecord.ID,
		"trade_no":       tradeNo,
		"wallet_address": walletAddr,
		"amount":         fmt.Sprintf("%.8f", amountETH),
		"token":          req.Token,
		"network":        req.Network,
		"expires_at":     expiresAt,
		"qr_code":        fmt.Sprintf("x402:%s?value=%s&token=%s", walletAddr, fmt.Sprintf("%.8f", amountETH), req.Token),
		"message":        "X402 支付订单已创建",
	})
}

// X402Callback X402 支付回调
func (h *PaymentHandler) X402Callback(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "读取请求体失败"})
		return
	}
	defer func() {
		if err := c.Request.Body.Close(); err != nil {
			log.Printf("X402 callback request body close failed: %v", err)
		}
	}()

	// 解析回调数据
	var callbackData struct {
		TradeNo       string `json:"trade_no"`
		TxHash        string `json:"tx_hash"`
		BlockNumber   int64  `json:"block_number"`
		Confirmations int    `json:"confirmations"`
		Status        string `json:"status"` // "confirmed", "confirming", "failed"
		Amount        string `json:"amount"`
		Token         string `json:"token"`
		Signature     string `json:"signature"` // 回调签名 (用于验证来源)
	}

	if err := json.Unmarshal(body, &callbackData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的回调数据", "error": err.Error()})
		return
	}

	// 校验回调签名 (HMAC-SHA256)，防止伪造回调白嫖开通。
	secret := h.webhookSecretFor(model.PaymentGatewayX402)
	signFields := map[string]string{
		"trade_no":      callbackData.TradeNo,
		"tx_hash":       callbackData.TxHash,
		"block_number":  strconv.FormatInt(callbackData.BlockNumber, 10),
		"confirmations": strconv.Itoa(callbackData.Confirmations),
		"status":        callbackData.Status,
		"amount":        callbackData.Amount,
		"token":         callbackData.Token,
	}
	if err := verifyX402Signature(signFields, callbackData.Signature, secret); err != nil {
		log.Printf("X402 callback signature verification failed for trade_no=%s: %v", callbackData.TradeNo, err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "签名验证失败"})
		return
	}

	// 查询支付记录
	payment, err := h.gatewayService.GetRecordByTradeNo(callbackData.TradeNo)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"message": "支付记录不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库错误"})
		return
	}

	// 防止重复处理
	if payment.Status != model.PaymentStatusPending {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "already processed"})
		return
	}

	// 验证确认数 (至少需要 1 个确认)
	// Stub: hardcoded to 1. Should read from X402Config.MinConfirmations.
	minConfirms := 1
	if callbackData.Confirmations < minConfirms {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "insufficient confirmations"})
		return
	}

	// 根据状态更新订单
	rawBody := string(body)
	txHash := callbackData.TxHash
	switch callbackData.Status {
	case "confirmed", "success":
		if err := h.gatewayService.MarkOrderPaid(callbackData.TradeNo, txHash, rawBody); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "更新支付状态失败", "error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "payment confirmed"})
	case "failed":
		now := time.Now()
		if err := database.Get().Model(&model.PaymentRecord{}).
			Where("trade_no = ?", callbackData.TradeNo).
			Updates(map[string]any{
				"status":       model.PaymentStatusCancelled,
				"tx_hash":      &txHash,
				"notify_data":  &rawBody,
				"cancelled_at": &now,
			}).Error; err != nil {
			log.Printf("failed to mark payment failed for trade_no=%s: %v", callbackData.TradeNo, err)
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "payment failed"})
	default:
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "pending"})
	}
}

// X402CheckPayment 查询 X402 支付状态
func (h *PaymentHandler) X402CheckPayment(c *gin.Context) {
	paymentID := c.Param("id")

	// 尝试按 trade_no 或 ID 查询
	var payment *model.PaymentRecord
	var err error

	// 先按 trade_no 查询 (可能是字符串 ID)
	payment, err = h.gatewayService.GetRecordByTradeNo(paymentID)
	if err != nil {
		// 再按数字 ID 查询
		if id, parseErr := parseUint(paymentID); parseErr == nil {
			payment, err = h.getPaymentRecordByID(id)
		}
	}

	if err != nil {
		if err == gorm.ErrRecordNotFound || payment == nil {
			panelError(c, "支付记录不存在")
			return
		}
		log.Printf("x402 payment lookup failed: %v", err)
		panelError(c, "数据库错误")
		return
	}

	// 确定状态描述
	statusDesc := "pending"
	confirms := 0
	switch payment.Status {
	case model.PaymentStatusPaid:
		statusDesc = "confirmed"
		confirms = 6 // 已确认
	case model.PaymentStatusCancelled:
		statusDesc = "failed"
	case model.PaymentStatusRefunded:
		statusDesc = "refunded"
	case model.PaymentStatusExpired:
		statusDesc = "expired"
	}

	panelSuccess(c, gin.H{
		"payment_id":     payment.ID,
		"trade_no":       payment.TradeNo,
		"status":         statusDesc,
		"status_code":    payment.Status,
		"confirms":       confirms,
		"tx_hash":        payment.TxHash,
		"wallet_address": payment.WalletAddress,
		"network":        payment.Network,
		"created_at":     payment.CreatedAt,
	})
}

// ====== 法币支付 (Stripe/PayPal 等) ======

// FiatCreatePayment 创建法币支付
func (h *PaymentHandler) FiatCreatePayment(c *gin.Context) {
	var req struct {
		OrderID  uint   `json:"order_id" binding:"required"`
		Provider string `json:"provider" binding:"required"` // stripe, paypal
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误: "+err.Error())
		return
	}

	// 验证订单存在
	db := database.Get()
	var order model.Order
	if err := db.Where("id = ?", req.OrderID).First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			panelError(c, "订单不存在")
			return
		}
		log.Printf("fiat payment order lookup failed: %v", err)
		panelError(c, "数据库错误")
		return
	}

	if order.Status != 0 {
		panelError(c, "订单已支付或已取消")
		return
	}

	// 生成商户订单号
	fiatNonce := make([]byte, 4)
	if _, err := rand.Read(fiatNonce); err != nil {
		log.Printf("fiat trade number generation failed: %v", err)
		panelError(c, "生成支付单号失败")
		return
	}
	tradeNo := fmt.Sprintf("FIAT%s%s", time.Now().Format("20060102150405"), hex.EncodeToString(fiatNonce))

	switch req.Provider {
	case "stripe":
		// Stripe Checkout Session creation stub:
		//   1. Set stripe.Key = config.StripeSecretKey
		//   2. Create a stripe.CheckoutSession with line items, success_url, cancel_url
		//   3. Return session.URL for user redirect
		//   4. Store the session_id in the payment record for later webhook correlation
		// Current implementation uses mock data.
		log.Printf("[STUB] Stripe Checkout Session creation not yet implemented, using mock response")

		paymentRecord := &model.PaymentRecord{
			TradeNo:      tradeNo,
			GatewayType:  model.PaymentMethodFiat,
			Provider:     model.PaymentProviderStripe,
			UserID:       order.UserID,
			Amount:       float64(order.TotalAmount) / 100,
			ActualAmount: float64(order.TotalAmount) / 100,
			Currency:     "USD",
			Status:       model.PaymentStatusPending,
			OrderID:      &req.OrderID,
		}

		if err := h.gatewayService.CreateRecord(paymentRecord); err != nil {
			log.Printf("stripe payment record creation failed: %v", err)
			panelError(c, "创建支付记录失败")
			return
		}

		// 模拟 Stripe Checkout URL
		checkoutURL := fmt.Sprintf("https://checkout.stripe.com/pay/cs_test_%s", tradeNo)
		panelSuccess(c, gin.H{
			"payment_id":   paymentRecord.ID,
			"trade_no":     tradeNo,
			"provider":     "stripe",
			"checkout_url": checkoutURL,
			"session_id":   fmt.Sprintf("cs_test_%s", tradeNo),
			"amount":       paymentRecord.Amount,
			"currency":     "USD",
			"message":      "Stripe 支付订单已创建 (模拟)",
		})

	case "paypal":
		// PayPal Order creation stub:
		//   1. Initialize PayPal Client (ClientID, ClientSecret from config)
		//   2. Create a PayPal Order with the payment amount and currency
		//   3. Return approve_url for user authorization
		//   4. Store the PayPal order_id in the payment record
		// Current implementation uses mock data.
		log.Printf("[STUB] PayPal Order creation not yet implemented, using mock response")

		paymentRecord := &model.PaymentRecord{
			TradeNo:      tradeNo,
			GatewayType:  model.PaymentMethodFiat,
			Provider:     model.PaymentProviderPayPal,
			UserID:       order.UserID,
			Amount:       float64(order.TotalAmount) / 100,
			ActualAmount: float64(order.TotalAmount) / 100,
			Currency:     "USD",
			Status:       model.PaymentStatusPending,
			OrderID:      &req.OrderID,
		}

		if err := h.gatewayService.CreateRecord(paymentRecord); err != nil {
			log.Printf("paypal payment record creation failed: %v", err)
			panelError(c, "创建支付记录失败")
			return
		}

		// 模拟 PayPal Approve URL
		approveURL := fmt.Sprintf("https://www.paypal.com/checkoutnow?token=PAYPAL_%s", tradeNo)
		panelSuccess(c, gin.H{
			"payment_id":  paymentRecord.ID,
			"trade_no":    tradeNo,
			"provider":    "paypal",
			"approve_url": approveURL,
			"order_id":    fmt.Sprintf("PAYPAL_ORDER_%s", tradeNo),
			"amount":      paymentRecord.Amount,
			"currency":    "USD",
			"message":     "PayPal 支付订单已创建 (模拟)",
		})

	default:
		panelError(c, "不支持的支付方式")
	}
}

// StripeWebhook Stripe 回调
func (h *PaymentHandler) StripeWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "读取请求体失败"})
		return
	}
	defer func() {
		if err := c.Request.Body.Close(); err != nil {
			log.Printf("Stripe webhook request body close failed: %v", err)
		}
	}()

	// 获取 Stripe 签名头
	stripeSig := c.GetHeader("Stripe-Signature")

	// 按 Stripe 官方算法校验签名 (HMAC-SHA256)，含时间戳防重放。
	secret := h.webhookSecretFor(model.PaymentGatewayStripe)
	if err := verifyStripeSignature(body, stripeSig, secret, time.Now()); err != nil {
		log.Printf("Stripe webhook signature verification failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "签名验证失败"})
		return
	}

	// 解析事件
	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID                string `json:"id"`
				Status            string `json:"status"`
				AmountTotal       int64  `json:"amount_total"`
				Currency          string `json:"currency"`
				ClientReferenceID string `json:"client_reference_id"`
				Metadata          struct {
					TradeNo string `json:"trade_no"`
					OrderID string `json:"order_id"`
				} `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的事件数据"})
		return
	}

	rawBody := string(body)

	switch event.Type {
	case "checkout.session.completed":
		// 支付成功 - 处理订单
		tradeNo := event.Data.Object.Metadata.TradeNo
		if tradeNo == "" {
			tradeNo = event.Data.Object.ClientReferenceID
		}

		if tradeNo == "" {
			c.JSON(http.StatusOK, gin.H{"received": true, "message": "no trade_no in metadata"})
			return
		}

		if err := h.gatewayService.MarkOrderPaid(tradeNo, event.Data.Object.ID, rawBody); err != nil {
			// 已处理过的订单不报错
			c.JSON(http.StatusOK, gin.H{"received": true, "message": "already processed or error: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"received": true})

	case "checkout.session.expired":
		// 会话过期
		tradeNo := event.Data.Object.Metadata.TradeNo
		if tradeNo == "" {
			tradeNo = event.Data.Object.ClientReferenceID
		}
		if tradeNo != "" {
			now := time.Now()
			if err := database.Get().Model(&model.PaymentRecord{}).
				Where("trade_no = ?", tradeNo).
				Updates(map[string]any{
					"status":       model.PaymentStatusExpired,
					"notify_data":  &rawBody,
					"cancelled_at": &now,
				}).Error; err != nil {
				log.Printf("failed to mark payment expired for trade_no=%s: %v", tradeNo, err)
			}
		}
		c.JSON(http.StatusOK, gin.H{"received": true})

	case "checkout.session.async_payment_failed":
		// 支付失败
		tradeNo := event.Data.Object.Metadata.TradeNo
		if tradeNo == "" {
			tradeNo = event.Data.Object.ClientReferenceID
		}
		if tradeNo != "" {
			now := time.Now()
			if err := database.Get().Model(&model.PaymentRecord{}).
				Where("trade_no = ?", tradeNo).
				Updates(map[string]any{
					"status":       model.PaymentStatusCancelled,
					"notify_data":  &rawBody,
					"cancelled_at": &now,
				}).Error; err != nil {
				log.Printf("failed to mark payment failed for trade_no=%s: %v", tradeNo, err)
			}
		}
		c.JSON(http.StatusOK, gin.H{"received": true})

	default:
		// 其他事件，记录日志
		c.JSON(http.StatusOK, gin.H{"received": true, "message": "event type not handled: " + event.Type})
	}
}

// PayPalWebhook PayPal 回调
func (h *PaymentHandler) PayPalWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "读取请求体失败"})
		return
	}
	defer func() {
		if err := c.Request.Body.Close(); err != nil {
			log.Printf("PayPal webhook request body close failed: %v", err)
		}
	}()

	// 按 PayPal 官方 verify-webhook-signature API 校验回调真实性。
	if err := h.verifyPayPalWebhook(c, body); err != nil {
		log.Printf("PayPal webhook signature verification failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "签名验证失败"})
		return
	}

	// 解析事件
	var event struct {
		ID        string `json:"id"`
		EventType string `json:"event_type"` // CHECKOUT.ORDER.APPROVED, PAYMENT.CAPTURE.COMPLETED 等
		Resource  struct {
			ID            string `json:"id"`
			Status        string `json:"status"` // COMPLETED, APPROVED, etc.
			PurchaseUnits []struct {
				ReferenceID string `json:"reference_id"`
				Payments    struct {
					Captures []struct {
						ID     string `json:"id"`
						Amount struct {
							Value    string `json:"value"`
							Currency string `json:"currency_code"`
						} `json:"amount"`
					} `json:"captures"`
				} `json:"payments"`
			} `json:"purchase_units"`
			CustomID string `json:"custom_id"` // 用于传递 trade_no
		} `json:"resource"`
	}

	if err := json.Unmarshal(body, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的事件数据"})
		return
	}

	rawBody := string(body)
	tradeNo := event.Resource.CustomID

	switch event.EventType {
	case "CHECKOUT.ORDER.APPROVED", "PAYMENT.CAPTURE.COMPLETED":
		// 支付成功
		if tradeNo == "" {
			c.JSON(http.StatusOK, gin.H{"received": true, "message": "no custom_id in resource"})
			return
		}

		if err := h.gatewayService.MarkOrderPaid(tradeNo, event.Resource.ID, rawBody); err != nil {
			c.JSON(http.StatusOK, gin.H{"received": true, "message": "already processed or error: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"received": true})

	case "CHECKOUT.ORDER.COMPLETED":
		// 订单已完成
		if tradeNo != "" {
			if err := h.gatewayService.MarkOrderPaid(tradeNo, event.Resource.ID, rawBody); err != nil {
				c.JSON(http.StatusOK, gin.H{"received": true})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"received": true})

	case "CHECKOUT.ORDER.PROCESSING", "PAYMENT.CAPTURE.DENIED":
		// 处理中或拒绝
		if tradeNo != "" {
			now := time.Now()
			if err := database.Get().Model(&model.PaymentRecord{}).
				Where("trade_no = ?", tradeNo).
				Updates(map[string]any{
					"status":       model.PaymentStatusCancelled,
					"notify_data":  &rawBody,
					"cancelled_at": &now,
				}).Error; err != nil {
				log.Printf("failed to mark payment denied for trade_no=%s: %v", tradeNo, err)
			}
		}
		c.JSON(http.StatusOK, gin.H{"received": true})

	default:
		// 其他事件，记录日志
		c.JSON(http.StatusOK, gin.H{"received": true, "message": "event type not handled: " + event.EventType})
	}
}

// ====== 通用支付接口 ======

// GetPaymentMethods 获取可用支付方式
func (h *PaymentHandler) GetPaymentMethods(c *gin.Context) {
	// 从数据库读取启用的支付方式配置
	var paymentConfigs []model.Payment
	if err := database.Get().Where("enable = ?", 1).Order("sort ASC, id ASC").Find(&paymentConfigs).Error; err != nil {
		log.Printf("failed to load payment configs: %v", err)
	}

	// 构建已启用的支付方式集合
	enabledProviders := make(map[string]bool)
	for _, pc := range paymentConfigs {
		enabledProviders[pc.Provider] = true
	}

	// 预定义支付方式列表 (包含所有支持的支付方式)
	methods := []gin.H{
		{
			"id":       "x402",
			"name":     "虚拟货币支付",
			"method":   "crypto",
			"provider": "x402",
			"icon":     "cryptocurrency",
			"tokens":   []string{"ETH", "USDT", "USDC"},
			"networks": []string{"sepolia", "base-sepolia", "ethereum", "polygon", "arbitrum", "base"},
			"enabled":  enabledProviders["x402"] || len(paymentConfigs) == 0, // 默认启用 (无配置时)
		},
		{
			"id":       "wechat",
			"name":     "微信支付",
			"method":   "fiat",
			"provider": "wechat_pay",
			"icon":     "wechat",
			"enabled":  enabledProviders["wechat_pay"],
		},
		{
			"id":       "alipay",
			"name":     "支付宝",
			"method":   "fiat",
			"provider": "alipay",
			"icon":     "alipay",
			"enabled":  enabledProviders["alipay"],
		},
		{
			"id":       "stripe",
			"name":     "信用卡支付",
			"method":   "fiat",
			"provider": "stripe",
			"icon":     "credit-card",
			"enabled":  enabledProviders["stripe"],
		},
		{
			"id":       "paypal",
			"name":     "PayPal",
			"method":   "fiat",
			"provider": "paypal",
			"icon":     "paypal",
			"enabled":  enabledProviders["paypal"],
		},
		{
			"id":       "usdt",
			"name":     "USDT 加密货币",
			"method":   "crypto",
			"provider": "usdt",
			"icon":     "usdt",
			"tokens":   []string{"USDT"},
			"networks": []string{"TRC20", "ERC20", "BEP20"},
			"enabled":  enabledProviders["usdt"],
		},
	}

	panelSuccess(c, methods)
}

// GetPaymentStatus 查询支付状态
func (h *PaymentHandler) GetPaymentStatus(c *gin.Context) {
	tradeNo := c.Param("trade_no")

	// 从数据库查询支付记录
	payment, err := h.gatewayService.GetRecordByTradeNo(tradeNo)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"message": "支付记录不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库错误"})
		return
	}

	// 构建状态描述
	statusDesc := "pending"
	switch payment.Status {
	case model.PaymentStatusPaid:
		statusDesc = "paid"
	case model.PaymentStatusCancelled:
		statusDesc = "cancelled"
	case model.PaymentStatusRefunded:
		statusDesc = "refunded"
	case model.PaymentStatusExpired:
		statusDesc = "expired"
	}

	data := gin.H{
		"trade_no":      payment.TradeNo,
		"status":        payment.Status,
		"status_text":   statusDesc,
		"method":        payment.GatewayType,
		"provider":      payment.Provider,
		"amount":        payment.Amount,
		"actual_amount": payment.ActualAmount,
		"currency":      payment.Currency,
		"created_at":    payment.CreatedAt,
		"paid_at":       payment.PaidAt,
	}

	// 附加区块链相关信息 (如果存在)
	if payment.TxHash != nil && *payment.TxHash != "" {
		data["tx_hash"] = payment.TxHash
	}
	if payment.WalletAddress != nil && *payment.WalletAddress != "" {
		data["wallet_address"] = payment.WalletAddress
	}
	if payment.Network != nil && *payment.Network != "" {
		data["network"] = payment.Network
	}

	panelSuccess(c, data)
}

// getPaymentRecordByID 根据数字 ID 获取支付记录 (辅助方法)
func (h *PaymentHandler) getPaymentRecordByID(id uint) (*model.PaymentRecord, error) {
	var record model.PaymentRecord
	err := database.Get().First(&record, id).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// parseUint 解析字符串为 uint (辅助方法)
func parseUint(s string) (uint, error) {
	var result uint
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid character: %c", ch)
		}
		result = result*10 + uint(ch-'0')
	}
	return result, nil
}
