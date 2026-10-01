// Package native implements the platform package's read routes in the
// package itself: the backup configuration, list and statistics on the
// kernel's v2_backup_config and v2_backup_record tables adopted in place
// (kernel.storage.adopt), and the system audit log through the
// kapi_system_audit_log_v1 kernel view. Legacy handlers and native routes
// share the rows, so a route can switch between them at any time. Responses
// are byte-compatible with the legacy handlers
// (internal/tests/platformcompat).
//
// Eight routes have no native handler and stay bridged:
//   - The system configuration routes (list, get, set, delete) work on
//     v2_system_config, a protected kernel table that no package may adopt.
//     Its values include secrets (the single-key answer returns them in
//     clear), so no kernel view may expose it either, and every write also
//     records an entry in the protected v2_operation_log audit trail.
//   - Updating the backup configuration records an audit entry in
//     v2_operation_log as well, and it refreshes the copy of the
//     configuration the kernel's backup service keeps in memory, from which
//     the kernel's backup creation takes its storage path. A native write
//     would leave that copy stale.
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
	// Now defaults to time.Now.
	Now func() time.Time
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	return map[string]pluginhostsdk.NativeHandler{
		"platform.admin.system.audit_logs.get":    s.GetAuditLogs,
		"platform.admin.system.backup.config.get": s.GetBackupConfig,
		"platform.admin.system.backups.get":       s.ListBackups,
		"platform.admin.system.backup.stats.get":  s.GetBackupStats,
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
