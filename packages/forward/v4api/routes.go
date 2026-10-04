package v4api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"google.golang.org/protobuf/proto"
)

// routeView is a route with Control's enforcement reason ("quota" or
// "expired" when Control itself pauses it).
type routeView struct {
	Route    json.RawMessage `json:"route"`
	Enforced string          `json:"enforced,omitempty"`
}

func (s *Service) listRoutes(ctx context.Context, request Request, _ map[string]string) Response {
	listRequest := &forwardv1.ListRoutesRequest{
		Owner: query(request, "owner"), NodeRef: query(request, "node_ref"), PageToken: query(request, "page_token"),
	}
	if raw := query(request, "page_size"); raw != "" {
		size, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return failure(http.StatusBadRequest, "invalid_request", "page_size must be a number", nil)
		}
		listRequest.PageSize = uint32(size)
	}
	answer, err := s.Forward.ListRoutes(ctx, listRequest)
	if err != nil {
		return fromStatus(err)
	}
	routes := make([]routeView, 0, len(answer.GetRoutes()))
	for _, route := range answer.GetRoutes() {
		routes = append(routes, routeView{Route: pj(route), Enforced: answer.GetEnforced()[route.GetId()]})
	}
	return data(http.StatusOK, map[string]any{"routes": routes, "next_page_token": answer.GetNextPageToken()})
}

// createRoute: the body is a Route (protojson). Control assigns its id,
// revision and times; owner defaults to "admin".
func (s *Service) createRoute(ctx context.Context, request Request, _ map[string]string) Response {
	route := &forwardv1.Route{}
	if answer := decode(request.Body, route, false); answer != nil {
		return *answer
	}
	if route.GetOwner() == "" {
		route.Owner = "admin"
	}
	if route.GetOwner() != "admin" {
		// User routes are the commercial self-service's (v4.3).
		return failure(http.StatusBadRequest, "invalid_request", `owner must be "admin"`, nil)
	}
	answer, err := s.Forward.CreateRoute(ctx, &forwardv1.CreateRouteRequest{RequestId: s.requestID(request, ""), Route: route})
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusCreated, routeView{Route: pj(answer.GetRoute())})
}

// previewRoute: the body is a PlanRouteRequest ({"route": ..., "nodes":
// [...]}); nothing is stored. Violations are part of the answer.
func (s *Service) previewRoute(ctx context.Context, request Request, _ map[string]string) Response {
	plan := &forwardv1.PlanRouteRequest{}
	if answer := decode(request.Body, plan, false); answer != nil {
		return *answer
	}
	if plan.GetRoute() != nil && plan.GetRoute().GetOwner() == "" {
		plan.Route.Owner = "admin"
	}
	answer, err := s.Forward.PlanRoute(ctx, plan)
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, map[string]any{
		"states": pjList(answer.GetStates()), "allocations": pjList(answer.GetAllocations()),
		"violations": violationsOf(answer.GetViolations()), "warnings": nonNil(answer.GetWarnings()),
	})
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (s *Service) getRoute(ctx context.Context, _ Request, params map[string]string) Response {
	answer, err := s.Forward.GetRoute(ctx, &forwardv1.GetRouteRequest{RouteId: params["id"]})
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, routeView{Route: pj(answer.GetRoute()), Enforced: answer.GetEnforced()})
}

// updateRoute: the body is the whole Route; its revision is the revision
// it replaces (409 revision_conflict when stale), and the path names it.
func (s *Service) updateRoute(ctx context.Context, request Request, params map[string]string) Response {
	route := &forwardv1.Route{}
	if answer := decode(request.Body, route, false); answer != nil {
		return *answer
	}
	if route.GetId() != "" && route.GetId() != params["id"] {
		return failure(http.StatusBadRequest, "invalid_request", "the body's id is not the path's", nil)
	}
	if route.GetRevision() == 0 {
		return failure(http.StatusBadRequest, "invalid_request", "revision (the revision being replaced) is required", nil)
	}
	if route.GetOwner() == "" {
		route.Owner = "admin"
	}
	route.Id = params["id"]
	expected := route.GetRevision()
	answer, err := s.Forward.UpdateRoute(ctx, &forwardv1.UpdateRouteRequest{RequestId: s.requestID(request, ""), Route: route, ExpectedRevision: expected})
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, routeView{Route: pj(answer.GetRoute())})
}

func (s *Service) deleteRoute(ctx context.Context, request Request, params map[string]string) Response {
	if _, err := s.Forward.DeleteRoute(ctx, &forwardv1.DeleteRouteRequest{RequestId: s.requestID(request, ""), RouteId: params["id"]}); err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"deleted": params["id"]})
}

func (s *Service) pauseRoute(ctx context.Context, request Request, params map[string]string) Response {
	return s.setPaused(ctx, request, params["id"], true)
}

func (s *Service) resumeRoute(ctx context.Context, request Request, params map[string]string) Response {
	return s.setPaused(ctx, request, params["id"], false)
}

// setPaused reads the route and writes it back with paused set at the
// revision it read; a route already so answers unchanged. A concurrent
// change answers 409 revision_conflict.
func (s *Service) setPaused(ctx context.Context, request Request, routeID string, paused bool) Response {
	current, err := s.Forward.GetRoute(ctx, &forwardv1.GetRouteRequest{RouteId: routeID})
	if err != nil {
		return fromStatus(err)
	}
	route := current.GetRoute()
	if route.GetPaused() == paused {
		return data(http.StatusOK, routeView{Route: pj(route), Enforced: current.GetEnforced()})
	}
	candidate := proto.Clone(route).(*forwardv1.Route)
	candidate.Paused = paused
	suffix := "resume"
	if paused {
		suffix = "pause"
	}
	answer, err := s.Forward.UpdateRoute(ctx, &forwardv1.UpdateRouteRequest{
		RequestId: s.requestID(request, suffix), Route: candidate, ExpectedRevision: route.GetRevision(),
	})
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, routeView{Route: pj(answer.GetRoute()), Enforced: current.GetEnforced()})
}

// window reads since and until (Unix milliseconds) from the query.
func window(request Request) (int64, int64, *Response) {
	var bounds [2]int64
	for i, key := range []string{"since", "until"} {
		raw := query(request, key)
		if raw == "" {
			continue
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 0 {
			answer := failure(http.StatusBadRequest, "invalid_request", key+" must be Unix milliseconds", nil)
			return 0, 0, &answer
		}
		bounds[i] = value
	}
	return bounds[0], bounds[1], nil
}

// routeStats answers a route's ledger totals per hop and node
// (GetRouteStats) and its hourly buckets over the window.
func (s *Service) routeStats(ctx context.Context, request Request, params map[string]string) Response {
	since, until, bad := window(request)
	if bad != nil {
		return *bad
	}
	totals, err := s.Forward.GetRouteStats(ctx, &forwardv1.GetRouteStatsRequest{RouteId: params["id"]})
	if err != nil {
		return fromStatus(err)
	}
	traffic, err := s.Forward.GetTraffic(ctx, &forwardv1.GetTrafficRequest{RouteId: params["id"], SinceUnixMs: since, UntilUnixMs: until})
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, map[string]any{
		"counters": pjList(totals.GetCounters()), "series": pjList(traffic.GetBuckets()), "truncated": traffic.GetTruncated(),
	})
}

func (s *Service) routeHealth(ctx context.Context, _ Request, params map[string]string) Response {
	answer, err := s.Forward.GetRouteHealth(ctx, &forwardv1.GetRouteHealthRequest{RouteId: params["id"]})
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"health": pjList(answer.GetHealth())})
}

