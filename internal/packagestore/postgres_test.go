package packagestore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openPostgresKernel connects as the kernel role from ANIX_TEST_POSTGRES_DSN.
// The role should be a non-superuser with CREATEROLE, as in production.
func openPostgresKernel(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	db := openPostgres(t, dsn)
	var databaseName string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	require.NoError(t, db.AutoMigrate(&model.PackageStorage{}, &model.User{}))
	require.NoError(t, EnsureKernelAPIViews(db))
	return db, dsn
}

func openPostgres(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 4)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	return hex.EncodeToString(raw)
}

// dropPackageStorage removes what a lease provisioned. The kernel owns the
// package schema, so it can drop it with the package's tables inside.
func dropPackageStorage(t *testing.T, db *gorm.DB, packageID string) {
	t.Helper()
	role := quoteIdent(RoleName(packageID))
	for _, statement := range []string{
		"DROP SCHEMA IF EXISTS " + quoteIdent(SchemaName(packageID)) + " CASCADE",
		"REVOKE ALL ON ALL TABLES IN SCHEMA public FROM " + role,
		"REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM " + role,
		"DROP ROLE IF EXISTS " + role,
	} {
		if exists, _ := roleExists(db, RoleName(packageID)); !exists && !strings.HasPrefix(statement, "DROP SCHEMA") {
			continue
		}
		require.NoError(t, db.Exec(statement).Error, statement)
	}
	require.NoError(t, db.Delete(&model.PackageStorage{}, "package_id = ?", packageID).Error)
}

func requirePermissionDenied(t *testing.T, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a PostgreSQL error, got %v", err)
	require.Equal(t, "42501", pgErr.Code, pgErr.Message)
}

func TestPostgresLeaseProvisionsALeastPrivilegeRole(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	suffix := randomSuffix(t)
	packageID := "pkgtest-" + suffix
	adopted := "v2_pkgtest_" + suffix
	require.NoError(t, kernel.Exec("CREATE TABLE "+quoteIdent(adopted)+" (id BIGSERIAL PRIMARY KEY, title TEXT NOT NULL)").Error)
	t.Cleanup(func() { _ = kernel.Exec("DROP TABLE IF EXISTS " + quoteIdent(adopted)).Error })
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	require.NoError(t, kernel.Create(&model.User{Email: "lease-" + suffix + "@example.com", Password: "hash", Token: "token-" + suffix, UUID: "uuid-" + suffix}).Error)
	t.Cleanup(func() { _ = kernel.Exec("DELETE FROM v2_user WHERE email = ?", "lease-"+suffix+"@example.com").Error })

	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	holder := Holder{PackageID: packageID, Version: "4.1.0", Generation: 3}
	ctx := context.Background()
	lease, err := store.Lease(ctx, holder, Grants{Storage: true, AdoptTables: []string{adopted}, Views: []string{"kapi_user_directory_v1"}})
	require.NoError(t, err)
	require.Equal(t, DriverPostgres, lease.Driver)
	require.Equal(t, SchemaName(packageID), lease.Schema)
	require.EqualValues(t, 1, lease.LeaseGeneration)
	require.NotContains(t, lease.DSN, "kernel")

	pkg := openPostgres(t, lease.DSN)
	var identity struct {
		UserName      string
		CurrentSchema string
	}
	require.NoError(t, pkg.Raw("SELECT current_user AS user_name, current_schema() AS current_schema").Scan(&identity).Error)
	require.Equal(t, RoleName(packageID), identity.UserName)
	require.Equal(t, SchemaName(packageID), identity.CurrentSchema)

	// Own schema: create and use tables.
	require.NoError(t, pkg.Exec("CREATE TABLE notes (id SERIAL PRIMARY KEY, body TEXT)").Error)
	require.NoError(t, pkg.Exec("INSERT INTO notes (body) VALUES ('hello')").Error)
	var schema string
	require.NoError(t, pkg.Raw("SELECT schemaname FROM pg_tables WHERE tablename = 'notes'").Scan(&schema).Error)
	require.Equal(t, SchemaName(packageID), schema)

	// Adopted table: full DML including its sequence.
	require.NoError(t, pkg.Exec("INSERT INTO "+quoteIdent(adopted)+" (title) VALUES ('first')").Error)
	require.NoError(t, pkg.Exec("UPDATE "+quoteIdent(adopted)+" SET title = 'renamed'").Error)
	var titles []string
	require.NoError(t, pkg.Raw("SELECT title FROM "+quoteIdent(adopted)).Scan(&titles).Error)
	require.Equal(t, []string{"renamed"}, titles)
	require.NoError(t, pkg.Exec("DELETE FROM "+quoteIdent(adopted)).Error)
	requirePermissionDenied(t, pkg.Exec("TRUNCATE "+quoteIdent(adopted)).Error)
	requirePermissionDenied(t, pkg.Exec("ALTER TABLE "+quoteIdent(adopted)+" ADD COLUMN extra TEXT").Error)

	// Views are readable; their base tables and kernel tables are not.
	var emails []string
	require.NoError(t, pkg.Raw("SELECT email FROM kapi_user_directory_v1 WHERE email = ?", "lease-"+suffix+"@example.com").Scan(&emails).Error)
	require.Len(t, emails, 1)
	require.ErrorContains(t, pkg.Raw("SELECT password FROM kapi_user_directory_v1").Scan(&emails).Error, "does not exist")
	var count int64
	requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM v2_user").Scan(&count).Error)
	requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM v4_kernel_package_storage").Scan(&count).Error)
	requirePermissionDenied(t, pkg.Exec("CREATE TABLE public.escape (id INT)").Error)
	requirePermissionDenied(t, pkg.Exec("CREATE ROLE sneaky").Error)

	var attributes struct {
		ConnLimit int
		Inherit   bool
	}
	require.NoError(t, kernel.Raw("SELECT rolconnlimit AS conn_limit, rolinherit AS inherit FROM pg_roles WHERE rolname = ?", RoleName(packageID)).Scan(&attributes).Error)
	require.Equal(t, ConnectionLimit, attributes.ConnLimit)
	require.False(t, attributes.Inherit)

	// A new lease without the adopted table revokes it at once, even for an
	// open connection, and rotates the password.
	next, err := store.Lease(ctx, holder, Grants{Storage: true})
	require.NoError(t, err)
	require.EqualValues(t, 2, next.LeaseGeneration)
	requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM "+quoteIdent(adopted)).Scan(&count).Error)
	requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM kapi_user_directory_v1").Scan(&count).Error)
	require.NoError(t, pkg.Raw("SELECT count(*) FROM notes").Scan(&count).Error, "own tables survive a new lease")
	require.EqualValues(t, 1, count)

	stale, err := gorm.Open(postgres.Open(lease.DSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err == nil {
		err = stale.Exec("SELECT 1").Error
		if sqlDB, dbErr := stale.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	}
	require.Error(t, err, "the previous lease's password no longer works")
	fresh := openPostgres(t, next.DSN)
	require.NoError(t, fresh.Exec("SELECT 1").Error)
}

// A role granted kapi_system_audit_log_v1 reads the system audit trail and
// nothing else of v2_operation_log, and cannot change it through the view.
func TestPostgresSystemAuditLogViewIsReadOnly(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	require.NoError(t, kernel.AutoMigrate(&model.OperationLog{}))
	// Views are immutable once created; recreate this one so the test sees
	// the current definition.
	require.NoError(t, kernel.Exec("DROP VIEW IF EXISTS kapi_system_audit_log_v1").Error)
	require.NoError(t, EnsureKernelAPIViews(kernel))
	suffix := randomSuffix(t)
	packageID := "pkgtest-" + suffix
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	action := "audit-" + suffix
	require.NoError(t, kernel.Create(&[]model.OperationLog{{Module: "system", Action: action}, {Module: "user", Action: action}}).Error)
	t.Cleanup(func() { _ = kernel.Exec("DELETE FROM v2_operation_log WHERE action = ?", action).Error })

	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	lease, err := store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
		Grants{Storage: true, Views: []string{"kapi_system_audit_log_v1"}})
	require.NoError(t, err)
	pkg := openPostgres(t, lease.DSN)
	var modules []string
	require.NoError(t, pkg.Raw("SELECT module FROM kapi_system_audit_log_v1 WHERE action = ?", action).Scan(&modules).Error)
	require.Equal(t, []string{"system"}, modules)
	var count int64
	requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM v2_operation_log").Scan(&count).Error)
	requirePermissionDenied(t, pkg.Exec("DELETE FROM kapi_system_audit_log_v1 WHERE action = ?", action).Error)
	requirePermissionDenied(t, pkg.Exec("UPDATE kapi_system_audit_log_v1 SET content = '' WHERE action = ?", action).Error)

	// A cheap function of the package's own records every row it is shown.
	// The view is a security barrier, so it sees only the system rows, even
	// with the planner told to run it before the view's filter.
	require.NoError(t, pkg.Exec("CREATE TABLE seen (module text)").Error)
	require.NoError(t, pkg.Exec(`CREATE FUNCTION peek(m text) RETURNS boolean LANGUAGE plpgsql COST 0.0000001 AS $$
		BEGIN INSERT INTO seen VALUES (m); RETURN true; END $$`).Error)
	var seen []string
	require.NoError(t, pkg.Transaction(func(tx *gorm.DB) error {
		for _, setting := range []string{"enable_indexscan", "enable_bitmapscan", "enable_indexonlyscan"} {
			if err := tx.Exec("SET LOCAL " + setting + " = off").Error; err != nil {
				return err
			}
		}
		if err := tx.Raw("SELECT module FROM kapi_system_audit_log_v1 WHERE peek(module)").Scan(&modules).Error; err != nil {
			return err
		}
		return tx.Raw("SELECT DISTINCT module FROM seen").Scan(&seen).Error
	}))
	require.Equal(t, []string{"system"}, seen)
}

// A row-filtering view created before it was a security barrier becomes one.
func TestPostgresExistingRowFilterViewBecomesASecurityBarrier(t *testing.T) {
	kernel, _ := openPostgresKernel(t)
	require.NoError(t, kernel.AutoMigrate(&model.OperationLog{}))
	require.NoError(t, kernel.Exec("DROP VIEW IF EXISTS kapi_system_audit_log_v1").Error)
	require.NoError(t, kernel.Exec("CREATE VIEW kapi_system_audit_log_v1 AS SELECT id FROM v2_operation_log WHERE module = 'system'").Error)
	t.Cleanup(func() {
		_ = kernel.Exec("DROP VIEW IF EXISTS kapi_system_audit_log_v1").Error
		_ = EnsureKernelAPIViews(kernel)
	})

	require.NoError(t, EnsureKernelAPIViews(kernel))
	var options []string
	require.NoError(t, kernel.Raw("SELECT unnest(reloptions) FROM pg_class WHERE relname = 'kapi_system_audit_log_v1' AND relnamespace = current_schema()::regnamespace").Scan(&options).Error)
	require.Contains(t, options, "security_barrier=true")
}

func TestPostgresLeaseFailsWithoutPartialState(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	packageID := "pkgtest-" + randomSuffix(t)
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}

	_, err := store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
		Grants{Storage: true, AdoptTables: []string{"v2_missing_" + randomSuffix(t)}})
	require.ErrorIs(t, err, ErrGrantTargetMissing)
	exists, err := roleExists(kernel, RoleName(packageID))
	require.NoError(t, err)
	require.False(t, exists, "the role is created in the same transaction")

	_, err = store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
		Grants{Storage: true, AdoptTables: []string{"v2_user"}, Views: []string{"v2_user"}})
	require.ErrorIs(t, err, ErrGrantTargetMissing, "a table is not a view")
}

func TestPostgresLeaseRequiresCreateRole(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	var canCreate bool
	require.NoError(t, kernel.Raw("SELECT rolcreaterole OR rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&canCreate).Error)
	if !canCreate {
		t.Skip("the test kernel role cannot create roles")
	}
	suffix := randomSuffix(t)
	limited := "pkgtest_limited_" + suffix
	require.NoError(t, kernel.Exec("CREATE ROLE "+quoteIdent(limited)+" LOGIN PASSWORD 'limited'").Error)
	t.Cleanup(func() { _ = kernel.Exec("DROP ROLE IF EXISTS " + quoteIdent(limited)).Error })
	limitedDSN, err := packageDSN(kernelDSN, limited, "limited", "pkgtest")
	require.NoError(t, err)
	limitedDB := openPostgres(t, limitedDSN)

	_, err = Store{DB: limitedDB, Driver: "postgres", DSN: limitedDSN}.Lease(context.Background(),
		Holder{PackageID: "pkgtest-" + suffix, Version: "4.1.0", Generation: 1}, Grants{Storage: true})
	require.ErrorIs(t, err, ErrCreateRoleRequired)
	require.ErrorContains(t, err, "ALTER ROLE \""+limited+"\" CREATEROLE")
}

func TestPostgresLeaseResetsOrRefusesAnExistingRole(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	packageID := "pkgtest-" + randomSuffix(t)
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	require.NoError(t, kernel.Exec("CREATE ROLE "+quoteIdent(RoleName(packageID))+" NOLOGIN INHERIT CREATEROLE").Error)
	_, err := store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 1}, Grants{Storage: true})
	require.NoError(t, err)
	var attributes struct{ Login, Inherit, CreateRole bool }
	require.NoError(t, kernel.Raw("SELECT rolcanlogin AS login, rolinherit AS inherit, rolcreaterole AS create_role FROM pg_roles WHERE rolname = ?", RoleName(packageID)).Scan(&attributes).Error)
	require.Equal(t, struct{ Login, Inherit, CreateRole bool }{Login: true}, attributes)

	var superuser bool
	require.NoError(t, kernel.Raw("SELECT rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&superuser).Error)
	if !superuser {
		return
	}
	elevatedID := "pkgtest-" + randomSuffix(t)
	t.Cleanup(func() { dropPackageStorage(t, kernel, elevatedID) })
	require.NoError(t, kernel.Exec("CREATE ROLE "+quoteIdent(RoleName(elevatedID))+" NOLOGIN SUPERUSER").Error)
	_, err = store.Lease(context.Background(), Holder{PackageID: elevatedID, Version: "4.1.0", Generation: 1}, Grants{Storage: true})
	require.ErrorContains(t, err, "elevated attributes")
}

func TestPostgresRemoteReplicasShareOneLease(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	packageID := "pkgtest-" + randomSuffix(t)
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	settings, err := parseDSN(kernelDSN)
	require.NoError(t, err)
	remoteHost := settings["host"]
	if settings["port"] != "" {
		remoteHost += ":" + settings["port"]
	}
	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN, RemoteDatabaseHost: remoteHost, Leases: NewLeaseCache()}
	holder := Holder{PackageID: packageID, Version: "4.1.0", Generation: 2, Remote: true}

	first, err := store.Lease(context.Background(), holder, Grants{Storage: true})
	require.NoError(t, err)
	replica := openPostgres(t, first.DSN)
	require.NoError(t, replica.Exec("SELECT 1").Error)
	second, err := store.Lease(context.Background(), holder, Grants{Storage: true})
	require.NoError(t, err)
	require.Equal(t, first.DSN, second.DSN, "a second replica does not rotate the first one's password")
	fresh := openPostgres(t, second.DSN)
	require.NoError(t, fresh.Exec("SELECT 1").Error)
	require.EqualValues(t, 1, second.LeaseGeneration)
}
