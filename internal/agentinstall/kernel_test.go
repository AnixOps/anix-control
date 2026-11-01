package agentinstall

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testKernelSignature = "-----BEGIN SSH SIGNATURE-----\nU1NIU0lH\n-----END SSH SIGNATURE-----\n"

func writeKernelRelease(t *testing.T, dir, version string) []byte {
	t.Helper()
	release := filepath.Join(dir, "anx-node", version)
	require.NoError(t, os.MkdirAll(release, 0o755))
	asset := KernelAsset(version, "x86_64")
	data := []byte("fake anx-node package")
	require.NoError(t, os.WriteFile(filepath.Join(release, asset), data, 0o644))
	sums := sha256Hex(data) + "  " + asset + "\n" + sha256Hex([]byte("bin")) + "  anx-node-" + version + "-x86_64-linux-musl/anx-node\n"
	require.NoError(t, os.WriteFile(filepath.Join(release, SumsAssetName), []byte(sums), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(release, SumsSignatureAssetName), []byte(testKernelSignature), 0o644))
	return data
}

func TestUpgradeKernels(t *testing.T) {
	dir := t.TempDir()
	data := writeKernelRelease(t, dir, "0.1.0-alpha.2")

	kernels, err := UpgradeKernels(dir, "https://panel.example.com/")
	require.NoError(t, err)
	require.Len(t, kernels, 1)
	kernel := kernels[0]
	assert.Equal(t, "amd64", kernel.Arch)
	assert.Equal(t, "anx-node-0.1.0-alpha.2-x86_64-linux-musl.tar.gz", kernel.Asset)
	assert.Equal(t, "https://panel.example.com/install/kernel/anx-node/0.1.0-alpha.2/anx-node-0.1.0-alpha.2-x86_64-linux-musl.tar.gz", kernel.URL)
	assert.Equal(t, sha256Hex(data), kernel.SHA256)
	assert.Equal(t, int64(len(data)), kernel.Size)
	assert.Equal(t, testKernelSignature, kernel.SumsSignature)
	// What Control sends passes the check the Agent makes.
	request := validRequestWith(t, dir)
	request.Kernels = kernels
	require.NoError(t, request.Validate())

	none, err := UpgradeKernels(t.TempDir(), "https://panel.example.com")
	require.NoError(t, err)
	assert.Empty(t, none)
	// The newest release wins.
	writeKernelRelease(t, dir, "0.1.0-alpha.10")
	writeKernelRelease(t, dir, "0.1.0-alpha.9")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "anx-node", "notes"), 0o755))
	assert.Equal(t, "0.1.0-alpha.10", LatestKernelVersion(dir))
	writeKernelRelease(t, dir, "0.1.0")
	assert.Equal(t, "0.1.0", LatestKernelVersion(dir))
}

func validRequestWith(t *testing.T, dir string) agentcontrol.UpgradeRequest {
	t.Helper()
	return agentcontrol.UpgradeRequest{
		Schema: agentcontrol.UpgradeSchemaV1, CampaignID: "c", Action: agentcontrol.UpgradeActionUpgrade, TargetVersion: "v4.2.0",
		Artifacts: []agentcontrol.UpgradeArtifact{{
			Arch: "amd64", Asset: "anix-agent-linux-64.zip", URL: "https://panel.example.com/install/agent/v4.2.0/anix-agent-linux-64.zip",
			SHA256: sha256Hex([]byte(dir)), Size: 1, Signature: base64.StdEncoding.EncodeToString(make([]byte, 64)),
		}},
	}
}

func TestKernelReleaseHeld(t *testing.T) {
	dir := t.TempDir()
	assert.Empty(t, KernelReleaseHeld(dir))
	assert.Empty(t, KernelReleaseHeld(""))
	writeKernelRelease(t, dir, "0.1.0-alpha.9")
	assert.Equal(t, "0.1.0-alpha.9", KernelReleaseHeld(dir))
	// Without its signature the release cannot be verified by an installer.
	require.NoError(t, os.Remove(filepath.Join(dir, "anx-node", "0.1.0-alpha.9", SumsSignatureAssetName)))
	assert.Empty(t, KernelReleaseHeld(dir))
}

func TestUpgradeKernelsRefuses(t *testing.T) {
	refuse := func(t *testing.T, dir, version, control string) {
		t.Helper()
		require.Equal(t, version, LatestKernelVersion(dir))
		_, err := UpgradeKernels(dir, control)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrKernelRelease), err)
	}
	t.Run("http control", func(t *testing.T) {
		dir := t.TempDir()
		writeKernelRelease(t, dir, "0.1.0")
		refuse(t, dir, "0.1.0", "http://panel.example.com")
	})
	t.Run("empty release", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "anx-node", "0.1.0"), 0o755))
		refuse(t, dir, "0.1.0", "https://panel.example.com")
	})
	t.Run("missing signature", func(t *testing.T) {
		dir := t.TempDir()
		writeKernelRelease(t, dir, "0.1.0")
		require.NoError(t, os.Remove(filepath.Join(dir, "anx-node", "0.1.0", SumsSignatureAssetName)))
		refuse(t, dir, "0.1.0", "https://panel.example.com")
	})
	t.Run("tampered package", func(t *testing.T) {
		dir := t.TempDir()
		writeKernelRelease(t, dir, "0.1.0")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "anx-node", "0.1.0", KernelAsset("0.1.0", "x86_64")), []byte("other"), 0o644))
		refuse(t, dir, "0.1.0", "https://panel.example.com")
	})
	t.Run("no package listed", func(t *testing.T) {
		dir := t.TempDir()
		writeKernelRelease(t, dir, "0.1.0")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "anx-node", "0.1.0", SumsAssetName), []byte(sha256Hex(nil)+"  other.tar.gz\n"), 0o644))
		refuse(t, dir, "0.1.0", "https://panel.example.com")
	})
}

func TestKernelPath(t *testing.T) {
	path, err := KernelPath("/srv/artifacts", "0.1.0-alpha.2", "anx-node-0.1.0-alpha.2-x86_64-linux-musl.tar.gz")
	require.NoError(t, err)
	assert.Equal(t, "/srv/artifacts/anx-node/0.1.0-alpha.2/anx-node-0.1.0-alpha.2-x86_64-linux-musl.tar.gz", path)
	_, err = KernelPath("/srv/artifacts", "0.1.0-alpha.2", SumsSignatureAssetName)
	require.NoError(t, err)
	for _, bad := range [][2]string{
		{"0.1.0-alpha.2", "../SHA256SUMS"},
		{"0.1.0-alpha.2", "anx-node-0.1.0-alpha.3-x86_64-linux-musl.tar.gz"},
		{"../x", SumsAssetName},
		{"0.1.0-alpha.2", "anx-node"},
	} {
		_, err := KernelPath("/srv/artifacts", bad[0], bad[1])
		assert.Error(t, err, bad)
	}
	_, err = KernelPath("", "0.1.0", SumsAssetName)
	assert.Error(t, err)
}

func TestUpgradeKernelsOffersArm64WhenReleaseHasIt(t *testing.T) {
	dir := t.TempDir()
	version := "0.1.0-alpha.5"
	release := filepath.Join(dir, "anx-node", version)
	require.NoError(t, os.MkdirAll(release, 0o755))
	x86 := []byte("x86 package")
	arm := []byte("arm package")
	x86Asset := KernelAsset(version, "x86_64")
	armAsset := KernelAsset(version, "aarch64")
	require.NoError(t, os.WriteFile(filepath.Join(release, x86Asset), x86, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(release, armAsset), arm, 0o644))
	sums := sha256Hex(arm) + "  " + armAsset + "\n" + sha256Hex(x86) + "  " + x86Asset + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(release, SumsAssetName), []byte(sums), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(release, SumsSignatureAssetName), []byte(testKernelSignature), 0o644))

	kernels, err := UpgradeKernels(dir, "https://panel.example.com")
	require.NoError(t, err)
	require.Len(t, kernels, 2)
	assert.Equal(t, "amd64", kernels[0].Arch)
	assert.Equal(t, "arm64", kernels[1].Arch)
	assert.Equal(t, armAsset, kernels[1].Asset)
	assert.Equal(t, sha256Hex(arm), kernels[1].SHA256)
	request := validRequestWith(t, dir)
	request.Kernels = kernels
	require.NoError(t, request.Validate())
}
