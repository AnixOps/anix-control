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

// RecordChangesTx appends change log rows for the subscribers in tx.
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
	return tx.Create(&rows).Error
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
