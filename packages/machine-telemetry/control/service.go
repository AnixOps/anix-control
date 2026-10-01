package main

import (
	"context"
	"log"
	"strings"

	kerneltelemetryv1 "github.com/AnixOps/anix-control/sdk/api/kerneltelemetry/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/machine-telemetry/native"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// machineTelemetryBridge is what the machine-telemetry host needs from the
// package bridge.
type machineTelemetryBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newMachineTelemetryService returns the machine-telemetry host's router.
// The routes in machineTelemetryRoutes have a native handler: the traffic
// routes on the kapi_traffic_log_v1 and kapi_user_directory_v1 kernel
// views, the dashboard on the kernel's KernelTelemetry over the bridge
// connection (local socket or module listener; a bridge without one leaves
// it legacy). Such a route serves natively once the kernel sets its mode,
// and falls back to the legacy handler otherwise. The routes in
// bridgedRoutes always relay to the legacy handler, the monitoring
// WebSocket included.
func newMachineTelemetryService(bridge machineTelemetryBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
	if conn, ok := bridge.(interface {
		Conn() grpc.ClientConnInterface
	}); ok && conn.Conn() != nil {
		service.Telemetry = kerneltelemetryv1.NewKernelTelemetryClient(conn.Conn())
	}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "machine-telemetry", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := machineTelemetryRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			// The package's own /api/v3/plugins routes (control_routes)
			// arrive as "machine-telemetry.control.<digest>" and relay to the kernel.
			return nativeRoute || bridgedRoute || strings.HasPrefix(routeID, pluginControlRoutePrefix)
		},
		Native: service.Handlers(),
	})
}

// machineTelemetryRoutes are the package's compatibility routes with a
// native handler.
var machineTelemetryRoutes = map[string]struct{}{
	"telemetry.admin.traffic.hourly.get":       {},
	"telemetry.admin.traffic.user_ranking.get": {},
	"telemetry.admin.dashboard.get":            {},
}

// pluginControlRoutePrefix starts the route id the kernel gives the
// package's own plugin control routes (service.PluginControlBridgeRouteID).
const pluginControlRoutePrefix = "machine-telemetry.control."

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler.
//
//   - The system information is the kernel binary's own build metadata
//     (version, build time, code and commit, set when the kernel is linked).
//   - The monitoring WebSocket streams the kernel's node list and node
//     statistics from v2_node, a protected kernel table; WebSocket routes
//     always relay to the kernel.
var bridgedRoutes = map[string]struct{}{
	"telemetry.admin.system.info.get": {},
	"telemetry.admin.ws.monitor.get":  {},
}
