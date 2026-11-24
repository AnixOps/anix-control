package agente2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The suite's binaries: Control (./cmd/server) from this checkout with the
// built-in package set signed by the suite's key, and the real Agent from
// the pinned anix-agent checkout (ANIXOPS_AGENT_ROOT).
// ANIXOPS_AGENT_E2E_BIN_DIR supplies an Agent and anixcore built before
// (the CI job builds them once); Control is always built here, because its
// embedded packages are signed with the key this run generates.
type binaries struct {
	Control      string
	Agent        string
	Anixcore     string
	AgentVersion string
	AgentCommit  string
	ControlRoot  string
	AgentRoot    string
}

// agentBuildTags are the cores compiled into the Agent under test: anixcore
// drives the `anixcore` binary that serves the VLESS protocol the suite's
// node runs (wireguard is always in).
const agentBuildTags = "anixcore"

func controlRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func agentRoot(t *testing.T, control string) string {
	t.Helper()
	if configured := strings.TrimSpace(os.Getenv("ANIXOPS_AGENT_ROOT")); configured != "" {
		return filepath.Clean(configured)
	}
	for _, candidate := range []string{
		filepath.Join(control, "anix-agent"),
		filepath.Join(filepath.Dir(control), "anix-agent"),
	} {
		if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
			return candidate
		}
	}
	t.Skip("no anix-agent checkout: set ANIXOPS_AGENT_ROOT")
	return ""
}

func agentGo() string {
	if configured := strings.TrimSpace(os.Getenv("ANIXOPS_AGENT_GO")); configured != "" {
		return configured
	}
	return "go"
}

func buildEnv(extra ...string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env, "GOWORK=off", "GOEXPERIMENT=jsonv2", "CGO_ENABLED=0")
	return append(env, extra...)
}

func buildBinaries(t *testing.T, dir string, keys testPKI) binaries {
	t.Helper()
	out := binaries{ControlRoot: controlRoot(t)}
	out.AgentRoot = agentRoot(t, out.ControlRoot)
	out.AgentCommit = gitHead(out.AgentRoot)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	out.Control = buildControl(t, out.ControlRoot, dir, keys)
	if prebuilt := strings.TrimSpace(os.Getenv("ANIXOPS_AGENT_E2E_BIN_DIR")); prebuilt != "" {
		out.Agent = filepath.Join(prebuilt, "anix-agent")
		out.Anixcore = filepath.Join(prebuilt, "anixcore")
		for _, path := range []string{out.Agent, out.Anixcore} {
			_, err := os.Stat(path)
			require.NoError(t, err, "ANIXOPS_AGENT_E2E_BIN_DIR lacks %s", filepath.Base(path))
		}
	} else {
		// anixcore is built from networkcore_anixops (cargo build -p
		// anixcore), not here: point at a build of it.
		out.Anixcore = strings.TrimSpace(os.Getenv("ANIXOPS_ANIXCORE_BIN"))
		if out.Anixcore == "" {
			t.Skip("no anixcore binary: set ANIXOPS_ANIXCORE_BIN (cargo build -p anixcore in networkcore_anixops)")
		}
		out.Agent = filepath.Join(dir, "anix-agent")
		version := "-X github.com/AnixOps/anix-agent/v5/cmd.version=" + agentE2EVersion
		goBuild(t, agentGo(), out.AgentRoot, out.Agent, buildEnv(), "-trimpath", "-tags", agentBuildTags, "-ldflags", version, ".")
	}
	out.AgentVersion = agentE2EVersion
	return out
}

// buildControl builds the built-in package set signed with the suite's key
// (packages/shared/build_package.py --builtin, as the release does with the
// real key) and compiles ./cmd/server with it in place of
// internal/builtinpkg/dist, without touching the checkout.
func buildControl(t *testing.T, root, dir string, keys testPKI) string {
	t.Helper()
	set := filepath.Join(dir, "builtin-packages")
	cmd := exec.Command("python3", "packages/shared/build_package.py", "--all", "--builtin",
		"--version", packageVersion, "--out", set, "--signing-key", keys.SigningPrivateKeyPath)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "build the built-in package set: %s", output)
	entries, err := os.ReadDir(set)
	require.NoError(t, err)
	replace := map[string]string{}
	for _, entry := range entries {
		replace[filepath.Join(root, "internal", "builtinpkg", "dist", entry.Name())] = filepath.Join(set, entry.Name())
	}
	overlay, err := json.Marshal(map[string]any{"Replace": replace})
	require.NoError(t, err)
	overlayPath := filepath.Join(dir, "builtin-packages.overlay.json")
	require.NoError(t, os.WriteFile(overlayPath, overlay, 0o600))
	binary := filepath.Join(dir, "anix-control")
	goBuild(t, "go", root, binary, buildEnv(), "-overlay", overlayPath, "./cmd/server")
	return binary
}

// agentE2EVersion is the version the suite's Agent build reports.
const agentE2EVersion = "5.0.11-e2e"

func gitHead(dir string) string {
	output, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

// packageVersion is the version of the built-in packages the suite's
// Control embeds.
const packageVersion = "4.0.0"
