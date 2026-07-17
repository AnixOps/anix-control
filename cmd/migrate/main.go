// Command migrate is a one-off tool that imports data from an old XBoard
// (PHP) MySQL dump into this project's GORM-backed database. It preserves
// user passwords (bcrypt is cross-compatible) and subscription tokens
// verbatim, and converts the old per-protocol server tables into the new
// Node/NodeProtocol/SubscriptionGroup model.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// tablesWrittenByMigration lists every table this tool inserts into. When
// -reset is passed, these are truncated first so old-dump primary keys can
// be reused verbatim without colliding with whatever is already in the
// target database.
var tablesWrittenByMigration = []string{
	"v2_user_subscription_group",
	"v2_subscription_group_node_protocols",
	"v2_node_protocol",
	"v2_node",
	"v2_subscription_group",
	"v2_invite_code",
	"v2_system_config",
	"v2_user",
	"v2_plan",
}

// skippedTables are present in the old dump but intentionally not migrated
// (see plan doc): legacy audit/stat tables with no new-schema equivalent, and
// tables that are empty or edge-case in this particular dump.
var skippedTables = []string{
	"failed_jobs", "migrations",
	"v2_commission_log", "v2_log", "v2_mail_log", "v2_notice",
	"v2_stat", "v2_stat_server", "v2_stat_user",
	"v2_knowledge", "v2_ticket", "v2_ticket_message",
	"v2_coupon", "v2_order", "v2_payment", "v2_server_route",
}

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to config.yaml (target database)")
	dumpPath := flag.String("dump", "", "path to old XBoard mysqldump (.sql or .sql.gz)")
	dryRun := flag.Bool("dry-run", false, "parse and report only; do not touch the target database")
	reset := flag.Bool("reset", false, "truncate target tables before importing (required unless they are already empty)")
	flag.Parse()

	if *dumpPath == "" {
		log.Fatal("missing required -dump <path to xboard mysqldump>")
	}

	tables, err := ParseDump(*dumpPath)
	if err != nil {
		log.Fatalf("failed to parse dump: %v", err)
	}

	report := newReport()
	for _, name := range skippedTables {
		if t, ok := tables[name]; ok {
			report.skip(name, len(t.Rows))
		}
	}

	plan, err := buildPlan(tables)
	if err != nil {
		log.Fatalf("failed to build migration plan: %v", err)
	}

	if *dryRun {
		plan.printSummary(report)
		fmt.Println("\n-dry-run: no database was touched.")
		return
	}

	resolvedConfigPath, err := resolveConfigPath(*configPath)
	if err != nil {
		log.Fatalf("failed to locate config: %v", err)
	}
	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	driver := strings.ToLower(cfg.Database.Driver)
	if driver == "" || driver == "sqlite" || driver == "sqlite3" {
		cfg.Database.Database = resolveRuntimePath(cfg.Database.Database, resolvedConfigPath)
	}

	if err := database.Init(&cfg.Database); err != nil {
		log.Fatalf("failed to init database: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	db := database.Get()

	if err := ensureTargetIsSafe(db, *reset); err != nil {
		log.Fatalf("%v", err)
	}

	if err := runImport(db, plan); err != nil {
		log.Fatalf("import failed: %v", err)
	}

	plan.printSummary(report)
	fmt.Println("\nImport complete.")
	fmt.Println("Manual follow-up required:")
	fmt.Println("  - Payment gateways were not migrated (old and new provider models are incompatible).")
	fmt.Println("    Reconfigure payment methods in the admin panel.")
	fmt.Println("  - Spot-check a couple of migrated users' speed_limit values; units are copied verbatim")
	fmt.Println("    from the old database and were not converted.")
}

// ensureTargetIsSafe truncates the tables this tool writes to when -reset is
// passed, or verifies they are already empty otherwise. This exists so old
// primary keys can be reused verbatim (preserving FK relationships) without
// silently overwriting whatever is currently in the target database.
func ensureTargetIsSafe(db *gorm.DB, reset bool) error {
	if reset {
		// Delete in reverse dependency order.
		for i := len(tablesWrittenByMigration) - 1; i >= 0; i-- {
			table := tablesWrittenByMigration[i]
			if err := db.Exec(fmt.Sprintf("DELETE FROM %s", table)).Error; err != nil {
				return fmt.Errorf("reset table %s: %w", table, err)
			}
		}
		return nil
	}

	for _, table := range tablesWrittenByMigration {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			return fmt.Errorf("check table %s: %w", table, err)
		}
		if count > 0 {
			return fmt.Errorf("table %s already has %d row(s); pass -reset to truncate target tables first, or migrate into an empty database", table, count)
		}
	}
	return nil
}

// resolveConfigPath mirrors cmd/server/main.go's lookup: try the raw path
// relative to cwd, then relative to the project root inferred from this
// binary's working directory.
func resolveConfigPath(rawPath string) (string, error) {
	if filepath.IsAbs(rawPath) {
		if fileExists(rawPath) {
			return rawPath, nil
		}
		return rawPath, fmt.Errorf("config file not found: %s", rawPath)
	}
	if fileExists(rawPath) {
		abs, err := filepath.Abs(rawPath)
		if err != nil {
			return rawPath, nil
		}
		return abs, nil
	}
	return rawPath, fmt.Errorf("config file not found: %s (run this tool from the project root)", rawPath)
}

// resolveRuntimePath resolves a relative path against the directory the
// resolved config file lives in, matching cmd/server/main.go's behavior for
// the sqlite database path.
func resolveRuntimePath(rawPath, resolvedConfigPath string) string {
	if rawPath == "" || filepath.IsAbs(rawPath) {
		return rawPath
	}
	configDir := filepath.Dir(resolvedConfigPath)
	if strings.EqualFold(filepath.Base(configDir), "config") {
		candidate := filepath.Join(filepath.Dir(configDir), rawPath)
		if fileExists(candidate) {
			return candidate
		}
	}
	return filepath.Join(configDir, rawPath)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// silence unused-import complaints when model is only referenced by other
// files in this package during partial builds.
var _ = model.User{}
