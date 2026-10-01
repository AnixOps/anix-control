package nodesecrets

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// allLegacyRows reads every column of every legacy row of the split tables,
// as stored: the bytes an older binary would read.
func allLegacyRows(t *testing.T, db *gorm.DB) map[string][]map[string]any {
	t.Helper()
	out := map[string][]map[string]any{}
	for _, table := range Tables() {
		var rows []map[string]any
		require.NoError(t, db.Table(table).Order("id").Find(&rows).Error)
		for _, row := range rows {
			for column, value := range row {
				if bytes, ok := value.([]byte); ok {
					row[column] = string(bytes)
				}
				if moment, ok := value.(time.Time); ok {
					row[column] = moment.UTC().Format(time.RFC3339Nano)
				}
			}
		}
		out[table] = rows
	}
	return out
}

func finalizeAll(t *testing.T, db *gorm.DB) []TableFinalize {
	t.Helper()
	results, err := Finalize(context.Background(), db, FinalizeOptions{Confirm: true, Actor: "test", BatchSize: 1})
	require.NoError(t, err, "%+v", results)
	return results
}

func splitState(t *testing.T, db *gorm.DB, table string) model.NodeSecretSplit {
	t.Helper()
	var row model.NodeSecretSplit
	require.NoError(t, db.Where("table_name = ?", table).First(&row).Error)
	return row
}

// requireFinalizedColumns requires the legacy columns of the fixture to
// hold exactly what section 4.4 leaves there.
func (f *fixture) requireFinalizedColumns(t *testing.T, db *gorm.DB) {
	t.Helper()
	var nodes []model.Node
	require.NoError(t, db.Order("id").Find(&nodes).Error)
	require.Equal(t, Tombstone(uint64(nodes[0].ID)), nodes[0].APIKey)
	require.Equal(t, Tombstone(uint64(nodes[1].ID)), nodes[1].APIKey)
	require.Empty(t, nodes[2].APIKey, "a node without a key keeps none")
	for _, node := range nodes {
		require.Empty(t, node.APIKeyHash)
	}
	require.Equal(t, Tombstone(uint64(nodes[0].ID)), nodes[0].Secret)
	require.Empty(t, nodes[1].Secret)
	require.Equal(t, Redact(f.rawConfig), *nodes[0].RawConfig)
	require.Contains(t, *nodes[0].RawConfig, Placeholder)
	require.Contains(t, *nodes[0].RawConfig, "fake-raw-public", "the public key stays")

	var keys []model.AuthorizedKey
	require.NoError(t, db.Order("id").Find(&keys).Error)
	for _, key := range keys {
		require.Equal(t, Tombstone(uint64(key.ID)), key.Key)
		require.Empty(t, key.KeyHash)
	}
	var forwards []model.ForwardNode
	require.NoError(t, db.Order("id").Find(&forwards).Error)
	require.Equal(t, Tombstone(uint64(forwards[0].ID)), forwards[0].APIToken)
	require.Empty(t, forwards[1].APIToken, "an Ansible machine keeps no token")
	var agents []model.ForwardCleanAgent
	require.NoError(t, db.Order("id").Find(&agents).Error)
	for _, agent := range agents {
		require.Equal(t, Tombstone(uint64(agent.ID)), agent.Token)
	}
	var protocols []model.NodeProtocol
	require.NoError(t, db.Order("id").Find(&protocols).Error)
	require.Equal(t, Redact(f.reality), *protocols[0].RealitySettings)
	require.Equal(t, f.plainProto, *protocols[0].Settings, "a column without a secret is untouched")
	require.Equal(t, Placeholder, *protocols[1].CustomConfig)
	var peers []model.WireGuardPeer
	require.NoError(t, db.Order("id").Find(&peers).Error)
	for _, peer := range peers {
		require.Equal(t, Tombstone(uint64(peer.ID)), peer.PrivateKey)
		require.Equal(t, Tombstone(uint64(peer.ID)), peer.PresharedKey)
		require.NotEmpty(t, peer.PublicKey)
	}
}

// TestFinalizeRefusals: finalize needs the confirmation, phase dual_read
// and a recent clean verification; one refusal refuses every table, and a
// refusal changes nothing.
func TestFinalizeRefusals(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		seedLegacy(t, db)
		_, err := Backfill(ctx, db, BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		before := allLegacyRows(t, db)

		refused := func(options FinalizeOptions, reason string) {
			t.Helper()
			results, err := Finalize(ctx, db, options)
			require.ErrorIs(t, err, ErrFinalizeRefused)
			found := false
			for _, result := range results {
				found = found || strings.Contains(result.Refused, reason)
			}
			require.True(t, found, "%+v", results)
			require.Equal(t, before, allLegacyRows(t, db), "a refusal changes no legacy column")
			for _, table := range Tables() {
				require.NotEqual(t, PhaseFinalized, splitState(t, db, table).Phase)
			}
		}
		// From dual_write.
		refused(FinalizeOptions{Confirm: true}, "finalize needs phase dual_read")
		// Without the confirmation.
		setPhase(t, db, PhaseDualRead)
		refused(FinalizeOptions{}, "confirm it")
		// Without a recent verification.
		refused(FinalizeOptions{Confirm: true, Now: time.Now().Add(2 * VerifyMaxAge)}, "older than")
		// After a verification that found a mismatch.
		require.NoError(t, db.Model(&model.ForwardCleanAgent{}).Where("token = ?", "fake-clean-token-1").
			UpdateColumn("token", "fake-clean-token-drifted").Error)
		_, err = Verify(ctx, db, VerifyOptions{Tables: []string{TableCleanAgent}})
		require.ErrorIs(t, err, ErrMismatch)
		before = allLegacyRows(t, db)
		refused(FinalizeOptions{Confirm: true, Tables: []string{TableCleanAgent}}, "found mismatches")
		// One refused table refuses all of them.
		refused(FinalizeOptions{Confirm: true}, "found mismatches")
		for _, entry := range audits(t, db) {
			require.NotEqual(t, "finalize_started", entry.Action, "a refusal audits nothing")
		}
	})
}

// TestFinalizeWritesTombstonesAndReadsOnlyTheNewTables: finalize writes the
// tombstones of section 4.4 in batches; afterwards every reader finds every
// secret in the new tables, never falls back, and never accepts a
// tombstone; verify compares by presence.
func TestFinalizeWritesTombstonesAndReadsOnlyTheNewTables(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		f := seedLegacy(t, db)
		toDualRead(t, db)
		want := f.read(t, db)
		newBefore := takeSnapshot(t, db)
		before := fallbacksTotal()

		results := finalizeAll(t, db)
		for _, result := range results {
			require.NotNil(t, result.FinalizedAt, result.Table)
			require.Equal(t, 2, result.Passes, "%s: the second pass finds nothing left", result.Table)
			state := splitState(t, db, result.Table)
			require.Equal(t, PhaseFinalized, state.Phase)
			require.NotNil(t, state.FinalizedAt)
			require.Equal(t, "test", state.FinalizedBy)
			require.Equal(t, PhaseFinalized, ReadPhase(db, result.Table))
		}
		f.requireFinalizedColumns(t, db)
		// The new tables are unchanged, but for the kept originals.
		newAfter := takeSnapshot(t, db)
		require.Equal(t, newBefore.Credentials, newAfter.Credentials)
		var positions []model.ProtocolSecret
		for _, row := range newAfter.Secrets {
			if row.Scope != scopeLegacyOriginal {
				positions = append(positions, row)
			}
		}
		require.Equal(t, newBefore.Secrets, positions)

		// Every reader answers as before, from the new tables only.
		got := f.read(t, db)
		require.Equal(t, before, fallbacksTotal(), "a finalized reader never falls back")
		require.Equal(t, want.NodeKeyMatches, got.NodeKeyMatches)
		require.Equal(t, want.NodeByKey, got.NodeByKey)
		require.Equal(t, want.NodeKeys, got.NodeKeys)
		require.Equal(t, want.NodeSecrets, got.NodeSecrets)
		require.Equal(t, want.RegistrationKeys, got.RegistrationKeys)
		require.True(t, got.ForwardMatches)
		require.Equal(t, want.ForwardToken, got.ForwardToken)
		require.Equal(t, want.CleanByToken, got.CleanByToken)
		require.Equal(t, want.CleanWithToken, got.CleanWithToken)
		equalJSON(t, f.reality, got.Reality)
		require.Equal(t, f.notJSON, got.Custom)
		equalJSON(t, f.rawConfig, got.RawConfig)
		require.Equal(t, want.PeerPrivate, got.PeerPrivate)
		require.Equal(t, want.PeerPreshared, got.PeerPreshared)

		// No tombstone authenticates.
		var nodes []model.Node
		require.NoError(t, db.Order("id").Find(&nodes).Error)
		for _, node := range nodes {
			require.False(t, NodeAPIKeyMatches(db, &node, node.APIKey))
			_, err := NodeByAPIKey(db, Tombstone(uint64(node.ID)), true)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		}
		_, err := CleanAgentByToken(db, Tombstone(uint64(f.agents[0].ID)))
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)

		// A secret only the legacy column holds is never read: no fallback.
		require.NoError(t, db.Model(&model.Node{}).Where("id = ?", nodes[2].ID).
			UpdateColumns(map[string]any{"api_key": "fake-legacy-only", "api_key_hash": hashSecret("fake-legacy-only"), "secret": "fake-legacy-secret"}).Error)
		require.NoError(t, db.First(&nodes[2], nodes[2].ID).Error)
		_, err = NodeByAPIKey(db, "fake-legacy-only", true)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		require.False(t, NodeAPIKeyMatches(db, &nodes[2], "fake-legacy-only"))
		require.Empty(t, NodeSharedSecret(db, &nodes[2]))
		// A literal secret a finalized JSON column holds anyway is ignored.
		literal := `{"private_key":"fake-planted","short_id":"01"}`
		protocol := model.NodeProtocol{NodeID: nodes[1].ID, Name: "planted", Type: model.ProtocolVLESS, Port: 9443, RealitySettings: &literal}
		require.NoError(t, db.Create(&protocol).Error)
		ResolveProtocol(db, &protocol)
		require.NotContains(t, *protocol.RealitySettings, "fake-planted")
		require.Contains(t, *protocol.RealitySettings, Placeholder)
		require.Equal(t, before, fallbacksTotal())
		require.NoError(t, db.Delete(&protocol).Error)
		require.NoError(t, db.Model(&model.Node{}).Where("id = ?", nodes[2].ID).
			UpdateColumns(map[string]any{"api_key": "", "api_key_hash": "", "secret": ""}).Error)

		// verify compares by presence once finalized.
		requireVerified(t, db)
		// A finalize of a finalized table changes nothing.
		again := finalizeAll(t, db)
		for _, result := range again {
			require.Zero(t, result.Tombstoned)
			require.Zero(t, result.Passes)
		}
		// The phase command never leaves finalized.
		_, err = SetPhase(ctx, db, PhaseOptions{Phase: PhaseDualWrite})
		require.ErrorIs(t, err, ErrPhaseRefused)
		var actions []string
		for _, entry := range audits(t, db) {
			actions = append(actions, entry.Action)
			require.NotContains(t, entry.Content, "fake-", "an audit entry holds no secret")
		}
		require.Contains(t, actions, "finalize_started")
		require.Contains(t, actions, "finalized")
	})
}

// TestFinalizeRespectsUniqueIndexes: a tombstone that would collide with a
// value another row holds is refused, the batch rolled back; the finalize
// resumes once the row is fixed.
func TestFinalizeRespectsUniqueIndexes(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		f := seedLegacy(t, db)
		// The keyless node holds the tombstone of the first node, as an
		// import or a package could have written it.
		taken := Tombstone(uint64(f.nodes[0].ID))
		require.NoError(t, db.Model(&model.Node{}).Where("id = ?", f.nodes[2].ID).UpdateColumn("api_key", taken).Error)
		// So does a clean agent's token of another agent's id.
		agent := model.ForwardCleanAgent{Name: "c3", Token: Tombstone(uint64(f.agents[0].ID))}
		require.NoError(t, db.Create(&agent).Error)
		toDualRead(t, db)

		results, err := Finalize(ctx, db, FinalizeOptions{Confirm: true, Tables: []string{TableNode}, BatchSize: 10})
		require.ErrorIs(t, err, ErrTombstoneCollision)
		require.Contains(t, err.Error(), "v2_node.api_key")
		require.NotContains(t, err.Error(), "fake-")
		require.Nil(t, results[0].FinalizedAt)
		var first model.Node
		require.NoError(t, db.First(&first, f.nodes[0].ID).Error)
		require.Equal(t, "fake-node-key-1", first.APIKey, "the batch rolled back")
		require.Equal(t, "fake-node-secret-1", first.Secret)
		state := splitState(t, db, TableNode)
		require.Equal(t, PhaseFinalized, state.Phase)
		require.Nil(t, state.FinalizedAt, "an interrupted finalize")
		// The readers already read only the new tables.
		require.True(t, NodeAPIKeyMatches(db, &first, "fake-node-key-1"))

		_, err = Finalize(ctx, db, FinalizeOptions{Confirm: true, Tables: []string{TableCleanAgent}})
		require.ErrorIs(t, err, ErrTombstoneCollision)

		// Fixed: the rows get values of their own; the finalize resumes,
		// without a new verification, and every tombstone is unique.
		require.NoError(t, db.Model(&model.Node{}).Where("id = ?", f.nodes[2].ID).UpdateColumn("api_key", "").Error)
		require.NoError(t, db.Model(&model.ForwardCleanAgent{}).Where("id = ?", agent.ID).UpdateColumn("token", "fake-clean-token-3").Error)
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return Sync(tx, TableCleanAgent, agent.ID) }))
		results, err = Finalize(ctx, db, FinalizeOptions{Confirm: true, Tables: []string{TableNode, TableCleanAgent},
			Now: time.Now().Add(3 * VerifyMaxAge)})
		require.NoError(t, err)
		for _, result := range results {
			require.True(t, result.Resumed, result.Table)
			require.NotNil(t, result.FinalizedAt)
		}
		var agents []model.ForwardCleanAgent
		require.NoError(t, db.Order("id").Find(&agents).Error)
		seen := map[string]bool{}
		for _, row := range agents {
			require.Equal(t, Tombstone(uint64(row.ID)), row.Token)
			require.False(t, seen[row.Token])
			seen[row.Token] = true
		}
		found, err := CleanAgentByToken(db, "fake-clean-token-3")
		require.NoError(t, err)
		require.Equal(t, agent.ID, found.ID)
	})
}

// TestFinalizeResumesAfterAnInterruption: a finalize stopped between two
// batches leaves the table finalized without finalized_at; the readers
// read only the new tables meanwhile, a writer's Sync writes tombstones,
// and running finalize again completes it.
func TestFinalizeResumesAfterAnInterruption(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		f := seedLegacy(t, db)
		toDualRead(t, db)
		want := f.read(t, db)
		stop := errors.New("interrupted")
		batches := 0
		_, err := Finalize(ctx, db, FinalizeOptions{Confirm: true, BatchSize: 1, afterBatch: func(string) error {
			batches++
			if batches == 1 {
				return stop
			}
			return nil
		}})
		require.ErrorIs(t, err, stop)
		var nodes []model.Node
		require.NoError(t, db.Order("id").Find(&nodes).Error)
		require.True(t, IsTombstone(nodes[0].APIKey), "the first batch committed")
		require.Equal(t, "fake-node-key-2", nodes[1].APIKey, "the second did not run")
		for _, table := range Tables() {
			state := splitState(t, db, table)
			require.Equal(t, PhaseFinalized, state.Phase, table)
			require.Nil(t, state.FinalizedAt, table)
		}
		// Meanwhile every reader still finds every secret.
		got := f.read(t, db)
		require.Equal(t, want.NodeByKey, got.NodeByKey)
		require.Equal(t, want.RegistrationKeys, got.RegistrationKeys)
		require.Equal(t, want.CleanByToken, got.CleanByToken)
		require.Equal(t, want.PeerPrivate, got.PeerPrivate)
		// A writer of a finalized table writes tombstones, not secrets.
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.Node{}).Where("id = ?", nodes[1].ID).Update("name", "n2-renamed").Error; err != nil {
				return err
			}
			return Sync(tx, TableNode, nodes[1].ID)
		}))
		require.NoError(t, db.First(&nodes[1], nodes[1].ID).Error)
		require.Equal(t, Tombstone(uint64(nodes[1].ID)), nodes[1].APIKey)

		results, err := Finalize(ctx, db, FinalizeOptions{Confirm: true, BatchSize: 1})
		require.NoError(t, err)
		for _, result := range results {
			require.True(t, result.Resumed)
			require.NotNil(t, result.FinalizedAt)
		}
		f.requireFinalizedColumns(t, db)
		requireVerified(t, db)
	})
}

// TestWritersOfAFinalizedTable: after finalize a writer's secret reaches
// the new tables only; a tombstone saved back keeps the secret; a cleared
// value removes it.
func TestWritersOfAFinalizedTable(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := seedLegacy(t, db)
		toDualRead(t, db)
		finalizeAll(t, db)
		write := func(table string, id uint, model any, updates map[string]any) {
			t.Helper()
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(model).Where("id = ?", id).UpdateColumns(updates).Error; err != nil {
					return err
				}
				return Sync(tx, table, id)
			}))
		}
		// A rotated key: the new version, and a tombstone in the column.
		node := f.nodes[0]
		write(TableNode, node.ID, &model.Node{}, map[string]any{"api_key": "fake-node-key-rotated", "api_key_hash": hashSecret("fake-node-key-rotated")})
		require.NoError(t, db.First(&node, node.ID).Error)
		require.Equal(t, Tombstone(uint64(node.ID)), node.APIKey)
		require.Empty(t, node.APIKeyHash)
		require.Equal(t, "fake-node-key-rotated", credential(t, db, SubjectProxy, node.ID, KindNodeAPIKey).Value)
		found, err := NodeByAPIKey(db, "fake-node-key-rotated", false)
		require.NoError(t, err)
		require.Equal(t, node.ID, found.ID)
		_, err = NodeByAPIKey(db, "fake-node-key-1", false)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		// A row loaded and saved back as it is keeps every secret.
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(&node).Error; err != nil {
				return err
			}
			return Sync(tx, TableNode, node.ID)
		}))
		require.Equal(t, "fake-node-secret-1", credential(t, db, SubjectProxy, node.ID, KindNodeSharedSecret).Value)
		equalJSON(t, f.rawConfig, func() string { n := node; ResolveNodeRawConfig(db, &n); return *n.RawConfig }())
		// A cleared forward token is gone.
		write(TableForwardNode, f.forwards[0].ID, &model.ForwardNode{}, map[string]any{"api_token": ""})
		var forward model.ForwardNode
		require.NoError(t, db.First(&forward, f.forwards[0].ID).Error)
		require.Empty(t, ForwardNodeToken(db, &forward))
		// A protocol written with a new literal secret: redacted in the
		// column, the value in the new table.
		reality := `{"short_id":"02","private_key":"fake-reality-rotated"}`
		write(TableNodeProtocol, f.protocols[0].ID, &model.NodeProtocol{}, map[string]any{"reality_settings": reality})
		var protocol model.NodeProtocol
		require.NoError(t, db.First(&protocol, f.protocols[0].ID).Error)
		require.Equal(t, Redact(reality), *protocol.RealitySettings)
		ResolveProtocol(db, &protocol)
		equalJSON(t, reality, *protocol.RealitySettings)
		// A document sent back with the placeholder keeps the secret.
		write(TableNodeProtocol, f.protocols[0].ID, &model.NodeProtocol{}, map[string]any{"reality_settings": `{"private_key":"********","short_id":"03"}`})
		require.NoError(t, db.First(&protocol, f.protocols[0].ID).Error)
		ResolveProtocol(db, &protocol)
		equalJSON(t, `{"private_key":"fake-reality-rotated","short_id":"03"}`, *protocol.RealitySettings)
		requireVerified(t, db)
	})
}

// TestOlderBinaryFailsClosedAfterFinalize: a binary that reads the legacy
// columns (the legacy rule of every reader, as in phase dual_write)
// authenticates no one after finalize, neither with the real secrets nor
// with a tombstone, and finds no secret to present.
func TestOlderBinaryFailsClosedAfterFinalize(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := seedLegacy(t, db)
		toDualRead(t, db)
		finalizeAll(t, db)
		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("1 = 1").Update("phase", PhaseDualWrite).Error)
		ForgetPhases(db)
		got := f.read(t, db)
		require.Equal(t, []bool{false, false}, got.NodeKeyMatches)
		require.Equal(t, []uint{0, 0}, got.NodeByKey)
		require.Equal(t, []uint{0, 0}, got.RegistrationKeys)
		require.False(t, got.ForwardMatches)
		require.Equal(t, []uint{0, 0}, got.CleanByToken)
		require.Equal(t, []uint{0, 0}, got.CleanWithToken)
		for _, value := range append(append(got.NodeKeys, got.NodeSecrets...), got.ForwardToken) {
			require.True(t, Unusable(value), "%q", value)
		}
		for _, value := range append(got.PeerPrivate, got.PeerPreshared...) {
			require.True(t, IsTombstone(value))
		}
		require.NotContains(t, got.Reality, "fake-reality-private")
		require.NotContains(t, got.RawConfig, "fake-raw-private")
		var nodes []model.Node
		require.NoError(t, db.Order("id").Find(&nodes).Error)
		for _, node := range nodes[:2] {
			require.False(t, NodeAPIKeyMatches(db, &node, node.APIKey))
			_, err := NodeByAPIKey(db, node.APIKey, true)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		}
		_, err := CleanAgentByToken(db, Tombstone(uint64(f.agents[0].ID)))
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})
}

// TestUnsplitRoundTrip: unsplit writes every legacy column back byte for
// byte (the original JSON documents, the empty hash of an old row), drops
// the kept originals, returns the tables to dual_read and verifies them.
func TestUnsplitRoundTrip(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		f := seedLegacy(t, db)
		// A raw configuration as an administrator typed it: spaces and key
		// order a re-encoding would not keep.
		typed := "{ \"wireguard\": {\"public_key\": \"fake-raw-public\", \"private_key\": \"fake-raw-private\"},\n  \"server_port\": 51820 }"
		require.NoError(t, db.Model(&model.Node{}).Where("id = ?", f.nodes[0].ID).UpdateColumn("raw_config", typed).Error)
		f.rawConfig = typed
		toDualRead(t, db)
		original := allLegacyRows(t, db)
		want := f.read(t, db)

		finalizeAll(t, db)
		require.NotEqual(t, original, allLegacyRows(t, db))
		dropped := []string{}
		results, err := Unsplit(ctx, db, UnsplitOptions{Confirm: true, Actor: "test", BatchSize: 1,
			DropViews: func(_ *gorm.DB, table string) error { dropped = append(dropped, table); return nil }})
		require.NoError(t, err, "%+v", results)
		require.ElementsMatch(t, Tables(), dropped)
		for _, result := range results {
			require.Equal(t, PhaseFinalized, result.From)
			require.Zero(t, result.Unresolved, result.Table)
			require.NotNil(t, result.Verify)
			require.True(t, result.Verify.Match, result.Table)
			state := splitState(t, db, result.Table)
			require.Equal(t, PhaseDualRead, state.Phase)
			require.Nil(t, state.FinalizedAt)
		}
		require.Equal(t, original, allLegacyRows(t, db), "every legacy column byte for byte")
		var kept int64
		require.NoError(t, db.Model(&model.ProtocolSecret{}).Where("scope = ?", scopeLegacyOriginal).Count(&kept).Error)
		require.Zero(t, kept)
		require.Equal(t, want, f.read(t, db))
		requireVerified(t, db)
		// The way back on: dual_write reads the restored columns.
		setPhase(t, db, PhaseDualWrite)
		require.Equal(t, want, f.read(t, db))
		// And forth again.
		setPhase(t, db, PhaseDualRead)
		finalizeAll(t, db)
		f.requireFinalizedColumns(t, db)
	})
}

// TestUnsplitRefusals: unsplit needs the confirmation, a finalized (or
// dual_read) table, and refuses a table a package adopted unless told.
func TestUnsplitRefusals(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		seedLegacy(t, db)
		_, err := Backfill(ctx, db, BackfillOptions{})
		require.NoError(t, err)
		results, err := Unsplit(ctx, db, UnsplitOptions{Confirm: true})
		require.ErrorIs(t, err, ErrUnsplitRefused)
		require.Contains(t, results[0].Refused, "unsplit needs phase finalized")

		requireVerified(t, db)
		setPhase(t, db, PhaseDualRead)
		finalizeAll(t, db)
		_, err = Unsplit(ctx, db, UnsplitOptions{})
		require.ErrorIs(t, err, ErrUnsplitRefused)
		require.NoError(t, db.AutoMigrate(&model.PackageStorage{}))
		require.NoError(t, db.Create(&model.PackageStorage{PackageID: "proxy-node", Driver: "sqlite", PackageVersion: "4.2.0",
			GrantsJSON: `{"adopt_tables":["v2_node"],"views":[]}`}).Error)
		results, err = Unsplit(ctx, db, UnsplitOptions{Confirm: true, Tables: []string{TableNode}})
		require.ErrorIs(t, err, ErrUnsplitRefused)
		require.Contains(t, results[0].Refused, "proxy-node")
		require.Equal(t, PhaseFinalized, splitState(t, db, TableNode).Phase)
		_, err = Unsplit(ctx, db, UnsplitOptions{Confirm: true, Tables: []string{TableNode}, AllowAdopted: true})
		require.NoError(t, err)
		require.Equal(t, PhaseDualRead, splitState(t, db, TableNode).Phase)
	})
}

// TestAdoptionAllowedOnlyOnceFinalized is the capability check of section
// 4.6.
func TestAdoptionAllowedOnlyOnceFinalized(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		seedLegacy(t, db)
		require.NoError(t, AdoptionAllowed(db, "v2_knowledge"), "not a split table")
		require.ErrorContains(t, AdoptionAllowed(db, TableNode), "phase dual_write")
		toDualRead(t, db)
		require.ErrorContains(t, AdoptionAllowed(db, TableNode), "phase dual_read")
		_, err := Finalize(ctx, db, FinalizeOptions{Confirm: true, Tables: []string{TableNode}, afterBatch: func(string) error {
			return errors.New("interrupted")
		}})
		require.Error(t, err)
		require.ErrorContains(t, AdoptionAllowed(db, TableNode), "finalizing")
		tables, err := FinalizedTables(db)
		require.NoError(t, err)
		require.Empty(t, tables)
		_, err = Finalize(ctx, db, FinalizeOptions{Confirm: true, Tables: []string{TableNode}})
		require.NoError(t, err)
		require.NoError(t, AdoptionAllowed(db, TableNode))
		require.Error(t, AdoptionAllowed(db, TableNodeProtocol))
		tables, err = FinalizedTables(db)
		require.NoError(t, err)
		require.Equal(t, []string{TableNode}, tables)
	})
}
