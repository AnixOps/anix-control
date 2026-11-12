package kernelforward

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/planner"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"github.com/AnixOps/anix-control/v5/internal/model"
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
		// A port another listener holds on the same node is refused when
		// the route is saved, not left waiting in the plan.
		if routeID != "" && tx.Migrator().HasTable(&model.NetworkModel{}) {
			conflicts, err := s.network(tx).ListenConflicts(tx, routeID)
			if err != nil {
				return err
			}
			if len(conflicts) > 0 {
				violations := make([]planner.RouteViolation, len(conflicts))
				for i, message := range conflicts {
					violations[i] = planner.RouteViolation{RouteID: routeID, Violation: validate.Violation{Field: "listen.port", Code: validate.CodePortInUse, Message: message}}
				}
				return refused(violations, routeID)
			}
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
	outcome.notify()
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
		violations := validate.Proto(candidate, validate.Options{OnCreate: true, Now: now})
		if len(violations) > 0 {
			return "", false, refused(routeViolations(candidate.GetId(), violations), candidate.GetId())
		}
		encoded, err := encodeRoute(candidate)
		if err != nil {
			return "", false, err
		}
		if err := tx.Create(&model.Line{
			ID: candidate.GetId(), Owner: candidate.GetOwner(), Revision: 1, Route: encoded, CreatedAt: now, UpdatedAt: now,
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
		var rows []model.Line
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
		if violations := validate.Proto(route, validate.Options{Now: now}); len(violations) > 0 {
			return "", false, refused(routeViolations(route.GetId(), violations), route.GetId())
		}
		candidate := proto.Clone(route).(*forwardv1.Route)
		candidate.Revision = rows[0].Revision + 1
		candidate.CreatedAtUnixMs, candidate.UpdatedAtUnixMs = stored.GetCreatedAtUnixMs(), now.UnixMilli()
		encoded, err := encodeRoute(candidate)
		if err != nil {
			return "", false, err
		}
		if err := tx.Model(&model.Line{}).Where("id = ?", candidate.GetId()).Updates(map[string]any{
			"owner": candidate.GetOwner(), "revision": candidate.GetRevision(), "route": encoded, "updated_at": now,
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

// DeleteRoute removes a route, as ForwardControl.DeleteRoute; the next plan
// drops it from the network model.
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
		result := tx.Where("id = ?", routeID).Delete(&model.Line{})
		if result.Error != nil {
			return "", false, result.Error
		}
		if result.RowsAffected == 0 {
			return "", false, ErrNotFound
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
	var rows []model.Line
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
	query := db.Order("id")
	if opts.NodeRef == "" {
		query = query.Limit(size + 1)
	}
	if opts.Owner != "" {
		query = query.Where("owner = ?", opts.Owner)
	}
	if opts.PageToken != "" {
		query = query.Where("id > ?", opts.PageToken)
	}
	var rows []model.Line
	if err := query.Find(&rows).Error; err != nil {
		return nil, "", err
	}
	routes := make([]*forwardv1.Route, 0, min(len(rows), size+1))
	for _, row := range rows {
		route, err := decodeRoute(row)
		if err != nil {
			return nil, "", err
		}
		if opts.NodeRef != "" && !usesNode(route, opts.NodeRef) {
			continue
		}
		routes = append(routes, route)
		if len(routes) > size {
			break
		}
	}
	next := ""
	if len(routes) > size {
		routes = routes[:size]
		next = routes[size-1].GetId()
	}
	return routes, next, nil
}

// PlanRoute checks one route against the inventory without storing
// anything, as ForwardControl.PlanRoute: request nodes replace the
// inventory's of the same reference. Routes run on the network model, so
// the answer holds only violations.
func (s *Service) PlanRoute(ctx context.Context, request *forwardv1.PlanRouteRequest) (*forwardv1.PlanRouteResponse, error) {
	if request.GetRoute() == nil {
		return nil, fmt.Errorf("%w: route is required", ErrInvalidRequest)
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	inv, err := loadInventory(db)
	if err != nil {
		return nil, err
	}
	return planner.PlanRoute(request, inv.nodes, planner.Options{
		ReservedPorts: inv.reserved, Now: s.now(), OnCreate: request.GetRoute().GetId() == "",
	}), nil
}

// usesNode reports whether a hop of route runs on ref.
func usesNode(route *forwardv1.Route, ref string) bool {
	for _, hop := range route.GetHops() {
		if slices.Contains(hop.GetNodeRefs(), ref) {
			return true
		}
	}
	return false
}
