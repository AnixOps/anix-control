package agente2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// agentNode is the real Agent of the suite: a credential-only proxy node as
// Control's installer writes it (no ApiKey, a one-time enrollment
// credential, the stream data plane over mTLS, every core compiled in),
// plus the plugin supervisor (machine-telemetry) and the forward component
// of a proxy node.
type agentNode struct {
	proc           *process
	nodeID         uint
	dir            string
	configPath     string
	credentialPath string
	pkiDir         string
	streamDir      string
	forwardDir     string
	pluginRoot     string
	gostDir        string
	shimDir        string
	shimLog        string
	gostPIDFile    string
}

// agentLayout lays the Agent's directories out under dir and the plugin
// sockets under a short directory (a unix socket path is at most 107
// bytes).
func newAgentNode(t *testing.T, dir string, nodeID uint) *agentNode {
	t.Helper()
	socketBase, err := os.MkdirTemp("", "ae2e")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(socketBase) })
	a := &agentNode{
		nodeID:         nodeID,
		dir:            dir,
		configPath:     filepath.Join(dir, "config.json"),
		credentialPath: filepath.Join(dir, "enroll.token"),
		pkiDir:         filepath.Join(dir, "pki"),
		streamDir:      filepath.Join(dir, "stream"),
		forwardDir:     filepath.Join(dir, "forward"),
		pluginRoot:     filepath.Join(dir, "plugins"),
		gostDir:        filepath.Join(dir, "gost"),
		shimDir:        filepath.Join(dir, "shim"),
		shimLog:        filepath.Join(dir, "shim.log"),
	}
	for _, path := range []string{a.pkiDir, a.streamDir, a.forwardDir, a.pluginRoot, a.gostDir, a.shimDir} {
		require.NoError(t, os.MkdirAll(path, 0o700))
	}
	currentAgentSettings.gostRuntimeDir = filepath.Join(socketBase, "gost")
	a.writeConfig(t, socketBase)
	return a
}

type agentSettings struct {
	GRPCHost         string
	SigningPublicKey string
	RealNftables     bool
	gostRuntimeDir   string
}

func (a *agentNode) writeConfig(t *testing.T, socketBase string) {
	t.Helper()
	settings := currentAgentSettings
	config := map[string]any{
		"Log":   map[string]any{"Level": "debug", "Output": ""},
		"Cores": []any{},
		"Nodes": []any{map[string]any{
			"ApiHost":                   "https://localhost",
			"Transport":                 "grpc",
			"GRPCHost":                  settings.GRPCHost,
			"GRPCUseTLS":                true,
			"GRPCServerName":            "localhost",
			"GRPCKeepalive":             30,
			"AgentControlEnabled":       true,
			"AgentControlAllowInsecure": false,
			"AgentNode":                 fmt.Sprintf("proxy-%d", a.nodeID),
			"NodeID":                    a.nodeID,
			"Timeout":                   10,
			"AgentIdentity": map[string]any{
				"Enroll": "auto", "CertDir": a.pkiDir, "EnrollCredentialFile": a.credentialPath,
			},
			"AgentStream":             map[string]any{"StateDir": a.streamDir},
			"PluginSupervisorEnabled": true,
			"PluginRoot":              a.pluginRoot,
			"PluginSocketDir":         socketBase,
			"PluginOfficialPublicKey": settings.SigningPublicKey,
			"MaintenanceEnvironment":  "development",
		}},
		"Forward": map[string]any{
			"Enable":   true,
			"NodeKind": "proxy",
			"StateDir": a.forwardDir,
			"Nftables": map[string]any{"Disable": !settings.RealNftables},
			"Gost": map[string]any{
				"Binary": filepath.Join(a.shimDir, "gost"), "Dir": a.gostDir, "RuntimeDir": settings.gostRuntimeDir,
			},
		},
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(a.configPath, encoded, 0o600))
	a.writeShims(t)
}

// currentAgentSettings is set by the suite before newAgentNode.
var currentAgentSettings agentSettings

// writeShims puts stand-ins for the host tools the Agent runs on its PATH
// (systemctl, journalctl, ss, gost), so the Agent never touches the host's
// systemd units, and logs every call for the assertions:
//
//   - systemctl answers for anixops-gost.service as the installer's unit
//     would: loaded and owned by the driver. With a real gost
//     (ANIXOPS_GOST_BIN, the pinned release CI downloads) start, reload
//     and stop run it as a detached process with the driver's
//     configuration, as the unit's ExecStart and ExecReload do; without
//     one the unit stays inactive and gost hops fail to apply.
//   - the updater path is inactive (no Control-pushed upgrades here);
//   - ss -V and the listening-socket list come from the real ss when the
//     host has it.
func (a *agentNode) writeShims(t *testing.T) {
	t.Helper()
	log := a.shimLog
	gostBin := strings.TrimSpace(os.Getenv("ANIXOPS_GOST_BIN"))
	realSS, _ := exec.LookPath("ss")
	a.gostPIDFile = filepath.Join(a.dir, "gost.pid")
	runtimeDir := currentAgentSettings.gostRuntimeDir
	config := filepath.Join(a.gostDir, "gost.json")
	systemctl := `#!/bin/sh
echo "systemctl $*" >> "` + log + `"
GOST="` + gostBin + `"
PIDFILE="` + a.gostPIDFile + `"
running() { [ -s "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; }
case "$1" in
  show)
    echo "LoadState=loaded"
    echo "Description=AnixOps forward gost (anixops-forward-driver gost v1)"
    if running; then echo "ActiveState=active"; echo "InvocationID=e2e$(cat "$PIDFILE")"; echo "MainPID=$(cat "$PIDFILE")";
    else echo "ActiveState=inactive"; echo "InvocationID="; echo "MainPID=0"; fi
    exit 0 ;;
  start)
    [ -n "$GOST" ] || exit 0
    running && exit 0
    mkdir -p "` + runtimeDir + `"
    [ -f "` + config + `" ] || exit 0
    setsid "$GOST" -C "` + config + `" >> "` + filepath.Join(a.dir, "gost.log") + `" 2>&1 < /dev/null &
    echo $! > "$PIDFILE"
    sleep 0.3
    exit 0 ;;
  reload) running && kill -HUP "$(cat "$PIDFILE")"; exit 0 ;;
  stop) running && kill "$(cat "$PIDFILE")"; rm -f "$PIDFILE"; exit 0 ;;
  is-active) echo "inactive"; exit 3 ;;
  status)
    echo "* anixops-gost.service - AnixOps forward gost (agent E2E stand-in)"
    if running; then echo "   Active: active (running)"; exit 0; fi
    echo "   Active: inactive (dead)"; exit 3 ;;
  *) exit 0 ;;
esac
`
	ss := `#!/bin/sh
echo "ss $*" >> "` + log + `"
if [ -n "` + realSS + `" ]; then exec "` + realSS + `" "$@"; fi
case "$1" in
  -V) echo "ss utility, iproute2-6.1.0" ;;
esac
exit 0
`
	gost := `#!/bin/sh
echo "gost $*" >> "` + log + `"
if [ -n "` + gostBin + `" ]; then exec "` + gostBin + `" "$@"; fi
case "$1" in
  -V) echo "gost v3.2.6 (agent E2E stand-in)" ;;
esac
exit 0
`
	shims := map[string]string{
		"systemctl": systemctl,
		"journalctl": `#!/bin/sh
echo "journalctl $*" >> "` + log + `"
echo "2026-10-04T00:00:00+0000 e2e anixops-gost[1]: agent E2E log line"
`,
		"ss":   ss,
		"gost": gost,
	}
	for name, body := range shims {
		require.NoError(t, os.WriteFile(filepath.Join(a.shimDir, name), []byte(body), 0o755))
	}
	t.Cleanup(a.stopGost)
}

// stopGost ends the detached gost the systemctl stand-in started.
func (a *agentNode) stopGost() {
	data, err := os.ReadFile(a.gostPIDFile)
	if err != nil {
		return
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// RealGost tells whether hops on the gost engine really run.
func RealGost() bool { return strings.TrimSpace(os.Getenv("ANIXOPS_GOST_BIN")) != "" }

// ShimCalls is what the Agent ran through the shims.
func (a *agentNode) ShimCalls() string {
	data, _ := os.ReadFile(a.shimLog)
	return string(data)
}

// WriteCredential writes a one-time enrollment credential where the Agent
// looks for it (mode 0600, as the installer writes it).
func (a *agentNode) WriteCredential(t *testing.T, credential string) {
	t.Helper()
	tmp := a.credentialPath + ".tmp"
	require.NoError(t, os.WriteFile(tmp, []byte(strings.TrimSpace(credential)+"\n"), 0o600))
	require.NoError(t, os.Rename(tmp, a.credentialPath))
}

// Env is the Agent's environment: the shims first on PATH, and Control's
// gRPC server CA as the system trust store.
func agentEnv(shimDir, serverCA string) []string {
	var env []string
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "PATH=") || strings.HasPrefix(entry, "SSL_CERT_FILE=") || strings.HasPrefix(entry, "SSL_CERT_DIR=") {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "PATH="+shimDir+":"+os.Getenv("PATH"), "SSL_CERT_FILE="+serverCA, "SSL_CERT_DIR=/nonexistent")
}

// spoolFiles lists the files of the Agent's data-plane state (the report
// spool among them).
func (a *agentNode) StreamFiles() []string {
	var files []string
	_ = filepath.Walk(a.streamDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(a.streamDir, path)
			files = append(files, fmt.Sprintf("%s:%d", rel, info.Size()))
		}
		return nil
	})
	return files
}
