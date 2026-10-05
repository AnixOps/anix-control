package kernelforward

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/planner"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// Route writes (ForwardControl.CreateRoute, UpdateRoute, DeleteRoute). Each
// runs in one transaction under the plan lock: the request id's replay
// check, the write, and the plan of every route with it. A refused plan
// rolls the write back; an accepted one commits the route, its
// allocations, the generations that moved and the request id, and then
// notifies the state listeners.

const (
	methodCreate = "create_route"
	methodUpdate = "update_route"
	methodDelete = "delete_route"

	defaultPageSize = 100
	maxPageSize     = 1000
)

func checkRequestID(requestID string) (string, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || len(requestID) > MaxRequestID {
		return "", fmt.Errorf("%w: request_id is required and at most %d bytes", ErrInvalidRequest, MaxRequestID)
	}
	return requestID, nil
}

// requestHash is the SHA-256 of a request's deterministic encoding, the
// request id cleared, with its method.
func requestHash(method string, request proto.Message) string {
	encoded, _ := proto.MarshalOptions{Deterministic: true}.Marshal(request)
	sum := sha256.Sum256(append([]byte(method+"\x00"), encoded...))
	return hex.EncodeToString(sum[:])
}

// replay answers the recorded response of requestID into response; found
// is false for a new request id, ErrRequestConflict for one recorded for
// another request.
func replay(tx *gorm.DB, requestID, method, hash string, response proto.Message) (bool, error) {
	var rows []model.KernelForwardRequest
	if err := tx.Where("request_id = ?", requestID).Limit(1).Find(&rows).Error; err != nil {
		return false, fmt.Errorf("kernel forward: request ledger: %w", err)
	}
	if len(rows) == 0 {
		return false, nil
	}
	if rows[0].Method != method || rows[0].RequestHash != hash {
		return false, ErrRequestConflict
	}
	if err := jsonRead.Unmarshal([]byte(rows[0].ResponseJSON), response); err != nil {
		return false, fmt.Errorf("kernel forward: recorded response of %s: %w", requestID, err)
	}
	return true, nil
}

func record(tx *gorm.DB, requestID, method, routeID, hash string, response proto.Message, now time.Time) error {
	encoded, err := jsonWrite.Marshal(response)
	if err != nil {
		return err
	}
	return tx.Create(&model.KernelForwardRequest{
		RequestID: requestID, Method: method, RouteID: routeID, RequestHash: hash, ResponseJSON: string(encoded), CreatedAt: now,
	}).Error
}

func encodeRoute(route *forwardv1.Route) (string, error) {
	encoded, err := jsonWrite.Marshal(route)
	if err != nil {
		return "", fmt.Errorf("kernel forward: encode route: %w", err)
	}
	return string(encoded), nil
}

// write runs one route write: body changes the routes in tx and answers
// the route id the plan's violations are judged against.
func (s *Service) write(ctx context.Context, reason string, body func(tx *gorm.DB, now time.Time) (routeID string, done bool, err error)) error {
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	var outcome PlanOutcome
	err = db.Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if _, err := lockPlan(tx, now); err != nil {
			return err
		}
		routeID, done, err := body(tx, now)
		if err != nil || done {
			return err
		}
		routes, err := loadRoutes(tx)
		if err != nil {
			return err
		}
		outcome, err = s.replanTx(tx, routes, now)
		if err != nil {
			return err
		}
		if outcome.Refused {
			return refused(outcome.Violations, routeID)
		}
		return recordPlan(tx, outcome, reason, now)
	})
	if err != nil {
		return err
	}
	notify(outcome.Changed)
	return nil
}

// CreateRoute validates, plans and stores route under a new id, as
// ForwardControl.CreateRoute. A *RefusedError refuses it.
func (s *Service) CreateRoute(ctx context.Context, requestID string, route *forwardv1.Route) (*forwardv1.Route, error) {
	requestID, err := checkRequestID(requestID)
	if err != nil {
		return nil, err
	}
	if route == nil {
		return nil, fmt.Errorf("%w: route is required", ErrInvalidRequest)
	}
	if route.GetId() != "" {
		return nil, fmt.Errorf("%w: route id is assigned by Control", ErrInvalidRequest)
	}
	hash := requestHash(methodCreate, &forwardv1.CreateRouteRequest{Route: route})
	response := &forwardv1.CreateRouteResponse{}
	var candidateID string
	err = s.write(ctx, methodCreate, func(tx *gorm.DB, now time.Time) (string, bool, error) {
		if found, err := replay(tx, requestID, methodCreate, hash, response); err != nil || found {
			return "", found, err
		}
		candidate := proto.Clone(route).(*forwardv1.Route)
		candidateID = newULID(now)
		candidate.Id, candidate.Revision = candidateID, 1
		candidate.CreatedAtUnixMs, candidate.UpdatedAtUnixMs = now.UnixMilli(), now.UnixMilli()
		if violations := validate.Proto(candidate, validate.Options{OnCreate: true, Now: now, EnableAnixOps: s.anixOpsEnabled()}); len(violations) > 0 {
			return "", false, refused(routeViolations(candidate.GetId(), violations), candidate.GetId())
		}
		encoded, err := encodeRoute(candidate)
		if err != nil {
			return "", false, err
		}
		if err := tx.Create(&model.KernelForwardRoute{
			ID: candidate.GetId(), Owner: candidate.GetOwner(), Revision: 1, RouteJSON: encoded, CreatedAt: now, UpdatedAt: now,
		}).Error; err != nil {
			return "", false, fmt.Errorf("kernel forward: store route: %w", err)
		}
		response.Route = candidate
		return candidate.GetId(), false, record(tx, requestID, methodCreate, candidate.GetId(), hash, response, now)
	})
	var refusal *RefusedError
	if errors.As(err, &refusal) {
		// The new route has no id yet: its violations name none.
		for i := range refusal.Violations {
			if refusal.Violations[i].RouteID == candidateID {
				refusal.Violations[i].RouteID = ""
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return response.GetRoute(), nil
}

// UpdateRoute replaces a stored route at expectedRevision, as
// ForwardControl.UpdateRoute.
func (s *Service) UpdateRoute(ctx context.Context, requestID string, route *forwardv1.Route, expectedRevision uint64) (*forwardv1.Route, error) {
	requestID, err := checkRequestID(requestID)
	if err != nil {
		return nil, err
	}
	if route.GetId() == "" {
		return nil, fmt.Errorf("%w: route id is required", ErrInvalidRequest)
	}
	if expectedRevision == 0 {
		return nil, fmt.Errorf("%w: expected_revision is required", ErrInvalidRequest)
	}
	hash := requestHash(methodUpdate, &forwardv1.UpdateRouteRequest{Route: route, ExpectedRevision: expectedRevision})
	response := &forwardv1.UpdateRouteResponse{}
	err = s.write(ctx, methodUpdate, func(tx *gorm.DB, now time.Time) (string, bool, error) {
		if found, err := replay(tx, requestID, methodUpdate, hash, response); err != nil || found {
			return "", found, err
		}
		var rows []model.KernelForwardRoute
		if err := tx.Where("id = ?", route.GetId()).Limit(1).Find(&rows).Error; err != nil {
			return "", false, err
		}
		if len(rows) == 0 {
			return "", false, ErrNotFound
		}
		stored, err := decodeRoute(rows[0])
		if err != nil {
			return "", false, err
		}
		if rows[0].Revision != expectedRevision {
			return "", false, ErrRevisionMismatch
		}
		candidate := proto.Clone(route).(*forwardv1.Route)
		candidate.Revision = rows[0].Revision + 1
		candidate.CreatedAtUnixMs, candidate.UpdatedAtUnixMs = stored.GetCreatedAtUnixMs(), now.UnixMilli()
		encoded, err := encodeRoute(candidate)
		if err != nil {
			return "", false, err
		}
		if err := tx.Model(&model.KernelForwardRoute{}).Where("id = ?", candidate.GetId()).Updates(map[string]any{
			"owner": candidate.GetOwner(), "revision": candidate.GetRevision(), "route_json": encoded, "updated_at": now,
		}).Error; err != nil {
			return "", false, fmt.Errorf("kernel forward: store route: %w", err)
		}
		response.Route = candidate
		return candidate.GetId(), false, record(tx, requestID, methodUpdate, candidate.GetId(), hash, response, now)
	})
	if err != nil {
		return nil, err
	}
	return response.GetRoute(), nil
}

// DeleteRoute removes a route, as ForwardControl.DeleteRoute: its hops
// leave every node's state in the next generation, and its ports and marks
// stay held for ReleaseGrace.
func (s *Service) DeleteRoute(ctx context.Context, requestID, routeID string) error {
	requestID, err := checkRequestID(requestID)
	if err != nil {
		return err
	}
	if routeID == "" {
		return fmt.Errorf("%w: route_id is required", ErrInvalidRequest)
	}
	hash := requestHash(methodDelete, &forwardv1.DeleteRouteRequest{RouteId: routeID})
	return s.write(ctx, methodDelete, func(tx *gorm.DB, now time.Time) (string, bool, error) {
		if found, err := replay(tx, requestID, methodDelete, hash, &forwardv1.DeleteRouteResponse{}); err != nil || found {
			return "", found, err
		}
		result := tx.Where("id = ?", routeID).Delete(&model.KernelForwardRoute{})
		if result.Error != nil {
			return "", false, result.Error
		}
		if result.RowsAffected == 0 {
			return "", false, ErrNotFound
		}
		if err := tx.Model(&model.KernelForwardAllocation{}).Where("route_id = ? AND released_at IS NULL", routeID).
			Updates(map[string]any{"released_at": now, "updated_at": now}).Error; err != nil {
			return "", false, fmt.Errorf("kernel forward: release allocations: %w", err)
		}
		return routeID, false, record(tx, requestID, methodDelete, routeID, hash, &forwardv1.DeleteRouteResponse{}, now)
	})
}

func routeViolations(routeID string, violations validate.Violations) []planner.RouteViolation {
	out := make([]planner.RouteViolation, 0, len(violations))
	for _, v := range violations {
		out = append(out, planner.RouteViolation{RouteID: routeID, Violation: v})
	}
	return out
}

// GetRoute answers a stored route; ErrNotFound for an unknown id.
func (s *Service) GetRoute(ctx context.Context, routeID string) (*forwardv1.Route, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	var rows []model.KernelForwardRoute
	if err := db.Where("id = ?", routeID).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return decodeRoute(rows[0])
}

// ListOptions filter and page ListRoutes.
type ListOptions struct {
	// Owner keeps the routes of one owner; empty for every route.
	Owner string
	// NodeRef keeps the routes with a hop on the node.
	NodeRef string
	// PageSize is 100 when 0, at most 1000.
	PageSize uint32
	// PageToken is the next_page_token of the previous page.
	PageToken string
}

// ListRoutes answers routes by id, a page at a time, and the token of the
// next page (empty after the last).
func (s *Service) ListRoutes(ctx context.Context, opts ListOptions) ([]*forwardv1.Route, string, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, "", err
	}
	size := int(opts.PageSize)
	if size == 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	query := db.Order("id").Limit(size + 1)
	if opts.Owner != "" {
		query = query.Where("owner = ?", opts.Owner)
	}
	if opts.PageToken != "" {
		query = query.Where("id > ?", opts.PageToken)
	}
	if opts.NodeRef != "" {
		query = query.Where("id IN (?)", db.Model(&model.KernelForwardAllocation{}).Select("route_id").
			Where("node_ref = ? AND released_at IS NULL", opts.NodeRef))
	}
	var rows []model.KernelForwardRoute
	if err := query.Find(&rows).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > size {
		rows = rows[:size]
		next = rows[size-1].ID
	}
	routes := make([]*forwardv1.Route, 0, len(rows))
	for _, row := range rows {
		route, err := decodeRoute(row)
		if err != nil {
			return nil, "", err
		}
		routes = append(routes, route)
	}
	return routes, next, nil
}

// PlanRoute runs the planner for one route without storing anything, as
// ForwardControl.PlanRoute: request nodes replace the inventory's of the
// same reference, the route keeps its own allocations when it has an id,
// and every other allocation (deleted routes in their grace period too) is
// taken.
func (s *Service) PlanRoute(ctx context.Context, request *forwardv1.PlanRouteRequest) (*forwardv1.PlanRouteResponse, error) {
	if request.GetRoute() == nil {
		return nil, fmt.Errorf("%w: route is required", ErrInvalidRequest)
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	inv, err := loadInventory(db)
	if err != nil {
		return nil, err
	}
	var rows []model.KernelForwardAllocation
	if err := db.Where("released_at IS NULL OR released_at >= ?", now.Add(-ReleaseGrace)).Find(&rows).Error; err != nil {
		return nil, err
	}
	own, taken := planner.Allocations{}, planner.Allocations{}
	routeID := request.GetRoute().GetId()
	for _, row := range rows {
		key := planner.Key{RouteID: row.RouteID, HopIndex: row.HopIndex, NodeRef: row.NodeRef}
		slot := planner.Slot{Port: row.Port, Mark: row.Mark}
		if routeID != "" && row.RouteID == routeID && row.ReleasedAt == nil {
			own[key] = slot
		} else {
			taken[key] = slot
		}
	}
	response, err := planner.PlanRoute(request, inv.nodes, own, planner.Options{
		Cluster: s.cluster(), ReservedPorts: inv.reserved, Taken: taken, Now: now, OnCreate: routeID == "",
		EnableAnixOps: s.anixOpsEnabled(),
	})
	if err != nil {
		if errors.Is(err, planner.ErrNoCluster) {
			return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		}
		return nil, err
	}
	return response, nil
}
