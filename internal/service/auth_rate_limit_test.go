package service

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
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
