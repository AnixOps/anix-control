package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newLegacyCleanupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.ForwardNode{}, &model.ForwardTunnel{}, &model.Forward{},
		&model.ForwardRule{}, &model.SpeedLimit{}, &model.ForwardUserTunnel{}, &model.SystemConfig{}))
	return db
}

// nodeXRecorder is a fake NodeX that records the execute calls.
type nodeXRecorder struct {
	mu       sync.Mutex
	requests []nodeXForwardExecuteRequest
	tokens   []string
	status   int
}

func (r *nodeXRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	var decoded nodeXForwardExecuteRequest
	_ = json.Unmarshal(body, &decoded)
	r.mu.Lock()
	r.requests = append(r.requests, decoded)
	r.tokens = append(r.tokens, req.Header.Get("Authorization"))
	status := r.status
	r.mu.Unlock()
	if req.URL.Path != defaultForwardRuntimeNodeXExecutePath {
		http.NotFound(w, req)
		return
	}
	if status != 0 && status != http.StatusOK {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"gost refused"}`))
		return
	}
	_, _ = w.Write([]byte(`{"data":{"backend":"gost","status":2,"message":"deleted"}}`))
}

func seedLegacyNodeX(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&model.ForwardNode{ID: 1, Name: "hk", Type: "relay", Host: "192.0.2.1", Port: 22, APIPort: 9000, APIToken: "node-token-1", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 2, Name: "jp", Type: "exit", Host: "192.0.2.2", Port: 22, APIPort: 9000, APIToken: "node-token-2", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 3, Name: "idle", Type: "exit", Host: "192.0.2.3", Port: 22, APIPort: 9000, Enabled: true}).Error)
	require.NoError(t, db.Create(&model.ForwardTunnel{ID: 5, Name: "hk-jp", InNodeID: 1, Protocol: "tcp", Type: 2, Status: 1}).Error)
	require.NoError(t, db.Create(&model.Forward{ID: 7, UserID: 1, Name: "web", TunnelID: 5, InPort: 20001, RemoteAddr: "198.51.100.7:443", RuntimeBackend: "gost"}).Error)
	// A forward on another backend is not NodeX's.
	require.NoError(t, db.Create(&model.Forward{ID: 8, UserID: 1, Name: "nft", TunnelID: 5, InPort: 20002, RemoteAddr: "198.51.100.8:443", RuntimeBackend: "nftables_ansible"}).Error)
	require.NoError(t, db.Create(&model.ForwardRule{ID: 9, Name: "legacy", RelayNodeID: 1, ListenPort: 30000, ExitNodeID: 2, TargetHost: "198.51.100.9", TargetPort: 80}).Error)
}

func configureNodeX(t *testing.T, db *gorm.DB, baseURL string) {
	t.Helper()
	configs := NewSystemConfigService(db)
	require.NoError(t, configs.Set(forwardRuntimeNodeXBaseURLConfigKey, baseURL, "string", "forward", ""))
	require.NoError(t, configs.Set(forwardRuntimeNodeXTokenConfigKey, "nodex-token", "string", "forward", ""))
}

func TestLegacyForwardNodeXCleaner(t *testing.T) {
	db := newLegacyCleanupDB(t)
	seedLegacyNodeX(t, db)
	ctx := context.Background()
	cleaner := &LegacyForwardNodeXCleaner{DB: db}
	hk := forwardlegacy.NodeInfo{ID: 1, Ref: "forward-1", Name: "hk", Host: "192.0.2.1"}

	// Not configured: the resources cannot be deleted.
	result := cleaner.CleanNode(ctx, hk)
	assert.Equal(t, forwardlegacy.StateUnreachable, result.State)
	assert.Contains(t, result.Detail, "NodeX is not configured")

	recorder := &nodeXRecorder{}
	server := httptest.NewServer(recorder)
	t.Cleanup(server.Close)
	configureNodeX(t, db, server.URL)

	result = cleaner.CleanNode(ctx, hk)
	assert.Equal(t, forwardlegacy.StateClean, result.State, result.Detail)
	assert.Equal(t, 2, result.Items)
	require.Len(t, recorder.requests, 2)
	for _, request := range recorder.requests {
		assert.Equal(t, model.ForwardRuntimeJobActionDelete, request.Action)
		assert.Equal(t, model.ForwardRuntimeBackendGost, request.Backend)
	}
	assert.Equal(t, uint(7), recorder.requests[0].PanelForward.Forward.ID)
	assert.Equal(t, "node-token-1", recorder.requests[0].PanelForward.IngressNode.APIToken)
	assert.Equal(t, uint(9), recorder.requests[1].LegacyRule.Rule.ID)
	assert.Equal(t, "Bearer nodex-token", recorder.tokens[0])

	// The exit node of the legacy rule is cleaned with it.
	result = cleaner.CleanNode(ctx, forwardlegacy.NodeInfo{ID: 2, Ref: "forward-2", Name: "jp"})
	assert.Equal(t, forwardlegacy.StateClean, result.State)
	assert.Equal(t, 1, result.Items)

	// A node nothing references needs nothing.
	result = cleaner.CleanNode(ctx, forwardlegacy.NodeInfo{ID: 3, Ref: "forward-3", Name: "idle"})
	assert.Equal(t, forwardlegacy.PathNotNeeded, result.State)

	// NodeX refusing a delete leaves the node dirty.
	recorder.status = http.StatusInternalServerError
	result = cleaner.CleanNode(ctx, hk)
	assert.Equal(t, forwardlegacy.StateDirty, result.State)
	assert.Contains(t, result.Detail, "gost refused")

	// NodeX down: unreachable.
	server.Close()
	result = cleaner.CleanNode(ctx, hk)
	assert.Equal(t, forwardlegacy.StateUnreachable, result.State, result.Detail)
}

func TestLegacyForwardNodeXCleanerAfterDrop(t *testing.T) {
	db := newLegacyCleanupDB(t)
	require.NoError(t, db.Migrator().DropTable(&model.Forward{}, &model.ForwardTunnel{}, &model.ForwardRule{}))
	result := (&LegacyForwardNodeXCleaner{DB: db}).CleanNode(context.Background(), forwardlegacy.NodeInfo{ID: 1})
	assert.Equal(t, forwardlegacy.PathNotNeeded, result.State)
}

func TestLegacyForwardAnsibleCleaner(t *testing.T) {
	db := newLegacyCleanupDB(t)
	ctx := context.Background()
	// Node 1 is an Ansible machine; node 2 executed a flux forward on the
	// iptables backend; node 3 neither.
	require.NoError(t, db.Create(&model.ForwardNode{ID: 1, Name: "ansible-1", Type: "relay", Host: "203.0.113.1", Tags: `["ansible-machine"]`, Enabled: true}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 2, Name: "ipt", Type: "exit", Host: "203.0.113.2", APIPort: 9000, APIToken: "t-2", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 3, Name: "nodex", Type: "exit", Host: "203.0.113.3", APIPort: 9000, APIToken: "t-3", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.ForwardTunnel{ID: 5, Name: "ipt", InNodeID: 2, Protocol: "both", Type: 1, Status: 1}).Error)
	require.NoError(t, db.Create(&model.Forward{ID: 7, UserID: 1, Name: "nat", TunnelID: 5, InPort: 20001, RemoteAddr: "198.51.100.7:443,198.51.100.8:8443", RuntimeBackend: "iptables_ansible"}).Error)

	var calls [][]string
	output, runErr := "", error(nil)
	cleaner := &LegacyForwardAnsibleCleaner{DB: db, Run: func(_ context.Context, command string, args []string, _ string, _ map[string]string) (string, error) {
		assert.Equal(t, "ansible-playbook", command)
		calls = append(calls, args)
		return output, runErr
	}}

	result := cleaner.CleanNode(ctx, forwardlegacy.NodeInfo{ID: 1, Name: "ansible-1", Host: "203.0.113.1"})
	assert.Equal(t, forwardlegacy.StateClean, result.State, result.Detail)
	require.Len(t, calls, 1)
	joined := strings.Join(calls[0], " ")
	assert.Contains(t, joined, "--limit 203.0.113.1")
	assert.Contains(t, joined, `{"legacy_masquerade": []}`)
	assert.True(t, strings.HasSuffix(calls[0][len(calls[0])-1], "forward_legacy_cleanup.yml"))

	result = cleaner.CleanNode(ctx, forwardlegacy.NodeInfo{ID: 2, Name: "ipt", Host: "203.0.113.2"})
	assert.Equal(t, forwardlegacy.StateClean, result.State)
	assert.Equal(t, 1, result.Items)
	assert.Contains(t, strings.Join(calls[1], " "),
		`{"legacy_masquerade": ["tcp,198.51.100.7,443","udp,198.51.100.7,443","tcp,198.51.100.8,8443","udp,198.51.100.8,8443"]}`)

	result = cleaner.CleanNode(ctx, forwardlegacy.NodeInfo{ID: 3, Name: "nodex", Host: "203.0.113.3"})
	assert.Equal(t, forwardlegacy.PathNotNeeded, result.State)
	assert.Len(t, calls, 2)

	output, runErr = "fatal: [203.0.113.1]: UNREACHABLE! => {}", errors.New("exit status 4")
	result = cleaner.CleanNode(ctx, forwardlegacy.NodeInfo{ID: 1, Name: "ansible-1", Host: "203.0.113.1"})
	assert.Equal(t, forwardlegacy.StateUnreachable, result.State)

	output, runErr = "LEGACY_FORWARD_DIRTY: nft table inet v2b_forward", errors.New("exit status 2")
	result = cleaner.CleanNode(ctx, forwardlegacy.NodeInfo{ID: 1, Name: "ansible-1", Host: "203.0.113.1"})
	assert.Equal(t, forwardlegacy.StateDirty, result.State)
	assert.Contains(t, result.Detail, "LEGACY_FORWARD_DIRTY")
}

// TestLegacyForwardCleanupScript runs the playbook's script against fake
// nft, iptables and systemctl commands that keep their state in files.
func TestLegacyForwardCleanupScript(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the cleanup script runs on Linux hosts")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	script, err := filepath.Abs("../../config/deploy/ansible/playbooks/files/forward_legacy_cleanup.sh")
	require.NoError(t, err)
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o700))
		return path
	}
	tables := write("tables", "inet v2b_forward\nip anixops_forward\ninet anixops_fwd\n")
	rules := write("rules", "-N V2B_FWD_7_TCP\n-N DOCKER\n-A PREROUTING -p tcp -m tcp --dport 20001 -j V2B_FWD_7_TCP\n"+
		"-A PREROUTING -j DOCKER\n-A POSTROUTING -d 198.51.100.7/32 -p tcp -m tcp --dport 443 -j MASQUERADE\n")
	nft := write("nft", `#!/usr/bin/env bash
state=`+tables+`
case "$1" in
  list) grep -qx "$3 $4" "$state" ;;
  delete) grep -vx "$3 $4" "$state" > "$state.new"; mv "$state.new" "$state" ;;
esac
`)
	iptables := write("iptables", `#!/usr/bin/env bash
state=`+rules+`
shift 2 # -t nat
case "$1" in
  -S) if [[ -n "${2:-}" ]]; then grep -- "^-A $2 " "$state" || true; else cat "$state"; fi ;;
  -D) shift; rule="-A $*"
      if [[ "$rule" == *"-j V2B_FWD_"* ]]; then grep -vxF -- "$rule" "$state" > "$state.new"; mv "$state.new" "$state"; else
        [[ "$1 $3 $5" == "POSTROUTING tcp 198.51.100.7" ]] && { grep -v MASQUERADE "$state" > "$state.new"; mv "$state.new" "$state"; }; fi ;;
  -C) [[ "$2 $4 $6" == "POSTROUTING tcp 198.51.100.7" ]] && grep -q MASQUERADE "$state" ;;
  -F) ;;
  -X) grep -vx -- "-N $2" "$state" > "$state.new"; mv "$state.new" "$state" ;;
esac
`)
	root := filepath.Join(dir, "root")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "etc/v2board-forward-agent"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "etc/systemd/system"), 0o700))
	write("root/etc/systemd/system/v2forward-agent.service", "[Unit]\n")
	systemctl := write("systemctl", "#!/usr/bin/env bash\nexit 0\n")

	command := exec.Command(bash, script, "tcp,198.51.100.7,443", "udp,198.51.100.7,443") // #nosec G204 -- the repository's own script.
	command.Env = append(os.Environ(), "NFT="+nft, "IPTABLES="+iptables, "SYSTEMCTL="+systemctl, "LEGACY_ROOT="+root)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Contains(t, string(output), "LEGACY_FORWARD_CLEAN")
	remainingTables, err := os.ReadFile(tables)
	require.NoError(t, err)
	assert.Equal(t, "inet anixops_fwd\n", string(remainingTables), "only the legacy tables are removed")
	remainingRules, err := os.ReadFile(rules)
	require.NoError(t, err)
	assert.Equal(t, "-N DOCKER\n-A PREROUTING -j DOCKER\n", string(remainingRules), "only the legacy chains and rules are removed")
	assert.NoDirExists(t, filepath.Join(root, "etc/v2board-forward-agent"))
	assert.NoFileExists(t, filepath.Join(root, "etc/systemd/system/v2forward-agent.service"))

	// A table that cannot be deleted is reported and fails the run.
	require.NoError(t, os.WriteFile(tables, []byte("ip v2b_forward\n"), 0o600))
	stuck := write("nft-stuck", "#!/usr/bin/env bash\nstate="+tables+"\n[[ \"$1\" == list ]] && grep -qx \"$3 $4\" \"$state\"\n[[ \"$1\" == delete ]] && exit 1\nexit 0\n")
	command = exec.Command(bash, script) // #nosec G204 -- the repository's own script.
	command.Env = append(os.Environ(), "NFT="+stuck, "IPTABLES="+iptables, "SYSTEMCTL="+systemctl, "LEGACY_ROOT="+root)
	output, err = command.CombinedOutput()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 3, exitErr.ExitCode())
	assert.Contains(t, string(output), "LEGACY_FORWARD_DIRTY: nft table ip v2b_forward")
}

// After the v4.2 upgrade dropped the flux tables the latency prober still
// probes the forward and proxy nodes.
func TestForwardLatencyProberWithoutFluxTables(t *testing.T) {
	db := newLegacyCleanupDB(t)
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	require.NoError(t, db.Migrator().DropTable(&model.Forward{}, &model.ForwardTunnel{}))
	require.NoError(t, db.Create(&model.ForwardNode{ID: 1, Name: "hk", Host: "192.0.2.1", Port: 22, Enabled: true}).Error)
	prober := &ForwardLatencyProber{db: db}
	targets, err := prober.enumerateTargets(context.Background())
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, model.LatencyTargetTypeForwardNode, targets[0].TargetType)
}
