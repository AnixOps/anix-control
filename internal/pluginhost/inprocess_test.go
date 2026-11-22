//go:build unix

package pluginhost

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/stretchr/testify/require"
)

// A host reads the settings the kernel gives it, its own package's entries
// included, and nothing of Control's environment or another package's
// entries.
func TestHostGetsOnlyItsSettings(t *testing.T) {
	t.Setenv("ANIX_IDENTITY_TOKEN_LIFETIME", "1h")
	seen := make(chan map[string]string, 2)
	manager := newHostTestManager(t, ManagerConfig{
		StartupTimeout:     10 * time.Second,
		MaxResponseBytes:   2 << 20,
		PackageEnvironment: map[string][]string{"knowledge": {"ANIX_TEST_SECRET=s3cret"}},
		InProcess: func(ctx context.Context, packageID, version string) error {
			return modulesdk.Run(ctx, modulesdk.Options{
				PackageID: packageID, PackageVersion: version,
				Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
					seen <- map[string]string{
						"secret": host.Getenv("ANIX_TEST_SECRET"), "lifetime": host.Getenv("ANIX_IDENTITY_TOKEN_LIFETIME"),
					}
					return testPackage{leaseID: host.LeaseID}, nil
				},
			})
		},
	})
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	require.NoError(t, manager.Start(context.Background(), writeVerifiedArtifactRef(t, "knowledge", "4.3.0"), 1))
	require.Equal(t, map[string]string{"secret": "s3cret", "lifetime": ""}, <-seen)
	require.NoError(t, manager.Start(context.Background(), writeVerifiedArtifactRef(t, "ticket", "4.0.0"), 1))
	require.Equal(t, map[string]string{"secret": "", "lifetime": ""}, <-seen)
}

// A panic while a host starts fails that start; Control lives on.
func TestHostPanicFailsOnlyItsStart(t *testing.T) {
	manager := newHostTestManager(t, ManagerConfig{
		StartupTimeout: 5 * time.Second,
		InProcess:      func(context.Context, string, string) error { panic("broken package") },
	})
	manager.logf = t.Logf
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	err := manager.Start(context.Background(), writeVerifiedArtifactRef(t, "knowledge", "4.3.0"), 1)
	require.ErrorIs(t, err, ErrHostUnavailable)
	require.ErrorContains(t, err, "panic: broken package")
}

// No host starts without InProcess: nothing else can run one.
func TestHostNeedsALinkedHost(t *testing.T) {
	manager, err := NewManager(ManagerConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	err = manager.Start(context.Background(), writeVerifiedArtifactRef(t, "knowledge", "4.3.0"), 1)
	require.ErrorIs(t, err, ErrHostUnavailable)
	require.Empty(t, manager.hosts)
}
