package service

import (
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A node with entries (internal/nodeentry) is offered once per entry and
// protocol, at the entry's host and port and named after the entry; a NAT
// entry that does not map the protocol's port leaves it out. A node without
// entries is offered at its host.
func TestSubscriptionOffersANodeOncePerEntry(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	svc := &SubscriptionService{db: db}
	ctx := &model.TemplateRenderContext{UUID: "u"}
	node := &model.Node{ID: 60, Name: "SH relay", Host: "relay.example.com"}
	ss := &model.NodeProtocol{ID: 7, NodeID: 60, Name: "JP", Type: model.ProtocolShadowsocks, Port: 30000}
	trojan := &model.NodeProtocol{ID: 8, NodeID: 60, Name: "JP trojan", Type: model.ProtocolTrojan, Port: 8443}

	plain := svc.nodeProtocolEntries(node, ss, ctx, nil, "")
	require.Len(t, plain, 1)
	require.Equal(t, "relay.example.com", plain[0].Server)
	require.Equal(t, 30000, plain[0].Port)

	entries := []model.NodeEntry{
		{ID: 1, NodeID: 60, Label: "CN2 v6", Kind: model.NodeEntryDirect, Host: "2001:db8::10"},
		{ID: 2, NodeID: 60, Label: "CN2 NAT", Kind: model.NodeEntryNAT, Host: "198.51.100.7", PortMap: `[{"from":30000,"to":30099,"external":21000}]`},
	}
	offered := svc.nodeProtocolEntries(node, ss, ctx, entries, "")
	require.Len(t, offered, 2)
	require.Equal(t, "node-60-7-e1", offered[0].ID)
	require.Equal(t, "SH relay - JP | CN2 v6", offered[0].Name)
	require.Equal(t, "2001:db8::10", offered[0].Server)
	require.Equal(t, 30000, offered[0].Port)
	require.Equal(t, "SH relay - JP | CN2 NAT", offered[1].Name)
	require.Equal(t, "198.51.100.7", offered[1].Server)
	require.Equal(t, 21000, offered[1].Port)

	offered = svc.nodeProtocolEntries(node, trojan, ctx, entries, "")
	require.Len(t, offered, 1, "the NAT entry does not map 8443")
	require.Equal(t, "2001:db8::10", offered[0].Server)
}
