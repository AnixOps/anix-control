package agente2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The suite's binaries: Control (./cmd/server) from this checkout, the real
// Agent and its machine-telemetry plugin from the pinned anix-agent
// checkout (ANIXOPS_AGENT_ROOT), and the signed packages Control installs.
// ANIXOPS_AGENT_E2E_BIN_DIR reuses binaries built before (the CI job builds
// them once as the runner user and runs the privileged lane under sudo).
type binaries struct {
	Control        string
	Agent          string
	TelemetryAMD64 string
	TelemetryARM64 string
	AgentVersion   string
	AgentCommit    string
	ControlRoot    string
	AgentRoot      string
}

// agentBuildTags are the cores compiled into the Agent under test: sing-box
// serves the VLESS protocol the suite's node runs (wireguard is always in).
const agentBuildTags = "sing"

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
		filepath.Join(control, "V2bX_AnixOps"),
		filepath.Join(filepath.Dir(control), "V2bX_AnixOps"),
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

func buildBinaries(t *testing.T, dir string) binaries {
	t.Helper()
	out := binaries{ControlRoot: controlRoot(t)}
	out.AgentRoot = agentRoot(t, out.ControlRoot)
	out.AgentCommit = gitHead(out.AgentRoot)
	prebuilt := strings.TrimSpace(os.Getenv("ANIXOPS_AGENT_E2E_BIN_DIR"))
	binDir := dir
	if prebuilt != "" {
		binDir = prebuilt
	}
	out.Control = filepath.Join(binDir, "anix-control")
	out.Agent = filepath.Join(binDir, "anix-agent")
	out.TelemetryAMD64 = filepath.Join(binDir, "machine-telemetry-linux-amd64")
	out.TelemetryARM64 = filepath.Join(binDir, "machine-telemetry-linux-arm64")
	if prebuilt != "" {
		for _, path := range []string{out.Control, out.Agent, out.TelemetryAMD64, out.TelemetryARM64} {
			_, err := os.Stat(path)
			require.NoError(t, err, "ANIXOPS_AGENT_E2E_BIN_DIR lacks %s", filepath.Base(path))
		}
	} else {
		require.NoError(t, os.MkdirAll(binDir, 0o755))
		goBuild(t, "go", out.ControlRoot, out.Control, buildEnv(), "./cmd/server")
		version := "-X github.com/AnixOps/anix-agent/v4/cmd.version=" + agentE2EVersion
		goBuild(t, agentGo(), out.AgentRoot, out.Agent, buildEnv(), "-trimpath", "-tags", agentBuildTags, "-ldflags", version, ".")
		goBuild(t, agentGo(), out.AgentRoot, out.TelemetryAMD64, buildEnv("GOOS=linux", "GOARCH=amd64"), "-trimpath", "-buildvcs=false", "./cmd/machine-telemetry")
		goBuild(t, agentGo(), out.AgentRoot, out.TelemetryARM64, buildEnv("GOOS=linux", "GOARCH=arm64"), "-trimpath", "-buildvcs=false", "./cmd/machine-telemetry")
	}
	out.AgentVersion = agentE2EVersion
	return out
}

// agentE2EVersion is the version the suite's Agent build reports.
const agentE2EVersion = "4.2.0-e2e"

func gitHead(dir string) string {
	output, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

// signedPackage is one official package built and signed with the suite's
// test key by packages/shared/build_package.py (as the release does with
// the real key).
type signedPackage struct {
	ID        string
	Version   string
	Dir       string
	Artifact  []byte
	Manifest  []byte
	Signature string
}

// packageVersion is the version of every package the suite installs.
const packageVersion = "4.0.0"

func buildSignedPackage(t *testing.T, bins binaries, keys testPKI, dir, id string) signedPackage {
	t.Helper()
	out := filepath.Join(dir, id)
	args := []string{
		"packages/shared/build_package.py",
		"--package", id,
		"--version", packageVersion,
		"--out", out,
		"--signing-key", keys.SigningPrivateKeyPath,
		"--formal-release",
		"--official-public-key", keys.SigningPublicKeyPath,
		"--platform", "linux/amd64",
		"--platform", "linux/arm64",
	}
	if id == "machine-telemetry" {
		args = append(args,
			"--agent-binary", "machine-telemetry@linux/amd64="+bins.TelemetryAMD64,
			"--agent-binary", "machine-telemetry@linux/arm64="+bins.TelemetryARM64)
	}
	cmd := exec.Command("python3", args...)
	cmd.Dir = bins.ControlRoot
	cmd.Env = buildEnv()
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "build signed %s: %s", id, output)
	stem := filepath.Join(out, id+"-"+packageVersion)
	artifact, err := os.ReadFile(stem + ".anxp")
	require.NoError(t, err)
	manifest, err := os.ReadFile(stem + ".manifest.json")
	require.NoError(t, err)
	signature, err := os.ReadFile(stem + ".manifest.sig")
	require.NoError(t, err)
	return signedPackage{ID: id, Version: packageVersion, Dir: out, Artifact: artifact, Manifest: manifest, Signature: strings.TrimSpace(string(signature))}
}
