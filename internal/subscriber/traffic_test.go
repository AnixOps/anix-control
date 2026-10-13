package subscriber

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecordTrafficSumsDedupesAndReportsExhaustion(t *testing.T) {
	db := openDB(t)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "a@example.test", Token: "t1", UUID: "u1", TransferEnable: 100, U: 40, D: 40},
		{ID: 2, Email: "b@example.test", Token: "t2", UUID: "u2", TransferEnable: 0},
	}).Error)
	record := func(batch string, entries ...TrafficEntry) TrafficResult {
		var result TrafficResult
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			var err error
			result, err = RecordTrafficTx(tx, batch, entries, time.Now())
			return err
		}))
		return result
	}

	first := record("node:1:1", TrafficEntry{UserID: 1, Upload: 5, Download: 5}, TrafficEntry{UserID: 1, Upload: 10},
		TrafficEntry{UserID: 2, Upload: 1 << 40}, TrafficEntry{UserID: 99, Upload: 1})
	require.True(t, first.Applied)
	require.Equal(t, []uint{1}, first.Exhausted, "80 + 20 reaches the 100-byte limit; no limit never exhausts")
	var user model.User
	require.NoError(t, db.Take(&user, 1).Error)
	require.Equal(t, int64(55), user.U)
	require.Equal(t, int64(45), user.D)

	again := record("node:1:1", TrafficEntry{UserID: 1, Upload: 5})
	require.False(t, again.Applied)
	require.Equal(t, []uint{1}, again.Exhausted, "a repeated batch answers with its first result")
	require.NoError(t, db.Take(&user, 1).Error)
	require.Equal(t, int64(55), user.U, "and changes nothing")

	unbatched := record("", TrafficEntry{UserID: 1, Upload: 1})
	require.Empty(t, unbatched.Exhausted, "already over the limit: no new exhaustion")

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := RecordTrafficTx(tx, "node:1:2", []TrafficEntry{{UserID: 1, Upload: -1}}, time.Now())
		require.ErrorIs(t, err, ErrNegativeTraffic)
		return nil
	}))
}

func TestResetTrafficOncePerRequest(t *testing.T) {
	db := openDB(t)
	require.NoError(t, db.Create(&model.User{ID: 1, Email: "a@example.test", Token: "t1", UUID: "u1", U: 7, D: 8}).Error)
	reset := func(request string) ResetResult {
		var result ResetResult
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			var err error
			result, err = ResetTrafficTx(tx, request, []uint{1, 99}, time.Now())
			return err
		}))
		return result
	}
	first := reset("reset:monthly:2026-10")
	require.True(t, first.Applied)
	require.Equal(t, int64(1), first.Reset)
	require.NoError(t, db.Model(&model.User{}).Where("id = 1").Update("u", 3).Error)
	again := reset("reset:monthly:2026-10")
	require.False(t, again.Applied)
	var user model.User
	require.NoError(t, db.Take(&user, 1).Error)
	require.Equal(t, int64(3), user.U, "a repeated reset request changes nothing")
}

// recordTrafficPerUser is RecordTrafficTx as it was before the counters were
// updated in batches: one UPDATE per subscriber. The batched writer must
// leave the same counters, the same exhausted list and the same change log.
func recordTrafficPerUser(tx *gorm.DB, entries []TrafficEntry, now time.Time) (TrafficResult, error) {
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
	result := TrafficResult{Applied: true}
	if len(totals) > 0 {
		ids := make([]uint, 0, len(totals))
		for id := range totals {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		var users []model.User
		if err := tx.Select("id", "u", "d", "transfer_enable").Where("id IN ?", ids).Order("id").Find(&users).Error; err != nil {
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
	return result, RecordChangesTx(tx, result.Exhausted, false, now)
}

type counterRow struct {
	ID uint
	U  int64
	D  int64
}

func counters(t *testing.T, db *gorm.DB) []counterRow {
	t.Helper()
	var rows []counterRow
	require.NoError(t, db.Model(&model.User{}).Select("id", "u", "d").Order("id").Scan(&rows).Error)
	return rows
}

func changeLog(t *testing.T, db *gorm.DB) []uint {
	t.Helper()
	var ids []uint
	require.NoError(t, db.Model(&model.SubscriberChange{}).Order("id").Pluck("user_id", &ids).Error)
	return ids
}

// Random reports against two identical databases, one updated per
// subscriber and one in batches: the same counters, exhausted subscribers
// and change log after every report. The reports have more subscribers than
// a batch, repeated and unknown ones, zero traffic and subscribers without a
// limit.
func TestRecordTrafficBatchedUpdatesEqualPerSubscriberUpdates(t *testing.T) {
	requireBatchedUpdatesEqualPerSubscriberUpdates(t, openDB, 12)
}

// The same on PostgreSQL, where the CASE arms are typed and SELECT ... FOR
// UPDATE locks rows (skipped without ANIX_TEST_POSTGRES_DSN).
func TestPostgresRecordTrafficBatchedUpdatesEqualPerSubscriberUpdates(t *testing.T) {
	requireBatchedUpdatesEqualPerSubscriberUpdates(t, openPostgresDB, 3)
}

func requireBatchedUpdatesEqualPerSubscriberUpdates(t *testing.T, open func(*testing.T) *gorm.DB, seeds int64) {
	for seed := int64(1); seed <= seeds; seed++ {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			random := rand.New(rand.NewSource(seed))
			old, batched := open(t), open(t)
			users := 1 + random.Intn(3*trafficUpdateBatch+17)
			for id := 1; id <= users; id++ {
				row := model.User{
					ID: uint(id), Email: fmt.Sprintf("u%d@example.test", id), Token: fmt.Sprintf("t%d", id), UUID: fmt.Sprintf("id%d", id),
					U: random.Int63n(1 << 30), D: random.Int63n(1 << 30),
				}
				if random.Intn(5) > 0 { // some subscribers have no limit
					row.TransferEnable = row.U + row.D + random.Int63n(1<<31)
				}
				require.NoError(t, old.Create(&row).Error)
				require.NoError(t, batched.Create(&row).Error)
			}
			now := time.Unix(1_800_000_000, 0)
			for report := 0; report < 8; report++ {
				entries := make([]TrafficEntry, random.Intn(2*users+5))
				for i := range entries {
					entries[i] = TrafficEntry{
						// ids past users are unknown subscribers; 0 is skipped
						UserID:   uint(random.Intn(users + 6)),
						Upload:   random.Int63n(1 << 29),
						Download: random.Int63n(1 << 29),
					}
					if random.Intn(6) == 0 {
						entries[i].Upload, entries[i].Download = 0, 0
					}
				}
				var want, got TrafficResult
				require.NoError(t, old.Transaction(func(tx *gorm.DB) (err error) {
					want, err = recordTrafficPerUser(tx, entries, now)
					return err
				}))
				require.NoError(t, batched.Transaction(func(tx *gorm.DB) (err error) {
					got, err = RecordTrafficTx(tx, "", entries, now)
					return err
				}))
				require.Equal(t, want.Applied, got.Applied)
				require.Equal(t, want.Exhausted, got.Exhausted, "report %d", report)
				require.Equal(t, counters(t, old), counters(t, batched), "report %d", report)
				require.Equal(t, changeLog(t, old), changeLog(t, batched), "report %d", report)
			}
		})
	}
}

func TestRecordTrafficBatchesStatements(t *testing.T) {
	db := openDB(t)
	const users = 3*trafficUpdateBatch + 1
	for id := 1; id <= users; id++ {
		require.NoError(t, db.Create(&model.User{ID: uint(id), Email: fmt.Sprintf("u%d@example.test", id), Token: fmt.Sprintf("t%d", id), UUID: fmt.Sprintf("id%d", id)}).Error)
	}
	entries := make([]TrafficEntry, 0, users)
	for id := 1; id <= users; id++ {
		entries = append(entries, TrafficEntry{UserID: uint(id), Upload: int64(id), Download: 2 * int64(id)})
	}
	var updates int
	require.NoError(t, db.Callback().Update().After("gorm:update").Register("test:count_updates", func(d *gorm.DB) { updates++ }))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := RecordTrafficTx(tx, "", entries, time.Now())
		return err
	}))
	require.Equal(t, 4, updates, "%d subscribers are 4 statements, not %d", users, users)
	for _, row := range counters(t, db) {
		require.Equal(t, int64(row.ID), row.U)
		require.Equal(t, 2*int64(row.ID), row.D)
	}
}
