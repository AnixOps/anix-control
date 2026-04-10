package service

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anixops/v2board/internal/config"
)

const (
	defaultLoginRateLimitEnabled       = true
	defaultLoginRateLimitMaxAttempts   = 6
	defaultLoginRateLimitWindowSeconds = 300
	defaultLoginRateLimitLockoutSecond = 600
)

type LoginRateLimitOptions struct {
	Enabled      bool
	MaxAttempts  int
	Window       time.Duration
	Lockout      time.Duration
	CleanupAfter time.Duration
}

type loginRateLimitState struct {
	Failures    int
	WindowStart time.Time
	LockUntil   time.Time
	LastSeen    time.Time
}

type LoginRateLimiter struct {
	mu      sync.Mutex
	nowFunc func() time.Time
	states  map[string]loginRateLimitState
}

func NewLoginRateLimiter() *LoginRateLimiter {
	return &LoginRateLimiter{
		nowFunc: time.Now,
		states:  make(map[string]loginRateLimitState),
	}
}

var globalLoginRateLimiter = NewLoginRateLimiter()

func GetLoginRateLimiter() *LoginRateLimiter {
	return globalLoginRateLimiter
}

func ResetLoginRateLimiterForTest() {
	globalLoginRateLimiter = NewLoginRateLimiter()
}

func ResolveLoginRateLimitOptions(cfg *config.Config) LoginRateLimitOptions {
	enabled := defaultLoginRateLimitEnabled
	maxAttempts := defaultLoginRateLimitMaxAttempts
	windowSeconds := defaultLoginRateLimitWindowSeconds
	lockoutSeconds := defaultLoginRateLimitLockoutSecond

	if cfg != nil {
		loginRateCfg := cfg.Auth.LoginRateLimit
		if loginRateCfg.Enabled != nil {
			enabled = *loginRateCfg.Enabled
		}
		if loginRateCfg.MaxAttempts > 0 {
			maxAttempts = loginRateCfg.MaxAttempts
		}
		if loginRateCfg.WindowSeconds > 0 {
			windowSeconds = loginRateCfg.WindowSeconds
		}
		if loginRateCfg.LockoutSeconds > 0 {
			lockoutSeconds = loginRateCfg.LockoutSeconds
		}
	}

	window := time.Duration(windowSeconds) * time.Second
	lockout := time.Duration(lockoutSeconds) * time.Second

	return LoginRateLimitOptions{
		Enabled:      enabled,
		MaxAttempts:  maxAttempts,
		Window:       window,
		Lockout:      lockout,
		CleanupAfter: window + lockout + 5*time.Minute,
	}
}

func BuildLoginRateLimitKey(email, ip string) string {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	normalizedIP := strings.TrimSpace(ip)
	if normalizedIP == "" {
		normalizedIP = "unknown"
	}
	return fmt.Sprintf("%s|%s", normalizedEmail, normalizedIP)
}

func (l *LoginRateLimiter) Check(key string, options LoginRateLimitOptions) (bool, time.Duration) {
	if !options.Enabled || key == "" {
		return false, 0
	}

	now := l.nowFunc()

	l.mu.Lock()
	defer l.mu.Unlock()

	state, ok := l.states[key]
	if !ok {
		return false, 0
	}

	state.LastSeen = now

	if state.LockUntil.After(now) {
		l.states[key] = state
		return true, state.LockUntil.Sub(now)
	}

	if state.WindowStart.Add(options.Window).Before(now) {
		delete(l.states, key)
		return false, 0
	}

	l.states[key] = state
	return false, 0
}

func (l *LoginRateLimiter) RecordFailure(key string, options LoginRateLimitOptions) {
	if !options.Enabled || key == "" || options.MaxAttempts <= 0 {
		return
	}

	now := l.nowFunc()

	l.mu.Lock()
	defer l.mu.Unlock()

	state, ok := l.states[key]
	if !ok || state.WindowStart.Add(options.Window).Before(now) {
		state = loginRateLimitState{
			Failures:    1,
			WindowStart: now,
			LastSeen:    now,
		}
	} else {
		state.Failures++
		state.LastSeen = now
	}

	if state.Failures >= options.MaxAttempts {
		state.LockUntil = now.Add(options.Lockout)
	}

	l.states[key] = state
	l.cleanupLocked(now, options.CleanupAfter)
}

func (l *LoginRateLimiter) RecordSuccess(key string) {
	if key == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.states, key)
}

func (l *LoginRateLimiter) cleanupLocked(now time.Time, ttl time.Duration) {
	if len(l.states) < 2048 {
		return
	}

	for key, state := range l.states {
		if state.LastSeen.Add(ttl).Before(now) {
			delete(l.states, key)
		}
	}
}
