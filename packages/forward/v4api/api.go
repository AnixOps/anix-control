// Package v4api is the forward package's v4 administrator API,
// /api/v4/forward/* (docs/architecture/forward-sdk.md section 3, F5a). It
// is a thin HTTP layer over the kernel's ForwardControl
// (kernel.forward.v1): routes, the node inventory and the forward node
// registry (the v2 forward node and Ansible machine routes rewritten), the
// traffic ledger and the observability views. It reads and writes no
// table: the kernel owns the forwarding state, and no answer carries a node
// credential.
//
// The kernel serves the API as the package's manifest control route
// /api/v4/plugins/forward/* and keeps two checks of its own: DELETE needs a
// super administrator, and the paths config/editions.json reserves for the
// commercial edition (user self-service, plans, multipliers; v4.3) do not
// exist in the community edition. Every endpoint here is in both editions
// (H23: core forwarding, load balancing, failover and onboarding);
// commercial endpoints join Endpoints with EditionCommercial under one of
// those prefixes.
//
// Conventions: JSON bodies; contract messages are protojson with the
// proto field names and enum names ("ENGINE_NFTABLES"). An answer is
// {"data": ...}; a refusal {"error": {"code", "message", "violations"}},
// each violation {field, message, code, route_id} with the stable codes of
// sdk/forward/validate. Writes take the Idempotency-Key header as their
// request id (one is generated without it), so a retried write applies
// once.
package v4api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Path prefixes the kernel sends: the control route namespace, and the
// public prefix for callers that pass it through unchanged.
const (
	ControlPrefix = "/api/v4/plugins/forward"
	PublicPrefix  = "/api/v4/forward"
)

// Route is the package's manifest control route; the kernel serves it at
// /api/v4/forward/* too, and dispatches both with RouteID.
const Route = ControlPrefix + "/*"

// RouteID is the route id the kernel gives Route: the package id,
// ".control." and the hex SHA-256 of the package id, a NUL byte and the
// route (the kernel's service.PluginControlBridgeRouteID).
var RouteID = func() string {
	digest := sha256.Sum256([]byte("forward\x00" + Route))
	return "forward.control." + hex.EncodeToString(digest[:])
}()

// Editions an endpoint is served in.
const (
	EditionAll        = "all"
	EditionCommercial = "commercial"
)

// maxBody bounds a request body (the kernel caps it at 1 MiB too).
const maxBody = 1 << 20

// Request is one API call as the package host receives it.
type Request struct {
	Method string
	// Path is the request path under ControlPrefix or PublicPrefix.
	Path           string
	Query          map[string][]string
	Body           []byte
	IdempotencyKey string
	// ActorID is the kernel-authenticated caller.
	ActorID uint
	// SuperAdmin is the kernel's word that the caller may DELETE (a super
	// administrator). The kernel sends it on the list routes, whose answers
	// carry it as can_delete (F5b D7); it is false elsewhere.
	SuperAdmin bool
}

// Response is the HTTP answer.
type Response struct {
	StatusCode int
	Body       []byte
}

// Service answers the API on ForwardControl.
type Service struct {
	Forward forwardv1.ForwardControlClient
	// NewRequestID names a write without an Idempotency-Key; a random id by
	// default.
	NewRequestID func() string
}

func (s *Service) requestID(request Request, suffix string) string {
	if key := strings.TrimSpace(request.IdempotencyKey); key != "" {
		if suffix != "" {
			key += ":" + suffix
		}
		if len(key) > 128 {
			key = key[:128]
		}
		return key
	}
	if s.NewRequestID != nil {
		return s.NewRequestID()
	}
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	return "v4api-" + hex.EncodeToString(raw[:])
}

// handler answers one endpoint; params are the path's {name} segments.
type handler func(s *Service, ctx context.Context, request Request, params map[string]string) Response

// Endpoint is one method and path pattern of the API.
type Endpoint struct {
	Method string
	// Pattern is the path under the prefix; {name} matches one segment.
	Pattern string
	// Edition is EditionAll or EditionCommercial.
	Edition string
	// Summary says what it does.
	Summary string
	// Replaces names the v2 route it rewrites, if any.
	Replaces string
	serve    handler
}

// Endpoints lists the API, in match order.
func Endpoints() []Endpoint {
	out := make([]Endpoint, len(endpoints))
	copy(out, endpoints)
	return out
}

var endpoints = []Endpoint{
	{http.MethodGet, "/routes", EditionAll, "List routes", "", (*Service).listRoutes},
	{http.MethodPost, "/routes", EditionAll, "Create a route", "", (*Service).createRoute},
	{http.MethodPost, "/routes/preview", EditionAll, "Plan a route without storing it", "", (*Service).previewRoute},
	{http.MethodGet, "/routes/{id}", EditionAll, "Get a route", "", (*Service).getRoute},
	{http.MethodPut, "/routes/{id}", EditionAll, "Replace a route at its revision", "", (*Service).updateRoute},
	{http.MethodDelete, "/routes/{id}", EditionAll, "Delete a route (super administrator)", "", (*Service).deleteRoute},
	{http.MethodPost, "/routes/{id}/pause", EditionAll, "Pause a route", "", (*Service).pauseRoute},
	{http.MethodPost, "/routes/{id}/resume", EditionAll, "Resume a route", "", (*Service).resumeRoute},
	{http.MethodGet, "/routes/{id}/stats", EditionAll, "A route's counters per hop and node, and its hourly series", "", (*Service).routeStats},
	{http.MethodGet, "/routes/{id}/health", EditionAll, "A route's upstream health", "", (*Service).routeHealth},
	{http.MethodPost, "/routes/{id}/diagnose", EditionAll, "Diagnose a route: config, hop reachability, delivery and port conflicts", "", (*Service).diagnoseRoute},

	{http.MethodGet, "/nodes", EditionAll, "List the node inventory", "GET /api/v2/admin/forward/nodes", (*Service).listNodes},
	{http.MethodPost, "/nodes", EditionAll, "Add a forward node", "POST /api/v2/admin/forward/nodes", (*Service).createNode},
	{http.MethodGet, "/nodes/{ref}", EditionAll, "A node with its desired state and latest report", "GET /api/v2/admin/forward/nodes/:id, POST .../:id/check", (*Service).getNode},
	{http.MethodPut, "/nodes/{ref}", EditionAll, "Replace a forward node's fields", "PUT /api/v2/admin/forward/nodes/:id", (*Service).updateNode},
	{http.MethodDelete, "/nodes/{ref}", EditionAll, "Delete a forward node (super administrator)", "DELETE /api/v2/admin/forward/nodes/:id", (*Service).deleteNode},
	{http.MethodPut, "/nodes/{ref}/settings", EditionAll, "Replace a node's forwarding settings", "", (*Service).setNodeSettings},
	{http.MethodPost, "/nodes/{ref}/toggle", EditionAll, "Enable or disable a forward node", "POST /api/v2/admin/forward/nodes/:id/toggle", (*Service).toggleNode},

	{http.MethodGet, "/ansible-machines", EditionAll, "List the Ansible machines", "GET /api/v2/admin/forward/ansible-machines", (*Service).listAnsible},
	{http.MethodPost, "/ansible-machines", EditionAll, "Add an Ansible machine", "POST /api/v2/admin/forward/ansible-machines", (*Service).createAnsible},
	{http.MethodGet, "/ansible-machines/{id}", EditionAll, "An Ansible machine", "GET /api/v2/admin/forward/ansible-machines/:id, POST .../:id/check", (*Service).getAnsible},
	{http.MethodPut, "/ansible-machines/{id}", EditionAll, "Replace an Ansible machine's fields", "PUT /api/v2/admin/forward/ansible-machines/:id", (*Service).updateAnsible},
	{http.MethodDelete, "/ansible-machines/{id}", EditionAll, "Delete an Ansible machine (super administrator)", "DELETE /api/v2/admin/forward/ansible-machines/:id", (*Service).deleteAnsible},
	{http.MethodPost, "/ansible-machines/{id}/toggle", EditionAll, "Enable or disable an Ansible machine", "POST /api/v2/admin/forward/ansible-machines/:id/toggle", (*Service).toggleAnsible},

	{http.MethodGet, "/stats", EditionAll, "Traffic totals per route, hop and node, and the hourly series", "POST /api/v2/admin/forward/{nodes,ansible-machines}/:id/sync-stats", (*Service).stats},
	{http.MethodGet, "/observability/targets", EditionAll, "Every route target with its health", "GET /api/v2/admin/forward/observability/targets", (*Service).observabilityTargets},
	{http.MethodGet, "/observability/topology", EditionAll, "Nodes, hops and targets as a graph with health", "GET /api/v2/admin/forward/observability/topology", (*Service).observabilityTopology},
	{http.MethodGet, "/observability/trend", EditionAll, "Hourly traffic trend", "GET /api/v2/admin/forward/observability/trend", (*Service).observabilityTrend},
}

// match finds the endpoint of method and the path under the prefix.
func match(method, path string) (Endpoint, map[string]string, bool, bool) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	pathKnown := false
	for _, endpoint := range endpoints {
		pattern := strings.Split(strings.Trim(endpoint.Pattern, "/"), "/")
		if len(pattern) != len(segments) {
			continue
		}
		params := map[string]string{}
		matched := true
		for i, part := range pattern {
			if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
				if segments[i] == "" {
					matched = false
					break
				}
				params[strings.Trim(part, "{}")] = segments[i]
				continue
			}
			if part != segments[i] {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		pathKnown = true
		if endpoint.Method == method {
			return endpoint, params, true, true
		}
	}
	return Endpoint{}, nil, false, pathKnown
}

// Serve answers one request.
func (s *Service) Serve(ctx context.Context, request Request) Response {
	path, ok := strings.CutPrefix(request.Path, ControlPrefix)
	if !ok {
		path, ok = strings.CutPrefix(request.Path, PublicPrefix)
	}
	if !ok || (path != "" && !strings.HasPrefix(path, "/")) {
		return failure(http.StatusNotFound, "not_found", "route not found", nil)
	}
	if s.Forward == nil {
		return failure(http.StatusServiceUnavailable, "forward_unavailable", "the kernel's ForwardControl is not reachable", nil)
	}
	if len(request.Body) > maxBody {
		return failure(http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds 1 MiB", nil)
	}
	endpoint, params, found, pathKnown := match(request.Method, path)
	if !found {
		if pathKnown {
			return failure(http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		}
		return failure(http.StatusNotFound, "not_found", "route not found", nil)
	}
	return endpoint.serve(s, ctx, request, params)
}

// ---------------------------------------------------------------------------
// Answers
// ---------------------------------------------------------------------------

var (
	jsonOut = protojson.MarshalOptions{UseProtoNames: true}
	jsonIn  = protojson.UnmarshalOptions{}
)

// pj encodes a contract message for an answer.
func pj(message proto.Message) json.RawMessage {
	if message == nil {
		return json.RawMessage("null")
	}
	encoded, err := jsonOut.Marshal(message)
	if err != nil {
		return json.RawMessage("null")
	}
	return encoded
}

func pjList[T proto.Message](messages []T) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, pj(message))
	}
	return out
}

func data(status int, value any) Response {
	body, err := json.Marshal(map[string]any{"data": value})
	if err != nil {
		return failure(http.StatusInternalServerError, "encode_failed", "answer could not be encoded", nil)
	}
	return Response{StatusCode: status, Body: body}
}

// Violation is one reason a write was refused.
type Violation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
	RouteID string `json:"route_id,omitempty"`
}

func failure(status int, code, message string, violations []Violation) Response {
	payload := map[string]any{"code": code, "message": message}
	if len(violations) > 0 {
		payload["violations"] = violations
	}
	body, _ := json.Marshal(map[string]any{"error": payload})
	return Response{StatusCode: status, Body: body}
}

func violationsOf(list []*forwardv1.Violation) []Violation {
	out := make([]Violation, 0, len(list))
	for _, v := range list {
		out = append(out, Violation{Field: v.GetField(), Message: v.GetMessage(), Code: v.GetCode(), RouteID: v.GetRouteId()})
	}
	return out
}

// detailViolations extracts the violations a refusal's details carry.
func detailViolations(st *status.Status) []Violation {
	for _, detail := range st.Details() {
		switch answer := detail.(type) {
		case *forwardv1.CreateRouteResponse:
			return violationsOf(answer.GetViolations())
		case *forwardv1.UpdateRouteResponse:
			return violationsOf(answer.GetViolations())
		case *forwardv1.PlanRouteResponse:
			return violationsOf(answer.GetViolations())
		case *forwardv1.UpdateForwardNodeResponse:
			return violationsOf(answer.GetViolations())
		case *forwardv1.DeleteForwardNodeResponse:
			return violationsOf(answer.GetViolations())
		}
	}
	return nil
}

// fromStatus maps a ForwardControl error to an answer:
//
//	INVALID_ARGUMENT     400 invalid_route (with violations) or invalid_request
//	FAILED_PRECONDITION  409 refused (with violations) or idempotency_conflict
//	NOT_FOUND            404 not_found
//	ABORTED              409 revision_conflict
//	UNIMPLEMENTED        501 not_implemented
//	PERMISSION_DENIED,
//	UNAVAILABLE          503 forward_unavailable
//	DEADLINE_EXCEEDED    504 timeout
//	RESOURCE_EXHAUSTED   429 rate_limited
//	anything else        502 forward_failed
func fromStatus(err error) Response {
	st, ok := status.FromError(err)
	if !ok {
		if errors.Is(err, context.DeadlineExceeded) {
			return failure(http.StatusGatewayTimeout, "timeout", "the kernel did not answer in time", nil)
		}
		return failure(http.StatusBadGateway, "forward_failed", "the kernel's ForwardControl failed", nil)
	}
	violations := detailViolations(st)
	switch st.Code() {
	case codes.InvalidArgument:
		if len(violations) > 0 {
			return failure(http.StatusBadRequest, "invalid_route", st.Message(), violations)
		}
		return failure(http.StatusBadRequest, "invalid_request", st.Message(), nil)
	case codes.FailedPrecondition:
		if len(violations) > 0 {
			return failure(http.StatusConflict, "refused", st.Message(), violations)
		}
		return failure(http.StatusConflict, "idempotency_conflict", st.Message(), nil)
	case codes.NotFound:
		return failure(http.StatusNotFound, "not_found", st.Message(), nil)
	case codes.Aborted:
		return failure(http.StatusConflict, "revision_conflict", st.Message(), nil)
	case codes.Unimplemented:
		return failure(http.StatusNotImplemented, "not_implemented", st.Message(), nil)
	case codes.PermissionDenied, codes.Unavailable:
		return failure(http.StatusServiceUnavailable, "forward_unavailable", st.Message(), nil)
	case codes.DeadlineExceeded:
		return failure(http.StatusGatewayTimeout, "timeout", st.Message(), nil)
	case codes.ResourceExhausted:
		return failure(http.StatusTooManyRequests, "rate_limited", st.Message(), nil)
	default:
		return failure(http.StatusBadGateway, "forward_failed", st.Message(), nil)
	}
}

// decode reads a contract message from the body; an empty body when
// allowEmpty leaves message as it is.
func decode(body []byte, message proto.Message, allowEmpty bool) *Response {
	if len(strings.TrimSpace(string(body))) == 0 {
		if allowEmpty {
			return nil
		}
		answer := failure(http.StatusBadRequest, "invalid_request", "request body is required", nil)
		return &answer
	}
	if err := jsonIn.Unmarshal(body, message); err != nil {
		answer := failure(http.StatusBadRequest, "invalid_request", "request body: "+err.Error(), nil)
		return &answer
	}
	return nil
}

func query(request Request, key string) string {
	if values := request.Query[key]; len(values) > 0 {
		return strings.TrimSpace(values[0])
	}
	return ""
}
