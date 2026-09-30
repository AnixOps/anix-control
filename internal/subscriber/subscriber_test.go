package subscriber

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSubscriptionGroup{}, &model.SubscriberRequest{}))
	return db
}

func apply(t *testing.T, db *gorm.DB, e Entitlement, now time.Time) EntitlementResult {
	t.Helper()
	var result EntitlementResult
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = ApplyEntitlementTx(tx, e, now)
		return err
	}))
	return result
}

func subscriberRow(t *testing.T, db *gorm.DB, id uint) (model.User, []uint) {
	t.Helper()
	var user model.User
	require.NoError(t, db.Take(&user, id).Error)
	var groups []uint
	require.NoError(t, db.Model(&model.UserSubscriptionGroup{}).Where("user_id = ?", id).Order("group_id").Pluck("group_id", &groups).Error)
	return user, groups
}

func TestApplyEntitlementGrantsRenewsAndIsIdempotent(t *testing.T) {
	db := openDB(t)
	require.NoError(t, db.Create(&model.User{ID: 1, Email: "a@example.test", Token: "t1", UUID: "u1", U: 5, D: 7}).Error)
	require.NoError(t, db.Create(&model.UserSubscriptionGroup{UserID: 1, GroupID: 9}).Error)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	speed := int64(100)
	plan := PlanSnapshot{PlanID: 3, GroupID: 4, SubscriptionGroupIDs: []uint{4, 5}, TransferBytes: 10 * bytesPerGiB, SpeedLimit: &speed}
	month := &Period{Months: 1}

	first := apply(t, db, Entitlement{RequestID: "order:1", UserID: 1, Plan: plan, Period: month, RenewSamePlan: true, ResetTraffic: true}, now)
	require.True(t, first.Applied)
	require.Equal(t, now.AddDate(0, 1, 0).Unix(), *first.ExpiresAt)
	user, groups := subscriberRow(t, db, 1)
	require.Equal(t, uint(3), *user.PlanID)
	require.Equal(t, uint(4), *user.GroupID)
	require.Equal(t, int64(10*bytesPerGiB), user.TransferEnable)
	require.Equal(t, int64(100), *user.SpeedLimit)
	require.Zero(t, user.U+user.D)
	require.Equal(t, []uint{4, 5}, groups, "the plan's subscription groups replace the old ones")

	// A retried request changes nothing and answers with the first result.
	require.NoError(t, db.Model(&model.User{}).Where("id = 1").Update("u", 99).Error)
	again := apply(t, db, Entitlement{RequestID: "order:1", UserID: 1, Plan: plan, Period: month, RenewSamePlan: true, ResetTraffic: true}, now.Add(time.Hour))
	require.False(t, again.Applied)
	require.Equal(t, *first.ExpiresAt, *again.ExpiresAt)
	user, _ = subscriberRow(t, db, 1)
	require.Equal(t, int64(99), user.U)

	// Renewing the same, unexpired plan extends from the current expiry.
	renewed := apply(t, db, Entitlement{RequestID: "order:2", UserID: 1, Plan: plan, Period: month, RenewSamePlan: true}, now.AddDate(0, 0, 10))
	require.Equal(t, now.AddDate(0, 2, 0).Unix(), *renewed.ExpiresAt)
	// Another plan starts from now.
	other := plan
	other.PlanID = 8
	switched := apply(t, db, Entitlement{RequestID: "order:3", UserID: 1, Plan: other, Period: month, RenewSamePlan: true}, now.AddDate(0, 0, 10))
	require.Equal(t, now.AddDate(0, 0, 10).AddDate(0, 1, 0).Unix(), *switched.ExpiresAt)
}

func TestApplyEntitlementAdministratorAssignment(t *testing.T) {
	db := openDB(t)
	expires := int64(1900000000)
	require.NoError(t, db.Create(&model.User{ID: 2, Email: "b@example.test", Token: "t2", UUID: "u2", ExpiredAt: &expires, U: 3}).Error)
	require.NoError(t, db.Create(&model.UserSubscriptionGroup{UserID: 2, GroupID: 9}).Error)
	plan := PlanSnapshot{PlanID: 3, GroupID: 4, SubscriptionGroupIDs: []uint{4}, TransferBytes: bytesPerGiB}

	result := apply(t, db, Entitlement{UserID: 2, Plan: plan, ResetTraffic: true, KeepSubscriptionGroups: true}, time.Now())
	require.True(t, result.Applied)
	user, groups := subscriberRow(t, db, 2)
	require.Equal(t, expires, *user.ExpiredAt, "no period and no expiry keep the expiry")
	require.Equal(t, []uint{9}, groups, "kept subscription groups")
	require.Zero(t, user.U)

	exact := int64(2000000000)
	apply(t, db, Entitlement{UserID: 2, Plan: plan, ExpiresAt: &exact, KeepSubscriptionGroups: true}, time.Now())
	user, _ = subscriberRow(t, db, 2)
	require.Equal(t, exact, *user.ExpiredAt)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := ApplyEntitlementTx(tx, Entitlement{UserID: 99, Plan: plan}, time.Now())
		require.ErrorIs(t, err, ErrSubscriberNotFound)
		_, err = ApplyEntitlementTx(tx, Entitlement{UserID: 2, Plan: plan, Period: &Period{Months: 1}, ExpiresAt: &exact}, time.Now())
		require.Error(t, err, "a period and an expiry together are refused")
		return nil
	}))
}
