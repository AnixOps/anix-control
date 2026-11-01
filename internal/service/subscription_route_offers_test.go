package service

import (
	"context"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/routesubscription"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
)

// A forward route published in subscriptions (Phase 10) offers its target
// protocol again at its entry, after the protocol's own node, in the
// protocol's groups.
func TestSubscriptionOffersPublishedForwardRoutes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.SubscriptionGroup{}, &model.NodeEntry{}, &model.SubscriptionGroupProtocol{},
		&model.ForwardNode{}, &model.KernelForwardRoute{}, &model.KernelForwardAllocation{}, &model.ForwardRouteSubscription{}, &model.OperationLog{}))
	svc := &SubscriptionService{db: db}

	now := time.Now().Unix()
	group := &model.SubscriptionGroup{Name: "ok", Enable: 1}
	require.NoError(t, db.Create(group).Error)
	node := &model.Node{Name: "KFC JP", Host: "104.251.233.29", Show: 1, Status: model.NodeStatusOnline, LastCheckAt: &now}
	require.NoError(t, db.Create(node).Error)
	protocol := model.NodeProtocol{NodeID: node.ID, Name: "ss", Type: model.ProtocolShadowsocks, Port: 11111, Enable: 1, Show: 1}
	require.NoError(t, db.Create(&protocol).Error)
	require.NoError(t, db.Exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (?, ?)", group.ID, protocol.ID).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 1, Name: "newnb-1488", Host: "sh.example.com", Port: 7000, Enabled: true}).Error)
	route := &forwardv1.Route{
		Id: "r1", Owner: "admin", Name: "KFC-JP",
		Listen:  &forwardv1.Listen{Port: 15301, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Hops:    []*forwardv1.Hop{{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, NodeRefs: []string{"forward-1"}}},
		Targets: []*forwardv1.Target{{Host: "104.251.233.29", Port: 11111}},
	}
	encoded, err := protojson.Marshal(route)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.KernelForwardRoute{ID: "r1", Owner: "admin", Revision: 1, RouteJSON: string(encoded), CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error)

	nodes := func() []*model.ParsedNode {
		parsed, err := svc.getInternalNodes(nil, &model.TemplateRenderContext{UUID: "u"}, []*model.SubscriptionGroup{group})
		require.NoError(t, err)
		return parsed
	}
	require.Len(t, nodes(), 1, "an unpublished route offers nothing")

	_, err = routesubscription.New(db).Set(context.Background(), "r1", true, "", routesubscription.Actor{})
	require.NoError(t, err)
	parsed := nodes()
	require.Len(t, parsed, 2)
	require.Equal(t, "KFC JP - ss", parsed[0].Name)
	require.Equal(t, "KFC JP - ss | KFC-JP", parsed[1].Name)
	require.Equal(t, "sh.example.com", parsed[1].Server)
	require.Equal(t, 15301, parsed[1].Port)
	require.Equal(t, group.ID, parsed[1].GroupID)
	require.NotEqual(t, parsed[0].ID, parsed[1].ID)
}
