package main

import (
	"context"
	"log"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/protocol-runtime/native"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// protocolRuntimeBridge is what the protocol-runtime host needs from the
// package bridge.
type protocolRuntimeBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newProtocolRuntimeService returns the protocol-runtime host's router. The
// routes in protocolRuntimeRoutes have a native handler; such a route serves
// natively once the kernel sets its mode, and falls back to the legacy
// handler otherwise:
//   - the protocol templates, and the diagnostic task history and detail on
//     the adopted v2_agent_diagnostic_task;
//   - a node's protocols and their writes on v2_node_protocol, which the
//     lease adopts only once the node credential split is finalized: until
//     then they answer from the legacy handler, and a host started before
//     the finalize keeps them legacy until it restarts. The secrets in a
//     write reach the kernel only, as sealed handles (PutSecretDocument);
//   - node synchronization, Agent Control and the administrator's agent
//     routes, through the kernel's KernelNodeOps over the bridge connection
//     (local socket or module listener). A bridge without one leaves the
//     routes that need it legacy.
//
// The routes in bridgedRoutes always relay to the legacy handler, the agent
// WebSocket included.
func newProtocolRuntimeService(bridge protocolRuntimeBridge, leaseID string) (*pluginhostsdk.Router, error) {
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
	"protocol.admin.protocol_templates.get":                 {},
	"protocol.admin.agent.tasks.get":                        {},
	"protocol.admin.agent.tasks.task_id.get":                {},
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
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler. All are
// kernel-owned: the agent routes (registration, heartbeat, task poll,
// result, monitor and the WebSocket) authenticate node credentials
// (v2_node, v2_forward_node) in the kernel, mark nodes online in those
// tables, keep connections and reports in the kernel's memory, and hand out
// and complete the forward package's bridge tasks and runtime jobs.
// WebSocket routes always relay to the kernel.
var bridgedRoutes = map[string]struct{}{
	"protocol.agent.register.post":  {},
	"protocol.agent.heartbeat.post": {},
	"protocol.agent.tasks.get":      {},
	"protocol.agent.result.post":    {},
	"protocol.agent.monitor.post":   {},
	"protocol.agent.ws.get":         {},
}
