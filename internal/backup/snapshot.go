package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/glebarez/sqlite"
)

// snapshotBusyTimeoutMillis is how long the snapshot connection waits for
// a lock. A WAL reader only waits in rare moments (another connection
// recovering or restarting the log), never for a writer.
const snapshotBusyTimeoutMillis = 10000

// Snapshot writes a consistent copy of the SQLite database at databasePath
// to destination, which must not exist.
//
// It opens a dedicated connection to the database file and runs VACUUM
// INTO on it. VACUUM INTO reads the source in a single read transaction, so
// the copy is one consistent state of the database, and writes a compacted
// database in rollback-journal mode. In WAL mode a reader neither waits for
// the writer nor blocks it: the panel keeps committing while the snapshot
// runs (TestSnapshotDoesNotNeedTheWriter, TestSnapshotUnderConcurrent
// Writers). Neither of the panel's own connections can do it: the read
// pool is query_only, where VACUUM INTO is refused as a write, and the
// writer would hold every write of the panel for the snapshot's length.
// The dedicated connection begins no transaction of its own (VACUUM refuses
// to run inside one) and is closed afterwards.
func Snapshot(ctx context.Context, databasePath, destination string) error {
	if inMemory(databasePath) {
		return errors.New("an in-memory SQLite database cannot be backed up")
	}
	if _, err := os.Stat(sqliteFile(databasePath)); err != nil {
		return fmt.Errorf("the SQLite database: %w", err)
	}
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("the snapshot %s already exists", destination)
	}
	dsn := sqliteFile(databasePath) + fmt.Sprintf("?_pragma=busy_timeout(%d)", snapshotBusyTimeoutMillis)
	db, err := sql.Open(sqlite.DriverName, dsn)
	if err != nil {
		return fmt.Errorf("open the snapshot connection: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open the snapshot connection: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "VACUUM INTO ?", destination); err != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("VACUUM INTO the snapshot: %w", err)
	}
	// VACUUM INTO does not sync its output; the bundle that copies it is
	// synced, but a snapshot verified or installed from disk must be too.
	file, err := os.OpenFile(destination, os.O_RDWR, 0) // #nosec G304 -- the snapshot this call just wrote.
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(destination, fileMode)
}
