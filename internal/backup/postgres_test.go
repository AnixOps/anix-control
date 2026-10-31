package backup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPostgresFromConfig(t *testing.T) {
	pg, err := PostgresFromConfig(&config.DatabaseConfig{Driver: "postgres", Host: "db.internal", Port: 5433, Username: "anix", Password: "p'w", Database: "panel", SSLMode: "require"})
	require.NoError(t, err)
	assert.Equal(t, "sslmode=require dbname='panel' user='anix' host='db.internal' port=5433 password='p\\'w'", pg.dsn("panel"))
	assert.Contains(t, pg.env("panel"), "PGPASSWORD=p'w")

	pg, err = PostgresFromConfig(&config.DatabaseConfig{Driver: "postgres", DSN: "host=10.0.0.5 port=5432 user=kernel password=secret dbname=anix sslmode=verify-full TimeZone=UTC"})
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.5", pg.Host)
	assert.Equal(t, "anix", pg.Database)
	assert.Equal(t, "verify-full", pg.SSLMode)

	_, err = PostgresFromConfig(&config.DatabaseConfig{Driver: "postgres"})
	require.Error(t, err, "a database name is required")
}

func TestPostgresMissingToolsAreReported(t *testing.T) {
	pg := &Postgres{Database: "panel", DumpBin: "pg_dump_that_does_not_exist"}
	require.ErrorIs(t, pg.CheckTools(), ErrPostgresTools)
}

func TestPostgresPanelNeedsAConnection(t *testing.T) {
	m := &Manager{Driver: "postgres", DataDir: t.TempDir()}
	require.ErrorIs(t, m.Supported(), ErrNeedsSQLite)
	m.Postgres = &Postgres{Database: "panel"}
	require.NoError(t, m.Supported())
}

func TestDatabaseMemberName(t *testing.T) {
	assert.Equal(t, MemberDatabase, DatabaseMemberName(""))
	assert.Equal(t, MemberDatabase, DatabaseMemberName(KindSQLite))
	assert.Equal(t, MemberDatabasePG, DatabaseMemberName(KindPostgres))
	assert.Equal(t, KindSQLite, (&Manifest{}).Kind())
}

// pgFixture is a migrated PostgreSQL database of its own on the server
// ANIX_TEST_POSTGRES_DSN names (the role needs CREATEDB), with a data
// directory and root key.
type pgFixture struct {
	pg      *Postgres
	admin   *Postgres
	dataDir string
	keyFile []byte
	db      *gorm.DB
}

func newPGFixture(t *testing.T) *pgFixture {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	for _, bin := range []string{"pg_dump", "pg_restore"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not installed", bin)
		}
	}
	parsed, err := pgconn.ParseConfig(dsn)
	require.NoError(t, err)
	admin := &Postgres{Host: parsed.Host, Port: int(parsed.Port), User: parsed.User, Password: parsed.Password, Database: parsed.Database}
	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	name := "anix_backup_test_" + hex.EncodeToString(suffix)
	adminDB, err := admin.open(admin.Database)
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminDB.Close() })
	_, err = adminDB.Exec("CREATE DATABASE " + quoteIdent(name))
	require.NoError(t, err)
	copyPG := *admin
	copyPG.Database = name
	f := &pgFixture{pg: &copyPG, admin: admin, dataDir: filepath.Join(t.TempDir(), "data")}
	t.Cleanup(func() {
		rows, err := adminDB.Query("SELECT datname FROM pg_database WHERE datname = $1 OR datname LIKE $2", name, name+"\\_replaced\\_%")
		if err != nil {
			return
		}
		var names []string
		for rows.Next() {
			var n string
			_ = rows.Scan(&n)
			names = append(names, n)
		}
		_ = rows.Close()
		for _, n := range names {
			_, _ = adminDB.Exec("DROP DATABASE IF EXISTS " + quoteIdent(n) + " WITH (FORCE)")
		}
	})

	require.NoError(t, os.MkdirAll(f.dataDir, 0o700))
	key, err := rootkey.Generate()
	require.NoError(t, err)
	f.keyFile = rootkey.File{Key: key}.Marshal()
	require.NoError(t, os.WriteFile(rootkey.Path(f.dataDir), f.keyFile, 0o600))

	f.db, err = database.Open(&config.DatabaseConfig{Driver: "postgres", Host: f.pg.Host, Port: f.pg.Port, Username: f.pg.User, Password: f.pg.Password, Database: name, LogLevel: "silent"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.CloseHandle(f.db) })
	ctx := context.Background()
	_, err = migrations.Migrate(ctx, f.db, migrations.Options{ProgramVersion: testVersion})
	require.NoError(t, err)
	authority, err := modulepki.New(modulepki.Options{DB: f.db, Cluster: "default", KEK: key.Bytes()})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(ctx))
	require.NoError(t, f.db.Create(&model.User{ID: 1, Email: "a@example.com", Password: "x", UUID: "u-1", Token: "t-1"}).Error)
	require.NoError(t, f.db.Create(&model.User{ID: 2, Email: "b@example.com", Password: "x", UUID: "u-2", Token: "t-2"}).Error)
	return f
}

func (f *pgFixture) source() Source {
	return Source{
		DataDir: f.dataDir, Postgres: f.pg, RootKey: f.keyFile, Config: []byte("domain: panel.example.com\n"),
		Domain: "panel.example.com", ProgramVersion: testVersion, HostName: "panel-host",
	}
}

// A PostgreSQL bundle is written from one snapshot, verified by restoring
// it into a scratch database (which is dropped again) and restored into an
// empty database; a database with tables is refused and, with Replace,
// renamed aside, never deleted.
func TestPostgresBundleRoundTrip(t *testing.T) {
	ctx := context.Background()
	f := newPGFixture(t)

	bundle, err := Write(ctx, f.source(), TypeManual, time.Now())
	require.NoError(t, err)
	assert.Equal(t, KindPostgres, bundle.Manifest.Kind())
	assert.Equal(t, int64(2), bundle.Manifest.RowCounts["v2_user"])
	names := []string{}
	for _, member := range readMembers(t, bundle.Path) {
		names = append(names, member.header.Name)
	}
	assert.Equal(t, []string{MemberManifest, MemberDatabasePG, MemberRootKey, MemberConfig}, names)

	options := VerifyOptions{ScratchDir: ScratchDir(f.dataDir), ProgramVersion: testVersion, Postgres: f.pg}
	_, err = VerifyFile(ctx, bundle.Path, options)
	require.NoError(t, err)
	var leftovers int
	adminDB, err := f.admin.open(f.admin.Database)
	require.NoError(t, err)
	defer func() { _ = adminDB.Close() }()
	require.NoError(t, adminDB.QueryRow("SELECT count(*) FROM pg_database WHERE datname LIKE 'anix_verify\\_%'").Scan(&leftovers))
	assert.Zero(t, leftovers, "the scratch database is dropped")

	options.Postgres = nil
	_, err = VerifyFile(ctx, bundle.Path, options)
	require.ErrorIs(t, err, ErrPostgresBundle)

	// Restore into a database that does not exist yet.
	targetName := f.pg.Database + "_restored"
	target := *f.pg
	target.Database = targetName
	t.Cleanup(func() { _, _ = adminDB.Exec("DROP DATABASE IF EXISTS " + quoteIdent(targetName) + " WITH (FORCE)") })
	targetDir := filepath.Join(t.TempDir(), "data")
	result, err := Restore(ctx, RestoreOptions{Bundle: bundle.Path, Driver: "postgres", Postgres: &target, DataDir: targetDir, ProgramVersion: testVersion})
	require.NoError(t, err)
	assert.Contains(t, result.Written, "PostgreSQL database "+targetName)
	assert.Equal(t, f.keyFile, readFile(t, rootkey.Path(targetDir)))
	restored, err := database.Open(&config.DatabaseConfig{Driver: "postgres", Host: target.Host, Port: target.Port, Username: target.User, Password: target.Password, Database: targetName, LogLevel: "silent"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.CloseHandle(restored) })
	var users int64
	require.NoError(t, restored.Model(&model.User{}).Count(&users).Error)
	assert.Equal(t, int64(2), users)
	require.NoError(t, database.CloseHandle(restored))

	// A second restore into the now non-empty database is refused ...
	_, err = Restore(ctx, RestoreOptions{Bundle: bundle.Path, Driver: "postgres", Postgres: &target, DataDir: filepath.Join(t.TempDir(), "data"), ProgramVersion: testVersion})
	requireRestoreRefused(t, err, "nothing was restored")
	assert.Contains(t, err.Error(), "is not empty")
	// ... and with Replace the old database is renamed, not deleted.
	result, err = Restore(ctx, RestoreOptions{
		Bundle: bundle.Path, Driver: "postgres", Postgres: &target, DataDir: filepath.Join(t.TempDir(), "data"), ProgramVersion: testVersion,
		Replace: true, Now: func() time.Time { return time.Date(2026, 10, 31, 1, 0, 0, 0, time.UTC) },
	})
	require.NoError(t, err)
	require.Len(t, result.Moved, 1)
	assert.Contains(t, result.Moved[0], targetName+"_replaced_20261031T010000Z")
	var renamed int
	require.NoError(t, adminDB.QueryRow("SELECT count(*) FROM pg_database WHERE datname = $1", targetName+"_replaced_20261031T010000Z").Scan(&renamed))
	assert.Equal(t, 1, renamed)
	t.Cleanup(func() {
		_, _ = adminDB.Exec("DROP DATABASE IF EXISTS " + quoteIdent(targetName+"_replaced_20261031T010000Z") + " WITH (FORCE)")
	})
}

// A bundle whose root.key does not belong to the database fails its
// verification, as on SQLite.
func TestPostgresVerificationCatchesAForeignRootKey(t *testing.T) {
	ctx := context.Background()
	f := newPGFixture(t)
	other, err := rootkey.Generate()
	require.NoError(t, err)
	source := f.source()
	source.RootKey = rootkey.File{Key: other}.Marshal()
	bundle, err := Write(ctx, source, TypeManual, time.Now())
	require.NoError(t, err)
	_, err = VerifyFile(ctx, bundle.Path, VerifyOptions{ScratchDir: ScratchDir(f.dataDir), ProgramVersion: testVersion, Postgres: f.pg})
	var failed *VerifyError
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, ProblemRootKey, failed.Kind)
}
