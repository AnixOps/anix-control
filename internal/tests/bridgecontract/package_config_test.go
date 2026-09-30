package bridgecontract

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/stretchr/testify/require"
)

type recordingHostOperations struct {
	seen   []packagebridge.HostIdentity
	config packagebridge.PackageConfig
	lease  packagebridge.StorageLease
	err    error
}

func (o *recordingHostOperations) LeaseStorage(_ context.Context, host packagebridge.HostIdentity) (packagebridge.StorageLease, error) {
	o.seen = append(o.seen, host)
	return o.lease, o.err
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

func TestLeaseStorageIsAuthorizedByTheSessionIdentity(t *testing.T) {
	operations := &recordingHostOperations{lease: packagebridge.StorageLease{
		Driver: "postgres", DSN: "host='db' user='anix_pkg_knowledge' password='p'", Schema: "pkg_knowledge",
		LeaseGeneration: 4, AdoptedTables: []string{"v2_knowledge"}, Views: []string{"kapi_user_directory_v1"},
	}}
	client := dialSessionWithOperations(t, operations)

	lease, err := client.LeaseStorage(context.Background())
	require.NoError(t, err)
	require.Equal(t, packagebridgesdk.StorageLease{
		Driver: "postgres", DSN: "host='db' user='anix_pkg_knowledge' password='p'", Schema: "pkg_knowledge",
		LeaseGeneration: 4, AdoptedTables: []string{"v2_knowledge"}, Views: []string{"kapi_user_directory_v1"},
	}, lease)
	require.Equal(t, []packagebridge.HostIdentity{{PackageID: "knowledge", Version: "4.0.1", Generation: 7}}, operations.seen)
}

func TestLeaseStorageReportsKernelRefusals(t *testing.T) {
	client := dialSessionWithOperations(t, &recordingHostOperations{err: packagebridge.ErrHostFenced})
	_, err := client.LeaseStorage(context.Background())
	require.ErrorIs(t, err, packagebridgesdk.ErrHostFenced)

	client = dialSessionWithOperations(t, &recordingHostOperations{err: fmt.Errorf("%w: package does not declare kernel.storage.v1", packagebridge.ErrStorageUnavailable)})
	_, err = client.LeaseStorage(context.Background())
	require.ErrorIs(t, err, packagebridgesdk.ErrStorageUnavailable)
	require.ErrorContains(t, err, "kernel.storage.v1")

	client = dialSessionWithOperations(t, &recordingHostOperations{err: errors.New("connection refused to db.internal")})
	_, err = client.LeaseStorage(context.Background())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "db.internal", "internal failures are not passed to the host")

	client = dialSessionWithOperations(t, nil)
	_, err = client.LeaseStorage(context.Background())
	require.ErrorIs(t, err, packagebridgesdk.ErrSessionOperationUnsupported)
}
