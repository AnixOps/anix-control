package database

import (
	"database/sql"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

const (
	// SQLiteBusyTimeoutMillis is how long a SQLite statement waits for a
	// lock another connection holds before it fails with "database is
	// locked". Writers of this process queue in Go, so the wait is for
	// other processes: the package hosts that share the file. The package
	// store (sdk/packagestoresdk) uses the same value.
	SQLiteBusyTimeoutMillis = 10000

	// SQLiteReadConns is the size of the read connection pool.
	SQLiteReadConns = 4
)

// sqliteConnectionString returns the writer connection settings for a SQLite
// path: a busy timeout, and for file databases WAL, synchronous NORMAL and
// incremental auto-vacuum, and transactions that begin IMMEDIATE.
//
//   - A deferred transaction that reads before it writes cannot upgrade to a
//     writer once another connection committed after its read began, and
//     SQLite fails it with "database is locked" at once, without waiting
//     out the busy timeout; an immediate one waits for the write lock at
//     BEGIN instead.
//   - synchronous NORMAL in WAL mode syncs the log at checkpoints instead of
//     at every commit: a power loss may drop the last commits, the database
//     stays consistent, and a commit costs no fsync.
//   - auto_vacuum only takes effect on a database file that has no tables
//     yet, which is why it comes before journal_mode: a new database
//     returns freed pages with PRAGMA incremental_vacuum (the retention
//     worker does), an existing one keeps its setting.
//   - foreign key enforcement stays off: the schema and its data were never
//     run under it.
func sqliteConnectionString(databasePath string) string {
	databasePath = strings.TrimSpace(databasePath)
	dsn := databasePath + dsnSeparator(databasePath) +
		fmt.Sprintf("_pragma=busy_timeout(%d)&_txlock=immediate", SQLiteBusyTimeoutMillis)
	if sqliteInMemory(databasePath) {
		return dsn
	}
	return dsn + "&_pragma=synchronous(NORMAL)&_pragma=auto_vacuum(INCREMENTAL)&_pragma=journal_mode(WAL)"
}

// sqliteReadConnectionString returns the settings of a read pool connection:
// query_only, so a write sent to a reader fails loudly instead of queueing
// behind the writer, and deferred transactions (BEGIN IMMEDIATE needs write
// access).
func sqliteReadConnectionString(databasePath string) string {
	databasePath = strings.TrimSpace(databasePath)
	return databasePath + dsnSeparator(databasePath) +
		fmt.Sprintf("_pragma=busy_timeout(%d)&_pragma=query_only(1)", SQLiteBusyTimeoutMillis)
}

func dsnSeparator(databasePath string) string {
	if strings.Contains(databasePath, "?") {
		return "&"
	}
	return "?"
}

// sqliteInMemory reports whether a SQLite path names an in-memory database,
// which cannot be shared between connections the way a file can.
func sqliteInMemory(databasePath string) bool {
	return databasePath == ":memory:" || strings.HasPrefix(databasePath, "file::memory:") ||
		strings.Contains(databasePath, "mode=memory")
}

// splitSQLiteHandle makes handle's one connection the single writer and adds
// the read pool: reads of handle run on readDialector's connections, writes
// and transactions stay on the writer. It returns the read pools to close.
func splitSQLiteHandle(handle *gorm.DB, readDialector gorm.Dialector) ([]*sql.DB, error) {
	writer, err := handle.DB()
	if err != nil {
		return nil, fmt.Errorf("get database instance: %w", err)
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)

	resolver := dbresolver.Register(dbresolver.Config{Replicas: []gorm.Dialector{readDialector}})
	if err := handle.Use(resolver); err != nil {
		return nil, fmt.Errorf("configure the SQLite read pool: %w", err)
	}
	var readers []*sql.DB
	if err := resolver.Call(func(pool gorm.ConnPool) error {
		if reader, ok := pool.(*sql.DB); ok && reader != writer {
			reader.SetMaxOpenConns(SQLiteReadConns)
			reader.SetMaxIdleConns(SQLiteReadConns)
			readers = append(readers, reader)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return readers, nil
}
