package backup

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
	"time"
)

// The daily backup's time of day: within the window that starts at
// DailyWindowStart (local time) and lasts DailyWindow, at a minute derived
// from a stable hash of the panel's domain, so the panels of a fleet do not
// all back up at once and one panel always backs up at the same time.
const (
	DailyWindowStart = 3 * time.Hour
	DailyWindow      = 2 * time.Hour
)

// DailyOffset is the minute of the window seed (the domain, else the host
// name) backs up at.
func DailyOffset(seed string) time.Duration {
	sum := sha256.Sum256([]byte("anixops daily backup v1\x00" + strings.ToLower(strings.TrimSpace(seed))))
	minutes := binary.BigEndian.Uint64(sum[:8]) % uint64(DailyWindow/time.Minute)
	return DailyWindowStart + time.Duration(minutes)*time.Minute // #nosec G115 -- below 120.
}

// NextDaily answers the first daily backup time after now, in now's
// location.
func NextDaily(seed string, now time.Time) time.Time {
	year, month, day := now.Date()
	at := time.Date(year, month, day, 0, 0, 0, 0, now.Location()).Add(DailyOffset(seed))
	if !at.After(now) {
		at = time.Date(year, month, day+1, 0, 0, 0, 0, now.Location()).Add(DailyOffset(seed))
	}
	return at
}

// catchUpAfter: when the newest daily bundle is older than this at the
// worker's start (a new panel, or one that was down at its backup time),
// the worker backs up once catchUpDelay after its start.
const (
	catchUpAfter = 26 * time.Hour
	catchUpDelay = 2 * time.Minute
)

// Worker runs the daily backup, its verification and the retention. It is
// one of the singleton workers: one Control process runs it.
type Worker struct {
	Manager *Manager
	// Seed picks the daily time (DailyOffset): the domain, else the host
	// name.
	Seed string
	// Now defaults to time.Now (local time); After to time.After.
	Now   func() time.Time
	After func(time.Duration) <-chan time.Time
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func (w *Worker) after(d time.Duration) <-chan time.Time {
	if w.After != nil {
		return w.After(d)
	}
	return time.After(d)
}

// Run works until ctx ends. At its start it verifies the newest bundle not
// verified yet (the pre-migration backup of this start, a manual one) and
// applies the retention; then it backs up every day at the panel's time.
func (w *Worker) Run(ctx context.Context) {
	m := w.Manager
	if m == nil {
		return
	}
	if err := m.Supported(); err != nil {
		m.logf("Backup: the daily backup does not run: %v", err)
		return
	}
	w.verifyAndPrune(ctx)
	next := NextDaily(w.Seed, w.now())
	if w.dailyMissed(ctx) {
		next = w.now().Add(catchUpDelay)
	}
	m.logf("Backup: the next daily backup is at %s.", next.Format("2006-01-02 15:04 MST"))
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.after(next.Sub(w.now())):
		}
		w.RunDaily(ctx)
		next = NextDaily(w.Seed, w.now())
	}
}

// RunDaily takes the daily bundle, verifies it and applies the retention.
func (w *Worker) RunDaily(ctx context.Context) {
	m := w.Manager
	if _, _, err := m.CreateWait(ctx, TypeDaily); err != nil {
		if ctx.Err() == nil {
			m.logf("Backup: the daily backup FAILED: %v", err)
		}
		return
	}
	w.verifyAndPrune(ctx)
}

func (w *Worker) verifyAndPrune(ctx context.Context) {
	m := w.Manager
	if _, _, err := m.VerifyNewestUnverified(ctx); err != nil && ctx.Err() == nil {
		var failed *VerifyError
		if !errors.As(err, &failed) {
			m.logf("Backup: the verification did not run: %v", err)
		}
	}
	if _, err := m.Prune(ctx); err != nil && ctx.Err() == nil {
		m.logf("Backup: the retention did not run: %v", err)
	}
}

// dailyMissed reports whether the newest daily bundle is older than
// catchUpAfter (or there is none).
func (w *Worker) dailyMissed(ctx context.Context) bool {
	rows, err := w.Manager.List(ctx)
	if err != nil {
		return false
	}
	for _, row := range rows {
		if row.Type == TypeDaily {
			return w.now().Sub(row.CreatedAt) > catchUpAfter
		}
	}
	return true
}
