package main

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	pluginhostv1 "github.com/AnixOps/anix-control/sdk/api/pluginhost/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/notification/native"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type bridgeStub struct{ operation string }

// settingsStub stands for the kernel's KernelSettings.
type settingsStub struct{}

func (settingsStub) GetSettings(context.Context, *kernelsettingsv1.GetSettingsRequest, ...grpc.CallOption) (*kernelsettingsv1.GetSettingsResponse, error) {
	return &kernelsettingsv1.GetSettingsResponse{}, nil
}

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
func TestNotificationHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newNotificationService(bridge, "lease-1")
	require.NoError(t, err)
	for _, route := range []string{"notification.user.notifications.get", "notification.telegram.webhook.post"} {
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
	require.Len(t, handlers, len(notificationRoutes))
	withoutSettings := (&native.Service{}).Handlers()
	for _, route := range []string{native.EmailConfigGetRouteID, native.EmailConfigPutRouteID, native.TestSendRouteID} {
		require.NotContains(t, withoutSettings, route, "without KernelSettings the e-mail routes stay legacy")
	}
	for route := range notificationRoutes {
		require.Contains(t, handlers, route, "every native route has a handler")
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route, "a bridged route has no native handler")
		require.NotContains(t, notificationRoutes, route)
	}
}

// The host accepts exactly the package's declared compatibility routes.
func TestNotificationHostRoutesAreThePackageRoutes(t *testing.T) {
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
	for route := range notificationRoutes {
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
func TestNotificationManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{
		"kernel.storage.v1", "kernel.storage.adopt:v2_notification_template", "kernel.storage.adopt:v2_notification_log",
		"kernel.storage.adopt:v2_telegram_bot", "kernel.storage.adopt:v2_telegram_user", "kernel.view:kapi_user_directory_v1",
		// The e-mail configuration holds the SMTP password, which the GET
		// answers and the test send uses.
		"kernel.settings.mail.read.v1", "kernel.settings.mail.write.v1", "kernel.settings.mail.secrets.v1",
	}, manifest.Capabilities)
}

// modesBridge is bridgeStub with route modes set by the kernel.
type modesBridge struct {
	bridgeStub
	modes map[string]string
}

func (b *modesBridge) GetPackageConfig(context.Context) (packagebridgesdk.PackageConfig, error) {
	return packagebridgesdk.PackageConfig{Revision: 1, RouteModes: b.modes}, nil
}

// Setting the webhook natively needs the request's scheme and host, which a
// kernel sends as DispatchRequest fields; a request from a kernel that does
// not send them is answered by the legacy handler.
func TestNotificationHostSetsTheWebhookNativelyOnlyWithTheRequestAddress(t *testing.T) {
	bridge := &modesBridge{modes: map[string]string{native.SetWebhookRouteID: pluginhostsdk.RouteModeNative}}
	router, err := newNotificationService(bridge, "lease-1")
	require.NoError(t, err)
	router.Refresh(context.Background())
	server, err := pluginhostsdk.NewServer(pluginhostsdk.ServerConfig{PackageID: "notification", PackageVersion: "4.1.0"}, router)
	require.NoError(t, err)
	dispatch := func(scheme, host string) {
		t.Helper()
		bridge.operation = ""
		response, err := server.Dispatch(context.Background(), &pluginhostv1.DispatchRequest{
			PackageId: "notification", PackageVersion: "4.1.0", RouteGeneration: 1, RequestId: "request-1",
			RouteId: native.SetWebhookRouteID, Method: "POST", PrincipalJson: []byte(`{"actor_id":1,"admin":true}`),
			RequestMetadataJson: []byte(`{"path":"/api/v2/admin/telegram/webhook"}`), BridgeCapability: make([]byte, 32),
			DeadlineUnixMillis: time.Now().Add(5 * time.Second).UnixMilli(), RequestScheme: scheme, RequestHost: host,
		})
		require.NoError(t, err)
		require.EqualValues(t, 200, response.GetStatusCode())
	}

	dispatch("", "")
	require.Equal(t, native.SetWebhookRouteID, bridge.operation, "without the request address the legacy handler answers")
	dispatch("https", "")
	require.Equal(t, native.SetWebhookRouteID, bridge.operation, "without the request host the legacy handler answers")
	dispatch("https", "panel.example.test:8443")
	require.Empty(t, bridge.operation, "with the request address the package answers")
}
