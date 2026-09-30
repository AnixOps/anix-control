package moduleruntime

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	modulepkiv1 "github.com/AnixOps/anix-control/sdk/api/modulepki/v1"
	packagebridgev1 "github.com/AnixOps/anix-control/sdk/api/packagebridge/v1"
	"github.com/AnixOps/anix-control/sdk/moduletls"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeBinder struct {
	mu          sync.Mutex
	generations map[string]*packagebridge.GenerationSession
	bindings    []packagebridge.InstanceBinding
}

func (b *fakeBinder) BindInstance(_ context.Context, binding packagebridge.InstanceBinding) (*packagebridge.GenerationSession, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bindings = append(b.bindings, binding)
	generation, ok := b.generations[binding.PackageID]
	if !ok || generation.Identity().Version != binding.Version {
		return nil, packagebridge.ErrNotRemote
	}
	return generation, nil
}

type mutableClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *mutableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mutableClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type listenerFixture struct {
	authority  *modulepki.Authority
	address    string
	kernel     moduletls.Source
	generation *packagebridge.GenerationSession
	binder     *fakeBinder
	bridge     *packagebridge.ModuleBridge
	clock      *mutableClock
	calls      chan packagebridge.Call
}

// newPKIFixture creates the kernel CA and its TLS identity.
func newPKIFixture(t *testing.T) *listenerFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")+"?_pragma=busy_timeout(10000)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ServiceCA{}, &model.ModuleEnrollment{}, &model.ModuleCertificate{}))
	kek := make([]byte, 32)
	_, err = rand.Read(kek)
	require.NoError(t, err)
	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: "prod", KEK: kek})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(context.Background()))
	kernel, err := authority.KernelTLS(context.Background(), time.Minute)
	require.NoError(t, err)
	return &listenerFixture{authority: authority, kernel: kernel}
}

// serve runs the module listener with the fixture's bridge.
func (f *listenerFixture) serve(t *testing.T) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() {
		served <- (&Listener{TLS: f.kernel, Cluster: "prod", PKI: f.authority, Bridge: f.bridge}).Serve(ctx, listener)
	}()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-served)
	})
	f.address = listener.Addr().String()
}

func newListenerFixture(t *testing.T) *listenerFixture {
	t.Helper()
	fixture := newPKIFixture(t)
	calls := make(chan packagebridge.Call, 4)
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil, packagebridge.Operation{
		PackageID: "knowledge", RouteID: "knowledge.article.list", Name: "knowledge.article.list",
		Handler: func(_ context.Context, call packagebridge.Call) (packagebridge.Response, error) {
			calls <- call
			return packagebridge.Response{StatusCode: 200, Body: []byte(`{"ok":true}`)}, nil
		},
	})
	require.NoError(t, err)
	generation, err := packagebridge.NewGenerationSession(packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.1.0", Generation: 9}, allowlist, packagebridge.SessionOptions{})
	require.NoError(t, err)
	binder := &fakeBinder{generations: map[string]*packagebridge.GenerationSession{"knowledge": generation}}
	clock := &mutableClock{now: time.Now()}
	bridge, err := packagebridge.NewModuleBridge(binder, packagebridge.ModuleBridgeOptions{Cluster: "prod", Now: clock.Now})
	require.NoError(t, err)
	fixture.generation, fixture.binder, fixture.bridge, fixture.clock, fixture.calls = generation, binder, bridge, clock, calls
	fixture.serve(t)
	return fixture
}

// enroll returns a client certificate for packageID and the enrollment id.
func (f *listenerFixture) enroll(t *testing.T, packageID string) (*tls.Certificate, string) {
	t.Helper()
	credential, enrollment, err := f.authority.CreateEnrollment(context.Background(), modulepki.EnrollmentRequest{PackageID: packageID, TTL: time.Hour})
	require.NoError(t, err)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	require.NoError(t, err)
	response, err := modulepkiv1.NewModulePKIClient(f.dial(t, nil)).Enroll(context.Background(), &modulepkiv1.EnrollRequest{
		EnrollmentCredential: credential, PackageId: packageID, Cluster: "prod", CsrDer: csr,
	})
	require.NoError(t, err)
	return &tls.Certificate{Certificate: [][]byte{response.GetCertificate().GetCertificateDer()}, PrivateKey: key}, enrollment.ID
}

func (f *listenerFixture) dial(t *testing.T, certificate *tls.Certificate) *grpc.ClientConn {
	t.Helper()
	kernelID, err := moduletls.Kernel("prod")
	require.NoError(t, err)
	source := moduletls.Source{Roots: f.kernel.Roots}
	if certificate != nil {
		source.Certificate = func() (*tls.Certificate, error) { return certificate, nil }
	}
	connection, err := grpc.NewClient(f.address, grpc.WithTransportCredentials(credentials.NewTLS(source.ClientConfig(moduletls.AcceptExactly(kernelID)))))
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

func bind(t *testing.T, client packagebridgev1.KernelPackageBridgeClient, instance string) context.Context {
	t.Helper()
	response, err := client.Bind(context.Background(), &packagebridgev1.BindRequest{
		PackageId: "knowledge", PackageVersion: "4.1.0", InstanceId: instance,
		AdvertiseAddr: "10.0.0.7:7000", LeaseId: "lease-" + instance, ImageDigest: "sha256:abc",
	})
	require.NoError(t, err)
	require.EqualValues(t, 9, response.GetRouteGeneration())
	require.EqualValues(t, 5000, response.GetHeartbeatIntervalMillis())
	return metadata.AppendToOutgoingContext(context.Background(), packagebridge.SessionMetadataKey, base64.RawURLEncoding.EncodeToString(response.GetSessionToken()))
}

func invoke(ctx context.Context, client packagebridgev1.KernelPackageBridgeClient, capability []byte) error {
	_, err := client.Invoke(ctx, &packagebridgev1.InvokeRequest{Capability: capability, Operation: "knowledge.article.list"})
	return err
}

func mint(t *testing.T, generation *packagebridge.GenerationSession) []byte {
	t.Helper()
	capability, err := generation.Mint(packagebridge.Request{
		RequestID: "request-1", RouteID: "knowledge.article.list", Method: "GET", Deadline: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	return capability
}

func TestRemoteInstancesBindAndShareTheGenerationCapabilities(t *testing.T) {
	fixture := newListenerFixture(t)
	first, _ := fixture.enroll(t, "knowledge")
	second, _ := fixture.enroll(t, "knowledge")
	clientA := packagebridgev1.NewKernelPackageBridgeClient(fixture.dial(t, first))
	clientB := packagebridgev1.NewKernelPackageBridgeClient(fixture.dial(t, second))
	sessionA := bind(t, clientA, "pod-a")
	sessionB := bind(t, clientB, "pod-b")

	heartbeat, err := clientA.Heartbeat(sessionA, &packagebridgev1.HeartbeatRequest{})
	require.NoError(t, err)
	require.False(t, heartbeat.GetFenced())
	require.EqualValues(t, 9, heartbeat.GetRouteGeneration())

	// A capability minted for the generation can be redeemed by any of its
	// instances, exactly once.
	capability := mint(t, fixture.generation)
	require.NoError(t, invoke(sessionB, clientB, capability))
	call := <-fixture.calls
	require.Equal(t, packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.1.0", Generation: 9}, call.Host)
	require.Equal(t, codes.PermissionDenied, status.Code(invoke(sessionA, clientA, capability)), "one-shot")

	bridge := fixture.binder
	require.Len(t, bridge.bindings, 2)
	require.Equal(t, "10.0.0.7:7000", bridge.bindings[0].AdvertiseAddr)
}

func TestModuleListenerRejectsUnauthenticatedAndForeignCallers(t *testing.T) {
	fixture := newListenerFixture(t)
	anonymous := packagebridgev1.NewKernelPackageBridgeClient(fixture.dial(t, nil))
	_, err := anonymous.Bind(context.Background(), &packagebridgev1.BindRequest{PackageId: "knowledge"})
	require.Equal(t, codes.Unauthenticated, status.Code(err))

	ticketCertificate, _ := fixture.enroll(t, "ticket")
	ticket := packagebridgev1.NewKernelPackageBridgeClient(fixture.dial(t, ticketCertificate))
	_, err = ticket.Bind(context.Background(), &packagebridgev1.BindRequest{
		PackageId: "knowledge", PackageVersion: "4.1.0", InstanceId: "pod-x", AdvertiseAddr: "10.0.0.9:7000", LeaseId: "lease",
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "a certificate binds only its own package")
	_, err = ticket.Bind(context.Background(), &packagebridgev1.BindRequest{
		PackageId: "ticket", PackageVersion: "4.1.0", InstanceId: "pod-x", AdvertiseAddr: "10.0.0.9:7000", LeaseId: "lease",
	})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "ticket has no remote installation")

	knowledgeCertificate, _ := fixture.enroll(t, "knowledge")
	knowledge := packagebridgev1.NewKernelPackageBridgeClient(fixture.dial(t, knowledgeCertificate))
	session := bind(t, knowledge, "pod-a")
	// A stolen token is useless under another module's certificate.
	require.Equal(t, codes.Unauthenticated, status.Code(invoke(session, ticket, mint(t, fixture.generation))))
	// Calls without a session are refused.
	require.Equal(t, codes.Unauthenticated, status.Code(invoke(context.Background(), knowledge, mint(t, fixture.generation))))
	_, err = knowledge.Bind(context.Background(), &packagebridgev1.BindRequest{
		PackageId: "knowledge", PackageVersion: "4.0.0", InstanceId: "pod-a", AdvertiseAddr: "10.0.0.7:7000", LeaseId: "lease",
	})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "another version is not the desired one")
}

func TestFencedGenerationsLoseTheirSession(t *testing.T) {
	fixture := newListenerFixture(t)
	certificate, _ := fixture.enroll(t, "knowledge")
	client := packagebridgev1.NewKernelPackageBridgeClient(fixture.dial(t, certificate))
	session := bind(t, client, "pod-a")
	require.NoError(t, invoke(session, client, mint(t, fixture.generation)))
	<-fixture.calls

	pending := mint(t, fixture.generation)
	fixture.generation.SetDraining(true)
	heartbeat, err := client.Heartbeat(session, &packagebridgev1.HeartbeatRequest{})
	require.NoError(t, err)
	require.True(t, heartbeat.GetDraining())

	require.NoError(t, fixture.generation.Close())
	_, err = fixture.generation.Mint(packagebridge.Request{RequestID: "r", RouteID: "knowledge.article.list", Deadline: time.Now().Add(time.Minute)})
	require.ErrorIs(t, err, packagebridge.ErrBridgeClosed)
	require.Equal(t, codes.PermissionDenied, status.Code(invoke(session, client, pending)))
	heartbeat, err = client.Heartbeat(session, &packagebridgev1.HeartbeatRequest{})
	require.NoError(t, err)
	require.True(t, heartbeat.GetFenced(), "the heartbeat reports the fence once")
	_, err = client.Heartbeat(session, &packagebridgev1.HeartbeatRequest{})
	require.Equal(t, codes.Unauthenticated, status.Code(err), "a fenced session is dropped")
}

func TestRevokedCertificatesAreRejectedAtOnce(t *testing.T) {
	fixture := newListenerFixture(t)
	certificate, enrollmentID := fixture.enroll(t, "knowledge")
	client := packagebridgev1.NewKernelPackageBridgeClient(fixture.dial(t, certificate))
	session := bind(t, client, "pod-a")
	_, err := client.Heartbeat(session, &packagebridgev1.HeartbeatRequest{})
	require.NoError(t, err, "the certificate's good state is now cached")

	require.NoError(t, fixture.authority.RevokeEnrollment(context.Background(), enrollmentID))
	_, err = client.Heartbeat(session, &packagebridgev1.HeartbeatRequest{})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Contains(t, status.Convert(err).Message(), "revoked")
}

func TestSessionsExpireWithoutHeartbeats(t *testing.T) {
	fixture := newListenerFixture(t)
	certificate, _ := fixture.enroll(t, "knowledge")
	client := packagebridgev1.NewKernelPackageBridgeClient(fixture.dial(t, certificate))
	session := bind(t, client, "pod-a")
	fixture.clock.Advance(10 * time.Second)
	_, err := client.Heartbeat(session, &packagebridgev1.HeartbeatRequest{})
	require.NoError(t, err)
	fixture.clock.Advance(10 * time.Second)
	require.NoError(t, invoke(session, client, mint(t, fixture.generation)), "a heartbeat 10s ago keeps the 15s session alive")
	<-fixture.calls
	fixture.clock.Advance(16 * time.Second)
	_, err = client.Heartbeat(session, &packagebridgev1.HeartbeatRequest{})
	require.Equal(t, codes.Unauthenticated, status.Code(err))

	// Rebinding the same instance replaces its session.
	fresh := bind(t, client, "pod-a")
	_, err = client.Heartbeat(fresh, &packagebridgev1.HeartbeatRequest{})
	require.NoError(t, err)
}
