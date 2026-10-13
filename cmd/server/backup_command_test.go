package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/backup"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// backupFixture is an installation: a configuration file naming the data
// directory and the SQLite database, root.key, and a database with an
// administrator, a user, a node and the module CA sealed with the key.
type backupFixture struct {
	root, dataDir, dbPath, configPath string
	key                               rootkey.Key
}

// newBackupFixture prepares the database with the schema prepare gives it.
func newBackupFixture(t *testing.T, prepare func(t *testing.T, db *gorm.DB)) *backupFixture {
	t.Helper()
	root := t.TempDir()
	f := &backupFixture{root: root, dataDir: filepath.Join(root, "data"), dbPath: filepath.Join(root, "db", "control.db"), configPath: filepath.Join(root, "etc", "config.yaml")}
	require.NoError(t, os.MkdirAll(filepath.Dir(f.configPath), 0o700))
	require.NoError(t, os.MkdirAll(f.dataDir, 0o700))
	require.NoError(t, os.WriteFile(f.configPath, []byte(fmt.Sprintf("domain: panel.example.com\ndata_dir: %s\ndatabase:\n  driver: sqlite\n  database: %s\n", f.dataDir, f.dbPath)), 0o600))
	key, err := rootkey.Generate()
	require.NoError(t, err)
	f.key = key
	require.NoError(t, os.WriteFile(rootkey.Path(f.dataDir), rootkey.File{Key: key}.Marshal(), 0o600))

	db, err := database.Open(&config.DatabaseConfig{Driver: "sqlite", Database: f.dbPath, LogLevel: "silent"})
	require.NoError(t, err)
	defer func() { _ = database.CloseHandle(db) }()
	prepare(t, db)
	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: "default", KEK: key.Bytes()})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(context.Background()))
	require.NoError(t, db.Exec("INSERT INTO v2_user (email, password, uuid, token, is_admin, created_at, updated_at) VALUES ('admin@example.com', 'x', 'u-1', 't-1', 1, datetime('now'), datetime('now')), ('user@example.com', 'x', 'u-2', 't-2', 0, datetime('now'), datetime('now'))").Error)
	require.NoError(t, db.Exec("INSERT INTO v2_node (name, host, port) VALUES ('hk-01', 'hk-01.example.com', 443)").Error)
	return f
}

// migrated is the current schema.
func migrated(t *testing.T, db *gorm.DB) {
	t.Helper()
	_, err := migrations.Migrate(context.Background(), db, migrations.Options{ProgramVersion: version})
	require.NoError(t, err)
}

// released420 is the schema Control 4.2.0 left in an empty SQLite file
// (internal/database/migrations/testdata/releases): an older program's
// database, without the migration runner's tables.
func released420(t *testing.T, db *gorm.DB) {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "internal", "database", "migrations", "testdata", "releases", "v4.2.0-production.sqlite.sql.gz"))
	require.NoError(t, err)
	defer func() { _ = file.Close() }()
	reader, err := gzip.NewReader(file)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	var statement strings.Builder
	for _, line := range strings.Split(string(body), "\n") {
		if statement.Len() == 0 && (strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "--")) {
			continue
		}
		statement.WriteString(line + "\n")
		if strings.HasSuffix(strings.TrimSpace(line), ";") {
			require.NoError(t, db.Exec(statement.String()).Error, statement.String())
			statement.Reset()
		}
	}
}

func (f *backupFixture) run(t *testing.T, arguments ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code, handled := runBackupCommand(append([]string{"backup"}, arguments...), &stdout, &stderr)
	require.True(t, handled)
	return code, stdout.String(), stderr.String()
}

func (f *backupFixture) bundles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(f.dataDir, "backups"))
	require.NoError(t, err)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func countRows(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Table(table).Count(&count).Error)
	return count
}

func TestBackupCommandUsage(t *testing.T) {
	f := newBackupFixture(t, migrated)
	for _, arguments := range [][]string{nil, {"frobnicate"}, {"restore"}, {"restore", "a.tar", "b.tar"}, {"create", "-type", "weekly"}, {"list", "extra"}} {
		code, _, stderr := f.run(t, arguments...)
		assert.Equal(t, backupExitUsage, code, "%v", arguments)
		assert.Contains(t, stderr, "usage:")
	}
	code, _, stderr := f.run(t, "list", "-config", filepath.Join(f.root, "missing.yaml"))
	assert.Equal(t, backupExitUsage, code)
	assert.Contains(t, stderr, "config file not found")
	_, handled := runBackupCommand([]string{"config", "check"}, io.Discard, io.Discard)
	assert.False(t, handled)
}

// create writes and verifies a bundle and records it; list shows it;
// verify checks it again or any file, and fails a damaged one.
func TestBackupCommandCreateListVerify(t *testing.T) {
	f := newBackupFixture(t, migrated)
	code, stdout, stderr := f.run(t, "create", "-config", f.configPath)
	require.Equal(t, backupExitOK, code, stderr)
	assert.Contains(t, stdout, "Wrote "+filepath.Join(f.dataDir, "backups", "anix-control-"))
	assert.Contains(t, stdout, "it restores (passed)")
	names := f.bundles(t)
	require.Len(t, names, 1)
	assert.True(t, strings.HasSuffix(names[0], "-manual.tar"))

	code, stdout, _ = f.run(t, "list", "-config", f.configPath)
	require.Equal(t, backupExitOK, code)
	assert.Contains(t, stdout, names[0])
	assert.Contains(t, stdout, "passed")
	code, stdout, _ = f.run(t, "list", "-json", "-config", f.configPath)
	require.Equal(t, backupExitOK, code)
	assert.Contains(t, stdout, `"verify_status": "passed"`)

	code, stdout, stderr = f.run(t, "verify", "-config", f.configPath)
	require.Equal(t, backupExitOK, code, stderr)
	assert.Contains(t, stdout, "Verified "+names[0])

	// A copy elsewhere is verified and nothing is recorded; a damaged one
	// fails with status 1.
	elsewhere := filepath.Join(f.root, "copy.tar")
	data, err := os.ReadFile(filepath.Join(f.dataDir, "backups", names[0]))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(elsewhere, data, 0o600))
	code, stdout, stderr = f.run(t, "verify", elsewhere, "-config", f.configPath)
	require.Equal(t, backupExitOK, code, stderr)
	assert.Contains(t, stdout, "Verified "+elsewhere)
	require.NoError(t, os.WriteFile(elsewhere, data[:len(data)/2], 0o600))
	code, _, stderr = f.run(t, "verify", elsewhere, "-config", f.configPath)
	assert.Equal(t, backupExitFailed, code)
	assert.Contains(t, stderr, "archive:")

	code, _, stderr = f.run(t, "create", "-type", "verify", "-config", f.configPath)
	require.Equal(t, backupExitOK, code, stderr)
	assert.Len(t, f.bundles(t), 2)
}

// A running Control (the shared lock) does not stop a backup; a restore in
// progress (the exclusive lock) does.
func TestBackupCommandLocks(t *testing.T) {
	f := newBackupFixture(t, migrated)
	running, err := backup.LockService(f.dataDir)
	require.NoError(t, err)
	code, _, stderr := f.run(t, "create", "-config", f.configPath)
	require.Equal(t, backupExitOK, code, stderr)
	bundle := filepath.Join(f.dataDir, "backups", f.bundles(t)[0])

	code, _, stderr = f.run(t, "restore", bundle, "--replace", "-config", f.configPath)
	assert.Equal(t, backupExitRefused, code)
	assert.Contains(t, stderr, "the AnixOps Control service is running on this data directory")
	running.Release()

	restoring, err := backup.LockExclusive(f.dataDir)
	require.NoError(t, err)
	code, _, stderr = f.run(t, "create", "-config", f.configPath)
	assert.Equal(t, backupExitRefused, code)
	assert.Contains(t, stderr, "a backup restore is running")
	restoring.Release()
}

func TestBackupCommandOnPostgreSQL(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(fmt.Sprintf("data_dir: %s\ndatabase:\n  driver: postgres\n  host: 127.0.0.1\n  database: anix_control\n", root)), 0o600))
	f := &backupFixture{root: root}
	for _, arguments := range [][]string{{"create"}, {"list"}, {"verify"}, {"restore", filepath.Join(root, "a.tar")}} {
		code, _, stderr := f.run(t, append(arguments, "-config", configPath)...)
		assert.Equal(t, backupExitRefused, code, "%v", arguments)
		assert.Contains(t, stderr, "backups need SQLite", "%v", arguments)
	}
}

// A bundle whose database a newer Control migrated is refused with the
// start's own message, and nothing changes.
func TestBackupCommandRefusesABundleOfANewerControl(t *testing.T) {
	f := newBackupFixture(t, func(t *testing.T, db *gorm.DB) {
		migrated(t, db)
		steps, err := migrations.Steps()
		require.NoError(t, err)
		require.NoError(t, db.Create(&migrations.MigrationRecord{
			Version: len(steps) + 1, Name: "from_the_future", Checksum: "sha256:00", AppliedAt: time.Now(), ProgramVersion: "5.1.0",
		}).Error)
	})
	source, err := os.ReadFile(f.dbPath)
	require.NoError(t, err)
	_, err = backup.Write(context.Background(), backup.Source{
		DataDir: f.dataDir, DatabasePath: f.dbPath, RootKey: rootkey.File{Key: f.key}.Marshal(), ProgramVersion: "5.1.0",
	}, backup.TypeManual, time.Now())
	require.NoError(t, err)
	bundle := filepath.Join(f.dataDir, "backups", f.bundles(t)[0])
	code, _, stderr := f.run(t, "restore", bundle, "--replace", "-config", f.configPath)
	assert.Equal(t, backupExitRefused, code)
	assert.Contains(t, stderr, "this database was migrated by a newer AnixOps Control")
	assert.Contains(t, stderr, "Control 5.1.0")
	after, err := os.ReadFile(f.dbPath)
	require.NoError(t, err)
	assert.Equal(t, source, after, "nothing changed")
}

// The whole path: a bundle of a database an older Control (4.2.0) left is
// taken, restored over a damaged installation with --replace, and the next
// start through the real bootstrap migrates it (after a pre-migration
// backup): the administrator, the user and the node are there and the
// restored root key opens the restored CA.
func TestRestoreThenStartMigrates(t *testing.T) {
	ctx := context.Background()
	f := newBackupFixture(t, released420)
	code, stdout, stderr := f.run(t, "create", "-config", f.configPath)
	require.Equal(t, backupExitOK, code, stderr)
	assert.Contains(t, stdout, "schema 0000", "Control 4.2.0 had no migration records")
	bundle := filepath.Join(f.dataDir, "backups", f.bundles(t)[0])

	// The installation breaks: another key, an empty database.
	other, err := rootkey.Generate()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(rootkey.Path(f.dataDir), rootkey.File{Key: other}.Marshal(), 0o600))
	require.NoError(t, os.WriteFile(f.dbPath, nil, 0o600))

	code, _, stderr = f.run(t, "restore", bundle, "-config", f.configPath)
	assert.Equal(t, backupExitRefused, code)
	assert.Contains(t, stderr, "the target is not empty")
	code, stdout, stderr = f.run(t, "restore", bundle, "--replace", "-config", f.configPath)
	require.Equal(t, backupExitOK, code, stderr)
	assert.Contains(t, stdout, "Moved aside into "+filepath.Join(f.dataDir, "replaced-"))
	assert.Contains(t, stdout, "the configuration "+f.configPath+" already was the bundle's")

	// Start: the configuration, the root key and the database as the
	// server reads them.
	cfg, _, err := config.LoadFile(f.configPath, nil)
	require.NoError(t, err)
	cfg.Database.LogLevel = "silent"
	saved := activeRootKey
	t.Cleanup(func() { activeRootKey = saved })
	activeRootKey, err = resolveRootKey(cfg, true, t.Logf)
	require.NoError(t, err)
	assert.True(t, activeRootKey.Key.Equal(f.key), "the restored root key")
	savedDir := forwardLegacyDataDir
	forwardLegacyDataDir = t.TempDir()
	t.Cleanup(func() { forwardLegacyDataDir = savedDir })
	savedManager := backup.Default()
	t.Cleanup(func() { backup.SetDefault(savedManager) })
	release := setupBackups(cfg, f.configPath, true)
	t.Cleanup(release)
	_ = database.Close()
	require.NoError(t, database.Init(&cfg.Database))
	t.Cleanup(func() { _ = database.Close() })

	require.NoError(t, bootstrapDatabase(ctx, cfg))
	db := database.Get()
	steps, err := migrations.Steps()
	require.NoError(t, err)
	assert.Equal(t, int64(len(steps)), countRows(t, db, migrations.MigrationTable), "every migration ran")
	assert.Equal(t, int64(2), countRows(t, db, "v2_user"))
	assert.Equal(t, int64(1), countRows(t, db, "v2_node"))

	// The start took a pre-migration bundle of the restored database; it
	// verifies.
	var preMigration string
	for _, name := range f.bundles(t) {
		if strings.HasSuffix(name, "-pre-migration.tar") {
			preMigration = name
		}
	}
	require.NotEmpty(t, preMigration, "%v", f.bundles(t))
	row, found, err := backup.Default().VerifyNewestUnverified(ctx)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, preMigration, row.Path)
	assert.Equal(t, model.BackupVerifyPassed, row.VerifyStatus)
	assert.Equal(t, 0, row.SchemaVersion)
}

// A pre-migration backup that fails stops the migration; an empty
// database needs none.
func TestPreMigrationBackupFailureStopsTheStart(t *testing.T) {
	ctx := context.Background()
	f := newBackupFixture(t, released420)
	cfg, _, err := config.LoadFile(f.configPath, nil)
	require.NoError(t, err)
	cfg.Database.LogLevel = "silent"
	savedManager := backup.Default()
	t.Cleanup(func() { backup.SetDefault(savedManager) })
	backup.SetDefault(newBackupManager(cfg, f.configPath, database.Get))
	// backups/ cannot be a directory.
	require.NoError(t, os.WriteFile(filepath.Join(f.dataDir, "backups"), []byte("in the way"), 0o600))
	_ = database.Close()
	require.NoError(t, database.Init(&cfg.Database))
	t.Cleanup(func() { _ = database.Close() })

	err = bootstrapDatabase(ctx, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the backup taken before migrating the database failed, so nothing was migrated")
	assert.False(t, database.Get().Migrator().HasTable(migrations.MigrationTable), "nothing was migrated")

	// A new database: no backup, the migrations run.
	empty := filepath.Join(f.root, "db", "empty.db")
	cfg.Database.Database = empty
	backup.SetDefault(newBackupManager(cfg, f.configPath, database.Get))
	_ = database.Close()
	require.NoError(t, database.Init(&cfg.Database))
	savedDir := forwardLegacyDataDir
	forwardLegacyDataDir = t.TempDir()
	t.Cleanup(func() { forwardLegacyDataDir = savedDir })
	cfg.ModuleRuntime.CAKEK = f.key.Base64()
	require.NoError(t, database.Get().Exec("SELECT 1").Error)
	_ = bootstrapDatabase(ctx, cfg) // the first administrator needs a password or a claim; the schema is what counts
	assert.True(t, database.Get().Migrator().HasTable(&model.Backup{}), "the migrations ran without a backup")
}
