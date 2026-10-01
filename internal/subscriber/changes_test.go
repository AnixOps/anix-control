package subscriber

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func changedUsers(t *testing.T, db *gorm.DB, after uint64) []uint {
	t.Helper()
	changes, resync, err := ChangesAfter(db, after, 100)
	require.NoError(t, err)
	require.False(t, resync)
	ids := make([]uint, 0, len(changes))
	for _, change := range changes {
		ids = append(ids, change.UserID)
	}
	return ids
}

func TestSubscriberWritesFeedTheChangeLog(t *testing.T) {
	db := openDB(t)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "a@example.test", Token: "t1", UUID: "u1", TransferEnable: 10},
		{ID: 2, Email: "b@example.test", Token: "t2", UUID: "u2", TransferEnable: 1000},
	}).Error)
	now := time.Now()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if _, err := ApplyEntitlementTx(tx, Entitlement{UserID: 2, Plan: PlanSnapshot{PlanID: 1, TransferBytes: 1000}}, now); err != nil {
			return err
		}
		if _, err := RecordTrafficTx(tx, "", []TrafficEntry{{UserID: 1, Upload: 20}, {UserID: 2, Upload: 1}}, now); err != nil {
			return err
		}
		_, err := ResetTrafficTx(tx, "", []uint{1}, now)
		return err
	}))
	require.Equal(t, []uint{2, 1, 1}, changedUsers(t, db, 0),
		"the entitlement, the exhaustion (not the traffic that stays within limits) and the reset")

	cursor, err := Cursor(db)
	require.NoError(t, err)
	require.Empty(t, changedUsers(t, db, cursor))
}

func TestChangeLogResyncAndPrune(t *testing.T) {
	db := openDB(t)
	old := time.Now().Add(-ChangeRetention - time.Hour)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := RecordChangesTx(tx, []uint{1, 2}, false, old); err != nil {
			return err
		}
		return RecordChangesTx(tx, []uint{3}, true, time.Now())
	}))
	pruned, err := PruneChanges(db, time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(2), pruned)

	_, resync, err := ChangesAfter(db, 1, 10)
	require.NoError(t, err)
	require.True(t, resync, "a cursor before the oldest kept row must list again")
	changes, resync, err := ChangesAfter(db, 2, 10)
	require.NoError(t, err)
	require.False(t, resync)
	require.Len(t, changes, 1)
	require.True(t, changes[0].Deleted)
}

func TestRequestLedgerPrune(t *testing.T) {
	db := openDB(t)
	now := time.Now()
	require.NoError(t, Record(db, "old", "apply_entitlement", 1, nil, now.Add(-RequestRetention-time.Hour)))
	require.NoError(t, Record(db, "kept", "apply_entitlement", 1, nil, now.Add(-RequestRetention+time.Hour)))

	pruned, err := PruneRequests(db, now)
	require.NoError(t, err)
	require.Equal(t, int64(1), pruned)
	var ids []string
	require.NoError(t, db.Model(&model.SubscriberRequest{}).Pluck("request_id", &ids).Error)
	require.Equal(t, []string{"kept"}, ids)
}

func TestActiveMatchesNodeUserLists(t *testing.T) {
	db := openDB(t)
	now := time.Now()
	past, future := now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix()
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "active@x", Token: "t1", UUID: "u1", TransferEnable: 10, ExpiredAt: &future},
		{ID: 2, Email: "banned@x", Token: "t2", UUID: "u2", TransferEnable: 10, Banned: 1},
		{ID: 3, Email: "expired@x", Token: "t3", UUID: "u3", TransferEnable: 10, ExpiredAt: &past},
		{ID: 4, Email: "exhausted@x", Token: "t4", UUID: "u4", TransferEnable: 10, U: 6, D: 4},
		{ID: 5, Email: "no-traffic@x", Token: "t5", UUID: "u5"},
		{ID: 6, Email: "no-expiry@x", Token: "t6", UUID: "u6", TransferEnable: 10},
	}).Error)
	var ids []uint
	require.NoError(t, Active(db.Model(&model.User{}), now).Order("id").Pluck("id", &ids).Error)
	require.Equal(t, []uint{1, 6}, ids)
}
