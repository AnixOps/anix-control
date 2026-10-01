package main

import (
	"context"
	"log"

	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/proxy-node/native"
	"gorm.io/gorm"
)

// proxyNodeBridge is what the proxy-node host needs from the package
// bridge, the WebSocket relay for the agent connection included.
type proxyNodeBridge interface {
	pluginhostsdk.RouterBridge
	pluginhostsdk.RouterWebSocketBridge
	packagestoresdk.Leaser
}

// newProxyNodeService returns the proxy-node host's router. The routes in
// proxyNodeRoutes have a native handler on the adopted v2_load_balancer and
// v2_node_log tables and the kapi_node_status_v1 view; such a route serves
// natively once the kernel sets its mode, and falls back to the legacy
// handler otherwise. The routes in bridgedRoutes always relay to the legacy
// handler, the agent WebSocket included.
func newProxyNodeService(bridge proxyNodeBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "proxy-node", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := proxyNodeRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute
		},
		Native: service.Handlers(),
	})
}

// proxyNodeRoutes are the package's compatibility routes with a native
// handler.
var proxyNodeRoutes = map[string]struct{}{
	"proxy.loadbalancer.get":        {},
	"proxy.loadbalancer.post":       {},
	"proxy.loadbalancer.id.get":     {},
	"proxy.loadbalancer.id.put":     {},
	"proxy.loadbalancer.id.delete":  {},
	"proxy.admin.nodes.stats.get":   {},
	"proxy.admin.nodes.id.logs.get": {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler.
//
// v2_node holds each node's API key, key hash and shared secret, which the
// kernel's node authentication checks: the node API, UniProxy, the agent
// WebSocket and gRPC control stream, and agent package downloads. A package
// that could read or write those columns could act as any node, and so read
// every subscriber's proxy credentials from the UniProxy user list, or let
// in a node of its choosing. Until column-level grants can withhold them,
// v2_node is a protected kernel table, and so is v2_authorized_key, whose
// registration keys mint node credentials. v2_node_protocol, with each
// protocol's Reality private key, settings and custom configuration, is the
// protocol-runtime package's.
//
//   - Node list and detail: their answers embed each node's protocols, Reality
//     private keys and custom configurations included, which no kernel view
//     may carry.
//   - Node creation issues the node's API key and secret into v2_node.
//   - Node update writes v2_node and clears the kernel's in-memory node
//     cache.
//   - Node deletion removes the node's protocols and WireGuard peers in one
//     kernel transaction.
//   - Node credentials answer the API key and secret.
//   - The raw configuration is the node's runtime configuration, WireGuard
//     private keys included; its update writes v2_node.
//   - Configuration validation reads no table, but it runs the kernel's
//     WireGuard protocol validator, which the protocol routes and the raw
//     configuration update share.
//   - Authorization keys (list, creation, deletion and the internal
//     creation for automation): v2_authorized_key holds each registration
//     key in clear, the list answers it, and the kernel's HTTP and gRPC
//     registration read the table.
//   - Node registration, heartbeat and runtime health: a registration mints
//     node credentials; the others are authenticated by the node's API key
//     in the kernel and write v2_node, the heartbeat also adding traffic up
//     the node's parent chain.
//   - The agent WebSocket is a live connection the kernel holds and pushes
//     to.
//   - UniProxy: node-authenticated; the user list carries every eligible
//     subscriber's UUID, a traffic push records subscriber traffic, and the
//     online list lives in the kernel's in-memory cache.
//   - Load balancer statistics and health check: they read the forward
//     package's v2_forward_node, and the check probes each forward node over
//     the network and writes its status.
var bridgedRoutes = map[string]struct{}{
	"proxy.admin.nodes.get":                  {},
	"proxy.admin.nodes.post":                 {},
	"proxy.admin.nodes.id.get":               {},
	"proxy.admin.nodes.id.put":               {},
	"proxy.admin.nodes.id.delete":            {},
	"proxy.admin.nodes.id.credentials.get":   {},
	"proxy.admin.nodes.id.raw_config.get":    {},
	"proxy.admin.nodes.id.raw_config.put":    {},
	"proxy.admin.nodes.validate_config.post": {},
	"proxy.admin.auth_keys.get":              {},
	"proxy.admin.auth_keys.post":             {},
	"proxy.admin.auth_keys.id.delete":        {},
	"proxy.internal.auth_keys.post":          {},
	"proxy.node.register.post":               {},
	"proxy.node.heartbeat.post":              {},
	"proxy.node.runtime_health.post":         {},
	"proxy.node.ws.get":                      {},
	"proxy.server.uniproxy.config.get":       {},
	"proxy.server.uniproxy.user.get":         {},
	"proxy.server.uniproxy.alivelist.get":    {},
	"proxy.server.uniproxy.push.post":        {},
	"proxy.server.uniproxy.alive.post":       {},
	"proxy.loadbalancer.id.stats.get":        {},
	"proxy.loadbalancer.id.check.post":       {},
}
