package pluginhost

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	identityplatform "github.com/AnixOps/anix-control/v5/packages/identity-platform/control"
	"github.com/stretchr/testify/require"
)

// A built-in host started with InProcess runs on a goroutine of Control:
// no child process, the same package bridge, and Stop ends it.
func TestBuiltinHostRunsInProcess(t *testing.T) {
	calls := make(chan packagebridge.Call, 1)
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil, packagebridge.Operation{
		PackageID: "identity-platform", RouteID: "identity.auth.login", Name: "identity.auth.login",
		Handler: func(_ context.Context, call packagebridge.Call) (packagebridge.Response, error) {
			calls <- call
			return packagebridge.Response{StatusCode: 200, Body: []byte(`{"code":0,"msg":"操作成功","ts":1,"data":{"token":"in-process"}}`)}, nil
		},
	})
	require.NoError(t, err)
	manager, err := NewManager(ManagerConfig{
		RuntimeDir: shortHostTempDir(t), BridgeFactory: packagebridge.NewFactory(allowlist), StartupTimeout: 10 * time.Second,
		InProcess: func(ctx context.Context, packageID, version string) error {
			require.Equal(t, "identity-platform", packageID)
			return identityplatform.Run(ctx, version)
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	ref := writeBuiltinArtifactRef(t, "identity-platform", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	host := manager.hosts[hostKey(ref.PackageID, ref.Version)]
	require.NotNil(t, host)
	require.Nil(t, host.command)

	response, err := manager.Dispatch(context.Background(), DispatchInput{
		PackageID: "identity-platform", Version: "4.0.0", Generation: 7,
		RequestID: "in-process-1", RouteID: "identity.auth.login", Method: "POST",
		Body:          []byte(`{"email":"u@example.test","password":"secret"}`),
		PrincipalJSON: []byte(`{"actor_id":0,"admin":false,"package_id":"identity-platform"}`),
		Deadline:      time.Now().Add(5 * time.Second),
	})
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.JSONEq(t, `{"code":0,"msg":"操作成功","ts":1,"data":{"token":"in-process"}}`, string(response.Body))
	select {
	case call := <-calls:
		require.Equal(t, []byte(`{"email":"u@example.test","password":"secret"}`), call.Request.Body)
	case <-time.After(time.Second):
		t.Fatal("the in-process host did not invoke the package bridge")
	}

	require.NoError(t, manager.Stop(context.Background(), ref.PackageID, ref.Version, 8))
	require.Empty(t, manager.hosts)
	require.True(t, hostExited(host))
}

// An in-process host reads the settings a child would find in its
// environment, its package's entries included, and nothing else of
// Control's environment.
func TestInProcessHostGetsTheChildEnvironment(t *testing.T) {
	t.Setenv("ANIX_IDENTITY_TOKEN_LIFETIME", "1h")
	seen := make(chan map[string]string, 1)
	manager, err := NewManager(ManagerConfig{
		RuntimeDir: shortHostTempDir(t), BridgeFactory: knowledgeBridge(t), StartupTimeout: 10 * time.Second,
		MaxResponseBytes:   2 << 20,
		PackageEnvironment: map[string][]string{"knowledge": {"ANIX_TEST_SECRET=s3cret"}},
		InProcess: func(ctx context.Context, packageID, version string) error {
			return modulesdk.Run(ctx, modulesdk.Options{
				PackageID: packageID, PackageVersion: version,
				Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
					seen <- map[string]string{
						"secret": host.Getenv("ANIX_TEST_SECRET"), "lifetime": host.Getenv("ANIX_IDENTITY_TOKEN_LIFETIME"),
						"version": host.Getenv(BuiltinHostVersionEnvironment), "limit": host.Getenv(hostMaxResponseBytesEnvironment),
					}
					return healthyPackage{leaseID: host.LeaseID}, nil
				},
			})
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	ref := writeBuiltinArtifactRef(t, "knowledge", "4.3.0")
	require.NoError(t, manager.Start(context.Background(), ref, 1))
	require.Equal(t, map[string]string{"secret": "s3cret", "lifetime": "", "version": "4.3.0", "limit": "2097152"}, <-seen)
}

// A panic while an in-process host starts fails that start; Control lives on.
func TestInProcessHostPanicFailsOnlyItsStart(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		RuntimeDir: shortHostTempDir(t), BridgeFactory: knowledgeBridge(t), StartupTimeout: 5 * time.Second,
		InProcess: func(context.Context, string, string) error { panic("broken package") },
	})
	require.NoError(t, err)
	manager.logf = t.Logf
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	err = manager.Start(context.Background(), writeBuiltinArtifactRef(t, "knowledge", "4.3.0"), 1)
	require.ErrorIs(t, err, ErrHostUnavailable)
	require.ErrorContains(t, err, "panic: broken package")
}

func knowledgeBridge(t *testing.T) packagebridge.SessionFactory {
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil, packagebridge.Operation{
		PackageID: "knowledge", RouteID: "knowledge.article.list", Name: "knowledge.article.list",
		Handler: func(context.Context, packagebridge.Call) (packagebridge.Response, error) {
			return packagebridge.Response{StatusCode: 204}, nil
		},
	})
	require.NoError(t, err)
	return packagebridge.NewFactory(allowlist)
}

type healthyPackage struct{ leaseID string }

func (healthyPackage) Dispatch(context.Context, pluginhostsdk.DispatchRequest) (pluginhostsdk.DispatchResponse, error) {
	return pluginhostsdk.DispatchResponse{StatusCode: 204}, nil
}

func (healthyPackage) Migrate(context.Context, pluginhostsdk.MigrationRequest) (pluginhostsdk.MigrationResponse, error) {
	return pluginhostsdk.MigrationResponse{}, nil
}

func (p healthyPackage) Health(context.Context) (pluginhostsdk.HealthResponse, error) {
	return pluginhostsdk.HealthResponse{Healthy: true, LeaseID: p.leaseID}, nil
}

func (healthyPackage) Drain(context.Context) (pluginhostsdk.DrainResponse, error) {
	return pluginhostsdk.DrainResponse{Drained: true}, nil
}
