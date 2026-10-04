package bridgecontract

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	modulepkiv1 "github.com/AnixOps/anix-control/sdk/api/modulepki/v1"
	"github.com/AnixOps/anix-control/sdk/moduletls"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/moduleruntime"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
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
	require.NoError(t, db.AutoMigrate(append(model.KernelForwardModels(), &model.ForwardNode{}, &model.Node{}, &model.NodeProtocol{})...))
	require.NoError(t, db.Create(&[]model.ForwardNode{
		{ID: 11, Name: "entry", Host: "192.0.2.11", Port: 7000, Enabled: true},
		{ID: 12, Name: "exit", Host: "192.0.2.12", Port: 7000, Enabled: true},
	}).Error)
	forward := &kernelforward.Service{DB: db, Cluster: func() string { return "prod" },
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
		Listen: &forwardv1.Listen{Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Hops: []*forwardv1.Hop{
			{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: forwardv1.Engine_ENGINE_NFTABLES, NodeRefs: []string{"forward-11"}},
			{Role: forwardv1.HopRole_HOP_ROLE_EXIT, Engine: forwardv1.Engine_ENGINE_NFTABLES, NodeRefs: []string{"forward-12"},
				Ingress: &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_RAW}},
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
	require.Len(t, plan.GetStates(), 2)
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

// A network module reaches ForwardControl on the mTLS module listener: each
// call runs as its bound instance's generation, and a fenced generation
// loses the contract.
func TestKernelForwardOverTheModuleListener(t *testing.T) {
	db := openSQLiteForTest(t, "kernel.db", "_pragma=journal_mode(WAL)")
	grants := &forwardGrants{packages: map[string]bool{"forward": true}}
	kernel := newForwardKernel(t, db, grants)
	pkiDB := openSQLiteForTest(t, "pki.db")
	require.NoError(t, pkiDB.AutoMigrate(&model.ServiceCA{}, &model.ModuleEnrollment{}, &model.ModuleCertificate{}))
	kek := make([]byte, 32)
	_, err := rand.Read(kek)
	require.NoError(t, err)
	authority, err := modulepki.New(modulepki.Options{DB: pkiDB, Cluster: "prod", KEK: kek})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(context.Background()))
	kernelTLS, err := authority.KernelTLS(context.Background(), time.Minute)
	require.NoError(t, err)

	allowlist, err := packagebridge.NewAllowlistWithFallback(nil)
	require.NoError(t, err)
	generation, err := packagebridge.NewGenerationSession(packagebridge.HostIdentity{PackageID: "forward", Version: "4.2.0", Generation: 5}, allowlist, packagebridge.SessionOptions{})
	require.NoError(t, err)
	bridge, err := packagebridge.NewModuleBridge(&nodeOpsBinder{generation: generation}, packagebridge.ModuleBridgeOptions{Cluster: "prod"})
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() {
		served <- (&moduleruntime.Listener{TLS: kernelTLS, Cluster: "prod", PKI: authority, Bridge: bridge, KernelForward: kernel.For}).Serve(ctx, listener)
	}()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-served)
	})

	kernelID, err := moduletls.Kernel("prod")
	require.NoError(t, err)
	dial := func(certificate *tls.Certificate) *tls.Config {
		source := moduletls.Source{Roots: kernelTLS.Roots}
		if certificate != nil {
			source.Certificate = func() (*tls.Certificate, error) { return certificate, nil }
		}
		return source.ClientConfig(moduletls.AcceptExactly(kernelID))
	}
	credential, _, err := authority.CreateEnrollment(context.Background(), modulepki.EnrollmentRequest{PackageID: "forward", TTL: time.Hour})
	require.NoError(t, err)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	require.NoError(t, err)
	anonymous, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(dial(nil))))
	require.NoError(t, err)
	t.Cleanup(func() { _ = anonymous.Close() })
	enrolled, err := modulepkiv1.NewModulePKIClient(anonymous).Enroll(context.Background(), &modulepkiv1.EnrollRequest{
		EnrollmentCredential: credential, PackageId: "forward", Cluster: "prod", CsrDer: csr,
	})
	require.NoError(t, err)
	certificate := &tls.Certificate{Certificate: [][]byte{enrolled.GetCertificate().GetCertificateDer()}, PrivateKey: key}

	_, err = forwardv1.NewForwardControlClient(anonymous).ListRoutes(context.Background(), &forwardv1.ListRoutesRequest{})
	require.Equal(t, codes.Unauthenticated, status.Code(err), "no client certificate")

	module, err := packagebridgesdk.DialNetwork(packagebridgesdk.NetworkConfig{
		KernelAddr: listener.Addr().String(), TLS: dial(certificate), PackageID: "forward", PackageVersion: "4.2.0",
		InstanceID: "pod-a", AdvertiseAddr: "10.0.0.7:7000", LeaseID: "lease-a",
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, module.Close()) })
	require.NoError(t, module.Bind(context.Background()))

	exerciseForward(t, module.Conn())
	grants.mu.Lock()
	require.Equal(t, packagebridge.HostIdentity{PackageID: "forward", Version: "4.2.0", Generation: 5}, grants.seen[0])
	grants.mu.Unlock()

	require.NoError(t, generation.Close())
	_, err = forwardv1.NewForwardControlClient(module.Conn()).ListRoutes(context.Background(), &forwardv1.ListRoutesRequest{})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "a fenced generation loses ForwardControl")
}
