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

// The plan views show an order what it needs of a plan, and a view whose
// source table does not exist yet is left out.
func TestPlanViewsShowWhatAnOrderNeeds(t *testing.T) {
	db, _ := openSQLiteKernel(t)
	exists, err := viewExists(db, "kapi_plan_subscription_group_v1")
	require.NoError(t, err)
	require.False(t, exists, "no view without v2_plan_subscription_group")
	require.NoError(t, db.AutoMigrate(&model.Plan{}, &model.PlanSubscriptionGroup{}))
	require.NoError(t, EnsureKernelAPIViews(db))

	var columns []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('kapi_plan_catalog_v1') ORDER BY cid").Scan(&columns).Error)
	require.Equal(t, []string{
		"id", "group_id", "transfer_enable", "speed_limit", "device_limit", "month_price", "quarter_price",
		"half_year_price", "year_price", "two_year_price", "three_year_price", "onetime_price",
	}, columns)
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('kapi_plan_subscription_group_v1') ORDER BY cid").Scan(&columns).Error)
	require.Equal(t, []string{"plan_id", "group_id"}, columns)
}

// kapi_order_billing_v1 shows a payment whose order it is, what it costs
// and its status, and nothing else of v2_order.
func TestOrderBillingViewShowsWhatAPaymentNeeds(t *testing.T) {
	db, _ := openSQLiteKernel(t)
	exists, err := viewExists(db, "kapi_order_billing_v1")
	require.NoError(t, err)
	require.False(t, exists, "no view without v2_order")
	require.NoError(t, db.AutoMigrate(&model.Order{}))
	require.NoError(t, EnsureKernelAPIViews(db))
	var columns []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('kapi_order_billing_v1') ORDER BY cid").Scan(&columns).Error)
	require.Equal(t, []string{"id", "user_id", "total_amount", "status"}, columns)
}

// kapi_traffic_log_v1 shows each traffic report's user, bytes, rate and
// time, and nothing else of v2_server_log.
func TestTrafficLogViewShowsTheTrafficCharts(t *testing.T) {
	db, _ := openSQLiteKernel(t)
	exists, err := viewExists(db, "kapi_traffic_log_v1")
	require.NoError(t, err)
	require.False(t, exists, "no view without v2_server_log")
	require.NoError(t, db.AutoMigrate(&model.TrafficLog{}))
	require.NoError(t, EnsureKernelAPIViews(db))
	var columns []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('kapi_traffic_log_v1') ORDER BY cid").Scan(&columns).Error)
	require.Equal(t, []string{"user_id", "u", "d", "rate", "log_at"}, columns)
}

// kapi_user_referral_v1 shows who invited whom, and nothing else of v2_user.
func TestReferralViewShowsWhoInvitedWhom(t *testing.T) {
	db, _ := openSQLiteKernel(t)
	var columns []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('kapi_user_referral_v1') ORDER BY cid").Scan(&columns).Error)
	require.Equal(t, []string{"id", "invite_user_id"}, columns)
}

// kapi_affiliate_settings_v1 shows the value of the affiliate settings key
// and no other system configuration row.
func TestAffiliateSettingsViewShowsOneKey(t *testing.T) {
	db, _ := openSQLiteKernel(t)
	exists, err := viewExists(db, "kapi_affiliate_settings_v1")
	require.NoError(t, err)
	require.False(t, exists, "no view without v2_system_config")
	require.NoError(t, db.AutoMigrate(&model.SystemConfig{}))
	require.NoError(t, EnsureKernelAPIViews(db))
	require.NoError(t, db.Create(&[]model.SystemConfig{
		{Key: "smtp.password", Value: "smtp-secret"},
		{Key: InviteSettingsKey, Value: `{"code_prefix":"AFF"}`},
		{Key: InviteSettingsKey + ".copy", Value: "other"},
	}).Error)
	var columns []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('kapi_affiliate_settings_v1') ORDER BY cid").Scan(&columns).Error)
	require.Equal(t, []string{"value"}, columns)
	var values []string
	require.NoError(t, db.Raw("SELECT value FROM kapi_affiliate_settings_v1").Scan(&values).Error)
	require.Equal(t, []string{`{"code_prefix":"AFF"}`}, values)
}

// The subscription views show a subscriber's groups and their expiry, a
// protocol's node and a node's last report, and nothing else of their
// tables: no token, uuid, address, key or settings.
func TestSubscriptionViewsShowWhatTheSubscriptionPackageNeeds(t *testing.T) {
	db, _ := openSQLiteKernel(t)
	for _, view := range []string{"kapi_user_subscription_group_v1", "kapi_node_protocol_v1", "kapi_node_heartbeat_v1"} {
		exists, err := viewExists(db, view)
		require.NoError(t, err)
		require.False(t, exists, "no %s without its table", view)
	}
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.SubscriptionGroup{}, &model.UserSubscriptionGroup{}))
	require.NoError(t, EnsureKernelAPIViews(db))
	for view, want := range map[string][]string{
		"kapi_user_subscription_group_v1": {"user_id", "group_id", "expire_at"},
		"kapi_node_protocol_v1":           {"id", "node_id"},
		"kapi_node_heartbeat_v1":          {"id", "last_check_at"},
	} {
		var columns []string
		require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('"+view+"') ORDER BY cid").Scan(&columns).Error)
		require.Equal(t, want, columns, view)
	}
}

// kapi_node_status_v1 shows a node's status, last check and traffic, and
// none of its credentials.
func TestNodeStatusViewShowsNoCredentials(t *testing.T) {
	db, _ := openSQLiteKernel(t)
	exists, err := viewExists(db, "kapi_node_status_v1")
	require.NoError(t, err)
	require.False(t, exists, "no view without v2_node")
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	require.NoError(t, EnsureKernelAPIViews(db))
	var columns []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('kapi_node_status_v1') ORDER BY cid").Scan(&columns).Error)
	require.Equal(t, []string{"id", "status", "last_check_at", "total_upload", "total_download"}, columns)
}

// kapi_forward_node_v1 shows every column of a forward node but its API
// token, which authenticates the node's agent.
func TestForwardNodeViewShowsNoToken(t *testing.T) {
	db, _ := openSQLiteKernel(t)
	exists, err := viewExists(db, "kapi_forward_node_v1")
	require.NoError(t, err)
	require.False(t, exists, "no view without v2_forward_node")
	require.NoError(t, db.AutoMigrate(&model.ForwardNode{}))
	require.NoError(t, EnsureKernelAPIViews(db))
	require.NoError(t, db.Create(&model.ForwardNode{Name: "relay", Type: "relay", Host: "198.51.100.1", Port: 443, APIToken: "node-secret"}).Error)

	var tableColumns, viewColumns []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('v2_forward_node') ORDER BY cid").Scan(&tableColumns).Error)
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('kapi_forward_node_v1') ORDER BY cid").Scan(&viewColumns).Error)
	require.Contains(t, tableColumns, "api_token")
	var withoutToken []string
	for _, column := range tableColumns {
		if column != "api_token" {
			withoutToken = append(withoutToken, column)
		}
	}
	require.Equal(t, withoutToken, viewColumns)
	var hosts []string
	require.NoError(t, db.Raw("SELECT host FROM kapi_forward_node_v1").Scan(&hosts).Error)
	require.Equal(t, []string{"198.51.100.1"}, hosts)
}

// kapi_forward_runtime_settings_v1 shows the keys that choose the forward
// runtime backend and no other system configuration row, the NodeX token
// included.
func TestForwardRuntimeSettingsViewShowsTheBackendKeys(t *testing.T) {
	db, _ := openSQLiteKernel(t)
	require.NoError(t, db.AutoMigrate(&model.SystemConfig{}))
	require.NoError(t, EnsureKernelAPIViews(db))
	require.NoError(t, db.Create(&[]model.SystemConfig{
		{Key: "forward.runtime.nodex.token", Value: "nodex-secret"},
		{Key: "forward.runtime.nodex.base_url", Value: "https://nodex.internal"},
		{Key: "forward.runtime.nodex_mode", Value: "true"},
		{Key: "forward.runtime_backend", Value: "gost"},
		{Key: "forward.runtime.ansible.backend", Value: "nftables_ansible"},
		{Key: "forward.runtime.ansible.config", Value: `{"inventory":"x"}`},
		{Key: "smtp.password", Value: "smtp-secret"},
	}).Error)
	var columns []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('kapi_forward_runtime_settings_v1') ORDER BY cid").Scan(&columns).Error)
	require.Equal(t, []string{"key", "value"}, columns)
	var rows []struct{ Key, Value string }
	require.NoError(t, db.Raw("SELECT key, value FROM kapi_forward_runtime_settings_v1 ORDER BY key").Scan(&rows).Error)
	require.Equal(t, []struct{ Key, Value string }{
		{"forward.runtime.ansible.backend", "nftables_ansible"}, {"forward.runtime.nodex_mode", "true"}, {"forward.runtime_backend", "gost"},
	}, rows)
}
