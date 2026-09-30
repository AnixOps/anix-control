package main

import (
	"context"
	"log"

	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/platform/native"
	"gorm.io/gorm"
)

// platformBridge is what the platform host needs from the package bridge.
type platformBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newPlatformService returns the platform host's router. The routes in
// platformRoutes have a native handler on the adopted backup tables and the
// kapi_system_audit_log_v1 view; such a route serves natively once the
// kernel sets its mode, and falls back to the legacy handler otherwise. The
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
	"platform.admin.system.backups.get":       {},
	"platform.admin.system.backup.stats.get":  {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler.
//   - The system configuration lives in v2_system_config, a protected kernel
//     table that no package may adopt; its values include secrets (the
//     single-key answer returns them in clear), which no kernel view may
//     expose; and each write records an audit entry in the protected
//     v2_operation_log.
//   - Updating the backup configuration records an audit entry as well, and
//     refreshes the copy the kernel's backup service keeps in memory, which
//     backup creation reads its storage path from.
//   - Creating, deleting and restoring a backup write, remove and restore
//     archives of the database and files on the kernel's disk.
var bridgedRoutes = map[string]struct{}{
	"platform.admin.system.configs.get":             {},
	"platform.admin.system.configs.key.get":         {},
	"platform.admin.system.configs.key.put":         {},
	"platform.admin.system.configs.key.delete":      {},
	"platform.admin.system.backup.config.put":       {},
	"platform.admin.system.backup.post":             {},
	"platform.admin.system.backups.id.delete":       {},
	"platform.admin.system.backups.id.restore.post": {},
}
