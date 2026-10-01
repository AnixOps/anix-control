package main

import (
	"context"
	"log"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/platform/native"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// platformBridge is what the platform host needs from the package bridge.
type platformBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newPlatformService returns the platform host's router. The routes in
// platformRoutes have a native handler on the adopted backup record table,
// the kapi_system_audit_log_v1 view and the kernel's KernelSettings; such a
// route serves natively once the kernel sets its mode, and falls back to
// the legacy handler otherwise. Reading and updating the backup
// configuration call KernelSettings over the bridge connection (local
// socket or module listener); a bridge without one leaves them legacy. The
// routes in bridgedRoutes always relay to the legacy handler.
func newPlatformService(bridge platformBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
	if conn, ok := bridge.(interface {
		Conn() grpc.ClientConnInterface
	}); ok && conn.Conn() != nil {
		service.Settings = kernelsettingsv1.NewKernelSettingsClient(conn.Conn())
	}
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
	"platform.admin.system.audit_logs.get":    {},
	"platform.admin.system.backup.config.get": {},
	"platform.admin.system.backup.config.put": {},
	"platform.admin.system.backups.get":       {},
	"platform.admin.system.backup.stats.get":  {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler.
//   - The system configuration routes work on any key of v2_system_config,
//     a protected kernel table. KernelSettings reaches the keys of a
//     namespace only, and the package holds no grant over every key, which
//     would amount to every secret: the answers mask secrets, but a write
//     can point an address the kernel sends a secret to (the NodeX or SMTP
//     host) anywhere (docs/architecture/settings-service.md).
//   - Creating, deleting and restoring a backup write, remove and restore
//     archives of the database and files on the kernel's disk.
var bridgedRoutes = map[string]struct{}{
	"platform.admin.system.configs.get":             {},
	"platform.admin.system.configs.key.get":         {},
	"platform.admin.system.configs.key.put":         {},
	"platform.admin.system.configs.key.delete":      {},
	"platform.admin.system.backup.post":             {},
	"platform.admin.system.backups.id.delete":       {},
	"platform.admin.system.backups.id.restore.post": {},
}
