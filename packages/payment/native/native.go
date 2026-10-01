// Package native implements the payment package's v2 routes in the package
// itself, on the kernel's v2_payment_gateway, v2_payment_record and
// v2_payment tables adopted in place (kernel.storage.adopt). Legacy handlers
// and native routes share the tables, so a route can switch between them at
// any time. Responses are byte-compatible with the legacy handlers
// (internal/tests/paymentcompat).
//
// The package owns the payment gateways and so holds their secrets
// (merchant keys, webhook secrets) in v2_payment_gateway; its administrator
// answers show them as a placeholder, as the kernel's do. An order is read
// through the kernel view kapi_order_billing_v1 (its buyer, total and
// status) only: a payment is created for the caller's own pending order and
// its exact total.
//
// The four callback routes (the gateway callback, the x402 callback, the
// Stripe and PayPal webhooks) verify the provider's signature with the
// gateway's secrets, as the kernel's do (signatures.go). A paid callback
// then takes two steps (callbacks.go, docs/architecture/order-service.md):
//  1. the package marks its payment record paid and adds it to its
//     gateway's statistics, in one transaction on its own tables;
//  2. the kernel applies the paid record to its order
//     (KernelOrder.CompleteOrderPayment, kernel.order.complete.v1): it
//     re-checks that the record pays the order, marks the order paid and
//     completes it, in one transaction, once per trade number.
//
// Every repeat of a callback whose record is already paid takes step 2
// again, so a failure between the steps converges on the provider's next
// delivery; an order is never paid without a paid record. Without a bridge
// connection the callbacks have no native handler and stay legacy.
package native

import (
	"context"
	"net/http"
	"time"

	kernelorderv1 "github.com/AnixOps/anix-control/sdk/api/kernelorder/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// Payment statuses, as the kernel's model.PaymentStatus* constants.
const (
	PaymentStatusPending   = 0
	PaymentStatusPaid      = 1
	PaymentStatusCancelled = 2
	PaymentStatusRefunded  = 3
	PaymentStatusExpired   = 4
)

// PaymentGateway is a v2_payment_gateway row. Its fields, tags and type name
// match the kernel model, so answers are the same.
type PaymentGateway struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	Name    string `gorm:"size:100;not null" json:"name"`
	Type    string `gorm:"size:20;not null" json:"type"` // alipay/wechat/stripe/usdt/epay
	Enabled bool   `gorm:"default:false" json:"enabled"`
	Icon    string `gorm:"size:255" json:"icon"`

	// Config is the gateway's JSON configuration, secrets included.
	Config string `gorm:"type:text" json:"config"`

	FeeRate  float64 `gorm:"default:0" json:"fee_rate"`
	FeeFixed float64 `gorm:"default:0" json:"fee_fixed"`

	MinAmount float64 `gorm:"default:1" json:"min_amount"`
	MaxAmount float64 `gorm:"default:10000" json:"max_amount"`

	Sort        int    `gorm:"default:0" json:"sort"`
	Description string `gorm:"size:500" json:"description"`

	TotalOrders int64   `json:"total_orders"`
	TotalAmount float64 `json:"total_amount"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (PaymentGateway) TableName() string { return "v2_payment_gateway" }

// PaymentRecord is a v2_payment_record row, with the kernel model's fields
// and tags.
type PaymentRecord struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	TradeNo        string `gorm:"size:64;uniqueIndex" json:"trade_no"`
	GatewayID      uint   `json:"gateway_id"`
	GatewayType    string `json:"gateway_type"`
	GatewayTradeNo string `gorm:"size:100" json:"gateway_trade_no"`
	Provider       string `gorm:"size:50" json:"provider"`
	UserID         uint   `json:"user_id"`

	Amount       float64 `json:"amount"`
	FeeAmount    float64 `json:"fee_amount"`
	ActualAmount float64 `json:"actual_amount"`
	Currency     string  `gorm:"size:10;default:'CNY'" json:"currency"`

	Status      int        `json:"status"`
	PaidAt      *time.Time `json:"paid_at"`
	CancelledAt *time.Time `json:"cancelled_at"`
	RefundedAt  *time.Time `json:"refunded_at"`

	NotifyData string `gorm:"type:text" json:"notify_data"`
	ClientIP   string `gorm:"size:50" json:"client_ip"`

	TxHash        *string `gorm:"size:255" json:"tx_hash"`
	WalletAddress *string `gorm:"size:255" json:"wallet_address"`
	Network       *string `gorm:"size:50" json:"network"`

	OrderID *uint `json:"order_id"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (PaymentRecord) TableName() string { return "v2_payment_record" }

// Payment is a v2_payment row: the older payment method configuration the
// method list reads.
type Payment struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:100" json:"name"`
	Icon      *string   `gorm:"size:255" json:"icon"`
	Method    string    `gorm:"size:20" json:"method"`
	Provider  string    `gorm:"size:50" json:"provider"`
	Config    string    `gorm:"type:text" json:"config"`
	Notify    *string   `gorm:"size:255" json:"notify"`
	Sort      int       `gorm:"default:0" json:"sort"`
	Enable    int       `gorm:"default:1" json:"enable"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (Payment) TableName() string { return "v2_payment" }

// PaymentChannel is a gateway as users see it.
type PaymentChannel struct {
	ID          uint    `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Icon        string  `json:"icon"`
	MinAmount   float64 `json:"min_amount"`
	MaxAmount   float64 `json:"max_amount"`
	FeeRate     float64 `json:"fee_rate"`
	FeeFixed    float64 `json:"fee_fixed"`
	Description string  `json:"description"`
}

// BillingOrder is a row of kapi_order_billing_v1: what a payment needs of
// an order.
type BillingOrder struct {
	ID          uint `gorm:"primaryKey"`
	UserID      uint
	TotalAmount int64 // cents
	Status      int
}

// TableName is the kernel view.
func (BillingOrder) TableName() string { return "kapi_order_billing_v1" }

// OrderCompleter is the part of KernelOrder the callbacks call.
type OrderCompleter interface {
	CompleteOrderPayment(ctx context.Context, in *kernelorderv1.CompleteOrderPaymentRequest, opts ...grpc.CallOption) (*kernelorderv1.CompleteOrderPaymentResponse, error)
}

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// tables and the granted view are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Orders is the kernel's KernelOrder; without it the callbacks have no
	// native handler and stay legacy.
	Orders OrderCompleter
	// HTTPClient calls PayPal's API to verify a webhook delivery; nil uses
	// a client with the kernel's 10-second timeout.
	HTTPClient *http.Client
	// Now defaults to time.Now.
	Now func() time.Time
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{
		"payment.admin.payment.gateways.get":            s.AdminGateways,
		"payment.admin.payment.gateways.post":           s.AdminCreateGateway,
		"payment.admin.payment.gateways.id.put":         s.AdminUpdateGateway,
		"payment.admin.payment.gateways.id.delete":      s.AdminDeleteGateway,
		"payment.admin.payment.gateways.id.toggle.post": s.AdminToggleGateway,
		"payment.admin.payment.records.get":             s.AdminRecords,
		"payment.admin.payment.stats.get":               s.AdminStats,
		"payment.user.payment.channels.get":             s.UserChannels,
		"payment.user.payment.create.post":              s.UserCreatePayment,
		"payment.user.payment.status.trade_no.get":      s.UserPaymentStatus,
		"payment.user.payment.records.get":              s.UserRecords,
		"payment.payment.methods.get":                   s.PaymentMethods,
		"payment.payment.status.trade_no.get":           s.PublicPaymentStatus,
		"payment.payment.x402.create.post":              s.X402CreatePayment,
		"payment.payment.x402.check.id.get":             s.X402CheckPayment,
		"payment.payment.fiat.create.post":              s.FiatCreatePayment,
	}
	if s.Orders != nil {
		handlers[CallbackRouteID] = s.PaymentCallback
		handlers[X402CallbackRouteID] = s.X402Callback
		handlers[StripeWebhookRouteID] = s.StripeWebhook
		handlers[PayPalWebhookRouteID] = s.PayPalWebhook
	}
	return handlers
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) panel(data any) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(data, s.now()))
}

func (s *Service) panelError(message string) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelError(message, s.now()))
}

// defaultQuery is gin's Context.DefaultQuery: the first value of a query
// parameter that is present, even empty, else fallback.
func defaultQuery(request pluginhostsdk.NativeRequest, key, fallback string) string {
	if values, ok := request.Metadata.Query[key]; ok && len(values) > 0 {
		return values[0]
	}
	return fallback
}

// queryValue is gin's Context.Query: the first value of a query parameter,
// else empty.
func queryValue(request pluginhostsdk.NativeRequest, key string) string {
	return defaultQuery(request, key, "")
}
