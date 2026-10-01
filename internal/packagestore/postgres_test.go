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

// A role granted the plan views reads the plan catalog and its subscription
// groups and nothing else of v2_plan, and cannot change them through the
// views.
func TestPostgresPlanViewsAreReadOnly(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	require.NoError(t, kernel.AutoMigrate(&model.Plan{}, &model.PlanSubscriptionGroup{}))
	require.NoError(t, EnsureKernelAPIViews(kernel))
	suffix := randomSuffix(t)
	packageID := "pkgtest-" + suffix
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	price := int64(1000)
	plan := model.Plan{Name: "catalog-" + suffix, GroupID: 3, TransferEnable: 10, MonthPrice: &price}
	require.NoError(t, kernel.Create(&plan).Error)
	t.Cleanup(func() { _ = kernel.Delete(&model.Plan{}, plan.ID).Error })
	group := model.SubscriptionGroup{Name: "catalog-" + suffix}
	require.NoError(t, kernel.Create(&group).Error)
	t.Cleanup(func() { _ = kernel.Delete(&model.SubscriptionGroup{}, group.ID).Error })
	require.NoError(t, kernel.Create(&model.PlanSubscriptionGroup{PlanID: plan.ID, GroupID: group.ID}).Error)
	t.Cleanup(func() { _ = kernel.Where("plan_id = ?", plan.ID).Delete(&model.PlanSubscriptionGroup{}).Error })

	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	lease, err := store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
		Grants{Storage: true, Views: []string{"kapi_plan_catalog_v1", "kapi_plan_subscription_group_v1"}})
	require.NoError(t, err)
	pkg := openPostgres(t, lease.DSN)
	var prices []int64
	require.NoError(t, pkg.Raw("SELECT month_price FROM kapi_plan_catalog_v1 WHERE id = ?", plan.ID).Scan(&prices).Error)
	require.Equal(t, []int64{1000}, prices)
	var groups []uint
	require.NoError(t, pkg.Raw("SELECT group_id FROM kapi_plan_subscription_group_v1 WHERE plan_id = ?", plan.ID).Scan(&groups).Error)
	require.Equal(t, []uint{group.ID}, groups)
	var names []string
	require.ErrorContains(t, pkg.Raw("SELECT name FROM kapi_plan_catalog_v1").Scan(&names).Error, "does not exist")
	var count int64
	requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM v2_plan").Scan(&count).Error)
	requirePermissionDenied(t, pkg.Exec("UPDATE kapi_plan_catalog_v1 SET month_price = 1 WHERE id = ?", plan.ID).Error)
	requirePermissionDenied(t, pkg.Exec("DELETE FROM kapi_plan_subscription_group_v1 WHERE plan_id = ?", plan.ID).Error)
}

// A role granted kapi_order_billing_v1 reads an order's buyer, total and
// status, and can neither read v2_order itself nor change it through the
// view.
func TestPostgresOrderBillingViewIsReadOnly(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	require.NoError(t, kernel.AutoMigrate(&model.Order{}))
	require.NoError(t, EnsureKernelAPIViews(kernel))
	suffix := randomSuffix(t)
	packageID := "pkgtest-" + suffix
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	user := model.User{Email: "billing-" + suffix + "@example.test", Token: "billing-" + suffix, UUID: "billing-" + suffix}
	require.NoError(t, kernel.Create(&user).Error)
	t.Cleanup(func() { _ = kernel.Delete(&model.User{}, user.ID).Error })
	plan := model.Plan{Name: "billing-" + suffix}
	require.NoError(t, kernel.Create(&plan).Error)
	t.Cleanup(func() { _ = kernel.Delete(&model.Plan{}, plan.ID).Error })
	order := model.Order{UserID: user.ID, PlanID: plan.ID, TradeNo: "billing-" + suffix, TotalAmount: 4200, CallbackNo: &suffix}
	require.NoError(t, kernel.Create(&order).Error)
	t.Cleanup(func() { _ = kernel.Delete(&model.Order{}, order.ID).Error })

	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	lease, err := store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
		Grants{Storage: true, Views: []string{"kapi_order_billing_v1"}})
	require.NoError(t, err)
	pkg := openPostgres(t, lease.DSN)
	var rows []struct {
		UserID      uint
		TotalAmount int64
		Status      int
	}
	require.NoError(t, pkg.Raw("SELECT user_id, total_amount, status FROM kapi_order_billing_v1 WHERE id = ?", order.ID).Scan(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, user.ID, rows[0].UserID)
	require.Equal(t, int64(4200), rows[0].TotalAmount)
	var tradeNos []string
	require.ErrorContains(t, pkg.Raw("SELECT trade_no FROM kapi_order_billing_v1").Scan(&tradeNos).Error, "does not exist")
	var count int64
	requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM v2_order").Scan(&count).Error)
	requirePermissionDenied(t, pkg.Exec("UPDATE kapi_order_billing_v1 SET status = 1 WHERE id = ?", order.ID).Error)
}

// A role granted kapi_traffic_log_v1 reads the traffic reports' users,
// bytes, rate and time, and can neither read v2_server_log itself nor change
// it through the view.
func TestPostgresTrafficLogViewIsReadOnly(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	require.NoError(t, kernel.AutoMigrate(&model.TrafficLog{}))
	require.NoError(t, EnsureKernelAPIViews(kernel))
	suffix := randomSuffix(t)
	packageID := "pkgtest-" + suffix
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	entry := model.TrafficLog{UserID: 4242, ServerID: 7, ServerType: "vless-" + suffix, U: 100, D: 200, Rate: 1.5, LogAt: 1_790_000_000}
	require.NoError(t, kernel.Create(&entry).Error)
	t.Cleanup(func() { _ = kernel.Delete(&model.TrafficLog{}, entry.ID).Error })

	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	lease, err := store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
		Grants{Storage: true, Views: []string{"kapi_traffic_log_v1"}})
	require.NoError(t, err)
	pkg := openPostgres(t, lease.DSN)
	var rows []struct {
		UserID uint
		U, D   int64
		Rate   float64
		LogAt  int64
	}
	require.NoError(t, pkg.Raw("SELECT user_id, u, d, rate, log_at FROM kapi_traffic_log_v1 WHERE log_at = ? AND user_id = ?", entry.LogAt, entry.UserID).Scan(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, int64(200), rows[0].D)
	var serverTypes []string
	require.ErrorContains(t, pkg.Raw("SELECT server_type FROM kapi_traffic_log_v1").Scan(&serverTypes).Error, "does not exist")
	var count int64
	requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM v2_server_log").Scan(&count).Error)
	requirePermissionDenied(t, pkg.Exec("UPDATE kapi_traffic_log_v1 SET u = 0 WHERE user_id = ?", entry.UserID).Error)
}

// A role granted kapi_user_referral_v1 reads who invited whom and nothing
// else of v2_user.
func TestPostgresReferralViewIsReadOnly(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	suffix := randomSuffix(t)
	packageID := "pkgtest-" + suffix
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	inviter := model.User{Email: "inviter-" + suffix + "@example.test", Token: "inviter-" + suffix, UUID: "inviter-" + suffix}
	require.NoError(t, kernel.Create(&inviter).Error)
	t.Cleanup(func() { _ = kernel.Delete(&model.User{}, inviter.ID).Error })
	invited := model.User{Email: "invited-" + suffix + "@example.test", Token: "invited-" + suffix, UUID: "invited-" + suffix, InviteUserID: &inviter.ID}
	require.NoError(t, kernel.Create(&invited).Error)
	t.Cleanup(func() { _ = kernel.Delete(&model.User{}, invited.ID).Error })

	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	lease, err := store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
		Grants{Storage: true, Views: []string{"kapi_user_referral_v1"}})
	require.NoError(t, err)
	pkg := openPostgres(t, lease.DSN)
	var inviters []uint
	require.NoError(t, pkg.Raw("SELECT invite_user_id FROM kapi_user_referral_v1 WHERE id = ?", invited.ID).Scan(&inviters).Error)
	require.Equal(t, []uint{inviter.ID}, inviters)
	var emails []string
	require.ErrorContains(t, pkg.Raw("SELECT email FROM kapi_user_referral_v1").Scan(&emails).Error, "does not exist")
	requirePermissionDenied(t, pkg.Exec("UPDATE kapi_user_referral_v1 SET invite_user_id = NULL WHERE id = ?", invited.ID).Error)
}

// A role granted kapi_affiliate_settings_v1 reads the affiliate settings
// and no other v2_system_config row, not even through a function of its
// own that the planner could otherwise run before the view's filter.
func TestPostgresAffiliateSettingsViewHidesOtherKeys(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	require.NoError(t, kernel.AutoMigrate(&model.SystemConfig{}))
	// Views are immutable once created; recreate this one so the test sees
	// the current definition.
	require.NoError(t, kernel.Exec("DROP VIEW IF EXISTS kapi_affiliate_settings_v1").Error)
	require.NoError(t, EnsureKernelAPIViews(kernel))
	suffix := randomSuffix(t)
	packageID := "pkgtest-" + suffix
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	secret := "smtp-secret-" + suffix
	settings := `{"code_prefix":"AFF` + suffix + `"}`
	require.NoError(t, kernel.Where("key IN ?", []string{InviteSettingsKey, "smtp.password." + suffix}).Delete(&model.SystemConfig{}).Error)
	require.NoError(t, kernel.Create(&[]model.SystemConfig{
		{Key: "smtp.password." + suffix, Value: secret},
		{Key: InviteSettingsKey, Value: settings},
	}).Error)
	t.Cleanup(func() {
		_ = kernel.Where("key IN ?", []string{InviteSettingsKey, "smtp.password." + suffix}).Delete(&model.SystemConfig{}).Error
	})

	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	lease, err := store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
		Grants{Storage: true, Views: []string{"kapi_affiliate_settings_v1"}})
	require.NoError(t, err)
	pkg := openPostgres(t, lease.DSN)
	var values []string
	require.NoError(t, pkg.Raw("SELECT value FROM kapi_affiliate_settings_v1").Scan(&values).Error)
	require.Equal(t, []string{settings}, values)
	var count int64
	requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM v2_system_config").Scan(&count).Error)
	requirePermissionDenied(t, pkg.Exec("UPDATE kapi_affiliate_settings_v1 SET value = 'x'").Error)

	// A cheap function in the package's own schema records every value it
	// is shown. The view is a security barrier, so it sees only the rows the
	// view shows.
	require.NoError(t, pkg.Exec("CREATE TABLE seen (value text)").Error)
	require.NoError(t, pkg.Exec(`CREATE FUNCTION peek(v text) RETURNS boolean LANGUAGE plpgsql COST 0.0000001 AS $$
		BEGIN INSERT INTO seen VALUES (v); RETURN true; END $$`).Error)
	// Without the key's index the filter is a plain condition the planner
	// would run after the cheaper function; any role may turn indexes off.
	var seen []string
	require.NoError(t, pkg.Transaction(func(tx *gorm.DB) error {
		for _, setting := range []string{"enable_indexscan", "enable_bitmapscan", "enable_indexonlyscan"} {
			if err := tx.Exec("SET LOCAL " + setting + " = off").Error; err != nil {
				return err
			}
		}
		if err := tx.Raw("SELECT value FROM kapi_affiliate_settings_v1 WHERE peek(value)").Scan(&values).Error; err != nil {
			return err
		}
		return tx.Raw("SELECT value FROM seen").Scan(&seen).Error
	}))
	require.NotContains(t, seen, secret)
	require.Equal(t, []string{settings}, seen)
}

// The subscription package's role writes its adopted tables, including
// rows that reference plans and node protocols it cannot read, and reads
// memberships, protocols and nodes only through their views: no membership
// write, no protocol settings, no node key.
func TestPostgresSubscriptionGrants(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	require.NoError(t, kernel.AutoMigrate(&model.Plan{}, &model.Node{}, &model.NodeProtocol{}, &model.SubscriptionGroup{},
		&model.SubscriptionTemplate{}, &model.PlanSubscriptionGroup{}, &model.UserSubscriptionGroup{}))
	require.NoError(t, EnsureKernelAPIViews(kernel))
	suffix := randomSuffix(t)
	packageID := "pkgtest-" + suffix
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	user := model.User{Email: "subscriber-" + suffix + "@example.test", Token: "subscriber-" + suffix, UUID: "subscriber-" + suffix}
	plan := model.Plan{Name: "plan-" + suffix}
	group := model.SubscriptionGroup{Name: "group-" + suffix, Enable: 1}
	node := model.Node{Name: "node-" + suffix, APIKey: "key-" + suffix, Secret: "secret-" + suffix}
	require.NoError(t, kernel.Create(&user).Error)
	require.NoError(t, kernel.Create(&plan).Error)
	require.NoError(t, kernel.Create(&group).Error)
	require.NoError(t, kernel.Create(&node).Error)
	protocol := model.NodeProtocol{NodeID: node.ID, Name: "p", RealitySettings: ptr(`{"private_key":"pk-` + suffix + `"}`)}
	require.NoError(t, kernel.Create(&protocol).Error)
	expires := int64(4102444800)
	member := model.UserSubscriptionGroup{UserID: user.ID, GroupID: group.ID, ExpireAt: &expires}
	require.NoError(t, kernel.Create(&member).Error)
	t.Cleanup(func() {
		kernel.Exec("DELETE FROM v2_subscription_group_node_protocols WHERE subscription_group_id = ?", group.ID)
		kernel.Where("group_id = ?", group.ID).Delete(&model.SubscriptionTemplate{})
		kernel.Where("group_id = ?", group.ID).Delete(&model.PlanSubscriptionGroup{})
		kernel.Where("group_id = ?", group.ID).Delete(&model.UserSubscriptionGroup{})
		kernel.Delete(&model.SubscriptionGroup{}, group.ID)
		kernel.Delete(&model.NodeProtocol{}, protocol.ID)
		kernel.Delete(&model.Node{}, node.ID)
		kernel.Delete(&model.Plan{}, plan.ID)
		kernel.Delete(&model.User{}, user.ID)
	})

	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	lease, err := store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 1}, Grants{
		Storage: true,
		AdoptTables: []string{
			"v2_plan_subscription_group", "v2_subscription_group", "v2_subscription_group_node_protocols", "v2_subscription_template",
		},
		Views: []string{
			"kapi_node_heartbeat_v1", "kapi_node_protocol_v1", "kapi_plan_catalog_v1", "kapi_subscriber_entitlement_v1",
			"kapi_user_subscription_group_v1",
		},
	})
	require.NoError(t, err)
	pkg := openPostgres(t, lease.DSN)

	require.NoError(t, pkg.Exec("UPDATE v2_subscription_group SET priority = 3 WHERE id = ?", group.ID).Error)
	require.NoError(t, pkg.Exec("INSERT INTO v2_subscription_template (group_id, name, created_at, updated_at) VALUES (?, 't', now(), now())", group.ID).Error)
	require.NoError(t, pkg.Exec("INSERT INTO v2_plan_subscription_group (plan_id, group_id, created_at) VALUES (?, ?, now())", plan.ID, group.ID).Error)
	require.NoError(t, pkg.Exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (?, ?)", group.ID, protocol.ID).Error)

	var members []struct {
		UserID   uint
		GroupID  uint
		ExpireAt *int64
	}
	require.NoError(t, pkg.Raw("SELECT * FROM kapi_user_subscription_group_v1 WHERE group_id = ?", group.ID).Scan(&members).Error)
	require.Len(t, members, 1)
	require.Equal(t, user.ID, members[0].UserID)
	require.Equal(t, expires, *members[0].ExpireAt)
	var nodeIDs []uint
	require.NoError(t, pkg.Raw("SELECT node_id FROM kapi_node_protocol_v1 WHERE id = ?", protocol.ID).Scan(&nodeIDs).Error)
	require.Equal(t, []uint{node.ID}, nodeIDs)
	var heartbeats int64
	require.NoError(t, pkg.Raw("SELECT count(*) FROM kapi_node_heartbeat_v1 WHERE id = ?", node.ID).Scan(&heartbeats).Error)
	require.EqualValues(t, 1, heartbeats)

	var values []string
	require.ErrorContains(t, pkg.Raw("SELECT reality_settings FROM kapi_node_protocol_v1").Scan(&values).Error, "does not exist")
	require.ErrorContains(t, pkg.Raw("SELECT api_key FROM kapi_node_heartbeat_v1").Scan(&values).Error, "does not exist")
	var count int64
	for _, table := range []string{"v2_node_protocol", "v2_node", "v2_user_subscription_group", "v2_plan", "v2_user"} {
		requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM "+table).Scan(&count).Error)
	}
	requirePermissionDenied(t, pkg.Exec("DELETE FROM kapi_user_subscription_group_v1 WHERE group_id = ?", group.ID).Error)
	requirePermissionDenied(t, pkg.Exec("INSERT INTO v2_user_subscription_group (user_id, group_id, created_at) VALUES (?, ?, now())", user.ID, group.ID).Error)
}

func ptr[T any](value T) *T { return &value }

// A role granted kapi_node_status_v1 reads a node's status and traffic, and
// can neither read a node's credentials, nor v2_node itself, nor change it
// through the view.
func TestPostgresNodeStatusViewHidesNodeCredentials(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	require.NoError(t, kernel.AutoMigrate(&model.Node{}))
	require.NoError(t, EnsureKernelAPIViews(kernel))
	suffix := randomSuffix(t)
	packageID := "pkgtest-" + suffix
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	checked := int64(1_790_000_000)
	node := model.Node{Name: "node-" + suffix, Host: "node.example.test", APIKey: "key-" + suffix, APIKeyHash: "hash-" + suffix,
		Secret: "secret-" + suffix, Status: model.NodeStatusOnline, LastCheckAt: &checked, TotalUpload: 7, TotalDownload: 9}
	require.NoError(t, kernel.Create(&node).Error)
	t.Cleanup(func() { _ = kernel.Delete(&model.Node{}, node.ID).Error })

	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	lease, err := store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
		Grants{Storage: true, Views: []string{"kapi_node_status_v1"}})
	require.NoError(t, err)
	pkg := openPostgres(t, lease.DSN)
	var rows []struct {
		Status        int
		LastCheckAt   int64
		TotalUpload   int64
		TotalDownload int64
	}
	require.NoError(t, pkg.Raw("SELECT status, last_check_at, total_upload, total_download FROM kapi_node_status_v1 WHERE id = ?", node.ID).Scan(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, int(model.NodeStatusOnline), rows[0].Status)
	require.Equal(t, checked, rows[0].LastCheckAt)
	require.Equal(t, int64(7), rows[0].TotalUpload)
	for _, column := range []string{"api_key", "api_key_hash", "secret", "raw_config"} {
		var values []string
		require.ErrorContains(t, pkg.Raw("SELECT "+column+" FROM kapi_node_status_v1").Scan(&values).Error, "does not exist", column)
	}
	var count int64
	requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM v2_node").Scan(&count).Error)
	requirePermissionDenied(t, pkg.Exec("UPDATE kapi_node_status_v1 SET status = 3 WHERE id = ?", node.ID).Error)
}

// A package granted the forward views reads forward nodes without their API
// tokens and only the backend keys of the system configuration, and neither
// source table. The settings view is a security barrier.
func TestPostgresForwardViewsHideTokensAndOtherKeys(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	require.NoError(t, kernel.AutoMigrate(&model.SystemConfig{}, &model.ForwardNode{}))
	// Views are immutable once created; recreate these so the test sees the
	// current definitions.
	require.NoError(t, kernel.Exec("DROP VIEW IF EXISTS kapi_forward_node_v1").Error)
	require.NoError(t, kernel.Exec("DROP VIEW IF EXISTS kapi_forward_runtime_settings_v1").Error)
	require.NoError(t, EnsureKernelAPIViews(kernel))
	suffix := randomSuffix(t)
	packageID := "pkgtest-" + suffix
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	token := "node-secret-" + suffix
	node := model.ForwardNode{Name: "relay-" + suffix, Type: "relay", Host: "198.51.100.1", Port: 443, APIToken: token}
	require.NoError(t, kernel.Create(&node).Error)
	t.Cleanup(func() { _ = kernel.Delete(&model.ForwardNode{}, node.ID).Error })
	secretKey := "forward.runtime.nodex.token." + suffix
	keys := []string{secretKey, "forward.runtime_backend"}
	require.NoError(t, kernel.Where("key IN ?", keys).Delete(&model.SystemConfig{}).Error)
	require.NoError(t, kernel.Create(&[]model.SystemConfig{
		{Key: secretKey, Value: "nodex-secret-" + suffix},
		{Key: "forward.runtime_backend", Value: "gost-" + suffix},
	}).Error)
	t.Cleanup(func() { _ = kernel.Where("key IN ?", keys).Delete(&model.SystemConfig{}).Error })

	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	lease, err := store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
		Grants{Storage: true, Views: []string{"kapi_forward_node_v1", "kapi_forward_runtime_settings_v1"}})
	require.NoError(t, err)
	pkg := openPostgres(t, lease.DSN)

	var hosts []string
	require.NoError(t, pkg.Raw("SELECT host FROM kapi_forward_node_v1 WHERE id = ?", node.ID).Scan(&hosts).Error)
	require.Equal(t, []string{"198.51.100.1"}, hosts)
	var tokens []string
	require.Error(t, pkg.Raw("SELECT api_token FROM kapi_forward_node_v1").Scan(&tokens).Error, "the view has no token column")
	var count int64
	requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM v2_forward_node").Scan(&count).Error)
	requirePermissionDenied(t, pkg.Raw("SELECT count(*) FROM v2_system_config").Scan(&count).Error)
	requirePermissionDenied(t, pkg.Exec("UPDATE kapi_forward_runtime_settings_v1 SET value = 'x'").Error)

	var values []string
	require.NoError(t, pkg.Raw("SELECT value FROM kapi_forward_runtime_settings_v1 WHERE key = 'forward.runtime_backend'").Scan(&values).Error)
	require.Equal(t, []string{"gost-" + suffix}, values)

	// A cheap function in the package's own schema records every value it
	// is shown; the view is a security barrier, so it never sees the NodeX
	// token.
	require.NoError(t, pkg.Exec("CREATE TABLE seen (value text)").Error)
	require.NoError(t, pkg.Exec(`CREATE FUNCTION peek(v text) RETURNS boolean LANGUAGE plpgsql COST 0.0000001 AS $$
		BEGIN INSERT INTO seen VALUES (v); RETURN true; END $$`).Error)
	var seen []string
	require.NoError(t, pkg.Transaction(func(tx *gorm.DB) error {
		for _, setting := range []string{"enable_indexscan", "enable_bitmapscan", "enable_indexonlyscan"} {
			if err := tx.Exec("SET LOCAL " + setting + " = off").Error; err != nil {
				return err
			}
		}
		if err := tx.Raw("SELECT value FROM kapi_forward_runtime_settings_v1 WHERE peek(value)").Scan(&values).Error; err != nil {
			return err
		}
		return tx.Raw("SELECT value FROM seen").Scan(&seen).Error
	}))
	require.NotContains(t, seen, "nodex-secret-"+suffix)
	require.Contains(t, seen, "gost-"+suffix)
}
