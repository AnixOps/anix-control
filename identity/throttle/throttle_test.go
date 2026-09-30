package throttle

import (
	"context"
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
