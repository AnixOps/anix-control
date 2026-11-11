package control

import (
	"context"
	"log"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/packages/protocol-runtime/native"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// protocolRuntimeBridge is what the protocol-runtime host needs from the
// package bridge.
type protocolRuntimeBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newProtocolRuntimeService returns the protocol-runtime host's router.
// Every route in protocolRuntimeRoutes is served by its native handler
// alone (the kernel has no handler for them):
//   - the protocol templates, and the diagnostic task history and detail on
//     the adopted v2_agent_diagnostic_task;
//   - a node's protocols and their writes on v2_node_protocol, which the
//     lease adopts once the node credential split is finalized (the kernel
//     starts on a finalized split only). The secrets in a write reach the
//     kernel only, as sealed handles (PutSecretDocument);
//   - node synchronization, Agent Control and the administrator's agent
//     routes, through the kernel's KernelNodeOps over the bridge connection
//     (local socket or module listener).
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
			_, ok := protocolRuntimeRoutes[routeID]
			return ok
		},
		Native: service.Handlers(), NativeOnly: true,
	})
}

// protocolRuntimeRoutes are the package's compatibility routes.
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
}
