package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNodeSecretsCommand(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.WireGuardPeer{},
		&model.ForwardNode{}))
	raw := `{"server_port":51820,"wireguard":{"private_key":"fake-cli-private"}}`
	require.NoError(t, db.Create(&model.Node{Name: "n", Host: "203.0.113.40", RawConfig: &raw}).Error)
	require.NoError(t, nodesecrets.SeedForTest(db))
	ctx := context.Background()

	var output bytes.Buffer
	require.NoError(t, runNodeSecretsCommand(ctx, db, []string{"status"}, &output))
	var status struct {
		Tables []model.NodeSecretSplit `json:"tables"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &status))
	require.Len(t, status.Tables, len(nodesecrets.Tables()))
	for _, table := range status.Tables {
		require.Equal(t, nodesecrets.PhaseFinalized, table.Phase)
		require.NotNil(t, table.FinalizedAt)
	}
	require.NotContains(t, output.String(), "fake-", "status prints no secret")

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

	// The commands of the phases before finalize are gone.
	for _, arguments := range [][]string{
		{}, {"status", "extra"}, {"validate", "extra"}, {"backfill"}, {"verify"}, {"phase", "all", "dual_read"},
		{"finalize", "-confirm", "all"}, {"unsplit", "-confirm", "all"},
	} {
		require.Error(t, runNodeSecretsCommand(ctx, db, arguments, &output), "%v", arguments)
	}
}
