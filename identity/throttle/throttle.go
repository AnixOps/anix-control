// Package throttle limits repeated failures per key (login and registration
// attempts) in a SQL table, so every replica of the identity service sees
// the same counts.
//
// A key collects attempts inside a window; reaching MaxAttempts locks it for
// Lockout. Success clears it. Attempt reserves an attempt before the check it
// guards runs, so concurrent requests cannot all pass a lock that none of them
// has counted yet; Check and RecordFailure count only after the fact and stay
// for callers that can tolerate that. The table is
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

// Attempt reserves one attempt on key before the guarded check runs, in one
// transaction that holds the key's row: the attempt that reaches MaxAttempts
// is still allowed and locks the key for Lockout, and every later attempt is
// refused until the lockout ends. Concurrent callers are counted one by one,
// so a burst cannot exceed MaxAttempts. A success clears the key
// (RecordSuccess); an attempt that checked nothing is given back (Release).
func (l *Limiter) Attempt(ctx context.Context, key string, options Options) (bool, time.Duration, error) {
	if !options.Enabled || key == "" || options.MaxAttempts <= 0 {
		return true, 0, nil
	}
	now := millis(l.now())
	allowed, wait := true, time.Duration(0)
	err := l.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The row exists before it is locked, so the first attempts of a key
		// also serialize on it.
		if err := tx.Table(l.Table).Clauses(clause.OnConflict{DoNothing: true}).
			Create(&row{Key: key, WindowStart: now, LastSeen: now}).Error; err != nil {
			return err
		}
		var state row
		if err := tx.Table(l.Table).Clauses(clause.Locking{Strength: "UPDATE"}).Where("throttle_key = ?", key).Take(&state).Error; err != nil {
			return err
		}
		if state.LockUntil > now {
			allowed, wait = false, time.Duration(state.LockUntil-now)*time.Millisecond
			return nil
		}
		if state.WindowStart+options.Window.Milliseconds() < now {
			state.Failures, state.WindowStart, state.LockUntil = 0, now, 0
		}
		state.Failures++
		state.LastSeen = now
		if state.Failures >= options.MaxAttempts {
			state.LockUntil = now + options.Lockout.Milliseconds()
		}
		if err := tx.Table(l.Table).Where("throttle_key = ?", key).
			Updates(map[string]any{"failures": state.Failures, "window_start": state.WindowStart, "lock_until": state.LockUntil, "last_seen": state.LastSeen}).Error; err != nil {
			return err
		}
		stale := now - (options.Window + options.Lockout + 5*time.Minute).Milliseconds()
		return tx.Table(l.Table).Where("last_seen < ? AND lock_until < ? AND throttle_key <> ?", stale, now, key).Delete(&row{}).Error
	})
	return allowed, wait, err
}

// Release gives back one attempt reserved by Attempt that checked nothing
// (for example a password that was right and now waits for its second
// factor). It never lifts a lock another failure set.
func (l *Limiter) Release(ctx context.Context, key string, options Options) error {
	if !options.Enabled || key == "" || options.MaxAttempts <= 0 {
		return nil
	}
	return l.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state row
		err := tx.Table(l.Table).Clauses(clause.Locking{Strength: "UPDATE"}).Where("throttle_key = ?", key).Take(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if state.Failures <= 0 {
			return nil
		}
		state.Failures--
		updates := map[string]any{"failures": state.Failures}
		// A key locked by this very attempt is unlocked again when it falls
		// below the limit; a lock that already ran out stays as it is.
		if state.Failures < options.MaxAttempts && state.LockUntil > millis(l.now()) {
			updates["lock_until"] = 0
		}
		return tx.Table(l.Table).Where("throttle_key = ?", key).Updates(updates).Error
	})
}

// RecordSuccess clears key.
func (l *Limiter) RecordSuccess(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	return l.DB.WithContext(ctx).Table(l.Table).Where("throttle_key = ?", key).Delete(&row{}).Error
}
