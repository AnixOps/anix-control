package agentinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readyInput is a Control that can hand out a working install command.
func readyInput() ReadinessInput {
	return ReadinessInput{
		PublicURL: "https://panel.example.com", GRPCEnabled: true, GRPCPort: 50051,
		GRPCTLSCertFile: "/etc/anix/grpc.crt", GRPCTLSKeyFile: "/etc/anix/grpc.key",
		BuiltinPKI: true, CAKEK: true, MTLSMode: "required",
		AgentVersion: "v4.2.0", ReleaseVersion: "4.2.1",
	}
}

func checkOf(t *testing.T, readiness Readiness, id string) ReadinessCheck {
	t.Helper()
	for _, check := range readiness.Checks {
		if check.ID == id {
			return check
		}
	}
	require.FailNow(t, "no such check", id)
	return ReadinessCheck{}
}

func TestReadinessOfAConfiguredControl(t *testing.T) {
	readiness := CheckReadiness(readyInput())

	assert.True(t, readiness.Ready)
	ids := make([]string, 0, len(readiness.Checks))
	for _, check := range readiness.Checks {
		ids = append(ids, check.ID)
		assert.True(t, check.OK, check.ID+": "+check.Message)
		assert.Empty(t, check.Fix, check.ID)
		assert.NotEmpty(t, check.Message, check.ID)
	}
	assert.Equal(t, []string{CheckPublicURL, CheckGRPC, CheckCA, CheckMTLSMode, CheckAgentRelease, CheckScriptSignature}, ids)
	assert.Contains(t, checkOf(t, readiness, CheckGRPC).Message, "panel.example.com:50051")
}

func TestReadinessPublicURL(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate  func(*ReadinessInput)
		ok      bool
		message string
	}{
		"unset and no request origin": {func(in *ReadinessInput) { in.PublicURL = "" }, false, "not set"},
		"plain http":                  {func(in *ReadinessInput) { in.PublicURL = "http://panel.example.com" }, false, "not https"},
		"loopback":                    {func(in *ReadinessInput) { in.PublicURL = "https://127.0.0.1:8443" }, false, "loopback"},
		"localhost":                   {func(in *ReadinessInput) { in.PublicURL = "https://localhost" }, false, "loopback"},
		"no host":                     {func(in *ReadinessInput) { in.PublicURL = "https://" }, false, "not an address"},
		"https request origin": {func(in *ReadinessInput) {
			in.PublicURL, in.RequestOrigin = "", "https://panel.example.com/"
		}, true, "this page was opened with"},
		"http request origin": {func(in *ReadinessInput) {
			in.PublicURL, in.RequestOrigin = "", "http://10.0.0.5:8080"
		}, false, "not https"},
	} {
		t.Run(name, func(t *testing.T) {
			in := readyInput()
			tc.mutate(&in)
			readiness := CheckReadiness(in)
			check := checkOf(t, readiness, CheckPublicURL)
			assert.Equal(t, tc.ok, check.OK, check.Message)
			assert.Equal(t, LevelError, check.Level)
			assert.Contains(t, check.Message, tc.message)
			assert.Equal(t, tc.ok, check.Fix == "")
			assert.Equal(t, tc.ok, readiness.Ready)
		})
	}
}

func TestReadinessGRPC(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate   func(*ReadinessInput)
		ok       bool
		contains []string
	}{
		"explicit target":    {func(in *ReadinessInput) { in.GRPCTarget = "grpc.example.com:8443" }, true, []string{"grpc.example.com:8443"}},
		"default port":       {func(in *ReadinessInput) { in.GRPCPort = 0 }, true, []string{"panel.example.com:50051"}},
		"listener off":       {func(in *ReadinessInput) { in.GRPCEnabled = false }, false, []string{"gRPC listener is off", "grpc.enabled"}},
		"no certificate":     {func(in *ReadinessInput) { in.GRPCTLSCertFile = "" }, false, []string{"no TLS certificate", "grpc.tls_cert_file"}},
		"no key":             {func(in *ReadinessInput) { in.GRPCTLSKeyFile = " " }, false, []string{"no TLS certificate"}},
		"target not host":    {func(in *ReadinessInput) { in.GRPCTarget = "grpc.example.com" }, false, []string{"not host:port", "agent_install.grpc_target"}},
		"loopback target":    {func(in *ReadinessInput) { in.GRPCTarget = "127.0.0.1:50051" }, false, []string{"loopback"}},
		"nothing to resolve": {func(in *ReadinessInput) { in.PublicURL = "" }, false, []string{"cannot work out the gRPC address"}},
		"everything wrong": {func(in *ReadinessInput) {
			in.GRPCEnabled, in.GRPCTLSCertFile = false, ""
		}, false, []string{"listener is off", "no TLS certificate"}},
	} {
		t.Run(name, func(t *testing.T) {
			in := readyInput()
			tc.mutate(&in)
			check := checkOf(t, CheckReadiness(in), CheckGRPC)
			assert.Equal(t, tc.ok, check.OK, check.Message)
			assert.Equal(t, LevelError, check.Level)
			for _, part := range tc.contains {
				assert.Contains(t, check.Message+" "+check.Fix, part)
			}
		})
	}
}

func TestReadinessCAAndMTLSMode(t *testing.T) {
	in := readyInput()
	in.CAKEK = false
	readiness := CheckReadiness(in)
	ca := checkOf(t, readiness, CheckCA)
	assert.False(t, ca.OK)
	assert.Contains(t, ca.Message, "module_runtime.ca_kek is not set")
	assert.Contains(t, ca.Fix, "openssl rand -base64 32")
	assert.False(t, readiness.Ready)

	in = readyInput()
	in.CAKEKProblem = "the key must be 32 bytes"
	ca = checkOf(t, CheckReadiness(in), CheckCA)
	assert.False(t, ca.OK)
	assert.Contains(t, ca.Message, "the key must be 32 bytes")

	in = readyInput()
	in.BuiltinPKI = false
	assert.Contains(t, checkOf(t, CheckReadiness(in), CheckCA).Message, "external")

	for mode, ok := range map[string]bool{"": true, "required": true, "preferred": true, "optional": true, "off": false} {
		in = readyInput()
		in.MTLSMode = mode
		readiness = CheckReadiness(in)
		check := checkOf(t, readiness, CheckMTLSMode)
		assert.Equal(t, ok, check.OK, mode)
		assert.Equal(t, ok, readiness.Ready, mode)
		if !ok {
			assert.Contains(t, check.Fix, "required")
		}
	}
}

func TestReadinessAgentRelease(t *testing.T) {
	// Pinned, with no local release and no mirror: GitHub releases.
	in := readyInput()
	check := checkOf(t, CheckReadiness(in), CheckAgentRelease)
	assert.True(t, check.OK)
	assert.Contains(t, check.Message, "v4.2.0")
	assert.Contains(t, check.Message, "GitHub releases")

	// Not pinned: Control's own version, with a note that it cannot be verified.
	in.AgentVersion = ""
	check = checkOf(t, CheckReadiness(in), CheckAgentRelease)
	assert.True(t, check.OK)
	assert.Contains(t, check.Message, "v4.2.1")
	assert.Contains(t, check.Message, "cannot check")

	// A development build knows no release: an error.
	in.ReleaseVersion = "dev"
	readiness := CheckReadiness(in)
	check = checkOf(t, readiness, CheckAgentRelease)
	assert.False(t, check.OK)
	assert.Equal(t, LevelError, check.Level)
	assert.Contains(t, check.Fix, "agent_install.agent_version")
	assert.False(t, readiness.Ready)

	// A mirror counts as a source.
	in = readyInput()
	in.CNMirrorURL = "https://mirror.example.com/anix-agent"
	assert.Contains(t, checkOf(t, CheckReadiness(in), CheckAgentRelease).Message, "mirror")

	// An artifact directory that does not hold the release is a warning:
	// the command still works from GitHub.
	in = readyInput()
	in.ArtifactDir = t.TempDir()
	readiness = CheckReadiness(in)
	check = checkOf(t, readiness, CheckAgentRelease)
	assert.False(t, check.OK)
	assert.Equal(t, LevelWarning, check.Level)
	assert.Contains(t, check.Fix, filepath.Join(strings.TrimRight(in.ArtifactDir, "/"), "v4.2.0"))
	assert.True(t, readiness.Ready, "a warning does not block the command")

	// With both architectures and their digests the Control serves it.
	release := filepath.Join(in.ArtifactDir, "v4.2.0")
	require.NoError(t, os.MkdirAll(release, 0o750))
	for _, asset := range []string{"anix-agent-linux-64.zip", "anix-agent-linux-arm64-v8a.zip"} {
		require.NoError(t, os.WriteFile(filepath.Join(release, asset), []byte("zip"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(release, asset+".dgst"),
			[]byte("SHA2-256= "+strings.Repeat("ab", 32)+"\n"), 0o600))
	}
	check = checkOf(t, CheckReadiness(in), CheckAgentRelease)
	assert.True(t, check.OK)
	assert.Contains(t, check.Message, "served by this Control")
}

func TestReadinessScriptSignature(t *testing.T) {
	// Not configured: nothing to verify, so it passes.
	check := checkOf(t, CheckReadiness(readyInput()), CheckScriptSignature)
	assert.True(t, check.OK)
	assert.Equal(t, LevelWarning, check.Level)
	assert.Contains(t, check.Message, "unsigned")

	private, public := testKey(t)
	dir := t.TempDir()
	good := filepath.Join(dir, "install.sh.sig")
	require.NoError(t, os.WriteFile(good, sign(private, Script), 0o600))
	stale := filepath.Join(dir, "stale.sig")
	require.NoError(t, os.WriteFile(stale, sign(private, []byte("an older script")), 0o600))

	in := readyInput()
	in.SignatureFile, in.OfficialPublicKey = good, public
	check = checkOf(t, CheckReadiness(in), CheckScriptSignature)
	assert.True(t, check.OK, check.Message)
	assert.Contains(t, check.Message, "verifies")

	// A configured signature that does not verify is a warning, not a block.
	for _, path := range []string{stale, filepath.Join(dir, "missing.sig")} {
		in.SignatureFile = path
		readiness := CheckReadiness(in)
		check = checkOf(t, readiness, CheckScriptSignature)
		assert.False(t, check.OK, path)
		assert.Equal(t, LevelWarning, check.Level)
		assert.NotEmpty(t, check.Fix)
		assert.True(t, readiness.Ready)
	}
}
