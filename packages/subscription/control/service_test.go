package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/subscription/native"
	"github.com/AnixOps/anix-control/v4/packages/subscription/native/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type bridgeStub struct {
	operation string
	modes     map[string]string
	lease     packagebridgesdk.StorageLease
}

func (s *bridgeStub) LeaseStorage(context.Context) (packagebridgesdk.StorageLease, error) {
	if s.lease.Driver == "" {
		return packagebridgesdk.StorageLease{}, packagebridgesdk.ErrSessionOperationUnsupported
	}
	return s.lease, nil
}

func (s *bridgeStub) GetPackageConfig(context.Context) (packagebridgesdk.PackageConfig, error) {
	if s.modes == nil {
		return packagebridgesdk.PackageConfig{}, packagebridgesdk.ErrSessionOperationUnsupported
	}
	return packagebridgesdk.PackageConfig{Revision: 1, RouteModes: s.modes}, nil
}

func (s *bridgeStub) Invoke(_ context.Context, _ []byte, operation string, _ []byte) (packagebridgesdk.Response, error) {
	s.operation = operation
	return packagebridgesdk.Response{StatusCode: 200, Body: []byte(`{"code":0,"msg":"操作成功","ts":1,"data":null}`)}, nil
}

func dispatch(t *testing.T, service *pluginhostsdk.Router, route string, request pluginhostsdk.DispatchRequest) pluginhostsdk.DispatchResponse {
	t.Helper()
	request.RouteID = route
	request.BridgeCapability = make([]byte, 32)
	request.DeadlineUnixMillis = time.Now().Add(5 * time.Second).UnixMilli()
	response, err := service.Dispatch(context.Background(), request)
	require.NoError(t, err)
	return response
}

// Until the kernel sets a route's mode, the host relays it to the legacy
// handler, bridged routes always; routes outside the package are refused.
func TestSubscriptionHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newSubscriptionService(bridge, "lease-1")
	require.NoError(t, err)
	for _, route := range []string{
		"subscription.admin.subscription.groups.get", "subscription.admin.subscription.groups.id.protocols.post",
		"subscription.admin.subscription.users.user_id.groups.post", "subscription.user.subscription.get",
	} {
		response := dispatch(t, service, route, pluginhostsdk.DispatchRequest{})
		require.EqualValues(t, 200, response.StatusCode)
		require.Equal(t, route, bridge.operation)
	}

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "plan.admin.plans.get", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{}).Handlers()
	require.Len(t, handlers, len(subscriptionRoutes))
	for route := range subscriptionRoutes {
		require.Contains(t, handlers, route, "every native route has a handler")
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route, "a bridged route has no native handler")
		require.NotContains(t, subscriptionRoutes, route)
	}
}

// The host accepts exactly the package's declared compatibility routes.
func TestSubscriptionHostRoutesAreThePackageRoutes(t *testing.T) {
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
	for route := range subscriptionRoutes {
		got = append(got, route)
	}
	for route := range bridgedRoutes {
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
}

// The package adopts its four tables and reads the plan, entitlement,
// membership and node views. It adopts no v2_user* table, reads no user
// directory (e-mail addresses) and no node or protocol settings, and calls
// no KernelSubscriber family: the routes that would change subscriber state
// stay bridged.
func TestSubscriptionManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{
		"kernel.storage.v1", "kernel.storage.adopt:v2_subscription_group", "kernel.storage.adopt:v2_subscription_template",
		"kernel.storage.adopt:v2_plan_subscription_group", "kernel.storage.adopt:v2_subscription_group_node_protocols",
		"kernel.view:kapi_plan_catalog_v1", "kernel.view:kapi_subscriber_entitlement_v1", "kernel.view:kapi_user_subscription_group_v1",
		"kernel.view:kapi_node_protocol_v1", "kernel.view:kapi_node_heartbeat_v1",
	}, manifest.Capabilities)
}

// A native route runs on the leased storage: the adopted tables keep their
// kernel names whatever the lease's prefix for the package's own tables.
func TestSubscriptionHostServesNativeRoutesOnTheLease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	// A table stands in for the kernel view here.
	require.NoError(t, db.AutoMigrate(&model.SubscriptionGroup{}, &model.SubscriptionTemplate{}, &model.GroupProtocol{}, &native.NodeProtocol{}))
	require.NoError(t, db.Create(&model.SubscriptionGroup{ID: 1, Name: "default", Enable: 1}).Error)
	require.NoError(t, db.Create(&model.SubscriptionTemplate{ID: 1, GroupID: 1, Name: "hk", Enable: 1}).Error)
	require.NoError(t, db.Create(&[]native.NodeProtocol{{ID: 7, NodeID: 1}, {ID: 8, NodeID: 1}}).Error)
	require.NoError(t, db.Create(&model.GroupProtocol{SubscriptionGroupID: 1, NodeProtocolID: 8}).Error)

	link, get := "subscription.admin.subscription.groups.id.protocols.post", "subscription.admin.subscription.groups.id.get"
	stub := &bridgeStub{
		modes: map[string]string{link: "native", get: "native"},
		lease: packagebridgesdk.StorageLease{
			Driver: "sqlite", DSN: path, TablePrefix: "pkg_subscription_",
			AdoptedTables: []string{
				"v2_plan_subscription_group", "v2_subscription_group", "v2_subscription_group_node_protocols", "v2_subscription_template",
			},
		},
	}
	service, err := newSubscriptionService(stub, "lease-1")
	require.NoError(t, err)
	service.Refresh(context.Background())
	_, effective := service.Mode(link)
	require.Equal(t, "native", effective)

	params := pluginhostsdk.RequestMetadata{PathParams: map[string]string{"id": "1"}}
	response := dispatch(t, service, link, pluginhostsdk.DispatchRequest{
		Method: "POST", PrincipalJSON: []byte(`{"actor_id":1,"admin":true}`), RequestBody: []byte(`{"protocol_ids":[7]}`), Metadata: params,
	})
	require.JSONEq(t, `"更新成功"`, mustField(t, response.ResponseBody, "data", "message"))
	require.Empty(t, stub.operation, "a native route does not reach the legacy handler")
	var links []model.GroupProtocol
	require.NoError(t, db.Find(&links).Error)
	require.Equal(t, []model.GroupProtocol{{SubscriptionGroupID: 1, NodeProtocolID: 7}}, links)

	response = dispatch(t, service, get, pluginhostsdk.DispatchRequest{Method: "GET", Metadata: params})
	require.JSONEq(t, `"hk"`, mustField(t, response.ResponseBody, "data", "templates", "0", "name"))
}

// mustField returns the JSON of a nested field of body.
func mustField(t *testing.T, body []byte, path ...string) string {
	t.Helper()
	var value any
	require.NoError(t, json.Unmarshal(body, &value), "%s", body)
	for _, key := range path {
		switch node := value.(type) {
		case map[string]any:
			value = node[key]
		case []any:
			index := 0
			for _, digit := range key {
				index = index*10 + int(digit-'0')
			}
			require.Less(t, index, len(node), "%s", body)
			value = node[index]
		default:
			t.Fatalf("%v is not in %s", path, body)
		}
	}
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}
