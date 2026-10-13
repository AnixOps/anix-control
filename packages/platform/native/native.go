// Package native implements the platform package's route in the package
// itself: the system audit log through the kapi_system_audit_log_v1 kernel
// view. Legacy handler and native route share the rows, so the route can
// switch between them at any time. Responses are byte-compatible with the
// legacy handler (internal/tests/platformcompat).
//
// The system configuration routes (list, get, set, delete) have no native
// handler and stay bridged: they work on any key of v2_system_config, a
// protected kernel table. KernelSettings reaches only the keys of a
// namespace, and a grant over every key would amount to every secret: the
// answers mask secrets, but writing an address (the NodeX or SMTP host) next
// to a secret the kernel sends there amounts to reading it. Their reason is
// in docs/architecture/settings-service.md.
//
// The backup routes the package once served (the backup list, statistics and
// configuration) are gone with the backup settings nothing executed: the
// backups are the kernel's /api/v5/backups (docs/reference/backups.md).
package native

import (
	"context"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"gorm.io/gorm"
)

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the
	// kapi_system_audit_log_v1 view is visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Now defaults to time.Now.
	Now func() time.Time
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	return map[string]pluginhostsdk.NativeHandler{
		"platform.admin.system.audit_logs.get": s.GetAuditLogs,
	}
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
