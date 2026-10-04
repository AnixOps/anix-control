package v4api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// fakeForward is a ForwardControl client with one stored route and node.
type fakeForward struct {
	forwardv1.ForwardControlClient
	route    *forwardv1.Route
	node     *forwardv1.NodeSummary
	requests []string
	updates  []*forwardv1.UpdateRouteRequest
	nodeOps  []proto.Message
	err      error
}

func newFake() *fakeForward {
	return &fakeForward{
		route: &forwardv1.Route{Id: "01R", Owner: "admin", Name: "hk-jp", Revision: 3},
		node: &forwardv1.NodeSummary{NodeRef: "forward-7", Kind: "forward", Record: &forwardv1.ForwardNodeRecord{
			Id: 7, Name: "relay", Host: "192.0.2.7", Transport: forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE, Enabled: true,
		}},
	}
}

func (f *fakeForward) CreateRoute(_ context.Context, in *forwardv1.CreateRouteRequest, _ ...grpc.CallOption) (*forwardv1.CreateRouteResponse, error) {
	f.requests = append(f.requests, in.GetRequestId())
	if f.err != nil {
		return nil, f.err
	}
	route := proto.Clone(in.GetRoute()).(*forwardv1.Route)
	route.Id, route.Revision = "01NEW", 1
	return &forwardv1.CreateRouteResponse{Route: route}, nil
}

func (f *fakeForward) GetRoute(_ context.Context, in *forwardv1.GetRouteRequest, _ ...grpc.CallOption) (*forwardv1.GetRouteResponse, error) {
	if in.GetRouteId() != f.route.GetId() {
		return nil, status.Error(codes.NotFound, "forward route not found")
	}
	return &forwardv1.GetRouteResponse{Route: f.route, Enforced: "quota"}, nil
}

func (f *fakeForward) UpdateRoute(_ context.Context, in *forwardv1.UpdateRouteRequest, _ ...grpc.CallOption) (*forwardv1.UpdateRouteResponse, error) {
	f.updates = append(f.updates, in)
	if f.err != nil {
		return nil, f.err
	}
	route := proto.Clone(in.GetRoute()).(*forwardv1.Route)
	route.Revision = in.GetExpectedRevision() + 1
	return &forwardv1.UpdateRouteResponse{Route: route}, nil
}

func (f *fakeForward) DeleteRoute(_ context.Context, in *forwardv1.DeleteRouteRequest, _ ...grpc.CallOption) (*forwardv1.DeleteRouteResponse, error) {
	f.requests = append(f.requests, in.GetRequestId())
	return &forwardv1.DeleteRouteResponse{}, f.err
}

func (f *fakeForward) ListRoutes(context.Context, *forwardv1.ListRoutesRequest, ...grpc.CallOption) (*forwardv1.ListRoutesResponse, error) {
	return &forwardv1.ListRoutesResponse{Routes: []*forwardv1.Route{f.route}, Enforced: map[string]string{"01R": "expired"}}, nil
}

func (f *fakeForward) ListNodes(context.Context, *forwardv1.ListNodesRequest, ...grpc.CallOption) (*forwardv1.ListNodesResponse, error) {
	return &forwardv1.ListNodesResponse{Nodes: []*forwardv1.NodeSummary{f.node}}, nil
}

func (f *fakeForward) GetNode(_ context.Context, in *forwardv1.GetNodeRequest, _ ...grpc.CallOption) (*forwardv1.GetNodeResponse, error) {
	if in.GetNodeRef() != f.node.GetNodeRef() {
		return nil, status.Error(codes.NotFound, "forward node not found")
	}
	return &forwardv1.GetNodeResponse{Node: f.node}, nil
}

func (f *fakeForward) UpdateForwardNode(_ context.Context, in *forwardv1.UpdateForwardNodeRequest, _ ...grpc.CallOption) (*forwardv1.UpdateForwardNodeResponse, error) {
	f.nodeOps = append(f.nodeOps, in)
	if f.err != nil {
		return nil, f.err
	}
	node := proto.Clone(f.node).(*forwardv1.NodeSummary)
	node.Record = in.GetNode()
	return &forwardv1.UpdateForwardNodeResponse{Node: node}, nil
}

func (f *fakeForward) CreateForwardNode(_ context.Context, in *forwardv1.CreateForwardNodeRequest, _ ...grpc.CallOption) (*forwardv1.CreateForwardNodeResponse, error) {
	f.nodeOps = append(f.nodeOps, in)
	return &forwardv1.CreateForwardNodeResponse{Node: &forwardv1.NodeSummary{NodeRef: "forward-8", Record: in.GetNode()}}, nil
}

func (f *fakeForward) DeleteForwardNode(_ context.Context, in *forwardv1.DeleteForwardNodeRequest, _ ...grpc.CallOption) (*forwardv1.DeleteForwardNodeResponse, error) {
	f.nodeOps = append(f.nodeOps, in)
	return &forwardv1.DeleteForwardNodeResponse{}, f.err
}

func serve(t *testing.T, fake *fakeForward, method, path, body string, headers ...string) (int, map[string]any) {
	t.Helper()
	svc := &Service{Forward: fake, NewRequestID: func() string { return "generated" }}
	request := Request{Method: method, Path: ControlPrefix + path, Body: []byte(body), Query: map[string][]string{}}
	if len(headers) > 0 {
		request.IdempotencyKey = headers[0]
	}
	if i := strings.Index(path, "?"); i >= 0 {
		request.Path = ControlPrefix + path[:i]
		for _, pair := range strings.Split(path[i+1:], "&") {
			key, value, _ := strings.Cut(pair, "=")
			request.Query[key] = append(request.Query[key], value)
		}
	}
	answer := svc.Serve(context.Background(), request)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(answer.Body, &decoded), string(answer.Body))
	return answer.StatusCode, decoded
}

func errorCode(body map[string]any) string {
	failure, _ := body["error"].(map[string]any)
	code, _ := failure["code"].(string)
	return code
}

func TestServeMatchesPaths(t *testing.T) {
	fake := newFake()
	code, body := serve(t, fake, http.MethodGet, "/nowhere", "")
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, "not_found", errorCode(body))
	code, body = serve(t, fake, http.MethodPatch, "/routes", "")
	assert.Equal(t, http.StatusMethodNotAllowed, code)
	assert.Equal(t, "method_not_allowed", errorCode(body))

	svc := &Service{}
	answer := svc.Serve(context.Background(), Request{Method: http.MethodGet, Path: PublicPrefix + "/routes"})
	assert.Equal(t, http.StatusServiceUnavailable, answer.StatusCode, "no ForwardControl connection")
	answer = svc.Serve(context.Background(), Request{Method: http.MethodGet, Path: "/api/v4/forwarding/routes"})
	assert.Equal(t, http.StatusNotFound, answer.StatusCode)

	code, body = serve(t, fake, http.MethodGet, "/routes", "")
	require.Equal(t, http.StatusOK, code)
	routes := body["data"].(map[string]any)["routes"].([]any)
	require.Len(t, routes, 1)
	assert.Equal(t, "expired", routes[0].(map[string]any)["enforced"])
	assert.Equal(t, "hk-jp", routes[0].(map[string]any)["route"].(map[string]any)["name"])
}

// Writes carry the Idempotency-Key as their request id, a generated one
// without it; the owner is the administrator's.
func TestCreateRouteRequestIDsAndOwner(t *testing.T) {
	fake := newFake()
	code, body := serve(t, fake, http.MethodPost, "/routes", `{"name":"x","listen":{"protocol":"L4_PROTOCOL_TCP"}}`, "key-1")
	require.Equal(t, http.StatusCreated, code, body)
	assert.Equal(t, "admin", body["data"].(map[string]any)["route"].(map[string]any)["owner"])
	code, _ = serve(t, fake, http.MethodPost, "/routes", `{"name":"y"}`)
	require.Equal(t, http.StatusCreated, code)
	assert.Equal(t, []string{"key-1", "generated"}, fake.requests)

	code, body = serve(t, fake, http.MethodPost, "/routes", `{"name":"y","owner":"user:5"}`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "invalid_request", errorCode(body))
	code, body = serve(t, fake, http.MethodPost, "/routes", `{"nam":"typo"}`)
	assert.Equal(t, http.StatusBadRequest, code, "unknown fields are refused")
	assert.Equal(t, "invalid_request", errorCode(body))
	code, _ = serve(t, fake, http.MethodPost, "/routes", ``)
	assert.Equal(t, http.StatusBadRequest, code)
}

// A refusal's violations come from the status details, with their codes.
func TestRefusalsCarryViolations(t *testing.T) {
	fake := newFake()
	st, err := status.New(codes.InvalidArgument, "forward route refused").WithDetails(&forwardv1.CreateRouteResponse{Violations: []*forwardv1.Violation{
		{Field: "hops[1].ingress.security", Message: "nftables cannot originate TLS", Code: "link_unsupported"},
	}})
	require.NoError(t, err)
	fake.err = st.Err()
	code, body := serve(t, fake, http.MethodPost, "/routes", `{"name":"x"}`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "invalid_route", errorCode(body))
	violations := body["error"].(map[string]any)["violations"].([]any)
	require.Len(t, violations, 1)
	assert.Equal(t, "link_unsupported", violations[0].(map[string]any)["code"])
	assert.Equal(t, "hops[1].ingress.security", violations[0].(map[string]any)["field"])

	st, err = status.New(codes.FailedPrecondition, "refused").WithDetails(&forwardv1.CreateRouteResponse{Violations: []*forwardv1.Violation{
		{Field: "listen.port", Code: "port_in_use", RouteId: "01OTHER"},
	}})
	require.NoError(t, err)
	fake.err = st.Err()
	code, body = serve(t, fake, http.MethodPost, "/routes", `{"name":"x"}`)
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "refused", errorCode(body))
	assert.Equal(t, "01OTHER", body["error"].(map[string]any)["violations"].([]any)[0].(map[string]any)["route_id"])

	for grpcCode, want := range map[codes.Code]struct {
		status int
		code   string
	}{
		codes.FailedPrecondition: {http.StatusConflict, "idempotency_conflict"},
		codes.Aborted:            {http.StatusConflict, "revision_conflict"},
		codes.NotFound:           {http.StatusNotFound, "not_found"},
		codes.Unavailable:        {http.StatusServiceUnavailable, "forward_unavailable"},
		codes.PermissionDenied:   {http.StatusServiceUnavailable, "forward_unavailable"},
		codes.Unimplemented:      {http.StatusNotImplemented, "not_implemented"},
		codes.Internal:           {http.StatusBadGateway, "forward_failed"},
	} {
		fake.err = status.Error(grpcCode, "x")
		code, body = serve(t, fake, http.MethodPost, "/routes", `{"name":"x"}`)
		assert.Equal(t, want.status, code, grpcCode.String())
		assert.Equal(t, want.code, errorCode(body), grpcCode.String())
	}
}

func TestUpdateRouteNeedsItsRevision(t *testing.T) {
	fake := newFake()
	code, body := serve(t, fake, http.MethodPut, "/routes/01R", `{"name":"x"}`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "invalid_request", errorCode(body))
	code, _ = serve(t, fake, http.MethodPut, "/routes/01R", `{"id":"01X","revision":3}`)
	assert.Equal(t, http.StatusBadRequest, code)
	code, body = serve(t, fake, http.MethodPut, "/routes/01R", `{"name":"x","revision":3}`, "k")
	require.Equal(t, http.StatusOK, code, body)
	require.Len(t, fake.updates, 1)
	assert.Equal(t, uint64(3), fake.updates[0].GetExpectedRevision())
	assert.Equal(t, "01R", fake.updates[0].GetRoute().GetId())
	assert.Equal(t, "k", fake.updates[0].GetRequestId())
}

// Pause and resume write the route back at the revision they read; a route
// already so is answered unchanged.
func TestPauseAndResume(t *testing.T) {
	fake := newFake()
	code, body := serve(t, fake, http.MethodPost, "/routes/01R/pause", "", "op")
	require.Equal(t, http.StatusOK, code, body)
	require.Len(t, fake.updates, 1)
	assert.True(t, fake.updates[0].GetRoute().GetPaused())
	assert.Equal(t, uint64(3), fake.updates[0].GetExpectedRevision())
	assert.Equal(t, "op:pause", fake.updates[0].GetRequestId())
	assert.Equal(t, "quota", body["data"].(map[string]any)["enforced"])

	code, _ = serve(t, fake, http.MethodPost, "/routes/01R/resume", "")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, fake.updates, 1, "a running route is not written")

	code, body = serve(t, fake, http.MethodPost, "/routes/404/pause", "")
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, "not_found", errorCode(body))
}

// /ansible-machines reaches only the forward nodes on the Ansible
// transport, and forces it on create.
func TestAnsibleMachinesAreThatTransport(t *testing.T) {
	fake := newFake()
	code, body := serve(t, fake, http.MethodGet, "/ansible-machines/7", "")
	require.Equal(t, http.StatusOK, code, body)
	fake.node.Record.Transport = forwardv1.NodeTransport_NODE_TRANSPORT_AGENT
	code, _ = serve(t, fake, http.MethodGet, "/ansible-machines/7", "")
	assert.Equal(t, http.StatusNotFound, code)
	code, _ = serve(t, fake, http.MethodDelete, "/ansible-machines/7", "")
	assert.Equal(t, http.StatusNotFound, code)
	assert.Empty(t, fake.nodeOps)
	code, _ = serve(t, fake, http.MethodGet, "/ansible-machines/x", "")
	assert.Equal(t, http.StatusBadRequest, code)

	code, body = serve(t, fake, http.MethodPost, "/ansible-machines", `{"node":{"name":"m","host":"192.0.2.9","transport":"NODE_TRANSPORT_AGENT"}}`)
	require.Equal(t, http.StatusCreated, code, body)
	created := fake.nodeOps[0].(*forwardv1.CreateForwardNodeRequest)
	assert.Equal(t, forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE, created.GetNode().GetTransport())
}

func TestToggleNode(t *testing.T) {
	fake := newFake()
	code, _ := serve(t, fake, http.MethodPost, "/nodes/forward-7/toggle", `{}`)
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = serve(t, fake, http.MethodPost, "/nodes/proxy-7/toggle", `{"enabled":false}`)
	assert.Equal(t, http.StatusBadRequest, code)
	code, body := serve(t, fake, http.MethodPost, "/nodes/forward-7/toggle", `{"enabled":false}`, "t")
	require.Equal(t, http.StatusOK, code, body)
	update := fake.nodeOps[0].(*forwardv1.UpdateForwardNodeRequest)
	assert.False(t, update.GetNode().GetEnabled())
	assert.Equal(t, "relay", update.GetNode().GetName(), "the other fields are kept")
	assert.Equal(t, "t:disable", update.GetRequestId())

	st, err := status.New(codes.FailedPrecondition, "in use").WithDetails(&forwardv1.UpdateForwardNodeResponse{Violations: []*forwardv1.Violation{
		{Field: "hops[0].node_refs[0]", Code: "node_in_use", RouteId: "01R"},
	}})
	require.NoError(t, err)
	fake.err = st.Err()
	code, body = serve(t, fake, http.MethodPost, "/nodes/forward-7/toggle", `{"enabled":false}`)
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "node_in_use", body["error"].(map[string]any)["violations"].([]any)[0].(map[string]any)["code"])
}

// Every endpoint is in both editions (H23); a commercial one must sit under
// a prefix config/editions.json reserves, which the kernel hides in the
// community edition (internal/tests/forwardv4 checks the table).
func TestEndpointsAreCommunityAdminEndpoints(t *testing.T) {
	seen := map[string]bool{}
	for _, endpoint := range Endpoints() {
		key := endpoint.Method + " " + endpoint.Pattern
		assert.False(t, seen[key], "duplicate %s", key)
		seen[key] = true
		assert.Equal(t, EditionAll, endpoint.Edition, key)
		assert.NotContains(t, endpoint.Pattern, "/self", "user self-service is commercial (v4.3)")
		assert.NotEmpty(t, endpoint.Summary, key)
	}
	assert.Len(t, seen, 40)
}

// diagnoseForward answers DiagnoseRoute; err refuses it.
type diagnoseForward struct {
	*fakeForward
	diagnosed []*forwardv1.DiagnoseRouteRequest
}

func (f *diagnoseForward) DiagnoseRoute(_ context.Context, in *forwardv1.DiagnoseRouteRequest, _ ...grpc.CallOption) (*forwardv1.DiagnoseRouteResponse, error) {
	f.diagnosed = append(f.diagnosed, in)
	if f.err != nil {
		return nil, f.err
	}
	if in.GetRouteId() != f.route.GetId() {
		return nil, status.Error(codes.NotFound, "forward route not found")
	}
	return &forwardv1.DiagnoseRouteResponse{RouteId: in.GetRouteId(), StartedAtUnixMs: 1700000000000, Steps: []*forwardv1.DiagnoseStep{{
		NodeRef: "forward-7", HopIndex: 1, Kind: forwardv1.ProbeKind_PROBE_KIND_DELIVERY, Vantage: forwardv1.DiagnoseVantage_DIAGNOSE_VANTAGE_NODE,
		Target: "198.51.100.10:443", Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP,
		Result: &forwardv1.ProbeResult{Status: forwardv1.ProbeStatus_PROBE_STATUS_FAILED, Code: "unreachable", Message: "connection refused"},
	}}, Nodes: []*forwardv1.DiagnoseNode{{NodeRef: "forward-7", Connected: true, NodeVantage: true}}}, nil
}

func TestDiagnoseRoute(t *testing.T) {
	fake := &diagnoseForward{fakeForward: newFake()}
	svc := &Service{Forward: fake}
	post := func(path, body string) (int, map[string]any) {
		answer := svc.Serve(context.Background(), Request{Method: http.MethodPost, Path: PublicPrefix + path, Body: []byte(body)})
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(answer.Body, &decoded), string(answer.Body))
		return answer.StatusCode, decoded
	}
	code, body := post("/routes/01R/diagnose", "")
	require.Equal(t, http.StatusOK, code, body)
	data := body["data"].(map[string]any)
	assert.Equal(t, "01R", data["route_id"])
	assert.Equal(t, "1700000000000", data["started_at_unix_ms"], "64-bit integers are strings")
	step := data["steps"].([]any)[0].(map[string]any)
	assert.Equal(t, "PROBE_KIND_DELIVERY", step["kind"])
	assert.Equal(t, "DIAGNOSE_VANTAGE_NODE", step["vantage"])
	assert.Equal(t, "PROBE_STATUS_FAILED", step["result"].(map[string]any)["status"])
	assert.Equal(t, true, data["nodes"].([]any)[0].(map[string]any)["node_vantage"])
	assert.Nil(t, data["ok"], "false is left out")

	code, _ = post("/routes/01R/diagnose", `{"timeout_ms": 5000, "route_id": "other"}`)
	require.Equal(t, http.StatusOK, code)
	last := fake.diagnosed[len(fake.diagnosed)-1]
	assert.EqualValues(t, 5000, last.GetTimeoutMs())
	assert.Equal(t, "01R", last.GetRouteId(), "the path names the route")

	code, body = post("/routes/01R/diagnose", `{"timeout": 1}`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "invalid_request", errorCode(body))
	code, body = post("/routes/missing/diagnose", "")
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, "not_found", errorCode(body))
	fake.err = status.Error(codes.ResourceExhausted, "too many route diagnoses are running; retry shortly")
	code, body = post("/routes/01R/diagnose", "")
	assert.Equal(t, http.StatusTooManyRequests, code)
	assert.Equal(t, "rate_limited", errorCode(body))
	answer := svc.Serve(context.Background(), Request{Method: http.MethodGet, Path: PublicPrefix + "/routes/01R/diagnose"})
	assert.Equal(t, http.StatusMethodNotAllowed, answer.StatusCode)
}

// The list answers say whether the caller may DELETE (F5b D7): the kernel
// passes the super administrator rule as Request.SuperAdmin.
func TestListAnswersCarryCanDelete(t *testing.T) {
	fake := newFake()
	for _, superAdmin := range []bool{false, true} {
		svc := &Service{Forward: fake}
		for _, path := range []string{"/routes", "/nodes", "/ansible-machines"} {
			answer := svc.Serve(context.Background(), Request{Method: http.MethodGet, Path: ControlPrefix + path, SuperAdmin: superAdmin})
			require.Equal(t, http.StatusOK, answer.StatusCode, string(answer.Body))
			var decoded struct {
				Data map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(answer.Body, &decoded))
			assert.Equal(t, superAdmin, decoded.Data["can_delete"], path)
		}
	}
	// A single route's answer has no can_delete.
	_, body := serve(t, fake, http.MethodGet, "/routes/01R", "")
	assert.NotContains(t, body["data"], "can_delete")
}
