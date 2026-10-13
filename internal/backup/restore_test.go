package backup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// holdDatabaseEnv makes the test binary a helper process that keeps a
// SQLite database open, the way a running Control (or a package host) does.
const holdDatabaseEnv = "ANIX_BACKUP_TEST_HOLD_DATABASE"

func TestMain(m *testing.M) {
	if path := os.Getenv(holdDatabaseEnv); path != "" {
		os.Exit(holdDatabase(path))
	}
	os.Exit(m.Run())
}

func holdDatabase(path string) int {
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		fmt.Println("error", err)
		return 1
	}
	var count int64
	if err := db.Raw("SELECT count(*) FROM sqlite_master").Scan(&count).Error; err != nil {
		fmt.Println("error", err)
		return 1
	}
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin) // until the test closes stdin
	return 0
}

// target is where a restore writes: an empty data directory, database
// directory and configuration directory.
type target struct {
	dataDir, dbPath, configPath string
}

func newTarget(t *testing.T) target {
	t.Helper()
	root := t.TempDir()
	return target{dataDir: filepath.Join(root, "data"), dbPath: filepath.Join(root, "db", "control.db"), configPath: filepath.Join(root, "etc", "config.yaml")}
}

func (tg target) options(bundle string) RestoreOptions {
	return RestoreOptions{
		Bundle: bundle, Driver: "sqlite", DataDir: tg.dataDir, DatabasePath: tg.dbPath, ConfigPath: tg.configPath,
		ProgramVersion: testVersion, Now: func() time.Time { return time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC) },
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func userCount(t *testing.T, path string) int64 {
	t.Helper()
	db, closeDB, err := openSnapshot(path)
	require.NoError(t, err)
	defer closeDB()
	var count int64
	require.NoError(t, db.Raw("SELECT count(*) FROM v2_user").Scan(&count).Error)
	return count
}

func requireRestoreRefused(t *testing.T, err error, contains string) {
	t.Helper()
	var refusal *RefusedError
	require.True(t, errors.As(err, &refusal), "want a refusal, got %v", err)
	assert.Contains(t, err.Error(), "nothing was restored")
	assert.Contains(t, err.Error(), contains)
}

func TestRestoreIntoAnEmptyTarget(t *testing.T) {
	p := newPanel(t)
	bundle := p.write(t, TypeDaily, time.Now())
	tg := newTarget(t)

	result, err := Restore(context.Background(), tg.options(bundle.Path))
	require.NoError(t, err)
	assert.Equal(t, bundle.Manifest, result.Manifest)
	assert.Equal(t, []string{tg.dbPath, rootkey.Path(tg.dataDir), tg.configPath}, result.Written)
	assert.Empty(t, result.ReplacedDir)
	for _, path := range result.Written {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), path)
	}
	assert.Equal(t, int64(2), userCount(t, tg.dbPath))
	assert.Equal(t, p.keyFile, readFile(t, rootkey.Path(tg.dataDir)))
	assert.Equal(t, p.config, readFile(t, tg.configPath))
	assert.Empty(t, ls(t, filepath.Join(tg.dataDir, "tmp")), "the extracted copy is removed")
}

func TestRestoreRefusesANonEmptyTarget(t *testing.T) {
	p := newPanel(t)
	bundle := p.write(t, TypeDaily, time.Now())
	for _, existing := range []string{"database", "wal", "root.key", "config"} {
		t.Run(existing, func(t *testing.T) {
			tg := newTarget(t)
			path := map[string]string{"database": tg.dbPath, "wal": tg.dbPath + "-wal", "root.key": rootkey.Path(tg.dataDir), "config": tg.configPath}[existing]
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte("existing"), 0o600))

			_, err := Restore(context.Background(), tg.options(bundle.Path))
			requireRestoreRefused(t, err, "the target is not empty: "+path)
			assert.Contains(t, err.Error(), "--replace")
			assert.Equal(t, []byte("existing"), readFile(t, path), "nothing was changed")
		})
	}
}

// A configuration file that already is the bundle's is not in the way.
func TestRestoreKeepsAnIdenticalConfiguration(t *testing.T) {
	p := newPanel(t)
	bundle := p.write(t, TypeDaily, time.Now())
	tg := newTarget(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(tg.configPath), 0o700))
	require.NoError(t, os.WriteFile(tg.configPath, p.config, 0o640))
	result, err := Restore(context.Background(), tg.options(bundle.Path))
	require.NoError(t, err)
	assert.True(t, result.ConfigUnchanged)
	assert.Equal(t, []string{tg.dbPath, rootkey.Path(tg.dataDir)}, result.Written)
}

// --replace moves what is in the way into replaced-<UTC time>/ and never
// deletes anything.
func TestRestoreReplaceMovesAside(t *testing.T) {
	p := newPanel(t)
	bundle := p.write(t, TypeDaily, time.Now())
	tg := newTarget(t)
	old := map[string][]byte{
		tg.dbPath:                []byte("the old database"),
		tg.dbPath + "-wal":       []byte("its log"),
		rootkey.Path(tg.dataDir): []byte("the old root key"),
		tg.configPath:            []byte("domain: old.example.com\n"),
	}
	for path, data := range old {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, data, 0o600))
	}
	options := tg.options(bundle.Path)
	options.Replace = true
	result, err := Restore(context.Background(), options)
	require.NoError(t, err)

	aside := filepath.Join(tg.dataDir, "replaced-20261013T093000Z")
	assert.Equal(t, aside, result.ReplacedDir)
	info, err := os.Stat(aside)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	assert.ElementsMatch(t, []string{"control.db", "control.db-wal", "root.key", "config.yaml"}, ls(t, aside))
	assert.Equal(t, old[tg.dbPath], readFile(t, filepath.Join(aside, "control.db")))
	assert.Equal(t, old[tg.dbPath+"-wal"], readFile(t, filepath.Join(aside, "control.db-wal")))
	assert.Equal(t, old[rootkey.Path(tg.dataDir)], readFile(t, filepath.Join(aside, "root.key")))
	assert.Equal(t, old[tg.configPath], readFile(t, filepath.Join(aside, "config.yaml")))
	assert.Len(t, result.Moved, 4)

	assert.Equal(t, int64(2), userCount(t, tg.dbPath))
	assert.NoFileExists(t, tg.dbPath+"-wal", "the old log is not left beside the restored database")
	assert.Equal(t, p.keyFile, readFile(t, rootkey.Path(tg.dataDir)))
	assert.Equal(t, p.config, readFile(t, tg.configPath))
}

func TestRestoreRefusesWhileControlRuns(t *testing.T) {
	p := newPanel(t)
	bundle := p.write(t, TypeDaily, time.Now())
	tg := newTarget(t)
	require.NoError(t, os.MkdirAll(tg.dataDir, 0o700))
	running, err := LockService(tg.dataDir)
	require.NoError(t, err)
	_, err = Restore(context.Background(), tg.options(bundle.Path))
	requireRestoreRefused(t, err, "the AnixOps Control service is running on this data directory")
	require.ErrorIs(t, err, ErrServiceRunning)
	assert.NoFileExists(t, tg.dbPath)
	running.Release()

	// A Control start finds the restore's exclusive lock.
	restoring, err := LockExclusive(tg.dataDir)
	require.NoError(t, err)
	_, err = LockService(tg.dataDir)
	require.ErrorIs(t, err, ErrRestoreRunning)
	restoring.Release()
	held, err := LockService(tg.dataDir)
	require.NoError(t, err)
	held.Release()
	info, err := os.Stat(filepath.Join(tg.dataDir, LockFileName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "whoever created it, every Control can open it")

	// Without a data directory there is nothing to lock.
	missing, err := LockService(filepath.Join(t.TempDir(), "absent"))
	require.NoError(t, err)
	missing.Release()
	assert.NoDirExists(t, filepath.Join(t.TempDir(), "absent"))
}

// Another process with the database open (a Control without the lock, a
// package host left behind) is found through SQLite's own locks.
func TestRestoreRefusesADatabaseAnotherProcessHasOpen(t *testing.T) {
	p := newPanel(t)
	bundle := p.write(t, TypeDaily, time.Now())
	tg := newTarget(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(tg.dbPath), 0o700))
	require.NoError(t, Snapshot(context.Background(), p.dbPath, tg.dbPath))

	helper := exec.Command(os.Args[0], "-test.run=^$")
	helper.Env = append(os.Environ(), holdDatabaseEnv+"="+tg.dbPath)
	stdin, err := helper.StdinPipe()
	require.NoError(t, err)
	stdout, err := helper.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, helper.Start())
	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready\n", line)

	options := tg.options(bundle.Path)
	options.Replace = true
	_, err = Restore(context.Background(), options)
	requireRestoreRefused(t, err, "another process has the database "+tg.dbPath+" open")
	assert.NoDirExists(t, filepath.Join(tg.dataDir, "replaced-20261013T093000Z"), "nothing was moved")

	require.NoError(t, stdin.Close())
	require.NoError(t, helper.Wait())
	_, err = Restore(context.Background(), options)
	require.NoError(t, err, "the database is free once the process ended")
}

// A bundle whose database a newer Control migrated is refused with the
// start's own message.
func TestRestoreRefusesABundleOfANewerControl(t *testing.T) {
	p := newPanel(t)
	steps, err := migrations.Steps()
	require.NoError(t, err)
	require.NoError(t, p.db.Create(&migrations.MigrationRecord{
		Version: len(steps) + 1, Name: "from_the_future", Checksum: "sha256:00", AppliedAt: time.Now(), ProgramVersion: "5.1.0",
	}).Error)
	bundle := p.write(t, TypeManual, time.Now())
	tg := newTarget(t)
	_, err = Restore(context.Background(), tg.options(bundle.Path))
	requireRestoreRefused(t, err, "this database was migrated by a newer AnixOps Control")
	assert.Contains(t, err.Error(), "applied by Control 5.1.0")
	var newer *migrations.RefusedError
	assert.True(t, errors.As(err, &newer))
	assert.NoFileExists(t, tg.dbPath)
}

func TestRestoreRefusesADamagedBundle(t *testing.T) {
	p := newPanel(t)
	bundle := p.write(t, TypeDaily, time.Now())
	tamper(t, bundle.Path, func(name string, data []byte) []byte {
		if name == MemberDatabase {
			return flipPages(name, data)
		}
		return data
	}, true, nil)
	tg := newTarget(t)
	_, err := Restore(context.Background(), tg.options(bundle.Path))
	requireRestoreRefused(t, err, "the bundle failed its verification: integrity")
	assert.NoFileExists(t, tg.dbPath)
}

func TestRestoreWithoutAConfigurationFile(t *testing.T) {
	p := newPanel(t)
	bundle := p.write(t, TypeDaily, time.Now())
	tg := newTarget(t)
	options := tg.options(bundle.Path)
	options.ConfigPath = ""
	result, err := Restore(context.Background(), options)
	require.NoError(t, err)
	restored := filepath.Join(tg.dataDir, "config.yaml.restored")
	assert.Equal(t, p.config, readFile(t, restored))
	assert.Contains(t, result.ConfigNote, "-config "+restored)
}

func TestRestoreNeedsSQLite(t *testing.T) {
	p := newPanel(t)
	bundle := p.write(t, TypeDaily, time.Now())
	tg := newTarget(t)
	options := tg.options(bundle.Path)
	options.Driver = "postgres"
	_, err := Restore(context.Background(), options)
	requireRestoreRefused(t, err, "backups need SQLite")
	require.ErrorIs(t, err, ErrNeedsSQLite)
}
