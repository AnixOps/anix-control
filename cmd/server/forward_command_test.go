package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestForwardCommandResetsNodeGeneration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(append(model.KernelForwardModels(), &model.OperationLog{})...))
	now := time.Now().UTC()
	require.NoError(t, db.Create(&model.KernelForwardNodeState{
		NodeRef: "forward-7", Generation: 3, StateHash: "abc",
		StateJSON: `{"node_ref":"forward-7","generation":3,"state_hash":"abc"}`, PlannedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.KernelForwardNodeReport{
		NodeRef: "forward-7", Generation: 9, StateHash: "def", ReportJSON: `{}`, ObservedAt: now, ReceivedAt: now,
	}).Error)
	ctx := context.Background()
	run := func(arguments ...string) (string, error) {
		var output bytes.Buffer
		err := runAdminCommand(ctx, nil, db, append([]string{"forward"}, arguments...), &output)
		return output.String(), err
	}

	for _, arguments := range [][]string{{}, {"reset-node"}, {"bogus", "forward-7"}, {"reset-node", "forward-7", "extra"}} {
		_, err := run(arguments...)
		require.ErrorContains(t, err, "invalid forward command", arguments)
	}
	_, err = run("reset-node", "forward-8")
	require.ErrorIs(t, err, kernelforward.ErrNoState)
	_, err = run("reset-node", "node-7")
	require.ErrorIs(t, err, kernelforward.ErrInvalidRequest)

	output, err := run("reset-node", "forward-7")
	require.NoError(t, err)
	var printed map[string]any
	require.NoError(t, json.Unmarshal([]byte(output), &printed))
	assert.Equal(t, map[string]any{
		"node_ref": "forward-7", "previous_generation": float64(3), "reported_generation": float64(9),
		"generation": float64(10), "state_hash": "abc", "audited": true,
	}, printed)
	state, found, err := kernelforward.New(db).State(ctx, "forward-7")
	require.NoError(t, err)
	require.True(t, found)
	assert.EqualValues(t, 10, state.GetGeneration())
	var entry model.OperationLog
	require.NoError(t, db.First(&entry, "action = ?", "forward.reset_node").Error)
	assert.Equal(t, "system/cli", entry.Username)
	assert.Contains(t, entry.Content, `"generation":10`)
}

func newForwardCommandDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(append(model.KernelForwardModels(), &model.OperationLog{}, &model.ForwardNode{}, &model.Node{}, &model.NodeProtocol{})...))
	require.NoError(t, db.Create(&[]model.ForwardNode{
		{ID: 11, Name: "hk-entry", Type: "relay", Host: "192.0.2.11", Port: 7000, Enabled: true},
		{ID: 12, Name: "jp-exit", Type: "exit", Host: "192.0.2.12", Port: 7000, Enabled: true},
	}).Error)
	caps := &forwardv1.NodeCapabilities{Engines: []*forwardv1.EngineCapabilities{{
		Engine: forwardv1.Engine_ENGINE_NFTABLES, Available: true, Udp: true,
		Strategies:     []forwardv1.BalanceStrategy{forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN},
		LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
	}}}
	for _, id := range []uint32{11, 12} {
		_, _, err := kernelforward.New(db).RecordHello(context.Background(), agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: id}, caps, "4.2.0")
		require.NoError(t, err)
	}
	return db
}

func TestForwardCommandRoutesNodesAndStats(t *testing.T) {
	db := newForwardCommandDB(t)
	ctx := context.Background()
	dir := t.TempDir()
	run := func(arguments ...string) (string, error) {
		var output bytes.Buffer
		err := runAdminCommand(ctx, nil, db, append([]string{"forward"}, arguments...), &output)
		return output.String(), err
	}
	routeFile := filepath.Join(dir, "route.json")
	require.NoError(t, os.WriteFile(routeFile, []byte(`{"name":"hk-jp","listen":{"port":31000,"protocol":"L4_PROTOCOL_TCP"},
	 "hops":[{"role":"HOP_ROLE_ENTRY","engine":"ENGINE_NFTABLES","node_refs":["forward-11"]},
	         {"role":"HOP_ROLE_EXIT","engine":"ENGINE_NFTABLES","node_refs":["forward-12"],"ingress":{"security":"LINK_SECURITY_RAW"}}],
	 "targets":[{"host":"198.51.100.10","port":443}]}`), 0o600))

	for _, arguments := range [][]string{{"routes"}, {"routes", "create"}, {"routes", "get"}, {"routes", "bogus", "x"}, {"nodes", "set"},
		{"nodes", "set", "forward-12"}, {"nodes", "set", "forward-12", "--defaults", "--label", "a=b"}, {"stats", "extra"}, {"routes", "list", "--nope"}} {
		_, err := run(arguments...)
		require.ErrorContains(t, err, "invalid forward command", arguments)
	}

	output, err := run("routes", "create", "-f", routeFile, "--request-id", "cli-1")
	require.NoError(t, err)
	created := &forwardv1.Route{}
	require.NoError(t, protojson.Unmarshal([]byte(output), created))
	assert.Equal(t, "admin", created.GetOwner())
	again, err := run("routes", "create", "-f", routeFile, "--request-id", "cli-1")
	require.NoError(t, err)
	assert.Equal(t, output, again, "the request id makes a retry apply once")
	_, err = run("routes", "create", "-f", routeFile)
	require.ErrorContains(t, err, "port_in_use", "a refusal prints the violations' codes")

	output, err = run("routes", "list")
	require.NoError(t, err)
	assert.Contains(t, output, created.GetId())
	assert.Contains(t, output, "forward-11 > forward-12")
	output, err = run("routes", "list", "--json", "--node", "forward-12")
	require.NoError(t, err)
	assert.Contains(t, output, `"hk-jp"`)
	output, err = run("routes", "pause", created.GetId())
	require.NoError(t, err)
	assert.Contains(t, output, `"paused": true`)
	output, err = run("routes", "resume", created.GetId())
	require.NoError(t, err)
	assert.NotContains(t, output, `"paused"`)
	output, err = run("routes", "get", created.GetId())
	require.NoError(t, err)
	assert.Contains(t, output, `"revision": "3"`)

	output, err = run("nodes", "set", "forward-12", "--port-range", "40000-40999", "--reserved", "40001,40002", "--label", "link=iepl", "--address", "203.0.113.12")
	require.NoError(t, err)
	assert.Contains(t, output, `"link": "iepl"`)
	settingsFile := filepath.Join(dir, "settings.json")
	require.NoError(t, os.WriteFile(settingsFile, []byte(`{"port_range":{"first":41000,"last":41999}}`), 0o600))
	_, err = run("nodes", "set", "forward-12", "-f", settingsFile)
	require.NoError(t, err)
	_, err = run("nodes", "set", "forward-12", "--port-range", "70000-1")
	require.Error(t, err)
	_, err = run("nodes", "set", "router-1", "--defaults")
	require.ErrorIs(t, err, kernelforward.ErrInvalidRequest)
	output, err = run("nodes", "list")
	require.NoError(t, err)
	assert.Contains(t, output, "forward-12")
	assert.Contains(t, output, "41000-41999")
	output, err = run("nodes", "list", "--json", "--kind", "forward")
	require.NoError(t, err)
	assert.Contains(t, output, `"node_ref": "forward-11"`)

	output, err = run("stats", "--route", created.GetId(), "--since", "2026-10-01T00:00:00Z")
	require.NoError(t, err)
	assert.Contains(t, output, "UP_BYTES")
	output, err = run("stats", "--json")
	require.NoError(t, err)
	assert.Contains(t, output, `"totals"`)
	_, err = run("stats", "--since", "yesterday")
	require.Error(t, err)

	_, err = run("routes", "delete", created.GetId())
	require.ErrorContains(t, err, "--yes")
	output, err = run("routes", "delete", created.GetId(), "--yes")
	require.NoError(t, err)
	assert.Contains(t, output, `"audited": true`)

	var actions []string
	require.NoError(t, db.Model(&model.OperationLog{}).Where("username = ?", "system/cli").Order("id").Pluck("action", &actions).Error)
	assert.Equal(t, []string{"forward.route_create", "forward.route_create", "forward.route_pause", "forward.route_resume",
		"forward.node_settings", "forward.node_settings", "forward.route_delete"}, actions)
}

// routes diagnose prints Control's view of a route; with no Agent sessions
// in the command line the node probes are SKIPPED and say where they run.
func TestForwardCommandDiagnose(t *testing.T) {
	db := newForwardCommandDB(t)
	ctx := context.Background()
	previous := forwardCommandProbes
	// Probes dial in parallel, so the fake records under a lock.
	var dialMu sync.Mutex
	var dialled []string
	forwardCommandProbes = service.DiagnosisProbes{Dial: func(_ context.Context, _, address string, _ time.Duration) (net.Conn, error) {
		dialMu.Lock()
		dialled = append(dialled, address)
		dialMu.Unlock()
		return nil, errors.New("connection refused")
	}}
	t.Cleanup(func() { forwardCommandProbes = previous })
	run := func(arguments ...string) (string, error) {
		var output bytes.Buffer
		err := runAdminCommand(ctx, nil, db, append([]string{"forward"}, arguments...), &output)
		return output.String(), err
	}
	route := &forwardv1.Route{Owner: "admin", Name: "diag", Listen: &forwardv1.Listen{Port: 31010, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Hops: []*forwardv1.Hop{
			{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: forwardv1.Engine_ENGINE_NFTABLES, NodeRefs: []string{"forward-11"}},
			{Role: forwardv1.HopRole_HOP_ROLE_EXIT, Engine: forwardv1.Engine_ENGINE_NFTABLES, NodeRefs: []string{"forward-12"},
				Ingress: &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_RAW}},
		},
		Targets: []*forwardv1.Target{{Host: "198.51.100.10", Port: 443}},
	}
	created, err := kernelforward.New(db).CreateRoute(ctx, "diag-1", route)
	require.NoError(t, err)

	_, err = run("routes", "diagnose")
	require.ErrorContains(t, err, "invalid forward command")
	output, err := run("routes", "diagnose", created.GetId(), "--timeout", "3000")
	require.ErrorIs(t, err, errDiagnosisFailed, "never reported: the config steps fail")
	assert.Contains(t, output, "HOP  NODE")
	assert.Contains(t, output, "never_reported")
	assert.Contains(t, output, "node_vantage_unavailable")
	assert.Contains(t, output, "POST /api/v4/forward/routes/{id}/diagnose")
	assert.Contains(t, output, "route "+created.GetId()+": FAILED")
	dialMu.Lock()
	assert.ElementsMatch(t, []string{"192.0.2.11:31010", "198.51.100.10:443"}, dialled)
	dialMu.Unlock()

	output, err = run("routes", "diagnose", created.GetId(), "--json")
	require.ErrorIs(t, err, errDiagnosisFailed)
	answer := &forwardv1.DiagnoseRouteResponse{}
	require.NoError(t, protojson.Unmarshal([]byte(output), answer))
	assert.True(t, answer.GetCached(), "moments later the diagnosis is answered again")
	assert.Equal(t, created.GetId(), answer.GetRouteId())

	_, err = run("routes", "diagnose", "01JF1A000000000000000000ZZ")
	require.ErrorIs(t, err, kernelforward.ErrNotFound)
	var actions []string
	require.NoError(t, db.Model(&model.OperationLog{}).Where("username = ?", "system/cli").Order("id").Pluck("action", &actions).Error)
	assert.Equal(t, []string{"forward.route_diagnose", "forward.route_diagnose"}, actions)
}
