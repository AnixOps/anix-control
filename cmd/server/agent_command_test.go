package main

import (
	"bytes"
	"context"
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
	for _, command := range []string{"module", "agent", "routes"} {
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
	require.Nil(t, pki, "optional mode runs without the agent PKI")

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
		&model.Node{}, &model.ForwardNode{}, &model.OperationLog{}))
	cfg := &config.Config{ModuleRuntime: config.ModuleRuntimeConfig{
		Cluster: "edge", CAKEK: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
	}}
	ctx := context.Background()
	require.NoError(t, ensureModulePKI(ctx, cfg, db), "the CA is created without the module runtime")
	var cas int64
	require.NoError(t, db.Model(&model.ServiceCA{}).Count(&cas).Error)
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

	// An external PKI holds no CA key: optional mode runs without agent
	// enrollment, the other modes refuse to start.
	external := &config.Config{ModuleRuntime: config.ModuleRuntimeConfig{Enabled: true, PKI: config.ModulePKIExternal}}
	pki, err = agentPKIForGRPC(external, db)
	require.NoError(t, err)
	require.Nil(t, pki)
	external.AgentControl.MTLS = config.AgentMTLSPreferred
	_, err = agentPKIForGRPC(external, db)
	require.ErrorIs(t, err, agentpki.ErrExternalPKI)
	require.NoError(t, (&serverRuntime{}).startKernelCAMaintenance(&config.Config{}, db), "no CA, no maintenance")
}
