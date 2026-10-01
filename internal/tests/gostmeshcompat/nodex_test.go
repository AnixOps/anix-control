package gostmeshcompat

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/gost-mesh/native"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// gostMeshHost is the identity the kernel serves the gost-mesh host as; its
// signed release declares the nodex namespace's read and secrets
// capabilities.
var gostMeshHost = packagebridge.HostIdentity{PackageID: "gost-mesh", Version: "4.1.0", Generation: 1}

const nodeXToken = "nodex-shared-token"

// nodeXRoute runs a NodeX route whose native side reads the settings
// through the real KernelSettings server on its database.
func nodeXRoute(t *testing.T, path, routeID string, legacy func(*handler.ForwardHandler, *gin.Context)) packagecompat.Route {
	return packagecompat.Route{
		Method: "GET", Pattern: path, RouteID: routeID,
		Legacy: func(c *gin.Context) { legacy(handler.NewForwardHandler(), c) },
		Models: []any{&model.SystemConfig{}, &model.OperationLog{}, &model.SettingsRequest{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{Settings: packagecompat.KernelSettings(t, db, packagecompat.SettingsGrants{Host: gostMeshHost, Capabilities: []string{
				service.SettingsCapability(service.SettingsNamespaceNodeX, service.SettingsAccessRead),
				service.SettingsCapability(service.SettingsNamespaceNodeX, service.SettingsAccessSecrets),
			}})}
			return service.Handlers()[routeID]
		},
	}
}

// nodeX serves a NodeX control plane: health answers /health, status the
// runtime status to a request with the shared token, and 401 otherwise.
func nodeX(t *testing.T, health func(http.ResponseWriter), status func(http.ResponseWriter)) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			health(w)
		case "/api/v2/internal/forward/runtime/status":
			if r.Header.Get("Authorization") != "Bearer "+nodeXToken || r.Header.Get("X-API-Key") != nodeXToken {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid forward api token"}`))
				return
			}
			status(w)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func write(code int, text string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(text))
	}
}

// nodeXSettings seeds the NodeX settings (a nil value is not stored) and
// rows of other namespaces.
func nodeXSettings(baseURL, token, timeout *string) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		rows := []model.SystemConfig{
			{Key: "forward.runtime_backend", Value: "gost", Type: "string", Group: "forward_runtime"},
			{Key: "forward.runtime.nodex_mode", Value: "true", Type: "bool", Group: "forward_runtime"},
		}
		for key, value := range map[string]*string{
			"forward.runtime.nodex.base_url": baseURL, "forward.runtime.nodex.token": token, "forward.runtime.nodex.timeout_seconds": timeout,
		} {
			if value != nil {
				rows = append(rows, model.SystemConfig{Key: key, Value: *value, Type: "string", Group: "forward_runtime"})
			}
		}
		require.NoError(t, db.Create(&rows).Error)
	}
}

func text(value string) *string { return &value }

const runtimeStatus = `{"data":{"version":" 1.4.2 ","executePath":"/api/v2/internal/forward/runtime/execute","statusPath":"/api/v2/internal/forward/runtime/status",
	"authRequired":true,"supports":{"resourceTypes":["panel_forward"],"backends":["GOST","iptables_ansible"],"actions":["apply","remove"]},
	"modes":{"gost":{"supported":false},"iptablesAnsible":{"backend":"nftables_ansible","supported":true,"ready":false,"command":"ansible-playbook",
	"inventoryPath":"/etc/ansible/hosts","issues":["inventory missing: /etc/ansible/hosts"," ","inventory missing: /etc/ansible/hosts"]}}}}`

func TestNodeXRouteParity(t *testing.T) {
	ok := write(http.StatusOK, " ok\n")
	healthy := nodeX(t, ok, write(http.StatusOK, runtimeStatus))
	gostByMode := nodeX(t, ok, write(http.StatusOK, `{"data":{"version":"1.5.0","supports":{"backends":[]},"modes":{"gost":{"supported":true}}}}`))
	noGost := nodeX(t, ok, write(http.StatusOK, `{"data":{"version":"1.0.0","supports":{"backends":["iptables_ansible"]}}}`))
	starting := nodeX(t, write(http.StatusOK, "starting"), write(http.StatusOK, runtimeStatus))
	unhealthy := nodeX(t, write(http.StatusServiceUnavailable, "maintenance <window>"), write(http.StatusBadGateway, `{"msg":"runtime down"}`))
	notJSON := nodeX(t, ok, write(http.StatusOK, `<html>NodeX</html>`))
	empty := nodeX(t, ok, write(http.StatusOK, "  "))
	noData := nodeX(t, ok, write(http.StatusOK, `{"message":"warming up"}`))
	failedPlain := nodeX(t, ok, write(http.StatusInternalServerError, `"plain failure"`))
	wrongData := nodeX(t, ok, write(http.StatusOK, `{"data":"ready"}`))
	wrongVersion := nodeX(t, ok, write(http.StatusOK, `{"data":{"version":5}}`))
	wrongMode := nodeX(t, ok, write(http.StatusOK, `{"data":{"modes":{"iptablesAnsible":{"issues":[1]}}}}`))
	refused := fmt.Sprintf("http://127.0.0.1:%d", closedPort(t))

	// Each case answers a diagnosis, whose check time is masked, or an
	// error.
	cases := []struct {
		packagecompat.Case
		failed bool
	}{
		{Case: packagecompat.Case{Name: "NodeX advertises gost among its backends", Seed: nodeXSettings(text(healthy), text(nodeXToken), text("5"))}},
		{Case: packagecompat.Case{Name: "NodeX supports the gost mode", Seed: nodeXSettings(text(" "+gostByMode+"/ "), text(" "+nodeXToken+" "), nil)}},
		{Case: packagecompat.Case{Name: "NodeX without gost", Seed: nodeXSettings(text(noGost), text(nodeXToken), text(" "))}},
		{Case: packagecompat.Case{Name: "health that is not ok", Seed: nodeXSettings(text(starting), text(nodeXToken), nil)}},
		{Case: packagecompat.Case{Name: "NodeX failing", Seed: nodeXSettings(text(unhealthy), text(nodeXToken), nil)}},
		{Case: packagecompat.Case{Name: "a wrong token", Seed: nodeXSettings(text(healthy), text("stale-token"), nil)}},
		{Case: packagecompat.Case{Name: "a status that is not JSON", Seed: nodeXSettings(text(notJSON), text(nodeXToken), nil)}},
		{Case: packagecompat.Case{Name: "an empty status", Seed: nodeXSettings(text(empty), text(nodeXToken), nil)}},
		{Case: packagecompat.Case{Name: "a status without data", Seed: nodeXSettings(text(noData), text(nodeXToken), nil)}},
		{Case: packagecompat.Case{Name: "a failed status as a JSON string", Seed: nodeXSettings(text(failedPlain), text(nodeXToken), nil)}},
		{Case: packagecompat.Case{Name: "status data of another type", Seed: nodeXSettings(text(wrongData), text(nodeXToken), nil)}},
		{Case: packagecompat.Case{Name: "a status field of another type", Seed: nodeXSettings(text(wrongVersion), text(nodeXToken), nil)}},
		{Case: packagecompat.Case{Name: "a nested status field of another type", Seed: nodeXSettings(text(wrongMode), text(nodeXToken), nil)}},
		{Case: packagecompat.Case{Name: "nothing listens", Seed: nodeXSettings(text(refused), text(nodeXToken), nil)}},
		{Case: packagecompat.Case{Name: "an address that is not a URL", Seed: nodeXSettings(text("http://nodex host:1"), text(nodeXToken), nil)}},
		{Case: packagecompat.Case{Name: "no address", Seed: nodeXSettings(nil, text(nodeXToken), nil)}, failed: true},
		{Case: packagecompat.Case{Name: "an address that is only a slash", Seed: nodeXSettings(text("/"), text(nodeXToken), nil)}, failed: true},
		{Case: packagecompat.Case{Name: "no token", Seed: nodeXSettings(text(healthy), text("   "), nil)}, failed: true},
		{Case: packagecompat.Case{Name: "a timeout that is not a number", Seed: nodeXSettings(text(healthy), text(nodeXToken), text("5s"))}, failed: true},
		{Case: packagecompat.Case{Name: "a timeout of zero", Seed: nodeXSettings(text(healthy), text(nodeXToken), text("0"))}, failed: true},
		{Case: packagecompat.Case{Name: "no settings at all"}, failed: true},
	}
	for _, r := range []packagecompat.Route{
		nodeXRoute(t, "/api/v2/admin/forward/nodex/status", native.NodeXStatusRouteID, (*handler.ForwardHandler).GetNodeXRuntimeStatus),
		nodeXRoute(t, "/api/v2/admin/forward/nodex/doctor", native.NodeXDoctorRouteID, (*handler.ForwardHandler).DiagnoseNodeXRuntime),
	} {
		for _, c := range cases {
			c.Path, c.Principal = r.Pattern, admin
			if !c.failed {
				c.Mask = []string{"data.checkedAt"}
			}
			packagecompat.RunRead(t, r, c.Case)
		}
	}
}

// openSettings is a database with the system configuration table.
func openSettings(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "settings.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SystemConfig{}, &model.SettingsRequest{}))
	return db
}

// The native diagnosis reaches NodeX with the token KernelSettings answers
// in clear, so the cases compare real answers; without the secrets
// capability the token reads masked and the route refuses to guess.
func TestNativeNodeXDiagnosisNeedsTheSecret(t *testing.T) {
	healthy := nodeX(t, write(http.StatusOK, "ok"), write(http.StatusOK, runtimeStatus))
	db := openSettings(t)
	nodeXSettings(text(healthy), text(nodeXToken), nil)(t, db)
	read := service.SettingsCapability(service.SettingsNamespaceNodeX, service.SettingsAccessRead)
	secrets := service.SettingsCapability(service.SettingsNamespaceNodeX, service.SettingsAccessSecrets)

	clear := &native.Service{Settings: packagecompat.KernelSettings(t, db, packagecompat.SettingsGrants{Host: gostMeshHost, Capabilities: []string{read, secrets}})}
	response, err := clear.NodeXStatus(context.Background(), pluginhostsdk.NativeRequest{})
	require.NoError(t, err)
	require.Contains(t, string(response.Body), `"runtimeReady":{"ready":true`)

	masked := &native.Service{Settings: packagecompat.KernelSettings(t, db, packagecompat.SettingsGrants{Host: gostMeshHost, Capabilities: []string{read}})}
	response, err = masked.NodeXStatus(context.Background(), pluginhostsdk.NativeRequest{})
	require.NoError(t, err)
	require.Contains(t, string(response.Body), `"msg":"forward.runtime.nodex.token is masked: the package needs kernel.settings.nodex.secrets.v1"`)
	require.NotContains(t, string(response.Body), nodeXToken)
}
