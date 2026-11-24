// Package lease provides database-backed leases so that work which must run
// in exactly one Control process (periodic resets, stats collection, the
// forward job executors) stays single even when several processes share one
// database: during a rolling update, or with more than one replica.
package lease

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Record is one named lease. Holder owns it until ExpiresAt; the holder
// renews it well before then. Generation is raised each time the lease
// changes holder: the fencing token of Check. The schema migrations create
// its table (internal/database/migrations).
type Record struct {
	Name       string    `gorm:"primaryKey;size:100"`
	Holder     string    `gorm:"size:200;not null;default:''"`
	ExpiresAt  time.Time `gorm:"not null;index"`
	AcquiredAt time.Time
	RenewedAt  time.Time
	Generation int64 `gorm:"not null;default:0"`
}

// ErrLost refuses a write of leader work whose lease another process holds
// now, or that expired.
var ErrLost = errors.New("lease lost: another Control process leads this work")

// TableName keeps the lease table with the other kernel tables.
func (Record) TableName() string { return "v4_kernel_lease" }

// InstanceID identifies this process as a lease holder: the host name (the
// pod name on Kubernetes) plus a random suffix, so a restarted process never
// inherits a lease it did not renew.
func InstanceID() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "control"
	}
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return host
	}
	return host + "-" + hex.EncodeToString(suffix)
}

// TryAcquire takes or renews the lease for holder when it is free, expired,
// or already held by holder. It reports whether holder owns the lease now,
// and the lease's generation, raised when holder took it from another.
func TryAcquire(ctx context.Context, db *gorm.DB, name, holder string, ttl time.Duration, now time.Time) (int64, bool, error) {
	if name == "" || holder == "" || ttl <= 0 {
		return 0, false, errors.New("lease name, holder and ttl are required")
	}
	acquired := false
	var generation int64
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		seed := Record{Name: name, ExpiresAt: time.Unix(0, 0).UTC()}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		var current Record
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("name = ?", name).First(&current).Error; err != nil {
			return err
		}
		if current.Holder != holder && current.ExpiresAt.After(now) {
			return nil
		}
		updates := map[string]any{"holder": holder, "expires_at": now.Add(ttl), "renewed_at": now}
		generation = current.Generation
		if current.Holder != holder {
			generation++
			updates["acquired_at"], updates["generation"] = now, generation
		}
		if err := tx.Model(&Record{}).Where("name = ?", name).Updates(updates).Error; err != nil {
			return err
		}
		acquired = true
		return nil
	})
	return generation, acquired, err
}

// DatabaseNow is the database's clock (PostgreSQL now()), so that every
// Control process judges a lease's expiry by one clock, whatever its host's
// clock says. A SQLite database is a file on one host, opened by processes
// on that host: there the process clock is the database clock.
func DatabaseNow(ctx context.Context, db *gorm.DB) (time.Time, error) {
	if db.Dialector.Name() != "postgres" {
		return time.Now().UTC(), nil
	}
	var now time.Time
	err := db.WithContext(ctx).Raw("SELECT now()").Scan(&now).Error
	return now.UTC(), err
}

type fenceKey struct{}

type fence struct {
	name, holder string
	generation   int64
}

// Check fences a write of leader work (backend review ARC-003): called in
// the write's transaction, it reads the lease the work runs under (locked
// against a takeover until the transaction ends, on PostgreSQL) and returns
// ErrLost unless this process still holds it, at the generation it led
// with, unexpired by the database clock. A write that does not come from
// leader work (an administrator's request) has no lease and passes.
func Check(tx *gorm.DB) error {
	f, ok := tx.Statement.Context.Value(fenceKey{}).(fence)
	if !ok {
		return nil
	}
	var current Record
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("name = ?", f.name).Take(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrLost
		}
		return err
	}
	now, err := DatabaseNow(tx.Statement.Context, tx)
	if err != nil {
		return err
	}
	if current.Holder != f.holder || current.Generation != f.generation || !current.ExpiresAt.After(now) {
		return ErrLost
	}
	return nil
}

// Release gives the lease up early if holder still owns it, so another
// process can take over without waiting for the expiry.
func Release(ctx context.Context, db *gorm.DB, name, holder string, now time.Time) error {
	return db.WithContext(ctx).Model(&Record{}).
		Where("name = ? AND holder = ?", name, holder).
		Update("expires_at", now).Error
}

// Elector runs work only while this process holds a named lease.
type Elector struct {
	DB     *gorm.DB
	Name   string
	Holder string
	// TTL is the lease duration; renewals happen every TTL/3. A leader that
	// cannot renew stops its work after 2/3 of the TTL, before another process
	// can take the lease over.
	TTL time.Duration
	Now func() time.Time

	mu     sync.Mutex
	leader bool
}

// IsLeader reports whether the elector currently runs its work.
func (e *Elector) IsLeader() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.leader
}

func (e *Elector) setLeader(value bool) {
	e.mu.Lock()
	e.leader = value
	e.mu.Unlock()
}

// clock is the time lease rows are written and judged by: the database
// clock, or Now in tests. How long the leader went without a renewal is
// measured on the process clock (lead), which needs no database.
func (e *Elector) clock(ctx context.Context) (time.Time, error) {
	if e.Now != nil {
		return e.Now(), nil
	}
	return DatabaseNow(ctx, e.DB)
}

func (e *Elector) tryAcquire(ctx context.Context) (int64, bool, error) {
	now, err := e.clock(ctx)
	if err != nil {
		return 0, false, err
	}
	return TryAcquire(ctx, e.DB, e.Name, e.Holder, e.TTL, now)
}

// Run blocks until ctx is done. Whenever the process holds the lease it runs
// work with a context that is cancelled as soon as leadership ends, and it
// waits for work to return before trying to lead again.
func (e *Elector) Run(ctx context.Context, work func(ctx context.Context)) {
	running.Store(e.Name, e)
	defer running.Delete(e.Name)
	interval := e.TTL / 3
	for {
		generation, acquired, err := e.tryAcquire(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("lease %s: acquire failed: %v", e.Name, err)
		}
		if acquired {
			e.lead(ctx, work, interval, generation)
			if ctx.Err() != nil {
				releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				now, err := e.clock(releaseCtx)
				if err == nil {
					err = Release(releaseCtx, e.DB, e.Name, e.Holder, now)
				}
				if err != nil {
					log.Printf("lease %s: release failed: %v", e.Name, err)
				}
				cancel()
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (e *Elector) lead(ctx context.Context, work func(ctx context.Context), interval time.Duration, generation int64) {
	log.Printf("lease %s: acquired by %s (generation %d)", e.Name, e.Holder, generation)
	e.setLeader(true)
	defer e.setLeader(false)

	// The work's writes that call Check are fenced by this generation.
	workCtx, stopWork := context.WithCancel(context.WithValue(ctx, fenceKey{}, fence{e.Name, e.Holder, generation}))
	done := make(chan struct{})
	go func() {
		defer close(done)
		work(workCtx)
	}()
	defer func() {
		stopWork()
		<-done
	}()

	lastRenewed := time.Now()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
		}
		current, renewed, err := e.tryAcquire(ctx)
		switch {
		case err == nil && renewed && current != generation:
			// It expired and was taken back after another holder: the
			// work restarts under the new generation.
			log.Printf("lease %s: taken again at generation %d; restarting leader work", e.Name, current)
			return
		case err == nil && renewed:
			lastRenewed = time.Now()
		case err == nil && !renewed:
			log.Printf("lease %s: lost to another process; stopping leader work", e.Name)
			return
		default:
			if ctx.Err() != nil {
				return
			}
			log.Printf("lease %s: renew failed: %v", e.Name, err)
			if time.Since(lastRenewed) >= e.TTL*2/3 {
				log.Printf("lease %s: could not renew for %s; stopping leader work", e.Name, time.Since(lastRenewed))
				return
			}
		}
	}
}

var running sync.Map // lease name -> *Elector

// WritePrometheus writes anixops_lease_leader: 1 for each lease this process
// currently holds, 0 for leases it is waiting for.
func WritePrometheus(body *strings.Builder) {
	var names []string
	running.Range(func(key, _ any) bool {
		names = append(names, key.(string))
		return true
	})
	if len(names) == 0 {
		return
	}
	sort.Strings(names)
	body.WriteString("# HELP anixops_lease_leader Whether this process holds the named lease (1) or waits for it (0).\n")
	body.WriteString("# TYPE anixops_lease_leader gauge\n")
	for _, name := range names {
		value, _ := running.Load(name)
		leader := "0"
		if value.(*Elector).IsLeader() {
			leader = "1"
		}
		body.WriteString("anixops_lease_leader{lease=\"" + name + "\"} " + leader + "\n")
	}
}
