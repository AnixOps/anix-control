package lease

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lease.db") + "?_pragma=busy_timeout(5000)"
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Record{}))
	return db
}

func TestTryAcquireGrantsOneHolderUntilExpiry(t *testing.T) {
	db := openSQLite(t)
	ctx := context.Background()
	start := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	ttl := 30 * time.Second

	_, ok, err := TryAcquire(ctx, db, "workers", "a", ttl, start)
	require.NoError(t, err)
	assert.True(t, ok, "a free lease is granted")

	_, ok, err = TryAcquire(ctx, db, "workers", "b", ttl, start.Add(10*time.Second))
	require.NoError(t, err)
	assert.False(t, ok, "a held lease is refused")

	_, ok, err = TryAcquire(ctx, db, "workers", "a", ttl, start.Add(20*time.Second))
	require.NoError(t, err)
	assert.True(t, ok, "the holder renews")

	_, ok, err = TryAcquire(ctx, db, "workers", "b", ttl, start.Add(45*time.Second))
	require.NoError(t, err)
	assert.False(t, ok, "the renewal extended the lease")

	_, ok, err = TryAcquire(ctx, db, "workers", "b", ttl, start.Add(51*time.Second))
	require.NoError(t, err)
	assert.True(t, ok, "an expired lease is taken over")

	require.NoError(t, Release(ctx, db, "workers", "a", start.Add(52*time.Second)), "releasing a lease you lost is a no-op")
	_, ok, err = TryAcquire(ctx, db, "workers", "a", ttl, start.Add(53*time.Second))
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, Release(ctx, db, "workers", "b", start.Add(54*time.Second)))
	_, ok, err = TryAcquire(ctx, db, "workers", "a", ttl, start.Add(54*time.Second))
	require.NoError(t, err)
	assert.True(t, ok, "a released lease is free immediately")
}

func TestInstanceIDsAreUnique(t *testing.T) {
	assert.NotEqual(t, InstanceID(), InstanceID())
}

// Two electors on one database: work runs in exactly one of them, and the
// other takes over after the leader stops.
func TestElectorsRunWorkInOneProcessAtATime(t *testing.T) {
	db := openSQLite(t)
	var (
		active     atomic.Int32
		maxActive  atomic.Int32
		startedBy  sync.Map
		leaderStop = make(map[string]context.CancelFunc)
	)
	work := func(holder string) func(ctx context.Context) {
		return func(ctx context.Context) {
			current := active.Add(1)
			for {
				seen := maxActive.Load()
				if current <= seen || maxActive.CompareAndSwap(seen, current) {
					break
				}
			}
			startedBy.Store(holder, true)
			<-ctx.Done()
			active.Add(-1)
		}
	}

	var wg sync.WaitGroup
	for _, holder := range []string{"a", "b"} {
		ctx, cancel := context.WithCancel(context.Background())
		leaderStop[holder] = cancel
		elector := &Elector{DB: db, Name: "workers", Holder: holder, TTL: 900 * time.Millisecond}
		wg.Add(1)
		go func() {
			defer wg.Done()
			elector.Run(ctx, work(holder))
		}()
	}

	require.Eventually(t, func() bool { return active.Load() == 1 }, 5*time.Second, 20*time.Millisecond)
	first := "a"
	if _, ok := startedBy.Load("a"); !ok {
		first = "b"
	}
	second := map[string]string{"a": "b", "b": "a"}[first]

	// Stopping the leader releases the lease; the other elector takes over.
	leaderStop[first]()
	require.Eventually(t, func() bool {
		_, ok := startedBy.Load(second)
		return ok && active.Load() == 1
	}, 5*time.Second, 20*time.Millisecond)

	leaderStop[second]()
	wg.Wait()
	assert.EqualValues(t, 1, maxActive.Load(), "work never ran in two electors at once")
	assert.EqualValues(t, 0, active.Load())
}

func TestWritePrometheusReportsRunningElectors(t *testing.T) {
	var body strings.Builder
	WritePrometheus(&body)
	assert.Empty(t, body.String(), "no electors, no series")

	elector := &Elector{Name: "metrics-test"}
	running.Store(elector.Name, elector)
	defer running.Delete(elector.Name)
	elector.setLeader(true)
	WritePrometheus(&body)
	assert.Contains(t, body.String(), `anixops_lease_leader{lease="metrics-test"} 1`)
}

// Concurrent acquisitions on PostgreSQL grant the lease to exactly one holder.
func TestPostgresConcurrentAcquireGrantsOneHolder(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	var databaseName string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") {
		t.Skipf("refusing to run against database %q", databaseName)
	}
	require.NoError(t, db.Migrator().DropTable(&Record{}))
	require.NoError(t, db.AutoMigrate(&Record{}))
	t.Cleanup(func() { _ = db.Migrator().DropTable(&Record{}) })

	now := time.Now().UTC()
	var (
		wg      sync.WaitGroup
		granted atomic.Int32
	)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, ok, err := TryAcquire(context.Background(), db, "workers", "holder-"+string(rune('a'+i)), time.Minute, now)
			assert.NoError(t, err)
			if ok {
				granted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	assert.EqualValues(t, 1, granted.Load())

	// The fence reads the row FOR SHARE and judges expiry by now().
	var winner Record
	require.NoError(t, db.Take(&winner, "name = ?", "workers").Error)
	fenced := context.WithValue(context.Background(), fenceKey{}, fence{"workers", winner.Holder, winner.Generation})
	assert.NoError(t, db.WithContext(fenced).Transaction(func(tx *gorm.DB) error { return Check(tx) }))
	stale := context.WithValue(context.Background(), fenceKey{}, fence{"workers", winner.Holder, winner.Generation + 1})
	assert.ErrorIs(t, db.WithContext(stale).Transaction(func(tx *gorm.DB) error { return Check(tx) }), ErrLost)
	databaseNow, err := DatabaseNow(context.Background(), db)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), databaseNow, time.Minute)
}

// A leader that cannot renew stops its work before the lease can expire.
func TestElectorStopsWorkWhenRenewalsFail(t *testing.T) {
	db := openSQLite(t)
	ttl := 600 * time.Millisecond
	elector := &Elector{DB: db, Name: "workers", Holder: "a", TTL: ttl}
	started := make(chan time.Time, 1)
	stopped := make(chan time.Time, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go elector.Run(ctx, func(ctx context.Context) {
		started <- time.Now()
		<-ctx.Done()
		stopped <- time.Now()
	})

	var began time.Time
	select {
	case began = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("elector never led")
	}
	require.NoError(t, db.Migrator().DropTable(&Record{}), "renewals fail from now on")
	select {
	case ended := <-stopped:
		assert.Less(t, ended.Sub(began), ttl+ttl/2, "work stops close to 2/3 of the TTL after the last renewal")
	case <-time.After(5 * time.Second):
		t.Fatal("work kept running without a renewed lease")
	}
	// Leadership is cleared after the work returns.
	require.Eventually(t, func() bool { return !elector.IsLeader() }, 2*time.Second, 10*time.Millisecond)
}

// ARC-003: the generation is raised each time the lease changes holder,
// not on renewals.
func TestTryAcquireRaisesTheGenerationOnATakeover(t *testing.T) {
	db := openSQLite(t)
	ctx := context.Background()
	start := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	ttl := 30 * time.Second
	steps := []struct {
		holder     string
		at         time.Duration
		generation int64
	}{
		{"a", 0, 1},                // first holder
		{"a", 10 * time.Second, 1}, // renewal
		{"b", 50 * time.Second, 2}, // takeover after expiry
		{"a", 90 * time.Second, 3}, // taken back
	}
	for _, step := range steps {
		generation, ok, err := TryAcquire(ctx, db, "workers", step.holder, ttl, start.Add(step.at))
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, step.generation, generation, "%s at %s", step.holder, step.at)
	}
}

// ARC-003: a leader's write fails with ErrLost once another process took
// its lease, or once the lease expired; writes outside leader work pass.
func TestCheckFencesTheWritesOfALostLeader(t *testing.T) {
	db := openSQLite(t)
	ctx := context.Background()
	now := time.Now().UTC()
	generation, ok, err := TryAcquire(ctx, db, "workers", "a", time.Minute, now)
	require.NoError(t, err)
	require.True(t, ok)
	leader := context.WithValue(ctx, fenceKey{}, fence{"workers", "a", generation})
	check := func(ctx context.Context) error {
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return Check(tx) })
	}

	assert.NoError(t, check(leader), "the holder writes")
	assert.NoError(t, check(ctx), "a write outside leader work is not fenced")

	require.NoError(t, Release(ctx, db, "workers", "a", now))
	assert.ErrorIs(t, check(leader), ErrLost, "an expired lease fences")

	_, ok, err = TryAcquire(ctx, db, "workers", "b", time.Minute, now)
	require.NoError(t, err)
	require.True(t, ok)
	assert.ErrorIs(t, check(leader), ErrLost, "another holder fences")

	require.NoError(t, Release(ctx, db, "workers", "b", now))
	_, ok, err = TryAcquire(ctx, db, "workers", "a", time.Minute, now)
	require.NoError(t, err)
	require.True(t, ok)
	assert.ErrorIs(t, check(leader), ErrLost, "the old generation stays fenced after the lease is taken back")
}

// ARC-003: leader work runs with its lease's fence.
func TestElectorWorkCarriesItsFence(t *testing.T) {
	db := openSQLite(t)
	elector := &Elector{DB: db, Name: "workers", Holder: "a", TTL: time.Minute}
	checked := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go elector.Run(ctx, func(ctx context.Context) {
		fenced, ok := ctx.Value(fenceKey{}).(fence)
		if !ok || fenced.generation != 1 {
			checked <- assert.AnError
			return
		}
		checked <- db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return Check(tx) })
		<-ctx.Done()
	})
	select {
	case err := <-checked:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("elector never led")
	}
}
