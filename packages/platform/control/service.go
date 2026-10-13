package control

import (
	"context"
	"log"

	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/packages/platform/native"
	"gorm.io/gorm"
)

// platformBridge is what the platform host needs from the package bridge.
type platformBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newPlatformService returns the platform host's router. The route in
// platformRoutes has a native handler on the kapi_system_audit_log_v1 view;
// it serves natively once the kernel sets its mode, and falls back to the
// legacy handler otherwise. The routes in bridgedRoutes always relay to the
// legacy handler.
func newPlatformService(bridge platformBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "platform", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := platformRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute
		},
		Native: service.Handlers(),
	})
}

// platformRoutes are the package's compatibility routes with a native
// handler.
var platformRoutes = map[string]struct{}{
	"platform.admin.system.audit_logs.get": {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler.
//   - The system configuration routes work on any key of v2_system_config,
//     a protected kernel table. KernelSettings reaches the keys of a
//     namespace only, and the package holds no grant over every key, which
//     would amount to every secret: the answers mask secrets, but a write
//     can point an address the kernel sends a secret to (the NodeX or SMTP
//     host) anywhere (docs/architecture/settings-service.md).
var bridgedRoutes = map[string]struct{}{
	"platform.admin.system.configs.get":        {},
	"platform.admin.system.configs.key.get":    {},
	"platform.admin.system.configs.key.put":    {},
	"platform.admin.system.configs.key.delete": {},
}
