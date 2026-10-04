package forwardlegacy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const postgresDSN = "ANIX_TEST_POSTGRES_DSN"

const (
	nodeToken       = "nodetoken-SECRET-1234567"
	cleanAgentToken = "cleanagent-TOKEN-7654321"
)

var start = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

// fluxModels are the flux forwarding models and what they reference.
func fluxModels() []any {
	return []any{
		&model.User{}, &model.Node{}, &model.ForwardNode{}, &model.ForwardRule{}, &model.ForwardRoute{}, &model.ForwardLog{},
		&model.ForwardStats{}, &model.ForwardTunnel{}, &model.ForwardUserTunnel{}, &model.Forward{}, &model.ForwardPortBinding{},
		&model.SpeedLimit{}, &model.ForwardRuntimeJob{}, &model.ForwardTrafficCursor{}, &model.ForwardCleanAgent{},
		&model.ForwardAgentBridgeTask{}, &model.AgentTransport{}, &model.AgentCertificate{}, &model.BackupRecord{},
	}
}

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(fluxModels()...))
	return db
}

// openPostgres creates a throwaway schema in the test database and returns
// a migrated connection whose search_path is that schema.
func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv(postgresDSN))
	if base == "" {
		t.Skip(postgresDSN + " is not set")
	}
	admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminDB.Close() })
	var databaseName string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	schema := "forward_legacy_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(fluxModels()...))
	return db
}

// seed writes a small flux forwarding setup: two forward nodes (the
// second on a clean agent), a tunnel with a forward and its grants, a
// legacy rule, and a runtime job whose payload predates NO-7.
func seed(t *testing.T, db *gorm.DB) {
	t.Helper()
	user := model.User{Email: "owner@example.com", Password: "x", UUID: "u-1", Token: "subscription-token-abcdef"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 1, Name: "hk-relay", Type: "relay", Host: "192.0.2.1", Port: 22, APIPort: 9000, APIToken: nodeToken, Enabled: true}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 2, Name: "jp-exit", Type: "exit", Host: "192.0.2.2", Port: 22, Enabled: true}).Error)
	nodeID := uint(2)
	seen := start
	require.NoError(t, db.Create(&model.ForwardCleanAgent{NodeID: &nodeID, Name: "jp", Token: cleanAgentToken, LastSeen: &seen}).Error)
	outNode := uint(2)
	tunnel := model.ForwardTunnel{ID: 5, Name: "hk-jp", InNodeID: 1, OutNodeID: &outNode, Protocol: "tcp", Status: 1}
	require.NoError(t, db.Create(&tunnel).Error)
	require.NoError(t, db.Create(&model.Forward{ID: 7, UserID: user.ID, Name: "web", TunnelID: 5, InPort: 20001, RemoteAddr: "198.51.100.7:443", RuntimeBackend: "gost"}).Error)
	require.NoError(t, db.Create(&model.ForwardUserTunnel{UserID: user.ID, TunnelID: 5, Flow: 100}).Error)
	require.NoError(t, db.Create(&model.SpeedLimit{CreatedTime: 1, UpdatedTime: 1, Name: "10M", Speed: 10, TunnelID: 5, TunnelName: "hk-jp"}).Error)
	require.NoError(t, db.Create(&model.ForwardRule{Name: "legacy", RelayNodeID: 1, ListenPort: 30000, ExitNodeID: 2, TargetHost: "198.51.100.8", TargetPort: 80}).Error)
	forwardID := uint(7)
	payload := `{"backend":"gost","panelForward":{"ingressNode":{"id":1,"apiToken":"` + nodeToken + `","host":"192.0.2.1"}},"note":"token ` + nodeToken + ` leaked"}`
	require.NoError(t, db.Create(&model.ForwardRuntimeJob{Backend: "gost", Action: "create", ForwardID: &forwardID, Payload: payload, Result: "ok " + cleanAgentToken}).Error)
}

type fakeCleaner struct {
	results map[uint]PathResult
	calls   []uint
}

func (f *fakeCleaner) CleanNode(_ context.Context, node NodeInfo) PathResult {
	f.calls = append(f.calls, node.ID)
	if result, ok := f.results[node.ID]; ok {
		return result
	}
	return PathResult{State: PathNotNeeded}
}

func TestSQLiteArchive(t *testing.T)   { runArchive(t, openSQLite(t)) }
func TestPostgresArchive(t *testing.T) { runArchive(t, openPostgres(t)) }

func runArchive(t *testing.T, db *gorm.DB) {
	seed(t, db)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "data", ArchiveDirName)

	written, err := WriteArchive(ctx, db, WriteOptions{Dir: dir, Trigger: "cli", Actor: "system/cli", ControlVersion: "4.2.0", Now: start})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "forward-legacy-archive-20261004T120000Z.json"), written.Record.Path)
	info, err := os.Stat(written.Record.Path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	assert.EqualValues(t, 1, written.Tables["v2_forward"])
	assert.EqualValues(t, 1, written.Tables["v2_forward_rule"])
	assert.EqualValues(t, 2, written.Tables["v2_forward_node"])
	assert.EqualValues(t, 0, written.Tables["v2_forward_stats"])

	// No secret in the file: not the columns, not inside a payload, not in
	// free text.
	content, err := os.ReadFile(written.Record.Path)
	require.NoError(t, err)
	for _, secret := range []string{nodeToken, cleanAgentToken, "subscription-token-abcdef"} {
		assert.NotContains(t, string(content), secret)
	}
	archive, err := VerifyArchive(&written.Record)
	require.NoError(t, err)
	assert.Equal(t, ArchiveSchema, archive.Schema)
	assert.Equal(t, "4.2.0", archive.ControlVersion)
	tables := map[string]ArchiveTable{}
	for _, table := range archive.Tables {
		tables[table.Name] = table
	}
	assert.Contains(t, tables["v2_forward_node"].RedactedColumns, "api_token")
	assert.False(t, tables["v2_forward_node"].Dropped)
	assert.Contains(t, tables["v2_forward_clean_agent"].RedactedColumns, "token")
	assert.True(t, tables["v2_forward"].Dropped)
	require.Len(t, tables["v2_forward"].Rows, 1)
	assert.Equal(t, "198.51.100.7:443", tables["v2_forward"].Rows[0]["remote_addr"])
	jobPayload, _ := tables["v2_forward_runtime_job"].Rows[0]["payload"].(string)
	assert.NotContains(t, jobPayload, "apiToken")
	assert.Contains(t, jobPayload, `"host":"192.0.2.1"`)
	assert.Contains(t, jobPayload, Redacted)

	// Never over an existing file: the same second is refused, a directory
	// gets a new timestamped file, an existing -o file is refused.
	_, err = WriteArchive(ctx, db, WriteOptions{Dir: dir, Trigger: "cli", Now: start})
	require.ErrorContains(t, err, "never overwrites")
	second, err := WriteArchive(ctx, db, WriteOptions{Output: dir, Trigger: "cli", Now: start.Add(time.Second)})
	require.NoError(t, err)
	assert.NotEqual(t, written.Record.Path, second.Record.Path)
	_, err = WriteArchive(ctx, db, WriteOptions{Output: written.Record.Path, Trigger: "cli", Now: start.Add(2 * time.Second)})
	require.ErrorContains(t, err, "never overwrites")
	unchanged, err := os.ReadFile(written.Record.Path)
	require.NoError(t, err)
	assert.Equal(t, content, unchanged)

	// The startup archive is written once, and again only when every
	// recorded file is gone.
	startupDir := filepath.Join(t.TempDir(), "startup")
	again, err := StartupArchive(ctx, db, startupDir, "4.2.0", start.Add(time.Minute))
	require.NoError(t, err)
	assert.Nil(t, again, "recorded archives exist")
	require.NoError(t, os.Remove(written.Record.Path))
	require.NoError(t, os.Remove(second.Record.Path))
	again, err = StartupArchive(ctx, db, startupDir, "4.2.0", start.Add(time.Minute))
	require.NoError(t, err)
	require.NotNil(t, again)
	assert.Equal(t, "startup", again.Record.Trigger)
	assert.True(t, strings.HasPrefix(again.Record.Path, startupDir))
	none, err := StartupArchive(ctx, db, startupDir, "4.2.0", start.Add(2*time.Minute))
	require.NoError(t, err)
	assert.Nil(t, none)

	// A changed file is no longer the archive.
	require.NoError(t, os.WriteFile(again.Record.Path, []byte("{}"), 0o600))
	_, err = VerifyArchive(&again.Record)
	require.ErrorContains(t, err, "SHA-256 mismatch")
}

func TestStartupArchiveWithoutFluxTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	written, err := StartupArchive(context.Background(), db, t.TempDir(), "4.2.0", start)
	require.NoError(t, err)
	assert.Nil(t, written, "a fresh v4.2 database has nothing to archive")
}

func TestSQLiteStatusAndAbandon(t *testing.T)   { runStatusAndAbandon(t, openSQLite(t)) }
func TestPostgresStatusAndAbandon(t *testing.T) { runStatusAndAbandon(t, openPostgres(t)) }

func runStatusAndAbandon(t *testing.T, db *gorm.DB) {
	seed(t, db)
	ctx := context.Background()
	require.NoError(t, db.Create(&model.ForwardNode{ID: 3, Name: "jp-exit", Type: "exit", Host: "192.0.2.3", Enabled: true}).Error)
	// Node 4 runs the enrolled Agent.
	require.NoError(t, db.Create(&model.ForwardNode{ID: 4, Name: "sg-agent", Type: "exit", Host: "192.0.2.4", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.AgentTransport{NodeKind: "forward", NodeID: 4, Transport: model.AgentTransportMTLSStream,
		AgentVersion: "4.2.0", FirstSeenAt: start, LastSeenAt: start}).Error)

	statuses, err := Status(ctx, db)
	require.NoError(t, err)
	require.Len(t, statuses, 4)
	for _, status := range statuses {
		assert.Equal(t, StateUnchecked, status.State)
	}

	nodeX := &fakeCleaner{results: map[uint]PathResult{1: {State: StateClean, Detail: "NodeX deleted 2 gost resource(s)", Items: 2}}}
	ansible := &fakeCleaner{results: map[uint]PathResult{3: {State: StateUnreachable, Detail: "192.0.2.3 did not answer over SSH"}}}
	statuses, err = Check(ctx, db, CheckOptions{NodeX: nodeX, Ansible: ansible, Now: start})
	require.NoError(t, err)
	byRef := map[string]NodeStatus{}
	for _, status := range statuses {
		byRef[status.Ref] = status
	}
	assert.Equal(t, StateClean, byRef["forward-1"].State, byRef["forward-1"].Detail)
	// The clean agent node without the new Agent cannot be verified.
	assert.Equal(t, StateUnreachable, byRef["forward-2"].State)
	assert.Contains(t, byRef["forward-2"].Detail, "clean-agent")
	assert.Equal(t, StateUnreachable, byRef["forward-3"].State)
	assert.Equal(t, StateClean, byRef["forward-4"].State)
	assert.Contains(t, byRef["forward-4"].Detail, "enrolled AnixOps Agent 4.2.0")
	assert.Len(t, byRef["forward-4"].Checks, 3)

	// Abandoning: by a unique name or forward-<id>, with a reason, and only
	// an unreachable node.
	_, err = Abandon(ctx, db, "jp-exit", "gone", "system/cli", start)
	require.ErrorContains(t, err, "2 forward nodes are named")
	_, err = Abandon(ctx, db, "forward-3", " ", "system/cli", start)
	require.ErrorContains(t, err, "reason is required")
	_, err = Abandon(ctx, db, "hk-relay", "gone", "system/cli", start)
	require.ErrorIs(t, err, ErrNotUnreachable)
	_, err = Abandon(ctx, db, "forward-9", "gone", "system/cli", start)
	require.ErrorContains(t, err, "no forward node")
	abandoned, err := Abandon(ctx, db, "forward-3", "machine returned to the provider", "system/cli", start)
	require.NoError(t, err)
	assert.Equal(t, StateAbandoned, abandoned.State)
	assert.Equal(t, "machine returned to the provider", abandoned.AbandonReason)
	assert.True(t, abandoned.Ready())

	// A later check that finds the node unreachable keeps the abandonment;
	// one that finds it dirty clears it.
	_, err = Check(ctx, db, CheckOptions{NodeX: nodeX, Ansible: ansible, Nodes: []string{"forward-3"}, Now: start.Add(time.Minute)})
	require.NoError(t, err)
	statuses, err = Status(ctx, db)
	require.NoError(t, err)
	assert.Equal(t, StateAbandoned, statuses[2].State)
	ansible.results[3] = PathResult{State: StateDirty, Detail: "inet v2b_forward still there"}
	_, err = Check(ctx, db, CheckOptions{NodeX: nodeX, Ansible: ansible, Nodes: []string{"forward-3"}, Now: start.Add(2 * time.Minute)})
	require.NoError(t, err)
	statuses, err = Status(ctx, db)
	require.NoError(t, err)
	assert.Equal(t, StateDirty, statuses[2].State)
	assert.Nil(t, statuses[2].AbandonedAt)
}

func TestSQLiteDrop(t *testing.T)   { runDrop(t, openSQLite(t)) }
func TestPostgresDrop(t *testing.T) { runDrop(t, openPostgres(t)) }

func runDrop(t *testing.T, db *gorm.DB) {
	seed(t, db)
	ctx := context.Background()
	require.NoError(t, EnsureSchema(db))
	now := start
	backup := filepath.Join(t.TempDir(), "control.dump")
	require.NoError(t, os.WriteFile(backup, []byte("-- PostgreSQL database dump"), 0o600))
	require.NoError(t, os.Chtimes(backup, now, now))
	drop := func(confirm, backupPath string) (*DropResult, error) {
		return Drop(ctx, db, DropOptions{Confirm: confirm, BackupPath: backupPath, Actor: "system/cli", Now: now, LockTimeout: 2 * time.Second})
	}
	refusal := func(err error) string {
		var refused *RefusedError
		require.ErrorAs(t, err, &refused)
		return err.Error()
	}

	// Nothing ready: every reason at once.
	_, err := drop("drop the tables", "")
	message := refusal(err)
	assert.Contains(t, message, "confirmation phrase is wrong")
	assert.Contains(t, message, "no archive")
	assert.Contains(t, message, "neither clean nor abandoned")
	assert.Contains(t, message, "no database backup")

	// An archive, then a node check: one node is dirty.
	_, err = WriteArchive(ctx, db, WriteOptions{Dir: t.TempDir(), Trigger: "cli", Now: now})
	require.NoError(t, err)
	cleaners := CheckOptions{
		NodeX:   &fakeCleaner{results: map[uint]PathResult{1: {State: StateClean, Detail: "deleted"}}},
		Ansible: &fakeCleaner{results: map[uint]PathResult{2: {State: StateDirty, Detail: "ip v2b_forward still there"}}},
		Now:     now,
	}
	_, err = Check(ctx, db, cleaners)
	require.NoError(t, err)
	_, err = drop(ConfirmPhrase, backup)
	message = refusal(err)
	assert.NotContains(t, message, "confirmation phrase")
	assert.NotContains(t, message, "no archive")
	assert.Contains(t, message, "forward-2 (jp-exit) dirty")
	_, err = Abandon(ctx, db, "forward-2", "gone", "system/cli", now)
	require.ErrorIs(t, err, ErrNotUnreachable, "a dirty node is cleaned, not abandoned")

	// The node is now unreachable and abandoned; still no backup.
	cleaners.Ansible = &fakeCleaner{results: map[uint]PathResult{2: {State: StateUnreachable, Detail: "no SSH"}}}
	_, err = Check(ctx, db, cleaners)
	require.NoError(t, err)
	_, err = Abandon(ctx, db, "forward-2", "decommissioned", "system/cli", now)
	require.NoError(t, err)
	_, err = drop(ConfirmPhrase, "")
	assert.Contains(t, refusal(err), "no database backup")
	_, err = drop(ConfirmPhrase, filepath.Join(t.TempDir(), "missing.dump"))
	assert.Contains(t, refusal(err), "not readable")
	old := filepath.Join(t.TempDir(), "old.dump")
	require.NoError(t, os.WriteFile(old, []byte("x"), 0o600))
	require.NoError(t, os.Chtimes(old, now.Add(-48*time.Hour), now.Add(-48*time.Hour)))
	_, err = drop(ConfirmPhrase, old)
	assert.Contains(t, refusal(err), "older than")
	_, err = drop(ConfirmPhrase+" ", backup)
	assert.Contains(t, refusal(err), "confirmation phrase is wrong")

	// The flux data changed since the archive.
	require.NoError(t, db.Create(&model.ForwardRule{Name: "late", RelayNodeID: 1, ListenPort: 30001, ExitNodeID: 2, TargetHost: "198.51.100.9", TargetPort: 80}).Error)
	_, err = drop(ConfirmPhrase, backup)
	assert.Contains(t, refusal(err), "changed since the archive (v2_forward_rule: 2 rows, archived 1)")
	_, err = WriteArchive(ctx, db, WriteOptions{Dir: t.TempDir(), Trigger: "cli", Now: now})
	require.NoError(t, err)

	// A running Control holds the singleton lease.
	require.NoError(t, db.Exec("CREATE TABLE v4_kernel_lease (name varchar(100) PRIMARY KEY, holder varchar(200), expires_at timestamp, acquired_at timestamp, renewed_at timestamp)").Error)
	require.NoError(t, db.Exec("INSERT INTO v4_kernel_lease (name, holder, expires_at) VALUES (?, ?, ?)", SingletonLease, "control-1", now.Add(20*time.Second)).Error)
	_, err = drop(ConfirmPhrase, backup)
	assert.Contains(t, refusal(err), "Control is running on this database")
	require.NoError(t, db.Exec("UPDATE v4_kernel_lease SET expires_at = ?", now.Add(-time.Second)).Error)

	// F5d (or another release) already removed a table: it is reported.
	require.NoError(t, db.Exec(`DROP TABLE "v2_forward_stats"`).Error)

	pre, err := CheckPreconditions(ctx, db, PreconditionOptions{BackupPath: backup, Now: now})
	require.NoError(t, err)
	require.Empty(t, pre.Blockers)
	result, err := drop(ConfirmPhrase, backup)
	require.NoError(t, err)
	assert.Equal(t, []string{"v2_forward_stats"}, result.AlreadyMissing)
	assert.Len(t, result.Dropped, len(DropTables)-1)
	assert.Equal(t, "operator", result.Backup.Source)
	for _, table := range DropTables {
		assert.False(t, db.Migrator().HasTable(table), table)
	}
	for _, table := range KeptTables {
		assert.True(t, db.Migrator().HasTable(table), table)
	}
	assert.True(t, Dropped(db))
	var recorded model.ForwardLegacyDrop
	require.NoError(t, db.First(&recorded).Error)
	var dropped []string
	require.NoError(t, json.Unmarshal([]byte(recorded.Dropped), &dropped))
	assert.Contains(t, dropped, "v2_forward")
	assert.Contains(t, recorded.Backup, backup)

	// After the drop the schema steps leave the tables dropped.
	kept := KeepModels(db, fluxModels())
	for _, value := range kept {
		named, ok := value.(tableNamer)
		if ok {
			assert.False(t, isDropTable(named.TableName()), named.TableName())
		}
	}
	require.NoError(t, db.AutoMigrate(kept...))
	ran := false
	require.NoError(t, UnlessDropped(func(*gorm.DB) error { ran = true; return nil })(db))
	assert.False(t, ran)
	for _, table := range DropTables {
		assert.False(t, db.Migrator().HasTable(table), table)
	}

	// And it never runs twice.
	_, err = drop(ConfirmPhrase, backup)
	assert.Contains(t, refusal(err), "already dropped")
	written, err := StartupArchive(ctx, db, t.TempDir(), "4.2.0", now)
	require.NoError(t, err)
	assert.Nil(t, written)
}

func TestDropAcceptsControlBackupRecord(t *testing.T) {
	db := openSQLite(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&model.ForwardNode{ID: 1, Name: "agent", Host: "192.0.2.1", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.AgentTransport{NodeKind: "forward", NodeID: 1, Transport: model.AgentTransportMTLSStream, FirstSeenAt: start, LastSeenAt: start}).Error)
	_, err := WriteArchive(ctx, db, WriteOptions{Dir: t.TempDir(), Trigger: "cli", Now: start})
	require.NoError(t, err)
	_, err = Check(ctx, db, CheckOptions{Now: start})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "backup_20261004_115900.db")
	require.NoError(t, os.WriteFile(path, []byte("SQLite format 3"), 0o600))
	require.NoError(t, os.Chtimes(path, start, start))
	completed := start.Add(-time.Minute)
	// A failed backup and a files-only backup do not count.
	require.NoError(t, db.Create(&model.BackupRecord{Name: "failed", Type: "database", Status: 2, Path: path, CompletedAt: &completed}).Error)
	require.NoError(t, db.Create(&model.BackupRecord{Name: "files", Type: "files", Status: 1, Path: path, CompletedAt: &completed}).Error)
	_, err = Drop(ctx, db, DropOptions{Confirm: ConfirmPhrase, Now: start})
	require.ErrorContains(t, err, "no database backup")
	record := model.BackupRecord{Name: "ok", Type: "database", Status: 1, Path: path, CompletedAt: &completed}
	require.NoError(t, db.Create(&record).Error)
	result, err := Drop(ctx, db, DropOptions{Confirm: ConfirmPhrase, Now: start})
	require.NoError(t, err)
	assert.Equal(t, "backup_record", result.Backup.Source)
	assert.Equal(t, record.ID, result.Backup.RecordID)
}

// TestPostgresDropIsAtomic: a table another session holds makes the drop
// fail within its lock timeout, and nothing is dropped.
func TestPostgresDropIsAtomic(t *testing.T) {
	db := openPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&model.ForwardNode{ID: 1, Name: "agent", Host: "192.0.2.1", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.AgentTransport{NodeKind: "forward", NodeID: 1, Transport: model.AgentTransportMTLSStream, FirstSeenAt: start, LastSeenAt: start}).Error)
	_, err := WriteArchive(ctx, db, WriteOptions{Dir: t.TempDir(), Trigger: "cli", Now: start})
	require.NoError(t, err)
	_, err = Check(ctx, db, CheckOptions{Now: start})
	require.NoError(t, err)
	backup := filepath.Join(t.TempDir(), "control.dump")
	require.NoError(t, os.WriteFile(backup, []byte("dump"), 0o600))
	require.NoError(t, os.Chtimes(backup, start, start))

	reader := db.Begin()
	require.NoError(t, reader.Error)
	require.NoError(t, reader.Exec(`LOCK TABLE "v2_forward_rule" IN ACCESS SHARE MODE`).Error)
	began := time.Now()
	_, err = Drop(ctx, db, DropOptions{Confirm: ConfirmPhrase, BackupPath: backup, Now: start, LockTimeout: 300 * time.Millisecond})
	require.ErrorContains(t, err, "nothing was dropped")
	assert.Less(t, time.Since(began), 5*time.Second)
	require.NoError(t, reader.Rollback().Error)
	for _, table := range DropTables {
		assert.True(t, db.Migrator().HasTable(table), table)
	}
	assert.False(t, Dropped(db))
}
