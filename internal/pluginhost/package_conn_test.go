//go:build unix

package pluginhost

import (
	"context"
	"errors"
	"testing"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

// servicePackage is testPackage plus a service beside the host protocol.
type servicePackage struct{ testPackage }

func (servicePackage) RegisterServices(registrar grpc.ServiceRegistrar) {
	healthv1.RegisterHealthServer(registrar, health.NewServer())
}

// PackageConn reaches the services the running package registered.
func TestPackageConnReachesThePackageServices(t *testing.T) {
	manager := newHostTestManager(t, ManagerConfig{InProcess: func(ctx context.Context, packageID, version string) error {
		return modulesdk.Run(ctx, modulesdk.Options{
			PackageID: packageID, PackageVersion: version,
			Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
				return servicePackage{testPackage{leaseID: host.LeaseID}}, nil
			},
		})
	}})
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	require.NoError(t, manager.Start(context.Background(), writeVerifiedArtifactRef(t, "knowledge", "4.0.0"), 7))

	conn, err := manager.PackageConn("knowledge")
	require.NoError(t, err)
	response, err := healthv1.NewHealthClient(conn).Check(context.Background(), &healthv1.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, healthv1.HealthCheckResponse_SERVING, response.GetStatus())

	_, err = manager.PackageConn("ticket")
	require.True(t, errors.Is(err, ErrHostUnavailable), err)
}
