package agentinstall

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpgradeArtifacts(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key := base64.StdEncoding.EncodeToString(public)
	dir := t.TempDir()
	writeSignedRelease(t, dir, "v4.2.0", private)

	artifacts, err := UpgradeArtifacts(dir, "v4.2.0", key, "https://panel.example.com/")
	require.NoError(t, err)
	require.Len(t, artifacts, 2)
	assert.Equal(t, "amd64", artifacts[0].Arch)
	assert.Equal(t, "https://panel.example.com/install/agent/v4.2.0/anix-agent-linux-64.zip", artifacts[0].URL)
	data := []byte("fake agent zip for amd64")
	assert.Equal(t, sha256Hex(data), artifacts[0].SHA256)
	assert.Equal(t, int64(len(data)), artifacts[0].Size)
	assert.Equal(t, "arm64", artifacts[1].Arch)
	// What Control sends passes the check the Agent makes.
	request := agentcontrol.UpgradeRequest{
		Schema: agentcontrol.UpgradeSchemaV1, CampaignID: "c", Action: agentcontrol.UpgradeActionUpgrade, TargetVersion: "v4.2.0", Artifacts: artifacts,
	}
	require.NoError(t, request.Validate())
	signature, err := base64.StdEncoding.DecodeString(artifacts[0].Signature)
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(public, data, signature))
}

func TestUpgradeArtifactsRefuses(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key := base64.StdEncoding.EncodeToString(public)
	otherPublic, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	fresh := func(t *testing.T) string {
		dir := t.TempDir()
		writeSignedRelease(t, dir, "v4.2.0", private)
		return dir
	}
	refuse := func(t *testing.T, dir, tag, publicKey, control string) {
		t.Helper()
		_, err := UpgradeArtifacts(dir, tag, publicKey, control)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUpgradeRelease), err)
	}
	t.Run("no directory", func(t *testing.T) { refuse(t, "", "v4.2.0", key, "https://p.example") })
	t.Run("not a tag", func(t *testing.T) { refuse(t, fresh(t), "latest", key, "https://p.example") })
	t.Run("http control", func(t *testing.T) { refuse(t, fresh(t), "v4.2.0", key, "http://p.example") })
	t.Run("missing release", func(t *testing.T) { refuse(t, fresh(t), "v4.3.0", key, "https://p.example") })
	t.Run("foreign key", func(t *testing.T) {
		refuse(t, fresh(t), "v4.2.0", base64.StdEncoding.EncodeToString(otherPublic), "https://p.example")
	})
	t.Run("tampered asset", func(t *testing.T) {
		dir := fresh(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "v4.2.0", Assets["arm64"]), []byte("tampered"), 0o600))
		refuse(t, dir, "v4.2.0", key, "https://p.example")
	})
	t.Run("missing signature", func(t *testing.T) {
		dir := fresh(t)
		require.NoError(t, os.Remove(filepath.Join(dir, "v4.2.0", Assets["amd64"]+".sig")))
		refuse(t, dir, "v4.2.0", key, "https://p.example")
	})
	t.Run("asset re-signed but not in SHA256SUMS", func(t *testing.T) {
		dir := fresh(t)
		data := []byte("another build")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "v4.2.0", Assets["amd64"]), data, 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "v4.2.0", Assets["amd64"]+".sig"), sign(private, data), 0o600))
		refuse(t, dir, "v4.2.0", key, "https://p.example")
	})
}
