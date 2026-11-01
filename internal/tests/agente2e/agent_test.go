package agente2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// agentNode is the real Agent of the suite: a credential-only proxy node as
// Control's installer writes it (no ApiKey, a one-time enrollment
// credential, the stream data plane over mTLS, the anixcore core),
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
	shimDir        string
	shimLog        string
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
		shimDir:        filepath.Join(dir, "shim"),
		shimLog:        filepath.Join(dir, "shim.log"),
	}
	for _, path := range []string{a.pkiDir, a.streamDir, a.forwardDir, a.pluginRoot, a.shimDir} {
		require.NoError(t, os.MkdirAll(path, 0o700))
	}
	a.writeConfig(t, socketBase)
	return a
}

type agentSettings struct {
	GRPCHost         string
	SigningPublicKey string
	RealNftables     bool
	AnixcoreBinary   string
}

func (a *agentNode) writeConfig(t *testing.T, socketBase string) {
	t.Helper()
	settings := currentAgentSettings
	config := map[string]any{
		"Log": map[string]any{"Level": "debug", "Output": ""},
		"Cores": []any{map[string]any{
			"Type":                "anixcore",
			"BinaryPath":          settings.AnixcoreBinary,
			"RuntimeDir":          filepath.Join(a.dir, "anixcore"),
			"AllowPrivateTargets": true,
		}},
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
// (systemctl, journalctl, ss), so the Agent never touches the host's
// systemd units, and logs every call for the assertions:
//
//   - systemctl reports every unit inactive (the updater path included: no
//     Control-pushed upgrades here);
//   - ss -V and the listening-socket list come from the real ss when the
//     host has it.
func (a *agentNode) writeShims(t *testing.T) {
	t.Helper()
	log := a.shimLog
	realSS, _ := exec.LookPath("ss")
	systemctl := `#!/bin/sh
echo "systemctl $*" >> "` + log + `"
case "$1" in
  show)
    echo "LoadState=not-found"
    echo "ActiveState=inactive"; echo "InvocationID="; echo "MainPID=0"
    exit 0 ;;
  is-active) echo "inactive"; exit 3 ;;
  status) echo "   Active: inactive (dead)"; exit 3 ;;
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
	shims := map[string]string{
		"systemctl": systemctl,
		"journalctl": `#!/bin/sh
echo "journalctl $*" >> "` + log + `"
echo "2026-10-04T00:00:00+0000 e2e anix-agent[1]: agent E2E log line"
`,
		"ss": ss,
	}
	for name, body := range shims {
		require.NoError(t, os.WriteFile(filepath.Join(a.shimDir, name), []byte(body), 0o755))
	}
}

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
