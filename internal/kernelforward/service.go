// Package kernelforward is the kernel's forwarding state
// (docs/architecture/forward-sdk.md section 8, F3a): routes and their port
// and mark allocations, the node inventory with each Agent's reported
// capabilities, the planner run on every change (sdk/forward/planner), each
// node's desired state and generation, the nodes' latest reports and the
// traffic ledger. It serves the ForwardControl contract to official packages
// (kernel.forward.v1) and gives the Agent Control stream what it carries:
// the node's state for its anixops.nodeconfig/v2 configuration
// (NodeConfigMember) and the acceptance of its reports (RecordReport).
//
// Every plan runs under one lock row (v4_kernel_forward_plan), so plans run
// one at a time across Control processes, and plans every route together:
// any violation refuses the whole plan and every node keeps its previous
// generation (forward-sdk.md section 5.1). A route write that the plan
// refuses is refused; a replan after an inventory change that is refused is
// recorded (PlanStatus) and logged.
package kernelforward

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/planner"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// ReleaseGrace is how long a deleted route's ports and marks stay held
	// (forward-sdk.md section 5.2), so a late packet never reaches a new
	// route.
	ReleaseGrace = 10 * time.Minute
	// RequestRetention is how long a write's request id is remembered.
	RequestRetention = 7 * 24 * time.Hour
	// MaxRequestID bounds a request id.
	MaxRequestID = 128
	// planRowID names the lock and status row.
	planRowID = "current"
)

// Enforcement reasons (KernelForwardRoute.Enforced).
const (
	EnforcedQuota   = "quota"
	EnforcedExpired = "expired"
)

var (
	// ErrNotFound: no route with that id.
	ErrNotFound = errors.New("forward route not found")
	// ErrRevisionMismatch: UpdateRoute's expected_revision is not the
	// stored revision.
	ErrRevisionMismatch = errors.New("forward route revision does not match")
	// ErrRequestConflict: the request id was recorded for another request.
	ErrRequestConflict = errors.New("request_id was used for another request")
	// ErrInvalidRequest: the request itself is malformed (request_id, an
	// id on create, expected_revision missing).
	ErrInvalidRequest = errors.New("invalid forward request")
)

// RefusedError is a route write refused by validation or planning.
// Violations carry the route each belongs to; Precondition is true when the
// nodes cannot host the routes (ports, marks, nodes, capabilities, another
// route) rather than the route being malformed.
type RefusedError struct {
	Violations   []planner.RouteViolation
	Precondition bool
}

func (e *RefusedError) Error() string {
	if len(e.Violations) == 0 {
		return "forward route refused"
	}
	first := e.Violations[0]
	return fmt.Sprintf("forward route refused: %s %s (and %d more)", first.RouteID, first.Error(), len(e.Violations)-1)
}

// ProtoViolations answers the violations in the contract, each with its
// route.
func (e *RefusedError) ProtoViolations() []*forwardv1.Violation {
	out := make([]*forwardv1.Violation, 0, len(e.Violations))
	for _, v := range e.Violations {
		p := v.ToProto()
		p.RouteId = v.RouteID
		out = append(out, p)
	}
	return out
}

// preconditionCodes are the violations of a route that is well formed but
// that the nodes cannot host.
var preconditionCodes = map[validate.Code]bool{
	validate.CodeUnknownNode: true, validate.CodeEngineNotAdvertised: true, validate.CodeEngineUnavailable: true,
	validate.CodeCapabilityMissing: true, validate.CodePortOutOfRange: true, validate.CodePortReserved: true,
	validate.CodePortInUse: true, validate.CodePortExhausted: true, validate.CodeNoPortRange: true,
	validate.CodeMarkExhausted: true, validate.CodeNoAddress: true,
}

// refused classifies violations against the route being written (routeID,
// empty on a create).
func refused(violations []planner.RouteViolation, routeID string) *RefusedError {
	err := &RefusedError{Violations: violations}
	for _, v := range violations {
		if v.RouteID != routeID || preconditionCodes[v.Code] {
			err.Precondition = true
		}
	}
	// A malformed field of the route itself wins: the caller fixes the
	// request first.
	for _, v := range violations {
		if v.RouteID == routeID && !preconditionCodes[v.Code] {
			err.Precondition = false
		}
	}
	return err
}

// Service is the kernel's forwarding state on one database. Its methods are
// safe for concurrent use; every plan serializes on the database's lock
// row.
type Service struct {
	DB *gorm.DB
	// Cluster names the Agent identities the planner pins on encrypted
	// links (planner.Options.Cluster); the module runtime's cluster
	// (module_runtime.cluster, "default") when nil.
	Cluster func() string
	// Now defaults to time.Now.
	Now func() time.Time
}

// New returns the service on db with the configured cluster.
func New(db *gorm.DB) *Service {
	return &Service{DB: db}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) cluster() string {
	if s.Cluster != nil {
		return s.Cluster()
	}
	if cfg := config.Get(); cfg != nil {
		return cfg.ModuleRuntime.ClusterOrDefault()
	}
	return "default"
}

func (s *Service) db(ctx context.Context) (*gorm.DB, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("kernel forward: database is not initialized")
	}
	return s.DB.WithContext(ctx), nil
}

// lockPlan takes the plan row for update in tx, creating it once.
func lockPlan(tx *gorm.DB, now time.Time) (*model.KernelForwardPlan, error) {
	row := model.KernelForwardPlan{ID: planRowID, UpdatedAt: now}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: create plan row: %w", err)
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", planRowID).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: lock plan row: %w", err)
	}
	return &row, nil
}

var (
	jsonWrite = protojson.MarshalOptions{UseProtoNames: true}
	jsonRead  = protojson.UnmarshalOptions{DiscardUnknown: true}
)

// StateListener receives the nodes whose desired state moved to a new
// generation, after the plan that moved it committed.
type StateListener func(nodes []agentcontrol.AgentNode)

var (
	listenersMu sync.Mutex
	listeners   = map[int]StateListener{}
	listenerID  int
)

// OnStateChange registers listener for every committed generation change,
// of every Service in the process. The Agent Control stream registers one
// that pushes the changed nodes' configurations. cancel removes it.
func OnStateChange(listener StateListener) (cancel func()) {
	listenersMu.Lock()
	defer listenersMu.Unlock()
	listenerID++
	id := listenerID
	listeners[id] = listener
	return func() {
		listenersMu.Lock()
		delete(listeners, id)
		listenersMu.Unlock()
	}
}

// notify hands the changed node references to every listener.
func notify(refs []string) {
	if len(refs) == 0 {
		return
	}
	nodes := make([]agentcontrol.AgentNode, 0, len(refs))
	for _, ref := range refs {
		node, err := agentcontrol.ParseAgentNode(ref)
		if err != nil {
			continue
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].String() < nodes[j].String() })
	listenersMu.Lock()
	current := make([]StateListener, 0, len(listeners))
	ids := make([]int, 0, len(listeners))
	for id := range listeners {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		current = append(current, listeners[id])
	}
	listenersMu.Unlock()
	for _, listener := range current {
		listener(nodes)
	}
}

func logRefusedPlan(reason string, violations []planner.RouteViolation) {
	if len(violations) == 0 {
		return
	}
	first := violations[0]
	slog.Warn("forward plan refused: every node keeps its state", "component", "kernel-forward", "reason", reason,
		"violations", len(violations), "route_id", first.RouteID, "field", first.Field, "code", string(first.Code), "message", first.Message)
}
