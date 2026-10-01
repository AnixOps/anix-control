package main

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/platform/native"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type bridgeStub struct{ operation string }

// settingsStub stands for the kernel's KernelSettings.
type settingsStub struct{}

func (settingsStub) PutSettings(context.Context, *kernelsettingsv1.PutSettingsRequest, ...grpc.CallOption) (*kernelsettingsv1.PutSettingsResponse, error) {
	return &kernelsettingsv1.PutSettingsResponse{Applied: true}, nil
}

func (s *bridgeStub) LeaseStorage(context.Context) (packagebridgesdk.StorageLease, error) {
	return packagebridgesdk.StorageLease{}, packagebridgesdk.ErrSessionOperationUnsupported
}

func (s *bridgeStub) GetPackageConfig(context.Context) (packagebridgesdk.PackageConfig, error) {
	return packagebridgesdk.PackageConfig{}, packagebridgesdk.ErrSessionOperationUnsupported
}

func (s *bridgeStub) Invoke(_ context.Context, _ []byte, operation string, _ []byte) (packagebridgesdk.Response, error) {
	s.operation = operation
	return packagebridgesdk.Response{StatusCode: 200, Body: []byte(`{"code":0,"msg":"操作成功","ts":1,"data":null}`)}, nil
}

// Until the kernel sets a route's mode, the host relays it to the legacy
// handler, bridged routes always; routes outside the package are refused.
func TestPlatformHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newPlatformService(bridge, "lease-1")
	require.NoError(t, err)
	for _, route := range []string{"platform.admin.system.backups.get", "platform.admin.system.backup.post"} {
		response, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
			RouteID: route, BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
		})
		require.NoError(t, err)
		require.EqualValues(t, 200, response.StatusCode)
		require.Equal(t, route, bridge.operation)
	}

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "ticket.user.ticket.get", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{Settings: settingsStub{}}).Handlers()
	require.Len(t, handlers, len(platformRoutes))
	require.NotContains(t, (&native.Service{}).Handlers(), native.BackupConfigUpdateRouteID,
		"without KernelSettings the backup configuration update stays legacy")
	for route := range platformRoutes {
		require.Contains(t, handlers, route, "every native route has a handler")
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route, "a bridged route has no native handler")
		require.NotContains(t, platformRoutes, route)
	}
}

// The host accepts exactly the package's declared compatibility routes.
func TestPlatformHostRoutesAreThePackageRoutes(t *testing.T) {
	raw, err := os.ReadFile("../compat/v2-routes.json")
	require.NoError(t, err)
	var declared struct {
		Routes []struct {
			PackageRoute string `json:"package_route"`
		} `json:"routes"`
	}
	require.NoError(t, json.Unmarshal(raw, &declared))
	var want, got []string
	for _, route := range declared.Routes {
		want = append(want, route.PackageRoute)
	}
	for route := range platformRoutes {
		got = append(got, route)
	}
	for route := range bridgedRoutes {
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
}

// The release declares exactly these kernel grants.
func TestPlatformManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{
		"kernel.storage.v1", "kernel.storage.adopt:v2_backup_config", "kernel.storage.adopt:v2_backup_record",
		"kernel.view:kapi_system_audit_log_v1",
		// Only the backup namespace: the generic system configuration
		// routes stay bridged (docs/architecture/settings-service.md).
		"kernel.settings.backup.write.v1",
	}, manifest.Capabilities)
}
