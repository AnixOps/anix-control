package service

import (
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A group's display names replace "<node> - <protocol>" in the
// subscription and its order is the subscription's order; protocols the
// layout leaves out follow by id under their default names.
func TestSubscriptionFollowsTheGroupProtocolLayout(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.SubscriptionGroup{}, &model.NodeEntry{}, &model.SubscriptionGroupProtocol{}))
	svc := &SubscriptionService{db: db}

	now := time.Now().Unix()
	group := &model.SubscriptionGroup{Name: "ok", Enable: 1}
	other := &model.SubscriptionGroup{Name: "other", Enable: 1}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(other).Error)
	var protocols []model.NodeProtocol
	for _, name := range []string{"HK", "US", "JP"} {
		node := &model.Node{Name: "BGP " + name, APIKey: "key-" + name, Host: strings.ToLower(name) + ".example.com", Show: 1, Status: model.NodeStatusOnline, LastCheckAt: &now}
		require.NoError(t, db.Create(node).Error)
		protocol := model.NodeProtocol{NodeID: node.ID, Name: "ss", Type: model.ProtocolShadowsocks, Port: 10000, Enable: 1, Show: 1}
		require.NoError(t, db.Create(&protocol).Error)
		require.NoError(t, db.Exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (?, ?)", group.ID, protocol.ID).Error)
		protocols = append(protocols, protocol)
	}
	hk, us, jp := protocols[0].ID, protocols[1].ID, protocols[2].ID

	names := func() []string {
		nodes, err := svc.getInternalNodes(nil, &model.TemplateRenderContext{UUID: "u"}, []*model.SubscriptionGroup{group})
		require.NoError(t, err)
		var out []string
		for _, node := range nodes {
			out = append(out, node.Name)
		}
		return out
	}
	require.Equal(t, []string{"BGP HK - ss", "BGP US - ss", "BGP JP - ss"}, names())

	layout, err := svc.ReplaceGroupProtocolLayout(group.ID, []GroupProtocolLayoutItem{{ProtocolID: jp, DisplayName: "  日本 01 "}, {ProtocolID: hk}})
	require.NoError(t, err)
	require.Equal(t, []GroupProtocolLayoutItem{{ProtocolID: jp, DisplayName: "日本 01"}, {ProtocolID: hk}, {ProtocolID: us}}, layout)
	require.Equal(t, []string{"日本 01", "BGP HK - ss", "BGP US - ss"}, names())

	listed, err := svc.GroupProtocolLayout(group.ID)
	require.NoError(t, err)
	require.Equal(t, layout, listed)

	for _, items := range [][]GroupProtocolLayoutItem{
		{{ProtocolID: hk}, {ProtocolID: hk}},
		{{ProtocolID: 999}},
		{{ProtocolID: hk, DisplayName: "a\nb"}},
		{{ProtocolID: hk, DisplayName: strings.Repeat("名", MaxGroupProtocolDisplayName+1)}},
	} {
		_, err := svc.ReplaceGroupProtocolLayout(group.ID, items)
		require.ErrorIs(t, err, ErrInvalidGroupProtocolLayout)
	}
	_, err = svc.ReplaceGroupProtocolLayout(other.ID, []GroupProtocolLayoutItem{{ProtocolID: hk}})
	require.ErrorIs(t, err, ErrInvalidGroupProtocolLayout, "the protocol is not linked to that group")
	_, err = svc.GroupProtocolLayout(404)
	require.ErrorIs(t, err, ErrSubscriptionGroupNotFound)
	require.Equal(t, []string{"日本 01", "BGP HK - ss", "BGP US - ss"}, names(), "a refused layout changes nothing")

	_, err = svc.ReplaceGroupProtocolLayout(group.ID, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"BGP HK - ss", "BGP US - ss", "BGP JP - ss"}, names())
}
