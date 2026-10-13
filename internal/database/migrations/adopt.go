package migrations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// anchorTables are the tables every start of every Control 4.x created,
// in production as in development: the first administrator's v2_user, the
// v2_node and v2_wireguard_peer schema steps, and the kernel tables of
// 4.0.0 (EnsureKernelSchema). A database without one of them is not a
// database a Control start prepared, so adoption refuses it.
var anchorTables = []string{
	"v2_node",
	"v2_user",
	"v2_wireguard_peer",
	"v3_kernel_access_group",
	"v3_kernel_access_group_plan",
	"v3_kernel_access_group_user",
	"v3_kernel_node_operation_revision",
	"v3_kernel_node_plugin_lifecycle",
	"v3_kernel_node_plugin_observed_state",
	"v3_kernel_node_service_assignment",
	"v3_kernel_operation",
	"v3_kernel_plugin",
	"v3_kernel_plugin_artifact",
	"v3_kernel_plugin_configuration",
	"v3_kernel_plugin_installation",
	"v3_kernel_plugin_lifecycle_plan",
	"v3_kernel_plugin_lifecycle_plan_step",
	"v3_kernel_plugin_release",
	"v3_kernel_plugin_target_lock",
	"v3_kernel_plugin_telemetry_state",
	"v3_kernel_plugin_trust_root",
	"v3_kernel_plugin_webui_asset",
	"v3_kernel_quota_policy",
	"v3_kernel_resource_grant",
	"v3_kernel_service_scope",
	"v3_kernel_topology",
	"v3_kernel_topology_deployment",
	"v3_kernel_topology_deployment_step",
	"v3_kernel_topology_edge",
	"v3_kernel_topology_observed_state",
	"v3_kernel_topology_revision",
	"v3_kernel_topology_vertex",
}

// legacyIndexes are indexes earlier releases created that the baseline
// replaced, by table. The start before this runner dropped them
// (EnsureKernelSchema); adoption does it once.
var legacyIndexes = map[string][]string{
	// The plain unique index on topology names, replaced by the partial
	// ux_topology_live_name so a deleted topology's name can be reused.
	"v3_kernel_topology": {model.LegacyTopologyNameIndex},
}

// runtimeJobActiveIndex is the partial unique index on the active runtime
// job of a flux forward. Before adoption creates it, duplicate active jobs
// are superseded, as the start before this runner did.
const runtimeJobActiveIndex = "idx_forward_runtime_job_active_forward"

// Adoption describes how the runner adopted a database an earlier Control
// created: what it added to reach the baseline, and the differences it
// left because only additions are safe on an existing database.
type Adoption struct {
	CreatedTables  []string
	AddedColumns   []string
	CreatedIndexes []string
	DroppedIndexes []string
	// SupersededJobs counts the duplicate active flux runtime jobs marked
	// failed before the active-job index was created.
	SupersededJobs int64
	// SkippedTables are the flux tables `forward legacy drop` removed;
	// adoption does not create them again.
	SkippedTables []string
	// Differences are what still differs from the baseline: column types,
	// nullability and defaults, constraints, index definitions, and the
	// tables and columns the baseline does not have. They are kept.
	Differences []string
}

// Summary is one line for the log.
func (a *Adoption) Summary() string {
	if len(a.CreatedTables)+len(a.AddedColumns)+len(a.CreatedIndexes)+len(a.DroppedIndexes) == 0 {
		return fmt.Sprintf("it matched the baseline (%d differences kept).", len(a.Differences))
	}
	return fmt.Sprintf("created %d tables, added %d columns, created %d indexes, dropped %d legacy indexes (%d differences kept).",
		len(a.CreatedTables), len(a.AddedColumns), len(a.CreatedIndexes), len(a.DroppedIndexes), len(a.Differences))
}

// adopt brings a database an earlier Control created to the baseline, in
// tx, the way the start before this runner did: it creates the missing
// tables and indexes, adds the missing columns and drops the replaced
// legacy indexes. It never alters or drops a column or table. It then
// checks that every baseline table, column and index is present, and
// refuses the database otherwise, or when it is not a Control database at
// all.
func adopt(ctx context.Context, tx *gorm.DB, baseline Step, options Options) (*Adoption, error) {
	dialect, err := dialectOf(tx)
	if err != nil {
		return nil, err
	}
	objects, err := parseBaseline(baseline.SQL[dialect])
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", baseline.ID(), err)
	}
	before, err := Inspect(ctx, tx)
	if err != nil {
		return nil, err
	}
	adoption := &Adoption{}
	skip := map[string]bool{}
	if forwardlegacy.Dropped(tx) {
		for _, table := range forwardlegacy.DropTables {
			skip[table] = true
			adoption.SkippedTables = append(adoption.SkippedTables, table)
		}
	}

	if err := recognize(before, objects, skip); err != nil {
		return nil, err
	}

	quote := quoteFor(dialect)
	exec := func(statement string) error {
		if err := tx.Exec(statement).Error; err != nil {
			return &RefusedError{Reason: fmt.Sprintf("adopting the existing database failed at %q: %v. Nothing was changed. Fix the database or restore a backup, then start again.", abbreviate(statement), err)}
		}
		return nil
	}
	created := map[string]bool{}
	for _, table := range objects.tables {
		if skip[table.name] {
			continue
		}
		existing := before.Tables[table.name]
		if existing == nil {
			if err := exec(table.statement); err != nil {
				return nil, err
			}
			created[table.name] = true
			adoption.CreatedTables = append(adoption.CreatedTables, table.name)
			continue
		}
		for _, column := range table.columns {
			if _, ok := existing.Columns[column.name]; ok {
				continue
			}
			if err := exec("ALTER TABLE " + quote(table.name) + " ADD COLUMN " + column.definition); err != nil {
				return nil, err
			}
			adoption.AddedColumns = append(adoption.AddedColumns, table.name+"."+column.name)
		}
	}
	for table, names := range legacyIndexes {
		existing := before.Tables[table]
		if existing == nil {
			continue
		}
		for _, name := range names {
			if _, ok := existing.Indexes[name]; !ok {
				continue
			}
			if err := exec("DROP INDEX " + quote(name)); err != nil {
				return nil, err
			}
			adoption.DroppedIndexes = append(adoption.DroppedIndexes, name)
		}
	}
	for _, index := range objects.indexes {
		if skip[index.table] {
			continue
		}
		if !created[index.table] {
			if _, ok := before.Tables[index.table].Indexes[index.name]; ok {
				continue
			}
		}
		if index.name == runtimeJobActiveIndex {
			superseded, err := supersedeDuplicateActiveJobs(tx, options.Now())
			if err != nil {
				return nil, fmt.Errorf("repair duplicate active forward runtime jobs: %w", err)
			}
			adoption.SupersededJobs = superseded
		}
		if err := exec(index.statement); err != nil {
			return nil, err
		}
		adoption.CreatedIndexes = append(adoption.CreatedIndexes, index.name)
	}

	after, err := Inspect(ctx, tx)
	if err != nil {
		return nil, err
	}
	expected, err := baselineSchema(ctx, tx, baseline)
	if err != nil {
		return nil, fmt.Errorf("build the baseline for comparison: %w", err)
	}
	missing, differences := compareToBaseline(expected, after, skip)
	if len(missing) > 0 {
		return nil, &RefusedError{Reason: fmt.Sprintf(
			"the existing database could not be brought to the %s schema; it still lacks: %s. Nothing was changed. Restore a complete backup, or start with an empty database.",
			baseline.ID(), strings.Join(missing, ", "))}
	}
	adoption.Differences = differences
	return adoption, nil
}

// recognize refuses a database that is not one a Control start prepared:
// one without the anchor tables. Its message lists the anchors it lacks
// and every other baseline table it lacks.
func recognize(schema *Schema, objects *baselineObjects, skip map[string]bool) error {
	var anchors []string
	for _, table := range anchorTables {
		if schema.Tables[table] == nil {
			anchors = append(anchors, table)
		}
	}
	if len(anchors) == 0 {
		return nil
	}
	var others []string
	isAnchor := map[string]bool{}
	for _, table := range anchorTables {
		isAnchor[table] = true
	}
	for _, table := range objects.tables {
		if schema.Tables[table.name] == nil && !isAnchor[table.name] && !skip[table.name] {
			others = append(others, table.name)
		}
	}
	reason := fmt.Sprintf(
		"the database is not empty, but it is not a database an AnixOps Control start prepared (an unrecognized or partial schema with %d tables): it lacks the tables every Control 4.x start creates: %s",
		len(schema.Tables), strings.Join(anchors, ", "))
	if len(others) > 0 {
		reason += fmt.Sprintf("; of the other baseline tables it lacks %d: %s", len(others), strings.Join(others, ", "))
	}
	return &RefusedError{Reason: reason + ". Nothing was changed. Point Control at an empty database, or restore a complete backup."}
}

// compareToBaseline answers what the adopted schema still lacks (tables,
// columns, indexes: adoption refuses) and how it differs otherwise (kept).
// Views and the runner's own tables are not compared: views belong to the
// kernel API views step that runs at every start.
func compareToBaseline(expected, got *Schema, skip map[string]bool) (missing, differences []string) {
	names := make([]string, 0, len(expected.Tables))
	for name := range expected.Tables {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if skip[name] {
			continue
		}
		want := expected.Tables[name]
		have := got.Tables[name]
		if have == nil {
			missing = append(missing, "table "+name)
			continue
		}
		for _, column := range sortedColumns(want) {
			current, ok := have.Columns[column.Name]
			if !ok {
				missing = append(missing, "column "+name+"."+column.Name)
				continue
			}
			if current.describe() != column.describe() {
				differences = append(differences, fmt.Sprintf("column %s.%s is %q, the baseline has %q", name, column.Name, current.describe(), column.describe()))
			}
		}
		for _, column := range sortedColumns(have) {
			if _, ok := want.Columns[column.Name]; !ok {
				differences = append(differences, fmt.Sprintf("column %s.%s is not in the baseline", name, column.Name))
			}
		}
		for _, index := range sortedIndexes(want) {
			current, ok := have.Indexes[index.Name]
			if !ok {
				missing = append(missing, "index "+index.Name+" on "+name)
				continue
			}
			if current.Definition != index.Definition {
				differences = append(differences, fmt.Sprintf("index %s is %q, the baseline has %q", index.Name, current.Definition, index.Definition))
			}
		}
		for _, index := range sortedIndexes(have) {
			if _, ok := want.Indexes[index.Name]; !ok {
				differences = append(differences, fmt.Sprintf("index %s on %s is not in the baseline", index.Name, name))
			}
		}
		wantConstraints, haveConstraints := setOf(want.Constraints), setOf(have.Constraints)
		for _, constraint := range want.Constraints {
			if !haveConstraints[constraint] {
				differences = append(differences, fmt.Sprintf("table %s lacks the baseline constraint %s", name, constraint))
			}
		}
		for _, constraint := range have.Constraints {
			if !wantConstraints[constraint] {
				differences = append(differences, fmt.Sprintf("table %s has the constraint %s, the baseline does not", name, constraint))
			}
		}
	}
	var extra []string
	for name := range got.Tables {
		if expected.Tables[name] == nil && name != MigrationTable && name != MetaTable {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		differences = append(differences, "table "+name+" is not in the baseline")
	}
	return missing, differences
}

func sortedColumns(table *Table) []Column {
	columns := make([]Column, 0, len(table.Columns))
	for _, column := range table.Columns {
		columns = append(columns, column)
	}
	sort.Slice(columns, func(i, j int) bool { return columns[i].Name < columns[j].Name })
	return columns
}

func sortedIndexes(table *Table) []Index {
	indexes := make([]Index, 0, len(table.Indexes))
	for _, index := range table.Indexes {
		indexes = append(indexes, index)
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i].Name < indexes[j].Name })
	return indexes
}

func setOf(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

// baselineSchema builds the baseline where it cannot touch the adopted
// database and reads it back: in a private in-memory SQLite database, or
// in a scratch PostgreSQL schema inside a savepoint of tx that is rolled
// back.
func baselineSchema(ctx context.Context, tx *gorm.DB, baseline Step) (*Schema, error) {
	dialect, err := dialectOf(tx)
	if err != nil {
		return nil, err
	}
	list, err := statements(baseline.SQL[dialect])
	if err != nil {
		return nil, err
	}
	if dialect == DialectSQLite {
		scratch, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			return nil, err
		}
		sqlDB, err := scratch.DB()
		if err != nil {
			return nil, err
		}
		defer func() { _ = sqlDB.Close() }()
		sqlDB.SetMaxOpenConns(1) // one connection is one in-memory database
		for _, statement := range list {
			if err := scratch.Exec(statement).Error; err != nil {
				return nil, fmt.Errorf("%s: %w", abbreviate(statement), err)
			}
		}
		return Inspect(ctx, scratch)
	}

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return nil, err
	}
	scratchSchema := "anix_baseline_" + hex.EncodeToString(suffix)
	var searchPath string
	if err := tx.Raw("SELECT current_setting('search_path')").Scan(&searchPath).Error; err != nil {
		return nil, err
	}
	const savepoint = "anix_baseline_scratch"
	if err := tx.SavePoint(savepoint).Error; err != nil {
		return nil, err
	}
	schema, buildErr := func() (*Schema, error) {
		if err := tx.Exec(`CREATE SCHEMA "` + scratchSchema + `"`).Error; err != nil {
			return nil, err
		}
		if err := tx.Exec("SELECT set_config('search_path', ?, true)", `"`+scratchSchema+`"`).Error; err != nil {
			return nil, err
		}
		for _, statement := range list {
			if err := tx.Exec(statement).Error; err != nil {
				return nil, fmt.Errorf("%s: %w", abbreviate(statement), err)
			}
		}
		return Inspect(ctx, tx)
	}()
	if err := tx.RollbackTo(savepoint).Error; err != nil {
		return nil, err
	}
	if err := tx.Exec("SELECT set_config('search_path', ?, true)", searchPath).Error; err != nil {
		return nil, err
	}
	return schema, buildErr
}

// supersedeDuplicateActiveJobs keeps one active (pending or running) flux
// runtime job per forward, preferring a running one and then the newest,
// and marks the others failed: the partial unique index cannot be created
// over duplicates. It is the repair the start before this runner made
// before creating the index.
func supersedeDuplicateActiveJobs(tx *gorm.DB, now time.Time) (int64, error) {
	active := []int{model.ForwardRuntimeJobStatusPending, model.ForwardRuntimeJobStatusRunning}
	var duplicates []struct {
		ForwardID uint
		Count     int64
	}
	if err := tx.Model(&model.ForwardRuntimeJob{}).
		Select("forward_id, COUNT(*) AS count").
		Where("forward_id IS NOT NULL AND status IN ?", active).
		Group("forward_id").
		Having("COUNT(*) > 1").
		Scan(&duplicates).Error; err != nil {
		return 0, err
	}
	var superseded int64
	for _, duplicate := range duplicates {
		var jobs []model.ForwardRuntimeJob
		if err := tx.Where("forward_id = ? AND status IN ?", duplicate.ForwardID, active).
			Order("CASE WHEN status = 1 THEN 0 ELSE 1 END ASC, id DESC").
			Find(&jobs).Error; err != nil {
			return 0, err
		}
		if len(jobs) <= 1 {
			continue
		}
		stale := make([]uint, 0, len(jobs)-1)
		for _, job := range jobs[1:] {
			stale = append(stale, job.ID)
		}
		completed := now
		update := tx.Model(&model.ForwardRuntimeJob{}).Where("id IN ?", stale).Updates(map[string]any{
			"status":       model.ForwardRuntimeJobStatusFailed,
			"error":        "superseded by newer active runtime job during schema repair",
			"completed_at": &completed,
		})
		if update.Error != nil {
			return 0, update.Error
		}
		superseded += update.RowsAffected
	}
	return superseded, nil
}

func quoteFor(dialect string) func(string) string {
	if dialect == DialectPostgres {
		return func(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }
	}
	return func(name string) string { return "`" + strings.ReplaceAll(name, "`", "``") + "`" }
}
