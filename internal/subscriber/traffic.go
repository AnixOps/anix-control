package subscriber

import (
	"errors"
	"sort"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNegativeTraffic means a traffic entry would decrease a counter.
var ErrNegativeTraffic = errors.New("traffic must not be negative")

// TrafficEntry is traffic to add to one subscriber's counters, in bytes
// already multiplied by any rate.
type TrafficEntry struct {
	UserID   uint
	Upload   int64
	Download int64
}

// TrafficResult is the outcome of RecordTrafficTx.
type TrafficResult struct {
	Applied bool `json:"-"`
	// Exhausted lists subscribers who crossed their transfer limit.
	Exhausted []uint `json:"exhausted,omitempty"`
}

// RecordTrafficTx adds traffic to subscribers' counters in tx, once per
// batch id (an empty batch id is not deduplicated: legacy node reports carry
// none). Entries for the same subscriber are summed, rows are updated in id
// order so concurrent reports cannot deadlock, and unknown subscribers are
// skipped, as before.
func RecordTrafficTx(tx *gorm.DB, batchID string, entries []TrafficEntry, now time.Time) (TrafficResult, error) {
	totals := make(map[uint][2]int64, len(entries))
	for _, entry := range entries {
		if entry.Upload < 0 || entry.Download < 0 {
			return TrafficResult{}, ErrNegativeTraffic
		}
		if entry.UserID == 0 || entry.Upload+entry.Download == 0 {
			continue
		}
		total := totals[entry.UserID]
		totals[entry.UserID] = [2]int64{total[0] + entry.Upload, total[1] + entry.Download}
	}
	var previous TrafficResult
	if seen, err := replay(tx, batchID, &previous); err != nil || seen {
		return previous, err
	}
	result := TrafficResult{Applied: true}
	if len(totals) > 0 {
		ids := make([]uint, 0, len(totals))
		for id := range totals {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		var users []model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "u", "d", "transfer_enable").
			Where("id IN ?", ids).Order("id").Find(&users).Error; err != nil {
			return TrafficResult{}, err
		}
		for _, user := range users {
			total := totals[user.ID]
			if err := tx.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
				"u": gorm.Expr("u + ?", total[0]),
				"d": gorm.Expr("d + ?", total[1]),
			}).Error; err != nil {
				return TrafficResult{}, err
			}
			before := user.U + user.D
			if user.TransferEnable > 0 && before < user.TransferEnable && before+total[0]+total[1] >= user.TransferEnable {
				result.Exhausted = append(result.Exhausted, user.ID)
			}
		}
	}
	if err := RecordChangesTx(tx, result.Exhausted, false, now); err != nil {
		return TrafficResult{}, err
	}
	return result, record(tx, batchID, "record_traffic", 0, result, now)
}

// ResetResult is the outcome of ResetTrafficTx.
type ResetResult struct {
	Applied bool  `json:"-"`
	Reset   int64 `json:"reset"`
}

// ResetTrafficTx zeroes subscribers' counters in tx, once per request id.
func ResetTrafficTx(tx *gorm.DB, requestID string, userIDs []uint, now time.Time) (ResetResult, error) {
	var previous ResetResult
	if seen, err := replay(tx, requestID, &previous); err != nil || seen {
		return previous, err
	}
	result := ResetResult{Applied: true}
	if len(userIDs) > 0 {
		update := tx.Model(&model.User{}).Where("id IN ?", userIDs).Updates(map[string]any{"u": 0, "d": 0})
		if update.Error != nil {
			return ResetResult{}, update.Error
		}
		result.Reset = update.RowsAffected
		if err := RecordChangesTx(tx, userIDs, false, now); err != nil {
			return ResetResult{}, err
		}
	}
	var owner uint
	if len(userIDs) == 1 {
		owner = userIDs[0]
	}
	return result, record(tx, requestID, "reset_traffic", owner, result, now)
}
