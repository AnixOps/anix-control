package control

import (
	"context"
	"log"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/packages/proxy-node/native"
	"google.golang.org/grpc"
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
// proxyNodeRoutes have a native handler; such a route serves natively once
// the kernel sets its mode, and falls back to the legacy handler otherwise.
// The node routes on v2_node and the registration key list need grants the
// kernel gives only once the node credential split is finalized: until the
// lease has them they answer from the legacy handler, and a host started
// before the finalize keeps them legacy until it restarts. The routes that
// reach the kernel's KernelNodeOps (a node's deletion and raw
// configuration, registration keys) use the
// bridge connection (local socket or module listener); a bridge without one
// leaves them legacy. The routes in bridgedRoutes always relay to the
// legacy handler, the agent WebSocket included.
func newProxyNodeService(bridge proxyNodeBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{
		Open: func(ctx context.Context) (*gorm.DB, error) {
			store, err := storage(ctx)
			if err != nil {
				return nil, err
			}
			return store.DB.WithContext(ctx), nil
		},
		Leased: func(ctx context.Context, name string) bool {
			store, err := storage(ctx)
			return err == nil && store.Leased(name)
		},
	}
	if conn, ok := bridge.(interface {
		Conn() grpc.ClientConnInterface
	}); ok && conn.Conn() != nil {
		service.NodeOps = kernelnodeopsv1.NewKernelNodeOpsClient(conn.Conn())
	}
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
	"proxy.admin.nodes.stats.get":         {},
	"proxy.admin.nodes.id.logs.get":       {},
	"proxy.admin.nodes.get":               {},
	"proxy.admin.nodes.id.get":            {},
	"proxy.admin.nodes.id.delete":         {},
	"proxy.admin.nodes.id.raw_config.get": {},
	"proxy.admin.nodes.id.raw_config.put": {},
	"proxy.admin.auth_keys.get":           {},
	"proxy.admin.auth_keys.post":          {},
	"proxy.admin.auth_keys.id.delete":     {},
	"proxy.internal.auth_keys.post":       {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler.
//
//   - Node creation (bridged) also creates the node's default protocol in
//     v2_node_protocol, protocol-runtime's table, in the kernel's
//     transaction; no KernelNodeOps call creates a protocol.
//   - Node update (bridged) also records a group change in the subscriber
//     change log (subscriber.RecordNodeGroupChangeTx), revokes the node's
//     agent certificates when it is disabled, and drops the kernel's node
//     cache, in the kernel's transaction; no KernelNodeOps call records a
//     node's group change, and SyncNode would also store and push the
//     desired configuration, which the legacy update does not.
//   - Configuration validation (kernel-owned): its answer's size is the
//     length of the configuration with its secrets, which the gateway
//     seals into handles that never resolve for this route, so neither the
//     package nor ValidateNodeConfig can compute it.
//   - Node credentials (kernel-owned, D4): the answer is the stored API key
//     and secret, and no contract call reveals a stored secret.
var bridgedRoutes = map[string]struct{}{
	"proxy.admin.nodes.post":                 {},
	"proxy.admin.nodes.id.put":               {},
	"proxy.admin.nodes.validate_config.post": {},
	"proxy.admin.nodes.id.credentials.get":   {},
}
