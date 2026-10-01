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
