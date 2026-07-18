package pluginhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestManagerRejectsDispatchWhenLeaseDoesNotMatchGeneration(t *testing.T) {
	manager := newTestManager(t, hostWithGeneration("knowledge", "4.0.0", 7))

	_, err := manager.Dispatch(context.Background(), DispatchInput{
		PackageID:     "knowledge",
		Version:       "4.0.0",
		Generation:    6,
		RequestID:     "request-1",
		RouteID:       "knowledge.article.list",
		Method:        "GET",
		PrincipalJSON: []byte(`{"actor_id":7}`),
		Deadline:      time.Now().Add(time.Second),
	})

	require.ErrorIs(t, err, ErrGenerationUnavailable)
}

func TestDefaultManagerReturnsConfiguredSupervisor(t *testing.T) {
	previous := DefaultManager()
	manager := newTestManager(t)
	SetDefaultManager(manager)
	t.Cleanup(func() { SetDefaultManager(previous) })

	require.Same(t, manager, DefaultManager())
}

func TestManagerRejectsStartWhenEntrypointDigestDoesNotMatch(t *testing.T) {
	manager := newTestManager(t)
	ref := writeVerifiedArtifactRef(t, "knowledge", "4.0.0")
	ref.EntrypointSHA256 = strings.Repeat("0", sha256.Size*2)

	err := manager.Start(context.Background(), ref, 7)

	require.ErrorIs(t, err, ErrHostIncompatible)
	require.Empty(t, manager.hosts)
}

func TestManagerStartsVerifiedHostOverPrivateUnixSocket(t *testing.T) {
	t.Setenv(hostTestChildEnvironment, "1")
	manager, err := NewManager(ManagerConfig{
		RuntimeDir:     filepath.Join(shortHostTempDir(t), "runtime"),
		StartupTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	ref := writeHostArtifactRef(t, "knowledge", "4.0.0")

	require.NoError(t, manager.Start(context.Background(), ref, 7))
	host := manager.hosts[hostKey(ref.PackageID, ref.Version)]
	require.NotNil(t, host)
	runtimeInfo, err := os.Stat(manager.runtimeDir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), runtimeInfo.Mode().Perm())
	socketInfo, err := os.Lstat(host.socketPath)
	require.NoError(t, err)
	require.NotZero(t, socketInfo.Mode()&os.ModeSocket)
	require.Equal(t, os.FileMode(0o600), socketInfo.Mode().Perm())

	response, err := manager.Dispatch(context.Background(), DispatchInput{
		PackageID:     ref.PackageID,
		Version:       ref.Version,
		Generation:    7,
		RequestID:     "request-1",
		RouteID:       "knowledge.article.list",
		Method:        "GET",
		PrincipalJSON: []byte(`{"actor_id":7}`),
		Deadline:      time.Now().Add(time.Second),
	})
	require.NoError(t, err)
	require.EqualValues(t, 202, response.StatusCode)
	require.Equal(t, []byte(`{"transport":"unix"}`), response.Body)

	require.NoError(t, manager.Stop(context.Background(), ref.PackageID, ref.Version, 7))
	require.NoFileExists(t, host.socketPath)
}

func TestManagerRestartsExitedHostAtSameGeneration(t *testing.T) {
	t.Setenv(hostTestChildEnvironment, "1")
	manager, err := NewManager(ManagerConfig{RuntimeDir: filepath.Join(shortHostTempDir(t), "runtime")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	ref := writeHostArtifactRef(t, "knowledge", "4.0.0")

	require.NoError(t, manager.Start(context.Background(), ref, 7))
	first := manager.hosts[hostKey(ref.PackageID, ref.Version)]
	require.NotNil(t, first)
	require.NoError(t, first.command.Process.Kill())
	require.Eventually(t, func() bool {
		select {
		case <-first.waitDone:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, manager.Start(context.Background(), ref, 7))
	second := manager.hosts[hostKey(ref.PackageID, ref.Version)]
	require.NotSame(t, first, second)

	response, err := manager.Dispatch(context.Background(), DispatchInput{
		PackageID: ref.PackageID, Version: ref.Version, Generation: 7,
		RequestID: "request-2", RouteID: "knowledge.article.list", Method: "GET",
		PrincipalJSON: []byte(`{"actor_id":7}`), Deadline: time.Now().Add(time.Second),
	})
	require.NoError(t, err)
	require.EqualValues(t, 202, response.StatusCode)
	require.NoError(t, manager.Shutdown(context.Background()))
}

func TestManagerShutdownStopsSupervisedHosts(t *testing.T) {
	t.Setenv(hostTestChildEnvironment, "1")
	manager, err := NewManager(ManagerConfig{RuntimeDir: shortHostTempDir(t)})
	require.NoError(t, err)
	ref := writeHostArtifactRef(t, "knowledge", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	host := manager.hosts[hostKey(ref.PackageID, ref.Version)]
	require.NotNil(t, host)

	require.NoError(t, manager.Shutdown(context.Background()))
	require.Empty(t, manager.hosts)
	require.NoFileExists(t, host.socketPath)
}

func newTestManager(t *testing.T, hosts ...*hostProcess) *Supervisor {
	t.Helper()
	manager, err := NewManager(ManagerConfig{RuntimeDir: t.TempDir()})
	require.NoError(t, err)
	for _, host := range hosts {
		manager.hosts[hostKey(host.packageID, host.version)] = host
	}
	return manager
}

func hostWithGeneration(packageID, version string, generation uint64) *hostProcess {
	return &hostProcess{packageID: packageID, version: version, generation: generation}
}

func writeVerifiedArtifactRef(t *testing.T, packageID, version string) ArtifactRef {
	t.Helper()
	directory := t.TempDir()
	artifactPath := filepath.Join(directory, "package.anxp")
	entrypointPath := filepath.Join(directory, "control-host")
	manifestPath := filepath.Join(directory, "manifest.json")
	artifact := []byte("verified package artifact")
	entrypoint := []byte("verified control host")
	manifest, err := json.Marshal(struct {
		ID      string   `json:"id"`
		Version string   `json:"version"`
		Targets []string `json:"targets"`
	}{ID: packageID, Version: version, Targets: []string{"control"}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(artifactPath, artifact, 0o600))
	require.NoError(t, os.WriteFile(entrypointPath, entrypoint, 0o700))
	require.NoError(t, os.WriteFile(manifestPath, manifest, 0o600))

	return ArtifactRef{
		PackageID:        packageID,
		Version:          version,
		ArtifactPath:     artifactPath,
		ArtifactSHA256:   testDigest(artifact),
		EntrypointPath:   entrypointPath,
		EntrypointSHA256: testDigest(entrypoint),
		ManifestPath:     manifestPath,
		ManifestSHA256:   testDigest(manifest),
	}
}

func writeHostArtifactRef(t *testing.T, packageID, version string) ArtifactRef {
	t.Helper()
	ref := writeVerifiedArtifactRef(t, packageID, version)
	entrypoint := []byte("#!/bin/sh\nexec " + strconv.Quote(os.Args[0]) + " -test.run=^TestPluginHostProcess$ --\n")
	require.NoError(t, os.WriteFile(ref.EntrypointPath, entrypoint, 0o700))
	ref.EntrypointSHA256 = testDigest(entrypoint)
	return ref
}

func shortHostTempDir(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("", "anix-host-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(directory)) })
	return directory
}

func testDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
