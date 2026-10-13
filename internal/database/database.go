// Package database opens the kernel's database and hands out its one
// *gorm.DB (Get, GetDB).
//
// # SQLite: one writer, a read pool
//
// SQLite allows one write transaction at a time, so on a database file the
// handle is split in two. Every write statement and every transaction
// (Begin, Transaction) runs on a single writer connection (MaxOpenConns 1):
// writers queue in Go instead of busy-waiting inside SQLite, and a
// transaction begins IMMEDIATE, so it takes the write lock at BEGIN and a
// read-then-write transaction cannot fail half way with "database is
// locked". Plain queries (First, Find, Count, Raw SELECT) run on a pool of
// read connections (SQLiteReadConns, query_only): in WAL mode a reader never
// blocks the writer or the other way round. Both use WAL, synchronous NORMAL
// and a 10 second busy timeout, which only matters to a process that is not
// this one: every package host opens the same file. The routing is GORM's
// dbresolver plugin; callers keep using the one handle.
//
// Rules that follow from the split:
//
//   - A read that must see the writes of its own transaction uses the
//     transaction's handle (tx), never the root handle: the root handle's
//     reader sees only what was committed, and a write on the root handle
//     while a transaction is open waits for that transaction to end, so it
//     is a deadlock when the transaction waits for it.
//   - A read after a committed write on the root handle sees the write: the
//     commit finishes before the next statement starts.
//   - A read that must not race another writer between its read and its
//     write goes into a transaction (a SELECT ... FOR UPDATE is a write
//     for the router, and SQLite has no row locks).
//   - Do not hold a *sql.Conn from the handle's *sql.DB across work that
//     also uses the handle: the writer pool has one connection.
//
// In-memory databases (":memory:", tests) keep a single handle: each
// connection of an in-memory database is its own database. PostgreSQL has
// one handle and a pool sized by the database settings; nothing above
// applies to it.
package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/logging"
	"github.com/glebarez/sqlite" // 纯Go实现的SQLite驱动，无需CGO
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	db          *gorm.DB
	initialized bool
	// readPools are the SQLite read connection pools of the handles Open
	// made, by the handle's writer connection pool; none for an in-memory
	// database and for PostgreSQL.
	readPools sync.Map // *sql.DB -> []*sql.DB
)

// Init 初始化数据库连接
func Init(cfg *config.DatabaseConfig) error {
	// 如果已经初始化，直接返回
	if initialized && db != nil {
		return nil
	}
	opened, err := Open(cfg)
	if err != nil {
		db = nil
		return err
	}
	db = opened
	initialized = true
	return nil
}

// Open opens a database handle as Init does but does not make it the
// process's handle (Get): the caller owns it and closes it with CloseHandle.
// A test or a tool that needs a database of its own, with the connection
// settings of the kernel's, opens it this way.
func Open(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector
	var readDialector gorm.Dialector

	switch cfg.Driver {
	case "sqlite", "sqlite3", "":
		// SQLite 为默认数据库
		dbPath := SQLitePath(cfg)

		// 确保目录存在
		dir := filepath.Dir(dbPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return nil, fmt.Errorf("failed to create database directory: %w", err)
			}
		}

		dialector = sqlite.Open(sqliteConnectionString(dbPath))
		if !sqliteInMemory(dbPath) {
			readDialector = sqlite.Open(sqliteReadConnectionString(dbPath))
		}

	case "postgres", "postgresql":
		// PostgreSQL
		dialector = postgres.Open(PostgresDSN(cfg))

	default:
		return nil, fmt.Errorf("unsupported database driver: %s (supported: sqlite, postgres)", cfg.Driver)
	}

	// 配置日志级别
	logLevel := logger.Info
	switch cfg.LogLevel {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "warn":
		logLevel = logger.Warn
	}

	// logger.Default's writer, or the JSON handler's.
	var sqlLogWriter logger.Writer = log.New(os.Stdout, "\r\n", log.LstdFlags)
	colorful := true
	if logging.JSON() {
		sqlLogWriter = log.New(logging.StdWriter(), "", 0)
		colorful = false
	}
	handle, err := gorm.Open(dialector, &gorm.Config{
		Logger: newSQLLogger(sqlLogWriter, colorful, logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	if readDialector != nil {
		pools, err := splitSQLiteHandle(handle, readDialector)
		if err != nil {
			if sqlDB, dbErr := handle.DB(); dbErr == nil {
				_ = sqlDB.Close()
			}
			return nil, err
		}
		if writer, err := handle.DB(); err == nil {
			readPools.Store(writer, pools)
		}
	}

	// 只有 PostgreSQL 需要设置连接池
	if cfg.Driver == "postgres" || cfg.Driver == "postgresql" {
		sqlDB, err := handle.DB()
		if err != nil {
			return nil, fmt.Errorf("failed to get database instance: %w", err)
		}

		// 设置连接池
		if cfg.MaxIdleConns > 0 {
			sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
		}
		if cfg.MaxOpenConns > 0 {
			sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
		}
		if cfg.ConnMaxLifetime > 0 {
			sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
		}
	}

	return handle, nil
}

// newSQLLogger returns GORM's SQL logger: logger.Default's settings at the
// configured level. It logs each statement with its placeholders and never
// the bound values (ParameterizedQueries): those are node API keys, tokens,
// password hashes and subscription UUIDs, and database.log_level: info logs
// every statement.
func newSQLLogger(writer logger.Writer, colorful bool, level logger.LogLevel) logger.Interface {
	return logger.New(writer, logger.Config{
		SlowThreshold:        200 * time.Millisecond,
		LogLevel:             level,
		Colorful:             colorful,
		ParameterizedQueries: true,
	})
}

// DefaultSQLitePath is the SQLite database file used when none is configured.
const DefaultSQLitePath = "config/data/v2board.db"

// SQLitePath returns the configured SQLite database file.
func SQLitePath(cfg *config.DatabaseConfig) string {
	if cfg == nil || cfg.Database == "" {
		return DefaultSQLitePath
	}
	return cfg.Database
}

// PackageStorageSource returns what package storage derives package
// connections from: the kernel's PostgreSQL connection string, or the
// absolute path of the SQLite database file. It holds kernel credentials and
// must stay in the kernel.
func PackageStorageSource(cfg *config.DatabaseConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("database configuration is required")
	}
	switch cfg.Driver {
	case "postgres", "postgresql":
		return PostgresDSN(cfg), nil
	case "sqlite", "sqlite3", "":
		return filepath.Abs(SQLitePath(cfg))
	default:
		return "", fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}
}

// Get 获取数据库实例
func Get() *gorm.DB {
	return db
}

// GetDB 获取数据库实例
func GetDB() *gorm.DB {
	return db
}

// Close 关闭数据库连接
func Close() error {
	if db == nil {
		return nil
	}
	if err := CloseHandle(db); err != nil {
		return err
	}
	db = nil
	initialized = false
	return nil
}

// CloseHandle closes a handle Open (or Init) made: its writer connection and
// the read pool behind it.
func CloseHandle(handle *gorm.DB) error {
	if handle == nil {
		return nil
	}
	sqlDB, err := handle.DB()
	if err != nil {
		return err
	}
	var closeErr error
	if pools, ok := readPools.LoadAndDelete(sqlDB); ok {
		for _, pool := range pools.([]*sql.DB) {
			if err := pool.Close(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
	}
	if err := sqlDB.Close(); err != nil {
		return err
	}
	return closeErr
}

// Reset resets the database state (for testing only)
func Reset() {
	db = nil
	initialized = false
}

// AutoMigrate 自动迁移数据库表
func AutoMigrate(models ...any) error {
	return db.AutoMigrate(models...)
}
