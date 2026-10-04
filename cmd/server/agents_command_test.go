package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAgentsTransportsCommand(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.ForwardNode{}, &model.ForwardCleanAgent{}, &model.AgentTransport{}, &model.AgentCertificate{}))
	now := time.Now().UTC()
	require.NoError(t, db.Create(&[]model.Node{
		{ID: 1, Name: "enrolled", APIKey: "a"}, {ID: 2, Name: "legacy", APIKey: "b"}, {ID: 3, Name: "off", APIKey: "c", Status: model.NodeStatusDisabled},
	}).Error)
	require.NoError(t, db.Create(&[]model.AgentTransport{
		{NodeKind: "proxy", NodeID: 1, Transport: model.AgentTransportMTLSStream, AgentVersion: "2.0.0", FirstSeenAt: now, LastSeenAt: now},
		{NodeKind: "proxy", NodeID: 2, Transport: model.AgentTransportAPIKeyStream, AgentVersion: "1.1.0", FirstSeenAt: now, LastSeenAt: now},
		{NodeKind: "proxy", NodeID: 2, Transport: model.AgentTransportUniProxy, FirstSeenAt: now, LastSeenAt: now.Add(-time.Minute)},
	}).Error)
	require.NoError(t, db.Create(&model.AgentCertificate{Serial: "0a1b", NodeKind: "proxy", NodeID: 1, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", NotAfter: now.Add(72 * time.Hour)}).Error)
	cfg := &config.Config{AgentControl: config.AgentControlConfig{LegacySunset: "2027-03-31"}}
	ctx := context.Background()
	run := func(arguments ...string) (string, error) {
		var output bytes.Buffer
		err := runAdminCommand(ctx, cfg, db, append([]string{"agents"}, arguments...), &output)
		return output.String(), err
	}

	for _, arguments := range [][]string{{}, {"bogus"}, {"transports", "extra"}, {"transports", "--mode", "x"}, {"transports", "--check-required", "--legacy-only"}} {
		_, err := run(arguments...)
		require.Error(t, err, arguments)
	}

	output, err := run("transports")
	require.NoError(t, err)
	require.Contains(t, output, "agent_control.mtls: preferred (legacy sunset: 2027-03-31)")
	require.Contains(t, output, "proxy-1")
	require.Contains(t, output, "0a1b until")
	require.Contains(t, output, "apikey-stream,uniproxy")
	require.Contains(t, output, "off (disabled)")
	require.Contains(t, output, "3 node(s): 1 mtls, 1 legacy, 0 third-party, 1 unseen.")

	output, err = run("transports", "--legacy-only")
	require.NoError(t, err)
	require.Contains(t, output, "proxy-2")
	require.NotContains(t, output, "proxy-1 ")
	require.Contains(t, output, "1 node(s) on a legacy AnixOps Agent channel")

	output, err = run("transports", "--legacy-only", "--json")
	require.NoError(t, err)
	var inventory agenttransport.Inventory
	require.NoError(t, json.Unmarshal([]byte(output), &inventory))
	require.Len(t, inventory.Nodes, 1)
	require.Equal(t, "legacy", inventory.Nodes[0].Status)
	require.Equal(t, "1.1.0", inventory.Nodes[0].AgentVersion)

	// The upgrade gate, under the 4.1 default preferred: proxy-2 is
	// legacy; proxy-3 is disabled.
	output, err = run("transports", "--check-required")
	require.ErrorIs(t, err, errAgentsNotReady)
	require.Equal(t, 3, adminCommandExitCode(err), "the gate's own exit status")
	require.Equal(t, 2, adminCommandExitCode(agentsUsageError()))
	require.Contains(t, output, "agent_control.mtls: preferred")
	require.Contains(t, output, "proxy-2")
	require.Contains(t, output, "apikey-stream")
	require.NotContains(t, output, "proxy-3", "a disabled node is not a blocker")
	require.Contains(t, output, "Not ready for agent_control.mtls: required:")
	require.Contains(t, output, "1 enabled node(s) still on a legacy AnixOps Agent channel")
	output, err = run("transports", "--check-required", "--json")
	require.ErrorIs(t, err, errAgentsNotReady)
	var gate struct {
		Mode             string                           `json:"mode"`
		ReadyForRequired bool                             `json:"ready_for_required"`
		Blockers         []agenttransport.RequiredBlocker `json:"required_blockers"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &gate))
	require.Equal(t, config.AgentMTLSPreferred, gate.Mode)
	require.False(t, gate.ReadyForRequired)
	require.Len(t, gate.Blockers, 1)
	require.Equal(t, agenttransport.BlockerLegacy, gate.Blockers[0].Reason)

	require.NoError(t, db.Where("node_id = ?", 2).Delete(&model.AgentTransport{}).Error)
	output, err = run("transports", "--legacy-only")
	require.NoError(t, err)
	require.Contains(t, output, "refuses none")

	// proxy-2 now was never seen and holds no certificate: it never
	// enrolled, which the gate also refuses.
	output, err = run("transports", "--check-required")
	require.ErrorIs(t, err, errAgentsNotReady)
	require.Contains(t, output, "never_enrolled")
	require.NoError(t, db.Create(&model.AgentCertificate{Serial: "0c2d", NodeKind: "proxy", NodeID: 2, Cluster: "prod", EnrollmentID: "e2", IssuerKeyID: "k", NotAfter: now.Add(72 * time.Hour)}).Error)
	output, err = run("transports", "--check-required")
	require.NoError(t, err, output)
	require.Contains(t, output, "Ready for agent_control.mtls: required")
	output, err = run("transports", "--check-required", "--json")
	require.NoError(t, err, output)
	require.Contains(t, output, `"ready_for_required": true`)

	// Without the table (a database the kernel never migrated) the command
	// fails rather than reporting an empty checklist.
	require.NoError(t, db.Migrator().DropTable(&model.AgentTransport{}))
	_, err = run("transports")
	require.Error(t, err)
	require.Error(t, runAgentsCommand(ctx, nil, db, []string{"transports"}, &bytes.Buffer{}))
}

func TestAgentTransportPolicyLog(t *testing.T) {
	for mode, expected := range map[string]string{
		"":                        "agent_control.mtls=preferred: legacy API-key agents are served with deprecation signals",
		config.AgentMTLSOff:       "neither requested nor accepted",
		config.AgentMTLSOptional:  "verified when presented",
		config.AgentMTLSRequired:  "accept client certificates only",
		config.AgentMTLSPreferred: "deprecation signals",
	} {
		cfg := &config.Config{AgentControl: config.AgentControlConfig{MTLS: mode}}
		line := agentTransportPolicyLog(cfg, nil)
		require.Contains(t, line, expected, mode)
		if mode == config.AgentMTLSOff {
			require.Contains(t, line, "agent enrollment: off")
		} else {
			require.Contains(t, line, "the gRPC listener is off")
		}
		require.Contains(t, line, "legacy sunset: none")
	}
	line := agentTransportPolicyLog(&config.Config{AgentControl: config.AgentControlConfig{LegacySunset: "2027-03-31"}}, nil)
	require.Contains(t, line, "legacy sunset: 2027-03-31")
}
