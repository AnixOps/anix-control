package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
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
// WebSocket included. Of the package's own control routes, the per-node
// services table (native.NodesRoute) is the package's alone and always
// served here; the others relay to the kernel.
func newMachineTelemetryService(bridge machineTelemetryBridge, leaseID string) (*machineTelemetryHost, error) {
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
	router, err := pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
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
	if err != nil {
		return nil, err
	}
	return &machineTelemetryHost{Router: router, owned: map[string]pluginhostsdk.NativeHandler{
		native.NodesRouteID: service.NodeServices,
	}}, nil
}

// machineTelemetryHost is the router plus the control routes the package
// owns outright. Route modes apply to compatibility routes, which have a
// legacy handler to fall back to; an owned control route has none, so it
// is always answered by the package.
type machineTelemetryHost struct {
	*pluginhostsdk.Router
	owned map[string]pluginhostsdk.NativeHandler
}

// Dispatch answers an owned control route itself and hands every other
// route to the router.
func (h *machineTelemetryHost) Dispatch(ctx context.Context, request pluginhostsdk.DispatchRequest) (pluginhostsdk.DispatchResponse, error) {
	handler, owned := h.owned[request.RouteID]
	if !owned {
		return h.Router.Dispatch(ctx, request)
	}
	if h.Draining() {
		return pluginhostsdk.DispatchResponse{}, errors.New("package is unavailable")
	}
	response, err := runOwned(ctx, handler, request)
	if err != nil {
		return pluginhostsdk.DispatchResponse{}, err
	}
	return pluginhostsdk.DispatchResponse{StatusCode: response.StatusCode, ResponseBody: response.Body, Headers: response.Headers}, nil
}

// errOwnedRoutePanicked reports a recovered panic in an owned route.
var errOwnedRoutePanicked = errors.New("owned route panicked")

func runOwned(ctx context.Context, handler pluginhostsdk.NativeHandler, request pluginhostsdk.DispatchRequest) (response pluginhostsdk.NativeResponse, err error) {
	var principal pluginhostsdk.Principal
	if len(request.PrincipalJSON) > 0 {
		if err := json.Unmarshal(request.PrincipalJSON, &principal); err != nil {
			return pluginhostsdk.NativeResponse{}, errors.New("package request principal is invalid")
		}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: route %s: %v", errOwnedRoutePanicked, request.RouteID, recovered)
		}
	}()
	response, err = handler(ctx, pluginhostsdk.NativeRequest{
		RouteID: request.RouteID, Method: request.Method, Body: request.RequestBody, Principal: principal,
		Metadata: request.Metadata,
	})
	if err == nil && response.StatusCode == 0 {
		response.StatusCode = http.StatusOK
	}
	return response, err
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
