package nodegroup

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupCRUDWithAPartialUniqueName(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		rate := 1.5
		created, err := f.svc.Create(ctx, GroupInput{
			Name: " HK ", MaxNodes: 10, RequireApproval: true, Rate: &rate, Tags: []string{"hk", "iepl", "hk"},
			MonthlyLimitBytes: 1 << 40, ResetDay: 15, Sort: 3,
			SubscriptionGroupIDs: []uint{f.subscriptions[1].ID, f.subscriptions[0].ID, f.subscriptions[1].ID},
		}, f.admin)
		require.NoError(t, err)
		assert.Equal(t, "HK", created.Name)
		assert.Equal(t, 10, created.MaxNodes)
		assert.True(t, created.RequireApproval)
		assert.Equal(t, 1.5, created.Rate)
		assert.Equal(t, []string{"hk", "iepl"}, created.Tags)
		assert.Equal(t, int64(1<<40), created.MonthlyLimitBytes)
		assert.Equal(t, 15, created.ResetDay)
		assert.Equal(t, []uint{f.subscriptions[0].ID, f.subscriptions[1].ID}, created.SubscriptionGroupIDs)
		assert.Empty(t, created.ProtocolTemplate)
		assert.Equal(t, f.clock.Now(), created.TokenResetAt.UTC())
		assert.Nil(t, created.TokenExpiresAt)

		// The stored token is its hash and a sealed copy; neither is the token.
		var stored model.NodeGroup
		require.NoError(t, f.db.First(&stored, created.ID).Error)
		assert.Len(t, stored.JoinTokenHash, 64)
		assert.NotContains(t, stored.JoinTokenSealed, "anixjoin_")

		// Defaults.
		forward, err := f.svc.Create(ctx, GroupInput{Name: "edge"}, f.admin)
		require.NoError(t, err)
		assert.Equal(t, 1.0, forward.Rate)
		assert.Equal(t, 1, forward.ResetDay)
		assert.Equal(t, []string{}, forward.Tags)
		assert.Equal(t, []uint{}, forward.SubscriptionGroupIDs)

		// Names are unique among live groups.
		_, err = f.svc.Create(ctx, GroupInput{Name: "HK"}, f.admin)
		assert.ErrorIs(t, err, ErrNameTaken)
		_, err = f.svc.Update(ctx, forward.ID, GroupInput{Name: "HK"}, f.admin)
		assert.ErrorIs(t, err, ErrNameTaken)

		listed, err := f.svc.List(ctx)
		require.NoError(t, err)
		require.Len(t, listed, 2)
		assert.Equal(t, "edge", listed[0].Name, "by sort, then id")

		// Update replaces the fields.
		updated, err := f.svc.Update(ctx, created.ID, GroupInput{Name: "HK 2", MaxNodes: 0, ResetDay: 2}, f.admin)
		require.NoError(t, err)
		assert.Equal(t, "HK 2", updated.Name)
		assert.False(t, updated.RequireApproval)
		assert.Equal(t, 1.0, updated.Rate)
		assert.Empty(t, updated.SubscriptionGroupIDs)
		_, err = f.svc.Update(ctx, 4242, GroupInput{Name: "x"}, f.admin)
		assert.ErrorIs(t, err, ErrNotFound)

		// Soft delete: the row stays, flagged and timed; the group is gone
		// for every read, and its name is free again.
		require.NoError(t, f.svc.Delete(ctx, created.ID, f.admin))
		_, err = f.svc.Get(ctx, created.ID)
		assert.ErrorIs(t, err, ErrNotFound)
		assert.ErrorIs(t, f.svc.Delete(ctx, created.ID, f.admin), ErrNotFound)
		var deleted model.NodeGroup
		require.NoError(t, f.db.Unscoped().First(&deleted, created.ID).Error)
		assert.EqualValues(t, 1, deleted.Deleted)
		require.NotNil(t, deleted.DeletedAt)
		again, err := f.svc.Create(ctx, GroupInput{Name: "HK 2"}, f.admin)
		require.NoError(t, err, "a deleted group's name can be used again")
		assert.NotEqual(t, created.ID, again.ID)

		// The database enforces it too: a second live row with the name is
		// refused, a deleted one is not.
		duplicate := model.NodeGroup{Name: "HK 2", Kind: "proxy", JoinTokenHash: strings.Repeat("e", 64), JoinTokenSealed: "x", TokenResetAt: time.Now()}
		assert.Error(t, f.db.Create(&duplicate).Error, "the partial unique index refuses a second live name")

		listed, err = f.svc.List(ctx)
		require.NoError(t, err)
		assert.Len(t, listed, 2)

		assert.Len(t, f.audits(t, AuditActionCreate), 3)
		assert.Len(t, f.audits(t, AuditActionUpdate), 1)
		deletes := f.audits(t, AuditActionDelete)
		require.Len(t, deletes, 1)
		assert.Equal(t, "root@example.com", deletes[0].Username)
		require.NotNil(t, deletes[0].TargetID)
		assert.Equal(t, created.ID, *deletes[0].TargetID)
	})
}

func TestGroupValidation(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	ctx := t.Context()
	negative, zero, huge := -1.0, 0.0, 1001.0
	past := f.clock.Now().Add(-time.Minute)
	cases := map[string]GroupInput{
		"no name":            {},
		"control characters": {Name: "a\x01b"},
		"long name":          {Name: strings.Repeat("n", 101)},
		"max nodes":          {Name: "g", MaxNodes: -1},
		"negative rate":      {Name: "g", Rate: &negative},
		"zero rate":          {Name: "g", Rate: &zero},
		"huge rate":          {Name: "g", Rate: &huge},
		"monthly limit":      {Name: "g", MonthlyLimitBytes: -1},
		"reset day":          {Name: "g", ResetDay: 29},
		"empty tag":          {Name: "g", Tags: []string{" "}},
		"too many tags":      {Name: "g", Tags: strings.Split("a,b,c,d,e,f,g,h,i,j,k,l,m,n,o,p,q", ",")},
		"expired token":      {Name: "g", TokenExpiresAt: &past},
		"missing subscription group": {
			Name: "g", SubscriptionGroupIDs: []uint{4242},
		},
		"bad template": {Name: "g", ProtocolTemplate: rawJSON(t, []map[string]any{{"type": "hysteria2", "port": 443}})},
	}
	for name, input := range cases {
		_, err := f.svc.Create(ctx, input, f.admin)
		assert.ErrorIs(t, err, ErrInvalid, name)
	}
	var count int64
	require.NoError(t, f.db.Model(&model.NodeGroup{}).Count(&count).Error)
	assert.Zero(t, count, "a refused group writes nothing")

	future := f.clock.Now().Add(24 * time.Hour)
	group, err := f.svc.Create(ctx, GroupInput{Name: "g", TokenExpiresAt: &future}, f.admin)
	require.NoError(t, err)
	require.NotNil(t, group.TokenExpiresAt)
	assert.False(t, group.TokenExpired)
	f.clock.Advance(25 * time.Hour)
	group, err = f.svc.Get(ctx, group.ID)
	require.NoError(t, err)
	assert.True(t, group.TokenExpired)
}

func TestGroupsNeedTheKeyEncryptionKey(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	group, _ := f.createGroup(t, GroupInput{Name: "g"})
	f.kek = ""
	_, err := f.svc.Create(t.Context(), GroupInput{Name: "h"}, f.admin)
	assert.ErrorIs(t, err, ErrNoKEK)
	_, err = f.svc.RevealToken(t.Context(), group.ID, f.admin)
	assert.ErrorIs(t, err, ErrNoKEK)
	_, err = f.svc.ResetToken(t.Context(), group.ID, f.admin)
	assert.ErrorIs(t, err, ErrNoKEK)
	f.kek = "not a key"
	_, err = f.svc.Create(t.Context(), GroupInput{Name: "h"}, f.admin)
	assert.ErrorIs(t, err, ErrNoKEK)
}

func TestJoinTokenSealingAndReset(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		group, token := f.createGroup(t, GroupInput{Name: "g"})
		assert.True(t, ValidToken(token), token)
		assert.True(t, strings.HasPrefix(token, "anixjoin_"))
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, "anixjoin_"))
		require.NoError(t, err)
		assert.Len(t, raw, 32)
		var stored model.NodeGroup
		require.NoError(t, f.db.First(&stored, group.ID).Error)
		assert.Equal(t, HashToken(token), stored.JoinTokenHash)

		// Revealing again answers the same token; each read is audited
		// without it.
		again, err := f.svc.RevealToken(ctx, group.ID, f.admin)
		require.NoError(t, err)
		assert.Equal(t, token, again.Token)
		reveals := f.audits(t, AuditActionTokenReveal)
		require.Len(t, reveals, 2)
		for _, entry := range reveals {
			assert.NotContains(t, entry.Content, token)
			assert.NotContains(t, entry.Content, stored.JoinTokenSealed)
		}

		// A sealed copy never opens under another key, nor for another
		// hash.
		sealer, err := f.svc.sealer()
		require.NoError(t, err)
		_, err = sealer.open(strings.Repeat("0", 64), stored.JoinTokenSealed)
		assert.ErrorIs(t, err, ErrSealedToken)
		other := &Service{KEK: func() string { return base64.StdEncoding.EncodeToString(make([]byte, 32)) }}
		otherSealer, err := other.sealer()
		require.NoError(t, err)
		_, err = otherSealer.open(stored.JoinTokenHash, stored.JoinTokenSealed)
		assert.ErrorIs(t, err, ErrSealedToken)

		joined, err := f.join(t, JoinRequest{Token: token, Hostname: "a", PublicAddresses: []string{"203.0.113.1"}})
		require.NoError(t, err)

		// A reset replaces the token: the old one joins nothing, the new
		// one does, and the node that joined keeps its certificate.
		f.clock.Advance(time.Minute)
		reset, err := f.svc.ResetToken(ctx, group.ID, f.admin)
		require.NoError(t, err)
		assert.Equal(t, f.clock.Now(), reset.TokenResetAt.UTC())
		fresh, err := f.svc.RevealToken(ctx, group.ID, f.admin)
		require.NoError(t, err)
		assert.NotEqual(t, token, fresh.Token)
		_, err = f.join(t, JoinRequest{Token: token, Hostname: "b", PublicAddresses: []string{"203.0.113.2"}})
		assert.ErrorIs(t, err, ErrJoinRejected)
		_, err = f.join(t, JoinRequest{Token: fresh.Token, Hostname: "b", PublicAddresses: []string{"203.0.113.2"}})
		require.NoError(t, err)
		var certificate model.AgentCertificate
		require.NoError(t, f.db.Where("serial = ?", joined.Issued.Serial).First(&certificate).Error)
		assert.Nil(t, certificate.RevokedAt)
		resets := f.audits(t, AuditActionTokenReset)
		require.Len(t, resets, 1)
		assert.NotContains(t, resets[0].Content, fresh.Token)
	})
}

func TestGroupDeleteRefusedWithLiveNodes(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		group, token := f.createGroup(t, GroupInput{Name: "g", RequireApproval: true})
		joined, err := f.join(t, JoinRequest{Token: token, Hostname: "a", PublicAddresses: []string{"203.0.113.1"}})
		require.NoError(t, err)
		err = f.svc.Delete(t.Context(), group.ID, f.admin)
		assert.ErrorIs(t, err, ErrInUse)

		// A rejected node is not live.
		_, err = f.svc.Reject(t.Context(), joined.Node, f.admin)
		require.NoError(t, err)
		view, err := f.svc.Get(t.Context(), group.ID)
		require.NoError(t, err)
		assert.Equal(t, NodeCounts{}, view.Nodes)
		require.NoError(t, f.svc.Delete(t.Context(), group.ID, f.admin))

		// The deleted group's token joins nothing.
		_, err = f.join(t, JoinRequest{Token: token, Hostname: "b", PublicAddresses: []string{"203.0.113.2"}})
		assert.True(t, errors.Is(err, ErrJoinRejected))
	})
}
