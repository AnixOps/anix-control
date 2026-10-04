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
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAgentCommandCreatesEnrollmentCredentials(t *testing.T) {
	cfg, db := moduleCommandFixture(t)
	require.NoError(t, db.AutoMigrate(&model.AgentEnrollment{}, &model.AgentCertificate{}, &model.Node{}, &model.ForwardNode{}, &model.OperationLog{}))
	node := model.Node{Name: "proxy-12", APIKeyHash: strings.Repeat("b", 64), Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&node).Error)
	nodeName := "proxy-" + strconv.FormatUint(uint64(node.ID), 10)
	ctx := context.Background()

	var output bytes.Buffer
	require.NoError(t, runAdminCommand(ctx, cfg, db, []string{"agent", "token", "create", "-node", nodeName, "-ttl", "2h"}, &output))
	var created struct {
		Enrollment model.AgentEnrollment `json:"enrollment"`
		Credential string                `json:"credential"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &created))
	require.True(t, strings.HasPrefix(created.Credential, "anixagt_"))
	require.Equal(t, "proxy", created.Enrollment.NodeKind)
	require.Equal(t, node.ID, created.Enrollment.NodeID)
	require.Equal(t, model.AgentEnrollmentMethodCredential, created.Enrollment.Method)
	var audit model.OperationLog
	require.NoError(t, db.Where("action = ?", agentpki.AuditActionTokenIssue).First(&audit).Error)
	require.Equal(t, "cli", audit.Username)

	for _, arguments := range [][]string{
		{"token"}, {"token", "list"}, {"token", "create"}, {"token", "create", "-node", "proxy-0"},
		{"token", "create", "-node", "proxy-4242"}, {"token", "create", "-node", nodeName, "-ttl", "169h"},
		{"token", "create", "-node", nodeName, "extra"}, {"token", "create", "-nope"},
	} {
		require.Error(t, runAgentCommand(ctx, cfg, db, arguments, &output), arguments)
	}

	err := runAgentCommand(ctx, &config.Config{}, db, []string{"token", "create", "-node", nodeName}, &output)
	require.True(t, errors.Is(err, agentpki.ErrDisabled))
	require.Error(t, runAdminCommand(ctx, cfg, db, []string{"bogus"}, &output))
}

func TestTakeAdminCommand(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	for _, command := range []string{"module", "agent", "agents", "routes"} {
		os.Args = []string{"anix-control", command, "token", "create"}
		require.Equal(t, []string{command, "token", "create"}, takeAdminCommand())
		require.Equal(t, []string{"anix-control"}, os.Args)
	}
	os.Args = []string{"anix-control", "-config", "x.yaml"}
	require.Nil(t, takeAdminCommand())
}

func TestAgentPKIForGRPC(t *testing.T) {
	pki, err := agentPKIForGRPC(&config.Config{}, nil)
	require.NoError(t, err)
	require.Nil(t, pki, "the default (preferred) runs without the agent PKI")

	required := &config.Config{AgentControl: config.AgentControlConfig{MTLS: config.AgentMTLSRequired}}
	_, err = agentPKIForGRPC(required, nil)
	require.ErrorIs(t, err, agentpki.ErrDisabled)

	cfg, db := moduleCommandFixture(t)
	pki, err = agentPKIForGRPC(cfg, db)
	require.NoError(t, err)
	require.NotNil(t, pki)
	require.Equal(t, "prod", pki.Cluster())
}

// TestAgentPKIWithTheCAAlone: agent enrollment needs only the built-in CA,
// not the module runtime or its :7443 listener.
func TestAgentPKIWithTheCAAlone(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ServiceCA{}, &model.ModuleCertificate{}, &model.AgentEnrollment{}, &model.AgentCertificate{},
		&model.Node{}, &model.ForwardNode{}, &model.OperationLog{}, &model.ForwardLinkCA{}))
	cfg := &config.Config{ModuleRuntime: config.ModuleRuntimeConfig{
		Cluster: "edge", CAKEK: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
	}}
	ctx := context.Background()
	require.NoError(t, ensureModulePKI(ctx, cfg, db), "the CA is created without the module runtime")
	var cas int64
	require.NoError(t, db.Model(&model.ServiceCA{}).Count(&cas).Error)
	require.Equal(t, int64(1), cas)
	require.NoError(t, db.Model(&model.ForwardLinkCA{}).Where("state = ?", model.ForwardLinkCAStateCurrent).Count(&cas).Error)
	require.Equal(t, int64(1), cas, "the forward link CA is created with it")
	require.NoError(t, ensureModulePKI(ctx, cfg, db), "ensuring again creates nothing")
	require.NoError(t, db.Model(&model.ForwardLinkCA{}).Count(&cas).Error)
	require.Equal(t, int64(1), cas)

	pki, err := agentPKIForGRPC(cfg, db)
	require.NoError(t, err)
	require.NotNil(t, pki)
	require.Equal(t, "edge", pki.Cluster())
	node := model.Node{Name: "proxy", APIKeyHash: strings.Repeat("e", 64), Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&node).Error)
	var output bytes.Buffer
	require.NoError(t, runAgentCommand(ctx, cfg, db, []string{"token", "create", "-node", "proxy-" + strconv.FormatUint(uint64(node.ID), 10)}, &output))
	require.Contains(t, output.String(), "anixagt_")

	// The module listener stays off; the CA's maintenance runs anyway.
	rt := &serverRuntime{workers: newBackgroundWorkers(ctx)}
	require.NoError(t, rt.startModuleRuntime(cfg, nil))
	require.NoError(t, rt.startKernelCAMaintenance(cfg, db))
	stopped, running := rt.workers.Stop(5 * time.Second)
	require.True(t, stopped, running)

	// An external PKI holds no CA key: every mode but required runs
	// without agent enrollment (preferred is the default), required refuses
	// to start.
	external := &config.Config{ModuleRuntime: config.ModuleRuntimeConfig{Enabled: true, PKI: config.ModulePKIExternal}}
	for _, mode := range []string{"", config.AgentMTLSOff, config.AgentMTLSOptional, config.AgentMTLSPreferred} {
		external.AgentControl.MTLS = mode
		pki, err = agentPKIForGRPC(external, db)
		require.NoError(t, err, mode)
		require.Nil(t, pki, mode)
	}
	external.AgentControl.MTLS = config.AgentMTLSRequired
	_, err = agentPKIForGRPC(external, db)
	require.ErrorIs(t, err, agentpki.ErrExternalPKI)
	require.NoError(t, (&serverRuntime{}).startKernelCAMaintenance(&config.Config{}, db), "no CA, no maintenance")
}

// TestAgentLinkCACommand lists, prints and rotates the forward link CA.
func TestAgentLinkCACommand(t *testing.T) {
	cfg, db := moduleCommandFixture(t)
	ctx := context.Background()

	var output bytes.Buffer
	require.NoError(t, runAdminCommand(ctx, cfg, db, []string{"agent", "link-ca", "list"}, &output))
	var rows []model.ForwardLinkCA
	require.NoError(t, json.Unmarshal(output.Bytes(), &rows))
	require.Len(t, rows, 1)
	require.Equal(t, model.ForwardLinkCAStateCurrent, rows[0].State)
	require.Contains(t, rows[0].CertificatePEM, "BEGIN CERTIFICATE")
	require.NotContains(t, output.String(), "sealed_key", "the sealed key is never printed")

	output.Reset()
	require.NoError(t, runAgentCommand(ctx, cfg, db, []string{"link-ca", "rotate"}, &output))
	var next model.ForwardLinkCA
	require.NoError(t, json.Unmarshal(output.Bytes(), &next))
	require.Equal(t, model.ForwardLinkCAStateNext, next.State)

	output.Reset()
	require.NoError(t, runAgentCommand(ctx, cfg, db, []string{"link-ca", "bundle"}, &output))
	require.Equal(t, 2, strings.Count(output.String(), "BEGIN CERTIFICATE"), "current and next")

	require.Error(t, runAgentCommand(ctx, cfg, db, []string{"link-ca", "bogus"}, &output))
	require.ErrorIs(t, runAgentCommand(ctx, &config.Config{}, db, []string{"link-ca", "list"}, &output), agentpki.ErrLinkDisabled)
}

func TestAgentOfflineBundleCommand(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sign := func(data []byte) []byte {
		return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(private, data)) + "\n")
	}
	artifacts := t.TempDir()
	release := filepath.Join(artifacts, "v4.2.0")
	require.NoError(t, os.MkdirAll(release, 0o750))
	zip := []byte("fake agent zip")
	digest := sha256.Sum256(zip)
	sums := []byte(hex.EncodeToString(digest[:]) + "  anix-agent-linux-64.zip\n")
	for name, data := range map[string][]byte{
		"anix-agent-linux-64.zip": zip, "anix-agent-linux-64.zip.sig": sign(zip), "SHA256SUMS": sums, "SHA256SUMS.sig": sign(sums),
	} {
		require.NoError(t, os.WriteFile(filepath.Join(release, name), data, 0o600))
	}
	cfg := &config.Config{}
	cfg.Plugins.OfficialPublicKey = base64.StdEncoding.EncodeToString(public)
	cfg.AgentInstall.ArtifactDir = artifacts
	cfg.AgentInstall.AgentVersion = "v4.2.0"
	cfg.GRPC.Port = 50051
	out := filepath.Join(t.TempDir(), "agent-offline-amd64.tar.gz")
	ctx := context.Background()

	var output bytes.Buffer
	err = runAdminCommand(ctx, cfg, nil, []string{"agent", "offline-bundle", "-arch", "amd64", "-o", out}, &output)
	require.ErrorContains(t, err, "agent_install.public_url", "no address nodes reach")

	output.Reset()
	require.NoError(t, runAdminCommand(ctx, cfg, nil, []string{"agent", "offline-bundle", "-arch", "amd64", "-o", out, "-control", "https://panel.example.com"}, &output))
	var written struct {
		File       string   `json:"file"`
		Asset      string   `json:"asset"`
		GRPCTarget string   `json:"grpc_target"`
		Files      []string `json:"files"`
		Install    string   `json:"install"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &written))
	require.Equal(t, out, written.File)
	require.Equal(t, "anix-agent-linux-64.zip", written.Asset)
	require.Equal(t, "panel.example.com:50051", written.GRPCTarget)
	require.Contains(t, written.Files, "SHA256SUMS.sig")
	require.Contains(t, written.Install, "--offline agent-offline-amd64.tar.gz --control https://panel.example.com")
	info, err := os.Stat(out)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	entries, err := os.ReadDir(filepath.Dir(out))
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary file is left")

	cfg.AgentInstall.PublicURL = "https://panel.example.com"
	for _, arguments := range [][]string{
		{"offline-bundle"}, {"offline-bundle", "-arch", "amd64"}, {"offline-bundle", "-o", out},
		{"offline-bundle", "-arch", "riscv64", "-o", out}, {"offline-bundle", "-arch", "arm64", "-o", out},
		{"offline-bundle", "-arch", "amd64", "-o", out, "extra"},
	} {
		require.Error(t, runAgentCommand(ctx, cfg, nil, arguments, &output), arguments)
	}
	entries, err = os.ReadDir(filepath.Dir(out))
	require.NoError(t, err)
	require.Len(t, entries, 1, "a failed bundle leaves no file behind")
}
