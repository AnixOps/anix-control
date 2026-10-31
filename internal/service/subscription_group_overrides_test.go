package service

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A group edits the nodes of a protocol without touching the protocol, the
// edits survive a layout change, and clearing them resets the node.
func TestGroupNodeOverridesEditAndReset(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.SubscriptionGroup{}, &model.NodeEntry{}, &model.SubscriptionGroupProtocol{}))
	svc := &SubscriptionService{db: db}

	now := time.Now().Unix()
	group := &model.SubscriptionGroup{Name: "g", Enable: 1}
	require.NoError(t, db.Create(group).Error)
	node := &model.Node{Name: "HK", APIKey: "k", Host: "hk.example.com", Show: 1, Status: model.NodeStatusOnline, LastCheckAt: &now}
	require.NoError(t, db.Create(node).Error)
	protocol := model.NodeProtocol{NodeID: node.ID, Name: "ss", Type: model.ProtocolShadowsocks, Port: 10000, Enable: 1, Show: 1}
	require.NoError(t, db.Create(&protocol).Error)
	require.NoError(t, db.Exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (?, ?)", group.ID, protocol.ID).Error)

	first := func() *model.ParsedNode {
		nodes, err := svc.getInternalNodes(nil, &model.TemplateRenderContext{UUID: "u"}, []*model.SubscriptionGroup{group})
		require.NoError(t, err)
		require.Len(t, nodes, 1)
		return nodes[0]
	}
	require.Equal(t, 10000, first().Port)

	require.NoError(t, svc.ReplaceGroupProtocolOverrides(group.ID, protocol.ID, map[string]map[string]any{
		"": {"name": "HK 家宽", "server": "1.2.3.4", "port": float64(443)},
	}))
	got := first()
	require.Equal(t, "HK 家宽", got.Name)
	require.Equal(t, "1.2.3.4", got.Server)
	require.Equal(t, 443, got.Port)
	require.Equal(t, string(model.ProtocolShadowsocks), got.Type)

	listed, err := svc.GroupProtocolNodes(group.ID, protocol.ID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, 10000, int(listed[0].Defaults["port"].(float64)), "defaults are the source's values")

	_, err = svc.ReplaceGroupProtocolLayout(group.ID, []GroupProtocolLayoutItem{{ProtocolID: protocol.ID}})
	require.NoError(t, err)
	require.Equal(t, 443, first().Port, "a layout change keeps the edits")

	for _, bad := range []map[string]any{{"type": "vless"}, {"uuid": "x"}, {"port": float64(0)}, {"name": " "}} {
		require.ErrorIs(t, svc.ReplaceGroupProtocolOverrides(group.ID, protocol.ID, map[string]map[string]any{"": bad}), ErrInvalidNodeOverride)
	}

	require.NoError(t, svc.ResetGroupOverrides(group.ID))
	got = first()
	require.Equal(t, 10000, got.Port)
	require.Equal(t, "hk.example.com", got.Server)
}
