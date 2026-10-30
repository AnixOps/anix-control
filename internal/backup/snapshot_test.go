package backup

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// counterInvariant opens a snapshot and checks it: PRAGMA integrity_check
// answers ok, and the counter equals the number of events, which every
// write transaction increments together.
func counterInvariant(t *testing.T, path string) int64 {
	t.Helper()
	db, closeDB, err := openSnapshot(path)
	require.NoError(t, err)
	defer closeDB()
	require.NoError(t, integrityCheck(context.Background(), db))
	var counter, events int64
	require.NoError(t, db.Raw("SELECT n FROM counter WHERE id = 1").Scan(&counter).Error)
	require.NoError(t, db.Raw("SELECT count(*) FROM events").Scan(&events).Error)
	require.Equal(t, events, counter, "the snapshot is one consistent state: the counter equals the events")
	return counter
}

func prepareCounter(t *testing.T, db *gorm.DB, rows int) {
	t.Helper()
	require.NoError(t, db.Exec("CREATE TABLE counter (id integer PRIMARY KEY, n integer NOT NULL)").Error)
	require.NoError(t, db.Exec("CREATE TABLE events (id integer PRIMARY KEY AUTOINCREMENT, payload blob NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO counter (id, n) VALUES (1, 0)").Error)
	payload := strings.Repeat("x", 1024)
	for done := 0; done < rows; done += 500 {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			for i := 0; i < 500 && done+i < rows; i++ {
				if err := tx.Exec("INSERT INTO events (payload) VALUES (?)", payload).Error; err != nil {
					return err
				}
			}
			return tx.Exec("UPDATE counter SET n = (SELECT count(*) FROM events) WHERE id = 1").Error
		}))
	}
}

// Snapshots taken while the panel's writer commits continuously are each
// one consistent state, pass PRAGMA integrity_check, and the writer keeps
// committing while they run: the snapshot never holds the panel's writer.
func TestSnapshotUnderConcurrentWriters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "control.db")
	db := openPanelDB(t, path)
	prepareCounter(t, db, 4000)

	type window struct{ start, end time.Time }
	var (
		stop      atomic.Bool
		commits   []time.Time
		commitsMu sync.Mutex
		writerErr error
		wg        sync.WaitGroup
	)
	payload := strings.Repeat("y", 1024)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			err := db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec("INSERT INTO events (payload) VALUES (?)", payload).Error; err != nil {
					return err
				}
				return tx.Exec("UPDATE counter SET n = n + 1 WHERE id = 1").Error
			})
			if err != nil {
				writerErr = err
				return
			}
			commitsMu.Lock()
			commits = append(commits, time.Now())
			commitsMu.Unlock()
			// A writer that never pauses grows the WAL faster than the
			// snapshots' read transactions let it checkpoint, so every
			// VACUUM INTO reads a longer log: under -race the 25 snapshots
			// ran past the 10 minute test timeout. A short pause still
			// commits throughout every snapshot.
			time.Sleep(2 * time.Millisecond)
		}
	}()

	var windows []window
	var counts []int64
	for i := 0; i < 25; i++ {
		destination := filepath.Join(dir, fmt.Sprintf("snapshot-%02d.db", i))
		start := time.Now()
		require.NoError(t, Snapshot(context.Background(), path, destination))
		windows = append(windows, window{start, time.Now()})
		counts = append(counts, counterInvariant(t, destination))
	}
	stop.Store(true)
	wg.Wait()
	require.NoError(t, writerErr)

	during := 0
	for _, commit := range commits {
		for _, w := range windows {
			if commit.After(w.start) && commit.Before(w.end) {
				during++
				break
			}
		}
	}
	t.Logf("%d commits, %d of them while a snapshot ran; counters %d .. %d", len(commits), during, counts[0], counts[len(counts)-1])
	assert.Positive(t, during, "the writer committed while snapshots ran")
	assert.Greater(t, counts[len(counts)-1], counts[0], "later snapshots see later commits")
	assert.Equal(t, int64(4000+len(commits)), counterInvariant(t, path), "the live database kept every commit")
}

// The snapshot does not need the panel's single writer connection: it
// completes while a write transaction holds it, and does not see that
// transaction's uncommitted rows.
func TestSnapshotDoesNotNeedTheWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "control.db")
	db := openPanelDB(t, path)
	prepareCounter(t, db, 10)

	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, tx.Exec("INSERT INTO events (payload) VALUES ('uncommitted')").Error)
	require.NoError(t, tx.Exec("UPDATE counter SET n = n + 1 WHERE id = 1").Error)

	destination := filepath.Join(dir, "snapshot.db")
	done := make(chan error, 1)
	go func() { done <- Snapshot(context.Background(), path, destination) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("the snapshot waited for the writer's transaction")
	}
	assert.Equal(t, int64(10), counterInvariant(t, destination), "only committed rows")
	require.NoError(t, tx.Commit().Error)
	assert.Equal(t, int64(11), counterInvariant(t, path))
}

func TestSnapshotRefuses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "control.db")
	openPanelDB(t, path).Exec("CREATE TABLE t (id integer)")
	require.ErrorContains(t, Snapshot(context.Background(), ":memory:", filepath.Join(dir, "a.db")), "in-memory")
	require.ErrorContains(t, Snapshot(context.Background(), filepath.Join(dir, "missing.db"), filepath.Join(dir, "b.db")), "the SQLite database")
	require.NoError(t, Snapshot(context.Background(), path, filepath.Join(dir, "c.db")))
	require.ErrorContains(t, Snapshot(context.Background(), path, filepath.Join(dir, "c.db")), "already exists")
}
