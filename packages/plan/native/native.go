// Package native implements the plan package's v2 routes in the package
// itself, on the kernel's v2_plan and v2_event tables adopted in place
// (kernel.storage.adopt). An administrator's plan assignment changes the
// subscriber through the kernel's KernelSubscriber contract
// (kernel.subscriber.entitlements.v1): the package never writes v2_user.
// Legacy handlers and native routes share the tables and the subscriber
// request ledger, so a route can switch between them at any time. Responses
// are byte-compatible with the legacy handlers (internal/tests/plancompat).
//
// The five /api/v2/speed-limit routes have no native handler and stay
// bridged: they are Flux forward limits, and v2_speed_limit, which the
// forward runtime reads, is not adopted. Their rows name forward tunnels,
// create, update and delete read the forward package's v2_forward_tunnel and
// v2_forward_user_tunnel, which no kernel view exposes, an update re-pushes
// the assigned forwards to their nodes, and the tunnel list is the forward
// package's own answer.
package native

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// Plan is a v2_plan row. Its fields, tags and type name match the kernel
// model, so answers and JSON binding errors are the same.
type Plan struct {
	ID                 uint    `gorm:"primaryKey" json:"id"`
	GroupID            uint    `gorm:"index" json:"group_id"`
	TransferEnable     int64   `json:"transfer_enable"` // GB
	SpeedLimit         *int64  `json:"speed_limit"`
	DeviceLimit        *int    `json:"device_limit"`
	Name               string  `gorm:"size:255" json:"name"`
	Content            *string `gorm:"type:text" json:"content"`
	Show               int     `gorm:"default:0" json:"show"`
	Sort               *int    `json:"sort"`
	Renew              int     `gorm:"default:1" json:"renew"`
	ResetPrice         *int64  `json:"reset_price"`
	ResetTrafficMethod *int    `json:"reset_traffic_method"`
	CapacityLimit      *int    `json:"capacity_limit"`

	// Prices
	MonthPrice     *int64 `json:"month_price"`
	QuarterPrice   *int64 `json:"quarter_price"`
	HalfYearPrice  *int64 `json:"half_year_price"`
	YearPrice      *int64 `json:"year_price"`
	TwoYearPrice   *int64 `json:"two_year_price"`
	ThreeYearPrice *int64 `json:"three_year_price"`
	OnetimePrice   *int64 `json:"onetime_price"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (Plan) TableName() string { return "v2_plan" }

// Event is a v2_event row: the plan domain's event log (plan.created,
// plan.updated, plan.deleted, plan.assigned). Nothing else writes it.
type Event struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Type        string     `gorm:"size:100;index" json:"type"`
	Payload     *string    `gorm:"type:text" json:"payload"`
	Status      string     `gorm:"size:20;default:'pending'" json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ProcessedAt *time.Time `json:"processed_at"`
}

// TableName is the adopted kernel table.
func (Event) TableName() string { return "v2_event" }

// Subscriber is the part of KernelSubscriber the native routes call.
type Subscriber interface {
	ApplyEntitlement(ctx context.Context, in *kernelsubscriberv1.ApplyEntitlementRequest, opts ...grpc.CallOption) (*kernelsubscriberv1.ApplyEntitlementResponse, error)
}

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// tables are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Subscriber is the kernel's KernelSubscriber; without it the
	// assignment route has no native handler and stays legacy.
	Subscriber Subscriber
	// Now defaults to time.Now.
	Now func() time.Time
	// NewToken names a request that carries neither an Idempotency-Key nor
	// an X-Request-ID; it defaults to a random UUID.
	NewToken func() string
}

// AssignRouteID is the administrator's plan assignment.
const AssignRouteID = "plan.admin.plans.id.assign.post"

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{
		"plan.admin.plans.get":       s.AdminList,
		"plan.admin.plans.post":      s.AdminCreate,
		"plan.admin.plans.id.get":    s.AdminGet,
		"plan.admin.plans.id.put":    s.AdminUpdate,
		"plan.admin.plans.id.delete": s.AdminDelete,
		"plan.user.plan.get":         s.UserPlans,
	}
	if s.Subscriber != nil {
		handlers[AssignRouteID] = s.AdminAssign
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
func pathID(request pluginhostsdk.NativeRequest) (uint64, bool) {
	id, err := strconv.ParseUint(request.Metadata.PathParams["id"], 10, 32)
	return id, err == nil
}

// emit writes an event as the kernel's PlanService does: the payload is the
// JSON of obj, and a failed write is ignored.
func (s *Service) emit(db *gorm.DB, eventType string, obj any) {
	if err := db.Create(s.event(eventType, obj)).Error; err != nil {
		log.Printf("plan event %s failed: %v", eventType, err)
	}
}

func (s *Service) event(eventType string, obj any) *Event {
	encoded, _ := json.Marshal(obj)
	payload := string(encoded)
	return &Event{Type: eventType, Payload: &payload, Status: "pending", CreatedAt: s.now()}
}
