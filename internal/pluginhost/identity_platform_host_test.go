package pluginhost

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	identityplatform "github.com/AnixOps/anix-control/v5/packages/identity-platform/control"

	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"github.com/stretchr/testify/require"
)

// The real identity-platform host, run as Control runs it, reaches the
// kernel only through the capability bridge, and Stop ends it.
func TestIdentityPlatformHostUsesTheCapabilityBridge(t *testing.T) {
	calls := make(chan packagebridge.Call, 1)
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil, packagebridge.Operation{
		PackageID: "identity-platform", RouteID: "identity.auth.login", Name: "identity.auth.login",
		Handler: func(_ context.Context, call packagebridge.Call) (packagebridge.Response, error) {
			calls <- call
			return packagebridge.Response{StatusCode: 200, Body: []byte(`{"code":0,"msg":"操作成功","ts":1,"data":{"token":"identity-host"}}`)}, nil
		},
	})
	require.NoError(t, err)
	manager := newHostTestManager(t, ManagerConfig{
		BridgeFactory: packagebridge.NewFactory(allowlist), StartupTimeout: 10 * time.Second, InProcess: serveIdentityPlatform,
	})
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	ref := writeVerifiedArtifactRef(t, "identity-platform", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))

	response, err := manager.Dispatch(context.Background(), DispatchInput{
		PackageID: "identity-platform", Version: "4.0.0", Generation: 7,
		RequestID: "identity-compiled-1", RouteID: "identity.auth.login", Method: "POST",
		Body:          []byte(`{"email":"u@example.test","password":"secret"}`),
		PrincipalJSON: []byte(`{"actor_id":0,"admin":false,"package_id":"identity-platform"}`),
		Metadata:      RequestMetadata{Path: "/api/v2/login", ClientIP: "198.51.100.42", UserAgent: "AnixOps-Test/1.0"},
		Deadline:      time.Now().Add(5 * time.Second),
	})
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.JSONEq(t, `{"code":0,"msg":"操作成功","ts":1,"data":{"token":"identity-host"}}`, string(response.Body))
	select {
	case call := <-calls:
		require.Equal(t, []byte(`{"email":"u@example.test","password":"secret"}`), call.Request.Body)
		var metadata RequestMetadata
		require.NoError(t, json.Unmarshal(call.Request.MetadataJSON, &metadata))
		require.Equal(t, "198.51.100.42", metadata.ClientIP)
		require.Equal(t, "AnixOps-Test/1.0", metadata.UserAgent)
	case <-time.After(time.Second):
		t.Fatal("identity host did not invoke the package bridge")
	}

	host := manager.hosts[hostKey(ref.PackageID, ref.Version)]
	require.NoError(t, manager.Stop(context.Background(), ref.PackageID, ref.Version, 8))
	require.Empty(t, manager.hosts)
	require.True(t, hostExited(host))
}

func serveIdentityPlatform(ctx context.Context, packageID, version string) error {
	if packageID != identityplatform.PackageID {
		return ErrHostIncompatible
	}
	return identityplatform.Run(ctx, version)
}
