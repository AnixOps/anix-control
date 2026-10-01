package main

import (
	"log"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/wireguard/native"
)

// newWireGuardService returns the wireguard host's router. The package's one
// route has a native handler and needs no storage; it serves natively once
// the kernel sets its mode, and falls back to the legacy handler otherwise.
func newWireGuardService(bridge pluginhostsdk.RouterBridge, leaseID string) (*pluginhostsdk.Router, error) {
	service := &native.Service{}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "wireguard", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := wireGuardRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute
		},
		Native: service.Handlers(),
	})
}

// wireGuardRoutes are the package's compatibility routes with a native
// handler.
var wireGuardRoutes = map[string]struct{}{
	"wireguard.admin.wireguard.keypair.post": {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler. There are none: the WireGuard protocols themselves, with their
// server keys, are protocol-runtime's routes on the protected
// v2_node_protocol, and the users' peers live in the protected
// v2_wireguard_peer, which the kernel's subscription rendering writes.
var bridgedRoutes = map[string]struct{}{}
