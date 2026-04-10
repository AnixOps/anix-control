package service

import (
	"testing"
	"time"

	"github.com/anixops/v2board/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestResolveLoginRateLimitOptions_Defaults(t *testing.T) {
	options := ResolveLoginRateLimitOptions(nil)

	assert.True(t, options.Enabled)
	assert.Equal(t, defaultLoginRateLimitMaxAttempts, options.MaxAttempts)
	assert.Equal(t, time.Duration(defaultLoginRateLimitWindowSeconds)*time.Second, options.Window)
	assert.Equal(t, time.Duration(defaultLoginRateLimitLockoutSecond)*time.Second, options.Lockout)
}

func TestResolveLoginRateLimitOptions_CustomConfig(t *testing.T) {
	enabled := false
	cfg := &config.Config{
		Auth: config.AuthConfig{
			LoginRateLimit: config.LoginRateLimitConfig{
				Enabled:        &enabled,
				MaxAttempts:    3,
				WindowSeconds:  30,
				LockoutSeconds: 45,
			},
		},
	}

	options := ResolveLoginRateLimitOptions(cfg)
	assert.False(t, options.Enabled)
	assert.Equal(t, 3, options.MaxAttempts)
	assert.Equal(t, 30*time.Second, options.Window)
	assert.Equal(t, 45*time.Second, options.Lockout)
}

func TestLoginRateLimiter_LocksAndResets(t *testing.T) {
	current := time.Unix(1_700_000_000, 0)
	limiter := NewLoginRateLimiter()
	limiter.nowFunc = func() time.Time { return current }

	options := LoginRateLimitOptions{
		Enabled:      true,
		MaxAttempts:  2,
		Window:       60 * time.Second,
		Lockout:      120 * time.Second,
		CleanupAfter: 5 * time.Minute,
	}
	key := BuildLoginRateLimitKey("user@example.com", "127.0.0.1")

	blocked, _ := limiter.Check(key, options)
	assert.False(t, blocked)

	limiter.RecordFailure(key, options)
	blocked, _ = limiter.Check(key, options)
	assert.False(t, blocked)

	limiter.RecordFailure(key, options)
	blocked, retry := limiter.Check(key, options)
	assert.True(t, blocked)
	assert.Greater(t, int(retry.Seconds()), 0)

	limiter.RecordSuccess(key)
	blocked, _ = limiter.Check(key, options)
	assert.False(t, blocked)
}
