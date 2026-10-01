package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNodeSecretsCommand(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.WireGuardPeer{}, &model.AuthorizedKey{},
		&model.ForwardNode{}, &model.ForwardCleanAgent{}, &model.OperationLog{}))
	require.NoError(t, nodesecrets.EnsureSchema(db))
	require.NoError(t, db.Create(&model.Node{Name: "n", Host: "203.0.113.40", APIKey: "fake-cli-key", Secret: "fake-cli-secret"}).Error)
	require.NoError(t, db.Create(&[]model.ForwardNode{
		{ID: 3, Name: "no-port", Host: "203.0.113.41", Port: 443, APIToken: "fake-cli-relay-token"},
		{ID: 4, Name: "pinned", Host: "203.0.113.42", Port: 443, APIPort: 18080, APIToken: "fake-cli-relay-token-2"},
	}).Error)
	ctx := context.Background()

	var output bytes.Buffer
	require.NoError(t, runNodeSecretsCommand(ctx, db, []string{"status"}, &output))
	var status struct {
		Tables   []model.NodeSecretSplit `json:"tables"`
		Unpinned struct {
			Count int                               `json:"count"`
			Nodes []nodesecrets.UnpinnedForwardNode `json:"nodes"`
		} `json:"forward_nodes_without_api_port"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &status))
	require.Len(t, status.Tables, len(nodesecrets.Tables()))
	require.Equal(t, nodesecrets.PhaseDualWrite, status.Tables[0].Phase)
	require.Equal(t, 1, status.Unpinned.Count)
	require.Equal(t, []nodesecrets.UnpinnedForwardNode{{ID: 3, Name: "no-port"}}, status.Unpinned.Nodes)
	require.NotContains(t, output.String(), "fake-cli-relay-token", "status prints no token")

	// Before any verification the readers may not move to dual_read.
	output.Reset()
	require.ErrorIs(t, runNodeSecretsCommand(ctx, db, []string{"phase", "all", "dual_read"}, &output), nodesecrets.ErrPhaseRefused)
	require.Contains(t, output.String(), "no verification has matched")

	// Before the backfill the forms differ: exit 3, with the report.
	output.Reset()
	require.ErrorIs(t, runNodeSecretsCommand(ctx, db, []string{"verify", "-table", "v2_node"}, &output), errNodeSecretsMismatch)
	require.Contains(t, output.String(), `"missing": 2`)
	require.NotContains(t, output.String(), "fake-")

	output.Reset()
	require.NoError(t, runNodeSecretsCommand(ctx, db, []string{"backfill", "-batch", "10"}, &output))
	var backfilled []nodesecrets.TableBackfill
	require.NoError(t, json.Unmarshal(output.Bytes(), &backfilled))
	require.Len(t, backfilled, len(nodesecrets.Tables()))
	require.EqualValues(t, 2, backfilled[0].Changed)
	require.NotContains(t, output.String(), "fake-")

	output.Reset()
	require.NoError(t, runNodeSecretsCommand(ctx, db, []string{"verify"}, &output))
	var verified []nodesecrets.TableVerify
	require.NoError(t, json.Unmarshal(output.Bytes(), &verified))
	for _, result := range verified {
		require.True(t, result.Match, result.Table)
	}
	require.NotContains(t, output.String(), "fake-")

	// Verified: the readers move to dual_read, and back, each change
	// audited with who made it.
	output.Reset()
	require.NoError(t, runNodeSecretsCommand(ctx, db, []string{"phase", "-by", "ops", "v2_node,v2_authorized_key", "dual_read"}, &output))
	var changes []nodesecrets.PhaseChange
	require.NoError(t, json.Unmarshal(output.Bytes(), &changes))
	require.Len(t, changes, 2)
	require.True(t, changes[0].Changed)
	require.Equal(t, nodesecrets.PhaseDualRead, changes[0].To)
	require.Equal(t, nodesecrets.PhaseDualRead, nodesecrets.ReadPhase(db, nodesecrets.TableNode))
	output.Reset()
	require.NoError(t, runNodeSecretsCommand(ctx, db, []string{"phase", "all", "dual_write"}, &output))
	require.Equal(t, nodesecrets.PhaseDualWrite, nodesecrets.ReadPhase(db, nodesecrets.TableNode))
	var audits []model.OperationLog
	require.NoError(t, db.Where("module = ?", "node_secrets").Order("id").Find(&audits).Error)
	require.Len(t, audits, 4)
	require.Equal(t, "ops", audits[0].Username)
	require.Equal(t, "phase_dual_write", audits[3].Action)

	// validate reports a masked key, exits 3 and prints no value.
	reality := `{"private_key":"********","short_id":"01"}`
	require.NoError(t, db.Create(&model.NodeProtocol{NodeID: 1, Name: "masked", Type: model.ProtocolVLESS, Port: 443, TLS: 2, RealitySettings: &reality}).Error)
	output.Reset()
	require.ErrorIs(t, runNodeSecretsCommand(ctx, db, []string{"validate"}, &output), errNodeSecretsInvalid)
	var report service.NodeSecretValidation
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	require.Len(t, report.Findings, 1)
	require.Equal(t, "/private_key", report.Findings[0].Field)
	require.Zero(t, report.Excluded)
	require.NotContains(t, output.String(), "fake-")

	for _, arguments := range [][]string{
		{}, {"finalize"}, {"unsplit"}, {"finalize", "-confirm"}, {"finalize", "-confirm", "-adopted-ok", "all"},
		{"finalize", "-confirm", "all", "extra"}, {"unsplit", "-confirm", ","}, {"status", "extra"}, {"backfill", "-batch", "0"}, {"verify", "extra"},
		{"phase"}, {"phase", "all"}, {"phase", "all", "finalized"}, {"phase", "all", "legacy"}, {"phase", "all", "dual_read", "extra"},
		{"phase", ",", "dual_read"}, {"validate", "extra"},
	} {
		require.Error(t, runNodeSecretsCommand(ctx, db, arguments, &output), "%v", arguments)
	}
	require.Error(t, runNodeSecretsCommand(ctx, db, []string{"backfill", "-table", "v2_user"}, &output))
}

// TestNodeSecretsFinalizeAndUnsplitCommands: finalize needs -confirm and
// dual_read, writes the tombstones and creates the views of the finalized
// remainder; unsplit writes the secrets back and drops them. Nothing is
// printed but counts.
func TestNodeSecretsFinalizeAndUnsplitCommands(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")+"?_txlock=immediate"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.WireGuardPeer{}, &model.AuthorizedKey{},
		&model.ForwardNode{}, &model.ForwardCleanAgent{}, &model.OperationLog{}))
	require.NoError(t, nodesecrets.EnsureSchema(db))
	node := model.Node{Name: "n", Host: "203.0.113.40", APIKey: "fake-cli-key", Secret: "fake-cli-secret"}
	require.NoError(t, db.Create(&node).Error)
	ctx := context.Background()
	var output bytes.Buffer
	run := func(arguments ...string) error {
		output.Reset()
		return runNodeSecretsCommand(ctx, db, arguments, &output)
	}
	viewExists := func(name string) bool {
		var count int64
		require.NoError(t, db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'view' AND name = ?", name).Scan(&count).Error)
		return count > 0
	}

	require.NoError(t, run("backfill"))
	require.NoError(t, run("verify"))
	require.ErrorIs(t, run("finalize", "-confirm", "v2_node"), nodesecrets.ErrFinalizeRefused)
	require.Contains(t, output.String(), "finalize needs phase dual_read")
	require.NoError(t, run("phase", "v2_node", "dual_read"))
	require.ErrorIs(t, run("finalize", "v2_node"), nodesecrets.ErrFinalizeRefused)
	require.Contains(t, output.String(), "confirm it")
	require.False(t, viewExists("kapi_node_public_v1"))

	require.NoError(t, run("finalize", "-confirm", "-by", "ops", "-batch", "1", "v2_node"))
	var finalized []nodesecrets.TableFinalize
	require.NoError(t, json.Unmarshal(output.Bytes(), &finalized))
	require.Len(t, finalized, 1)
	require.NotNil(t, finalized[0].FinalizedAt)
	require.NotContains(t, output.String(), "fake-")
	require.True(t, viewExists("kapi_node_public_v1"))
	require.True(t, viewExists("kapi_node_credential_status_v1"))
	var stored model.Node
	require.NoError(t, db.First(&stored, node.ID).Error)
	require.Equal(t, nodesecrets.Tombstone(uint64(node.ID)), stored.APIKey)

	require.Error(t, run("unsplit", "v2_node"))
	require.NoError(t, run("unsplit", "-confirm", "v2_node"))
	var unsplit []nodesecrets.TableUnsplit
	require.NoError(t, json.Unmarshal(output.Bytes(), &unsplit))
	require.True(t, unsplit[0].Verify.Match)
	require.NotContains(t, output.String(), "fake-")
	require.False(t, viewExists("kapi_node_public_v1"))
	require.NoError(t, db.First(&stored, node.ID).Error)
	require.Equal(t, "fake-cli-key", stored.APIKey)
	require.Equal(t, "fake-cli-secret", stored.Secret)
}
