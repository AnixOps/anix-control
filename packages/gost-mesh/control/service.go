package main

import (
	"log"
	"strings"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/gost-mesh/native"
	"google.golang.org/grpc"
)

// newGostMeshService returns the gost-mesh host's router. The routes in
// gostMeshRoutes have a native handler and need no storage; such a route
// serves natively once the kernel sets its mode, and falls back to the
// legacy handler otherwise. The NodeX status and diagnosis read their
// settings through the kernel's KernelSettings over the bridge connection
// (local socket or module listener); a bridge without one leaves them
// legacy. No route is bridged for good.
func newGostMeshService(bridge pluginhostsdk.RouterBridge, leaseID string) (*pluginhostsdk.Router, error) {
	service := &native.Service{}
	if conn, ok := bridge.(interface {
		Conn() grpc.ClientConnInterface
	}); ok && conn.Conn() != nil {
		service.Settings = kernelsettingsv1.NewKernelSettingsClient(conn.Conn())
	}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "gost-mesh", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := gostMeshRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			// The package's own /api/v3/plugins routes (control_routes)
			// arrive as "gost-mesh.control.<digest>" and relay to the kernel.
			return nativeRoute || bridgedRoute || strings.HasPrefix(routeID, pluginControlRoutePrefix)
		},
		Native: service.Handlers(),
	})
}

// gostMeshRoutes are the package's compatibility routes with a native
// handler: all of them.
var gostMeshRoutes = map[string]struct{}{
	"gost.admin.forward.test_connection.post": {},
	"gost.admin.forward.nodex.status.get":     {},
	"gost.admin.forward.nodex.doctor.get":     {},
}

// pluginControlRoutePrefix starts the route id the kernel gives the
// package's own plugin control routes (service.PluginControlBridgeRouteID).
const pluginControlRoutePrefix = "gost-mesh.control."

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler. There are none
// left.
var bridgedRoutes = map[string]struct{}{}
