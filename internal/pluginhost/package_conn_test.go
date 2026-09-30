//go:build unix

package pluginhost

import (
	"context"
	"errors"
	"testing"

	pluginhostv1 "github.com/AnixOps/anix-control/sdk/api/pluginhost/v1"
	"github.com/stretchr/testify/require"
)

// PackageConn reaches the running host for the contracts it serves beside
// the host protocol.
func TestPackageConnReachesTheRunningHost(t *testing.T) {
	manager, _ := newSupervisedTestManager(t)
	require.NoError(t, manager.Start(context.Background(), writeHostArtifactRef(t, "knowledge", "4.0.0"), 7))

	conn, err := manager.PackageConn("knowledge")
	require.NoError(t, err)
	health, err := pluginhostv1.NewControlPackageHostClient(conn).Health(context.Background(), &pluginhostv1.HealthRequest{})
	require.NoError(t, err)
	require.True(t, health.GetHealthy())

	_, err = manager.PackageConn("ticket")
	require.True(t, errors.Is(err, ErrHostUnavailable), err)
}
