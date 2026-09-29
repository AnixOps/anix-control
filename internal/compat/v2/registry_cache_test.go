package v2

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const knowledgeListPath = "/api/v2/user/knowledge"

func newKnowledgeFixture(t *testing.T) *signedRouteFixture {
	t.Helper()
	fixture := newSignedRouteFixture(t, false)
	fixture.addRelease(t, "knowledge", "4.0.0", knowledgeRoutes(knowledgeListPath, "knowledge.article.list"), 0)
	fixture.install(t, "knowledge", "4.0.0", 7)
	return fixture
}

func serveThroughGateway(t *testing.T, source RouteSource, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gateway := Gateway{
		Registry:   NewRegistry(source),
		Dispatcher: &gatewayDispatcherStub{output: pluginhost.DispatchOutput{StatusCode: http.StatusOK, Body: []byte(`{}`)}},
		Metrics:    NewGatewayMetrics(),
	}
	router := gin.New()
	router.Any("/api/v2/*path", gateway.Serve)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestVerifiedRouteCacheHitAvoidsArtifactReads(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	cache := NewVerifiedRouteCache(0)
	source := NewVerifiedRouteSourceWithCache(fixture.db, fixture.encodedKey, cache)

	route, err := source.ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
	require.NoError(t, err)
	require.Equal(t, "knowledge.article.list", route.PackageRoute)
	require.Equal(t, uint64(7), route.Generation)
	coldReads := fixture.artifactReads.Load()
	require.Positive(t, coldReads, "a cold resolution verifies the stored artifact")
	require.Equal(t, 1, cache.Len())

	for range 5 {
		route, err = source.ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
		require.NoError(t, err)
		require.Equal(t, "knowledge.article.list", route.PackageRoute)
	}
	require.Equal(t, coldReads, fixture.artifactReads.Load(), "cache hits must not read artifact blobs")

	uncached := NewVerifiedRouteSourceWithCache(fixture.db, fixture.encodedKey, nil)
	_, err = uncached.ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
	require.NoError(t, err)
	require.Equal(t, 2*coldReads, fixture.artifactReads.Load(), "a nil cache verifies on every resolution")
}

func TestVerifiedRouteCacheFailsClosedWhenTrustRootIsRetired(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	source := NewVerifiedRouteSourceWithCache(fixture.db, fixture.encodedKey, NewVerifiedRouteCache(0))
	require.Equal(t, http.StatusOK, serveThroughGateway(t, source, knowledgeListPath).Code)

	// Rotating the configured trust root retires every other root. The
	// cached declaration must not outlive the root that signed it.
	rotatedKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, err = service.EnsurePluginTrustRoot(fixture.db, rotatedKey)
	require.NoError(t, err)
	response := serveThroughGateway(t, source, knowledgeListPath)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), `"code":"package_unavailable"`)

	// Reinstating the original root restores service without re-verifying.
	reads := fixture.artifactReads.Load()
	_, err = service.EnsurePluginTrustRoot(fixture.db, fixture.publicKey)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, serveThroughGateway(t, source, knowledgeListPath).Code)
	require.Equal(t, reads, fixture.artifactReads.Load())
}

func TestVerifiedRouteCacheRechecksInstallationAndPluginEveryRequest(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	source := NewVerifiedRouteSourceWithCache(fixture.db, fixture.encodedKey, NewVerifiedRouteCache(0))
	require.Equal(t, http.StatusOK, serveThroughGateway(t, source, knowledgeListPath).Code)

	fixture.updateInstallation(t, "knowledge", map[string]any{"enabled": false})
	require.Equal(t, http.StatusServiceUnavailable, serveThroughGateway(t, source, knowledgeListPath).Code)
	fixture.updateInstallation(t, "knowledge", map[string]any{"enabled": true})
	require.Equal(t, http.StatusOK, serveThroughGateway(t, source, knowledgeListPath).Code)

	require.NoError(t, fixture.db.Model(&model.Plugin{}).Where("id = ?", "knowledge").Update("official", false).Error)
	require.Equal(t, http.StatusServiceUnavailable, serveThroughGateway(t, source, knowledgeListPath).Code)
}

func TestVerifiedRouteCacheBindsTheCurrentGeneration(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	source := NewVerifiedRouteSourceWithCache(fixture.db, fixture.encodedKey, NewVerifiedRouteCache(0))
	route, err := source.ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
	require.NoError(t, err)
	require.Equal(t, uint64(7), route.Generation)
	reads := fixture.artifactReads.Load()

	fixture.updateInstallation(t, "knowledge", map[string]any{"lifecycle_generation": 9})
	route, err = source.ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
	require.NoError(t, err)
	require.Equal(t, uint64(9), route.Generation)
	require.Equal(t, reads, fixture.artifactReads.Load(), "a generation bump reuses the verified declaration")
}

func TestVerifiedRouteCacheVerifiesANewReleaseVersion(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	cache := NewVerifiedRouteCache(0)
	source := NewVerifiedRouteSourceWithCache(fixture.db, fixture.encodedKey, cache)
	_, err := source.ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
	require.NoError(t, err)
	reads := fixture.artifactReads.Load()

	const newPath = "/api/v2/user/knowledge/:id"
	fixture.addRelease(t, "knowledge", "4.0.1", knowledgeRoutes(newPath, "knowledge.user.knowledge.id.get"), 0)
	fixture.updateInstallation(t, "knowledge", map[string]any{"desired_version": "4.0.1", "observed_version": "4.0.1", "lifecycle_generation": 8})

	route, err := source.ResolveV2Route(context.Background(), http.MethodGet, "/api/v2/user/knowledge/12")
	require.NoError(t, err)
	require.Equal(t, "4.0.1", route.Version)
	require.Equal(t, uint64(8), route.Generation)
	require.Equal(t, "knowledge.user.knowledge.id.get", route.PackageRoute)
	require.Greater(t, fixture.artifactReads.Load(), reads, "a new release is verified, not served from the old entry")
	require.Equal(t, 2, cache.Len())

	_, err = source.ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
	require.ErrorIs(t, err, ErrRouteNotDeclared)
}

func TestVerifiedRouteCacheDoesNotCacheFailures(t *testing.T) {
	fixture := newSignedRouteFixture(t, false)
	release := fixture.registerRelease(t, "knowledge", "4.0.0", knowledgeRoutes(knowledgeListPath, "knowledge.article.list"), 0)
	fixture.install(t, "knowledge", "4.0.0", 7)
	cache := NewVerifiedRouteCache(0)
	source := NewVerifiedRouteSourceWithCache(fixture.db, fixture.encodedKey, cache)

	_, err := source.ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
	require.ErrorIs(t, err, ErrPackageUnavailable, "an active release without its artifact fails closed")
	require.Zero(t, cache.Len())

	_, err = service.StorePluginArtifact(fixture.db, release.ID, release.artifact)
	require.NoError(t, err)
	route, err := source.ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
	require.NoError(t, err)
	require.Equal(t, "knowledge.article.list", route.PackageRoute)
	require.Equal(t, 1, cache.Len())
}

func TestVerifiedRouteCacheSharesConcurrentVerification(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	source := NewVerifiedRouteSourceWithCache(fixture.db, fixture.encodedKey, NewVerifiedRouteCache(0))
	uncached := NewVerifiedRouteSourceWithCache(fixture.db, fixture.encodedKey, nil)
	_, err := uncached.ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
	require.NoError(t, err)
	perVerification := fixture.artifactReads.Swap(0)

	var group sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := source.ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, perVerification, fixture.artifactReads.Load(), "concurrent misses share one verification")
}

func TestVerifiedRouteCacheEvictsLeastRecentlyUsed(t *testing.T) {
	cache := NewVerifiedRouteCache(2)
	first := verifiedRouteCacheKey{releaseID: 1}
	second := verifiedRouteCacheKey{releaseID: 2}
	third := verifiedRouteCacheKey{releaseID: 3}
	cache.put(first, []Route{{PackageRoute: "first"}})
	cache.put(second, []Route{{PackageRoute: "second"}})
	_, ok := cache.get(first)
	require.True(t, ok)
	cache.put(third, []Route{{PackageRoute: "third"}})

	require.Equal(t, 2, cache.Len())
	_, ok = cache.get(second)
	require.False(t, ok, "the least recently used entry is evicted")
	_, ok = cache.get(first)
	require.True(t, ok)
	_, ok = cache.get(third)
	require.True(t, ok)
}

func TestVerifiedRouteCacheKeySeparatesConfiguredTrustRoots(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	cache := NewVerifiedRouteCache(0)
	_, err := NewVerifiedRouteSourceWithCache(fixture.db, fixture.encodedKey, cache).ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
	require.NoError(t, err)

	otherKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	other := NewVerifiedRouteSourceWithCache(fixture.db, base64.StdEncoding.EncodeToString(otherKey), cache)
	_, err = other.ResolveV2Route(context.Background(), http.MethodGet, knowledgeListPath)
	require.NoError(t, err, "the stored, still-active trust root verifies the release")
	require.Equal(t, 2, cache.Len(), "entries are keyed by the configured trust root as well")
}

func TestVerifiedRouteCacheSelectsMostSpecificRoutePerRequest(t *testing.T) {
	fixture := newSignedRouteFixture(t, false)
	routes := `{"api_version":"v2","package_id":"knowledge","routes":[` +
		`{"method":"GET","legacy_path":"/api/v2/user/knowledge/:id","package_route":"knowledge.user.knowledge.id.get","envelope":"data"},` +
		`{"method":"GET","legacy_path":"/api/v2/user/knowledge/stats","package_route":"knowledge.user.knowledge.stats.get","envelope":"data"}]}`
	fixture.addRelease(t, "knowledge", "4.0.0", routes, 0)
	fixture.install(t, "knowledge", "4.0.0", 7)
	source := NewVerifiedRouteSourceWithCache(fixture.db, fixture.encodedKey, NewVerifiedRouteCache(0))

	for range 2 { // cold, then from the cache
		route, err := source.ResolveV2Route(context.Background(), http.MethodGet, "/api/v2/user/knowledge/stats")
		require.NoError(t, err)
		require.Equal(t, "knowledge.user.knowledge.stats.get", route.PackageRoute)
		require.Equal(t, uint64(7), route.Generation)
		route, err = source.ResolveV2Route(context.Background(), http.MethodGet, "/api/v2/user/knowledge/12")
		require.NoError(t, err)
		require.Equal(t, "knowledge.user.knowledge.id.get", route.PackageRoute)
	}
}
