// Package native implements the platform package's routes in the package
// itself: the backup list and statistics on the kernel's v2_backup_record
// table adopted in place (kernel.storage.adopt), the system audit log
// through the kapi_system_audit_log_v1 kernel view, and the backup
// configuration, read and updated through the kernel's KernelSettings
// contract (namespace backup, kernel.settings.backup.read.v1 and
// kernel.settings.backup.write.v1). Legacy handlers and native routes share
// the rows, so a route can switch between them at any time. Responses are
// byte-compatible with the legacy handlers (internal/tests/platformcompat).
//
// The package never sees the S3 keys: it holds no secrets capability, so
// KernelSettings answers them masked, as the kernel's handler shows them,
// and it has no access to the v2_backup_config row. The update is written
// by the kernel: it records the audit entry its own handler records, in
// the protected v2_operation_log, and makes its backup service reload the
// copy of the configuration it keeps in memory, from which backup creation
// takes its storage path.
//
// Seven routes have no native handler and stay bridged:
//   - The system configuration routes (list, get, set, delete) work on any
//     key of v2_system_config, a protected kernel table. KernelSettings
//     reaches only the keys of a namespace, and a grant over every key
//     would amount to every secret: the answers mask secrets, but writing
//     an address (the NodeX or SMTP host) next to a secret the kernel sends
//     there amounts to reading it. Their reason is in
//     docs/architecture/settings-service.md.
//   - Creating, deleting and restoring a backup write, remove and restore
//     archives of the whole database and the kernel's files on the kernel's
//     disk (restoring closes the kernel's database connection), and record
//     audit entries: kernel operations with no table-level equivalent.
package native

import (
	"context"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"gorm.io/gorm"
)

// Pagination limits of the kernel's handler.ClampPagination.
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// tables and the kapi_system_audit_log_v1 view are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Settings is the kernel's KernelSettings; without it the backup
	// configuration routes have no native handler and stay legacy.
	Settings Settings
	// NewToken names a request that carries neither an Idempotency-Key
	// nor a request id; it defaults to a random UUID.
	NewToken func() string
	// Now defaults to time.Now.
	Now func() time.Time
}

// Route ids of the routes that read and update the backup configuration
// through KernelSettings.
const (
	BackupConfigGetRouteID    = "platform.admin.system.backup.config.get"
	BackupConfigUpdateRouteID = "platform.admin.system.backup.config.put"
)

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{
		"platform.admin.system.audit_logs.get":   s.GetAuditLogs,
		"platform.admin.system.backups.get":      s.ListBackups,
		"platform.admin.system.backup.stats.get": s.GetBackupStats,
	}
	if s.Settings != nil {
		handlers[BackupConfigGetRouteID] = s.GetBackupConfig
		handlers[BackupConfigUpdateRouteID] = s.UpdateBackupConfig
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
