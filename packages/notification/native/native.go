// Package native implements the notification package's v2 routes in the
// package itself, on the kernel's notification and Telegram tables adopted in
// place (kernel.storage.adopt), and the e-mail configuration through the
// kernel's KernelSettings contract. Legacy handlers and native routes share
// the tables, so a route can switch between them at any time. Responses are
// byte-compatible with the legacy handlers (internal/tests/notificationcompat).
//
// The e-mail configuration (GET and PUT) and the test send read and write
// the notification.email.config key of the kernel's v2_system_config
// through KernelSettings, namespace mail. Its value holds the SMTP password,
// a secret: the package reads it in clear (kernel.settings.mail.secrets.v1)
// because it answers it to administrators, as the kernel's handler does,
// and sends the test e-mail itself. A host without the contract leaves the
// three routes legacy.
//
// Two routes have no native handler and stay bridged: setting the Telegram
// webhook derives its default URL from the request host, which package
// hosts are not sent; and the public Telegram webhook answers bot commands
// from subscriber data (the subscription token) that no kernel view exposes.
package native

import (
	"context"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"gorm.io/gorm"
)

// controlName is the kernel's branding.ControlName.
const controlName = "AnixOps Control"

// Pagination limits of the kernel's handler.ClampPagination.
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// tables and the kapi_user_directory_v1 view are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Settings is the kernel's KernelSettings; without it the e-mail
	// configuration and test send have no native handler and stay legacy.
	Settings Settings
	// NewToken names a request that carries neither an Idempotency-Key
	// nor a request id; it defaults to a random UUID.
	NewToken func() string
	// Now defaults to time.Now.
	Now func() time.Time
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{
		"notification.admin.notification.logs.get":            s.AdminLogs,
		"notification.admin.notification.templates.get":       s.AdminTemplates,
		"notification.admin.notification.templates.post":      s.AdminCreateTemplate,
		"notification.admin.notification.templates.id.put":    s.AdminUpdateTemplate,
		"notification.admin.notification.templates.id.delete": s.AdminDeleteTemplate,
		"notification.admin.telegram.bot.get":                 s.AdminGetBot,
		"notification.admin.telegram.bot.put":                 s.AdminUpdateBot,
		"notification.admin.telegram.webhook.delete":          s.AdminDeleteWebhook,
		"notification.admin.telegram.notify.post":             s.AdminTelegramNotify,
		"notification.admin.telegram.broadcast.post":          s.AdminTelegramBroadcast,
		"notification.admin.telegram.users.get":               s.AdminBindings,
		"notification.admin.telegram.users.id.notify.put":     s.AdminUpdateBindingNotify,
		"notification.user.notifications.get":                 s.UserNotifications,
		"notification.user.notifications.id.read.post":        s.UserMarkRead,
		"notification.user.notifications.read_all.post":       s.UserMarkAllRead,
		"notification.user.notifications.unread_count.get":    s.UserUnreadCount,
		"notification.user.telegram.status.get":               s.UserTelegramStatus,
		"notification.user.telegram.unbind.post":              s.UserTelegramUnbind,
		"notification.user.telegram.notify.post":              s.UserTelegramNotify,
	}
	if s.Settings != nil {
		handlers[EmailConfigGetRouteID] = s.AdminEmailConfig
		handlers[EmailConfigPutRouteID] = s.AdminUpdateEmailConfig
		handlers[TestSendRouteID] = s.AdminTestNotification
	}
	return handlers
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) panel(data any) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(data, s.now()))
}

func (s *Service) panelError(message string) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelError(message, s.now()))
}

// actor is the caller's user id; the kernel authenticated the request.
func actor(request pluginhostsdk.NativeRequest) uint {
	return request.Principal.ActorID
}

// getQuery is gin's Context.GetQuery on the forwarded query string.
func getQuery(request pluginhostsdk.NativeRequest, key string) (string, bool) {
	if values, ok := request.Metadata.Query[key]; ok && len(values) > 0 {
		return values[0], true
	}
	return "", false
}

// query is gin's Context.Query.
func query(request pluginhostsdk.NativeRequest, key string) string {
	value, _ := getQuery(request, key)
	return value
}

// defaultQuery is gin's Context.DefaultQuery.
func defaultQuery(request pluginhostsdk.NativeRequest, key, fallback string) string {
	if value, ok := getQuery(request, key); ok {
		return value
	}
	return fallback
}

// pagination reads page and page_size as the legacy handlers do: a value
// that does not parse counts as what strconv.Atoi returns.
func pagination(request pluginhostsdk.NativeRequest) (int, int) {
	page, _ := strconv.Atoi(defaultQuery(request, "page", "1"))
	pageSize, _ := strconv.Atoi(defaultQuery(request, "page_size", "20"))
	return page, pageSize
}

// clampPagination is the kernel's handler.ClampPagination.
func clampPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}
