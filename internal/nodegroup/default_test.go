package nodegroup

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureDefaultCreatesTheFirstGroup(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()

		view, created, err := f.svc.EnsureDefault(ctx, f.admin)
		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, DefaultGroupName, view.Name)
		assert.False(t, view.RequireApproval)
		assert.Len(t, f.audits(t, AuditActionCreate), 1)

		// A machine that joins is a clean node: no protocol.
		revealed, err := f.svc.RevealToken(ctx, view.ID, f.admin)
		require.NoError(t, err)
		joined, err := f.join(t, JoinRequest{
			Token: revealed.Token, MachineFingerprint: fingerprint("a"), Hostname: "first",
			PublicAddresses: []string{"203.0.113.5"},
		})
		require.NoError(t, err)
		var protocols int64
		require.NoError(t, f.db.Model(&model.NodeProtocol{}).Where("node_id = ?", joined.Node.ID).Count(&protocols).Error)
		assert.Zero(t, protocols)

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

func TestEnsureDefaultLeavesAPanelWithGroupsAlone(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		forward, _ := f.createGroup(t, GroupInput{Name: "edge", Sort: 1})
		proxy, _ := f.createGroup(t, GroupInput{Name: "HK", Sort: 0})

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
		var groups int64
		require.NoError(t, f.db.Model(&model.NodeGroup{}).Count(&groups).Error)
		assert.Zero(t, groups, "nothing is created when the group cannot be")
	})
}

func TestNodesOfAGroup(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		group, token := f.createGroup(t, GroupInput{Name: "HK", RequireApproval: true})

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

// A node that reported no version of its own shows the one its Agent said in
// its newest stream.
func TestNodesOfAGroupFillTheAgentVersion(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		group, token := f.createGroup(t, GroupInput{Name: "edge"})
		joined, err := f.join(t, JoinRequest{Token: token, MachineFingerprint: fingerprint("p"), RequestedName: "edge-02",
			PublicAddresses: []string{"203.0.113.8"}})
		require.NoError(t, err)
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", joined.Node.ID).Update("server_version", nil).Error)

		nodes, err := f.svc.Nodes(ctx, group.ID)
		require.NoError(t, err)
		require.Len(t, nodes, 1)
		assert.Equal(t, joined.Node.String(), nodes[0].Node)
		assert.Empty(t, nodes[0].AgentVersion)

		now := f.clock.Now()
		require.NoError(t, f.db.Create(&model.AgentTransport{
			NodeKind: "proxy", NodeID: uint(joined.Node.ID), Transport: model.AgentTransportMTLSStream, AgentVersion: "v5.0.2",
			FirstSeenAt: now, LastSeenAt: now,
		}).Error)
		nodes, err = f.svc.Nodes(ctx, group.ID)
		require.NoError(t, err)
		assert.Equal(t, "v5.0.2", nodes[0].AgentVersion)

		// Names stay unique in the group.
		again, err := f.join(t, JoinRequest{Token: token, RequestedName: "edge-02", PublicAddresses: []string{"203.0.113.7"}})
		require.NoError(t, err)
		assert.Equal(t, "edge-02-2", again.NodeName)
	})
}
