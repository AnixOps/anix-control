// Package throttle limits repeated failures per key (login and registration
// attempts) in a SQL table, so every replica of the identity service sees
// the same counts.
//
// A key collects failures inside a window; reaching MaxAttempts locks it for
// Lockout. Success clears it. The table is
//
//	throttle(throttle_key VARCHAR(255) PK, failures INT, window_start, lock_until, last_seen BIGINT)
//
// with times in Unix milliseconds.
package throttle

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Options configure one kind of attempt.
type Options struct {
	Enabled     bool
	MaxAttempts int
	Window      time.Duration
	Lockout     time.Duration
}

// Limiter counts failures in Table.
type Limiter struct {
	DB    *gorm.DB
	Table string
	// Now defaults to time.Now.
	Now func() time.Time
}

type row struct {
	Key         string `gorm:"column:throttle_key;primaryKey"`
	Failures    int    `gorm:"column:failures"`
	WindowStart int64  `gorm:"column:window_start"`
	LockUntil   int64  `gorm:"column:lock_until"`
	LastSeen    int64  `gorm:"column:last_seen"`
}

func (l *Limiter) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func millis(t time.Time) int64 { return t.UnixMilli() }

// Check reports whether key is locked, and for how long.
func (l *Limiter) Check(ctx context.Context, key string, options Options) (bool, time.Duration, error) {
	if !options.Enabled || key == "" {
		return false, 0, nil
	}
	now := millis(l.now())
	var state row
	err := l.DB.WithContext(ctx).Table(l.Table).Where("throttle_key = ?", key).Take(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if state.LockUntil > now {
		return true, time.Duration(state.LockUntil-now) * time.Millisecond, nil
	}
	if state.WindowStart+options.Window.Milliseconds() < now {
		return false, 0, l.DB.WithContext(ctx).Table(l.Table).Where("throttle_key = ?", key).Delete(&row{}).Error
	}
	return false, 0, nil
}

// RecordFailure counts a failure and locks the key at MaxAttempts.
func (l *Limiter) RecordFailure(ctx context.Context, key string, options Options) error {
	if !options.Enabled || key == "" || options.MaxAttempts <= 0 {
		return nil
	}
	now := millis(l.now())
	return l.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state row
		err := tx.Table(l.Table).Clauses(clause.Locking{Strength: "UPDATE"}).Where("throttle_key = ?", key).Take(&state).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && state.WindowStart+options.Window.Milliseconds() < now):
			state = row{Key: key, Failures: 1, WindowStart: now, LastSeen: now}
		case err != nil:
			return err
		default:
			state.Failures++
			state.LastSeen = now
		}
		if state.Failures >= options.MaxAttempts {
			state.LockUntil = now + options.Lockout.Milliseconds()
		}
		if err := tx.Table(l.Table).Clauses(clause.OnConflict{UpdateAll: true}).Create(&state).Error; err != nil {
			return err
		}
		// Forget keys idle longer than a window and a lockout.
		stale := now - (options.Window + options.Lockout + 5*time.Minute).Milliseconds()
		return tx.Table(l.Table).Where("last_seen < ? AND lock_until < ?", stale, now).Delete(&row{}).Error
	})
}

// RecordSuccess clears key.
func (l *Limiter) RecordSuccess(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	return l.DB.WithContext(ctx).Table(l.Table).Where("throttle_key = ?", key).Delete(&row{}).Error
}
