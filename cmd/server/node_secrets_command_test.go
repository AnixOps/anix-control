package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNodeSecretsCommand(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.WireGuardPeer{}, &model.AuthorizedKey{},
		&model.ForwardNode{}, &model.ForwardCleanAgent{}))
	require.NoError(t, nodesecrets.EnsureSchema(db))
	require.NoError(t, db.Create(&model.Node{Name: "n", Host: "203.0.113.40", APIKey: "fake-cli-key", Secret: "fake-cli-secret"}).Error)
	ctx := context.Background()

	var output bytes.Buffer
	require.NoError(t, runNodeSecretsCommand(ctx, db, []string{"status"}, &output))
	var splits []model.NodeSecretSplit
	require.NoError(t, json.Unmarshal(output.Bytes(), &splits))
	require.Len(t, splits, len(nodesecrets.Tables()))
	require.Equal(t, nodesecrets.PhaseDualWrite, splits[0].Phase)

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

	for _, arguments := range [][]string{
		{}, {"finalize"}, {"status", "extra"}, {"backfill", "-batch", "0"}, {"verify", "extra"},
	} {
		require.Error(t, runNodeSecretsCommand(ctx, db, arguments, &output), "%v", arguments)
	}
	require.Error(t, runNodeSecretsCommand(ctx, db, []string{"backfill", "-table", "v2_user"}, &output))
}
