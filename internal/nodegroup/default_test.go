package nodegroup

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The default template is a template ParseTemplate accepts, so the first
// group of a panel can never be one a join refuses.
func TestDefaultTemplateIsValid(t *testing.T) {
	encoded, err := json.Marshal(DefaultTemplate())
	require.NoError(t, err)
	entries, err := ParseTemplate(encoded)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	entry := entries[0]
	assert.Equal(t, "vless", entry.Type)
	assert.Equal(t, TemplateTLSReality, entry.TLS)
	assert.Equal(t, []int{DefaultPortRangeFrom, DefaultPortRangeTo}, entry.PortRange)
	require.NotNil(t, entry.Reality)
	assert.Equal(t, DefaultRealityServerName, entry.Reality.ServerName)
	assert.Equal(t, DefaultRealityServerName+":443", entry.Reality.Dest)
}

func TestEnsureDefaultCreatesTheFirstGroup(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		// A fresh panel seeds a subscription group named "default"; the
		// fixture's Standard and Premium are renamed away from the names
		// the default group looks for.
		require.NoError(t, f.db.Model(&model.SubscriptionGroup{}).Where("id = ?", f.subscriptions[0].ID).Update("name", "default").Error)

		view, created, err := f.svc.EnsureDefault(ctx, f.admin)
		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, DefaultGroupName, view.Name)
		assert.Equal(t, model.NodeGroupKindProxy, view.Kind)
		assert.False(t, view.RequireApproval)
		assert.Equal(t, []uint{f.subscriptions[0].ID}, view.SubscriptionGroupIDs, "the panel's seeded default subscription group")
		require.Len(t, view.ProtocolTemplate, 1)
		assert.Equal(t, DefaultRealityServerName, view.ProtocolTemplate[0].Reality.ServerName)
		assert.Len(t, f.audits(t, AuditActionCreate), 1)

		// A machine that joins gets one Reality protocol on a port of the
		// range, in the subscription group.
		revealed, err := f.svc.RevealToken(ctx, view.ID, f.admin)
		require.NoError(t, err)
		joined, err := f.join(t, JoinRequest{
			Token: revealed.Token, MachineFingerprint: fingerprint("a"), Hostname: "first",
			PublicAddresses: []string{"203.0.113.5"},
		})
		require.NoError(t, err)
		var protocols []model.NodeProtocol
		require.NoError(t, f.db.Preload("SubscriptionGroups").Where("node_id = ?", joined.Node.ID).Find(&protocols).Error)
		require.Len(t, protocols, 1)
		assert.Equal(t, model.ProtocolVLESS, protocols[0].Type)
		assert.Equal(t, 2, protocols[0].TLS)
		assert.GreaterOrEqual(t, protocols[0].Port, DefaultPortRangeFrom)
		assert.LessOrEqual(t, protocols[0].Port, DefaultPortRangeTo)
		require.Len(t, protocols[0].SubscriptionGroups, 1)
		assert.Equal(t, f.subscriptions[0].ID, protocols[0].SubscriptionGroups[0].ID)

		// Called again it creates nothing and answers the same group.
		again, created, err := f.svc.EnsureDefault(ctx, f.admin)
		require.NoError(t, err)
		assert.False(t, created)
		assert.Equal(t, view.ID, again.ID)
		assert.Len(t, f.audits(t, AuditActionCreate), 1)
		var count int64
		require.NoError(t, f.db.Model(&model.NodeGroup{}).Count(&count).Error)
		assert.EqualValues(t, 1, count)
	})
}

func TestEnsureDefaultCreatesTheSubscriptionGroupWhenThereIsNone(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		// Other subscription groups exist, but none is the default one: a
		// "vip" group must not receive the first node.
		require.NoError(t, f.db.Model(&model.SubscriptionGroup{}).Where("id = ?", f.subscriptions[0].ID).Update("name", "vip").Error)

		view, created, err := f.svc.EnsureDefault(ctx, f.admin)
		require.NoError(t, err)
		assert.True(t, created)
		require.Len(t, view.SubscriptionGroupIDs, 1)
		var group model.SubscriptionGroup
		require.NoError(t, f.db.First(&group, view.SubscriptionGroupIDs[0]).Error)
		assert.Equal(t, DefaultSubscriptionGroupName, group.Name)
		assert.Equal(t, 1, group.Enable)
		assert.NotEqual(t, f.subscriptions[0].ID, group.ID)
		assert.NotEqual(t, f.subscriptions[1].ID, group.ID)

		// The preferred name wins over the seeded one.
		require.NoError(t, f.svc.Delete(ctx, view.ID, f.admin))
		require.NoError(t, f.db.Model(&model.SubscriptionGroup{}).Where("id = ?", f.subscriptions[1].ID).Update("name", "default").Error)
		again, created, err := f.svc.EnsureDefault(ctx, f.admin)
		require.NoError(t, err)
		assert.True(t, created, "a deleted group's name and the panel's emptiness make a new default")
		assert.Equal(t, []uint{group.ID}, again.SubscriptionGroupIDs)
	})
}

func TestEnsureDefaultLeavesAPanelWithGroupsAlone(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		forward, _ := f.createGroup(t, GroupInput{Name: "edge", Kind: "forward", Sort: 1})
		proxy, _ := f.createGroup(t, GroupInput{Name: "HK", Kind: "proxy", Sort: 0})

		view, created, err := f.svc.EnsureDefault(ctx, f.admin)
		require.NoError(t, err)
		assert.False(t, created)
		assert.Equal(t, proxy.ID, view.ID, "the first group by sort")
		assert.NotEqual(t, forward.ID, view.ID)
		var count int64
		require.NoError(t, f.db.Model(&model.NodeGroup{}).Count(&count).Error)
		assert.EqualValues(t, 2, count)
	})
}

func TestEnsureDefaultNeedsTheKeyEncryptionKey(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		f.kek = ""
		_, created, err := f.svc.EnsureDefault(t.Context(), f.admin)
		assert.ErrorIs(t, err, ErrNoKEK)
		assert.False(t, created)
		var groups, subscriptions int64
		require.NoError(t, f.db.Model(&model.NodeGroup{}).Count(&groups).Error)
		require.NoError(t, f.db.Model(&model.SubscriptionGroup{}).Where("name = ?", DefaultSubscriptionGroupName).Count(&subscriptions).Error)
		assert.Zero(t, groups)
		assert.Zero(t, subscriptions, "nothing is created when the group cannot be")
	})
}

func TestNodesOfAGroup(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		group, token := f.createGroup(t, GroupInput{Name: "HK", Kind: "proxy", RequireApproval: true})

		empty, err := f.svc.Nodes(ctx, group.ID)
		require.NoError(t, err)
		assert.NotNil(t, empty)
		assert.Empty(t, empty, "waiting for a machine")

		first, err := f.join(t, JoinRequest{Token: token, MachineFingerprint: fingerprint("a"), RequestedName: "hk-01",
			PublicAddresses: []string{"203.0.113.5"}, AgentVersion: "v5.0.0"})
		require.NoError(t, err)
		second, err := f.join(t, JoinRequest{Token: token, MachineFingerprint: fingerprint("b"), RequestedName: "hk-02",
			PublicAddresses: []string{"203.0.113.6"}, AgentVersion: "v5.0.1"})
		require.NoError(t, err)
		third, err := f.join(t, JoinRequest{Token: token, MachineFingerprint: fingerprint("c"), RequestedName: "hk-03",
			PublicAddresses: []string{"203.0.113.7"}})
		require.NoError(t, err)

		// hk-01 checked in a minute ago and was approved; hk-02 checked in
		// an hour ago; hk-03 never did and is rejected (so not live).
		recent := f.clock.Now().Add(-time.Minute).Unix()
		old := f.clock.Now().Add(-time.Hour).Unix()
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", first.Node.ID).
			Updates(map[string]any{"approval": model.NodeApprovalApproved, "last_check_at": recent}).Error)
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", second.Node.ID).Update("last_check_at", old).Error)
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", third.Node.ID).Update("approval", model.NodeApprovalRejected).Error)

		nodes, err := f.svc.Nodes(ctx, group.ID)
		require.NoError(t, err)
		require.Len(t, nodes, 2)
		assert.Equal(t, "hk-02", nodes[0].Name, "newest first")
		assert.Equal(t, "hk-01", nodes[1].Name)

		online := nodes[1]
		assert.EqualValues(t, first.Node.ID, online.ID)
		assert.Equal(t, first.Node.String(), online.Node)
		assert.Equal(t, "proxy", online.Kind)
		assert.Equal(t, "203.0.113.5", online.Host)
		assert.Equal(t, model.NodeApprovalApproved, online.Approval)
		assert.True(t, online.Online)
		require.NotNil(t, online.LastSeen)
		assert.Equal(t, recent, online.LastSeen.Unix())
		assert.Equal(t, "v5.0.0", online.AgentVersion)
		assert.False(t, online.CreatedAt.IsZero())

		stale := nodes[0]
		assert.Equal(t, model.NodeApprovalPending, stale.Approval)
		assert.False(t, stale.Online, "checked in an hour ago")
		assert.Equal(t, "v5.0.1", stale.AgentVersion)

		// A disabled node is not online, whatever its last check says.
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", first.Node.ID).Update("status", model.NodeStatusDisabled).Error)
		nodes, err = f.svc.Nodes(ctx, group.ID)
		require.NoError(t, err)
		assert.False(t, nodes[1].Online)

		_, err = f.svc.Nodes(ctx, 4242)
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestNodesOfAForwardGroup(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		group, token := f.createGroup(t, GroupInput{Name: "edge", Kind: "forward"})
		joined, err := f.join(t, JoinRequest{Token: token, MachineFingerprint: fingerprint("f"), RequestedName: "edge-01",
			PublicAddresses: []string{"203.0.113.9"}, AgentVersion: "v5.0.0"})
		require.NoError(t, err)

		nodes, err := f.svc.Nodes(ctx, group.ID)
		require.NoError(t, err)
		require.Len(t, nodes, 1)
		assert.Equal(t, "forward", nodes[0].Kind)
		assert.Equal(t, joined.Node.String(), nodes[0].Node)
		assert.Equal(t, "edge-01", nodes[0].Name)
		assert.False(t, nodes[0].Online, "it has not connected")
		assert.Nil(t, nodes[0].LastSeen)

		// Online once its Agent checked in; the version is the one of the
		// Agent's newest stream.
		now := f.clock.Now()
		require.NoError(t, f.db.Model(&model.ForwardNode{}).Where("id = ?", joined.Node.ID).
			Updates(map[string]any{"status": model.ForwardNodeStatusOnline, "last_check": now.Add(-30 * time.Second)}).Error)
		require.NoError(t, f.db.Create(&model.AgentTransport{
			NodeKind: "forward", NodeID: uint(joined.Node.ID), Transport: model.AgentTransportMTLSStream, AgentVersion: "v5.0.2",
			FirstSeenAt: now, LastSeenAt: now,
		}).Error)
		nodes, err = f.svc.Nodes(ctx, group.ID)
		require.NoError(t, err)
		assert.True(t, nodes[0].Online)
		assert.Equal(t, "v5.0.2", nodes[0].AgentVersion)

		// Disabling the node takes it offline.
		require.NoError(t, f.db.Model(&model.ForwardNode{}).Where("id = ?", joined.Node.ID).Update("enabled", false).Error)
		nodes, err = f.svc.Nodes(ctx, group.ID)
		require.NoError(t, err)
		assert.False(t, nodes[0].Online)
	})
}
