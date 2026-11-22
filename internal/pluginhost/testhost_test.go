//go:build unix

package pluginhost

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"github.com/stretchr/testify/require"
)

// serveTestPackage is the InProcess of the tests' managers: it serves
// testPackage for the release through modulesdk, as a linked host does.
func serveTestPackage(ctx context.Context, packageID, version string) error {
	return modulesdk.Run(ctx, modulesdk.Options{
		PackageID: packageID, PackageVersion: version,
		Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
			return testPackage{leaseID: host.LeaseID}, nil
		},
	})
}

// crashTestHost ends a running host without the manager asking, as a
// failing host would.
func crashTestHost(t *testing.T, host *hostProcess) {
	t.Helper()
	require.NotNil(t, host)
	host.cancel()
}

// newHostTestManager returns a manager that runs serveTestPackage, with an
// empty package bridge; config may set anything else.
func newHostTestManager(t *testing.T, config ManagerConfig) *Supervisor {
	t.Helper()
	if config.RuntimeDir == "" {
		config.RuntimeDir = shortHostTempDir(t)
	}
	if config.BridgeFactory == nil {
		allowlist, err := packagebridge.NewAllowlistWithFallback(nil)
		require.NoError(t, err)
		config.BridgeFactory = packagebridge.NewFactory(allowlist)
	}
	if config.InProcess == nil {
		config.InProcess = serveTestPackage
	}
	manager, err := NewManager(config)
	require.NoError(t, err)
	return manager
}

// testPackage answers every dispatch with 202 and is always healthy.
type testPackage struct{ leaseID string }

func (testPackage) Dispatch(context.Context, pluginhostsdk.DispatchRequest) (pluginhostsdk.DispatchResponse, error) {
	return pluginhostsdk.DispatchResponse{StatusCode: 202, ResponseBody: []byte(`{"transport":"unix"}`)}, nil
}

func (testPackage) Migrate(context.Context, pluginhostsdk.MigrationRequest) (pluginhostsdk.MigrationResponse, error) {
	return pluginhostsdk.MigrationResponse{}, nil
}

func (p testPackage) Health(context.Context) (pluginhostsdk.HealthResponse, error) {
	return pluginhostsdk.HealthResponse{Healthy: true, LeaseID: p.leaseID}, nil
}

func (testPackage) Drain(context.Context) (pluginhostsdk.DrainResponse, error) {
	return pluginhostsdk.DrainResponse{Drained: true}, nil
}
