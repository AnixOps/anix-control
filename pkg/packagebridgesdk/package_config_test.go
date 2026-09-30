package packagebridgesdk_test

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/stretchr/testify/require"
)

type recordingHostOperations struct {
	seen   []packagebridge.HostIdentity
	config packagebridge.PackageConfig
	err    error
}

func (o *recordingHostOperations) PackageConfig(_ context.Context, host packagebridge.HostIdentity) (packagebridge.PackageConfig, error) {
	o.seen = append(o.seen, host)
	return o.config, o.err
}

func dialSessionWithOperations(t *testing.T, operations packagebridge.HostOperations) *packagebridgesdk.Client {
	t.Helper()
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil)
	require.NoError(t, err)
	session, child, err := packagebridge.NewSessionWithOptions(
		packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.0.1", Generation: 7},
		allowlist, packagebridge.SessionOptions{HostOperations: operations},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func TestGetPackageConfigIsAuthorizedByTheSessionIdentity(t *testing.T) {
	operations := &recordingHostOperations{config: packagebridge.PackageConfig{
		Revision: 3, ConfigHash: "abc", RouteModes: map[string]string{"knowledge.article.list": "shadow"},
	}}
	client := dialSessionWithOperations(t, operations)

	config, err := client.GetPackageConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, packagebridgesdk.PackageConfig{Revision: 3, ConfigHash: "abc", RouteModes: map[string]string{"knowledge.article.list": "shadow"}}, config)
	require.Equal(t, []packagebridge.HostIdentity{{PackageID: "knowledge", Version: "4.0.1", Generation: 7}}, operations.seen,
		"the kernel sees the identity bound at session creation, not anything the host sent")
}

func TestGetPackageConfigReportsFencedHosts(t *testing.T) {
	client := dialSessionWithOperations(t, &recordingHostOperations{err: packagebridge.ErrHostFenced})
	_, err := client.GetPackageConfig(context.Background())
	require.ErrorIs(t, err, packagebridgesdk.ErrHostFenced)
}

func TestGetPackageConfigIsUnsupportedWithoutKernelOperations(t *testing.T) {
	client := dialSessionWithOperations(t, nil)
	_, err := client.GetPackageConfig(context.Background())
	require.ErrorIs(t, err, packagebridgesdk.ErrSessionOperationUnsupported)
}
