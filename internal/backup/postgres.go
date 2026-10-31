package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver.
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Database kinds a bundle can hold (Manifest.DatabaseKind).
const (
	// KindSQLite is a VACUUM INTO snapshot (database.sqlite). A manifest
	// without a kind is SQLite: every bundle written before PostgreSQL
	// backups existed.
	KindSQLite = "sqlite"
	// KindPostgres is a pg_dump custom-format archive (database.pgdump).
	KindPostgres = "postgres"
)

// MemberDatabasePG is the PostgreSQL dump in a bundle.
const MemberDatabasePG = "database.pgdump"

// DatabaseMemberName is the member that holds the database of a bundle of
// the given kind.
func DatabaseMemberName(kind string) string {
	if kind == KindPostgres {
		return MemberDatabasePG
	}
	return MemberDatabase
}

// ErrPostgresTools answers a PostgreSQL backup when pg_dump or pg_restore
// is not installed on the machine Control runs on.
var ErrPostgresTools = errors.New("PostgreSQL backups need the PostgreSQL client tools (pg_dump and pg_restore, the same major version as the server or newer) on this machine: install the postgresql-client package")

// ErrPostgresBundle answers the verification or restore of a PostgreSQL
// bundle by a Control that is not configured for PostgreSQL.
var ErrPostgresBundle = errors.New("the bundle holds a PostgreSQL database: verify and restore it with a Control configured for PostgreSQL")

// Postgres is how backups reach a PostgreSQL server: the connection the
// panel itself uses and the client tools.
type Postgres struct {
	Host     string
	Port     int
	User     string
	Password string
	// Database is the panel's database.
	Database string
	// SSLMode is the libpq sslmode; "" means disable.
	SSLMode string

	// DumpBin and RestoreBin default to pg_dump and pg_restore.
	DumpBin    string
	RestoreBin string
}

var sslModePattern = regexp.MustCompile(`(?i)sslmode[=]['"]?([a-z-]+)`)

// PostgresFromConfig reads the connection of a PostgreSQL configuration. A
// configured dsn is parsed; the fields otherwise.
func PostgresFromConfig(cfg *config.DatabaseConfig) (*Postgres, error) {
	if cfg == nil {
		return nil, errors.New("no database configuration")
	}
	pg := &Postgres{Host: cfg.Host, Port: cfg.Port, User: cfg.Username, Password: cfg.Password, Database: cfg.Database, SSLMode: cfg.SSLMode}
	if dsn := strings.TrimSpace(cfg.DSN); dsn != "" {
		parsed, err := pgconn.ParseConfig(dsn)
		if err != nil {
			return nil, fmt.Errorf("database.dsn: %w", err)
		}
		pg = &Postgres{Host: parsed.Host, Port: int(parsed.Port), User: parsed.User, Password: parsed.Password, Database: parsed.Database, SSLMode: "prefer"}
		if match := sslModePattern.FindStringSubmatch(dsn); match != nil {
			pg.SSLMode = strings.ToLower(match[1])
		}
	}
	if pg.Database == "" {
		return nil, errors.New("the PostgreSQL database name is empty")
	}
	return pg, nil
}

func (p *Postgres) dumpBin() string {
	if p.DumpBin != "" {
		return p.DumpBin
	}
	return "pg_dump"
}

func (p *Postgres) restoreBin() string {
	if p.RestoreBin != "" {
		return p.RestoreBin
	}
	return "pg_restore"
}

// CheckTools answers ErrPostgresTools when pg_dump or pg_restore is
// missing.
func (p *Postgres) CheckTools() error {
	for _, bin := range []string{p.dumpBin(), p.restoreBin()} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("%w (%s was not found)", ErrPostgresTools, bin)
		}
	}
	return nil
}

func quoteConnValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return "'" + strings.ReplaceAll(value, "'", `\'`) + "'"
}

// dsn is a keyword/value connection string for database/sql (pgx) to the
// named database.
func (p *Postgres) dsn(database string) string {
	mode := p.SSLMode
	if mode == "" {
		mode = "disable"
	}
	parts := []string{"sslmode=" + mode, "dbname=" + quoteConnValue(database), "user=" + quoteConnValue(p.User)}
	if p.Host != "" {
		parts = append(parts, "host="+quoteConnValue(p.Host))
	}
	if p.Port > 0 {
		parts = append(parts, "port="+strconv.Itoa(p.Port))
	}
	if p.Password != "" {
		parts = append(parts, "password="+quoteConnValue(p.Password))
	}
	return strings.Join(parts, " ")
}

// env is the environment of a client tool connecting to database: the
// password never goes on a command line.
func (p *Postgres) env(database string) []string {
	mode := p.SSLMode
	if mode == "" {
		mode = "disable"
	}
	env := append(os.Environ(), "PGDATABASE="+database, "PGUSER="+p.User, "PGSSLMODE="+mode, "PGCONNECT_TIMEOUT=15")
	if p.Host != "" {
		env = append(env, "PGHOST="+p.Host)
	}
	if p.Port > 0 {
		env = append(env, "PGPORT="+strconv.Itoa(p.Port))
	}
	if p.Password != "" {
		env = append(env, "PGPASSWORD="+p.Password)
	}
	return env
}

func (p *Postgres) open(database string) (*sql.DB, error) {
	db, err := sql.Open("pgx", p.dsn(database))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// run runs a client tool and answers its error with the tool's own words.
func run(ctx context.Context, env []string, bin string, args ...string) error {
	command := exec.CommandContext(ctx, bin, args...) // #nosec G204 -- a fixed client tool with arguments Control built.
	command.Env = env
	var stderr bytes.Buffer
	command.Stderr = &limitedBuffer{buf: &stderr, limit: 4096}
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("%s: %s", bin, message)
	}
	return nil
}

// limitedBuffer keeps the first limit bytes written to it and discards the
// rest.
type limitedBuffer struct {
	buf   *bytes.Buffer
	limit int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.limit - l.buf.Len(); room > 0 {
		if len(p) > room {
			l.buf.Write(p[:room])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}

// dumpDatabase writes a consistent pg_dump custom-format archive of the
// panel's database to destination and answers the facts of the very same
// state. It opens a REPEATABLE READ transaction on a connection of its own,
// exports its snapshot and hands that snapshot to pg_dump (--snapshot), so
// the row counts in the manifest are exactly the state in the dump even
// while the panel keeps writing. dir is the bundle directory whose free
// space is checked against the database's size.
func (p *Postgres) dumpDatabase(ctx context.Context, destination, dir string) (snapshotFacts, error) {
	if err := p.CheckTools(); err != nil {
		return snapshotFacts{}, err
	}
	if _, err := os.Lstat(destination); err == nil {
		return snapshotFacts{}, fmt.Errorf("the snapshot %s already exists", destination)
	}
	pool, err := p.open(p.Database)
	if err != nil {
		return snapshotFacts{}, fmt.Errorf("open the PostgreSQL connection: %w", err)
	}
	defer func() { _ = pool.Close() }()
	tx, err := pool.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return snapshotFacts{}, fmt.Errorf("start the snapshot transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var snapshotID string
	if err := tx.QueryRowContext(ctx, "SELECT pg_export_snapshot()").Scan(&snapshotID); err != nil {
		return snapshotFacts{}, fmt.Errorf("export the snapshot: %w", err)
	}
	var size int64
	if err := tx.QueryRowContext(ctx, "SELECT pg_database_size(current_database())").Scan(&size); err == nil {
		if err := checkFreeSpaceFor(dir, size); err != nil {
			return snapshotFacts{}, err
		}
	}
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: tx}), &gorm.Config{Logger: logger.Discard, SkipDefaultTransaction: true})
	if err != nil {
		return snapshotFacts{}, fmt.Errorf("read the snapshot: %w", err)
	}
	facts, err := factsOf(ctx, gdb)
	if err != nil {
		return snapshotFacts{}, fmt.Errorf("read the snapshot: %w", err)
	}
	if err := run(ctx, p.env(p.Database), p.dumpBin(), "--format=custom", "--no-owner", "--no-acl", "--snapshot="+snapshotID, "--file="+destination); err != nil {
		_ = os.Remove(destination)
		return snapshotFacts{}, fmt.Errorf("pg_dump failed: %w", err)
	}
	// pg_dump does not sync its output; the bundle that copies it is synced,
	// but a dump verified or installed from disk must be too.
	file, err := os.OpenFile(destination, os.O_RDWR, 0) // #nosec G304 -- the dump this call just wrote.
	if err != nil {
		return snapshotFacts{}, err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		return snapshotFacts{}, syncErr
	}
	if closeErr != nil {
		return snapshotFacts{}, closeErr
	}
	if err := os.Chmod(destination, fileMode); err != nil {
		return snapshotFacts{}, err
	}
	return facts, nil
}

// quoteIdent quotes a PostgreSQL identifier.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// scratchDatabase creates an empty database next to the panel's for a
// verification and answers its name and the function that drops it.
func (p *Postgres) scratchDatabase(ctx context.Context) (string, func(), error) {
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return "", nil, err
	}
	name := "anix_verify_" + hex.EncodeToString(suffix)
	admin, err := p.open(p.Database)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = admin.Close() }()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+quoteIdent(name)); err != nil {
		return "", nil, fmt.Errorf("create the scratch database %s: %w. The verification restores the dump into a database of its own, so the database role needs CREATEDB", name, err)
	}
	drop := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		db, err := p.open(p.Database)
		if err != nil {
			return
		}
		defer func() { _ = db.Close() }()
		if _, err := db.ExecContext(cleanup, "DROP DATABASE IF EXISTS "+quoteIdent(name)+" WITH (FORCE)"); err != nil {
			_, _ = db.ExecContext(cleanup, "DROP DATABASE IF EXISTS "+quoteIdent(name))
		}
	}
	return name, drop, nil
}

// restoreInto restores the dump into the empty database named database.
func (p *Postgres) restoreInto(ctx context.Context, database, dump string) error {
	if err := run(ctx, p.env(database), p.restoreBin(), "--no-owner", "--no-acl", "--exit-on-error", "--dbname="+database, dump); err != nil {
		return fmt.Errorf("pg_restore failed: %w", err)
	}
	return nil
}

// openGorm opens a gorm handle to database with the migrations' dialect.
func (p *Postgres) openGorm(database string) (*gorm.DB, func(), error) {
	db, err := gorm.Open(postgres.Open(p.dsn(database)), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	sqlDB.SetMaxOpenConns(2)
	return db, func() { _ = sqlDB.Close() }, nil
}

// installDump restores a dump as the panel's database. The database must
// not hold tables: an existing one with tables is renamed aside first when
// replace is set (nothing is deleted) and refused otherwise. It answers
// the name an existing database was renamed to, or "".
func (p *Postgres) installDump(ctx context.Context, dump string, replace bool, now time.Time) (renamedTo string, err error) {
	admin, err := p.open("postgres")
	if err != nil {
		return "", err
	}
	defer func() { _ = admin.Close() }()
	var exists bool
	if err := admin.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", p.Database).Scan(&exists); err != nil {
		return "", fmt.Errorf("look for the database %s: %w", p.Database, err)
	}
	if exists {
		target, err := p.open(p.Database)
		if err != nil {
			return "", err
		}
		var tables int
		err = target.QueryRowContext(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema')").Scan(&tables)
		_ = target.Close()
		if err != nil {
			return "", fmt.Errorf("look into the database %s: %w", p.Database, err)
		}
		if tables > 0 {
			if !replace {
				return "", refused("nothing was restored", fmt.Errorf("the PostgreSQL database %s is not empty (%d tables). Restore into an empty database, or pass --replace to rename it aside first (nothing is deleted)", p.Database, tables))
			}
			renamedTo = p.Database + "_replaced_" + now.UTC().Format(timeFormat)
			if _, err := admin.ExecContext(ctx, "ALTER DATABASE "+quoteIdent(p.Database)+" RENAME TO "+quoteIdent(renamedTo)); err != nil {
				return "", fmt.Errorf("rename the database %s aside: %w (stop everything that is connected to it, Control included)", p.Database, err)
			}
			exists = false
		}
	}
	if !exists {
		if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+quoteIdent(p.Database)); err != nil {
			return renamedTo, fmt.Errorf("create the database %s: %w", p.Database, err)
		}
	}
	if err := p.restoreInto(ctx, p.Database, dump); err != nil {
		if renamedTo != "" {
			err = fmt.Errorf("%w (the previous database is kept as %s)", err, renamedTo)
		}
		return renamedTo, err
	}
	return renamedTo, nil
}
