//go:build unix

package pluginhost

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/shadowsamples"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type shadowBridge struct{}

func (shadowBridge) Invoke(context.Context, []byte, string, []byte) (packagebridgesdk.Response, error) {
	body := `{"code":0,"data":{"email":"alice@example.com","subscribe_url":"https://sub.example.com/s/legacy-secret","uuid":"7b3f6c1e-2d4a-4b8f-9c0d-1e2f3a4b5c6d","last_ip":"203.0.113.7","traffic":10},"msg":"ok","ts":1}`
	return packagebridgesdk.Response{StatusCode: 200, Body: []byte(body)}, nil
}

func (shadowBridge) GetPackageConfig(context.Context) (packagebridgesdk.PackageConfig, error) {
	return packagebridgesdk.PackageConfig{Revision: 3, RouteModes: map[string]string{"subscription.user.info.get": pluginhostsdk.RouteModeShadow}}, nil
}

// A shadow mismatch inside a package host reaches the kernel through the
// host's Health details and is stored sanitized.
func TestShadowMismatchInAHostIsStoredSanitized(t *testing.T) {
	router, err := pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "subscription", LeaseID: "lease-1", Bridge: shadowBridge{},
		Native: map[string]pluginhostsdk.NativeHandler{"subscription.user.info.get": func(context.Context, pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
			return pluginhostsdk.PanelJSON(v2compat.Panel{Code: 0, Msg: "ok", Data: map[string]any{
				"email": "alice@example.com", "subscribe_url": "https://sub.example.com/s/native-secret",
				"uuid": "8b3f6c1e-2d4a-4b8f-9c0d-1e2f3a4b5c6d", "last_ip": "203.0.113.8", "traffic": 11,
			}})
		}},
	})
	require.NoError(t, err)
	router.Refresh(context.Background())
	host, err := pluginhostsdk.NewServer(pluginhostsdk.ServerConfig{PackageID: "subscription", PackageVersion: "4.1.0"}, router)
	require.NoError(t, err)
	socketPath := startTestHostServerWithService(t, host)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := dialHostClient(ctx, socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	_, err = client.Dispatch(ctx, DispatchInput{
		PackageID: "subscription", Version: "4.1.0", Generation: 7, RequestID: "req-shadow-1",
		RouteID: "subscription.user.info.get", Method: "GET", PrincipalJSON: []byte(`{"actor_id":7}`),
		Body:             []byte(`{"password":"hunter2"}`),
		Metadata:         RequestMetadata{Path: "/api/v2/user/info", Query: map[string][]string{"token": {"query-secret"}}},
		BridgeCapability: make([]byte, 32), Deadline: time.Now().Add(5 * time.Second),
	})
	require.NoError(t, err)

	var details string
	require.Eventually(t, func() bool {
		health, err := client.Health(ctx, 7)
		if err != nil {
			return false
		}
		details = health.DetailsJSON
		var document struct {
			Samples []json.RawMessage `json:"shadow_samples"`
		}
		return json.Unmarshal([]byte(details), &document) == nil && len(document.Samples) == 1
	}, 5*time.Second, 10*time.Millisecond)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.ShadowMismatchSample{}))
	collector := &shadowsamples.Collector{DB: db, Route: func(id string) (shadowsamples.RouteInfo, bool) {
		return shadowsamples.RouteInfo{PackageID: "subscription", Path: "/api/v2/user/info"}, id == "subscription.user.info.get"
	}}
	stored, err := collector.Collect(ctx, []shadowsamples.Report{{PackageID: "subscription", Version: "4.1.0", DetailsJSON: details, CheckedAt: time.Now()}})
	require.NoError(t, err)
	require.Equal(t, 1, stored)

	var row model.ShadowMismatchSample
	require.NoError(t, db.First(&row).Error)
	require.Equal(t, "subscription.user.info.get", row.RouteID)
	require.Equal(t, "GET", row.Method)
	require.Equal(t, "/api/v2/user/info?token=***", row.Path)
	require.Equal(t, "req-shadow-1", row.RequestID)
	require.Equal(t, 200, row.LegacyStatus)
	require.Equal(t, 200, row.NativeStatus)
	require.JSONEq(t, `[
		{"path":"$.data.last_ip","kind":"changed","legacy":"203.0.*.*","native":"203.0.*.*"},
		{"path":"$.data.subscribe_url","kind":"changed","legacy":"***","native":"***"},
		{"path":"$.data.traffic","kind":"changed","legacy":10,"native":11},
		{"path":"$.data.uuid","kind":"changed","legacy":"***","native":"***"}
	]`, row.DiffJSON)
	for _, secret := range []string{"hunter2", "query-secret", "legacy-secret", "native-secret", "7b3f6c1e", "8b3f6c1e", "203.0.113", "alice@"} {
		require.NotContains(t, details, secret)
		require.NotContains(t, row.DiffJSON+row.Path, secret)
	}
}
