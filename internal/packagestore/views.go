package packagestore

import (
	"fmt"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"gorm.io/gorm"
)

// KernelAPIView is a versioned, read-only view that packages may be granted
// with the kernel.view:<name> capability. A published version never changes;
// a new shape gets a new version.
type KernelAPIView struct {
	Name string
	// Source is the kernel table the view reads.
	Source string
	// Joined are the other kernel tables the view reads; like Source, each
	// must exist before the view is created.
	Joined []string
	Query  string
	// PostgresQuery, when set, replaces Query on PostgreSQL: for a view
	// that filters rows by the reading package's role, which SQLite does
	// not have.
	PostgresQuery string
	// RowFilter marks a view that shows only some rows of its source. On
	// PostgreSQL it is a security_barrier view, so a function in a
	// package's query cannot see the rows the filter hides before the
	// filter applies.
	RowFilter bool
	// Finalized names the node credential split tables
	// (node-ops-service.md section 4.6) that must all be finalized before
	// the view is created, because it shows a column that held secrets
	// until then or the credential-free remainder of a split table. Unsplit
	// drops the view again (DropFinalizedViews).
	Finalized []string
	// FinalizedAny names split tables of which one must be finalized
	// before the view is created; the view shows no secret in any phase,
	// so unsplit leaves it.
	FinalizedAny []string
}

// InviteSettingsKey is the system configuration key holding the affiliate
// frontend settings, the one row of v2_system_config that
// kapi_affiliate_settings_v1 shows.
const InviteSettingsKey = "invite.frontend.config"

// KernelAPIViews lists every kernel API view. They expose only the columns a
// package may read: never password hashes, tokens, subscription UUIDs or
// node credentials.
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
		// A plan's name, which the order list and detail answers show
		// next to the order and the administrator's user list next to the
		// user. No price, content or limit.
		Name:   "kapi_plan_name_v1",
		Source: "v2_plan",
		Query:  "SELECT id, name FROM v2_plan",
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
		// Each node traffic report's bytes per user, its rate and when it
		// was logged, for the machine-telemetry package's traffic charts and
		// ranking. No node, token or credential.
		Name:   "kapi_traffic_log_v1",
		Source: "v2_server_log",
		Query:  "SELECT user_id, u, d, rate, log_at FROM v2_server_log",
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
	{
		// What the proxy-node package reads of a node: its status, last
		// check and traffic counters, for the node statistics and to know
		// that a node exists. Never its API key, key hash or shared secret,
		// which the kernel's node authentication checks.
		Name:   "kapi_node_status_v1",
		Source: "v2_node",
		Query:  "SELECT id, status, last_check_at, total_upload, total_download FROM v2_node",
	},
	{
		// What the forward package reads of a forward node: everything but
		// its API token, which authenticates the node's agent. The forward
		// node routes themselves stay in the kernel.
		Name:   "kapi_forward_node_v1",
		Source: "v2_forward_node",
		Query: "SELECT id, name, type, host, port, api_port, metrics_port, region, isp, datacenter, bandwidth, status, " +
			"last_check, latency, load, uptime, tags, weight, max_conn, enabled, total_upload, total_download, current_conn, " +
			"created_at, updated_at FROM v2_forward_node",
	},
	{
		// The system configuration keys that choose the forward runtime
		// backend (NodeX mode, the backend, the local Ansible backend):
		// names of backends and a switch, none of them secret. The NodeX
		// address and token and the Ansible settings are other rows, which
		// this view does not show.
		Name:   "kapi_forward_runtime_settings_v1",
		Source: "v2_system_config",
		Query: "SELECT key, value FROM v2_system_config WHERE key IN ('" +
			strings.Join(ForwardRuntimeSettingKeys, "', '") + "')",
		RowFilter: true,
	},
	{
		// The latest package report of each node, plugin and kind
		// (docs/architecture/package-reports.md): the payload the kernel
		// sanitized, when it was observed and received. On PostgreSQL a
		// package's role sees only the rows of its own plugin_id. SQLite has
		// no roles, so there the view shows every row and a package filters
		// by its own id; SQLite package storage isolates nothing anyway.
		Name:   "kapi_package_report_v1",
		Source: "v4_kernel_package_report_state",
		Query:  "SELECT " + packageReportViewColumns + " FROM v4_kernel_package_report_state",
		PostgresQuery: "SELECT " + packageReportViewColumns + " FROM v4_kernel_package_report_state " +
			"WHERE '" + RolePrefix + "' || replace(plugin_id, '-', '_') = current_user",
		RowFilter: true,
	},
	{
		// Each installation's configuration document as the kernel stored
		// it (v3_kernel_plugin_configuration), with the installation's
		// package, target and desired version: what a package needs to read
		// its own settings, such as machine-telemetry's per-node services
		// switch in its Agent installation's document. On PostgreSQL a
		// package's role sees only the rows of its own plugin_id. SQLite
		// has no roles, so there it shows every row and a package filters
		// by its own id. No official package declares secret_fields, so
		// no document holds a secret.
		Name:   "kapi_plugin_configuration_v1",
		Source: "v3_kernel_plugin_configuration",
		Joined: []string{"v3_kernel_plugin_installation"},
		Query:  "SELECT " + pluginConfigurationViewColumns + " " + pluginConfigurationViewFrom,
		PostgresQuery: "SELECT " + pluginConfigurationViewColumns + " " + pluginConfigurationViewFrom +
			" WHERE '" + RolePrefix + "' || replace(i.plugin_id, '-', '_') = current_user",
		RowFilter: true,
	},
}

const packageReportViewColumns = "node_kind, node_id, plugin_id, kind, version, payload_json, observed_at, received_at"

const (
	pluginConfigurationViewColumns = "i.plugin_id AS plugin_id, i.target AS target, i.desired_version AS desired_version, " +
		"c.revision AS revision, c.config_json AS config_json, c.updated_at AS updated_at"
	pluginConfigurationViewFrom = "FROM v3_kernel_plugin_configuration c JOIN v3_kernel_plugin_installation i ON i.id = c.installation_id"
)

// The views of the node credential split's remainder (node-ops-service.md
// section 4.6). Each exists only once its source is finalized: before that,
// EnsureKernelAPIViews leaves it out, and a lease leaves its grant out
// (FinalizedViewAvailable), so they are inert. None shows a moved column:
// no API key, key hash, shared secret, token, registration key or peer key;
// the JSON columns only in their finalized, redacted form.
func init() {
	KernelAPIViews = append(KernelAPIViews, splitViews...)
}

var splitViews = []KernelAPIView{
	{
		// A proxy node, without its API key, key hash and shared secret.
		// raw_config is the redacted document a finalized v2_node keeps.
		Name:   "kapi_node_public_v1",
		Source: "v2_node",
		Query: "SELECT " + quotedColumns("id", "name", "host", "port", "status", "tags", "group_id", "rate", "traffic_rate",
			"sort", "show", "auto_register", "parent_id", "monthly_limit", "monthly_upload", "monthly_download",
			"monthly_reset_day", "raw_config", "server_ip", "server_version", "server_os", "cpu_usage", "memory_usage",
			"disk_usage", "uptime", "online_users", "runtime_healthy", "runtime_error", "runtime_checked_at",
			"total_upload", "total_download", "last_check_at", "created_at", "updated_at") + " FROM v2_node",
		Finalized: []string{"v2_node"},
	},
	{
		// Every column of a node protocol: once finalized its settings
		// hold the placeholder at every secret position.
		Name:   "kapi_node_protocol_public_v1",
		Source: "v2_node_protocol",
		Query: "SELECT " + quotedColumns("id", "node_id", "name", "type", "port", "enable", "show", "sort", "group_id",
			"host", "tls", "alpn", "settings", "tls_settings", "transport", "transport_settings", "reality_settings",
			"custom_config", "created_at", "updated_at") + " FROM v2_node_protocol",
		Finalized: []string{"v2_node_protocol"},
	},
	{
		// Which credentials a node, forward node, clean agent or
		// registration key holds, and their state: "has a token" since the
		// legacy columns hold tombstones. No value and no hash.
		Name:   "kapi_node_credential_status_v1",
		Source: "v4_kernel_node_credential",
		Query: "SELECT " + quotedColumns("subject_kind", "subject_id", "kind", "version", "status", "rotated_at") +
			", (value <> '') AS has_value FROM v4_kernel_node_credential",
		FinalizedAny: []string{"v2_node", "v2_forward_node", "v2_authorized_key", "v2_forward_clean_agent"},
	},
	{
		// A registration key without the key: whether it holds one comes
		// from its credential.
		Name:   "kapi_registration_key_v1",
		Source: "v2_authorized_key",
		Query: "SELECT k.id, k.name, k.used, k.expire_at, k.created_at, k.updated_at, " +
			"EXISTS (SELECT 1 FROM v4_kernel_node_credential c WHERE c.subject_kind = 'registration_key' " +
			"AND c.subject_id = k.id AND c.kind = 'registration_key' AND c.status = 'active' AND c.value <> '') AS has_key " +
			"FROM v2_authorized_key k",
		// No row is filtered; the subquery's WHERE makes it a barrier
		// view on PostgreSQL, which costs nothing here.
		RowFilter: true,
		Finalized: []string{"v2_authorized_key"},
	},
	{
		// A clean agent, without its token.
		Name:   "kapi_forward_clean_agent_v1",
		Source: "v2_forward_clean_agent",
		Query: "SELECT " + quotedColumns("id", "node_id", "name", "version", "hostname", "os", "arch", "kernel", "public_ip",
			"private_ip", "capabilities", "status", "last_seen", "last_error", "revoked_at", "created_at", "updated_at") +
			" FROM v2_forward_clean_agent",
		Finalized: []string{"v2_forward_clean_agent"},
	},
	{
		// A user's WireGuard peer, without its private and preshared keys.
		Name:   "kapi_wireguard_peer_v1",
		Source: "v2_wireguard_peer",
		Query: "SELECT " + quotedColumns("id", "node_protocol_id", "user_id", "peer_ip", "public_key", "created_at", "updated_at") +
			" FROM v2_wireguard_peer",
		Finalized: []string{"v2_wireguard_peer"},
	},
}

// quotedColumns joins column names, quoted: show and sort are keywords.
func quotedColumns(columns ...string) string {
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = quoteIdent(column)
	}
	return strings.Join(quoted, ", ")
}

// FinalizedViewAvailable reports whether view may exist and be granted
// now: a view of the split's remainder only once its source is finalized.
// Every other view answers true.
func FinalizedViewAvailable(db *gorm.DB, view string) (bool, error) {
	for _, candidate := range KernelAPIViews {
		if candidate.Name != view {
			continue
		}
		if len(candidate.Finalized) == 0 && len(candidate.FinalizedAny) == 0 {
			return true, nil
		}
		finalized, err := nodesecrets.FinalizedTables(db)
		if err != nil {
			return false, err
		}
		return splitViewReady(candidate, finalized), nil
	}
	return true, nil
}

func splitViewReady(view KernelAPIView, finalized []string) bool {
	done := make(map[string]bool, len(finalized))
	for _, table := range finalized {
		done[table] = true
	}
	for _, table := range view.Finalized {
		if !done[table] {
			return false
		}
	}
	if len(view.FinalizedAny) == 0 {
		return true
	}
	for _, table := range view.FinalizedAny {
		if done[table] {
			return true
		}
	}
	return false
}

// DropFinalizedViews drops the views that may exist only while table is
// finalized (Finalized names it), in tx: unsplit writes secrets back into
// the columns they show.
func DropFinalizedViews(tx *gorm.DB, table string) error {
	for _, view := range KernelAPIViews {
		if !containsString(view.Finalized, table) {
			continue
		}
		if err := tx.Exec("DROP VIEW IF EXISTS " + quoteIdent(view.Name)).Error; err != nil {
			return fmt.Errorf("drop kernel API view %s: %w", view.Name, err)
		}
	}
	return nil
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// ForwardRuntimeSettingKeys are the system configuration keys
// kapi_forward_runtime_settings_v1 shows.
var ForwardRuntimeSettingKeys = []string{
	"forward.runtime.nodex_mode", "forward.runtime_backend", "forward.runtime.ansible.backend",
}

// EnsureKernelAPIViews creates the kernel API views that do not exist yet.
// Existing views keep their definition, since a published version is
// immutable; on PostgreSQL a RowFilter view, existing or new, is a security
// barrier. A view whose source table does not exist is left out; the kernel
// creates its tables before its views, so this only happens on a partial
// schema, and a lease that grants the view then fails as for any missing
// view.
//
// A view of the split's remainder (Finalized, FinalizedAny) is created only
// once its source is finalized; until then it is left out.
func EnsureKernelAPIViews(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	finalized, err := nodesecrets.FinalizedTables(db)
	if err != nil {
		return err
	}
	for _, view := range KernelAPIViews {
		if !splitViewReady(view, finalized) {
			continue
		}
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
		if !db.Migrator().HasTable(view.Source) || !hasTables(db, view.Joined) {
			continue
		}
		options := ""
		if barrier {
			options = " WITH (security_barrier)"
		}
		if err := db.Exec("CREATE VIEW " + quoteIdent(view.Name) + options + " AS " + view.query(db.Name())).Error; err != nil {
			return fmt.Errorf("create kernel API view %s: %w", view.Name, err)
		}
	}
	return nil
}

// query is the view's definition on driver.
func (view KernelAPIView) query(driver string) string {
	if driver == DriverPostgres && view.PostgresQuery != "" {
		return view.PostgresQuery
	}
	return view.Query
}

func hasTables(db *gorm.DB, tables []string) bool {
	for _, table := range tables {
		if !db.Migrator().HasTable(table) {
			return false
		}
	}
	return true
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
