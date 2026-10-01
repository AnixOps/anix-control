package subscriber

import (
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// ChangeRetention is how long the change log keeps rows; a consumer whose
// cursor is older lists again.
const ChangeRetention = 7 * 24 * time.Hour

// nodeFields are the v2_user columns whose change can alter what a node
// serves.
var nodeFields = []string{
	"banned", "uuid", "plan_id", "group_id", "expired_at", "transfer_enable", "speed_limit", "device_limit", "u", "d",
}

// TouchesNodeFields reports whether a v2_user update changes a column that
// node user lists depend on.
func TouchesNodeFields(updates map[string]any) bool {
	for _, field := range nodeFields {
		if _, ok := updates[field]; ok {
			return true
		}
	}
	return false
}

// changeBatch bounds the rows of one insert: a statement takes at most
// 65535 parameters on PostgreSQL.
const changeBatch = 1000

// RecordChangesTx appends change log rows for the subscribers in tx, in
// the given order.
func RecordChangesTx(tx *gorm.DB, userIDs []uint, deleted bool, now time.Time) error {
	if len(userIDs) == 0 {
		return nil
	}
	rows := make([]model.SubscriberChange, 0, len(userIDs))
	for _, id := range userIDs {
		if id != 0 {
			rows = append(rows, model.SubscriberChange{UserID: id, Deleted: deleted, CreatedAt: now})
		}
	}
	if len(rows) == 0 {
		return nil
	}
	if err := lockChangeLogTx(tx); err != nil {
		return err
	}
	return tx.CreateInBatches(&rows, changeBatch).Error
}

// lockChangeLogTx makes the change log's writers commit in id order, which
// consumers rely on: they read the rows after their cursor and move it to
// the last id read. A PostgreSQL sequence hands out ids when rows are
// inserted, not when they commit, so without the lock a later id could
// commit first, a consumer move past the earlier one, and that row, once
// committed, never be read. The transaction-scoped advisory lock is held
// until commit or rollback, so the next writer takes its ids after this one
// is visible. SQLite has a single writer already. Record changes after the
// transaction's other row locks: a writer waits here holding what it locked.
func lockChangeLogTx(tx *gorm.DB) error {
	if tx.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", model.SubscriberChange{}.TableName()).Error
}

// RecordNodeGroupChangeTx appends a change for each active subscriber a
// node served or now serves when its group moves from previous to next (nil
// is every subscriber). The node's users are derived from its group, so a
// watcher of the node would otherwise keep the old group's users and never
// get the new group's until each of them changed: the changes make it
// re-read those subscribers on the node, adding the new group's and
// removing the old group's. Nothing is recorded when the group is the same.
func RecordNodeGroupChangeTx(tx *gorm.DB, previous, next *uint, now time.Time) error {
	if sameGroup(previous, next) {
		return nil
	}
	query := Active(tx.Model(&model.User{}), now)
	if previous != nil && next != nil {
		query = query.Where("group_id IN ?", []uint{*previous, *next})
	}
	var affected []uint
	if err := query.Order("id").Pluck("id", &affected).Error; err != nil {
		return err
	}
	return RecordChangesTx(tx, affected, false, now)
}

func sameGroup(a, b *uint) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ChangesAfter returns up to limit changes after cursor. resync is true when
// rows after the cursor may have been pruned: the consumer must list again.
func ChangesAfter(db *gorm.DB, cursor uint64, limit int) (changes []model.SubscriberChange, resync bool, err error) {
	if cursor > 0 {
		var oldest model.SubscriberChange
		result := db.Order("id").Limit(1).Find(&oldest)
		if result.Error != nil {
			return nil, false, result.Error
		}
		if result.RowsAffected > 0 && oldest.ID > cursor+1 {
			return nil, true, nil
		}
	}
	err = db.Where("id > ?", cursor).Order("id").Limit(limit).Find(&changes).Error
	return changes, false, err
}

// Cursor returns the latest change cursor (0 when the log is empty).
func Cursor(db *gorm.DB) (uint64, error) {
	var cursor uint64
	err := db.Model(&model.SubscriberChange{}).Select("COALESCE(MAX(id), 0)").Scan(&cursor).Error
	return cursor, err
}

// PruneChanges deletes change log rows older than the retention.
func PruneChanges(db *gorm.DB, now time.Time) (int64, error) {
	result := db.Where("created_at < ?", now.Add(-ChangeRetention)).Delete(&model.SubscriberChange{})
	return result.RowsAffected, result.Error
}

// RequestRetention is how long the request ledger keeps a request id; a
// retry after that applies again.
const RequestRetention = 90 * 24 * time.Hour

// PruneRequests deletes request ledger rows older than RequestRetention.
func PruneRequests(db *gorm.DB, now time.Time) (int64, error) {
	result := db.Where("created_at < ?", now.Add(-RequestRetention)).Delete(&model.SubscriberRequest{})
	return result.RowsAffected, result.Error
}

// Active restricts a v2_user query to the subscribers a node serves: not
// banned, not expired, and within their traffic (a zero transfer limit
// serves nothing), as node user lists always have.
func Active(db *gorm.DB, now time.Time) *gorm.DB {
	return db.Where("banned = 0").
		Where("(expired_at IS NULL OR expired_at > ?)", now.Unix()).
		Where("(u + d) < transfer_enable")
}
