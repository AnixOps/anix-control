package agentinstall

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSignedRelease puts a signed fake Agent release in dir/<tag>/ the way
// anix-agent's release job publishes it.
func writeSignedRelease(t *testing.T, dir, tag string, private ed25519.PrivateKey) {
	t.Helper()
	release := filepath.Join(dir, tag)
	require.NoError(t, os.MkdirAll(release, 0o750))
	var sums bytes.Buffer
	for _, arch := range []string{"amd64", "arm64"} {
		asset := Assets[arch]
		data := []byte("fake agent zip for " + arch)
		require.NoError(t, os.WriteFile(filepath.Join(release, asset), data, 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(release, asset+".sig"), sign(private, data), 0o600))
		sums.WriteString(sha256Hex(data) + "  " + asset + "\n")
	}
	require.NoError(t, os.WriteFile(filepath.Join(release, SumsAssetName), sums.Bytes(), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(release, SumsSignatureAssetName), sign(private, sums.Bytes()), 0o600))
}

func readBundle(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	archive := tar.NewReader(gz)
	files := map[string][]byte{}
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		assert.Equal(t, byte(tar.TypeReg), header.Typeflag, header.Name)
		body, err := io.ReadAll(archive)
		require.NoError(t, err)
		files[header.Name] = body
	}
	return files
}

func bundleSettings(dir string) Settings {
	return Settings{
		ControlURL: "https://panel.example.com", GRPCTarget: "panel.example.com:50051", AgentVersion: "v4.2.0",
		ControlVersion: "v4.2.0", ArtifactDir: dir,
	}
}

func TestWriteOfflineBundle(t *testing.T) {
	private, public := testKey(t)
	dir := t.TempDir()
	writeSignedRelease(t, dir, "v4.2.0", private)
	scriptSignature := sign(private, Script)

	var out bytes.Buffer
	bundle, err := WriteOfflineBundle(&out, bundleSettings(dir), "arm64", public, scriptSignature)
	require.NoError(t, err)
	assert.Equal(t, "anix-agent-linux-arm64-v8a.zip", bundle.Asset)
	assert.Equal(t, sha256Hex([]byte("fake agent zip for arm64")), bundle.SHA256)
	assert.Equal(t, "panel.example.com:50051", bundle.GRPCTarget)

	files := readBundle(t, out.Bytes())
	assert.Len(t, files, len(bundle.Files))
	for _, name := range []string{"agent.env", "anix-agent-linux-arm64-v8a.zip", "anix-agent-linux-arm64-v8a.zip.sig",
		"SHA256SUMS", "SHA256SUMS.sig", "install.sh", "install.sh.sig"} {
		assert.Contains(t, files, name)
	}
	assert.NotContains(t, files, "anix-agent-linux-64.zip", "a bundle carries one architecture")
	assert.Equal(t, Script, files["install.sh"])
	assert.Equal(t, scriptSignature, files["install.sh.sig"])
	assert.Contains(t, string(files["agent.env"]), "grpc_target panel.example.com:50051\n")
	assert.Contains(t, string(files["agent.env"]), "agent_version v4.2.0\n")

	// Every entry is one the installer's --offline accepts.
	if bash, err := exec.LookPath("bash"); err == nil {
		for name := range files {
			command := exec.Command(bash, "-c", `source "$1"; offline_entry "$2"`, "_", "install.sh", name) // #nosec G204 -- test input.
			assert.NoError(t, command.Run(), "the installer refuses bundle entry %s", name)
		}
	}

	// Without the script's signature the bundle still installs.
	out.Reset()
	bundle, err = WriteOfflineBundle(&out, bundleSettings(dir), "amd64", public, nil)
	require.NoError(t, err)
	assert.NotContains(t, readBundle(t, out.Bytes()), "install.sh.sig")
	assert.NotContains(t, bundle.Files, "install.sh.sig")
}

func TestWriteOfflineBundleRefuses(t *testing.T) {
	private, public := testKey(t)
	_, other := testKey(t)
	release := func(t *testing.T) string {
		dir := t.TempDir()
		writeSignedRelease(t, dir, "v4.2.0", private)
		return dir
	}
	for name, tc := range map[string]struct {
		arch   string
		key    string
		mutate func(dir string)
		want   string
	}{
		"unknown arch":      {arch: "riscv64", want: "unknown architecture"},
		"other key":         {arch: "amd64", key: other, want: "SHA256SUMS"},
		"no artifact dir":   {arch: "amd64", mutate: func(string) {}, want: "artifact_dir"},
		"missing sums sig":  {arch: "amd64", mutate: func(dir string) { _ = os.Remove(filepath.Join(dir, "v4.2.0", SumsSignatureAssetName)) }, want: "SHA256SUMS.sig"},
		"missing asset sig": {arch: "amd64", mutate: func(dir string) { _ = os.Remove(filepath.Join(dir, "v4.2.0", "anix-agent-linux-64.zip.sig")) }, want: "anix-agent-linux-64.zip.sig"},
		"altered asset": {arch: "amd64", mutate: func(dir string) {
			_ = os.WriteFile(filepath.Join(dir, "v4.2.0", "anix-agent-linux-64.zip"), []byte("altered"), 0o600)
		}, want: "anix-agent-linux-64.zip"},
		"unlisted asset": {arch: "amd64", mutate: func(dir string) {
			sums := []byte(sha256Hex([]byte("x")) + "  other.zip\n")
			_ = os.WriteFile(filepath.Join(dir, "v4.2.0", SumsAssetName), sums, 0o600)
			_ = os.WriteFile(filepath.Join(dir, "v4.2.0", SumsSignatureAssetName), sign(private, sums), 0o600)
		}, want: "does not list"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := release(t)
			settings := bundleSettings(dir)
			if name == "no artifact dir" {
				settings.ArtifactDir = ""
			}
			if tc.mutate != nil {
				tc.mutate(dir)
			}
			key := public
			if tc.key != "" {
				key = tc.key
			}
			var out bytes.Buffer
			_, err := WriteOfflineBundle(&out, settings, tc.arch, key, nil)
			require.ErrorIs(t, err, ErrOfflineBundle)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestReleaseFilePath(t *testing.T) {
	path, err := ReleaseFilePath("/srv/agent", "v4.2.0", SumsAssetName)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/srv/agent", "v4.2.0", "SHA256SUMS"), path)
	_, err = ReleaseFilePath("/srv/agent", "v4.2.0", "anix-agent-linux-64.zip.sig")
	require.NoError(t, err)
	for _, bad := range [][2]string{{"../v4.2.0", SumsAssetName}, {"v4.2.0", "../SHA256SUMS"}, {"v4.2.0", "config.yaml"}} {
		_, err := ReleaseFilePath("/srv/agent", bad[0], bad[1])
		assert.Error(t, err, bad)
	}
	// The public artifact route still refuses the checksum files.
	_, err = ArtifactPath("/srv/agent", "v4.2.0", SumsAssetName)
	assert.Error(t, err)
}

func TestResolveSettings(t *testing.T) {
	settings, err := ResolveSettings(SettingsInput{ControlURL: "https://panel.example.com/", GRPCPort: 50052, ReleaseVersion: "4.2.0"})
	require.NoError(t, err)
	assert.Equal(t, "https://panel.example.com", settings.ControlURL)
	assert.Equal(t, "panel.example.com:50052", settings.GRPCTarget)
	assert.Equal(t, "v4.2.0", settings.AgentVersion)

	settings, err = ResolveSettings(SettingsInput{ControlURL: "https://panel.example.com", GRPCTarget: "grpc.example.com:443", AgentVersion: "v4.2.1", ReleaseVersion: "dev"})
	require.NoError(t, err)
	assert.Equal(t, "grpc.example.com:443", settings.GRPCTarget)
	assert.Equal(t, "v4.2.1", settings.AgentVersion)

	for name, in := range map[string]SettingsInput{
		"no address":  {ReleaseVersion: "4.2.0"},
		"http":        {ControlURL: "http://panel.example.com", ReleaseVersion: "4.2.0"},
		"dev version": {ControlURL: "https://panel.example.com", ReleaseVersion: "dev"},
	} {
		_, err := ResolveSettings(in)
		assert.Error(t, err, name)
	}
}
