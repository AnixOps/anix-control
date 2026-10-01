// Package platformcompat proves the platform package's native routes answer
// exactly as the kernel's legacy handlers, on SQLite and PostgreSQL: the same
// bytes (times the handlers take from their clock masked) and, where a route
// writes, the same resulting rows.
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
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/platform/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}

func route(method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy,
		Models: []any{&model.OperationLog{}, &model.BackupConfig{}, &model.BackupRecord{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}
			return service.Handlers()[routeID]
		},
	}
}

// system builds the legacy handler per request: its backup service keeps the
// configuration it read in memory, and the harness sets up a database per
// case.
func system(method func(*handler.SystemHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewSystemHandler(), c) }
}

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

func uintPtr(v uint) *uint { return &v }

func timePtr(v time.Time) *time.Time { return &v }

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

// seedBackups writes backup records: successful, failed and pending ones.
func seedBackups(t testing.TB, db *gorm.DB) {
	records := []model.BackupRecord{
		{ID: 1, Name: "backup_20260901_080000", Type: "database", Size: 1024, Path: "/var/lib/anix/backups/backup_20260901_080000.db", Status: 1,
			CreatedBy: uintPtr(1), CompletedAt: timePtr(seeded.Add(time.Minute)), CreatedAt: seeded, UpdatedAt: seeded.Add(time.Minute)},
		{ID: 2, Name: "backup_20260902_080000", Type: "full", Size: 4096, Path: "backups/backup_20260902_080000.zip", Status: 1, Auto: true,
			CompletedAt: timePtr(seeded.Add(24*time.Hour + time.Minute)), CreatedAt: seeded.Add(24 * time.Hour), UpdatedAt: seeded.Add(24 * time.Hour)},
		{ID: 3, Name: "backup_20260903_080000", Type: "files", Status: 2, Error: "disk full", CreatedBy: uintPtr(1),
			CreatedAt: seeded.Add(48 * time.Hour), UpdatedAt: seeded.Add(48 * time.Hour)},
		{ID: 4, Name: "backup_20260904_080000", Type: "database", Path: "backup_20260904_080000.db", Status: 0,
			CreatedAt: seeded.Add(72 * time.Hour), UpdatedAt: seeded.Add(72 * time.Hour)},
		{ID: 5, Name: "backup_20260831_080000", Type: "database", Size: 512, Path: "/srv/b/old.db", Status: 1,
			CompletedAt: timePtr(seeded.Add(-24 * time.Hour)), CreatedAt: seeded.Add(-24 * time.Hour), UpdatedAt: seeded.Add(-24 * time.Hour)},
	}
	require.NoError(t, db.Create(&records).Error)
	syncSequences(t, db)
}

// seedFailedBackups writes only backups that did not succeed.
func seedFailedBackups(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.Create(&[]model.BackupRecord{
		{ID: 1, Name: "b1", Type: "database", Size: 99, Status: 2, Error: "boom", CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, Name: "b2", Type: "database", Size: 7, Status: 0, CreatedAt: seeded.Add(time.Hour), UpdatedAt: seeded.Add(time.Hour)},
	}).Error)
	syncSequences(t, db)
}

// seedBackupConfig writes a backup configuration row.
func seedBackupConfig(cfg model.BackupConfig) func(t testing.TB, db *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		cfg.CreatedAt, cfg.UpdatedAt = seeded, seeded.Add(time.Hour)
		require.NoError(t, db.Create(&cfg).Error)
		// Create skips false booleans and zero numbers with a column default.
		require.NoError(t, db.Model(&model.BackupConfig{}).Where("id = ?", cfg.ID).UpdateColumns(map[string]any{
			"enabled": cfg.Enabled, "auto_backup": cfg.AutoBackup, "retention_days": cfg.RetentionDays,
			"backup_database": cfg.BackupDatabase, "backup_files": cfg.BackupFiles, "storage_type": cfg.StorageType,
		}).Error)
		syncSequences(t, db)
	}
}

// seedTwoBackupConfigs writes two rows; the handlers use the first.
func seedTwoBackupConfigs(t testing.TB, db *gorm.DB) {
	seedBackupConfig(model.BackupConfig{ID: 3, Enabled: true, Schedule: "interval:12", RetentionDays: 3, BackupDatabase: true, StorageType: "local", StoragePath: "/b3"})(t, db)
	seedBackupConfig(model.BackupConfig{ID: 2, Enabled: false, Schedule: "0 3 * * *", RetentionDays: 30, BackupDatabase: false, StorageType: "s3", StoragePath: "/b2"})(t, db)
}

// syncSequences moves PostgreSQL's id sequences past explicitly seeded ids,
// so both sides create the next row with the same id.
func syncSequences(t testing.TB, db *gorm.DB) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range []string{"v2_operation_log", "v2_backup_config", "v2_backup_record"} {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

// backupConfigs is the backup configuration after a request, without the
// times the handlers take from their clock.
func backupConfigs(t testing.TB, db *gorm.DB) any {
	var rows []struct {
		ID             uint
		Enabled        bool
		AutoBackup     bool
		Schedule       string
		RetentionDays  int
		BackupDatabase bool
		BackupFiles    bool
		StorageType    string
		StoragePath    string
		S3Bucket       string
		S3Region       string
		S3Endpoint     string
		S3AccessKey    string
		S3SecretKey    string
	}
	require.NoError(t, db.Model(&model.BackupConfig{}).Order("id").Find(&rows).Error)
	return rows
}

func read(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		c.Principal = admin
		packagecompat.RunRead(t, r, c)
	}
}

func write(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		c.Principal = admin
		c.Snapshot = backupConfigs
		packagecompat.RunWrite(t, r, c)
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

func TestBackupListRouteParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/system/backups", "platform.admin.system.backups.get", system((*handler.SystemHandler).ListBackups)), []packagecompat.Case{
		{Name: "newest first", Path: "/api/v2/admin/system/backups", Seed: seedBackups},
		{Name: "second page", Path: "/api/v2/admin/system/backups?page=2&page_size=2", Seed: seedBackups},
		{Name: "clamped page", Path: "/api/v2/admin/system/backups?page=-3&page_size=1000", Seed: seedBackups},
		{Name: "page size zero", Path: "/api/v2/admin/system/backups?page_size=0", Seed: seedBackups},
		{Name: "page that is not a number", Path: "/api/v2/admin/system/backups?page=abc", Seed: seedBackups},
		{Name: "no backups", Path: "/api/v2/admin/system/backups"},
	})
}

func TestBackupStatsRouteParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/system/backup/stats", "platform.admin.system.backup.stats.get", system((*handler.SystemHandler).GetBackupStats)), []packagecompat.Case{
		{Name: "successful backups", Path: "/api/v2/admin/system/backup/stats", Seed: seedBackups},
		{Name: "no successful backup", Path: "/api/v2/admin/system/backup/stats", Seed: seedFailedBackups},
		{Name: "no backups", Path: "/api/v2/admin/system/backup/stats"},
	})
}

func TestBackupConfigRouteParity(t *testing.T) {
	write(t, backupConfigRoute(t, "GET", native.BackupConfigGetRouteID, (*handler.SystemHandler).GetBackupConfig), []packagecompat.Case{
		{Name: "first read creates the defaults", Path: "/api/v2/admin/system/backup/config", Mask: []string{"data.created_at", "data.updated_at"}},
		{Name: "stored configuration with S3 keys masked", Path: "/api/v2/admin/system/backup/config", Seed: seedBackupConfig(model.BackupConfig{
			ID: 1, Enabled: true, AutoBackup: true, Schedule: "interval:6", RetentionDays: 14, BackupDatabase: true, BackupFiles: true,
			StorageType: "s3", StoragePath: "/srv/backups", S3Bucket: "bucket", S3Region: "eu-west-1", S3Endpoint: "https://s3.example.test",
			S3AccessKey: "AKIAEXAMPLE", S3SecretKey: "wJalrXUtnFEMI",
		})},
		{Name: "blank S3 keys and a cron schedule", Path: "/api/v2/admin/system/backup/config", Seed: seedBackupConfig(model.BackupConfig{
			ID: 1, Schedule: "0 3 * * *", RetentionDays: 0, StorageType: "local", S3AccessKey: "  ",
		})},
		{Name: "interval that is not a number", Path: "/api/v2/admin/system/backup/config", Seed: seedBackupConfig(model.BackupConfig{
			ID: 1, Schedule: " interval:x ", RetentionDays: 7, BackupDatabase: true, StorageType: "local",
		})},
		{Name: "the first of two rows", Path: "/api/v2/admin/system/backup/config", Seed: seedTwoBackupConfigs},
		{Name: "only the secret key set", Path: "/api/v2/admin/system/backup/config", Seed: seedBackupConfig(model.BackupConfig{
			ID: 4, RetentionDays: 7, BackupDatabase: true, StorageType: "s3", S3SecretKey: "only-secret",
		})},
		{Name: "a stored placeholder reads masked", Path: "/api/v2/admin/system/backup/config", Seed: seedBackupConfig(model.BackupConfig{
			ID: 1, RetentionDays: 7, StorageType: "s3", S3AccessKey: "********", S3SecretKey: "",
		})},
	})
}
