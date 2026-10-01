package main

import (
	"context"
	"log"

	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/subscription/native"
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
// legacy handler otherwise. The routes in bridgedRoutes always relay to the
// legacy handler.
func newSubscriptionService(bridge subscriptionBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
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
	"subscription.admin.subscription.stats.get":                            {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler.
var bridgedRoutes = map[string]struct{}{
	// Deleting a group also deletes its members' rows in
	// v2_user_subscription_group, which is subscriber state: the kernel is
	// its only writer, no package may adopt a v2_user* table, and
	// KernelSubscriber has no call that edits subscription group
	// membership.
	"subscription.admin.subscription.groups.id.delete": {},
	// Granting a user a group (with its own expiry, traffic and renewal
	// price) and taking it away write v2_user_subscription_group; see above.
	"subscription.admin.subscription.users.user_id.groups.post":            {},
	"subscription.admin.subscription.users.user_id.groups.group_id.delete": {},
	// These answer whole proxy-node rows: v2_node_protocol with its
	// settings, TLS and Reality settings (private keys included) and custom
	// configuration, and the v2_node it runs on. No kernel view may carry
	// keys, and the rows are the proxy-node package's.
	"subscription.admin.subscription.groups.id.protocols.get": {},
	"subscription.admin.subscription.protocols.available.get": {},
	// The preview renders a user's subscription with the kernel's renderer:
	// it reads the user's subscription token and proxy UUID, the plan, the
	// nodes and their protocols, and creates WireGuard peers and keys for
	// the user.
	"subscription.admin.subscription.preview.post": {},
	// The subscription link settings combine the kernel's process
	// configuration (app.subscribe_path) with app.subscribe_domains in the
	// protected v2_system_config; a package can see neither.
	"subscription.admin.system.subscription_settings.get": {},
	// A user's subscription summary is served from the kernel's cache
	// (30 seconds, in memory or Redis, with the time it was cached in the
	// answer), which a native answer cannot share, and carries the
	// subscription link settings above.
	"subscription.user.subscription.get": {},
}
