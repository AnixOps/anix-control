package kernelforward

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grants map[string]bool

func (g grants) AuthorizeCapability(_ context.Context, _ packagebridge.HostIdentity, capability string) error {
	if g[capability] {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

type authorizerFunc func() error

func (a authorizerFunc) AuthorizeCapability(context.Context, packagebridge.HostIdentity, string) error {
	return a()
}

var host = packagebridge.HostIdentity{PackageID: "forward", Version: "4.2.0", Generation: 1}

// Every ForwardControl call is authorized for kernel.forward.v1 against the
// calling host: a host without it, a fenced generation and a failed check
// are refused before anything is read.
func TestForwardControlAuthorization(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	calls := map[string]func(forwardv1.ForwardControlServer) error{
		"CreateRoute": func(s forwardv1.ForwardControlServer) error {
			_, err := s.CreateRoute(f.ctx, &forwardv1.CreateRouteRequest{RequestId: "r", Route: twoHop(0)})
			return err
		},
		"UpdateRoute": func(s forwardv1.ForwardControlServer) error {
			_, err := s.UpdateRoute(f.ctx, &forwardv1.UpdateRouteRequest{RequestId: "r"})
			return err
		},
		"DeleteRoute": func(s forwardv1.ForwardControlServer) error {
			_, err := s.DeleteRoute(f.ctx, &forwardv1.DeleteRouteRequest{RequestId: "r", RouteId: "x"})
			return err
		},
		"GetRoute": func(s forwardv1.ForwardControlServer) error {
			_, err := s.GetRoute(f.ctx, &forwardv1.GetRouteRequest{RouteId: "x"})
			return err
		},
		"ListRoutes": func(s forwardv1.ForwardControlServer) error {
			_, err := s.ListRoutes(f.ctx, &forwardv1.ListRoutesRequest{})
			return err
		},
		"PlanRoute": func(s forwardv1.ForwardControlServer) error {
			_, err := s.PlanRoute(f.ctx, &forwardv1.PlanRouteRequest{Route: twoHop(0)})
			return err
		},
		"GetRouteStats": func(s forwardv1.ForwardControlServer) error {
			_, err := s.GetRouteStats(f.ctx, &forwardv1.GetRouteStatsRequest{RouteId: "x"})
			return err
		},
		"GetRouteHealth": func(s forwardv1.ForwardControlServer) error {
			_, err := s.GetRouteHealth(f.ctx, &forwardv1.GetRouteHealthRequest{RouteId: "x"})
			return err
		},
		"DiagnoseRoute": func(s forwardv1.ForwardControlServer) error {
			_, err := s.DiagnoseRoute(f.ctx, &forwardv1.DiagnoseRouteRequest{RouteId: "x"})
			return err
		},
		"ListNodes": func(s forwardv1.ForwardControlServer) error {
			_, err := s.ListNodes(f.ctx, &forwardv1.ListNodesRequest{})
			return err
		},
		"GetNode": func(s forwardv1.ForwardControlServer) error {
			_, err := s.GetNode(f.ctx, &forwardv1.GetNodeRequest{NodeRef: "forward-11"})
			return err
		},
		"SetNodeSettings": func(s forwardv1.ForwardControlServer) error {
			_, err := s.SetNodeSettings(f.ctx, &forwardv1.SetNodeSettingsRequest{NodeRef: "forward-11"})
			return err
		},
		"CreateForwardNode": func(s forwardv1.ForwardControlServer) error {
			_, err := s.CreateForwardNode(f.ctx, &forwardv1.CreateForwardNodeRequest{RequestId: "n", Node: &forwardv1.ForwardNodeRecord{Name: "n", Host: "h"}})
			return err
		},
		"UpdateForwardNode": func(s forwardv1.ForwardControlServer) error {
			_, err := s.UpdateForwardNode(f.ctx, &forwardv1.UpdateForwardNodeRequest{RequestId: "n", Node: &forwardv1.ForwardNodeRecord{Id: 11, Name: "n", Host: "h"}})
			return err
		},
		"DeleteForwardNode": func(s forwardv1.ForwardControlServer) error {
			_, err := s.DeleteForwardNode(f.ctx, &forwardv1.DeleteForwardNodeRequest{RequestId: "n", Id: 13})
			return err
		},
		"GetTraffic": func(s forwardv1.ForwardControlServer) error {
			_, err := s.GetTraffic(f.ctx, &forwardv1.GetTrafficRequest{})
			return err
		},
	}
	refusals := []struct {
		name       string
		authorizer Authorizer
		service    *Service
		code       codes.Code
	}{
		{"other capability", grants{service.CapabilityTelemetryDashboard: true}, f.service, codes.PermissionDenied},
		{"fenced", authorizerFunc(func() error { return packagebridge.ErrHostFenced }), f.service, codes.PermissionDenied},
		{"check failed", authorizerFunc(func() error { return errors.New("database down") }), f.service, codes.Unavailable},
		{"no authorizer", nil, f.service, codes.Unavailable},
		{"no service", grants{service.CapabilityForward: true}, nil, codes.Unavailable},
	}
	for _, refusal := range refusals {
		server := (&Server{Service: refusal.service, Authorizer: refusal.authorizer}).For(host)
		for name, call := range calls {
			assert.Equal(t, refusal.code, status.Code(call(server)), "%s: %s", refusal.name, name)
		}
	}
	var routes int64
	require.NoError(t, f.db.Table("v4_kernel_forward_route").Count(&routes).Error)
	assert.Zero(t, routes, "a refused call writes nothing")
	var nodes int64
	require.NoError(t, f.db.Table("v2_forward_node").Count(&nodes).Error)
	assert.Equal(t, int64(3), nodes, "a refused node write writes nothing")
}

// The contract's answers and codes over the service.
func TestForwardControlServer(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	server := (&Server{Service: f.service, Authorizer: grants{service.CapabilityForward: true}}).For(host)
	ctx := f.ctx

	created, err := server.CreateRoute(ctx, &forwardv1.CreateRouteRequest{RequestId: "c1", Route: twoHop(0)})
	require.NoError(t, err)
	id := created.GetRoute().GetId()
	require.NotEmpty(t, id)

	// A refusal carries the response, with its violations, as a detail.
	bad := twoHop(0)
	bad.Targets = nil
	_, err = server.CreateRoute(ctx, &forwardv1.CreateRouteRequest{RequestId: "c2", Route: bad})
	st := status.Convert(err)
	require.Equal(t, codes.InvalidArgument, st.Code())
	require.Len(t, st.Details(), 1)
	answer, ok := st.Details()[0].(*forwardv1.CreateRouteResponse)
	require.True(t, ok)
	require.NotEmpty(t, answer.GetViolations())
	assert.Equal(t, "targets", answer.GetViolations()[0].GetField())
	unknown := twoHop(0)
	unknown.Hops[1].NodeRefs = []string{"forward-99"}
	_, err = server.CreateRoute(ctx, &forwardv1.CreateRouteRequest{RequestId: "c3", Route: unknown})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	_, err = server.CreateRoute(ctx, &forwardv1.CreateRouteRequest{RequestId: "c1", Route: unknown})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err), "a request id reused for another request")
	_, err = server.CreateRoute(ctx, &forwardv1.CreateRouteRequest{Route: twoHop(0)})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	route := created.GetRoute()
	route.Name = "renamed"
	_, err = server.UpdateRoute(ctx, &forwardv1.UpdateRouteRequest{RequestId: "u1", Route: route, ExpectedRevision: 9})
	assert.Equal(t, codes.Aborted, status.Code(err))
	broken := twoHop(0)
	broken.Id = id
	broken.Hops[0].NodeRefs = []string{"forward-99"}
	_, err = server.UpdateRoute(ctx, &forwardv1.UpdateRouteRequest{RequestId: "u2", Route: broken, ExpectedRevision: 1})
	st = status.Convert(err)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
	require.Len(t, st.Details(), 1)
	assert.Equal(t, id, st.Details()[0].(*forwardv1.UpdateRouteResponse).GetViolations()[0].GetRouteId())
	updated, err := server.UpdateRoute(ctx, &forwardv1.UpdateRouteRequest{RequestId: "u3", Route: route, ExpectedRevision: 1})
	require.NoError(t, err)
	assert.EqualValues(t, 2, updated.GetRoute().GetRevision())

	got, err := server.GetRoute(ctx, &forwardv1.GetRouteRequest{RouteId: id})
	require.NoError(t, err)
	assert.Equal(t, "renamed", got.GetRoute().GetName())
	_, err = server.GetRoute(ctx, &forwardv1.GetRouteRequest{RouteId: "missing"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	list, err := server.ListRoutes(ctx, &forwardv1.ListRoutesRequest{Owner: "admin"})
	require.NoError(t, err)
	assert.Len(t, list.GetRoutes(), 1)
	plan, err := server.PlanRoute(ctx, &forwardv1.PlanRouteRequest{Route: twoHop(0)})
	require.NoError(t, err)
	assert.Len(t, plan.GetAllocations(), 2)
	_, err = server.PlanRoute(ctx, &forwardv1.PlanRouteRequest{})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	f.report(entry, start.Add(time.Minute), 2, counter(id, 0, "e1", 10, 20))
	stats, err := server.GetRouteStats(ctx, &forwardv1.GetRouteStatsRequest{RouteId: id})
	require.NoError(t, err)
	require.Len(t, stats.GetCounters(), 1)
	assert.EqualValues(t, 20, stats.GetCounters()[0].GetDownBytes())
	health, err := server.GetRouteHealth(ctx, &forwardv1.GetRouteHealthRequest{RouteId: id})
	require.NoError(t, err)
	assert.Empty(t, health.GetHealth())
	_, err = server.GetRouteStats(ctx, &forwardv1.GetRouteStatsRequest{RouteId: "missing"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	_, err = server.GetRouteHealth(ctx, &forwardv1.GetRouteHealthRequest{RouteId: "missing"})
	assert.Equal(t, codes.NotFound, status.Code(err))

	// Without node checks the diagnosis answers Control's records and its
	// own dials (refused in tests).
	diagnosis, err := server.DiagnoseRoute(ctx, &forwardv1.DiagnoseRouteRequest{RouteId: id, TimeoutMs: 2000})
	require.NoError(t, err)
	assert.False(t, diagnosis.GetOk())
	assert.Equal(t, id, diagnosis.GetRouteId())
	_, err = server.DiagnoseRoute(ctx, &forwardv1.DiagnoseRouteRequest{RouteId: "missing"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	_, err = server.DiagnoseRoute(ctx, &forwardv1.DiagnoseRouteRequest{})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// A delete the other routes' plan refuses names them.
	other := f.create("c4", twoHop(0))
	f.hello(exit, nftCaps(forwardv1.Engine_ENGINE_GOST))
	_, err = server.DeleteRoute(ctx, &forwardv1.DeleteRouteRequest{RequestId: "d1", RouteId: id})
	st = status.Convert(err)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
	require.Len(t, st.Details(), 1)
	assert.Equal(t, other.GetId(), st.Details()[0].(*forwardv1.PlanRouteResponse).GetViolations()[0].GetRouteId())
	f.hello(exit, nftCaps())
	_, err = server.DeleteRoute(ctx, &forwardv1.DeleteRouteRequest{RequestId: "d1", RouteId: id})
	require.NoError(t, err)
	_, err = server.DeleteRoute(ctx, &forwardv1.DeleteRouteRequest{RequestId: "d2", RouteId: id})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestFailureCodes(t *testing.T) {
	for err, code := range map[error]codes.Code{
		ErrInvalidReport:            codes.InvalidArgument,
		context.Canceled:            codes.Canceled,
		context.DeadlineExceeded:    codes.DeadlineExceeded,
		errors.New("database down"): codes.Internal,
	} {
		assert.Equal(t, code, status.Code(failure(err, nil)), err.Error())
	}
	// A refusal without an answer has no details.
	st := status.Convert(failure(&RefusedError{}, nil))
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Empty(t, st.Details())
}

// Route ids are ULIDs: 26 Crockford characters whose first ten encode the
// creation time, unique, and accepted by validation.
func TestULID(t *testing.T) {
	at := time.UnixMilli(1469922850259)
	id := newULID(at)
	require.Len(t, id, 26)
	assert.Equal(t, "01ARZ3NDEK", id[:10], "the ULID specification's example time")
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		next := newULID(start)
		require.False(t, seen[next])
		seen[next] = true
		require.Equal(t, strings.ToUpper(next), next)
		require.LessOrEqual(t, next[0], byte('7'))
	}
	var max [16]byte
	for i := range max {
		max[i] = 0xff
	}
	assert.Equal(t, "7ZZZZZZZZZZZZZZZZZZZZZZZZZ", encodeULID(max))
}

// The maintenance worker runs until its context ends.
func TestRun(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan struct{})
	go func() {
		f.service.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}
