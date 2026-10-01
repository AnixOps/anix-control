package main

import (
	"context"
	"log"

	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/notification/native"
	"gorm.io/gorm"
)

// notificationBridge is what the notification host needs from the package
// bridge.
type notificationBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newNotificationService returns the notification host's router. The routes
// in notificationRoutes have a native handler on the adopted notification and
// Telegram tables; such a route serves natively once the kernel sets its
// mode, and falls back to the legacy handler otherwise. The routes in
// bridgedRoutes always relay to the legacy handler.
func newNotificationService(bridge notificationBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "notification", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := notificationRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute
		},
		Native: service.Handlers(),
	})
}

// notificationRoutes are the package's compatibility routes with a native
// handler.
var notificationRoutes = map[string]struct{}{
	"notification.admin.notification.logs.get":            {},
	"notification.admin.notification.templates.get":       {},
	"notification.admin.notification.templates.post":      {},
	"notification.admin.notification.templates.id.put":    {},
	"notification.admin.notification.templates.id.delete": {},
	"notification.admin.telegram.bot.get":                 {},
	"notification.admin.telegram.bot.put":                 {},
	"notification.admin.telegram.webhook.delete":          {},
	"notification.admin.telegram.notify.post":             {},
	"notification.admin.telegram.broadcast.post":          {},
	"notification.admin.telegram.users.get":               {},
	"notification.admin.telegram.users.id.notify.put":     {},
	"notification.user.notifications.get":                 {},
	"notification.user.notifications.id.read.post":        {},
	"notification.user.notifications.read_all.post":       {},
	"notification.user.notifications.unread_count.get":    {},
	"notification.user.telegram.status.get":               {},
	"notification.user.telegram.unbind.post":              {},
	"notification.user.telegram.notify.post":              {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler.
//   - The e-mail configuration and the test send keep their settings in the
//     kernel's v2_system_config, which the package does not own.
//   - Setting the webhook defaults its URL to the request's scheme and host,
//     which the kernel does not send to package hosts.
//   - The public Telegram webhook answers /sub with the member's
//     subscription token, which no kernel view exposes.
var bridgedRoutes = map[string]struct{}{
	"notification.admin.notification.email.config.get": {},
	"notification.admin.notification.email.config.put": {},
	"notification.admin.notification.test.post":        {},
	"notification.admin.telegram.webhook.post":         {},
	"notification.telegram.webhook.post":               {},
}
