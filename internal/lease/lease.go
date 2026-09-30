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
// renews it well before then.
type Record struct {
	Name       string    `gorm:"primaryKey;size:100"`
	Holder     string    `gorm:"size:200;not null;default:''"`
	ExpiresAt  time.Time `gorm:"not null;index"`
	AcquiredAt time.Time
	RenewedAt  time.Time
}

// TableName keeps the lease table with the other kernel tables.
func (Record) TableName() string { return "v4_kernel_lease" }

// EnsureSchema creates the lease table.
func EnsureSchema(db *gorm.DB) error {
	return db.AutoMigrate(&Record{})
}

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
// or already held by holder. It reports whether holder owns the lease now.
func TryAcquire(ctx context.Context, db *gorm.DB, name, holder string, ttl time.Duration, now time.Time) (bool, error) {
	if name == "" || holder == "" || ttl <= 0 {
		return false, errors.New("lease name, holder and ttl are required")
	}
	acquired := false
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
		if current.Holder != holder {
			updates["acquired_at"] = now
		}
		if err := tx.Model(&Record{}).Where("name = ?", name).Updates(updates).Error; err != nil {
			return err
		}
		acquired = true
		return nil
	})
	return acquired, err
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

func (e *Elector) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now().UTC()
}

// Run blocks until ctx is done. Whenever the process holds the lease it runs
// work with a context that is cancelled as soon as leadership ends, and it
// waits for work to return before trying to lead again.
func (e *Elector) Run(ctx context.Context, work func(ctx context.Context)) {
	running.Store(e.Name, e)
	defer running.Delete(e.Name)
	interval := e.TTL / 3
	for {
		acquired, err := TryAcquire(ctx, e.DB, e.Name, e.Holder, e.TTL, e.now())
		if err != nil && ctx.Err() == nil {
			log.Printf("lease %s: acquire failed: %v", e.Name, err)
		}
		if acquired {
			e.lead(ctx, work, interval)
			if ctx.Err() != nil {
				releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := Release(releaseCtx, e.DB, e.Name, e.Holder, e.now()); err != nil {
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

func (e *Elector) lead(ctx context.Context, work func(ctx context.Context), interval time.Duration) {
	log.Printf("lease %s: acquired by %s", e.Name, e.Holder)
	e.setLeader(true)
	defer e.setLeader(false)

	workCtx, stopWork := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		work(workCtx)
	}()
	defer func() {
		stopWork()
		<-done
	}()

	lastRenewed := e.now()
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
		renewed, err := TryAcquire(ctx, e.DB, e.Name, e.Holder, e.TTL, e.now())
		switch {
		case err == nil && renewed:
			lastRenewed = e.now()
		case err == nil && !renewed:
			log.Printf("lease %s: lost to another process; stopping leader work", e.Name)
			return
		default:
			if ctx.Err() != nil {
				return
			}
			log.Printf("lease %s: renew failed: %v", e.Name, err)
			if e.now().Sub(lastRenewed) >= e.TTL*2/3 {
				log.Printf("lease %s: could not renew for %s; stopping leader work", e.Name, e.now().Sub(lastRenewed))
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
