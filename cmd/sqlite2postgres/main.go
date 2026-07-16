package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/config"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/service"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	primaryControlConfigPath = "/opt/anixops/control/config/config.yaml"
	legacyControlConfigPath  = "/etc/v2board/config.yaml"
)

func main() {
	sourcePath := flag.String("source", "config/data/v2board.db", "source SQLite database path")
	targetConfig := flag.String("target-config", defaultControlConfigPath(), "target AnixOps Control config.yaml containing PostgreSQL database settings")
	targetDSN := flag.String("target-dsn", "", "target PostgreSQL DSN; overrides -target-config")
	reset := flag.Bool("reset", false, "truncate PostgreSQL tables before importing")
	dryRun := flag.Bool("dry-run", false, "inspect source and target only")
	flag.Parse()

	src, err := gorm.Open(sqlite.Open(*sourcePath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatalf("open sqlite source: %v", err)
	}
	srcSQL, err := src.DB()
	if err != nil {
		log.Fatalf("sqlite sql db: %v", err)
	}
	defer logClose("sqlite source", srcSQL.Close)

	pg, err := openPostgres(*targetConfig, *targetDSN)
	if err != nil {
		log.Fatalf("open postgres target: %v", err)
	}
	pgSQL, err := pg.DB()
	if err != nil {
		log.Fatalf("postgres sql db: %v", err)
	}
	defer logClose("postgres target", pgSQL.Close)

	tables, err := sqliteTables(src)
	if err != nil {
		log.Fatalf("list sqlite tables: %v", err)
	}
	if len(tables) == 0 {
		log.Fatalf("source has no application tables: %s", *sourcePath)
	}

	counts, err := tableCounts(src, tables)
	if err != nil {
		log.Fatalf("count sqlite rows: %v", err)
	}

	fmt.Printf("Source: %s\n", *sourcePath)
	fmt.Printf("Target: PostgreSQL\n")
	fmt.Printf("Tables: %d\n", len(tables))
	printImportantCounts(counts)

	if *dryRun {
		fmt.Println("dry-run: no PostgreSQL data changed")
		return
	}
	if !*reset {
		log.Fatalf("refusing to import without -reset")
	}

	if err := migrateSchema(pg); err != nil {
		log.Fatalf("migrate postgres schema: %v", err)
	}

	pgTables, err := postgresTableSet(pg)
	if err != nil {
		log.Fatalf("list postgres tables: %v", err)
	}
	importTables := make([]string, 0, len(tables))
	for _, table := range tables {
		if pgTables[table] {
			importTables = append(importTables, table)
		}
	}
	if len(importTables) != len(tables) {
		for _, table := range tables {
			if !pgTables[table] {
				log.Printf("warning: source table %s does not exist in postgres, skipped", table)
			}
		}
	}

	boolCols, err := postgresBoolColumns(pg)
	if err != nil {
		log.Fatalf("load postgres boolean columns: %v", err)
	}
	importTables, err = sortTablesByForeignKeys(pg, importTables)
	if err != nil {
		log.Fatalf("sort tables by foreign keys: %v", err)
	}

	start := time.Now()
	err = pg.Transaction(func(tx *gorm.DB) error {
		if err := truncateTables(tx, importTables); err != nil {
			return err
		}
		for _, table := range importTables {
			if err := copyTable(src, tx, table, boolCols[table]); err != nil {
				return err
			}
		}
		for _, table := range importTables {
			if err := resetSequence(tx, table); err != nil {
				return err
			}
		}
		if err := service.EnsureForwardRuntimeJobSchema(tx); err != nil {
			return err
		}
		return service.BackfillForwardPortBindings(tx)
	})
	if err != nil {
		log.Fatalf("import failed: %v", err)
	}

	fmt.Printf("Import complete in %s.\n", time.Since(start).Round(time.Millisecond))
	printImportantCounts(counts)
}

func defaultControlConfigPath() string {
	if _, err := os.Stat(primaryControlConfigPath); err == nil {
		return primaryControlConfigPath
	}
	if _, err := os.Stat(legacyControlConfigPath); err == nil {
		return legacyControlConfigPath
	}
	return primaryControlConfigPath
}

func openPostgres(configPath, dsn string) (*gorm.DB, error) {
	if dsn != "" {
		return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	}
	cfgPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	driver := strings.ToLower(cfg.Database.Driver)
	if driver != "postgres" && driver != "postgresql" {
		return nil, fmt.Errorf("target config database.driver is %q, expected postgres", cfg.Database.Driver)
	}
	dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Shanghai",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.Username, cfg.Database.Password, cfg.Database.Database)
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
}

func migrateSchema(db *gorm.DB) error {
	if err := db.AutoMigrate(allModels()...); err != nil {
		return err
	}
	if err := service.EnsureObservabilitySchema(db); err != nil {
		return err
	}
	if err := service.EnsureStatsSchema(db); err != nil {
		return err
	}
	if err := service.EnsureForwardBridgeSchema(db); err != nil {
		return err
	}
	if err := service.EnsureForwardPortBindingSchema(db); err != nil {
		return err
	}
	if err := service.EnsureForwardNodeMetricsPortColumn(db); err != nil {
		return err
	}
	if err := service.EnsureNodeRuntimeHealthSchema(db); err != nil {
		return err
	}
	return service.EnsureAgentDiagnosticTaskSchema(db)
}

func allModels() []any {
	models := []any{
		&model.User{}, &model.Plan{}, &model.Order{}, &model.Payment{}, &model.PaymentLog{},
		&model.ServerVMess{}, &model.ServerVLESS{}, &model.ServerTrojan{}, &model.ServerShadowsocks{},
		&model.Node{}, &model.NodeProtocol{}, &model.WireGuardPeer{}, &model.NodeGroup{}, &model.AuthorizedKey{},
		&model.TrafficLog{}, &model.OnlineLog{}, &model.StatUser{}, &model.StatServer{}, &model.NodeLog{},
		&model.SubscriptionGroup{}, &model.SubscriptionTemplate{}, &model.UserSubscriptionGroup{}, &model.PlanSubscriptionGroup{},
		&model.Event{}, &model.Ticket{}, &model.TicketMessage{}, &model.Coupon{}, &model.CouponUsage{}, &model.Knowledge{},
		&model.ForwardNode{}, &model.ForwardRule{}, &model.ForwardRoute{}, &model.ForwardLog{}, &model.ForwardStats{},
		&model.ForwardTunnel{}, &model.ForwardUserTunnel{}, &model.Forward{}, &model.ForwardPortBinding{}, &model.SpeedLimit{}, &model.ForwardRuntimeJob{},
		&model.ForwardTrafficCursor{}, &model.ForwardCleanAgent{}, &model.ForwardAgentBridgeTask{}, &model.ForwardLatencyBucket{},
		&model.PaymentGateway{}, &model.PaymentRecord{}, &model.TelegramBot{}, &model.TelegramUser{}, &model.TelegramChat{},
		&model.TelegramCommand{}, &model.TelegramNotification{}, &model.NotificationTemplate{}, &model.NotificationLog{},
		&model.UserMFA{}, &model.MFALoginAttempt{}, &model.UserLevel{}, &model.InviteCode{}, &model.CommissionRecord{},
		&model.CommissionWithdraw{}, &model.InviteConfig{}, &model.LoadBalancer{}, &model.SystemConfig{}, &model.BackupRecord{},
		&model.BackupConfig{}, &model.OperationLog{}, &model.AuditLog{},
	}
	return append(models, model.KernelModels()...)
}

func sqliteTables(db *gorm.DB) ([]string, error) {
	var tables []string
	err := db.Raw(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`).Scan(&tables).Error
	return tables, err
}

func tableCounts(db *gorm.DB, tables []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		counts[table] = count
	}
	return counts, nil
}

func printImportantCounts(counts map[string]int64) {
	for _, table := range []string{"v2_user", "v2_plan", "v2_node", "v2_node_protocol", "v2_subscription_group", "v2_forward", "v2_system_config"} {
		if count, ok := counts[table]; ok {
			fmt.Printf("  %-28s %d\n", table, count)
		}
	}
}

func postgresTableSet(db *gorm.DB) (map[string]bool, error) {
	var tables []string
	if err := db.Raw(`SELECT tablename FROM pg_tables WHERE schemaname = 'public'`).Scan(&tables).Error; err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(tables))
	for _, table := range tables {
		set[table] = true
	}
	return set, nil
}

func postgresBoolColumns(db *gorm.DB) (result map[string]map[string]bool, err error) {
	rows, err := db.Raw(`
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = 'public' AND data_type = 'boolean'
	`).Rows()
	if err != nil {
		return nil, err
	}
	defer joinRowsClose(&err, rows, "postgres boolean columns")

	result = map[string]map[string]bool{}
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return nil, err
		}
		if result[table] == nil {
			result[table] = map[string]bool{}
		}
		result[table][column] = true
	}
	return result, rows.Err()
}

func sortTablesByForeignKeys(db *gorm.DB, tables []string) ([]string, error) {
	type fk struct {
		Child  string
		Parent string
	}

	var fks []fk
	if err := db.Raw(`
		SELECT child.relname AS child, parent.relname AS parent
		FROM pg_constraint c
		JOIN pg_class child ON child.oid = c.conrelid
		JOIN pg_namespace child_ns ON child_ns.oid = child.relnamespace
		JOIN pg_class parent ON parent.oid = c.confrelid
		JOIN pg_namespace parent_ns ON parent_ns.oid = parent.relnamespace
		WHERE c.contype = 'f'
		  AND child_ns.nspname = 'public'
		  AND parent_ns.nspname = 'public'
	`).Scan(&fks).Error; err != nil {
		return nil, err
	}

	tableSet := make(map[string]bool, len(tables))
	order := make(map[string]int, len(tables))
	for i, table := range tables {
		tableSet[table] = true
		order[table] = i
	}

	indegree := make(map[string]int, len(tables))
	children := make(map[string][]string, len(tables))
	seenEdge := map[string]bool{}
	for _, table := range tables {
		indegree[table] = 0
	}
	for _, edge := range fks {
		if !tableSet[edge.Child] || !tableSet[edge.Parent] || edge.Child == edge.Parent {
			continue
		}
		key := edge.Parent + "\x00" + edge.Child
		if seenEdge[key] {
			continue
		}
		seenEdge[key] = true
		indegree[edge.Child]++
		children[edge.Parent] = append(children[edge.Parent], edge.Child)
	}
	for parent := range children {
		sort.SliceStable(children[parent], func(i, j int) bool {
			return order[children[parent][i]] < order[children[parent][j]]
		})
	}

	queue := make([]string, 0, len(tables))
	for _, table := range tables {
		if indegree[table] == 0 {
			queue = append(queue, table)
		}
	}

	result := make([]string, 0, len(tables))
	for len(queue) > 0 {
		table := queue[0]
		queue = queue[1:]
		result = append(result, table)
		for _, child := range children[table] {
			indegree[child]--
			if indegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	if len(result) != len(tables) {
		done := make(map[string]bool, len(result))
		for _, table := range result {
			done[table] = true
		}
		for _, table := range tables {
			if !done[table] {
				log.Printf("warning: table %s is in a foreign-key cycle or unresolved dependency; importing after acyclic tables", table)
				result = append(result, table)
			}
		}
	}
	return result, nil
}

func truncateTables(tx *gorm.DB, tables []string) error {
	if len(tables) == 0 {
		return nil
	}
	quoted := make([]string, 0, len(tables))
	for _, table := range tables {
		quoted = append(quoted, quoteIdent(table))
	}
	return tx.Exec("TRUNCATE TABLE " + strings.Join(quoted, ", ") + " RESTART IDENTITY CASCADE").Error
}

func copyTable(src, dst *gorm.DB, table string, boolCols map[string]bool) (err error) {
	rows, err := src.Table(table).Rows()
	if err != nil {
		return fmt.Errorf("read %s: %w", table, err)
	}
	defer joinRowsClose(&err, rows, "copy "+table)

	cols, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("columns %s: %w", table, err)
	}
	count := 0
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("scan %s: %w", table, err)
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = normalizeValue(values[i], boolCols[col])
		}
		if err := dst.Table(table).Create(row).Error; err != nil {
			return fmt.Errorf("insert %s row %d: %w", table, count+1, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate %s: %w", table, err)
	}
	log.Printf("imported %-32s %d rows", table, count)
	return nil
}

func logClose(name string, closeFn func() error) {
	if err := closeFn(); err != nil {
		log.Printf("close %s: %v", name, err)
	}
}

func joinRowsClose(errp *error, rows *sql.Rows, label string) {
	if closeErr := rows.Close(); closeErr != nil {
		*errp = errors.Join(*errp, fmt.Errorf("close %s rows: %w", label, closeErr))
	}
}

func normalizeValue(v any, wantBool bool) any {
	if v == nil {
		return nil
	}
	if b, ok := v.([]byte); ok {
		v = string(b)
	}
	if !wantBool {
		return v
	}
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	case int:
		return x != 0
	case float64:
		return x != 0
	case string:
		s := strings.TrimSpace(strings.ToLower(x))
		return s == "1" || s == "true" || s == "t" || s == "yes"
	default:
		return v
	}
}

func resetSequence(tx *gorm.DB, table string) error {
	var hasID bool
	if err := tx.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = ? AND column_name = 'id'
		)
	`, table).Scan(&hasID).Error; err != nil {
		return err
	}
	if !hasID {
		return nil
	}
	var max *int64
	if err := tx.Raw("SELECT MAX(id) FROM " + quoteIdent(table)).Scan(&max).Error; err != nil {
		return err
	}
	if max == nil {
		return nil
	}
	log.Printf("reset sequence %-32s max id %d", table, *max)
	return tx.Exec(`SELECT setval(pg_get_serial_sequence(?, 'id'), ?, true)`, table, *max).Error
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func init() {
	log.SetFlags(log.LstdFlags)
}
