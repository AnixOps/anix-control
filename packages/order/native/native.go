// Package native implements the order package's v2 routes in the package
// itself, on the kernel's v2_order and v2_coupon tables adopted in place
// (kernel.storage.adopt). Legacy handlers and native routes share the
// tables, so a route can switch between them at any time; the kernel keeps
// writing v2_order from the payment callbacks (paid, completed) and reads
// it for statistics. Responses are byte-compatible with the legacy handlers
// (internal/tests/ordercompat).
//
// Other domains are read through kernel views only: the plan catalog
// (kapi_plan_catalog_v1) and the subscription groups a plan grants
// (kapi_plan_subscription_group_v1) for pricing and completing an order,
// and the user directory (kapi_user_directory_v1) for the buyer's current
// plan. Completing an order grants its plan through the kernel's
// KernelSubscriber contract (kernel.subscriber.entitlements.v1): the package
// never writes v2_user. Completion pays no commission and sends no
// notification, in the kernel as here.
//
// The four order list and detail routes (administrator and user) have no
// native handler and stay bridged: their answers embed the buyer's whole
// v2_user row, subscription token and proxy UUID included, which no kernel
// view may expose.
package native

import (
	"context"
	"errors"
	"strconv"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// Order is a v2_order row. Its fields and tags match the kernel model
// without its relations, which the native routes never load, so answers are
// the same.
type Order struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	InviteUserID      *uint     `gorm:"index" json:"invite_user_id"`
	UserID            uint      `gorm:"index" json:"user_id"`
	PlanID            uint      `gorm:"index" json:"plan_id"`
	CouponID          *uint     `gorm:"index" json:"coupon_id"`
	PaymentID         *uint     `gorm:"index" json:"payment_id"`
	Type              int       `json:"type"` // 1: new, 2: renew, 3: upgrade, 4: reset
	Period            string    `gorm:"size:20" json:"period"`
	TradeNo           string    `gorm:"size:36;uniqueIndex" json:"trade_no"`
	CallbackNo        *string   `gorm:"size:255" json:"callback_no"`
	TotalAmount       int64     `json:"total_amount"`
	DiscountAmount    *int64    `json:"discount_amount"`
	SurplusAmount     *int64    `json:"surplus_amount"`
	RefundAmount      *int64    `json:"refund_amount"`
	Balance           *int64    `json:"balance"`
	SurplusOrder      *string   `gorm:"type:text" json:"surplus_order"`
	Status            int       `gorm:"default:0" json:"status"` // 0: pending, 1: paid, 2: cancelled, 3: completed, 4: discounted
	CommissionStatus  int       `gorm:"default:0" json:"commission_status"`
	CommissionBalance int64     `gorm:"default:0" json:"commission_balance"`
	PaidAt            *int64    `json:"paid_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (Order) TableName() string { return "v2_order" }

// Coupon is a v2_coupon row, with the kernel model's fields and tags.
type Coupon struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Code         string    `gorm:"size:32;uniqueIndex" json:"code"`
	Name         string    `gorm:"size:255" json:"name"`
	Type         int       `gorm:"default:1" json:"type"`       // 1: percentage, 2: fixed amount
	Value        int       `json:"value"`                       // percentage (0-100) or amount in cents
	LimitUse     *int      `json:"limit_use"`                   // null or -1 = unlimited
	LimitUseWith *uint     `gorm:"index" json:"limit_use_with"` // limit to specific plan ID
	LimitPeriod  *string   `gorm:"size:20" json:"limit_period"` // limit to specific period
	UseCount     int       `gorm:"default:0" json:"use_count"`  // how many times used
	StartedAt    int64     `json:"started_at"`                  // unix timestamp
	EndedAt      int64     `json:"ended_at"`                    // unix timestamp
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (Coupon) TableName() string { return "v2_coupon" }

// CatalogPlan is a row of kapi_plan_catalog_v1: what an order needs of a
// plan.
type CatalogPlan struct {
	ID             uint `gorm:"primaryKey"`
	GroupID        uint
	TransferEnable int64 // GiB
	SpeedLimit     *int64
	DeviceLimit    *int
	MonthPrice     *int64
	QuarterPrice   *int64
	HalfYearPrice  *int64
	YearPrice      *int64
	TwoYearPrice   *int64
	ThreeYearPrice *int64
	OnetimePrice   *int64
}

// TableName is the kernel view.
func (CatalogPlan) TableName() string { return "kapi_plan_catalog_v1" }

// PlanGroup is a row of kapi_plan_subscription_group_v1: a subscription
// group a plan grants.
type PlanGroup struct {
	PlanID  uint
	GroupID uint
}

// TableName is the kernel view.
func (PlanGroup) TableName() string { return "kapi_plan_subscription_group_v1" }

// DirectoryUser is the part of a kapi_user_directory_v1 row an order reads.
type DirectoryUser struct {
	ID     uint `gorm:"primaryKey"`
	PlanID *uint
}

// TableName is the kernel view.
func (DirectoryUser) TableName() string { return "kapi_user_directory_v1" }

// Subscriber is the part of KernelSubscriber the native routes call.
type Subscriber interface {
	ApplyEntitlement(ctx context.Context, in *kernelsubscriberv1.ApplyEntitlementRequest, opts ...grpc.CallOption) (*kernelsubscriberv1.ApplyEntitlementResponse, error)
}

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// tables and the granted views are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Subscriber is the kernel's KernelSubscriber; without it the route
	// that completes an order has no native handler and stays legacy.
	Subscriber Subscriber
	// Now defaults to time.Now.
	Now func() time.Time
}

// MarkPaidRouteID marks an order paid and completes it.
const MarkPaidRouteID = "order.admin.orders.id.paid.post"

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{
		"order.admin.coupon.get":            s.AdminCoupons,
		"order.admin.coupon.post":           s.AdminCreateCoupon,
		"order.admin.coupon.id.delete":      s.AdminDeleteCoupon,
		"order.user.coupon.check.post":      s.CheckCoupon,
		"order.admin.orders.stats.get":      s.AdminOrderStats,
		"order.admin.orders.id.status.put":  s.AdminUpdateOrderStatus,
		"order.admin.orders.id.cancel.post": s.AdminCancelOrder,
		"order.user.order.save.post":        s.SaveOrder,
	}
	if s.Subscriber != nil {
		handlers[MarkPaidRouteID] = s.AdminMarkOrderPaid
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

// pathID parses the :id path parameter as the legacy handlers do.
func pathID(request pluginhostsdk.NativeRequest) (uint64, error) {
	return strconv.ParseUint(request.Metadata.PathParams["id"], 10, 32)
}

// Messages of the kernel's service.ErrOrderNotFound and ErrOrderPlanNotFound.
var (
	errOrderNotFound     = errors.New("订单不存在")
	errOrderPlanNotFound = errors.New("关联套餐不存在")
)

// adminOrderError answers as the kernel's panelAdminOrderError.
func (s *Service) adminOrderError(fallback string, err error) (pluginhostsdk.NativeResponse, error) {
	if errors.Is(err, errOrderNotFound) || errors.Is(err, errOrderPlanNotFound) {
		return s.panelError(err.Error())
	}
	return s.panelError(fallback + ": " + err.Error())
}
