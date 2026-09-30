package moduleruntime

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	packagebridgev1 "github.com/AnixOps/anix-control/v4/api/packagebridge/v1"
	pluginhostv1 "github.com/AnixOps/anix-control/v4/api/pluginhost/v1"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/pkg/moduletls"
	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/AnixOps/anix-control/v4/pkg/pluginhostsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

// networkBridge is a minimal module-side bridge client over the module
// listener: it binds, heartbeats, and rebinds when its generation is fenced.
type networkBridge struct {
	client   packagebridgev1.KernelPackageBridgeClient
	instance string
	address  string
	lease    string

	mu    sync.Mutex
	token string
}

func (b *networkBridge) bind(ctx context.Context) error {
	response, err := b.client.Bind(ctx, &packagebridgev1.BindRequest{
		PackageId: "knowledge", PackageVersion: "4.1.0", InstanceId: b.instance,
		AdvertiseAddr: b.address, LeaseId: b.lease,
	})
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.token = base64.RawURLEncoding.EncodeToString(response.GetSessionToken())
	b.mu.Unlock()
	return nil
}

func (b *networkBridge) context(ctx context.Context) context.Context {
	b.mu.Lock()
	defer b.mu.Unlock()
	return metadata.AppendToOutgoingContext(ctx, packagebridge.SessionMetadataKey, b.token)
}

// run keeps the session alive and rebinds after fences until ctx ends.
func (b *networkBridge) run(ctx context.Context) {
	for ctx.Err() == nil {
		heartbeat, err := b.client.Heartbeat(b.context(ctx), &packagebridgev1.HeartbeatRequest{})
		if err != nil || heartbeat.GetFenced() {
			_ = b.bind(ctx)
		}
		select {
		case <-ctx.Done():
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (b *networkBridge) Invoke(ctx context.Context, capability []byte, operation string, payload []byte) (packagebridgesdk.Response, error) {
	response, err := b.client.Invoke(b.context(ctx), &packagebridgev1.InvokeRequest{Capability: capability, Operation: operation, Payload: payload})
	if err != nil {
		return packagebridgesdk.Response{}, err
	}
	return packagebridgesdk.Response{StatusCode: response.GetStatusCode(), Body: response.GetResponseBody()}, nil
}

func (b *networkBridge) GetPackageConfig(context.Context) (packagebridgesdk.PackageConfig, error) {
	return packagebridgesdk.PackageConfig{}, packagebridgesdk.ErrSessionOperationUnsupported
}

// startModuleInstance runs a knowledge module instance: an SDK host served
// over mTLS with its module certificate, bound to the kernel.
func startModuleInstance(t *testing.T, fixture *listenerFixture, instance string) *networkBridge {
	t.Helper()
	certificate, _ := fixture.enroll(t, "knowledge")
	roots := fixture.kernel.Roots
	source := moduletls.Source{Certificate: func() (*tls.Certificate, error) { return certificate, nil }, Roots: roots}
	kernelID, err := moduletls.Kernel("prod")
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	connection, err := grpc.NewClient(fixture.address, grpc.WithTransportCredentials(credentials.NewTLS(source.ClientConfig(moduletls.AcceptExactly(kernelID)))))
	require.NoError(t, err)
	bridge := &networkBridge{
		client: packagebridgev1.NewKernelPackageBridgeClient(connection), instance: instance,
		address: listener.Addr().String(), lease: "lease-" + instance,
	}
	router, err := pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{PackageID: "knowledge", LeaseID: bridge.lease, Bridge: bridge})
	require.NoError(t, err)
	host, err := pluginhostsdk.NewServer(pluginhostsdk.ServerConfig{PackageID: "knowledge", PackageVersion: "4.1.0"}, router)
	require.NoError(t, err)
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(source.ServerConfig(moduletls.AcceptExactly(kernelID), false))))
	pluginhostv1.RegisterControlPackageHostServer(server, host)
	go func() { _ = server.Serve(listener) }()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		bridge.run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		server.Stop()
		_ = connection.Close()
	})
	return bridge
}

func writeArtifactRef(t *testing.T) pluginhost.ArtifactRef {
	t.Helper()
	directory := t.TempDir()
	write := func(name string, content []byte) (string, string) {
		path := filepath.Join(directory, name)
		require.NoError(t, os.WriteFile(path, content, 0o600))
		digest := sha256.Sum256(content)
		return path, hex.EncodeToString(digest[:])
	}
	artifactPath, artifactDigest := write("package.anxp", []byte("knowledge artifact"))
	entrypointPath, entrypointDigest := write("control-host", []byte("#!/bin/false\n"))
	require.NoError(t, os.Chmod(entrypointPath, 0o700)) // #nosec G302 -- test entrypoint must be executable.
	manifestPath, manifestDigest := write("manifest.json", []byte(`{"id":"knowledge","version":"4.1.0","targets":["control"]}`))
	return pluginhost.ArtifactRef{
		PackageID: "knowledge", Version: "4.1.0",
		ArtifactPath: artifactPath, ArtifactSHA256: artifactDigest,
		EntrypointPath: entrypointPath, EntrypointSHA256: entrypointDigest,
		ManifestPath: manifestPath, ManifestSHA256: manifestDigest,
	}
}

type remoteFixture struct {
	*listenerFixture
	supervisor *pluginhost.Supervisor
}

// newRemoteFixture wires a Supervisor with the remote runtime to the module
// listener, as the server does.
func newRemoteFixture(t *testing.T) *remoteFixture {
	t.Helper()
	calls := make(chan packagebridge.Call, 16)
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil, packagebridge.Operation{
		PackageID: "knowledge", RouteID: "knowledge.article.list", Name: "knowledge.article.list",
		Handler: func(_ context.Context, call packagebridge.Call) (packagebridge.Response, error) {
			calls <- call
			return packagebridge.Response{StatusCode: 200, Body: []byte(`{"code":0,"data":[]}`)}, nil
		},
	})
	require.NoError(t, err)
	parent, err := filepath.Abs(".")
	require.NoError(t, err)
	runtimeDir, err := os.MkdirTemp(parent, ".anix-host-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	supervisor, err := pluginhost.NewManager(pluginhost.ManagerConfig{RuntimeDir: runtimeDir, BridgeFactory: packagebridge.NewFactory(allowlist)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = supervisor.Shutdown(context.Background()) })

	fixture := newListenerFixtureWithBinder(t, supervisor, calls)
	require.NoError(t, supervisor.EnableRemote(pluginhost.RemoteConfig{
		Instances: fixture.bridge, TLS: fixture.kernel, Cluster: "prod", BindTimeout: 10 * time.Second,
		IsRemote: func(_ context.Context, packageID string) (bool, error) { return packageID == "knowledge", nil },
	}))
	return &remoteFixture{listenerFixture: fixture, supervisor: supervisor}
}

func dispatchList(ctx context.Context, supervisor *pluginhost.Supervisor, generation uint64) (pluginhost.DispatchOutput, error) {
	return supervisor.Dispatch(ctx, pluginhost.DispatchInput{
		PackageID: "knowledge", Version: "4.1.0", Generation: generation, RequestID: "request-1",
		RouteID: "knowledge.article.list", Method: "GET", PrincipalJSON: []byte(`{"actor_id":7,"admin":false,"package_id":"knowledge"}`),
		Deadline: time.Now().Add(5 * time.Second),
	})
}

func TestRemoteRuntimeServesAndMovesGenerations(t *testing.T) {
	fixture := newRemoteFixture(t)
	ctx := context.Background()
	ref := writeArtifactRef(t)
	startModuleInstance(t, fixture.listenerFixture, "pod-a")
	startModuleInstance(t, fixture.listenerFixture, "pod-b")

	require.NoError(t, fixture.supervisor.Start(ctx, ref, 5), "Start waits until an instance is bound and healthy")
	for range 4 {
		response, err := dispatchList(ctx, fixture.supervisor, 5)
		require.NoError(t, err)
		require.EqualValues(t, 200, response.StatusCode)
		call := <-fixture.calls
		require.Equal(t, packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.1.0", Generation: 5, Remote: true}, call.Host)
	}
	require.Eventually(t, func() bool { return len(fixture.bridge.Instances("knowledge", 5)) == 2 }, 5*time.Second, 20*time.Millisecond)
	health, err := fixture.supervisor.Health(ctx, "knowledge", "4.1.0", 5)
	require.NoError(t, err)
	require.True(t, health.Healthy)

	// A new generation of the same version fences the old one; its
	// instances bind again and serve the new generation.
	require.NoError(t, fixture.supervisor.Start(ctx, ref, 6))
	_, err = dispatchList(ctx, fixture.supervisor, 5)
	require.ErrorIs(t, err, pluginhost.ErrGenerationUnavailable)
	response, err := dispatchList(ctx, fixture.supervisor, 6)
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	<-fixture.calls

	// Stopping fences the generation for good.
	require.NoError(t, fixture.supervisor.Stop(ctx, "knowledge", "4.1.0", 7))
	_, err = dispatchList(ctx, fixture.supervisor, 6)
	require.Error(t, err)
}

func TestRemoteStartFailsWithoutInstances(t *testing.T) {
	fixture := newRemoteFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err := fixture.supervisor.Start(ctx, writeArtifactRef(t), 5)
	require.ErrorIs(t, err, pluginhost.ErrHostUnavailable)
	_, err = dispatchList(context.Background(), fixture.supervisor, 5)
	require.True(t, errors.Is(err, pluginhost.ErrHostUnavailable) || errors.Is(err, pluginhost.ErrHostNotFound), err)
}

// newListenerFixtureWithBinder is newListenerFixture with a real binder.
func newListenerFixtureWithBinder(t *testing.T, binder packagebridge.Binder, calls chan packagebridge.Call) *listenerFixture {
	t.Helper()
	base := newPKIFixture(t)
	bridge, err := packagebridge.NewModuleBridge(binder, packagebridge.ModuleBridgeOptions{
		Cluster: "prod", HeartbeatInterval: 100 * time.Millisecond, SessionTTL: time.Second,
	})
	require.NoError(t, err)
	base.bridge = bridge
	base.calls = calls
	base.serve(t)
	return base
}
