package control

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
	"github.com/AnixOps/anix-control/v5/packages/notification/native"
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

// The native routes run natively from the start and never reach the
// kernel's legacy path; the bridged webhook relays to the kernel; routes
// outside the package are refused.
func TestNotificationHostServesItsNativeRoutesNatively(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newNotificationService(bridge, "lease-1")
	require.NoError(t, err)
	_, effective := service.Mode("notification.user.notifications.get")
	require.Equal(t, pluginhostsdk.RouteModeNative, effective)
	_, _ = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "notification.user.notifications.get", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Empty(t, bridge.operation, "a native route is not relayed")
	response, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "notification.telegram.webhook.post", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.Equal(t, "notification.telegram.webhook.post", bridge.operation)

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "ticket.user.ticket.get", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{Settings: settingsStub{}}).Handlers()
	require.Len(t, handlers, len(notificationRoutes))
	withoutSettings := (&native.Service{}).Handlers()
	for _, route := range []string{native.EmailConfigGetRouteID, native.EmailConfigPutRouteID, native.TestSendRouteID} {
		require.NotContains(t, withoutSettings, route, "without KernelSettings the e-mail routes have no handler")
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

// Setting the webhook without a url needs the request's scheme and host,
// which the kernel sends as DispatchRequest fields; without them the route
// fails, and is never relayed.
func TestNotificationHostSetsTheWebhookOnlyWithTheRequestAddress(t *testing.T) {
	bridge := &bridgeStub{}
	router, err := newNotificationService(bridge, "lease-1")
	require.NoError(t, err)
	server, err := pluginhostsdk.NewServer(pluginhostsdk.ServerConfig{PackageID: "notification", PackageVersion: "4.1.0"}, router)
	require.NoError(t, err)
	dispatch := func(scheme, host string) error {
		_, err := server.Dispatch(context.Background(), &pluginhostv1.DispatchRequest{
			PackageId: "notification", PackageVersion: "4.1.0", RouteGeneration: 1, RequestId: "request-1",
			RouteId: native.SetWebhookRouteID, Method: "POST", PrincipalJson: []byte(`{"actor_id":1,"admin":true}`),
			RequestMetadataJson: []byte(`{"path":"/api/v2/admin/telegram/webhook"}`), BridgeCapability: make([]byte, 32),
			DeadlineUnixMillis: time.Now().Add(5 * time.Second).UnixMilli(), RequestScheme: scheme, RequestHost: host,
		})
		return err
	}

	require.Error(t, dispatch("", ""))
	require.Error(t, dispatch("https", ""))
	require.NoError(t, dispatch("https", "panel.example.test:8443"))
	require.Empty(t, bridge.operation, "the route is never relayed")
}
