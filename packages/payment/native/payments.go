package native

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

// Payment methods and providers, as the kernel's model constants.
const (
	methodCrypto   = "crypto"
	providerX402   = "x402"
	providerStripe = "stripe"
	providerPayPal = "paypal"
)

// CreatePaymentRequest is the kernel's request type of the same name; the
// name appears in binding errors.
type CreatePaymentRequest struct {
	GatewayID uint    `json:"gateway_id" binding:"required"`
	Amount    float64 `json:"amount" binding:"required,min=1"`
	OrderID   *uint   `json:"order_id"`
}

// Messages of the kernel's ErrPaymentOrderNotFound, ErrPaymentOrderNotPending
// and ErrPaymentAmountMismatch.
var (
	errOrderNotFound   = errors.New("订单不存在")
	errOrderNotPending = errors.New("订单已支付或已取消")
	errAmountMismatch  = errors.New("支付金额与订单金额不符")
)

// amountPaysOrder reports whether amount (yuan) is an order's total (cents)
// to the cent.
func amountPaysOrder(amount float64, totalCents int64) bool {
	return math.Round(amount*100) == float64(totalCents)
}

// checkOrderPayable is the kernel's CheckOrderPayable on the order view: the
// order is the caller's, still pending, and amount is its total.
func checkOrderPayable(db *gorm.DB, orderID, userID uint, amount float64) error {
	var order BillingOrder
	if err := db.Select("id", "user_id", "status", "total_amount").Where("id = ?", orderID).Take(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errOrderNotFound
		}
		return err
	}
	if order.UserID != userID {
		return errOrderNotFound
	}
	if order.Status != 0 {
		return errOrderNotPending
	}
	if !amountPaysOrder(amount, order.TotalAmount) {
		return errAmountMismatch
	}
	return nil
}

// randomHex is n random bytes in hex.
func randomHex(n int) (string, error) {
	random := make([]byte, n)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
}

// UserCreatePayment is POST /api/v2/user/payment/create: a payment through
// an enabled gateway, within its limits, with its fee. A payment for an
// order pays the caller's own pending order in full.
func (s *Service) UserCreatePayment(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req CreatePaymentRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("invalid gateway")
	}
	var gateway PaymentGateway
	if err := db.First(&gateway, req.GatewayID).Error; err != nil {
		return s.panelError("invalid gateway")
	}
	if !gateway.Enabled {
		return s.panelError("gateway is disabled")
	}
	if err := validateGatewayCanBeEnabled(&gateway); err != nil {
		return s.panelError(err.Error())
	}
	if req.Amount < gateway.MinAmount {
		return s.panelError(fmt.Sprintf("金额不能小于 %.2f", gateway.MinAmount))
	}
	if req.Amount > gateway.MaxAmount {
		return s.panelError(fmt.Sprintf("金额不能大于 %.2f", gateway.MaxAmount))
	}
	userID := request.Principal.ActorID
	if req.OrderID != nil {
		if err := checkOrderPayable(db, *req.OrderID, userID, req.Amount); err != nil {
			if errors.Is(err, errOrderNotFound) || errors.Is(err, errOrderNotPending) || errors.Is(err, errAmountMismatch) {
				return s.panelError(err.Error())
			}
			log.Printf("payment order lookup failed: %v", err)
			return s.panelError("数据库错误")
		}
	}
	feeAmount := req.Amount*gateway.FeeRate + gateway.FeeFixed
	random, err := randomHex(4)
	if err != nil {
		return s.panelError(err.Error())
	}
	record := &PaymentRecord{
		TradeNo: "PAY" + s.now().Format("20060102150405") + random, GatewayID: gateway.ID, GatewayType: gateway.Type,
		UserID: userID, Amount: req.Amount, FeeAmount: feeAmount, ActualAmount: req.Amount + feeAmount, Currency: "CNY",
		Status: PaymentStatusPending, ClientIP: request.Metadata.ClientIP, OrderID: req.OrderID,
	}
	if err := db.Create(record).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{
		"trade_no":      record.TradeNo,
		"amount":        record.Amount,
		"fee_amount":    record.FeeAmount,
		"actual_amount": record.ActualAmount,
		"pay_url":       "",
		"qrcode":        "",
	})
}

// recordByTradeNo is a payment record by trade number.
func recordByTradeNo(db *gorm.DB, tradeNo string) (*PaymentRecord, error) {
	var record PaymentRecord
	if err := db.Where("trade_no = ?", tradeNo).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// UserPaymentStatus is GET /api/v2/user/payment/status/:trade_no for the
// caller's own payment; another user's is answered as missing.
func (s *Service) UserPaymentStatus(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		log.Printf("payment gateway status lookup failed: %v", err)
		return s.panelError("数据库错误")
	}
	record, err := recordByTradeNo(db, request.Metadata.PathParams["trade_no"])
	if err == nil && record.UserID != request.Principal.ActorID {
		err = gorm.ErrRecordNotFound
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError("支付记录不存在")
		}
		log.Printf("payment gateway status lookup failed: %v", err)
		return s.panelError("数据库错误")
	}
	return s.panel(map[string]any{
		"trade_no":      record.TradeNo,
		"amount":        record.Amount,
		"actual_amount": record.ActualAmount,
		"status":        record.Status,
		"paid_at":       record.PaidAt,
	})
}

// PublicPaymentStatus is GET /api/v2/payment/status/:trade_no, which needs
// no login: the trade number is the payment's reference.
func (s *Service) PublicPaymentStatus(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		log.Printf("payment status lookup failed: %v", err)
		return s.panelError("数据库错误")
	}
	payment, err := recordByTradeNo(db, request.Metadata.PathParams["trade_no"])
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError("支付记录不存在")
		}
		log.Printf("payment status lookup failed: %v", err)
		return s.panelError("数据库错误")
	}
	statusText := "pending"
	switch payment.Status {
	case PaymentStatusPaid:
		statusText = "paid"
	case PaymentStatusCancelled:
		statusText = "cancelled"
	case PaymentStatusRefunded:
		statusText = "refunded"
	case PaymentStatusExpired:
		statusText = "expired"
	}
	data := map[string]any{
		"trade_no":      payment.TradeNo,
		"status":        payment.Status,
		"status_text":   statusText,
		"method":        payment.GatewayType,
		"provider":      payment.Provider,
		"amount":        payment.Amount,
		"actual_amount": payment.ActualAmount,
		"currency":      payment.Currency,
		"created_at":    payment.CreatedAt,
		"paid_at":       payment.PaidAt,
	}
	if payment.TxHash != nil && *payment.TxHash != "" {
		data["tx_hash"] = payment.TxHash
	}
	if payment.WalletAddress != nil && *payment.WalletAddress != "" {
		data["wallet_address"] = payment.WalletAddress
	}
	if payment.Network != nil && *payment.Network != "" {
		data["network"] = payment.Network
	}
	return s.panel(data)
}

// PaymentMethods is GET /api/v2/payment/methods: the fixed method list,
// each enabled when an enabled v2_payment row names its provider.
func (s *Service) PaymentMethods(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var configs []Payment
	if db, err := s.Open(ctx); err != nil {
		log.Printf("failed to load payment configs: %v", err)
	} else if err := db.Where("enable = ?", 1).Order("sort ASC, id ASC").Find(&configs).Error; err != nil {
		log.Printf("failed to load payment configs: %v", err)
	}
	enabled := make(map[string]bool)
	for _, config := range configs {
		enabled[config.Provider] = true
	}
	return s.panel([]map[string]any{
		{
			"id": "x402", "name": "虚拟货币支付", "method": "crypto", "provider": "x402", "icon": "cryptocurrency",
			"tokens":   []string{"ETH", "USDT", "USDC"},
			"networks": []string{"sepolia", "base-sepolia", "ethereum", "polygon", "arbitrum", "base"},
			"enabled":  enabled["x402"],
		},
		{"id": "wechat", "name": "微信支付", "method": "fiat", "provider": "wechat_pay", "icon": "wechat", "enabled": enabled["wechat_pay"]},
		{"id": "alipay", "name": "支付宝", "method": "fiat", "provider": "alipay", "icon": "alipay", "enabled": enabled["alipay"]},
		{"id": "stripe", "name": "信用卡支付", "method": "fiat", "provider": "stripe", "icon": "credit-card", "enabled": enabled["stripe"]},
		{"id": "paypal", "name": "PayPal", "method": "fiat", "provider": "paypal", "icon": "paypal", "enabled": enabled["paypal"]},
		{
			"id": "usdt", "name": "USDT 加密货币", "method": "crypto", "provider": "usdt", "icon": "usdt",
			"tokens": []string{"USDT"}, "networks": []string{"TRC20", "ERC20", "BEP20"}, "enabled": enabled["usdt"],
		},
	})
}

// payableOrder is the caller's order a crypto or fiat payment is created
// for; another user's order is answered as missing.
func (s *Service) payableOrder(db *gorm.DB, orderID, userID uint, logPrefix string) (*BillingOrder, string) {
	var order BillingOrder
	if err := db.Where("id = ?", orderID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "订单不存在"
		}
		log.Printf("%s payment order lookup failed: %v", logPrefix, err)
		return nil, "数据库错误"
	}
	if order.UserID != userID {
		return nil, "订单不存在"
	}
	if order.Status != 0 {
		return nil, "订单已支付或已取消"
	}
	return &order, ""
}

// X402CreatePayment is POST /api/v2/payment/x402/create: a test-chain
// crypto payment of the caller's pending order, to a generated address.
func (s *Service) X402CreatePayment(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		OrderID uint   `json:"order_id" binding:"required"`
		Token   string `json:"token"`
		Network string `json:"network"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		log.Printf("x402 payment order lookup failed: %v", err)
		return s.panelError("数据库错误")
	}
	// x402 is an internal play coin: it is usable only when an enabled
	// v2_payment row names it, and refused like a disabled gateway otherwise.
	var x402Enabled int64
	if err := db.Model(&Payment{}).Where("provider = ? AND enable = ?", "x402", 1).Count(&x402Enabled).Error; err != nil || x402Enabled == 0 {
		return s.panelError("gateway is disabled")
	}
	order, message := s.payableOrder(db, req.OrderID, request.Principal.ActorID, "x402")
	if message != "" {
		return s.panelError(message)
	}
	address, err := randomHex(20)
	if err != nil {
		log.Printf("x402 payment address generation failed: %v", err)
		return s.panelError("生成支付地址失败")
	}
	walletAddr := "0x" + address
	amountETH := float64(order.TotalAmount) / 100_000_000
	nonce, err := randomHex(4)
	if err != nil {
		log.Printf("x402 trade number generation failed: %v", err)
		return s.panelError("生成支付单号失败")
	}
	now := s.now()
	tradeNo := "X402" + now.Format("20060102150405") + nonce
	record := &PaymentRecord{
		TradeNo: tradeNo, GatewayType: methodCrypto, Provider: providerX402, UserID: order.UserID,
		Amount: float64(order.TotalAmount) / 100, ActualAmount: amountETH, Currency: req.Token, Status: PaymentStatusPending,
		OrderID: &req.OrderID, WalletAddress: &walletAddr, Network: &req.Network,
	}
	if err := db.Create(record).Error; err != nil {
		log.Printf("x402 payment record creation failed: %v", err)
		return s.panelError("创建支付记录失败")
	}
	amount := fmt.Sprintf("%.8f", amountETH)
	return s.panel(map[string]any{
		"payment_id":     record.ID,
		"trade_no":       tradeNo,
		"wallet_address": walletAddr,
		"amount":         amount,
		"token":          req.Token,
		"network":        req.Network,
		"expires_at":     now.Add(30 * time.Minute).Unix(),
		"qr_code":        fmt.Sprintf("x402:%s?value=%s&token=%s", walletAddr, amount, req.Token),
		"message":        "X402 支付订单已创建",
	})
}

// X402CheckPayment is GET /api/v2/payment/x402/check/:id: the caller's
// payment by trade number, else by numeric id; another user's is answered
// as missing.
func (s *Service) X402CheckPayment(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	paymentID := request.Metadata.PathParams["id"]
	db, err := s.Open(ctx)
	if err != nil {
		log.Printf("x402 payment lookup failed: %v", err)
		return s.panelError("数据库错误")
	}
	payment, err := recordByTradeNo(db, paymentID)
	if err != nil {
		if id, parseErr := parseUint(paymentID); parseErr == nil {
			var record PaymentRecord
			if err = db.First(&record, id).Error; err == nil {
				payment = &record
			} else {
				payment = nil
			}
		}
	}
	if err == nil && payment.UserID != request.Principal.ActorID {
		err = gorm.ErrRecordNotFound
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || payment == nil {
			return s.panelError("支付记录不存在")
		}
		log.Printf("x402 payment lookup failed: %v", err)
		return s.panelError("数据库错误")
	}
	statusText := "pending"
	confirms := 0
	switch payment.Status {
	case PaymentStatusPaid:
		statusText = "confirmed"
		confirms = 6
	case PaymentStatusCancelled:
		statusText = "failed"
	case PaymentStatusRefunded:
		statusText = "refunded"
	case PaymentStatusExpired:
		statusText = "expired"
	}
	return s.panel(map[string]any{
		"payment_id":     payment.ID,
		"trade_no":       payment.TradeNo,
		"status":         statusText,
		"status_code":    payment.Status,
		"confirms":       confirms,
		"tx_hash":        payment.TxHash,
		"wallet_address": payment.WalletAddress,
		"network":        payment.Network,
		"created_at":     payment.CreatedAt,
	})
}

// parseUint is the kernel's digits-only parser of a numeric payment id; a
// number beyond 64 bits wraps around, as there.
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

// Messages of a refused Stripe or PayPal payment. Both providers have a
// verified webhook but no checkout creation: no Stripe Checkout Session or
// PayPal Order is ever requested, so there is no link a buyer could pay at.
// They are the kernel handler's messages (internal/tests/paymentcompat).
const (
	stripeNotImplementedMessage = "Stripe 支付尚未实现，未创建支付订单 (Stripe checkout is not implemented; no payment was created)"
	paypalNotImplementedMessage = "PayPal 支付尚未实现，未创建支付订单 (PayPal checkout is not implemented; no payment was created)"
)

// FiatCreatePayment is POST /api/v2/payment/fiat/create. The caller's order
// is validated as in the kernel, then a Stripe or PayPal payment is refused:
// neither provider's checkout is implemented, and an earlier version answered
// a simulated checkout link and stored a pending payment record that nothing
// could ever pay. No payment record is created.
func (s *Service) FiatCreatePayment(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		OrderID  uint   `json:"order_id" binding:"required"`
		Provider string `json:"provider" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		log.Printf("fiat payment order lookup failed: %v", err)
		return s.panelError("数据库错误")
	}
	if _, message := s.payableOrder(db, req.OrderID, request.Principal.ActorID, "fiat"); message != "" {
		return s.panelError(message)
	}
	switch req.Provider {
	case providerStripe:
		log.Printf("fiat payment refused: Stripe checkout is not implemented (order_id=%d)", req.OrderID)
		return s.panelError(stripeNotImplementedMessage)
	case providerPayPal:
		log.Printf("fiat payment refused: PayPal checkout is not implemented (order_id=%d)", req.OrderID)
		return s.panelError(paypalNotImplementedMessage)
	default:
		return s.panelError("不支持的支付方式")
	}
}
