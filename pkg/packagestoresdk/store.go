// Package packagestoresdk opens a package's own database storage from a
// kernel storage lease and runs the package's embedded SQL migrations.
//
// On PostgreSQL the lease logs in as the package's own role. Its tables live
// in the package schema; adopted kernel tables and kernel API views are
// reachable by their plain names. On SQLite the package shares the kernel's
// database file and its tables carry a prefix. Table and the __PKG_PREFIX__
// token in migrations hide the difference.
package packagestoresdk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/glebarez/sqlite"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// MaxOpenConns matches the connection limit of a package role.
const MaxOpenConns = 4

// Leaser issues storage leases; *packagebridgesdk.Client implements it.
type Leaser interface {
	LeaseStorage(ctx context.Context) (packagebridgesdk.StorageLease, error)
}

// LeaserFunc adapts a function to Leaser.
type LeaserFunc func(ctx context.Context) (packagebridgesdk.StorageLease, error)

func (f LeaserFunc) LeaseStorage(ctx context.Context) (packagebridgesdk.StorageLease, error) {
	return f(ctx)
}

// Store is an open package storage connection.
type Store struct {
	DB    *gorm.DB
	Lease packagebridgesdk.StorageLease
}

var ownNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// Open leases the package's storage and connects to it.
func Open(ctx context.Context, leaser Leaser) (*Store, error) {
	if leaser == nil {
		return nil, errors.New("package storage needs a leaser")
	}
	lease, err := leaser.LeaseStorage(ctx)
	if err != nil {
		return nil, err
	}
	var dialector gorm.Dialector
	switch lease.Driver {
	case "postgres":
		if !ownNamePattern.MatchString(lease.Schema) {
			return nil, fmt.Errorf("package storage lease has an invalid schema %q", lease.Schema)
		}
		sqlDB, err := openPostgres(lease.DSN, leaser)
		if err != nil {
			return nil, err
		}
		dialector = postgres.New(postgres.Config{Conn: sqlDB})
	case "sqlite":
		if !ownNamePattern.MatchString(strings.TrimSuffix(lease.TablePrefix, "_")) || !strings.HasSuffix(lease.TablePrefix, "_") {
			return nil, fmt.Errorf("package storage lease has an invalid table prefix %q", lease.TablePrefix)
		}
		dialector = sqlite.Open(sqliteConnectionString(lease.DSN))
	default:
		return nil, fmt.Errorf("package storage lease has an unsupported driver %q", lease.Driver)
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	if err != nil {
		return nil, fmt.Errorf("open package storage: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(MaxOpenConns)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("connect package storage: %w", err)
	}
	return &Store{DB: db, Lease: lease}, nil
}

// sqliteConnectionString matches the kernel's settings for the shared file.
func sqliteConnectionString(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

// Prefix is what the __PKG_PREFIX__ migration token expands to: the package
// schema and a dot on PostgreSQL, the table prefix on SQLite.
func (s *Store) Prefix() string {
	if s == nil {
		return ""
	}
	if s.Lease.Driver == "postgres" {
		return s.Lease.Schema + "."
	}
	return s.Lease.TablePrefix
}

// Table returns the full name of one of the package's own tables, for example
// "pkg_knowledge.notes" on PostgreSQL and "pkg_knowledge_notes" on SQLite.
// It panics on a name that is not lowercase letters, digits and underscores,
// which is a programming error.
func (s *Store) Table(name string) string {
	if !ownNamePattern.MatchString(name) {
		panic(fmt.Sprintf("packagestoresdk: invalid table name %q", name))
	}
	return s.Prefix() + name
}

// Close closes the connection pool.
func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	sqlDB, err := s.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// openPostgres opens the package role's pool. Every new connection asks the
// kernel for the current lease first: the kernel returns its cached lease,
// and when another lease rotated the role password (a kernel restart, a new
// replica) the pool picks up the new one instead of failing with 28P01.
func openPostgres(dsn string, leaser Leaser) (*sql.DB, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid package storage lease: %w", err)
	}
	return stdlib.OpenDB(*config, stdlib.OptionBeforeConnect(func(ctx context.Context, connection *pgx.ConnConfig) error {
		lease, err := leaser.LeaseStorage(ctx)
		if err != nil {
			return fmt.Errorf("refresh package storage lease: %w", err)
		}
		fresh, err := pgx.ParseConfig(lease.DSN)
		if err != nil {
			return fmt.Errorf("invalid package storage lease: %w", err)
		}
		connection.User, connection.Password = fresh.User, fresh.Password
		return nil
	})), nil
}
