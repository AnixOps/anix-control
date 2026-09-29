package v2

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"testing"
)

// benchmarkPackageCount and the default payload model a production control
// plane: 16 installed signed packages with ~10 MiB artifacts each.
const (
	benchmarkPackageCount         = 16
	benchmarkDefaultArtifactBytes = 10 << 20
	// benchmarkArtifactBytesEnvironment overrides the per-package payload,
	// e.g. ANIX_BENCH_ROUTE_ARTIFACT_BYTES=65536 for a quick smoke run.
	benchmarkArtifactBytesEnvironment = "ANIX_BENCH_ROUTE_ARTIFACT_BYTES"
)

func benchmarkArtifactBytes(b *testing.B) int {
	b.Helper()
	raw := os.Getenv(benchmarkArtifactBytesEnvironment)
	if raw == "" {
		return benchmarkDefaultArtifactBytes
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		b.Fatalf("%s must be a non-negative byte count", benchmarkArtifactBytesEnvironment)
	}
	return value
}

// BenchmarkResolveV2Route measures one request-time route resolution over 16
// installed packages on an on-disk SQLite database, with and without the
// verified-route cache. Without the cache every resolution re-reads and
// re-hashes every installed artifact.
func BenchmarkResolveV2Route(b *testing.B) {
	fixture := newSignedRouteFixture(b, true)
	artifactBytes := benchmarkArtifactBytes(b)
	lastPath := ""
	for index := range benchmarkPackageCount {
		packageID := fmt.Sprintf("bench-package-%02d", index)
		lastPath = fmt.Sprintf("/api/v2/bench/%02d/items/:id", index)
		routes := `{"api_version":"v2","package_id":"` + packageID + `","routes":[{"method":"GET","legacy_path":"` + lastPath +
			`","package_route":"` + packageID + `.items.get","envelope":"data"}]}`
		fixture.addRelease(b, packageID, "4.0.0", routes, artifactBytes)
		fixture.install(b, packageID, "4.0.0", 7)
	}
	requestPath := fmt.Sprintf("/api/v2/bench/%02d/items/42", benchmarkPackageCount-1)
	b.ReportMetric(float64(artifactBytes), "artifact-bytes")

	for _, variant := range []struct {
		name  string
		cache *VerifiedRouteCache
	}{
		{name: "cached", cache: NewVerifiedRouteCache(0)},
		{name: "uncached", cache: nil},
	} {
		b.Run(variant.name, func(b *testing.B) {
			source := NewVerifiedRouteSourceWithCache(fixture.db, fixture.encodedKey, variant.cache)
			// Warm the cache so the cached variant measures steady state.
			if _, err := source.ResolveV2Route(context.Background(), http.MethodGet, requestPath); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				route, err := source.ResolveV2Route(context.Background(), http.MethodGet, requestPath)
				if err != nil || route.Generation != 7 {
					b.Fatalf("resolve: route=%+v err=%v", route, err)
				}
			}
		})
	}
}
