package bridgecontract

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// forwardGrants holds kernel.forward.v1 for the listed packages, and
// records who called.
type forwardGrants struct {
	mu       sync.Mutex
	packages map[string]bool
	seen     []packagebridge.HostIdentity
}

func (g *forwardGrants) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seen = append(g.seen, host)
	if capability == service.CapabilityForward && g.packages[host.PackageID] {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

// newForwardKernel is the kernel's ForwardControl on db, with forward nodes
// 11 and 12 whose Agents reported the nftables engine.
func newForwardKernel(t *testing.T, db *gorm.DB, grants *forwardGrants) *kernelforward.Server {
	t.Helper()
	require.NoError(t, db.AutoMigrate(append(model.KernelForwardModels(), &model.ForwardNode{}, &model.Node{}, &model.NodeProtocol{}, &model.EgressRoute{})...))
	require.NoError(t, db.AutoMigrate(model.NetworkModels()...))
	require.NoError(t, db.Create(&[]model.ForwardNode{
		{ID: 11, Name: "entry", Host: "192.0.2.11", Port: 7000, Enabled: true},
		{ID: 12, Name: "exit", Host: "192.0.2.12", Port: 7000, Enabled: true},
	}).Error)
	forward := &kernelforward.Service{DB: db,
		Probes: service.DiagnosisProbes{Dial: func(context.Context, string, string, time.Duration) (net.Conn, error) {
			return nil, errors.New("connection refused (test)")
		}}}
	caps := &forwardv1.NodeCapabilities{Engines: []*forwardv1.EngineCapabilities{{
		Engine: forwardv1.Engine_ENGINE_NFTABLES, Available: true, Udp: true,
		Strategies:     []forwardv1.BalanceStrategy{forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN},
		LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
	}}}
	for _, id := range []uint32{11, 12} {
		_, _, err := forward.RecordHello(context.Background(), agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: id}, caps, "4.2.0")
		require.NoError(t, err)
	}
	return &kernelforward.Server{Service: forward, Authorizer: grants}
}

func forwardRoute() *forwardv1.Route {
	return &forwardv1.Route{
		Owner:  "admin",
		Listen: &forwardv1.Listen{Port: 30443, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Hops: []*forwardv1.Hop{
			{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: forwardv1.Engine_ENGINE_NETWORK, NodeRefs: []string{"forward-11"}},
			{Role: forwardv1.HopRole_HOP_ROLE_EXIT, Engine: forwardv1.Engine_ENGINE_NETWORK, NodeRefs: []string{"forward-12"},
				Ingress: &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_NETWORK}},
		},
		Targets: []*forwardv1.Target{{Host: "198.51.100.10", Port: 443}},
	}
}

// exerciseForward drives ForwardControl through connection as an
// authorized package.
func exerciseForward(t *testing.T, connection grpc.ClientConnInterface) {
	t.Helper()
	client := forwardv1.NewForwardControlClient(connection)
	ctx := context.Background()
	created, err := client.CreateRoute(ctx, &forwardv1.CreateRouteRequest{RequestId: "create-1", Route: forwardRoute()})
	require.NoError(t, err)
	id := created.GetRoute().GetId()
	require.Len(t, id, 26)
	got, err := client.GetRoute(ctx, &forwardv1.GetRouteRequest{RouteId: id})
	require.NoError(t, err)
	require.EqualValues(t, 1, got.GetRoute().GetRevision())
	list, err := client.ListRoutes(ctx, &forwardv1.ListRoutesRequest{NodeRef: "forward-12"})
	require.NoError(t, err)
	require.Len(t, list.GetRoutes(), 1)
	plan, err := client.PlanRoute(ctx, &forwardv1.PlanRouteRequest{Route: forwardRoute()})
	require.NoError(t, err)
	require.Empty(t, plan.GetViolations())
	_, err = client.GetRouteStats(ctx, &forwardv1.GetRouteStatsRequest{RouteId: id})
	require.NoError(t, err)
	_, err = client.GetRouteHealth(ctx, &forwardv1.GetRouteHealthRequest{RouteId: id})
	require.NoError(t, err)
	bad := forwardRoute()
	bad.Targets = nil
	_, err = client.CreateRoute(ctx, &forwardv1.CreateRouteRequest{RequestId: "create-2", Route: bad})
	st := status.Convert(err)
	require.Equal(t, codes.InvalidArgument, st.Code())
	require.Len(t, st.Details(), 1, "the violations cross the bridge as a detail")
	require.NotEmpty(t, st.Details()[0].(*forwardv1.CreateRouteResponse).GetViolations())
	diagnosis, err := client.DiagnoseRoute(ctx, &forwardv1.DiagnoseRouteRequest{RouteId: id, TimeoutMs: 2000})
	require.NoError(t, err)
	require.Equal(t, id, diagnosis.GetRouteId())
	require.NotEmpty(t, diagnosis.GetSteps())
	_, err = client.DeleteRoute(ctx, &forwardv1.DeleteRouteRequest{RequestId: "delete-1", RouteId: id})
	require.NoError(t, err)
}

// A local host reaches ForwardControl over its bridge connection, as the
// session's host identity; a host without kernel.forward.v1 is refused.
func TestKernelForwardOverTheLocalBridge(t *testing.T) {
	nodeOpsDatabases(t, func(t *testing.T, db *gorm.DB) {
		grants := &forwardGrants{packages: map[string]bool{"plan": true}}
		kernel := newForwardKernel(t, db, grants)
		client := dialPlanSession(t, packagebridge.SessionOptions{KernelForward: kernel.For})
		exerciseForward(t, client.Conn())
		require.Equal(t, packagebridge.HostIdentity{PackageID: "plan", Version: "4.0.0", Generation: 3}, grants.seen[0])

		grants.mu.Lock()
		grants.packages = map[string]bool{}
		grants.mu.Unlock()
		_, err := forwardv1.NewForwardControlClient(client.Conn()).ListRoutes(context.Background(), &forwardv1.ListRoutesRequest{})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})
}

func TestKernelForwardIsAbsentWithoutAProvider(t *testing.T) {
	client := dialPlanSession(t, packagebridge.SessionOptions{})
	_, err := forwardv1.NewForwardControlClient(client.Conn()).ListRoutes(context.Background(), &forwardv1.ListRoutesRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
}
