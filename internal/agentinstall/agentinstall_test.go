package agentinstall

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKey(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return private, base64.StdEncoding.EncodeToString(public)
}

func sign(private ed25519.PrivateKey, data []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(private, data)) + "\n")
}

func TestVerifySignature(t *testing.T) {
	private, public := testKey(t)
	_, other := testKey(t)
	signature := sign(private, Script)

	require.NoError(t, VerifySignature(Script, signature, public))
	// Line-wrapped base64, as some tools write it, still verifies.
	wrapped := append(append([]byte{}, signature[:40]...), append([]byte("\n"), signature[40:]...)...)
	require.NoError(t, VerifySignature(Script, wrapped, public))

	tampered := append(append([]byte{}, Script...), '\n')
	for name, check := range map[string]error{
		"tampered script": VerifySignature(tampered, signature, public),
		"other key":       VerifySignature(Script, signature, other),
		"not base64":      VerifySignature(Script, []byte("!!"), public),
		"short signature": VerifySignature(Script, []byte(base64.StdEncoding.EncodeToString([]byte("short"))), public),
		"bad key":         VerifySignature(Script, signature, "AAAA"),
	} {
		assert.ErrorIs(t, check, ErrNoSignature, name)
	}
}

func TestLoadSignature(t *testing.T) {
	private, public := testKey(t)
	dir := t.TempDir()
	good := filepath.Join(dir, "install.sh.sig")
	require.NoError(t, os.WriteFile(good, sign(private, Script), 0o600))
	stale := filepath.Join(dir, "stale.sig")
	require.NoError(t, os.WriteFile(stale, sign(private, []byte("an older script")), 0o600))

	data, err := LoadSignature(good, public)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	for _, path := range []string{"", filepath.Join(dir, "missing.sig"), stale} {
		_, err := LoadSignature(path, public)
		assert.ErrorIs(t, err, ErrNoSignature, path)
	}
}

// The release key in the script is the official package root Control
// ships, and the comment's verification recipe uses the same key.
func TestScriptCarriesTheOfficialKey(t *testing.T) {
	prod, err := os.ReadFile(filepath.Join("..", "..", "config", "config.prod.yaml"))
	require.NoError(t, err)
	match := regexp.MustCompile(`official_public_key:\s*"([^"]+)"`).FindSubmatch(prod)
	require.NotNil(t, match)
	official := string(match[1])

	assert.Contains(t, string(Script), `readonly OFFICIAL_PUBLIC_KEY="`+official+`"`)
	raw, err := base64.StdEncoding.DecodeString(official)
	require.NoError(t, err)
	der := append([]byte{0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00}, raw...)
	assert.Contains(t, string(Script), "'"+base64.StdEncoding.EncodeToString(der)+"'")
}

// The script installs anixops-gost.service exactly as the gost driver
// renders it (contracts/forward/v1/gost).
func TestScriptGostUnitMatchesTheContract(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("..", "..", "contracts", "forward", "v1", "gost", "anixops-gost.service"))
	require.NoError(t, err)
	script := string(Script)
	start := strings.Index(script, "render_gost_unit() {\n  cat <<'EOF'\n")
	require.GreaterOrEqual(t, start, 0)
	body := script[start+len("render_gost_unit() {\n  cat <<'EOF'\n"):]
	end := strings.Index(body, "\nEOF\n")
	require.GreaterOrEqual(t, end, 0)
	assert.Equal(t, strings.TrimRight(string(golden), "\n"), body[:end])
}

// The legacy cleanup names exactly the tables and units of the upgrade
// order (forward-sdk.md, section 10) and never WireGuard's gost relay.
func TestScriptLegacyCleanupIsNamed(t *testing.T) {
	script := string(Script)
	assert.Contains(t, script, `readonly LEGACY_NFT_TABLES=("inet v2b_forward" "ip v2b_forward" "ip anixops_forward")`)
	assert.Contains(t, script, `readonly LEGACY_UNITS=("v2forward-agent.service")`)
	assert.NotContains(t, script, "gost-wg-relay")
	assert.NotContains(t, script, "set -x")
}

func TestShellQuote(t *testing.T) {
	for value, want := range map[string]string{
		"forward-41":                   "forward-41",
		"https://ctl.example.com":      "https://ctl.example.com",
		"https://[2001:db8::1]:8443":   "'https://[2001:db8::1]:8443'",
		"anixagt_abc-DEF_123":          "anixagt_abc-DEF_123",
		"it's":                         `'it'\''s'`,
		"":                             "''",
		"a b":                          "'a b'",
		"$(reboot)":                    "'$(reboot)'",
		"https://ctl.example.com:8443": "https://ctl.example.com:8443",
	} {
		assert.Equal(t, want, ShellQuote(value), value)
	}
}

func TestRenderCommand(t *testing.T) {
	command := RenderCommand(CommandInput{
		ControlURL: "https://ctl.example.com", ScriptURL: "https://ctl.example.com/install.sh",
		Node: "forward-41", Token: "anixagt_TOKEN", Mirror: MirrorControl,
	})
	assert.Equal(t, "curl -fsSL https://ctl.example.com/install.sh | sudo bash -s -- --control https://ctl.example.com --node forward-41 --token anixagt_TOKEN", command)

	ipv6 := RenderCommand(CommandInput{
		ControlURL: "https://[2001:db8::1]:8443", ScriptURL: "https://[2001:db8::1]:8443/install.sh",
		Node: "proxy-7", Token: "anixagt_TOKEN", Mirror: MirrorCN,
	})
	assert.Equal(t, "curl -fsSLg 'https://[2001:db8::1]:8443/install.sh' | sudo bash -s -- --control 'https://[2001:db8::1]:8443' --node proxy-7 --token anixagt_TOKEN --mirror cn", ipv6)
}

func TestSettingsCommands(t *testing.T) {
	settings := Settings{
		ControlURL: "https://ctl.example.com:8443", GRPCTarget: "ctl.example.com:50051",
		AgentVersion: "v4.2.0", ControlVersion: "v4.2.0",
	}
	commands := settings.Commands("forward-41", "anixagt_TOKEN")
	require.Len(t, commands, 3)
	assert.Equal(t, MirrorControl, commands[0].Mirror)
	assert.False(t, commands[0].Available, "no artifact_dir: the control mirror falls back")
	assert.Contains(t, commands[0].Command, "curl -fsSL https://ctl.example.com:8443/install.sh | sudo bash -s -- --control https://ctl.example.com:8443")
	assert.NotContains(t, commands[0].Command, "--mirror")
	assert.False(t, commands[1].Available)
	assert.True(t, strings.HasSuffix(commands[1].Command, "--mirror cn"))
	assert.True(t, commands[2].Available)
	assert.Contains(t, commands[2].Command, "https://github.com/AnixOps/anix-control/releases/download/v4.2.0/agent-install.sh")
	assert.True(t, strings.HasSuffix(commands[2].Command, "--mirror github"))
	assert.Contains(t, commands[2].Command, "--control https://ctl.example.com:8443")
}

func writeRelease(t *testing.T, dir, tag string, assets map[string]string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, tag), 0o755))
	for asset, dgst := range assets {
		require.NoError(t, os.WriteFile(filepath.Join(dir, tag, asset), []byte("zip"), 0o644))
		if dgst != "" {
			require.NoError(t, os.WriteFile(filepath.Join(dir, tag, asset+".dgst"), []byte(dgst), 0o644))
		}
	}
}

func TestMetadataAndEnv(t *testing.T) {
	digestA := strings.Repeat("a", 64)
	digestB := strings.Repeat("B", 64)
	dir := t.TempDir()
	settings := Settings{
		ControlURL: "https://ctl.example.com", GRPCTarget: "ctl.example.com:50051", AgentVersion: "v4.2.0",
		ArtifactDir: dir, CNMirrorURL: "https://mirror.example.cn/anix-agent/",
	}

	// One asset without its digest: Control does not claim the release.
	writeRelease(t, dir, "v4.2.0", map[string]string{
		"anix-agent-linux-64.zip":        "MD5= 00\nSHA2-256= " + digestA + "\n",
		"anix-agent-linux-arm64-v8a.zip": "",
	})
	metadata := settings.Metadata()
	assert.NotContains(t, metadata.Sources, MirrorControl)
	assert.Empty(t, metadata.SHA256)
	assert.Equal(t, "https://mirror.example.cn/anix-agent", metadata.Sources[MirrorCN])

	require.NoError(t, os.WriteFile(filepath.Join(dir, "v4.2.0", "anix-agent-linux-arm64-v8a.zip.dgst"),
		[]byte("SHA256(anix-agent-linux-arm64-v8a.zip)= "+digestB+"\n"), 0o644))
	metadata = settings.Metadata()
	assert.Equal(t, "https://ctl.example.com/install/agent", metadata.Sources[MirrorControl])
	assert.Equal(t, map[string]string{"anix-agent-linux-64.zip": digestA, "anix-agent-linux-arm64-v8a.zip": strings.ToLower(digestB)}, metadata.SHA256)

	env, err := metadata.Env()
	require.NoError(t, err)
	assert.Equal(t, "format 1\n"+
		"agent_version v4.2.0\n"+
		"grpc_target ctl.example.com:50051\n"+
		"asset amd64 anix-agent-linux-64.zip\n"+
		"asset arm64 anix-agent-linux-arm64-v8a.zip\n"+
		"source control https://ctl.example.com/install/agent\n"+
		"source cn https://mirror.example.cn/anix-agent\n"+
		"source github https://github.com/AnixOps/anix-agent/releases/download\n"+
		"sha256 anix-agent-linux-64.zip "+digestA+"\n"+
		"sha256 anix-agent-linux-arm64-v8a.zip "+strings.ToLower(digestB)+"\n", env)

	for name, broken := range map[string]Metadata{
		"version": {AgentVersion: "latest", Sources: map[string]string{}},
		"source":  {AgentVersion: "v4.2.0", Sources: map[string]string{MirrorCN: "ftp://x"}},
		"unsafe":  {AgentVersion: "v4.2.0", GRPCTarget: "a b:1", Sources: map[string]string{}},
		"digest":  {AgentVersion: "v4.2.0", Sources: map[string]string{}, SHA256: map[string]string{"anix-agent-linux-64.zip": "zz"}},
	} {
		_, err := broken.Env()
		assert.Error(t, err, name)
	}
}

func TestParseDigest(t *testing.T) {
	digest := strings.Repeat("c", 64)
	for input, ok := range map[string]bool{
		"SHA2-256= " + digest:                 true,
		"SHA256= " + digest:                   true,
		"SHA256(./anix-agent.zip)= " + digest: true,
		"MD5= " + strings.Repeat("c", 32):     false,
		"SHA512= " + strings.Repeat("c", 128): false,
		digest + "  anix-agent-linux-64.zip":  false,
	} {
		got, found := ParseDigest([]byte(input))
		assert.Equal(t, ok, found, input)
		if ok {
			assert.Equal(t, digest, got)
		}
	}
}

func TestArtifactPath(t *testing.T) {
	path, err := ArtifactPath("/srv/agent", "v4.2.0", "anix-agent-linux-64.zip.dgst")
	require.NoError(t, err)
	assert.Equal(t, "/srv/agent/v4.2.0/anix-agent-linux-64.zip.dgst", path)
	for _, bad := range [][3]string{
		{"", "v4.2.0", "anix-agent-linux-64.zip"},
		{"/srv/agent", "..", "anix-agent-linux-64.zip"},
		{"/srv/agent", "v4.2.0", "../../etc/passwd"},
		{"/srv/agent", "v4.2.0", "anix-agent-linux-64.zip/.."},
		{"/srv/agent", "latest", "anix-agent-linux-64.zip"},
		{"/srv/agent", "v4.2.0", "config.json"},
	} {
		_, err := ArtifactPath(bad[0], bad[1], bad[2])
		assert.Error(t, err, bad)
	}
}

func TestValidNode(t *testing.T) {
	for node, ok := range map[string]bool{
		"forward-41": true, "proxy-1": true, "proxy-0": false, "module-1": false, "forward-": false, "forward-41 x": false,
	} {
		assert.Equal(t, ok, ValidNode(node), node)
	}
}

func TestScriptIsEmbeddedVerbatim(t *testing.T) {
	data, err := os.ReadFile("install.sh")
	require.NoError(t, err)
	assert.True(t, bytes.Equal(data, Script))
	assert.True(t, bytes.HasPrefix(Script, []byte("#!/usr/bin/env bash\n")))
}
