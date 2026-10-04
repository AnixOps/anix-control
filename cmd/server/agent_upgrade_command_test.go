package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agentinstall"
	"github.com/AnixOps/anix-control/v4/internal/agentupgrade"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAgentUpgradeCommand(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sign := func(data []byte) []byte {
		return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(private, data)) + "\n")
	}
	artifacts := t.TempDir()
	release := filepath.Join(artifacts, "v4.2.0")
	require.NoError(t, os.MkdirAll(release, 0o750))
	var sums []byte
	for _, asset := range agentinstall.Assets {
		data := []byte("fake agent zip " + asset)
		require.NoError(t, os.WriteFile(filepath.Join(release, asset), data, 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(release, asset+".sig"), sign(data), 0o600))
		digest := sha256.Sum256(data)
		sums = append(sums, []byte(hex.EncodeToString(digest[:])+"  "+asset+"\n")...)
	}
	require.NoError(t, os.WriteFile(filepath.Join(release, "SHA256SUMS"), sums, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(release, "SHA256SUMS.sig"), sign(sums), 0o600))
	cfg := &config.Config{}
	cfg.Plugins.OfficialPublicKey = base64.StdEncoding.EncodeToString(public)
	cfg.AgentInstall.ArtifactDir = artifacts
	cfg.AgentInstall.AgentVersion = "v4.2.0"
	previous := handler.ReleaseVersion
	handler.ReleaseVersion = "4.2.0"
	t.Cleanup(func() { handler.ReleaseVersion = previous })

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(append(model.AgentUpgradeModels(), &model.Node{}, &model.ForwardNode{}, &model.AgentTransport{}, &model.OperationLog{})...))
	for id := uint(1); id <= 3; id++ {
		require.NoError(t, db.Create(&model.Node{ID: id, Name: "proxy", APIKey: "key-" + strconv.Itoa(int(id)), Status: model.NodeStatusOnline}).Error)
		now := time.Now()
		require.NoError(t, db.Create(&model.AgentTransport{NodeKind: "proxy", NodeID: id, Transport: model.AgentTransportMTLSStream, FirstSeenAt: now, LastSeenAt: now}).Error)
	}
	ctx := context.Background()
	var output bytes.Buffer
	read := func() agentupgrade.CampaignView {
		t.Helper()
		var view agentupgrade.CampaignView
		require.NoError(t, json.Unmarshal(output.Bytes(), &view), output.String())
		output.Reset()
		return view
	}

	require.ErrorContains(t, runAdminCommand(ctx, cfg, db, []string{"agent", "upgrade", "status"}, &output), "no Agent upgrade campaign yet")
	require.ErrorContains(t, runAdminCommand(ctx, cfg, db, []string{"agent", "upgrade", "start"}, &output), "agent_install.public_url")
	require.NoError(t, runAdminCommand(ctx, cfg, db, []string{"agent", "upgrade", "start", "-control", "https://panel.example.com",
		"-exclude", "proxy-2", "-reason", "rollout"}, &output))
	started := read()
	require.Equal(t, "v4.2.0", started.TargetVersion)
	require.Equal(t, model.AgentUpgradeRunning, started.Status)
	require.Equal(t, 2, started.Total)
	require.Equal(t, "cli", started.Actor)
	require.Equal(t, []string{"proxy-2"}, started.Exclude.Nodes)

	require.NoError(t, runAdminCommand(ctx, cfg, db, []string{"agent", "upgrade", "status"}, &output))
	require.Equal(t, started.ID, read().ID)
	require.NoError(t, runAdminCommand(ctx, cfg, db, []string{"agent", "upgrade", "pause", "-id", started.ID}, &output))
	require.Equal(t, model.AgentUpgradePaused, read().Status)
	require.NoError(t, runAdminCommand(ctx, cfg, db, []string{"agent", "upgrade", "resume", "-id", started.ID}, &output))
	require.Equal(t, model.AgentUpgradeRunning, read().Status)
	require.NoError(t, runAdminCommand(ctx, cfg, db, []string{"agent", "upgrade", "abort", "-id", started.ID}, &output))
	require.Equal(t, model.AgentUpgradeAborted, read().Status)
	require.NoError(t, runAdminCommand(ctx, cfg, db, []string{"agent", "upgrade", "status", "-id", started.ID}, &output))
	require.Equal(t, model.AgentUpgradeAborted, read().Status)

	for _, arguments := range [][]string{
		{"upgrade", "pause"}, {"upgrade", "bogus"}, {"upgrade", "abort", "-id", started.ID, "extra"},
		{"upgrade", "resume", "-id", started.ID}, {"upgrade", "start", "-control", "https://panel.example.com", "-version", "v4.3.0"},
		{"upgrade", "status", "-id", "missing"},
	} {
		require.Error(t, runAgentCommand(ctx, cfg, db, arguments, &output), arguments)
	}
}
