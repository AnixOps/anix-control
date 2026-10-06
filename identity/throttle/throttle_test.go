package throttle

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestFailuresLockAKeyUntilTheLockoutEnds(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE t_throttle (throttle_key VARCHAR(255) PRIMARY KEY, failures INTEGER NOT NULL,
		window_start BIGINT NOT NULL, lock_until BIGINT NOT NULL, last_seen BIGINT NOT NULL)`).Error)
	now := time.Unix(1790000000, 0)
	limiter := &Limiter{DB: db, Table: "t_throttle", Now: func() time.Time { return now }}
	options := Options{Enabled: true, MaxAttempts: 3, Window: time.Minute, Lockout: 10 * time.Minute}
	ctx := context.Background()

	for range 2 {
		require.NoError(t, limiter.RecordFailure(ctx, "login|a", options))
	}
	blocked, _, err := limiter.Check(ctx, "login|a", options)
	require.NoError(t, err)
	require.False(t, blocked)
	require.NoError(t, limiter.RecordFailure(ctx, "login|a", options))
	blocked, wait, err := limiter.Check(ctx, "login|a", options)
	require.NoError(t, err)
	require.True(t, blocked)
	require.Equal(t, 10*time.Minute, wait)

	now = now.Add(10*time.Minute + time.Second)
	blocked, _, err = limiter.Check(ctx, "login|a", options)
	require.NoError(t, err)
	require.False(t, blocked, "the lockout ends and the old window is forgotten")

	require.NoError(t, limiter.RecordFailure(ctx, "login|b", options))
	now = now.Add(2 * time.Minute)
	require.NoError(t, limiter.RecordFailure(ctx, "login|b", options))
	require.NoError(t, limiter.RecordFailure(ctx, "login|b", options))
	blocked, _, err = limiter.Check(ctx, "login|b", options)
	require.NoError(t, err)
	require.False(t, blocked, "a failure after the window starts a new count")

	require.NoError(t, limiter.RecordSuccess(ctx, "login|b"))
	blocked, _, err = limiter.Check(ctx, "login|b", Options{})
	require.NoError(t, err)
	require.False(t, blocked, "disabled limits never block")
}

func attemptLimiter(t *testing.T, now *time.Time) *Limiter {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/throttle.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.Exec(`CREATE TABLE t_throttle (throttle_key VARCHAR(255) PRIMARY KEY, failures INTEGER NOT NULL,
		window_start BIGINT NOT NULL, lock_until BIGINT NOT NULL, last_seen BIGINT NOT NULL)`).Error)
	return &Limiter{DB: db, Table: "t_throttle", Now: func() time.Time { return *now }}
}

// A burst of concurrent attempts is counted one by one: with Check and
// RecordFailure, every request of the burst passed the check before any of them
// had recorded, so the limit did not hold against parallel guesses.
func TestAttemptAllowsExactlyTheLimitToABurst(t *testing.T) {
	now := time.Unix(1790000000, 0)
	limiter := attemptLimiter(t, &now)
	options := Options{Enabled: true, MaxAttempts: 5, Window: time.Minute, Lockout: 10 * time.Minute}

	const burst = 40
	var allowed atomic.Int32
	var wait sync.WaitGroup
	for range burst {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ok, _, err := limiter.Attempt(context.Background(), "login|a", options)
			if err == nil && ok {
				allowed.Add(1)
			}
		}()
	}
	wait.Wait()
	require.EqualValues(t, 5, allowed.Load(), "only MaxAttempts of a burst may run")

	ok, retry, err := limiter.Attempt(context.Background(), "login|a", options)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, 10*time.Minute, retry)
}

func TestAttemptLockoutReleaseAndSuccess(t *testing.T) {
	now := time.Unix(1790000000, 0)
	limiter := attemptLimiter(t, &now)
	options := Options{Enabled: true, MaxAttempts: 3, Window: time.Minute, Lockout: 10 * time.Minute}
	ctx := context.Background()
	attempt := func(key string) bool {
		ok, _, err := limiter.Attempt(ctx, key, options)
		require.NoError(t, err)
		return ok
	}

	require.True(t, attempt("k"))
	require.True(t, attempt("k"))
	require.True(t, attempt("k"), "the attempt that reaches the limit still runs")
	require.False(t, attempt("k"), "and locks the key")

	now = now.Add(10*time.Minute + time.Second)
	require.True(t, attempt("k"), "the lockout ends and the count starts again")

	// An attempt that checked nothing is given back, also the one that locked.
	require.True(t, attempt("r"))
	require.True(t, attempt("r"))
	require.True(t, attempt("r"))
	require.NoError(t, limiter.Release(ctx, "r", options))
	require.True(t, attempt("r"), "the released attempt is available again")
	require.False(t, attempt("r"))

	require.True(t, attempt("s"))
	require.True(t, attempt("s"))
	require.NoError(t, limiter.RecordSuccess(ctx, "s"))
	require.True(t, attempt("s"))
	require.True(t, attempt("s"))
	require.True(t, attempt("s"), "a success clears the key")

	off := Options{}
	for range 10 {
		ok, _, err := limiter.Attempt(ctx, "off", off)
		require.NoError(t, err)
		require.True(t, ok, "disabled limits never refuse")
	}

	now = now.Add(2 * time.Minute)
	require.True(t, attempt("w"))
	require.True(t, attempt("w"))
	now = now.Add(2 * time.Minute)
	require.True(t, attempt("w"))
	require.True(t, attempt("w"))
	require.True(t, attempt("w"), "attempts outside the window do not add up")
}
