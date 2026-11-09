package service

import (
	"sort"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A subscriber sees the entries of their pool (internal/entrypool) and the
// shared ones only where the node has none of theirs; a node whose entries
// are all in other pools is left out, never offered at its bare host.
func TestSubscriptionFollowsEntryPools(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Node{}, &model.NodeProtocol{}, &model.SubscriptionGroup{}, &model.NodeEntry{},
		&model.SubscriptionGroupProtocol{}, &model.ForwardNode{}, &model.KernelForwardRoute{}, &model.ForwardRouteSubscription{},
		&model.OperationLog{}, &model.EntryPoolPolicy{}, &model.UserEntryPool{}))
	svc := &SubscriptionService{db: db}

	plan := uint(1)
	old := time.Now().AddDate(-1, 0, 0)
	for _, id := range []uint{2, 3, 4} {
		require.NoError(t, db.Create(&model.User{ID: id, Email: string(rune('a'+id)) + "@example.com", UUID: "uuid-" + string(rune('a'+id)),
			Token: "token-" + string(rune('a'+id)), PlanID: &plan, CreatedAt: old}).Error)
	}
	require.NoError(t, db.Create(&model.EntryPoolPolicy{ID: 1, Shards: 2, QuarantinePool: "quarantine", UpdatedAt: old}).Error)
	require.NoError(t, db.Create(&model.UserEntryPool{UserID: 4, Pool: "canary-4", UpdatedAt: old}).Error)

	now := time.Now().Unix()
	group := &model.SubscriptionGroup{Name: "ok", Enable: 1}
	require.NoError(t, db.Create(group).Error)
	for _, n := range []struct {
		id   uint
		name string
	}{{1, "hk"}, {2, "jp"}} {
		require.NoError(t, db.Create(&model.Node{ID: n.id, Name: n.name, Host: n.name + ".example.com", Show: 1, Status: model.NodeStatusOnline, LastCheckAt: &now}).Error)
		protocol := model.NodeProtocol{NodeID: n.id, Name: "ss", Type: model.ProtocolShadowsocks, Port: 11111, Enable: 1, Show: 1}
		require.NoError(t, db.Create(&protocol).Error)
		require.NoError(t, db.Exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (?, ?)", group.ID, protocol.ID).Error)
	}
	require.NoError(t, db.Create(&[]model.NodeEntry{
		{NodeID: 1, Label: "shared", Kind: model.NodeEntryDirect, Host: "shared.example.com", Enabled: true},
		{NodeID: 1, Label: "s0", Kind: model.NodeEntryDirect, Host: "s0.example.com", Enabled: true, Pool: "shard-0"},
		{NodeID: 1, Label: "canary", Kind: model.NodeEntryDirect, Host: "canary4.example.com", Enabled: true, Pool: "canary-4"},
		{NodeID: 2, Label: "s1", Kind: model.NodeEntryDirect, Host: "s1.example.com", Enabled: true, Pool: "shard-1"},
	}).Error)

	servers := func(userID uint) []string {
		parsed, err := svc.getInternalNodes(nil, &model.TemplateRenderContext{UUID: "u", UserID: userID}, []*model.SubscriptionGroup{group})
		require.NoError(t, err)
		var out []string
		for _, node := range parsed {
			out = append(out, node.Server)
		}
		sort.Strings(out)
		return out
	}
	require.Equal(t, []string{"s0.example.com"}, servers(2), "shard-0: its own entry on hk, nothing on jp")
	require.Equal(t, []string{"s1.example.com", "shared.example.com"}, servers(3), "shard-1")
	require.Equal(t, []string{"canary4.example.com"}, servers(4), "the canary sees only its address")
	require.Len(t, servers(0), 4, "no subscriber (a preview): every entry")
}
