package packagestore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openSQLiteKernel(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kernel.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PackageStorage{}, &model.User{}))
	require.NoError(t, EnsureKernelAPIViews(db))
	require.NoError(t, EnsureKernelAPIViews(db), "views are created once")
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db, path
}

func TestSQLiteLeaseSharesTheKernelFileWithATablePrefix(t *testing.T) {
	db, path := openSQLiteKernel(t)
	require.NoError(t, db.Exec("CREATE TABLE v2_knowledge (id INTEGER PRIMARY KEY)").Error)
	now := time.Unix(1_790_000_000, 0)
	store := Store{DB: db, Driver: "sqlite3", DSN: path, Now: func() time.Time { return now }}
	holder := Holder{PackageID: "knowledge", Version: "4.1.0", Generation: 7}
	grants := Grants{Storage: true, AdoptTables: []string{"v2_knowledge", "v2_knowledge"}, Views: []string{"kapi_user_directory_v1"}}

	lease, err := store.Lease(context.Background(), holder, grants)
	require.NoError(t, err)
	require.Equal(t, Lease{
		Driver: DriverSQLite, DSN: path, TablePrefix: "pkg_knowledge_", LeaseGeneration: 1,
		AdoptedTables: []string{"v2_knowledge"}, Views: []string{"kapi_user_directory_v1"},
	}, lease)

	lease, err = store.Lease(context.Background(), holder, grants)
	require.NoError(t, err)
	require.EqualValues(t, 2, lease.LeaseGeneration)
	var row model.PackageStorage
	require.NoError(t, db.First(&row, "package_id = ?", "knowledge").Error)
	require.Equal(t, DriverSQLite, row.Driver)
	require.Equal(t, "4.1.0", row.PackageVersion)
	require.EqualValues(t, 7, row.HostGeneration)
	require.JSONEq(t, `{"adopt_tables":["v2_knowledge"],"views":["kapi_user_directory_v1"]}`, row.GrantsJSON)

	var columns []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('kapi_user_directory_v1') ORDER BY cid").Scan(&columns).Error)
	require.Equal(t, []string{"id", "email", "is_admin", "is_staff", "banned", "plan_id", "group_id", "expired_at", "created_at"}, columns)
}

// kapi_system_audit_log_v1 shows only the system audit trail, and a view
// whose source table does not exist yet is left out.
func TestSystemAuditLogViewShowsOnlyTheSystemModule(t *testing.T) {
	db, _ := openSQLiteKernel(t)
	exists, err := viewExists(db, "kapi_system_audit_log_v1")
	require.NoError(t, err)
	require.False(t, exists, "no view without v2_operation_log")

	require.NoError(t, db.AutoMigrate(&model.OperationLog{}))
	require.NoError(t, db.Create(&[]model.OperationLog{
		{Module: "system", Action: "update", TargetType: "system_config", Content: `{"key":"site.name"}`},
		{Module: "user", Action: "login"},
	}).Error)
	require.NoError(t, EnsureKernelAPIViews(db))

	var columns []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('kapi_system_audit_log_v1') ORDER BY cid").Scan(&columns).Error)
	require.Equal(t, []string{"id", "user_id", "username", "action", "module", "target_type", "target_id", "content", "ip", "user_agent", "status", "created_at"}, columns)
	var modules []string
	require.NoError(t, db.Raw("SELECT module FROM kapi_system_audit_log_v1").Scan(&modules).Error)
	require.Equal(t, []string{"system"}, modules)
}

// Every view names the table it reads, or EnsureKernelAPIViews leaves it
// out; a view that filters rows is a RowFilter view, so PostgreSQL makes it
// a security barrier.
func TestKernelAPIViewsDeclareSourceAndRowFilter(t *testing.T) {
	for _, view := range KernelAPIViews {
		require.NotEmpty(t, view.Source, view.Name)
		require.Contains(t, view.Query, "FROM "+view.Source, view.Name)
		require.Equal(t, strings.Contains(strings.ToUpper(view.Query), " WHERE "), view.RowFilter, view.Name)
	}
}

func TestLeaseRejectsUndeclaredOrUnknownStorage(t *testing.T) {
	db, path := openSQLiteKernel(t)
	store := Store{DB: db, Driver: "sqlite", DSN: path}
	ctx := context.Background()

	_, err := store.Lease(ctx, Holder{PackageID: "knowledge", Version: "4.1.0", Generation: 1}, Grants{})
	require.ErrorIs(t, err, ErrStorageNotDeclared)
	for _, id := range []string{"Knowledge", "knowledge_base", "1knowledge", "a.b", ""} {
		_, err = store.Lease(ctx, Holder{PackageID: id, Version: "4.1.0", Generation: 1}, Grants{Storage: true})
		require.ErrorIs(t, err, ErrPackageNotEligible, id)
	}
	_, err = store.Lease(ctx, Holder{PackageID: "knowledge", Version: "4.1.0", Generation: 1}, Grants{Storage: true, AdoptTables: []string{"v2_missing"}})
	require.ErrorIs(t, err, ErrGrantTargetMissing)
	_, err = Store{DB: db, Driver: "mysql"}.Lease(ctx, Holder{PackageID: "knowledge", Version: "4.1.0", Generation: 1}, Grants{Storage: true})
	require.ErrorContains(t, err, "mysql")

	var count int64
	require.NoError(t, db.Model(&model.PackageStorage{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestStorageNames(t *testing.T) {
	require.Equal(t, "anix_pkg_identity_platform", RoleName("identity-platform"))
	require.Equal(t, "pkg_identity_platform", SchemaName("identity-platform"))
	require.Equal(t, "pkg_identity_platform_", TablePrefix("identity-platform"))
}

func TestLeaseCacheSharesOneLeasePerGeneration(t *testing.T) {
	db, path := openSQLiteKernel(t)
	store := Store{DB: db, Driver: "sqlite", DSN: path, Leases: NewLeaseCache()}
	ctx := context.Background()
	holder := Holder{PackageID: "knowledge", Version: "4.1.0", Generation: 3}
	first, err := store.Lease(ctx, holder, Grants{Storage: true})
	require.NoError(t, err)
	second, err := store.Lease(ctx, holder, Grants{Storage: true})
	require.NoError(t, err)
	require.Equal(t, first, second, "replicas of one generation share the lease")

	holder.Generation = 4
	next, err := store.Lease(ctx, holder, Grants{Storage: true})
	require.NoError(t, err)
	require.EqualValues(t, 2, next.LeaseGeneration, "a new generation leases again")

	_, err = store.Lease(ctx, Holder{PackageID: "knowledge", Version: "4.1.0", Generation: 4, Remote: true}, Grants{Storage: true})
	require.ErrorContains(t, err, "need PostgreSQL")
}
