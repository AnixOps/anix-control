package service

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// forEachInviteCodeDatabase runs body on SQLite and, when
// ANIX_TEST_POSTGRES_DSN is set, on a throwaway PostgreSQL schema.
func forEachInviteCodeDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	seed := func(t *testing.T, db *gorm.DB) {
		require.NoError(t, db.AutoMigrate(&model.User{}, &model.InviteCode{}, &model.InviteConfig{}))
		require.NoError(t, db.Create(&model.User{ID: 7, Email: "owner@x", Token: "t7", UUID: "u7"}).Error)
	}
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		seed(t, db)
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		db := openReconcilerPostgres(t)
		seed(t, db)
		body(t, db)
	})
}

func TestAdminInviteCodesGenerateListAndRevoke(t *testing.T) {
	forEachInviteCodeDatabase(t, func(t *testing.T, db *gorm.DB) {
		now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		svc := NewInviteService(db)
		require.NoError(t, db.Create(&model.InviteConfig{Enabled: true, CodeCount: 1, CodeExpireDays: 30}).Error)

		for _, count := range []int{0, -1, MaxAdminInviteCodeBatch + 1} {
			_, err := svc.GenerateAdminInviteCodes(count, nil, now)
			require.Error(t, err, count)
		}
		tooLong := MaxAdminInviteCodeExpireDays + 1
		_, err := svc.GenerateAdminInviteCodes(1, &tooLong, now)
		require.Error(t, err)

		// The configured expiry by default; 0 never expires; a value
		// overrides it.
		configured, err := svc.GenerateAdminInviteCodes(3, nil, now)
		require.NoError(t, err)
		require.Len(t, configured, 3)
		for _, code := range configured {
			require.Len(t, code.Code, 8)
			require.Nil(t, code.UserID, "an administrator's code belongs to no user")
			require.Equal(t, 0, code.Status)
			require.NotNil(t, code.ExpiredAt)
			require.WithinDuration(t, now.AddDate(0, 0, 30), *code.ExpiredAt, time.Second)
		}
		never := 0
		forever, err := svc.GenerateAdminInviteCodes(1, &never, now)
		require.NoError(t, err)
		require.Nil(t, forever[0].ExpiredAt)
		one := 1
		short, err := svc.GenerateAdminInviteCodes(1, &one, now)
		require.NoError(t, err)

		// The per-user limit (code_count 1) is unchanged and does not
		// count the administrator's codes.
		owned, err := svc.GenerateUserInviteCode(7)
		require.NoError(t, err)
		_, err = svc.GenerateUserInviteCode(7)
		require.ErrorIs(t, err, ErrInviteCodeLimit)

		// Mark one administrator code used.
		usedAt := now
		usedBy := uint(7)
		require.NoError(t, db.Model(&model.InviteCode{}).Where("id = ?", configured[0].ID).
			Updates(map[string]any{"status": 1, "used_by": usedBy, "used_at": usedAt}).Error)

		list := func(status string, at time.Time) ([]model.InviteCode, int64) {
			codes, total, err := svc.ListInviteCodes(InviteCodeListQuery{Status: status, Page: 1, PageSize: 100}, at)
			require.NoError(t, err)
			return codes, total
		}
		all, total := list("", now)
		require.EqualValues(t, 6, total)
		require.Len(t, all, 6)
		_, total = list("used", now)
		require.EqualValues(t, 1, total)
		_, total = list("unused", now)
		require.EqualValues(t, 5, total)
		_, total = list("expired", now)
		require.EqualValues(t, 0, total)
		// Two days later the one-day code has expired.
		later := now.AddDate(0, 0, 2)
		expired, total := list("EXPIRED", later)
		require.EqualValues(t, 1, total)
		require.Equal(t, short[0].Code, expired[0].Code)
		_, total = list("unused", later)
		require.EqualValues(t, 4, total)
		_, _, err = svc.ListInviteCodes(InviteCodeListQuery{Status: "bogus", Page: 1, PageSize: 10}, now)
		require.ErrorIs(t, err, ErrInviteCodeFilter)

		// Paging keeps the newest first and the total of the filter.
		page, total, err := svc.ListInviteCodes(InviteCodeListQuery{Page: 2, PageSize: 5}, now)
		require.NoError(t, err)
		require.EqualValues(t, 6, total)
		require.Len(t, page, 1)

		// Revoking: a used code is kept, an unused one (a user's too) goes.
		require.ErrorIs(t, svc.RevokeInviteCode(configured[0].ID), ErrInviteCodeUsed)
		require.ErrorIs(t, svc.RevokeInviteCode(999999), ErrInviteCodeNotFound)
		require.NoError(t, svc.RevokeInviteCode(configured[1].ID))
		require.ErrorIs(t, svc.RevokeInviteCode(configured[1].ID), ErrInviteCodeNotFound)
		require.NoError(t, svc.RevokeInviteCode(owned.ID))
		_, total = list("", now)
		require.EqualValues(t, 4, total)
		// The user may generate again once the revoked code is gone.
		_, err = svc.GenerateUserInviteCode(7)
		require.NoError(t, err)
	})
}
