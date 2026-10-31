package migrations_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v5/internal/lease"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/soft_delete"
)

// The legacy oracle: how Control prepared its schema before the migration
// runner (cmd/server bootstrapDatabase at go_dev 4eb87485, release 4.2.1):
// migrateSchema (AutoMigrate of the application models; in production only
// the missing tables), then the Ensure* schema steps at every start. It is
// kept here, schema statements only, as the oracle the baseline was
// recorded from and is compared with (TestBaselineEqualsTheLegacySchema).
//
// legacyModelsPrepare builds the schema from today's models, which later
// migrations change, plus the tables later migrations create (laterModels).
// legacyPrepare is the legacy start as it was at the baseline: the same,
// less what later migrations added to the models (postBaselineIndexes),
// without the later tables, plus the tables later migrations dropped with
// their models (retiredKernelModels). A migration that changes a model
// records what it added in postBaselineIndexes, one that creates a table
// lists its model in laterModels, one that drops a table with its model
// keeps a frozen copy in retiredKernelModels
// (docs/reference/database-migrations.md, "Changing a model").

// legacySchemaModels is cmd/server's schemaModels() at 4eb87485.
func legacySchemaModels() []any {
	return []any{
		&model.User{}, &model.Plan{}, &model.Order{}, &model.Payment{}, &model.PaymentLog{},
		&model.ServerVMess{}, &model.ServerVLESS{}, &model.ServerTrojan{}, &model.ServerShadowsocks{},
		&model.Node{}, &model.NodeProtocol{}, &model.WireGuardPeer{}, &model.AuthorizedKey{},
		&model.TrafficLog{}, &model.OnlineLog{}, &model.StatUser{}, &model.StatServer{}, &model.NodeLog{},
		&model.SubscriptionGroup{}, &model.SubscriptionTemplate{}, &model.UserSubscriptionGroup{}, &model.PlanSubscriptionGroup{},
		&model.Event{},
		&model.Ticket{}, &model.TicketMessage{},
		&model.Coupon{}, &model.CouponUsage{},
		&model.Knowledge{},
		&model.ForwardNode{}, &model.ForwardRule{}, &model.ForwardRoute{}, &model.ForwardLog{}, &model.ForwardStats{},
		&model.ForwardTunnel{}, &model.ForwardUserTunnel{}, &model.Forward{}, &model.ForwardPortBinding{}, &model.SpeedLimit{},
		&model.ForwardRuntimeJob{}, &model.ForwardTrafficCursor{}, &model.ForwardCleanAgent{}, &model.ForwardAgentBridgeTask{},
		&model.ForwardLatencyBucket{},
		&model.PaymentGateway{}, &model.PaymentRecord{},
		&model.TelegramBot{}, &model.TelegramUser{}, &model.TelegramChat{}, &model.TelegramCommand{}, &model.TelegramNotification{},
		&model.NotificationTemplate{}, &model.NotificationLog{},
		&model.UserMFA{}, &model.MFALoginAttempt{},
		&model.UserLevel{}, &model.InviteCode{}, &model.CommissionRecord{}, &model.CommissionWithdraw{}, &model.InviteConfig{},
		&model.SystemConfig{}, &keptBackupRecord{}, &keptBackupConfig{}, &model.OperationLog{}, &model.AuditLog{},
	}
}

// keptBackupRecord is model.BackupRecord as the baseline created it. The
// model went with the old backup service (Control's backups are v5_backup);
// its table stays in the schema, unused, until a cleanup migration drops
// it, so both the baseline's start and today's create it.
type keptBackupRecord struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"size:100"`
	Type      string `gorm:"size:20"`
	Size      int64
	Path      string `gorm:"size:255"`
	Status    int    `gorm:"default:0"`
	Error     string `gorm:"type:text"`
	Auto      bool   `gorm:"default:false"`
	CreatedBy *uint

	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (keptBackupRecord) TableName() string { return "v2_backup_record" }

// keptBackupConfig is model.BackupConfig as the baseline created it: the
// schedule, retention and S3 settings nothing executed. Kept like
// keptBackupRecord.
type keptBackupConfig struct {
	ID             uint   `gorm:"primaryKey"`
	Enabled        bool   `gorm:"default:false"`
	AutoBackup     bool   `gorm:"default:false"`
	Schedule       string `gorm:"size:50"`
	RetentionDays  int    `gorm:"default:7"`
	BackupDatabase bool   `gorm:"default:true"`
	BackupFiles    bool   `gorm:"default:false"`
	StorageType    string `gorm:"size:20;default:local"`
	StoragePath    string `gorm:"size:255"`
	S3Bucket       string `gorm:"size:100"`
	S3Region       string `gorm:"size:50"`
	S3Endpoint     string `gorm:"size:255"`
	S3AccessKey    string `gorm:"size:100"`
	S3SecretKey    string `gorm:"size:100"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (keptBackupConfig) TableName() string { return "v2_backup_config" }

// postBaselineIndexes are the indexes later migrations added to the models:
// the legacy start of the baseline did not create them. legacyPrepare drops
// them after building from the models, so the databases it prepares are the
// baseline's and the adoption tests keep describing databases earlier
// releases left (an index is dropped here; a column a migration adds would
// be dropped the same way, after its index).
var postBaselineIndexes = []struct{ table, index string }{
	{"v2_server_log", "idx_v2_server_log_bucket"}, // 0002_server_log_buckets
}

// laterModels are the models of the tables later migrations create: the
// legacy start never had them, so legacyPrepare does not create them and
// legacyModelsPrepare does.
func laterModels() []any {
	return []any{
		&model.ACMECacheEntry{},            // 0003_acme_cache
		&model.Backup{},                    // 0005_backups
		&model.DestinationStat{},           // 0006_destination_stats
		&model.EgressRoute{},               // 0007_egress_routes
		&model.RealityTargetProbe{},        // 0008_reality_target_probes
		&model.RealityTargetAuto{},         // 0009_reality_target_auto
		&model.NodeEntry{},                 // 0010_node_entries
		&model.SubscriptionGroupProtocol{}, // 0011_subscription_group_protocols
		&model.ForwardRouteSubscription{},  // 0012_forward_route_subscriptions
		&model.IWANLine{},                  // 0013_iwan_lines
		&model.KernelForwardLatency{},      // 0014_forward_latency
		&model.KernelForwardDNSVantage{},   // 0015_forward_dns_vantage
		&model.NetworkModel{},              // 0016_network_model
		&model.NetworkTransportKey{},       // 0016_network_model
		&model.NetworkDevice{},             // 0016_network_model
		&model.NodeNetwork{},               // 0018_node_network
		&model.ReverseProxy{},              // 0022_reverse_proxies
		&model.ReverseProxyACMEAccount{},   // 0022_reverse_proxies
		&model.NodeUplinks{},               // 0023_node_uplinks
	}
}

// retiredRouteModeRevision is model.RouteModeRevision as the baseline
// created it; 0004_drop_route_modes dropped the table and the model.
type retiredRouteModeRevision struct {
	ID             uint      `gorm:"primaryKey"`
	GroupID        string    `gorm:"size:36;not null;index"`
	PackageID      string    `gorm:"size:120;not null;index"`
	RouteID        string    `gorm:"size:191;not null"`
	Action         string    `gorm:"size:16;not null"`
	FromMode       string    `gorm:"size:16;not null"`
	ToMode         string    `gorm:"size:16;not null"`
	ActorUserID    uint      `gorm:"not null;default:0"`
	Actor          string    `gorm:"size:191;not null;default:''"`
	Reason         string    `gorm:"type:text;not null;default:''"`
	ConfigRevision int64     `gorm:"not null"`
	CreatedAt      time.Time `gorm:"not null;index"`
}

func (retiredRouteModeRevision) TableName() string { return "v4_kernel_route_mode_revision" }

// retiredShadowMismatchSample is model.ShadowMismatchSample as the
// baseline created it; 0004_drop_route_modes dropped the table and the
// model.
type retiredShadowMismatchSample struct {
	ID             uint      `gorm:"primaryKey"`
	PackageID      string    `gorm:"size:120;not null;uniqueIndex:idx_v4_shadow_sample_key,priority:1;index:idx_v4_shadow_sample_route,priority:1"`
	SampleID       string    `gorm:"size:32;not null;uniqueIndex:idx_v4_shadow_sample_key,priority:2"`
	RouteID        string    `gorm:"size:191;not null;index:idx_v4_shadow_sample_route,priority:2"`
	PackageVersion string    `gorm:"size:64;not null;default:''"`
	Method         string    `gorm:"size:16;not null;default:''"`
	Path           string    `gorm:"size:512;not null;default:''"`
	LegacyStatus   int       `gorm:"not null;default:0"`
	NativeStatus   int       `gorm:"not null;default:0"`
	DiffJSON       string    `gorm:"type:text;not null"`
	DiffTruncated  bool      `gorm:"not null;default:false"`
	RequestID      string    `gorm:"size:128;not null;default:''"`
	ObservedAt     time.Time `gorm:"not null;index;index:idx_v4_shadow_sample_route,priority:3"`
	CreatedAt      time.Time `gorm:"not null"`
}

func (retiredShadowMismatchSample) TableName() string { return "v4_kernel_shadow_mismatch_sample" }

// retiredLoadBalancer is model.LoadBalancer as the baseline created it;
// 0021_drop_load_balancer dropped the table and the model.
type retiredLoadBalancer struct {
	ID            uint   `gorm:"primaryKey"`
	Name          string `gorm:"size:100"`
	GroupID       uint   `gorm:"index"`
	Strategy      string `gorm:"size:20;default:round-robin"`
	HealthCheck   bool   `gorm:"default:true"`
	CheckInterval int    `gorm:"default:60"`
	CheckTimeout  int    `gorm:"default:10"`
	NodeWeights   string `gorm:"type:text"`
	Enabled       bool   `gorm:"default:true"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (retiredLoadBalancer) TableName() string { return "v2_load_balancer" }

// retiredKernelModels are the kernel models later migrations dropped with
// their tables, frozen as the baseline had them: the legacy start of the
// baseline created them, today's models do not. legacyPrepare creates them,
// legacyModelsPrepare does not (a regenerated baseline file would list them
// after the other kernel tables).
func retiredKernelModels() []any {
	return []any{
		&retiredRouteModeRevision{}, &retiredShadowMismatchSample{}, // 0004_drop_route_modes
		&retiredLoadBalancer{}, // 0021_drop_load_balancer
	}
}

// baselineNodeGroup is model.NodeGroup as the baseline created it, with the
// kind, the protocol template and the subscription groups that
// 0020_node_groups_without_templates dropped with the model's fields.
type baselineNodeGroup struct {
	ID                   uint      `gorm:"primaryKey"`
	Name                 string    `gorm:"size:100;not null;uniqueIndex:ux_v5_node_group_live_name,where:deleted = 0"`
	Kind                 string    `gorm:"size:16;not null"`
	JoinTokenHash        string    `gorm:"size:64;not null;uniqueIndex"`
	JoinTokenSealed      string    `gorm:"type:text;not null"`
	TokenResetAt         time.Time `gorm:"not null"`
	TokenExpiresAt       *time.Time
	MaxNodes             int                   `gorm:"not null;default:0"`
	RequireApproval      bool                  `gorm:"not null;default:false"`
	Rate                 float64               `gorm:"not null;default:1"`
	Tags                 string                `gorm:"type:text"`
	MonthlyLimitBytes    int64                 `gorm:"not null;default:0"`
	ResetDay             int                   `gorm:"not null;default:1"`
	Sort                 int                   `gorm:"not null;default:0"`
	ProtocolTemplate     string                `gorm:"type:text"`
	SubscriptionGroupIDs string                `gorm:"type:text"`
	Deleted              soft_delete.DeletedAt `gorm:"column:deleted;not null;default:0;softDelete:flag,DeletedAtField:DeletedAt"`
	DeletedAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (baselineNodeGroup) TableName() string { return "v5_node_group" }

// legacyPrepare is the legacy start's schema work at the baseline on db.
// production is the env: "production" only creates missing tables of the
// application models, development and test AutoMigrate them all. The
// Ensure* steps run in both.
func legacyPrepare(t *testing.T, db *gorm.DB, production bool) {
	t.Helper()
	require.NoError(t, legacyPrepareErr(db, production))
	require.NoError(t, db.AutoMigrate(retiredKernelModels()...))
	// 0020_node_groups_without_templates dropped columns of v5_node_group:
	// the baseline's table is built from the frozen copy.
	require.NoError(t, db.Migrator().DropTable(&model.NodeGroup{}))
	require.NoError(t, db.AutoMigrate(&baselineNodeGroup{}))
	for _, added := range postBaselineIndexes {
		require.NoError(t, db.Exec("DROP INDEX "+added.index).Error)
	}
}

// legacyModelsPrepare is the legacy start's schema work with today's
// models, plus the tables later migrations create: the schema
// TestFreshSchemaEqualsTheLegacySchema compares with the migrations', so a
// model changed or added without a migration fails there.
func legacyModelsPrepare(t *testing.T, db *gorm.DB, production bool) {
	t.Helper()
	require.NoError(t, legacyPrepareErr(db, production))
	require.NoError(t, db.AutoMigrate(laterModels()...))
}

func legacyPrepareErr(db *gorm.DB, production bool) error {
	migrator := db.Migrator()
	// forwardlegacy.EnsureSchema
	for _, value := range model.ForwardLegacyModels() {
		if !migrator.HasTable(value) {
			if err := migrator.CreateTable(value); err != nil {
				return err
			}
		}
	}
	// migrateSchema
	models := forwardlegacy.KeepModels(db, legacySchemaModels())
	if production {
		var missing []any
		for _, value := range models {
			if !migrator.HasTable(value) {
				missing = append(missing, value)
			}
		}
		if len(missing) == len(models) {
			if err := db.AutoMigrate(models...); err != nil {
				return err
			}
		} else {
			for _, value := range missing {
				if err := migrator.CreateTable(value); err != nil {
					return err
				}
			}
		}
	} else if err := db.AutoMigrate(models...); err != nil {
		return err
	}
	// EnsureWireGuardPeerSchema, EnsureNodeRuntimeHealthSchema
	if err := db.AutoMigrate(&model.WireGuardPeer{}); err != nil {
		return err
	}
	if err := db.AutoMigrate(&model.Node{}); err != nil {
		return err
	}
	// EnsureKernelSchema (its seeding is SeedKernelDefaults now)
	if err := db.AutoMigrate(model.KernelModels()...); err != nil {
		return err
	}
	if migrator.HasIndex(&model.Topology{}, model.LegacyTopologyNameIndex) {
		if err := migrator.DropIndex(&model.Topology{}, model.LegacyTopologyNameIndex); err != nil {
			return err
		}
	}
	if !migrator.HasIndex(&model.Topology{}, model.TopologyLiveNameIndex) {
		if err := migrator.CreateIndex(&model.Topology{}, model.TopologyLiveNameIndex); err != nil {
			return err
		}
	}
	// nodesecrets.EnsureSchema (its state rows are EnsureStateRows now)
	if err := db.AutoMigrate(&model.NodeCredential{}, &model.ProtocolSecret{}, &model.NodeSecretSplit{}); err != nil {
		return err
	}
	// lease.EnsureSchema
	if err := db.AutoMigrate(&lease.Record{}); err != nil {
		return err
	}
	// EnsureObservabilitySchema
	if !migrator.HasTable(&model.ForwardLatencyBucket{}) {
		if err := db.AutoMigrate(&model.ForwardLatencyBucket{}); err != nil {
			return err
		}
	}
	// EnsureStatsSchema
	for _, value := range []any{&model.TrafficLog{}, &model.OnlineLog{}, &model.StatUser{}, &model.StatServer{}} {
		if err := db.AutoMigrate(value); err != nil {
			return err
		}
	}
	if !forwardlegacy.Dropped(db) {
		// EnsureForwardBridgeSchema
		if !migrator.HasTable(&model.ForwardAgentBridgeTask{}) {
			if err := db.AutoMigrate(&model.ForwardAgentBridgeTask{}); err != nil {
				return err
			}
		}
		// EnsureForwardRuntimeJobSchema (its duplicate repair is adoption's)
		if err := db.AutoMigrate(&model.ForwardRuntimeJob{}); err != nil {
			return err
		}
		if !migrator.HasIndex(&model.ForwardRuntimeJob{}, "idx_forward_runtime_job_active_forward") {
			if err := db.Exec(fmt.Sprintf(
				"CREATE UNIQUE INDEX IF NOT EXISTS idx_forward_runtime_job_active_forward ON %s (forward_id) WHERE forward_id IS NOT NULL AND status IN (%d, %d)",
				model.ForwardRuntimeJob{}.TableName(), model.ForwardRuntimeJobStatusPending, model.ForwardRuntimeJobStatusRunning,
			)).Error; err != nil {
				return err
			}
		}
		// EnsureForwardPortBindingSchema (its backfill runs at every start)
		if err := db.AutoMigrate(&model.ForwardPortBinding{}); err != nil {
			return err
		}
	}
	// EnsureForwardNodeColumns
	for _, field := range []string{"MetricsPort", "NodeGroupID", "MachineFingerprint", "Approval", "LastPublicAddress"} {
		if !migrator.HasColumn(&model.ForwardNode{}, field) {
			if err := migrator.AddColumn(&model.ForwardNode{}, field); err != nil {
				return err
			}
		}
	}
	for _, field := range []string{"NodeGroupID", "MachineFingerprint"} {
		if !migrator.HasIndex(&model.ForwardNode{}, field) {
			if err := migrator.CreateIndex(&model.ForwardNode{}, field); err != nil {
				return err
			}
		}
	}
	// EnsureAgentDiagnosticTaskSchema
	if !migrator.HasTable(&model.AgentDiagnosticTask{}) {
		if err := db.AutoMigrate(&model.AgentDiagnosticTask{}); err != nil {
			return err
		}
	}
	return nil
}

// ddlRecorder keeps the schema statements a GORM session executes.
type ddlRecorder struct {
	logger.Interface
	statements *[]string
}

func (r ddlRecorder) LogMode(logger.LogLevel) logger.Interface { return r }

func (r ddlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), err error) {
	if err != nil {
		return
	}
	statement, _ := fc()
	upper := strings.ToUpper(strings.TrimSpace(statement))
	for _, prefix := range []string{"CREATE ", "ALTER ", "DROP "} {
		if strings.HasPrefix(upper, prefix) {
			*r.statements = append(*r.statements, strings.TrimSpace(statement))
			return
		}
	}
}

var (
	recordedTable = regexp.MustCompile("^CREATE TABLE [`\"]?([A-Za-z0-9_]+)[`\"]? \\((.*)\\)$")
	recordedIndex = regexp.MustCompile("^CREATE (?:UNIQUE )?INDEX (?:IF NOT EXISTS )?[`\"]?([A-Za-z0-9_]+)[`\"]? ON [`\"]?([A-Za-z0-9_]+)[`\"]?")
)

// orderBaseline makes the recorded statements deterministic: GORM emits a
// table's indexes and foreign keys in map order. The tables keep the order
// they were created in (GORM orders them by dependency); the constraints of
// a table are sorted; the indexes follow all tables, by table and name.
func orderBaseline(t *testing.T, recorded []string) []string {
	t.Helper()
	var tables []string
	indexes := map[string][]string{}
	indexName := map[string]string{}
	for _, statement := range recorded {
		if match := recordedTable.FindStringSubmatch(statement); match != nil {
			parts, err := migrations.SplitTopLevel(match[2])
			require.NoError(t, err)
			var columns, constraints []string
			for _, part := range parts {
				if strings.HasPrefix(part, "CONSTRAINT ") {
					constraints = append(constraints, part)
				} else {
					columns = append(columns, part)
				}
			}
			sort.Strings(constraints)
			prefix := strings.TrimSuffix(statement, "("+match[2]+")")
			tables = append(tables, prefix+"("+strings.Join(append(columns, constraints...), ",")+")")
			continue
		}
		match := recordedIndex.FindStringSubmatch(statement)
		require.NotNil(t, match, "the legacy start executed a statement that is neither a table nor an index: %s", statement)
		indexes[match[2]] = append(indexes[match[2]], statement)
		indexName[statement] = match[1]
	}
	ordered := append([]string(nil), tables...)
	for _, table := range tables {
		name := recordedTable.FindStringSubmatch(table)[1]
		list := indexes[name]
		sort.Slice(list, func(i, j int) bool { return indexName[list[i]] < indexName[list[j]] })
		ordered = append(ordered, list...)
		delete(indexes, name)
	}
	require.Empty(t, indexes, "indexes on tables the legacy start did not create")
	return ordered
}

// TestWriteBaselineFromTheLegacyOracle is how sql/0001_baseline.*.sql were
// produced: the statements the legacy start executed on an empty database.
// It only runs when ANIX_MIGRATIONS_WRITE_BASELINE names a directory, and
// it writes there, never into sql/: a released migration is never
// regenerated. The PostgreSQL file needs ANIX_TEST_POSTGRES_DSN.
func TestWriteBaselineFromTheLegacyOracle(t *testing.T) {
	dir := strings.TrimSpace(os.Getenv("ANIX_MIGRATIONS_WRITE_BASELINE"))
	if dir == "" {
		t.Skip("ANIX_MIGRATIONS_WRITE_BASELINE is not set")
	}
	headers := map[string]string{
		migrations.DialectSQLite:   "SQLite",
		migrations.DialectPostgres: "PostgreSQL",
	}
	forEachDialect(t, func(t *testing.T, open opener) {
		db := open(t)
		dialect := migrations.DialectSQLite
		if db.Name() == "postgres" {
			dialect = migrations.DialectPostgres
		}
		var recorded []string
		session := db.Session(&gorm.Session{Logger: ddlRecorder{Interface: db.Logger, statements: &recorded}})
		legacyPrepare(t, session, true)
		var body strings.Builder
		fmt.Fprintf(&body, `-- 0001_baseline (%s): the schema a fresh AnixOps Control created before
-- the migration runner (go_dev 4eb87485, release 4.2.1): the tables and
-- indexes of the application models, the kernel models and the startup
-- schema steps, in the order that start created them, flux tables included
-- (they go with the forwarding cleanup). The kernel API views are not part
-- of it: packagestore creates them at every start.
--
-- Recorded once from that start on an empty database (legacy_oracle_test.go)
-- and never edited: every database records this file's checksum
-- (docs/reference/database-migrations.md).
`, headers[dialect])
		for _, statement := range orderBaseline(t, recorded) {
			require.NotContains(t, statement, "\n", "one statement per line")
			body.WriteString(statement + ";\n")
		}
		path := filepath.Join(dir, "0001_baseline."+dialect+".sql")
		require.NoError(t, os.WriteFile(path, []byte(body.String()), 0o644))
		t.Logf("wrote %s (%d statements)", path, len(recorded))
	})
}
