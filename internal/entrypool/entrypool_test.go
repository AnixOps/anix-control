package entrypool

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var now = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

func newService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pools.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.NodeEntry{}, &model.EntryPoolPolicy{}, &model.UserEntryPool{}, &model.OperationLog{}))
	plan := uint(1)
	old := now.AddDate(0, -6, 0)
	for id := uint(1); id <= 6; id++ {
		user := model.User{ID: id, Email: "u" + string(rune('0'+id)) + "@example.com", UUID: "uuid-" + string(rune('0'+id)),
			Token: "token-" + string(rune('0'+id)), PlanID: &plan, CreatedAt: old}
		if id == 6 {
			user.CreatedAt = now.AddDate(0, 0, -1) // new account
		}
		require.NoError(t, db.Create(&user).Error)
	}
	require.NoError(t, db.Create(&[]model.NodeEntry{
		{ID: 1, NodeID: 10, Label: "shared", Kind: model.NodeEntryDirect, Host: "shared.example.com", Enabled: true},
		{ID: 2, NodeID: 10, Label: "s0", Kind: model.NodeEntryDirect, Host: "s0.example.com", Enabled: true, Pool: "shard-0"},
		{ID: 3, NodeID: 11, Label: "q", Kind: model.NodeEntryDirect, Host: "q.example.com", Enabled: true, Pool: "quarantine"},
	}).Error)
	return &Service{DB: db, Now: func() time.Time { return now }}
}

func TestFilter(t *testing.T) {
	entries := map[uint][]model.NodeEntry{
		10: {{ID: 1}, {ID: 2, Pool: "shard-0"}},
		11: {{ID: 3, Pool: "quarantine"}},
		12: nil,
	}
	ids := func(list []model.NodeEntry) (out []uint) {
		for _, e := range list {
			out = append(out, e.ID)
		}
		return out
	}
	got, hidden := Filter(entries, "shard-0")
	require.Equal(t, []uint{2}, ids(got[10]))
	require.True(t, hidden[11])
	require.False(t, hidden[12])

	got, hidden = Filter(entries, "shard-1")
	require.Equal(t, []uint{1}, ids(got[10]))
	require.True(t, hidden[11])

	got, hidden = Filter(entries, "quarantine")
	require.Equal(t, []uint{1}, ids(got[10]))
	require.Equal(t, []uint{3}, ids(got[11]))
	require.Empty(t, hidden)

	got, hidden = Filter(entries, "")
	require.Equal(t, []uint{1}, ids(got[10]))
	require.True(t, hidden[11])
}

func TestPolicyAssignLeakSplit(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	actor := Actor{UserID: 1, Name: "admin"}

	pool, reason, err := UserPool(s.DB, 3, now)
	require.NoError(t, err)
	require.Equal(t, "", pool)
	require.Equal(t, "none", reason)

	_, err = s.SetPolicy(ctx, Policy{Shards: 1}, actor)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = s.SetPolicy(ctx, Policy{QuarantinePool: "shard-9"}, actor)
	require.ErrorIs(t, err, ErrInvalid)
	policy, err := s.SetPolicy(ctx, Policy{Shards: 2, QuarantineDays: 7}, actor)
	require.NoError(t, err)
	require.Equal(t, DefaultQuarantine, policy.QuarantinePool)

	for id, want := range map[uint]string{2: "shard-0", 3: "shard-1", 6: "quarantine"} {
		pool, _, err := UserPool(s.DB, id, now)
		require.NoError(t, err)
		require.Equal(t, want, pool, "user %d", id)
	}

	n, err := s.Assign(ctx, AssignInput{UserIDs: []uint{3, 3, 0}, Pool: "canary-3", Reason: "canary"}, actor)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	pool, reason, err = UserPool(s.DB, 3, now)
	require.NoError(t, err)
	require.Equal(t, "canary-3", pool)
	require.Equal(t, "assigned", reason)
	_, err = s.Assign(ctx, AssignInput{UserIDs: []uint{99}, Pool: "x"}, actor)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.Assign(ctx, AssignInput{UserIDs: []uint{1}, Pool: "Bad Pool"}, actor)
	require.ErrorIs(t, err, ErrInvalid)

	matches, err := s.Leak(ctx, LeakInput{Host: "S0.example.com"})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	require.Equal(t, "shard-0", matches[0].Pool)
	require.Equal(t, []uint{2, 4}, matches[0].UserIDs) // 6 is quarantined
	_, err = s.Leak(ctx, LeakInput{Host: "nowhere.example.com"})
	require.ErrorIs(t, err, ErrNotFound)

	split, err := s.Split(ctx, SplitInput{Pool: "shard-0", Into: [2]string{"leak-a", "leak-b"}}, actor)
	require.NoError(t, err)
	require.Equal(t, map[string]int{"leak-a": 1, "leak-b": 1}, split.Into)
	pool, _, err = UserPool(s.DB, 4, now)
	require.NoError(t, err)
	require.Equal(t, "leak-b", pool)

	overview, err := s.Overview(ctx)
	require.NoError(t, err)
	byPool := map[string]PoolView{}
	for _, view := range overview.Pools {
		byPool[view.Pool] = view
	}
	require.Equal(t, 1, byPool["quarantine"].Users)
	require.Len(t, byPool["quarantine"].Entries, 1)
	require.Equal(t, 1, byPool["canary-3"].Users)

	// Back to the policy.
	_, err = s.Assign(ctx, AssignInput{UserIDs: []uint{3}}, actor)
	require.NoError(t, err)
	pool, _, err = UserPool(s.DB, 3, now)
	require.NoError(t, err)
	require.Equal(t, "shard-1", pool)

	var logs int64
	require.NoError(t, s.DB.Model(&model.OperationLog{}).Where("module = ?", auditModule).Count(&logs).Error)
	require.Equal(t, int64(4), logs)
}
