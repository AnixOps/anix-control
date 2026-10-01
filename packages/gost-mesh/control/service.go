package main

import (
	"log"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/gost-mesh/native"
)

// newGostMeshService returns the gost-mesh host's router. The route in
// gostMeshRoutes has a native handler and needs no storage; it serves
// natively once the kernel sets its mode, and falls back to the legacy
// handler otherwise. The routes in bridgedRoutes always relay to the legacy
// handler.
func newGostMeshService(bridge pluginhostsdk.RouterBridge, leaseID string) (*pluginhostsdk.Router, error) {
	service := &native.Service{}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "gost-mesh", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := gostMeshRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute
		},
		Native: service.Handlers(),
	})
}

// gostMeshRoutes are the package's compatibility routes with a native
// handler.
var gostMeshRoutes = map[string]struct{}{
	"gost.admin.forward.test_connection.post": {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler. The NodeX
// runtime status and diagnosis read the NodeX address, shared token and
// timeout from the system configuration (forward.runtime.nodex.*, in the
// protected v2_system_config; the token is a secret no view shows) and call
// NodeX's health and runtime status endpoints with that token.
var bridgedRoutes = map[string]struct{}{
	"gost.admin.forward.nodex.status.get": {},
	"gost.admin.forward.nodex.doctor.get": {},
}
