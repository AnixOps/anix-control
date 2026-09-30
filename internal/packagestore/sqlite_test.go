package packagestore

import (
	"context"
	"path/filepath"
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
