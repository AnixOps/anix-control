package pluginhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestManagerRejectsStartWhenArtifactDigestDoesNotMatch(t *testing.T) {
	manager := newHostTestManager(t, ManagerConfig{})
	ref := writeVerifiedArtifactRef(t, "knowledge", "4.0.0")
	ref.ArtifactSHA256 = strings.Repeat("0", sha256.Size*2)

	err := manager.Start(context.Background(), ref, 7)

	require.ErrorIs(t, err, ErrHostIncompatible)
	require.Empty(t, manager.hosts)
}

func TestManagerStartsVerifiedHostAndCallsItsPackage(t *testing.T) {
	manager := newHostTestManager(t, ManagerConfig{StartupTimeout: 5 * time.Second})
	ref := writeVerifiedArtifactRef(t, "knowledge", "4.0.0")

	require.NoError(t, manager.Start(context.Background(), ref, 7))
	host := manager.hosts[hostKey(ref.PackageID, ref.Version)]
	require.NotNil(t, host)

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
	require.Equal(t, []byte(`{"transport":"direct"}`), response.Body)

	require.NoError(t, manager.Stop(context.Background(), ref.PackageID, ref.Version, 8))
	require.True(t, hostExited(host))
}

func TestManagerRestartsExitedHostAtSameGeneration(t *testing.T) {
	manager := newHostTestManager(t, ManagerConfig{})
	// Keep the watchdog out of the way: this covers the explicit Start path.
	manager.restartBaseDelay = time.Hour
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	ref := writeVerifiedArtifactRef(t, "knowledge", "4.0.0")

	require.NoError(t, manager.Start(context.Background(), ref, 7))
	first := manager.hosts[hostKey(ref.PackageID, ref.Version)]
	require.NotNil(t, first)
	crashTestHost(t, first)
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

func TestManagerAcceptsEquivalentRematerializedArtifactAtSameGeneration(t *testing.T) {
	manager := newHostTestManager(t, ManagerConfig{})
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	firstRef := writeVerifiedArtifactRef(t, "knowledge", "4.0.0")
	secondRef := writeVerifiedArtifactRef(t, "knowledge", "4.0.0")
	require.NotEqual(t, firstRef.ArtifactPath, secondRef.ArtifactPath)
	require.Equal(t, firstRef.ArtifactSHA256, secondRef.ArtifactSHA256)
	require.Equal(t, firstRef.ManifestSHA256, secondRef.ManifestSHA256)

	require.NoError(t, manager.Start(context.Background(), firstRef, 7))
	active := manager.hosts[hostKey(firstRef.PackageID, firstRef.Version)]
	require.NoError(t, manager.Start(context.Background(), secondRef, 7))
	require.Same(t, active, manager.hosts[hostKey(firstRef.PackageID, firstRef.Version)])

	differentRef := writeVerifiedArtifactRef(t, "knowledge", "4.0.0")
	different := []byte("another package artifact")
	require.NoError(t, os.WriteFile(differentRef.ArtifactPath, different, 0o600))
	differentRef.ArtifactSHA256 = testDigest(different)
	require.ErrorIs(t, manager.Start(context.Background(), differentRef, 7), ErrHostIncompatible)
}

func TestManagerReplacesPriorVersionForNewerGeneration(t *testing.T) {
	manager := newHostTestManager(t, ManagerConfig{})
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	firstRef := writeVerifiedArtifactRef(t, "knowledge", "4.0.0")
	secondRef := writeVerifiedArtifactRef(t, "knowledge", "4.1.0")

	require.NoError(t, manager.Start(context.Background(), firstRef, 7))
	first := manager.hosts[hostKey(firstRef.PackageID, firstRef.Version)]
	require.NotNil(t, first)

	require.NoError(t, manager.Start(context.Background(), secondRef, 8))
	second := manager.hosts[hostKey(secondRef.PackageID, secondRef.Version)]
	require.NotNil(t, second)
	require.NotSame(t, first, second)
	require.Eventually(t, func() bool { return hostExited(first) }, time.Second, 10*time.Millisecond)

	_, err := manager.Dispatch(context.Background(), DispatchInput{
		PackageID: firstRef.PackageID, Version: firstRef.Version, Generation: 7,
		RequestID: "old-request", RouteID: "knowledge.article.list", Method: "GET",
		PrincipalJSON: []byte(`{"actor_id":7}`), Deadline: time.Now().Add(time.Second),
	})
	require.ErrorIs(t, err, ErrGenerationUnavailable)

	response, err := manager.Dispatch(context.Background(), DispatchInput{
		PackageID: secondRef.PackageID, Version: secondRef.Version, Generation: 8,
		RequestID: "new-request", RouteID: "knowledge.article.list", Method: "GET",
		PrincipalJSON: []byte(`{"actor_id":7}`), Deadline: time.Now().Add(time.Second),
	})
	require.NoError(t, err)
	require.EqualValues(t, 202, response.StatusCode)
}

func TestManagerDrainsAndStopsOlderHostForNewerLifecycleGeneration(t *testing.T) {
	manager := newHostTestManager(t, ManagerConfig{})
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	ref := writeVerifiedArtifactRef(t, "knowledge", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	host := manager.hosts[hostKey(ref.PackageID, ref.Version)]
	require.NotNil(t, host)

	require.NoError(t, manager.Drain(context.Background(), ref.PackageID, ref.Version, 8, time.Now().Add(time.Second)))
	require.NoError(t, manager.Stop(context.Background(), ref.PackageID, ref.Version, 8))
	require.Eventually(t, func() bool { return hostExited(host) }, time.Second, 10*time.Millisecond)
	require.Empty(t, manager.hosts)
}

func TestManagerRejectsStaleLifecycleStopAgainstNewerHost(t *testing.T) {
	manager := newHostTestManager(t, ManagerConfig{})
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	ref := writeVerifiedArtifactRef(t, "knowledge", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 8))

	err := manager.Stop(context.Background(), ref.PackageID, ref.Version, 7)
	require.ErrorIs(t, err, ErrGenerationUnavailable)
	err = manager.Stop(context.Background(), ref.PackageID, ref.Version, 8)
	require.ErrorIs(t, err, ErrGenerationUnavailable)
}

func TestOpenVerifiedRegularFileRejectsReplacementBeforeOpen(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "package.anxp")
	replacementPath := filepath.Join(t.TempDir(), "replacement")
	require.NoError(t, os.WriteFile(sourcePath, []byte("original"), 0o600))
	require.NoError(t, os.WriteFile(replacementPath, []byte("replacement"), 0o600))

	file, err := openVerifiedRegularFileWithOpen(sourcePath, func(path string) (*os.File, error) {
		require.NoError(t, os.Rename(replacementPath, path))
		return os.Open(path)
	})

	require.Nil(t, file)
	require.ErrorIs(t, err, ErrHostIncompatible)
}

func TestManagerShutdownStopsSupervisedHosts(t *testing.T) {
	manager := newHostTestManager(t, ManagerConfig{})
	ref := writeVerifiedArtifactRef(t, "knowledge", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	host := manager.hosts[hostKey(ref.PackageID, ref.Version)]
	require.NotNil(t, host)

	require.NoError(t, manager.Shutdown(context.Background()))
	require.Empty(t, manager.hosts)
	require.True(t, hostExited(host))
}

func newTestManager(t *testing.T, hosts ...*hostProcess) *Supervisor {
	t.Helper()
	manager, err := NewManager(ManagerConfig{})
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
	manifestPath := filepath.Join(directory, "manifest.json")
	artifact := []byte("verified package artifact")
	manifest, err := json.Marshal(struct {
		ID      string   `json:"id"`
		Version string   `json:"version"`
		Targets []string `json:"targets"`
	}{ID: packageID, Version: version, Targets: []string{"control"}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(artifactPath, artifact, 0o600))
	require.NoError(t, os.WriteFile(manifestPath, manifest, 0o600))

	return ArtifactRef{
		PackageID:      packageID,
		Version:        version,
		ArtifactPath:   artifactPath,
		ArtifactSHA256: testDigest(artifact),
		ManifestPath:   manifestPath,
		ManifestSHA256: testDigest(manifest),
	}
}

func testDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
