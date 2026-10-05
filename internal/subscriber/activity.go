package subscriber

import (
	"log/slog"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// ActivityWriteInterval is the least time between two writes of one user's
// last-online time. A node reports a user's traffic and connections every
// minute or so; the stored time lags the report by less than this.
const ActivityWriteInterval = time.Minute

const (
	// activityBatch bounds the rows of one upsert.
	activityBatch = 500
	// activityMemoryLimit bounds the throttle's memory: past it, entries
	// older than the interval are dropped.
	activityMemoryLimit = 200_000
)

// activityThrottle remembers, per database, when each user's activity was
// last claimed for a write, so a report does not write per user per request.
type activityThrottle struct {
	mu   sync.Mutex
	last map[uint]int64
}

// activityThrottles holds one throttle per database handle (*sql.DB): a
// process that opens another database (a test, a restore) starts afresh.
var activityThrottles sync.Map

func activityThrottleFor(db *gorm.DB) *activityThrottle {
	var key any = db
	if sqlDB, err := db.DB(); err == nil {
		key = sqlDB
	}
	if existing, ok := activityThrottles.Load(key); ok {
		return existing.(*activityThrottle)
	}
	created, _ := activityThrottles.LoadOrStore(key, &activityThrottle{last: map[uint]int64{}})
	return created.(*activityThrottle)
}

// claim returns the users whose activity at (a Unix time) is to be written:
// those not claimed within the interval, each once. It marks them claimed.
func (t *activityThrottle) claim(userIDs []uint, at, interval int64) []uint {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.last) > activityMemoryLimit {
		for id, claimed := range t.last {
			if at-claimed >= interval {
				delete(t.last, id)
			}
		}
	}
	due := make([]uint, 0, len(userIDs))
	for _, id := range userIDs {
		if id == 0 {
			continue
		}
		if claimed, ok := t.last[id]; ok && (at < claimed || at-claimed < interval) {
			continue
		}
		t.last[id] = at
		due = append(due, id)
	}
	return due
}

// RecordOnline notes that the users were seen online at at: their traffic
// was reported, or a node listed their connections. It is the one writer of
// v4_kernel_user_activity and is best effort: it never fails or delays the
// report it belongs to (call it after the report committed), and an error is
// logged and not retried before the interval passes. Each user is written at
// most once per ActivityWriteInterval, in multi-row upserts that only move a
// stored time forward and only for users that exist, so concurrent Control processes and late reports
// cannot move it back.
func RecordOnline(db *gorm.DB, userIDs []uint, at time.Time) {
	if db == nil || len(userIDs) == 0 {
		return
	}
	due := activityThrottleFor(db).claim(userIDs, at.Unix(), int64(ActivityWriteInterval/time.Second))
	for start := 0; start < len(due); start += activityBatch {
		end := min(start+activityBatch, len(due))
		if err := upsertActivity(db, due[start:end], at.Unix()); err != nil {
			slog.Warn("user activity was not recorded", "component", "subscriber", "users", end-start, "error", err)
		}
	}
}

// upsertActivity sets the last-online time of those of the users that exist
// to at, unless a later one is stored. A report naming a user id that is not
// a subscriber (a node's mistake) leaves no row, as its traffic counts for
// nobody.
func upsertActivity(db *gorm.DB, userIDs []uint, at int64) error {
	activity, users := model.UserActivity{}.TableName(), model.User{}.TableName()
	return db.Exec(
		"INSERT INTO "+activity+" (user_id, last_online_at) SELECT id, ? FROM "+users+" WHERE id IN ? "+
			"ON CONFLICT (user_id) DO UPDATE SET last_online_at = excluded.last_online_at "+
			"WHERE excluded.last_online_at > "+activity+".last_online_at",
		at, userIDs).Error
}

// LastOnline returns the last-online time (a Unix time) of each of the users
// that has one; a user never seen online is not in the map.
func LastOnline(db *gorm.DB, userIDs []uint) (map[uint]int64, error) {
	seen := make(map[uint]int64, len(userIDs))
	if len(userIDs) == 0 {
		return seen, nil
	}
	var rows []model.UserActivity
	if err := db.Where("user_id IN ?", userIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		seen[row.UserID] = row.LastOnlineAt
	}
	return seen, nil
}

// ForgetActivityTx drops a deleted user's last-online time in tx, so a later
// user that gets the same id (SQLite reuses the highest one) does not inherit
// it. A database without the table is left alone.
func ForgetActivityTx(tx *gorm.DB, userID uint) error {
	if !tx.Migrator().HasTable(&model.UserActivity{}) {
		return nil
	}
	return tx.Where("user_id = ?", userID).Delete(&model.UserActivity{}).Error
}

// ResetActivityThrottleForTest forgets which users were written on db, so a
// test can record the same user again within the interval.
func ResetActivityThrottleForTest(db *gorm.DB) {
	throttle := activityThrottleFor(db)
	throttle.mu.Lock()
	defer throttle.mu.Unlock()
	throttle.last = map[uint]int64{}
}
