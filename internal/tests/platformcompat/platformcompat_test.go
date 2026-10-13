// Package platformcompat proves the platform package's native route answers
// exactly as the kernel's legacy handler, on SQLite and PostgreSQL: the same
// bytes. The backup routes it once compared are gone (the backups are
// /api/v5/backups); the audit log entries they wrote stay in the seed as
// data.
//
// The legacy audit log handler reads v2_operation_log; the native one reads
// the kernel view kapi_system_audit_log_v1 over it, so every audit case also
// proves the view shows exactly the rows the handler filters on.
package platformcompat

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/internal/handler"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/packagestore"
	"github.com/AnixOps/anix-control/v5/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v5/packages/platform/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}

func route(method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy,
		Models: []any{&model.OperationLog{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}
			return service.Handlers()[routeID]
		},
	}
}

// system builds the legacy handler per request: the harness sets up a
// database per case.
func system(method func(*handler.SystemHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewSystemHandler(), c) }
}

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

func uintPtr(v uint) *uint { return &v }

// seedViews creates the kernel views; the harness has migrated the tables.
func seedViews(t testing.TB, db *gorm.DB) {
	require.NoError(t, packagestore.EnsureKernelAPIViews(db))
}

// seedAudit writes an audit trail of module "system" with content the
// handlers redact, and entries of other modules the list leaves out.
func seedAudit(t testing.TB, db *gorm.DB) {
	seedViews(t, db)
	entries := []model.OperationLog{
		{ID: 1, UserID: uintPtr(1), Username: "admin@example.test", Action: "create", Module: "system", TargetType: "system_config", TargetID: uintPtr(4),
			Content: `{"group":"site","has_value":true,"key":"site.name","preserve_existing":false,"sensitive":false,"type":"string"}`, IP: "192.0.2.10", UserAgent: "ua/1", Status: 1},
		{ID: 2, UserID: uintPtr(1), Username: "admin@example.test", Action: "update", Module: "system", TargetType: "backup_config", TargetID: uintPtr(1),
			Content: `{"enabled":true,"s3_access_key_has_value":true,"s3_secret_key_has_value":false,"preserved_sensitive_fields":["s3_access_key"],"storage_type":"s3"}`, Status: 1},
		{ID: 3, Username: "ops@example.test", Action: "update", Module: "system", TargetType: "system_config",
			Content: `{"key":"smtp.password","nested":{"api_token":"abcdefghijkl","short_secret":"abc","note":"<ok>"},"credential":42}`, Status: 2},
		{ID: 4, Action: "delete", Module: "system", TargetType: "backup_record", TargetID: uintPtr(9), Content: "password=hunter2, token: abc; user=bob", Status: 1},
		{ID: 5, Action: "restore", Module: "system", TargetType: "backup_record", Content: `not json "token": "abc123" "Secret":"s3cr3t"`, Status: 1},
		{ID: 6, Action: "update", Module: "system", TargetType: "system_config", Content: "   ", Status: 1},
		{ID: 7, Action: "update", Module: "system", TargetType: "system_config", Content: `[{"token":"abc"}]`, Status: 1},
		{ID: 8, Action: "update", Module: "system", TargetType: "system_config", Content: "null", Status: 1},
		{ID: 9, UserID: uintPtr(2), Action: "login", Module: "user", TargetType: "user", Content: `{"token":"abcdefghijkl"}`, Status: 1},
		{ID: 10, Action: "update", Module: "System", TargetType: "system_config", Content: "{}", Status: 1},
	}
	for i := range entries {
		entries[i].CreatedAt = seeded.Add(time.Duration(i) * time.Minute)
	}
	require.NoError(t, db.Create(&entries).Error)
	syncSequences(t, db)
}

// syncSequences moves PostgreSQL's id sequences past explicitly seeded ids,
// so both sides create the next row with the same id.
func syncSequences(t testing.TB, db *gorm.DB) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range []string{"v2_operation_log"} {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

func read(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		c.Principal = admin
		packagecompat.RunRead(t, r, c)
	}
}

func TestAuditLogRouteParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/system/audit-logs", "platform.admin.system.audit_logs.get", system((*handler.SystemHandler).GetAuditLogs)), []packagecompat.Case{
		{Name: "system entries newest first, redacted", Path: "/api/v2/admin/system/audit-logs", Seed: seedAudit},
		{Name: "by target type", Path: "/api/v2/admin/system/audit-logs?target_type=system_config", Seed: seedAudit},
		{Name: "by action", Path: "/api/v2/admin/system/audit-logs?action=update", Seed: seedAudit},
		{Name: "by target type and action, trimmed", Path: "/api/v2/admin/system/audit-logs?target_type=%20backup_record%20&action=delete", Seed: seedAudit},
		{Name: "another module's action", Path: "/api/v2/admin/system/audit-logs?action=login", Seed: seedAudit},
		{Name: "second page", Path: "/api/v2/admin/system/audit-logs?page=2&page_size=3", Seed: seedAudit},
		{Name: "page past the end", Path: "/api/v2/admin/system/audit-logs?page=9&page_size=3", Seed: seedAudit},
		{Name: "page size above the limit", Path: "/api/v2/admin/system/audit-logs?page_size=500", Seed: seedAudit},
		{Name: "page and page size below one", Path: "/api/v2/admin/system/audit-logs?page=0&page_size=-1", Seed: seedAudit},
		{Name: "page that is not a number", Path: "/api/v2/admin/system/audit-logs?page=x&page_size=y", Seed: seedAudit},
		{Name: "no entries", Path: "/api/v2/admin/system/audit-logs", Seed: seedViews},
	})
}
