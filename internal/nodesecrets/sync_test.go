package nodesecrets

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fixture is a database written before P1: legacy rows only. Every value is
// fake.
type fixture struct {
	nodes      []model.Node
	keys       []model.AuthorizedKey
	forwards   []model.ForwardNode
	agents     []model.ForwardCleanAgent
	protocols  []model.NodeProtocol
	peers      []model.WireGuardPeer
	expireAt   int64
	revokedAt  time.Time
	rawConfig  string
	reality    string
	notJSON    string
	plainProto string
}

func seedLegacy(t *testing.T, db *gorm.DB) *fixture {
	t.Helper()
	f := &fixture{
		expireAt:   time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC).Unix(),
		revokedAt:  time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC),
		rawConfig:  `{"server_port":51820,"wireguard":{"private_key":"fake-raw-private","public_key":"fake-raw-public"}}`,
		reality:    `{"private_key":"fake-reality-private","short_id":"6ba85179","dest":"www.example.test:443"}`,
		notJSON:    "fake-custom-config",
		plainProto: `{"flow":"xtls-rprx-vision"}`,
	}
	f.nodes = []model.Node{
		{Name: "n1", Host: "203.0.113.1", APIKey: "fake-node-key-1", APIKeyHash: hashSecret("fake-node-key-1"), Secret: "fake-node-secret-1", RawConfig: &f.rawConfig},
		// An old row without a hash: the readers match the key itself.
		{Name: "n2", Host: "203.0.113.2", APIKey: "fake-node-key-2"},
		// A keyless node has no credential.
		{Name: "n3", Host: "203.0.113.3"},
	}
	for i := range f.nodes {
		require.NoError(t, db.Create(&f.nodes[i]).Error)
	}
	f.keys = []model.AuthorizedKey{
		{Name: "k1", Key: "fake-registration-1", KeyHash: hashSecret("fake-registration-1"), ExpireAt: &f.expireAt},
		{Name: "k2", Key: "fake-registration-2", KeyHash: hashSecret("fake-registration-2")},
	}
	for i := range f.keys {
		require.NoError(t, db.Create(&f.keys[i]).Error)
	}
	f.forwards = []model.ForwardNode{
		{Name: "f1", Host: "198.51.100.7", Port: 443, APIPort: 9000, APIToken: "fake-forward-token-1"},
		// An Ansible machine has no token.
		{Name: "f2", Host: "198.51.100.8", Port: 443},
	}
	for i := range f.forwards {
		require.NoError(t, db.Create(&f.forwards[i]).Error)
	}
	f.agents = []model.ForwardCleanAgent{
		{Name: "c1", Token: "fake-clean-token-1"},
		{Name: "c2", Token: "fake-clean-token-2", Status: model.ForwardCleanAgentStatusRevoked, RevokedAt: &f.revokedAt},
	}
	for i := range f.agents {
		require.NoError(t, db.Create(&f.agents[i]).Error)
	}
	f.protocols = []model.NodeProtocol{
		{NodeID: f.nodes[0].ID, Name: "reality", Type: model.ProtocolVLESS, Port: 443, TLS: 2, Settings: &f.plainProto, RealitySettings: &f.reality},
		{NodeID: f.nodes[0].ID, Name: "custom", Type: model.ProtocolVMess, Port: 8443, CustomConfig: &f.notJSON},
		{NodeID: f.nodes[1].ID, Name: "plain", Type: model.ProtocolVMess, Port: 443},
	}
	for i := range f.protocols {
		require.NoError(t, db.Create(&f.protocols[i]).Error)
	}
	f.peers = []model.WireGuardPeer{
		{NodeProtocolID: f.protocols[0].ID, UserID: 1, PeerIP: "10.66.0.2", PrivateKey: "fake-peer-private-1", PublicKey: "fake-peer-public-1", PresharedKey: "fake-peer-psk-1"},
		{NodeProtocolID: f.protocols[0].ID, UserID: 2, PeerIP: "10.66.0.3", PrivateKey: "fake-peer-private-2", PublicKey: "fake-peer-public-2", PresharedKey: "fake-peer-psk-2"},
	}
	for i := range f.peers {
		require.NoError(t, db.Create(&f.peers[i]).Error)
	}
	return f
}

// syncAll dual-writes every legacy row of the fixture, as the writers do.
func (f *fixture) syncAll(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		for table, ids := range f.ids() {
			if err := Sync(tx, table, ids...); err != nil {
				return err
			}
		}
		return nil
	}))
}

func (f *fixture) ids() map[string][]uint {
	ids := map[string][]uint{}
	for _, row := range f.nodes {
		ids[TableNode] = append(ids[TableNode], row.ID)
	}
	for _, row := range f.keys {
		ids[TableAuthorizedKey] = append(ids[TableAuthorizedKey], row.ID)
	}
	for _, row := range f.forwards {
		ids[TableForwardNode] = append(ids[TableForwardNode], row.ID)
	}
	for _, row := range f.agents {
		ids[TableCleanAgent] = append(ids[TableCleanAgent], row.ID)
	}
	for _, row := range f.protocols {
		ids[TableNodeProtocol] = append(ids[TableNodeProtocol], row.ID)
	}
	for _, row := range f.peers {
		ids[TableWireGuardPeer] = append(ids[TableWireGuardPeer], row.ID)
	}
	return ids
}

func credential(t *testing.T, db *gorm.DB, subjectKind string, subjectID uint, kind string) model.NodeCredential {
	t.Helper()
	var rows []model.NodeCredential
	require.NoError(t, db.Where("subject_kind = ? AND subject_id = ? AND kind = ? AND status <> ?", subjectKind, subjectID, kind, StatusRetired).Find(&rows).Error)
	require.Len(t, rows, 1, "%s %d %s", subjectKind, subjectID, kind)
	return rows[0]
}

func secrets(t *testing.T, db *gorm.DB, scope string, ownerID uint) map[string]string {
	t.Helper()
	var rows []model.ProtocolSecret
	require.NoError(t, db.Where("scope = ? AND owner_id = ?", scope, ownerID).Find(&rows).Error)
	out := map[string]string{}
	for _, row := range rows {
		out[row.ColumnName+" "+row.JSONPointer] = row.Value
	}
	return out
}

// requireVerified runs Verify and requires every table to match.
func requireVerified(t *testing.T, db *gorm.DB) []TableVerify {
	t.Helper()
	results, err := Verify(context.Background(), db, VerifyOptions{})
	require.NoError(t, err, "%+v", results)
	for _, result := range results {
		require.True(t, result.Match, "%+v", result)
		require.Equal(t, result.LegacyDigest, result.NewDigest)
	}
	return results
}

type snapshot struct {
	Credentials []model.NodeCredential
	Secrets     []model.ProtocolSecret
}

func takeSnapshot(t *testing.T, db *gorm.DB) snapshot {
	t.Helper()
	var s snapshot
	require.NoError(t, db.Order("id").Find(&s.Credentials).Error)
	require.NoError(t, db.Order("id").Find(&s.Secrets).Error)
	return s
}

func TestSyncDerivesEveryTable(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := seedLegacy(t, db)
		f.syncAll(t, db)

		key := credential(t, db, SubjectProxy, f.nodes[0].ID, KindNodeAPIKey)
		require.Equal(t, "fake-node-key-1", key.Value)
		require.Equal(t, hashSecret("fake-node-key-1"), key.KeyHash)
		require.Equal(t, StatusActive, key.Status)
		require.Equal(t, SourceDualWrite, key.Source)
		require.Equal(t, 1, key.Version)
		require.False(t, key.Sealed)
		require.Empty(t, key.KEKID)
		require.Equal(t, "fake-node-secret-1", credential(t, db, SubjectProxy, f.nodes[0].ID, KindNodeSharedSecret).Value)
		// No stored hash: the hash of the key.
		require.Equal(t, hashSecret("fake-node-key-2"), credential(t, db, SubjectProxy, f.nodes[1].ID, KindNodeAPIKey).KeyHash)
		var count int64
		require.NoError(t, db.Model(&model.NodeCredential{}).Where("subject_kind = ? AND subject_id IN ?", SubjectProxy, []uint{f.nodes[1].ID, f.nodes[2].ID}).Count(&count).Error)
		require.EqualValues(t, 1, count, "n2 has a key and no secret, n3 nothing")

		registration := credential(t, db, SubjectRegistrationKey, f.keys[0].ID, KindRegistrationKey)
		require.Equal(t, "fake-registration-1", registration.Value)
		require.NotNil(t, registration.ExpiresAt)
		require.Equal(t, f.expireAt, registration.ExpiresAt.Unix())
		require.Nil(t, credential(t, db, SubjectRegistrationKey, f.keys[1].ID, KindRegistrationKey).ExpiresAt)

		token := credential(t, db, SubjectForward, f.forwards[0].ID, KindForwardNodeToken)
		require.Equal(t, "fake-forward-token-1", token.Value)
		require.Equal(t, "198.51.100.7:9000", token.Endpoint)
		require.NoError(t, db.Model(&model.NodeCredential{}).Where("subject_kind = ? AND subject_id = ?", SubjectForward, f.forwards[1].ID).Count(&count).Error)
		require.Zero(t, count, "a forward node without a token")

		require.Equal(t, StatusActive, credential(t, db, SubjectCleanAgent, f.agents[0].ID, KindCleanAgentToken).Status)
		revoked := credential(t, db, SubjectCleanAgent, f.agents[1].ID, KindCleanAgentToken)
		require.Equal(t, StatusRevoked, revoked.Status)
		require.Equal(t, "fake-clean-token-2", revoked.Value)
		require.NotNil(t, revoked.RevokedAt)
		require.Equal(t, f.revokedAt.Unix(), revoked.RevokedAt.Unix())

		require.Equal(t, map[string]string{"raw_config /wireguard/private_key": `"fake-raw-private"`}, secrets(t, db, ScopeNodeRawConfig, f.nodes[0].ID))
		require.Equal(t, map[string]string{"reality_settings /private_key": `"fake-reality-private"`}, secrets(t, db, ScopeNodeProtocol, f.protocols[0].ID))
		require.Equal(t, map[string]string{"custom_config ": "fake-custom-config"}, secrets(t, db, ScopeNodeProtocol, f.protocols[1].ID))
		require.Empty(t, secrets(t, db, ScopeNodeProtocol, f.protocols[2].ID))
		require.Equal(t, map[string]string{"private_key ": "fake-peer-private-1", "preshared_key ": "fake-peer-psk-1"}, secrets(t, db, ScopeWireGuardPeer, f.peers[0].ID))

		requireVerified(t, db)
	})
}

func TestSyncRotatesChangesAndRemoves(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := seedLegacy(t, db)
		f.syncAll(t, db)
		node := f.nodes[0]
		sync := func(table string, id uint) {
			t.Helper()
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return Sync(tx, table, id) }))
		}

		// A new key retires the old version and keeps no copy of it.
		require.NoError(t, db.Model(&model.Node{}).Where("id = ?", node.ID).Updates(map[string]any{
			"api_key": "fake-node-key-1b", "api_key_hash": hashSecret("fake-node-key-1b"),
		}).Error)
		sync(TableNode, node.ID)
		var versions []model.NodeCredential
		require.NoError(t, db.Where("subject_kind = ? AND subject_id = ? AND kind = ?", SubjectProxy, node.ID, KindNodeAPIKey).Order("version").Find(&versions).Error)
		require.Len(t, versions, 2)
		require.Equal(t, StatusRetired, versions[0].Status)
		require.Empty(t, versions[0].Value)
		require.Empty(t, versions[0].KeyHash)
		require.NotNil(t, versions[0].RotatedAt)
		require.Equal(t, 2, versions[1].Version)
		require.Equal(t, "fake-node-key-1b", versions[1].Value)
		require.NotNil(t, versions[1].RotatedAt)
		requireVerified(t, db)

		// Revoking keeps the version; moving a forward node moves its pin.
		now := time.Now()
		require.NoError(t, db.Model(&model.ForwardCleanAgent{}).Where("id = ?", f.agents[0].ID).Updates(map[string]any{
			"status": model.ForwardCleanAgentStatusRevoked, "revoked_at": &now,
		}).Error)
		sync(TableCleanAgent, f.agents[0].ID)
		agent := credential(t, db, SubjectCleanAgent, f.agents[0].ID, KindCleanAgentToken)
		require.Equal(t, StatusRevoked, agent.Status)
		require.Equal(t, 1, agent.Version)
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", f.forwards[0].ID).Updates(map[string]any{"host": "198.51.100.9", "api_port": 9443}).Error)
		sync(TableForwardNode, f.forwards[0].ID)
		token := credential(t, db, SubjectForward, f.forwards[0].ID, KindForwardNodeToken)
		require.Equal(t, "198.51.100.9:9443", token.Endpoint)
		require.Equal(t, 1, token.Version)

		// A changed secret position counts a version; a removed one goes.
		changed := `{"private_key":"fake-reality-private-b","short_id":"6ba85179"}`
		require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", f.protocols[0].ID).Update("reality_settings", changed).Error)
		sync(TableNodeProtocol, f.protocols[0].ID)
		var position model.ProtocolSecret
		require.NoError(t, db.Where("scope = ? AND owner_id = ? AND json_pointer = ?", ScopeNodeProtocol, f.protocols[0].ID, "/private_key").First(&position).Error)
		require.Equal(t, `"fake-reality-private-b"`, position.Value)
		require.Equal(t, 2, position.Version)
		require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", f.protocols[0].ID).Update("reality_settings", `{"short_id":"6ba85179"}`).Error)
		sync(TableNodeProtocol, f.protocols[0].ID)
		require.Empty(t, secrets(t, db, ScopeNodeProtocol, f.protocols[0].ID))
		requireVerified(t, db)

		// A deleted row takes every row of it, retired versions included.
		require.NoError(t, db.Delete(&model.Node{}, node.ID).Error)
		sync(TableNode, node.ID)
		var count int64
		require.NoError(t, db.Model(&model.NodeCredential{}).Where("subject_kind = ? AND subject_id = ?", SubjectProxy, node.ID).Count(&count).Error)
		require.Zero(t, count)
		require.Empty(t, secrets(t, db, ScopeNodeRawConfig, node.ID))
		require.NoError(t, db.Delete(&model.WireGuardPeer{}, f.peers[0].ID).Error)
		sync(TableWireGuardPeer, f.peers[0].ID)
		require.Empty(t, secrets(t, db, ScopeWireGuardPeer, f.peers[0].ID))
		requireVerified(t, db)
	})
}

func TestBackfillThenVerifyIsIdempotent(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		seedLegacy(t, db)

		// Before the backfill the new tables are empty: every secret is
		// missing, and the report names none of them.
		results, err := Verify(ctx, db, VerifyOptions{})
		require.ErrorIs(t, err, ErrMismatch)
		missing := int64(0)
		for _, result := range results {
			require.False(t, result.Match && result.LegacySecrets > 0)
			missing += result.Missing
		}
		require.EqualValues(t, 15, missing)
		report, err := json.Marshal(results)
		require.NoError(t, err)
		require.NotContains(t, string(report), "fake-")

		backfilled, err := Backfill(ctx, db, BackfillOptions{BatchSize: 2})
		require.NoError(t, err)
		require.Len(t, backfilled, len(Tables()))
		changed := int64(0)
		for _, result := range backfilled {
			require.False(t, result.Resumed)
			changed += result.Changed
		}
		require.EqualValues(t, 15, changed)

		verified := requireVerified(t, db)
		var splits []model.NodeSecretSplit
		require.NoError(t, db.Order("table_name").Find(&splits).Error)
		require.Len(t, splits, len(Tables()))
		digests := map[string]string{}
		for _, result := range verified {
			digests[result.Table] = result.LegacyDigest
		}
		for _, split := range splits {
			require.Equal(t, PhaseDualWrite, split.Phase, "verify leaves the phase at P1")
			require.NotNil(t, split.BackfilledAt)
			require.NotNil(t, split.VerifiedAt)
			require.Equal(t, digests[split.Table], split.Digest)
			require.Zero(t, split.Mismatches)
		}
		var sources []string
		require.NoError(t, db.Model(&model.NodeCredential{}).Distinct().Pluck("source", &sources).Error)
		require.Equal(t, []string{SourceBackfill}, sources)

		// A second backfill changes nothing, not even a timestamp.
		before := takeSnapshot(t, db)
		again, err := Backfill(ctx, db, BackfillOptions{})
		require.NoError(t, err)
		for _, result := range again {
			require.Zero(t, result.Changed, result.Table)
		}
		require.Equal(t, before, takeSnapshot(t, db))
		requireVerified(t, db)
	})
}

func TestBackfillResumesAndPrunes(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		f := seedLegacy(t, db)
		// An interrupted pass stopped after the first node.
		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", TableNode).Updates(map[string]any{
			"backfill_cursor": f.nodes[0].ID, "backfilled_rows": 1, "backfilled_at": nil,
		}).Error)
		// A credential of a node that no longer exists.
		require.NoError(t, db.Create(&model.NodeCredential{
			SubjectKind: SubjectProxy, SubjectID: 999, Kind: KindNodeAPIKey, Version: 1, KeyHash: hashSecret("fake-orphan"),
			Value: "fake-orphan", Status: StatusActive, Source: SourceDualWrite, CreatedAt: time.Now(),
		}).Error)

		results, err := Backfill(ctx, db, BackfillOptions{Tables: []string{TableNode}, BatchSize: 1})
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.True(t, results[0].Resumed)
		require.EqualValues(t, 3, results[0].Rows)
		require.EqualValues(t, 1, results[0].Pruned)
		// The resumed pass skipped the first node: verify says so.
		verified, err := Verify(ctx, db, VerifyOptions{Tables: []string{TableNode}})
		require.ErrorIs(t, err, ErrMismatch)
		require.EqualValues(t, 3, verified[0].Missing, "the first node's key, secret and raw configuration")
		require.Zero(t, verified[0].Extra)

		// The pass completed, so the next one starts over and copies it.
		results, err = Backfill(ctx, db, BackfillOptions{Tables: []string{TableNode}})
		require.NoError(t, err)
		require.False(t, results[0].Resumed)
		require.EqualValues(t, 3, results[0].Changed)
		verified, err = Verify(ctx, db, VerifyOptions{Tables: []string{TableNode}})
		require.NoError(t, err)
		require.True(t, verified[0].Match)

		// Restart ignores an interrupted pass.
		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", TableNode).Updates(map[string]any{
			"backfill_cursor": f.nodes[2].ID, "backfilled_at": nil,
		}).Error)
		results, err = Backfill(ctx, db, BackfillOptions{Tables: []string{TableNode}, Restart: true})
		require.NoError(t, err)
		require.False(t, results[0].Resumed)
		require.EqualValues(t, 3, results[0].Rows)
	})
}

func TestVerifyDetectsMismatch(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		f := seedLegacy(t, db)
		_, err := Backfill(ctx, db, BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		var before model.NodeSecretSplit
		require.NoError(t, db.First(&before, "table_name = ?", TableForwardNode).Error)

		// A legacy write that skipped the writer, and a new row changed
		// behind its back.
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", f.forwards[0].ID).Update("api_token", "fake-forward-token-x").Error)
		require.NoError(t, db.Model(&model.ProtocolSecret{}).Where("scope = ? AND owner_id = ?", ScopeWireGuardPeer, f.peers[1].ID).
			Where("column_name = ?", "private_key").Update("value", "fake-peer-private-x").Error)
		require.NoError(t, db.Model(&model.NodeCredential{}).Where("subject_kind = ? AND subject_id = ?", SubjectRegistrationKey, f.keys[1].ID).
			Update("status", StatusRevoked).Error)

		results, err := Verify(ctx, db, VerifyOptions{})
		require.ErrorIs(t, err, ErrMismatch)
		byTable := map[string]TableVerify{}
		for _, result := range results {
			byTable[result.Table] = result
		}
		forward := byTable[TableForwardNode]
		require.False(t, forward.Match)
		require.NotEqual(t, forward.LegacyDigest, forward.NewDigest)
		require.EqualValues(t, 1, forward.Different)
		require.Equal(t, []Mismatch{{Problem: "different", Secret: "forward " + uintString(f.forwards[0].ID) + " forward_node_token"}}, forward.Samples)
		require.EqualValues(t, 1, byTable[TableWireGuardPeer].Different)
		require.Equal(t, "wireguard_peer "+uintString(f.peers[1].ID)+" private_key ", byTable[TableWireGuardPeer].Samples[0].Secret)
		require.EqualValues(t, 1, byTable[TableAuthorizedKey].Different, "the status is part of the digest")
		require.True(t, byTable[TableNode].Match)
		report, err := json.Marshal(results)
		require.NoError(t, err)
		require.NotContains(t, string(report), "fake-")

		var after model.NodeSecretSplit
		require.NoError(t, db.First(&after, "table_name = ?", TableForwardNode).Error)
		require.EqualValues(t, 1, after.Mismatches)
		require.NotNil(t, after.CheckedAt)
		require.Equal(t, before.Digest, after.Digest, "a mismatch keeps the last verified digest")
		require.Equal(t, before.VerifiedAt.Unix(), after.VerifiedAt.Unix())

		// A backfill repairs both kinds of drift.
		_, err = Backfill(ctx, db, BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
	})
}

func TestTombstonesAgainstUniqueIndexes(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		f := seedLegacy(t, db)
		f.syncAll(t, db)
		before := takeSnapshot(t, db)

		// Every unique credential column takes a tombstone per row.
		for _, row := range f.nodes {
			require.NoError(t, db.Model(&model.Node{}).Where("id = ?", row.ID).Updates(map[string]any{"api_key": Tombstone(uint64(row.ID)), "api_key_hash": ""}).Error)
		}
		for _, row := range f.keys {
			require.NoError(t, db.Model(&model.AuthorizedKey{}).Where("id = ?", row.ID).Updates(map[string]any{"key": Tombstone(uint64(row.ID)), "key_hash": ""}).Error)
		}
		for _, row := range f.agents {
			require.NoError(t, db.Model(&model.ForwardCleanAgent{}).Where("id = ?", row.ID).Update("token", Tombstone(uint64(row.ID))).Error)
		}
		// An empty value would collide on the second row.
		require.Error(t, db.Model(&model.AuthorizedKey{}).Where("id IN ?", []uint{f.keys[0].ID, f.keys[1].ID}).Update("key", "").Error)
		require.Error(t, db.Model(&model.ForwardCleanAgent{}).Where("id IN ?", []uint{f.agents[0].ID, f.agents[1].ID}).Update("token", "").Error)
		require.NoError(t, db.Model(&model.Node{}).Where("id = ?", f.nodes[2].ID).Update("api_key", "").Error, "one empty key fits")
		require.Error(t, db.Model(&model.Node{}).Where("id = ?", f.nodes[1].ID).Update("api_key", "").Error)

		// A tombstone is never copied, and never removes the secret it
		// stands for.
		f.syncAll(t, db)
		after := takeSnapshot(t, db)
		require.Equal(t, before.Credentials, after.Credentials)
		for _, row := range after.Credentials {
			require.False(t, IsTombstone(row.Value))
		}
		// In P1 a tombstone is unexpected: verify reports the secrets the
		// legacy columns no longer hold.
		results, err := Verify(ctx, db, VerifyOptions{Tables: []string{TableAuthorizedKey, TableCleanAgent}})
		require.ErrorIs(t, err, ErrMismatch)
		for _, result := range results {
			require.EqualValues(t, 2, result.Extra, result.Table)
		}

		// The new tables' own unique indexes: one row per version and per
		// position.
		duplicate := after.Credentials[0]
		duplicate.ID = 0
		require.Error(t, db.Create(&duplicate).Error)
		duplicate.Version++
		duplicate.Status = StatusRetired
		require.NoError(t, db.Create(&duplicate).Error)
		position := after.Secrets[0]
		position.ID = 0
		require.Error(t, db.Create(&position).Error)
	})
}

func TestSplitTablesAndPhase(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		rows, err := Status(context.Background(), db)
		require.NoError(t, err)
		require.Len(t, rows, len(Tables()))
		for _, row := range rows {
			require.Equal(t, PhaseDualWrite, row.Phase)
		}
		// EnsureSchema never changes an existing phase.
		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", TableNode).Update("phase", PhaseLegacy).Error)
		require.NoError(t, EnsureSchema(db))
		var split model.NodeSecretSplit
		require.NoError(t, db.First(&split, "table_name = ?", TableNode).Error)
		require.Equal(t, PhaseLegacy, split.Phase)

		_, err = Backfill(context.Background(), db, BackfillOptions{Tables: []string{"v2_user"}})
		require.Error(t, err)
		require.Error(t, Sync(db, "v2_user", 1))
	})
}

func TestSyncWithoutSplitTables(t *testing.T) {
	db := openSQLite(t)
	require.NoError(t, db.AutoMigrate(legacyModels...))
	f := seedLegacy(t, db)
	require.NoError(t, Sync(db, TableNode, f.nodes[0].ID), "nothing to dual-write")
	_, err := Backfill(context.Background(), db, BackfillOptions{})
	require.Error(t, err)
	_, err = Verify(context.Background(), db, VerifyOptions{})
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrMismatch))
}

func uintString(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
