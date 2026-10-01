package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
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
	for _, command := range []string{"module", "agent"} {
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
