package service

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPluginRuntimeSelection(t *testing.T) {
	db := newKernelTestDB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&model.Plugin{ID: "knowledge", Name: "Knowledge", Publisher: "AnixOps", Official: true}).Error)
	remote, err := PluginRuntimeIsRemote(db)(ctx, "knowledge")
	require.NoError(t, err)
	require.False(t, remote, "packages run locally by default")

	row, err := SetPluginRuntime(ctx, db, "knowledge", model.PluginRuntimeLocal, false, 7)
	require.NoError(t, err)
	require.Equal(t, model.PluginRuntimeLocal, row.Runtime)
	require.EqualValues(t, 7, row.UpdatedBy)

	_, err = SetPluginRuntime(ctx, db, "knowledge", model.PluginRuntimeRemote, false, 7)
	require.ErrorIs(t, err, ErrRemoteRuntimeDisabled)
	_, err = SetPluginRuntime(ctx, db, "knowledge", model.PluginRuntimeRemote, true, 7)
	require.ErrorContains(t, err, "requires PostgreSQL", "module instances cannot share a SQLite file")
	_, err = SetPluginRuntime(ctx, db, "knowledge", "cloud", true, 7)
	require.ErrorContains(t, err, "runtime must be")
	_, err = SetPluginRuntime(ctx, db, "missing-plugin", model.PluginRuntimeLocal, true, 7)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = SetPluginRuntime(ctx, db, "../etc", model.PluginRuntimeLocal, true, 7)
	require.Error(t, err)

	// A remote row (written directly, as on PostgreSQL) selects remote.
	require.NoError(t, db.Save(&model.PluginRuntime{PluginID: "knowledge", Runtime: model.PluginRuntimeRemote}).Error)
	remote, err = PluginRuntimeIsRemote(db)(ctx, "knowledge")
	require.NoError(t, err)
	require.True(t, remote)
}
