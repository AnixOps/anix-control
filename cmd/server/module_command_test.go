package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func moduleCommandFixture(t *testing.T) (*config.Config, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ServiceCA{}, &model.ModuleEnrollment{}, &model.ModuleCertificate{}, &model.ForwardLinkCA{}))
	cfg := &config.Config{ModuleRuntime: config.ModuleRuntimeConfig{
		Enabled: true, Cluster: "prod", CAKEK: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
	}}
	require.NoError(t, ensureModulePKI(context.Background(), cfg, db))
	return cfg, db
}

func TestModuleCommandManagesEnrollments(t *testing.T) {
	cfg, db := moduleCommandFixture(t)
	ctx := context.Background()
	var output bytes.Buffer
	require.NoError(t, runModuleCommand(ctx, cfg, db, []string{"token", "create", "-package", "identity-platform", "-ttl", "2h", "-reusable"}, &output))
	var created struct {
		Enrollment model.ModuleEnrollment `json:"enrollment"`
		Credential string                 `json:"credential"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &created))
	require.True(t, strings.HasPrefix(created.Credential, "anixenr_"))
	require.True(t, created.Enrollment.Reusable)

	output.Reset()
	require.NoError(t, runModuleCommand(ctx, cfg, db, []string{"token", "list"}, &output))
	require.Contains(t, output.String(), created.Enrollment.ID)
	require.NotContains(t, output.String(), created.Credential)

	require.NoError(t, runModuleCommand(ctx, cfg, db, []string{"token", "revoke", created.Enrollment.ID}, &output))
	output.Reset()
	require.NoError(t, runModuleCommand(ctx, cfg, db, []string{"ca", "rotate"}, &output))
	require.Contains(t, output.String(), `"state": "next"`)
	output.Reset()
	require.NoError(t, runModuleCommand(ctx, cfg, db, []string{"ca", "bundle"}, &output))
	require.Equal(t, 2, strings.Count(output.String(), "BEGIN CERTIFICATE"), "current and next CA")

	for _, arguments := range [][]string{{"token"}, {"token", "revoke"}, {"bogus", "command"}, {"token", "create", "-nope"}} {
		require.Error(t, runModuleCommand(ctx, cfg, db, arguments, &output), arguments)
	}
}

func TestModulePKIBootstrapIsOptional(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, ensureModulePKI(context.Background(), &config.Config{}, db), "disabled runtime creates nothing")
	err = runModuleCommand(context.Background(), &config.Config{}, db, []string{"token", "list"}, &bytes.Buffer{})
	require.True(t, errors.Is(err, modulepki.ErrBuiltinPKIDisabled))

	cfg := &config.Config{ModuleRuntime: config.ModuleRuntimeConfig{Enabled: true, CAKEK: "short"}}
	require.Error(t, ensureModulePKI(context.Background(), cfg, db))
}

func TestModuleCommandSelectsRuntimes(t *testing.T) {
	cfg, db := moduleCommandFixture(t)
	require.NoError(t, db.AutoMigrate(&model.Plugin{}, &model.PluginRuntime{}))
	require.NoError(t, db.Create(&model.Plugin{ID: "identity-platform", Name: "Identity", Publisher: "AnixOps", Official: true}).Error)
	ctx := context.Background()
	var output bytes.Buffer
	require.NoError(t, runModuleCommand(ctx, cfg, db, []string{"runtime", "set", "identity-platform", "local"}, &output))
	require.Contains(t, output.String(), `"runtime": "local"`)
	err := runModuleCommand(ctx, cfg, db, []string{"runtime", "set", "identity-platform", "remote"}, &output)
	require.ErrorContains(t, err, "requires PostgreSQL")
	output.Reset()
	require.NoError(t, runModuleCommand(ctx, cfg, db, []string{"runtime", "list"}, &output))
	require.Contains(t, output.String(), `"plugin_id": "identity-platform"`)
	require.Error(t, runModuleCommand(ctx, cfg, db, []string{"runtime", "set", "identity-platform"}, &output))
}
