package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func moduleCommandFixture(t *testing.T) (*config.Config, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ServiceCA{}, &model.ForwardLinkCA{}))
	cfg := &config.Config{ModuleRuntime: config.ModuleRuntimeConfig{
		Cluster: "prod", CAKEK: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
	}}
	require.NoError(t, ensureModulePKI(context.Background(), cfg, db))
	return cfg, db
}

func TestModuleCommandRotatesTheCA(t *testing.T) {
	cfg, db := moduleCommandFixture(t)
	ctx := context.Background()
	var output bytes.Buffer
	require.NoError(t, runModuleCommand(ctx, cfg, db, []string{"ca", "rotate"}, &output))
	require.Contains(t, output.String(), `"state": "next"`)
	output.Reset()
	require.NoError(t, runModuleCommand(ctx, cfg, db, []string{"ca", "bundle"}, &output))
	require.Equal(t, 2, strings.Count(output.String(), "BEGIN CERTIFICATE"), "current and next CA")

	for _, arguments := range [][]string{{"ca"}, {"token", "list"}, {"runtime", "list"}, {"ca", "bogus"}} {
		require.Error(t, runModuleCommand(ctx, cfg, db, arguments, &output), arguments)
	}
}

func TestModulePKIBootstrapIsOptional(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, ensureModulePKI(context.Background(), &config.Config{}, db), "disabled runtime creates nothing")
	err = runModuleCommand(context.Background(), &config.Config{}, db, []string{"ca", "bundle"}, &bytes.Buffer{})
	require.True(t, errors.Is(err, modulepki.ErrBuiltinPKIDisabled))

	cfg := &config.Config{ModuleRuntime: config.ModuleRuntimeConfig{CAKEK: "short"}}
	require.Error(t, ensureModulePKI(context.Background(), cfg, db))
}
