// Package forwardlegacy is the v4.2 forwarding upgrade (forward-sdk.md
// section 10, F5c, gate H15): it archives the flux forwarding data, checks
// that every forward node is clean of the old runtime, and, only when an
// administrator confirms it on the command line, drops the flux tables.
//
// The drop is the only place the flux tables are dropped, and it is never
// automatic. Once it is recorded (v4_forward_legacy_drop), Control no
// longer creates those tables at start (KeepModels, UnlessDropped) and no
// longer runs the flux workers.
package forwardlegacy

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// DropTables are the flux forwarding tables the drop removes, dependents
// first, so a foreign key never points at a table already gone. The F5
// audit: the flux forwards, tunnels, user tunnel grants and speed limits;
// the legacy rules; the runtime jobs with their port claims, traffic
// cursors and clean agent bridge tasks; and the pre-flux smart routes,
// connection log and hourly stats, which nothing reads any more.
var DropTables = []string{
	"v2_forward_port_binding",
	"v2_forward_traffic_cursor",
	"v2_forward_agent_bridge_task",
	"v2_forward_runtime_job",
	"v2_forward_user_tunnel",
	"v2_speed_limit",
	"v2_forward",
	"v2_forward_tunnel",
	"v2_forward_rule",
	"v2_forward_route",
	"v2_forward_log",
	"v2_forward_stats",
}

// KeptTables are archived for reference but not dropped:
//   - v2_forward_node is the node inventory and the Agent identity
//     (forward-<id>), which v4.2 keeps (section 10);
//   - v2_forward_clean_agent is a table of the node credential split
//     (nodesecrets) and the source of kapi_forward_clean_agent_v1; it goes
//     with the clean agent's code, not with the flux data.
//
// v2_forward_latency_bucket is neither: its probes also cover proxy nodes
// (LatencyTargetTypeNode), so it is not flux data.
var KeptTables = []string{"v2_forward_node", "v2_forward_clean_agent"}

// ConfirmPhrase is what `forward legacy drop --confirm` must be given,
// exactly.
const ConfirmPhrase = "DROP v4.1 FORWARDING TABLES"

// SingletonLease is the lease the running Control's singleton workers
// hold (cmd/server); the drop refuses while it is held.
const SingletonLease = "control.singleton-workers"

// Node states.
const (
	StateUnchecked   = "unchecked"
	StateClean       = "clean"
	StateDirty       = "dirty"
	StateUnreachable = "unreachable"
	StateAbandoned   = "abandoned"
)

// EnsureSchema creates the upgrade's own tables. They are new tables:
// nothing existing is altered.
func EnsureSchema(db *gorm.DB) error {
	for _, value := range model.ForwardLegacyModels() {
		if db.Migrator().HasTable(value) {
			continue
		}
		if err := db.Migrator().CreateTable(value); err != nil {
			return err
		}
	}
	return nil
}

// LatestDrop answers the recorded drop, nil when the tables were not
// dropped.
func LatestDrop(db *gorm.DB) (*model.ForwardLegacyDrop, error) {
	if !db.Migrator().HasTable(&model.ForwardLegacyDrop{}) {
		return nil, nil
	}
	var drop model.ForwardLegacyDrop
	err := db.Order("id DESC").First(&drop).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &drop, nil
}

// Dropped reports whether the flux tables were dropped. An error reading
// the record answers false: Control then behaves as before the drop.
func Dropped(db *gorm.DB) bool {
	drop, err := LatestDrop(db)
	return err == nil && drop != nil
}

// MissingTable reports whether err is the database saying that table does
// not exist: SQLite's "no such table: <table>", or PostgreSQL's undefined
// table (SQLSTATE 42P01) naming it. It is how a statement on a flux table
// finds out that Drop removed the table, without a metadata query before
// every statement: the code that runs on a database with the table keeps
// its statements and its errors, and only this answer means "dropped".
//
// Do not guard with Migrator().HasTable instead. It answers false when its
// own query fails (a cancelled context, a lost connection, a statement
// timeout), so a guard built on it takes any failure for a dropped table
// and answers "nothing there" where the caller must have the error.
// Another table missing, and every other error, is false.
func MissingTable(err error, table string) bool {
	if err == nil || table == "" {
		return false
	}
	message := strings.ToLower(err.Error())
	missing := strings.Contains(message, "no such table") || strings.Contains(message, "sqlstate 42p01")
	if !missing {
		var state interface{ SQLState() string }
		missing = errors.As(err, &state) && state.SQLState() == "42P01"
	}
	return missing && namesTable(message, strings.ToLower(table))
}

// namesTable reports whether message holds table as a whole identifier, so
// "v2_forward" is not found in the error of "v2_forward_rule".
func namesTable(message, table string) bool {
	isIdentifier := func(b byte) bool {
		return b == '_' || b == '$' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z'
	}
	for from := 0; from < len(message); {
		at := strings.Index(message[from:], table)
		if at < 0 {
			return false
		}
		start := from + at
		end := start + len(table)
		if (start == 0 || !isIdentifier(message[start-1])) && (end == len(message) || !isIdentifier(message[end])) {
			return true
		}
		from = start + 1
	}
	return false
}

// isDropTable reports whether table is one of DropTables.
func isDropTable(table string) bool {
	for _, candidate := range DropTables {
		if candidate == table {
			return true
		}
	}
	return false
}

type tableNamer interface{ TableName() string }

// KeepModels answers models without the flux tables once they were
// dropped, so the schema migration does not create them again. It keys on
// table names, so it keeps working whichever models a later release still
// lists.
func KeepModels(db *gorm.DB, models []any) []any {
	if !Dropped(db) {
		return models
	}
	kept := make([]any, 0, len(models))
	for _, value := range models {
		if named, ok := value.(tableNamer); ok && isDropTable(named.TableName()) {
			continue
		}
		kept = append(kept, value)
	}
	return kept
}

// UnlessDropped wraps a schema step that creates a flux table: once the
// tables were dropped it does nothing.
func UnlessDropped(step func(*gorm.DB) error) func(*gorm.DB) error {
	return func(db *gorm.DB) error {
		if Dropped(db) {
			return nil
		}
		return step(db)
	}
}

// presentTables answers which of tables exist.
func presentTables(db *gorm.DB, tables []string) map[string]bool {
	present := make(map[string]bool, len(tables))
	for _, table := range tables {
		present[table] = db.Migrator().HasTable(table)
	}
	return present
}

// AnyDropTablePresent reports whether one of the flux tables exists.
func AnyDropTablePresent(db *gorm.DB) bool {
	for _, table := range DropTables {
		if db.Migrator().HasTable(table) {
			return true
		}
	}
	return false
}

// ControlRunning answers the holder of the running Control's singleton
// lease and until when, when it is held now.
func ControlRunning(ctx context.Context, db *gorm.DB, now time.Time) (string, time.Time, bool, error) {
	if !db.Migrator().HasTable("v4_kernel_lease") {
		return "", time.Time{}, false, nil
	}
	var rows []struct {
		Holder    string
		ExpiresAt time.Time
	}
	if err := db.WithContext(ctx).Table("v4_kernel_lease").Select("holder, expires_at").
		Where("name = ?", SingletonLease).Scan(&rows).Error; err != nil {
		return "", time.Time{}, false, err
	}
	if len(rows) == 0 || rows[0].Holder == "" || !rows[0].ExpiresAt.After(now) {
		return "", time.Time{}, false, nil
	}
	return rows[0].Holder, rows[0].ExpiresAt, true, nil
}
