package pluginhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/stretchr/testify/require"
)

func TestIdentityPlatformCompiledHostUsesTheCapabilityBridge(t *testing.T) {
	calls := make(chan packagebridge.Call, 1)
	allowlist, err := packagebridge.NewAllowlist(packagebridge.Operation{
		PackageID: "identity-platform", RouteID: "identity.auth.login", Name: "identity.auth.login",
		Handler: func(_ context.Context, call packagebridge.Call) (packagebridge.Response, error) {
			calls <- call
			return packagebridge.Response{StatusCode: 200, Body: []byte(`{"code":0,"msg":"操作成功","ts":1,"data":{"token":"identity-host"}}`)}, nil
		},
	})
	require.NoError(t, err)
	manager, err := NewManager(ManagerConfig{
		RuntimeDir: shortHostTempDir(t), BridgeFactory: packagebridge.NewFactory(allowlist), StartupTimeout: 10 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	ref := buildIdentityPlatformArtifactRef(t)
	require.NoError(t, manager.Start(context.Background(), ref, 7))

	response, err := manager.Dispatch(context.Background(), DispatchInput{
		PackageID: "identity-platform", Version: "4.0.0", Generation: 7,
		RequestID: "identity-compiled-1", RouteID: "identity.auth.login", Method: "POST",
		Body:          []byte(`{"email":"u@example.test","password":"secret"}`),
		PrincipalJSON: []byte(`{"actor_id":0,"admin":false,"package_id":"identity-platform"}`),
		Metadata:      RequestMetadata{Path: "/api/v2/login", ClientIP: "198.51.100.42", UserAgent: "AnixOps-Test/1.0"},
		Deadline:      time.Now().Add(5 * time.Second),
	})
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.JSONEq(t, `{"code":0,"msg":"操作成功","ts":1,"data":{"token":"identity-host"}}`, string(response.Body))
	select {
	case call := <-calls:
		require.Equal(t, []byte(`{"email":"u@example.test","password":"secret"}`), call.Request.Body)
		var metadata RequestMetadata
		require.NoError(t, json.Unmarshal(call.Request.MetadataJSON, &metadata))
		require.Equal(t, "198.51.100.42", metadata.ClientIP)
		require.Equal(t, "AnixOps-Test/1.0", metadata.UserAgent)
	case <-time.After(time.Second):
		t.Fatal("identity host did not invoke the package bridge")
	}
}

func buildIdentityPlatformArtifactRef(t *testing.T) ArtifactRef {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	directory := t.TempDir()
	entrypointPath := filepath.Join(directory, "identity-control-host")
	command := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-buildid= -X main.packageVersion=4.0.0", "-o", entrypointPath, "./packages/identity-platform/control")
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	entrypoint, err := os.ReadFile(entrypointPath)
	require.NoError(t, err)
	artifact := []byte("identity-platform compiled test artifact")
	artifactPath := filepath.Join(directory, "package.anxp")
	require.NoError(t, os.WriteFile(artifactPath, artifact, 0o600))
	manifest, err := json.Marshal(struct {
		ID      string   `json:"id"`
		Version string   `json:"version"`
		Targets []string `json:"targets"`
	}{ID: "identity-platform", Version: "4.0.0", Targets: []string{"control"}})
	require.NoError(t, err)
	manifestPath := filepath.Join(directory, "manifest.json")
	require.NoError(t, os.WriteFile(manifestPath, manifest, 0o600))
	return ArtifactRef{
		PackageID: "identity-platform", Version: "4.0.0",
		ArtifactPath: artifactPath, ArtifactSHA256: digestBytes(artifact),
		EntrypointPath: entrypointPath, EntrypointSHA256: digestBytes(entrypoint),
		ManifestPath: manifestPath, ManifestSHA256: digestBytes(manifest),
	}
}

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
