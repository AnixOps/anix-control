package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseForwardAnsibleStatsOutput_RawUpAndDown(t *testing.T) {
	totals, found := parseForwardAnsibleStatsOutput(strings.Join([]string{
		`STATS_JSON {"protocol":"tcp","upload":300,"download":5000}`,
		`STATS_JSON {"protocol":"udp","upload":20,"download":40}`,
	}, "\n"))
	assert.True(t, found)
	assert.Equal(t, forwardAnsibleStatsTotals{Upload: 320, Download: 5040}, totals)
}

func TestParseForwardAnsibleStatsOutput_AnsibleDebugMessageAndDuplicates(t *testing.T) {
	// Default callback output of the debug task, plus the raw line once more
	// (as -v would print it): each protocol counts once.
	output := `TASK [Print STATS_JSON] ********************************************************
ok: [203.0.113.10] => {
    "msg": [
        "STATS_JSON {\"protocol\":\"tcp\",\"upload\":300,\"download\":5000}",
        "STATS_JSON {\"protocol\":\"udp\",\"upload\":0,\"download\":0}"
    ]
}
STATS_JSON {"protocol":"tcp","upload":300,"download":5000}
`
	totals, found := parseForwardAnsibleStatsOutput(output)
	assert.True(t, found)
	assert.Equal(t, forwardAnsibleStatsTotals{Upload: 300, Download: 5000}, totals)
}

func TestParseForwardAnsibleStatsOutput_LegacyAndIptablesBytes(t *testing.T) {
	totals, found := parseForwardAnsibleStatsOutput(`        "STATS_JSON {\"protocol\":\"tcp\",\"bytes\":120,\"legacy\":true}"` + "\n" + `STATS_JSON {"protocol":"udp","bytes":30}`)
	assert.True(t, found)
	assert.Equal(t, forwardAnsibleStatsTotals{Upload: 150, Legacy: true}, totals)

	// When both kinds show up, the ct-direction counters win.
	totals, found = parseForwardAnsibleStatsOutput(`STATS_JSON {"protocol":"tcp","bytes":999}` + "\n" + `STATS_JSON {"protocol":"tcp","upload":1,"download":2}`)
	assert.True(t, found)
	assert.Equal(t, forwardAnsibleStatsTotals{Upload: 1, Download: 2}, totals)
}

func TestParseForwardAnsibleStatsOutput_IgnoresNoiseAndNegatives(t *testing.T) {
	_, found := parseForwardAnsibleStatsOutput("PLAY RECAP\nSTATS_JSON not-json\n" + `STATS_JSON {"protocol":"tcp","upload":-1,"download":5}`)
	assert.False(t, found)
}

func TestForwardAnsibleStatsTotals_CursorKeys(t *testing.T) {
	ct := forwardAnsibleStatsTotals{Upload: 1, Download: 2}.trafficSnapshot(7, model.ForwardRuntimeBackendNftablesAnsible)
	assert.Equal(t, PanelForwardTrafficSnapshot{ForwardID: 7, Backend: "nftables_ansible:ct", UploadTotal: 1, DownloadTotal: 2}, ct)
	legacy := forwardAnsibleStatsTotals{Upload: 1, Legacy: true}.trafficSnapshot(7, model.ForwardRuntimeBackendNftablesAnsible)
	assert.Equal(t, "nftables_ansible", legacy.Backend)
}

// queuedStatsRunner answers each stats playbook run with the next output.
type queuedStatsRunner struct {
	outputs  []string
	lastArgs []string
}

func (r *queuedStatsRunner) Run(_ context.Context, _ string, args []string, _ string, _ map[string]string) (string, error) {
	r.lastArgs = append([]string(nil), args...)
	if len(r.outputs) == 0 {
		return "", nil
	}
	out := r.outputs[0]
	r.outputs = r.outputs[1:]
	return out, nil
}

func (s *PanelForwardServiceTestSuite) TestForwardAnsibleStatsWorker_RecordsUpAndDownAcrossMigrationAndReset() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendNftablesAnsible)

	user := &model.User{
		Email:          "nft-stats@example.com",
		Password:       "hash",
		Token:          "nft-stats-token",
		UUID:           "nft-stats-uuid",
		TransferEnable: 20 * bytesPerGiB,
	}
	require.NoError(s.T(), db.Create(user).Error)
	node := &model.ForwardNode{Name: "nft-stats-node", Type: model.ForwardNodeTypeRelay, Host: "203.0.113.10", Port: 22, Enabled: true}
	require.NoError(s.T(), db.Create(node).Error)
	tunnel := &model.ForwardTunnel{
		Name: "nft-stats-tunnel", Type: 1, InNodeID: node.ID, Protocol: "both",
		TCPListenAddr: "[::]", UDPListenAddr: "[::]", Flow: 1, TrafficRatio: 1,
		Status: model.ForwardTunnelStatusActive,
	}
	require.NoError(s.T(), db.Create(tunnel).Error)
	permission := &model.ForwardUserTunnel{UserID: user.ID, TunnelID: tunnel.ID, Status: model.ForwardUserTunnelStatusActive}
	require.NoError(s.T(), db.Create(permission).Error)
	forward := &model.Forward{
		UserID: user.ID, UserName: user.Email, Name: "nft-stats-forward", TunnelID: tunnel.ID,
		InPort: 15010, RemoteAddr: "10.0.0.1:80,[2001:db8::1]:80", Strategy: "round",
		Status: model.ForwardStatusActive, RuntimeBackend: model.ForwardRuntimeBackendNftablesAnsible,
	}
	require.NoError(s.T(), db.Create(forward).Error)

	runner := &queuedStatsRunner{outputs: []string{
		// Not migrated yet: legacy nat-chain counter, upload only.
		`STATS_JSON {"protocol":"tcp","bytes":120,"legacy":true}`,
		// Migrated: the ct counters start at 0 on a fresh cursor.
		`STATS_JSON {"protocol":"tcp","upload":1000,"download":4000}` + "\n" + `STATS_JSON {"protocol":"udp","upload":10,"download":20}`,
		`STATS_JSON {"protocol":"tcp","upload":1500,"download":6000}` + "\n" + `STATS_JSON {"protocol":"udp","upload":10,"download":20}`,
		// The node rebooted and the counters restarted from zero.
		`STATS_JSON {"protocol":"tcp","upload":100,"download":300}`,
	}}
	worker := NewForwardAnsibleStatsWorker(db)
	worker.runner = runner

	for range 4 {
		_, err := worker.runOnce(context.Background())
		require.NoError(s.T(), err)
	}

	// The stats playbook gets the nftables plan for the forward.
	require.NotEmpty(s.T(), runner.lastArgs)
	extraVars := runner.lastArgs[len(runner.lastArgs)-2]
	var vars map[string]any
	require.NoError(s.T(), json.Unmarshal([]byte(extraVars), &vars))
	assert.Contains(s.T(), vars, "nftables")
	assert.True(s.T(), strings.HasSuffix(runner.lastArgs[len(runner.lastArgs)-1], forwardAnsibleStatsNftablesPlaybookPath))

	// upload: 120 (legacy) + 1010 + 500 + 100 (after reset) = 1730
	// download: 0 (legacy) + 4020 + 2000 + 300 (after reset) = 6320
	var reloaded model.Forward
	require.NoError(s.T(), db.First(&reloaded, forward.ID).Error)
	assert.Equal(s.T(), int64(1730), reloaded.OutFlow)
	assert.Equal(s.T(), int64(6320), reloaded.InFlow)

	var reloadedUser model.User
	require.NoError(s.T(), db.First(&reloadedUser, user.ID).Error)
	assert.Equal(s.T(), int64(1730), reloadedUser.U)
	assert.Equal(s.T(), int64(6320), reloadedUser.D)

	var cursors []model.ForwardTrafficCursor
	require.NoError(s.T(), db.Where("forward_id = ?", forward.ID).Order("backend ASC").Find(&cursors).Error)
	require.Len(s.T(), cursors, 2)
	assert.Equal(s.T(), "nftables_ansible", cursors[0].Backend)
	assert.Equal(s.T(), int64(120), cursors[0].UploadTotal)
	assert.Equal(s.T(), "nftables_ansible:ct", cursors[1].Backend)
	assert.Equal(s.T(), int64(100), cursors[1].UploadTotal)
	assert.Equal(s.T(), int64(300), cursors[1].DownloadTotal)
}
