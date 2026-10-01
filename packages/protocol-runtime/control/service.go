package main

import (
	"context"
	"log"

	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/protocol-runtime/native"
	"gorm.io/gorm"
)

// protocolRuntimeBridge is what the protocol-runtime host needs from the
// package bridge.
type protocolRuntimeBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newProtocolRuntimeService returns the protocol-runtime host's router. The
// routes in protocolRuntimeRoutes have a native handler: the protocol
// templates, and the diagnostic task history and detail on the adopted
// v2_agent_diagnostic_task. Such a route serves natively once the kernel
// sets its mode, and falls back to the legacy handler otherwise. The routes
// in bridgedRoutes always relay to the legacy handler, the agent WebSocket
// included.
func newProtocolRuntimeService(bridge protocolRuntimeBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "protocol-runtime", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := protocolRuntimeRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute
		},
		Native: service.Handlers(),
	})
}

// protocolRuntimeRoutes are the package's compatibility routes with a native
// handler.
var protocolRuntimeRoutes = map[string]struct{}{
	"protocol.admin.protocol_templates.get":  {},
	"protocol.admin.agent.tasks.get":         {},
	"protocol.admin.agent.tasks.task_id.get": {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler.
//
//   - Node protocols (list, create, update, delete) live in v2_node_protocol,
//     a protected kernel table no package may adopt. Its rows hold each
//     protocol's Reality private key, WireGuard server private key and
//     custom configuration, and the kernel builds every node's configuration
//     from them (UniProxy, the gRPC node service) and renders them into every
//     subscription without validating them again: its protocol validator
//     runs only on the kernel's own writes. The list answers the keys in
//     clear (the administrator's editor round-trips them). Deleting a
//     protocol also deletes its users' WireGuard peers (private and
//     preshared keys, in the protected v2_wireguard_peer) and its
//     subscription group links (the subscription package's).
//   - Synchronizing a node, its Agent Control status and Agent Control
//     operations dispatch over, or read, the gRPC control streams the
//     kernel's Agent Control manager holds in memory, and check the node in
//     v2_node, a protected kernel table.
//   - The administrator's agent list, live monitoring data, task creation
//     and command execution use the agents' live WebSocket connections and
//     the reports the kernel keeps in memory; creating a task sends it over
//     the connection and waits for the agent's acknowledgement.
//   - The agent routes (registration, heartbeat, task poll, result, monitor
//     and the WebSocket) authenticate node credentials (v2_node,
//     v2_forward_node) in the kernel, mark nodes online in those tables,
//     keep connections and reports in the kernel's memory, and hand out and
//     complete the forward package's bridge tasks and runtime jobs.
//     WebSocket routes always relay to the kernel.
var bridgedRoutes = map[string]struct{}{
	"protocol.admin.nodes.id.protocols.get":                 {},
	"protocol.admin.nodes.id.protocols.post":                {},
	"protocol.admin.nodes.id.protocols.protocol_id.put":     {},
	"protocol.admin.nodes.id.protocols.protocol_id.delete":  {},
	"protocol.admin.nodes.id.sync.post":                     {},
	"protocol.admin.nodes.id.agent_control.get":             {},
	"protocol.admin.nodes.id.agent_control.operations.post": {},
	"protocol.admin.agent.list.get":                         {},
	"protocol.admin.agent.monitor.get":                      {},
	"protocol.admin.agent.tasks.post":                       {},
	"protocol.admin.agent.execute.post":                     {},
	"protocol.agent.register.post":                          {},
	"protocol.agent.heartbeat.post":                         {},
	"protocol.agent.tasks.get":                              {},
	"protocol.agent.result.post":                            {},
	"protocol.agent.monitor.post":                           {},
	"protocol.agent.ws.get":                                 {},
}
