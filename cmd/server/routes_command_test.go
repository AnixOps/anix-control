package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/routemodefixture"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRoutesCommandSetsAndRollsBackRouteModes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, service.EnsureKernelSchema(db))
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.OperationLog{}))
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	// knowledge.article.list and knowledge.admin.knowledge.post are
	// native-flagged in config/package-extraction.json.
	routemodefixture.Install(t, db, privateKey, "knowledge", []routemodefixture.Route{
		routemodefixture.HTTP("GET", "/api/v2/user/knowledge", "knowledge.article.list"),
		routemodefixture.HTTP("POST", "/api/v2/admin/knowledge", "knowledge.admin.knowledge.post"),
	})
	cfg := &config.Config{Plugins: config.PluginConfig{OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey)}}
	ctx := context.Background()
	run := func(arguments ...string) (string, error) {
		var output bytes.Buffer
		err := runAdminCommand(ctx, cfg, db, append([]string{"routes"}, arguments...), &output)
		return output.String(), err
	}

	for _, arguments := range [][]string{
		{}, {"bogus"}, {"list", "extra"}, {"set", "--mode", "shadow"}, {"set", "--package", "knowledge"},
		{"rollback"}, {"history", "--mode", "native"},
	} {
		_, err := run(arguments...)
		require.Error(t, err, arguments)
	}
	_, err = run("set", "--package", "knowledge", "--mode", "native", "--reason", "batch 1")
	require.ErrorContains(t, err, "--reason and --yes")
	_, err = run("set", "--package", "knowledge", "--mode", "native", "--yes")
	require.ErrorContains(t, err, "--reason and --yes")
	_, err = run("set", "--package", "knowledge", "--route", "knowledge.admin.knowledge.post", "--mode", "shadow")
	require.ErrorIs(t, err, service.ErrRouteModeRejected)

	output, err := run("list")
	require.NoError(t, err)
	require.Contains(t, output, "knowledge.article.list")
	require.Contains(t, output, "legacy,shadow,native")

	output, err = run("set", "--package", "knowledge", "--route", "knowledge.article.list", "--mode", "shadow")
	require.NoError(t, err)
	require.Contains(t, output, "knowledge.article.list: legacy -> shadow")
	output, err = run("set", "--package", "knowledge", "--mode", "native", "--reason", "shadow clean", "--yes", "--json")
	require.NoError(t, err)
	var result service.RouteModeResult
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	require.Len(t, result.Changes, 2)

	output, err = run("list", "--package", "knowledge", "--json")
	require.NoError(t, err)
	var packages []service.PackageRouteModes
	require.NoError(t, json.Unmarshal([]byte(output), &packages))
	require.Len(t, packages, 1)
	for _, route := range packages[0].Routes {
		require.Equal(t, "native", route.Configured, route.RouteID)
	}

	output, err = run("rollback", "--package", "knowledge", "--reason", "mismatch in production")
	require.NoError(t, err)
	require.Contains(t, output, "2 route(s) changed")

	output, err = run("history", "--package", "knowledge")
	require.NoError(t, err)
	require.Contains(t, output, "mismatch in production")
	output, err = run("history", "--json")
	require.NoError(t, err)
	var revisions []model.RouteModeRevision
	require.NoError(t, json.Unmarshal([]byte(output), &revisions))
	require.Len(t, revisions, 5)
	for _, revision := range revisions {
		require.Equal(t, service.RouteModeCLIActor, revision.Actor)
		require.Zero(t, revision.ActorUserID)
	}

	var audits []model.OperationLog
	require.NoError(t, db.Find(&audits).Error)
	require.Len(t, audits, 3)
	for _, audit := range audits {
		require.Equal(t, service.RouteModeCLIActor, audit.Username)
		require.Nil(t, audit.UserID)
	}
}
