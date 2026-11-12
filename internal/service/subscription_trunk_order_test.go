package service

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A node whose users leave over a trunk in the network model (a user
// ingress with a path through a transport) comes before direct nodes in its
// group and is marked trunk; the direct node keeps its place after it
// (anx F-51, 默认路径规则). Without a model the group's order stands.
func TestSubscriptionPutsTrunkNodesFirst(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.SubscriptionGroup{}, &model.NodeEntry{}, &model.SubscriptionGroupProtocol{},
		&model.ForwardNode{}, &model.Line{}, &model.ForwardRouteSubscription{}, &model.OperationLog{}, &model.NetworkModel{}))
	svc := &SubscriptionService{db: db}

	now := time.Now().Unix()
	group := &model.SubscriptionGroup{Name: "default", Enable: 1}
	require.NoError(t, db.Create(group).Error)
	add := func(name, host string) *model.Node {
		node := &model.Node{Name: name, Host: host, Show: 1, Status: model.NodeStatusOnline, LastCheckAt: &now}
		require.NoError(t, db.Create(node).Error)
		protocol := model.NodeProtocol{NodeID: node.ID, Name: "ss", Type: model.ProtocolShadowsocks, Port: 11111, Enable: 1, Show: 1}
		require.NoError(t, db.Create(&protocol).Error)
		require.NoError(t, db.Exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (?, ?)", group.ID, protocol.ID).Error)
		return node
	}
	direct := add("JP direct", "198.51.100.40")
	ingress := add("SH CN2", "sh.example.com")

	names := func() []string {
		parsed, err := svc.getInternalNodes(nil, &model.TemplateRenderContext{UUID: "u"}, []*model.SubscriptionGroup{group})
		require.NoError(t, err)
		var out []string
		for _, n := range parsed {
			out = append(out, n.Name+"/"+n.Path)
		}
		return out
	}
	require.Equal(t, []string{"JP direct - ss/", "SH CN2 - ss/"}, names(), "no model: the group's order")

	doc := networkmodel.Document{
		Lines: []networkmodel.Line{
			{ID: "sh-cn2", Node: fmt.Sprintf("proxy-%d", ingress.ID)},
			{ID: "jp", Node: fmt.Sprintf("proxy-%d", direct.ID)},
		},
		Ingresses: []networkmodel.Ingress{
			{ID: "sh-in", Line: "sh-cn2", Audience: "user"},
			{ID: "jp-in", Line: "jp", Audience: "user"},
		},
		Paths: []networkmodel.Path{
			{ID: "sh-jp", Ingress: "sh-in", Segments: []string{"sh-jp"}, Egress: "jp-out"},
			{ID: "jp-local", Ingress: "jp-in", Egress: "jp-out"},
		},
	}
	encoded, err := json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.NetworkModel{ID: 1, Revision: 1, DocumentJSON: string(encoded), UpdatedAt: time.Now()}).Error)
	require.Equal(t, []string{"SH CN2 - ss/trunk", "JP direct - ss/"}, names())
}
