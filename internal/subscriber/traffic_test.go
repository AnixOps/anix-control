package subscriber

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
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
