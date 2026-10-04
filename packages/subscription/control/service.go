package main

import (
	"context"
	"log"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/subscription/native"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// subscriptionBridge is what the subscription host needs from the package
// bridge.
type subscriptionBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newSubscriptionService returns the subscription host's router. The routes
// in subscriptionRoutes have a native handler on the adopted
// v2_subscription_group, v2_subscription_template,
// v2_plan_subscription_group and v2_subscription_group_node_protocols tables
// and the plan, user, membership, entitlement and node views; such a route
// serves natively once the kernel sets its mode, and falls back to the
// legacy handler otherwise. Deleting a group, granting or taking away a
// user's group and the user's subscription summary call the kernel's
// KernelSubscriber over the bridge connection (local socket or module
// listener); a bridge without one leaves them legacy. A group's protocols
// and the protocol pool read kapi_node_protocol_public_v1 and
// kapi_node_public_v1, which the kernel grants only once the node credential
// split of v2_node_protocol and v2_node is finalized; until the lease grants
// both, they answer from the legacy handler. The lease is taken when the
// host first opens its storage, so a host started before the finalize keeps
// them legacy until it restarts. The routes in bridgedRoutes always relay to
// the legacy handler.
func newSubscriptionService(bridge subscriptionBridge, leaseID string) (*pluginhostsdk.Router, error) {
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
		service.Subscriber = kernelsubscriberv1.NewKernelSubscriberClient(conn.Conn())
	}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "subscription", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := subscriptionRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute
		},
		Native: service.Handlers(),
	})
}

// subscriptionRoutes are the package's compatibility routes with a native
// handler.
var subscriptionRoutes = map[string]struct{}{
	"subscription.admin.subscription.formats.get":                          {},
	"subscription.admin.subscription.protocols.get":                        {},
	"subscription.admin.subscription.groups.get":                           {},
	"subscription.admin.subscription.groups.post":                          {},
	"subscription.admin.subscription.groups.id.get":                        {},
	"subscription.admin.subscription.groups.id.put":                        {},
	"subscription.admin.subscription.groups.id.delete":                     {},
	"subscription.admin.subscription.groups.id.templates.get":              {},
	"subscription.admin.subscription.groups.id.templates.post":             {},
	"subscription.admin.subscription.groups.id.protocols.post":             {},
	"subscription.admin.subscription.templates.id.get":                     {},
	"subscription.admin.subscription.templates.id.put":                     {},
	"subscription.admin.subscription.templates.id.delete":                  {},
	"subscription.admin.subscription.plans.plan_id.groups.get":             {},
	"subscription.admin.subscription.plans.plan_id.groups.post":            {},
	"subscription.admin.subscription.plans.plan_id.groups.group_id.delete": {},
	"subscription.admin.subscription.users.user_id.groups.get":             {},
	"subscription.admin.subscription.users.user_id.groups.post":            {},
	"subscription.admin.subscription.users.user_id.groups.group_id.delete": {},
	"subscription.admin.subscription.stats.get":                            {},
	"subscription.user.subscription.get":                                   {},
	"subscription.admin.subscription.groups.id.protocols.get":              {},
	"subscription.admin.subscription.protocols.available.get":              {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler.
var bridgedRoutes = map[string]struct{}{
	// The preview renders a user's subscription with the kernel's renderer:
	// it reads the user's subscription token and proxy UUID, the plan, the
	// nodes and their protocols, and creates WireGuard peers and keys for
	// the user. Whether the renderer stays in the kernel is an open decision
	// (package-extraction.md section 3.2, "What unblocks the bridged
	// routes").
	"subscription.admin.subscription.preview.post": {},
	// The subscription link settings combine the kernel's process
	// configuration (app.subscribe_path) with app.subscribe_domains in the
	// protected v2_system_config; a package can see neither. A KernelSettings
	// namespace for the subscription link, carrying the process
	// configuration too, would unblock it.
	"subscription.admin.system.subscription_settings.get": {},
}
