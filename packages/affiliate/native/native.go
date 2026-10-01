// Package native implements the affiliate package's v2 routes in the package
// itself, on the kernel's v2_commission_record, v2_commission_withdraw,
// v2_invite_config and v2_invite_code tables adopted in place
// (kernel.storage.adopt). Legacy
// handlers and native routes share the tables, so a route can switch
// between them at any time. Responses are byte-compatible with the legacy
// handlers (internal/tests/affiliatecompat).
//
// Other domains are read through kernel views only:
//   - kapi_user_referral_v1 (who invited whom) for the invite statistics;
//   - kapi_order_billing_v1 (an order's buyer and status) for the paying
//     users a user invited;
//   - kapi_subscriber_entitlement_v1 for the caller's commission balance;
//   - kapi_affiliate_settings_v1, the one non-secret row of the protected
//     v2_system_config that holds the affiliate's frontend settings (code
//     prefix and length, withdrawal fee and methods), which the package
//     writes through KernelSettings.
//
// The commission balance lives in v2_user, which the kernel owns. A
// withdrawal debits it and a rejected one is refunded through the kernel's
// KernelSubscriber.AdjustBalance (kernel.subscriber.balance.v1, kind
// COMMISSION), with the ledger ids the kernel's legacy handlers use
// (affiliate.withdraw:<id>, affiliate.withdraw.refund:<id>), so a balance
// changes once whichever side serves the request. The package never writes
// v2_user.
//
// The kernel writes a withdrawal and its debit, or a rejection and its
// refund, in one transaction. The module cannot: its table and the balance
// are changed by two parties. A withdrawal is therefore first recorded as a
// reservation (status -1), which no route approves or rejects; it becomes
// pending (0) only once the debit is applied, and is removed when the
// kernel refuses the debit. A reservation whose debit outcome is unknown
// (the call failed in transit) stays, and the request ledger tells whether
// it was debited. A rejection claims the withdrawal first, so concurrent
// decisions cannot both apply, then refunds it; a refund the kernel refuses
// puts the withdrawal back to pending, and one whose outcome is unknown
// leaves it rejected for an administrator to reconcile with the ledger.
// Neither path can pay out or refund an amount that was never debited.
//
// Updating the configuration (PUT /api/v2/admin/invite/config) writes the
// adopted v2_invite_config, then the frontend settings through the kernel's
// KernelSettings contract (namespace invite, kernel.settings.invite.write.v1):
// the settings row of the protected v2_system_config stays the kernel's to
// write, and the write makes the kernel's invite services reload the
// configuration they keep in memory. A host without the contract leaves the
// route legacy.
//
// A user's invite codes are rows of v2_invite_code, which Control's
// registration consumes. A user holds at most code_count unused codes; the
// kernel and the package count and create under the same PostgreSQL
// advisory lock, keyed by the user (LockUserInviteCodes), so concurrent
// generations served by either side never pass the limit.
package native

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// Withdrawal statuses. Reserving is the module's own: a withdrawal whose
// debit is not applied yet (see the package documentation).
const (
	withdrawReserving = -1
	withdrawPending   = 0
	withdrawApproved  = 1
	withdrawRejected  = 2
)

// Withdrawal is a v2_commission_withdraw row. Its fields and tags match the
// kernel model, so answers are the same.
type Withdrawal struct {
	ID      uint    `gorm:"primaryKey" json:"id"`
	UserID  uint    `gorm:"index" json:"user_id"`
	Amount  float64 `json:"amount"`
	Method  string  `gorm:"size:20" json:"method"`
	Account string  `gorm:"size:100" json:"account"`
	Name    string  `gorm:"size:50" json:"name"`
	Status  int     `gorm:"default:0" json:"status"`
	Remark  string  `gorm:"size:255" json:"remark"`

	ProcessedAt *time.Time `json:"processed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (Withdrawal) TableName() string { return "v2_commission_withdraw" }

// Commission is a v2_commission_record row, with the kernel model's fields
// and tags.
type Commission struct {
	ID         uint    `gorm:"primaryKey" json:"id"`
	UserID     uint    `gorm:"index" json:"user_id"`
	OrderID    uint    `gorm:"index" json:"order_id"`
	FromUserID uint    `gorm:"index" json:"from_user_id"`
	Amount     float64 `json:"amount"`
	Type       int     `json:"type"`
	Status     int     `gorm:"default:0" json:"status"`
	Remark     string  `gorm:"size:255" json:"remark"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (Commission) TableName() string { return "v2_commission_record" }

// InviteConfig is a v2_invite_config row, with the kernel model's fields,
// tags and column defaults.
type InviteConfig struct {
	ID      uint `gorm:"primaryKey" json:"id"`
	Enabled bool `gorm:"default:true" json:"enabled"`

	AutoGenerate   bool `gorm:"default:true" json:"auto_generate"`
	CodeCount      int  `gorm:"default:5" json:"code_count"`
	CodeExpireDays int  `gorm:"default:0" json:"code_expire_days"`

	CommissionEnabled   bool    `gorm:"default:true" json:"commission_enabled"`
	CommissionType      int     `gorm:"default:1" json:"commission_type"`
	CommissionRate      float64 `gorm:"default:0.1" json:"commission_rate"`
	CommissionFixed     float64 `gorm:"default:0" json:"commission_fixed"`
	CommissionMinAmount float64 `gorm:"default:10" json:"commission_min"`

	FirstOrderBonus   float64 `gorm:"default:0" json:"first_order_bonus"`
	FirstTrafficBonus int64   `gorm:"default:0" json:"first_traffic_bonus"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (InviteConfig) TableName() string { return "v2_invite_config" }

// ReferralUser is a row of kapi_user_referral_v1: a user and who invited
// them.
type ReferralUser struct {
	ID           uint `gorm:"primaryKey"`
	InviteUserID *uint
}

// TableName is the kernel view.
func (ReferralUser) TableName() string { return "kapi_user_referral_v1" }

// Entitlement is the part of a kapi_subscriber_entitlement_v1 row a
// withdrawal reads.
type Entitlement struct {
	ID                uint `gorm:"primaryKey"`
	CommissionBalance int64
}

// TableName is the kernel view.
func (Entitlement) TableName() string { return "kapi_subscriber_entitlement_v1" }

// Settings is the row of kapi_affiliate_settings_v1: the JSON value of the
// invite.frontend.config system configuration key.
type Settings struct {
	Value string
}

// TableName is the kernel view.
func (Settings) TableName() string { return "kapi_affiliate_settings_v1" }

// Subscriber is the part of KernelSubscriber the native routes call.
type Subscriber interface {
	AdjustBalance(ctx context.Context, in *kernelsubscriberv1.AdjustBalanceRequest, opts ...grpc.CallOption) (*kernelsubscriberv1.AdjustBalanceResponse, error)
}

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// tables and the granted views are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Subscriber is the kernel's KernelSubscriber; without it the routes
	// that change a commission balance have no native handler and stay
	// legacy.
	Subscriber Subscriber
	// KernelSettings is the kernel's KernelSettings; without it updating
	// the configuration has no native handler and stays legacy.
	KernelSettings KernelSettings
	// NewToken names a request that carries neither an Idempotency-Key
	// nor a request id; it defaults to a random UUID.
	NewToken func() string
	// Now defaults to time.Now.
	Now func() time.Time
}

// Route ids of the routes that change a commission balance.
const (
	WithdrawRouteID = "affiliate.user.invite.withdraw.post"
	ProcessRouteID  = "affiliate.admin.invite.withdrawals.id.process.post"
)

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{
		"affiliate.admin.invite.config.get":      s.AdminConfig,
		"affiliate.admin.invite.stats.get":       s.AdminStats,
		"affiliate.admin.invite.withdrawals.get": s.AdminWithdrawals,
		"affiliate.user.invite.commissions.get":  s.UserCommissions,
		"affiliate.user.invite.withdrawals.get":  s.UserWithdrawals,
		InviteInfoRouteID:                        s.InviteInfo,
		InviteGenerateRouteID:                    s.GenerateInviteCode,
	}
	if s.Subscriber != nil {
		handlers[WithdrawRouteID] = s.UserWithdraw
		handlers[ProcessRouteID] = s.AdminProcessWithdrawal
	}
	if s.KernelSettings != nil {
		handlers[ConfigUpdateRouteID] = s.AdminUpdateConfig
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

// jsonAnswer is the legacy handlers' c.JSON(code, body): encoding/json, as
// gin uses, with gin's content type.
func jsonAnswer(code int, body any) (pluginhostsdk.NativeResponse, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, err
	}
	return pluginhostsdk.NativeResponse{
		StatusCode: uint32(code), Body: encoded, // #nosec G115 -- HTTP status codes.
		Headers: []pluginhostsdk.Header{{Name: "Content-Type", Value: "application/json; charset=utf-8"}},
	}, nil
}

// errorAnswer is the legacy handlers' c.JSON(code, gin.H{"error": message}).
func errorAnswer(code int, message string) (pluginhostsdk.NativeResponse, error) {
	return jsonAnswer(code, map[string]any{"error": message})
}

func badRequest(message string) (pluginhostsdk.NativeResponse, error) {
	return errorAnswer(http.StatusBadRequest, message)
}

// defaultQuery is gin's c.DefaultQuery: the first value of a query key
// that is present, else fallback.
func defaultQuery(request pluginhostsdk.NativeRequest, key, fallback string) string {
	if values, ok := request.Metadata.Query[key]; ok && len(values) > 0 {
		return values[0]
	}
	return fallback
}

// query is gin's c.Query.
func query(request pluginhostsdk.NativeRequest, key string) string {
	return defaultQuery(request, key, "")
}
