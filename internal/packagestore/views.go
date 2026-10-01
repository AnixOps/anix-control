package packagestore

import (
	"fmt"

	"gorm.io/gorm"
)

// KernelAPIView is a versioned, read-only view that packages may be granted
// with the kernel.view:<name> capability. A published version never changes;
// a new shape gets a new version.
type KernelAPIView struct {
	Name string
	// Source is the kernel table the view reads.
	Source string
	Query  string
	// RowFilter marks a view that shows only some rows of its source. On
	// PostgreSQL it is a security_barrier view, so a function in a
	// package's query cannot see the rows the filter hides before the
	// filter applies.
	RowFilter bool
}

// InviteSettingsKey is the system configuration key holding the affiliate
// frontend settings, the one row of v2_system_config that
// kapi_affiliate_settings_v1 shows.
const InviteSettingsKey = "invite.frontend.config"

// KernelAPIViews lists every kernel API view. They expose only the columns a
// package may read: never password hashes, tokens or subscription UUIDs.
var KernelAPIViews = []KernelAPIView{
	{
		Name:   "kapi_user_directory_v1",
		Source: "v2_user",
		Query:  "SELECT id, email, is_admin, is_staff, banned, plan_id, group_id, expired_at, created_at FROM v2_user",
	},
	{
		// The system audit trail the kernel records for configuration and
		// backup changes, for the platform package's audit log list. Its
		// content holds which fields changed and whether a secret is set,
		// never a secret's value.
		Name:   "kapi_system_audit_log_v1",
		Source: "v2_operation_log",
		Query: "SELECT id, user_id, username, action, module, target_type, target_id, content, ip, user_agent, status, created_at " +
			"FROM v2_operation_log WHERE module = 'system'",
		RowFilter: true,
	},
	{
		// Entitlements and traffic counters (docs/architecture/subscriber-service.md);
		// no token or uuid.
		Name:   "kapi_subscriber_entitlement_v1",
		Source: "v2_user",
		Query: "SELECT id, plan_id, group_id, expired_at, transfer_enable, u, d, speed_limit, device_limit, " +
			"flow_reset_time, banned, balance, commission_balance FROM v2_user",
	},
	{
		// What an order needs of a plan: its prices, to price an order, and
		// what it grants (group, transfer in GiB, limits), to complete one.
		Name:   "kapi_plan_catalog_v1",
		Source: "v2_plan",
		Query: "SELECT id, group_id, transfer_enable, speed_limit, device_limit, month_price, quarter_price, " +
			"half_year_price, year_price, two_year_price, three_year_price, onetime_price FROM v2_plan",
	},
	{
		// The subscription groups a plan grants, which a completed order
		// gives the subscriber.
		Name:   "kapi_plan_subscription_group_v1",
		Source: "v2_plan_subscription_group",
		Query:  "SELECT plan_id, group_id FROM v2_plan_subscription_group",
	},
	{
		// What a payment needs of an order: whose it is, what it costs (in
		// cents) and whether it is still pending.
		Name:   "kapi_order_billing_v1",
		Source: "v2_order",
		Query:  "SELECT id, user_id, total_amount, status FROM v2_order",
	},
	{
		// Who invited whom, for the affiliate's invite statistics.
		Name:   "kapi_user_referral_v1",
		Source: "v2_user",
		Query:  "SELECT id, invite_user_id FROM v2_user",
	},
	{
		// The affiliate's frontend settings (code prefix and length,
		// withdrawal fee and methods): the value of one system
		// configuration key, which the kernel does not treat as sensitive.
		// No other row of v2_system_config, which holds secrets, is visible.
		Name:      "kapi_affiliate_settings_v1",
		Source:    "v2_system_config",
		Query:     "SELECT value FROM v2_system_config WHERE key = '" + InviteSettingsKey + "'",
		RowFilter: true,
	},
	{
		// The subscription groups each subscriber holds and until when, for
		// the subscription package's group lists and statistics. The kernel
		// stays the only writer of this subscriber state.
		Name:   "kapi_user_subscription_group_v1",
		Source: "v2_user_subscription_group",
		Query:  "SELECT user_id, group_id, expire_at FROM v2_user_subscription_group",
	},
	{
		// The node a node protocol runs on, to link protocols to
		// subscription groups and count a group's online nodes. No settings:
		// they hold keys.
		Name:   "kapi_node_protocol_v1",
		Source: "v2_node_protocol",
		Query:  "SELECT id, node_id FROM v2_node_protocol",
	},
	{
		// When a node last reported, to count online nodes. No address,
		// key or configuration.
		Name:   "kapi_node_heartbeat_v1",
		Source: "v2_node",
		Query:  "SELECT id, last_check_at FROM v2_node",
	},
}

// EnsureKernelAPIViews creates the kernel API views that do not exist yet.
// Existing views keep their definition, since a published version is
// immutable; on PostgreSQL a RowFilter view, existing or new, is a security
// barrier. A view whose source table does not exist is left out; the kernel
// creates its tables before its views, so this only happens on a partial
// schema, and a lease that grants the view then fails as for any missing
// view.
func EnsureKernelAPIViews(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	for _, view := range KernelAPIViews {
		if !grantTargetPattern.MatchString(view.Name) {
			return fmt.Errorf("invalid kernel API view name %q", view.Name)
		}
		exists, err := viewExists(db, view.Name)
		if err != nil {
			return err
		}
		barrier := view.RowFilter && db.Name() == DriverPostgres
		if exists {
			// Views created before they were barriers become barriers.
			if barrier {
				if err := db.Exec("ALTER VIEW " + quoteIdent(view.Name) + " SET (security_barrier = true)").Error; err != nil {
					return fmt.Errorf("make kernel API view %s a security barrier: %w", view.Name, err)
				}
			}
			continue
		}
		if !db.Migrator().HasTable(view.Source) {
			continue
		}
		options := ""
		if barrier {
			options = " WITH (security_barrier)"
		}
		if err := db.Exec("CREATE VIEW " + quoteIdent(view.Name) + options + " AS " + view.Query).Error; err != nil {
			return fmt.Errorf("create kernel API view %s: %w", view.Name, err)
		}
	}
	return nil
}

func viewExists(db *gorm.DB, name string) (bool, error) {
	var count int64
	query := "SELECT count(*) FROM sqlite_master WHERE type = 'view' AND name = ?"
	if db.Name() == DriverPostgres {
		query = "SELECT count(*) FROM pg_views WHERE schemaname = current_schema() AND viewname = ?"
	}
	err := db.Raw(query, name).Scan(&count).Error
	return count > 0, err
}
