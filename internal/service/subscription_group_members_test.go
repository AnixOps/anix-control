package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var groupMemberModels = []any{&model.User{}, &model.Plan{}, &model.SubscriptionGroup{}, &model.UserSubscriptionGroup{}}

// forEachMemberDatabase runs body on SQLite and, with POSTGRES_TEST_DSN, on
// PostgreSQL, each with the group tables empty.
func forEachMemberDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		require.NoError(t, db.AutoMigrate(groupMemberModels...))
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		body(t, openPostgresTestDB(t, groupMemberModels...))
	})
}

func memberIDs(members []GroupMember) []uint {
	ids := make([]uint, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserID)
	}
	return ids
}

// seedGroupMembers makes group 1 with five direct members granted at
// different times (two in the same instant), one of them expired, and a
// second group with one member of its own. User 9 holds the group only
// through their plan and primary group, which does not make them a member.
func seedGroupMembers(t *testing.T, db *gorm.DB) (granted time.Time) {
	t.Helper()
	granted = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	plan := model.Plan{ID: 5, Name: "Pro"}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&[]model.SubscriptionGroup{{ID: 1, Name: "premium"}, {ID: 2, Name: "lite"}, {ID: 3, Name: "empty"}}).Error)
	planID, groupID := uint(5), uint(1)
	users := []model.User{
		{ID: 1, Email: "ann@example.test", Token: "secret-token-1", UUID: "secret-uuid-1", PlanID: &planID},
		{ID: 2, Email: "bob_x@example.test", Token: "secret-token-2", UUID: "secret-uuid-2", Banned: 1},
		{ID: 3, Email: "cy%off@example.test", Token: "secret-token-3", UUID: "secret-uuid-3"},
		{ID: 4, Email: "dee@example.test", Token: "secret-token-4", UUID: "secret-uuid-4"},
		{ID: 5, Email: "eve@example.test", Token: "secret-token-5", UUID: "secret-uuid-5"},
		{ID: 9, Email: "via-plan@example.test", Token: "secret-token-9", UUID: "secret-uuid-9", PlanID: &planID, GroupID: &groupID},
	}
	require.NoError(t, db.Create(&users).Error)
	past, future := time.Now().Add(-time.Hour).Unix(), time.Now().Add(24*time.Hour).Unix()
	quota, price := int64(1<<30), int64(1500)
	memberships := []model.UserSubscriptionGroup{
		{UserID: 1, GroupID: 1, CreatedAt: granted},
		{UserID: 2, GroupID: 1, CreatedAt: granted.Add(time.Hour), ExpireAt: &future, TransferEnable: &quota, NextRenewPrice: &price},
		{UserID: 3, GroupID: 1, CreatedAt: granted.Add(2 * time.Hour), ExpireAt: &past},
		{UserID: 4, GroupID: 1, CreatedAt: granted.Add(2 * time.Hour)},
		{UserID: 5, GroupID: 1, CreatedAt: granted.Add(3 * time.Hour), ExpireAt: &past},
		{UserID: 1, GroupID: 2, CreatedAt: granted},
	}
	require.NoError(t, db.Create(&memberships).Error)
	return granted
}

// The members are the users granted the group directly, newest grant first
// (the same instant by id, newest first), with the grant's fields and the
// user's e-mail, ban flag and plan, and never a credential.
func TestListGroupMembers(t *testing.T) {
	forEachMemberDatabase(t, func(t *testing.T, db *gorm.DB) {
		granted := seedGroupMembers(t, db)
		svc := &SubscriptionService{db: db}

		all, err := svc.ListGroupMembers(1, GroupMembersParams{Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.EqualValues(t, 5, all.Total)
		require.Equal(t, []uint{5, 4, 3, 2, 1}, memberIDs(all.Members))
		require.Equal(t, 1, all.Page)
		require.Equal(t, 20, all.PageSize)

		bob := all.Members[3]
		require.Equal(t, "bob_x@example.test", bob.Email)
		require.Equal(t, 1, bob.Banned)
		require.Nil(t, bob.PlanID)
		require.NotNil(t, bob.ExpireAt)
		require.True(t, bob.Active)
		require.Equal(t, int64(1<<30), *bob.TransferEnable)
		require.Equal(t, int64(1500), *bob.NextRenewPrice)
		require.True(t, bob.CreatedAt.Equal(granted.Add(time.Hour)))
		ann := all.Members[4]
		require.Equal(t, uint(5), *ann.PlanID)
		require.Nil(t, ann.ExpireAt)
		require.True(t, ann.Active)
		require.False(t, all.Members[0].Active)

		// Pages continue the order without repeating or skipping a member.
		first, err := svc.ListGroupMembers(1, GroupMembersParams{Page: 1, PageSize: 2})
		require.NoError(t, err)
		second, err := svc.ListGroupMembers(1, GroupMembersParams{Page: 2, PageSize: 2})
		require.NoError(t, err)
		third, err := svc.ListGroupMembers(1, GroupMembersParams{Page: 3, PageSize: 2})
		require.NoError(t, err)
		beyond, err := svc.ListGroupMembers(1, GroupMembersParams{Page: 4, PageSize: 2})
		require.NoError(t, err)
		require.Equal(t, []uint{5, 4}, memberIDs(first.Members))
		require.Equal(t, []uint{3, 2}, memberIDs(second.Members))
		require.Equal(t, []uint{1}, memberIDs(third.Members))
		require.Empty(t, beyond.Members)
		require.EqualValues(t, 5, beyond.Total)
		require.NotNil(t, beyond.Members, "an empty page is [], not null")
	})
}

func TestListGroupMembersFilters(t *testing.T) {
	forEachMemberDatabase(t, func(t *testing.T, db *gorm.DB) {
		seedGroupMembers(t, db)
		svc := &SubscriptionService{db: db}
		list := func(groupID uint, params GroupMembersParams) *GroupMembersResult {
			params.Page, params.PageSize = 1, 20
			result, err := svc.ListGroupMembers(groupID, params)
			require.NoError(t, err)
			return result
		}

		active := list(1, GroupMembersParams{Status: "active"})
		require.EqualValues(t, 3, active.Total)
		require.Equal(t, []uint{4, 2, 1}, memberIDs(active.Members))
		expired := list(1, GroupMembersParams{Status: "expired"})
		require.EqualValues(t, 2, expired.Total)
		require.Equal(t, []uint{5, 3}, memberIDs(expired.Members))

		// The e-mail filter is a substring in which % and _ match themselves.
		require.Equal(t, []uint{1}, memberIDs(list(1, GroupMembersParams{Email: "ann"}).Members))
		require.Equal(t, []uint{2}, memberIDs(list(1, GroupMembersParams{Email: "_x"}).Members))
		require.Equal(t, []uint{3}, memberIDs(list(1, GroupMembersParams{Email: "%"}).Members))
		require.Equal(t, []uint{3}, memberIDs(list(1, GroupMembersParams{Email: "cy%off"}).Members))
		require.Empty(t, list(1, GroupMembersParams{Email: "dee", Status: "expired"}).Members)
		both := list(1, GroupMembersParams{Email: "dee", Status: "active"})
		require.EqualValues(t, 1, both.Total)
		require.Equal(t, []uint{4}, memberIDs(both.Members))
		require.Equal(t, []uint{4, 2, 1}, memberIDs(list(1, GroupMembersParams{Email: "e", Status: "active"}).Members))

		// Another group has its own members; one without members lists none.
		require.Equal(t, []uint{1}, memberIDs(list(2, GroupMembersParams{}).Members))
		none := list(3, GroupMembersParams{})
		require.Zero(t, none.Total)
		require.NotNil(t, none.Members)
		require.Empty(t, none.Members)
	})
}

func TestListGroupMembersRefusals(t *testing.T) {
	forEachMemberDatabase(t, func(t *testing.T, db *gorm.DB) {
		seedGroupMembers(t, db)
		svc := &SubscriptionService{db: db}
		_, err := svc.ListGroupMembers(99, GroupMembersParams{Page: 1, PageSize: 20})
		require.ErrorIs(t, err, ErrSubscriptionGroupNotFound)
		_, err = svc.ListGroupMembers(1, GroupMembersParams{Page: 1, PageSize: 20, Status: "enabled"})
		require.ErrorIs(t, err, ErrInvalidMemberStatus)
	})
}

// A member carries no credential: the answer has neither the subscription
// token nor the proxy uuid.
func TestListGroupMembersShowNoCredentials(t *testing.T) {
	forEachMemberDatabase(t, func(t *testing.T, db *gorm.DB) {
		seedGroupMembers(t, db)
		result, err := (&SubscriptionService{db: db}).ListGroupMembers(1, GroupMembersParams{Page: 1, PageSize: 20})
		require.NoError(t, err)
		encoded, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "secret-")
		var decoded struct {
			Members []map[string]any `json:"members"`
		}
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		for _, member := range decoded.Members {
			keys := make([]string, 0, len(member))
			for key := range member {
				keys = append(keys, key)
			}
			require.ElementsMatch(t, []string{"user_id", "email", "banned", "plan_id", "expire_at", "transfer_enable", "next_renew_price", "created_at", "active"}, keys)
		}
	})
}
