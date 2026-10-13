package subscriber

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
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

// trafficUpdateBatch bounds the subscribers of one UPDATE. The statement
// carries five values per subscriber (two CASE arms and one IN item), far
// below the 32766 parameters of SQLite and the 65535 of PostgreSQL, and a
// CASE with n arms costs n comparisons per updated row, so the batch is also
// kept small enough for that quadratic part to stay below the cost of the
// statements it replaces.
const trafficUpdateBatch = 100

// RecordTrafficTx adds traffic to subscribers' counters in tx, once per
// batch id (an empty batch id is not deduplicated: legacy node reports carry
// none). Entries for the same subscriber are summed, unknown subscribers are
// skipped, as before, and the counters are updated trafficUpdateBatch
// subscribers to a statement in id order, so concurrent reports cannot
// deadlock and a report costs a few statements, not one per subscriber.
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
	if seen, err := Replay(tx, batchID, &previous); err != nil || seen {
		return previous, err
	}
	result := TrafficResult{Applied: true}
	if len(totals) > 0 {
		ids := make([]uint, 0, len(totals))
		for id := range totals {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for start := 0; start < len(ids); start += trafficUpdateBatch {
			exhausted, err := addTrafficBatchTx(tx, ids[start:min(start+trafficUpdateBatch, len(ids))], totals)
			if err != nil {
				return TrafficResult{}, err
			}
			result.Exhausted = append(result.Exhausted, exhausted...)
		}
	}
	if err := RecordChangesTx(tx, result.Exhausted, false, now); err != nil {
		return TrafficResult{}, err
	}
	return result, Record(tx, batchID, "record_traffic", 0, result, now)
}

// addTrafficBatchTx adds totals to the counters of the subscribers among ids
// (ascending), one SELECT that locks them and one UPDATE, and returns those
// that crossed their transfer limit by it.
func addTrafficBatchTx(tx *gorm.DB, ids []uint, totals map[uint][2]int64) ([]uint, error) {
	var users []model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "u", "d", "transfer_enable").
		Where("id IN ?", ids).Order("id").Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}
	var exhausted []uint
	// u = u + CASE id WHEN <id> THEN <upload> ... END, and the same for d.
	// The values are cast: PostgreSQL types a bare parameter in a CASE as
	// text.
	var upload, download strings.Builder
	var uploadArgs, downloadArgs []any
	known := make([]uint, 0, len(users))
	for _, user := range users {
		total := totals[user.ID]
		upload.WriteString(" WHEN ? THEN CAST(? AS BIGINT)")
		uploadArgs = append(uploadArgs, user.ID, total[0])
		download.WriteString(" WHEN ? THEN CAST(? AS BIGINT)")
		downloadArgs = append(downloadArgs, user.ID, total[1])
		known = append(known, user.ID)
		before := user.U + user.D
		if user.TransferEnable > 0 && before < user.TransferEnable && before+total[0]+total[1] >= user.TransferEnable {
			exhausted = append(exhausted, user.ID)
		}
	}
	err := tx.Model(&model.User{}).Where("id IN ?", known).Updates(map[string]any{
		"u": gorm.Expr("u + CASE id"+upload.String()+" ELSE 0 END", uploadArgs...),
		"d": gorm.Expr("d + CASE id"+download.String()+" ELSE 0 END", downloadArgs...),
	}).Error
	return exhausted, err
}

// ResetResult is the outcome of ResetTrafficTx.
type ResetResult struct {
	Applied bool  `json:"-"`
	Reset   int64 `json:"reset"`
}

// ResetTrafficTx zeroes subscribers' counters in tx, once per request id.
func ResetTrafficTx(tx *gorm.DB, requestID string, userIDs []uint, now time.Time) (ResetResult, error) {
	var previous ResetResult
	if seen, err := Replay(tx, requestID, &previous); err != nil || seen {
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
	return result, Record(tx, requestID, "reset_traffic", owner, result, now)
}
