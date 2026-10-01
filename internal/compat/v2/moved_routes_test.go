package v2

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

const systemConfigsPath = "/api/v2/admin/system/configs"

func singleRoute(packageID, routeID string) string {
	return `{"api_version":"v2","package_id":"` + packageID + `","routes":[{"method":"GET","legacy_path":"` +
		systemConfigsPath + `","package_route":"` + routeID + `","envelope":"panel"}]}`
}

// During an upgrade an old identity-platform release and the new platform
// package both declare the moved route; the new owner serves it.
func TestMovedRouteResolvesToTheNewOwnerWhileBothDeclareIt(t *testing.T) {
	fixture := newSignedRouteFixture(t, false)
	fixture.addRelease(t, "identity-platform", "4.0.0", singleRoute("identity-platform", "identity.admin.system.configs.get"), 0)
	fixture.install(t, "identity-platform", "4.0.0", 3)
	source := NewVerifiedRouteSource(fixture.db, fixture.encodedKey)

	route, err := source.ResolveV2Route(context.Background(), http.MethodGet, systemConfigsPath)
	require.NoError(t, err)
	require.Equal(t, "identity-platform", route.PackageID, "the old owner serves the route until the new one is installed")
	require.Equal(t, "identity.admin.system.configs.get", route.PackageRoute)

	fixture.addRelease(t, "platform", "4.1.0", singleRoute("platform", "platform.admin.system.configs.get"), 0)
	fixture.install(t, "platform", "4.1.0", 1)
	route, err = source.ResolveV2Route(context.Background(), http.MethodGet, systemConfigsPath)
	require.NoError(t, err)
	require.Equal(t, "platform", route.PackageID)
	require.Equal(t, "platform.admin.system.configs.get", route.PackageRoute)

	fixture.updateInstallation(t, "platform", map[string]any{"enabled": false})
	route, err = source.ResolveV2Route(context.Background(), http.MethodGet, systemConfigsPath)
	require.NoError(t, err)
	require.Equal(t, "identity-platform", route.PackageID, "a disabled new owner does not hide the old declaration")
}

// The speed-limit routes moved between two packages that both have their
// own host: an old plan release and the new forward release both declare
// them, and forward serves them.
func TestSpeedLimitMovedFromPlanResolvesToForward(t *testing.T) {
	const path = "/api/v2/speed-limit/update"
	route := func(packageID, routeID string) string {
		return `{"api_version":"v2","package_id":"` + packageID + `","routes":[{"method":"POST","legacy_path":"` +
			path + `","package_route":"` + routeID + `","envelope":"panel"}]}`
	}
	fixture := newSignedRouteFixture(t, false)
	fixture.addRelease(t, "plan", "4.0.0", route("plan", "plan.speed_limit.update.post"), 0)
	fixture.install(t, "plan", "4.0.0", 2)
	fixture.addRelease(t, "forward", "4.1.0", route("forward", "forward.speed_limit.update.post"), 0)
	fixture.install(t, "forward", "4.1.0", 1)

	resolved, err := NewVerifiedRouteSource(fixture.db, fixture.encodedKey).ResolveV2Route(context.Background(), http.MethodPost, path)
	require.NoError(t, err)
	require.Equal(t, "forward", resolved.PackageID)
	require.Equal(t, "forward.speed_limit.update.post", resolved.PackageRoute)
}

// Only the declared move is preferred: any other package declaring the same
// route is still a conflict.
func TestMovedRoutePreferenceDoesNotResolveOtherConflicts(t *testing.T) {
	fixture := newSignedRouteFixture(t, false)
	fixture.addRelease(t, "identity-platform", "4.0.0", singleRoute("identity-platform", "identity.admin.system.configs.get"), 0)
	fixture.install(t, "identity-platform", "4.0.0", 3)
	fixture.addRelease(t, "knowledge", "4.0.0", singleRoute("knowledge", "knowledge.admin.system.configs.get"), 0)
	fixture.install(t, "knowledge", "4.0.0", 1)

	_, err := NewVerifiedRouteSource(fixture.db, fixture.encodedKey).ResolveV2Route(context.Background(), http.MethodGet, systemConfigsPath)
	require.True(t, errors.Is(err, ErrPackageUnavailable), err)
}

func TestMovedRoutesAreUniqueAndLeaveTheirOwner(t *testing.T) {
	seen := map[string]bool{}
	for _, moved := range MovedRoutes() {
		require.NotEqual(t, moved.FromPackage, moved.ToPackage)
		require.False(t, seen[moved.Method+" "+moved.LegacyPath], moved.LegacyPath)
		seen[moved.Method+" "+moved.LegacyPath] = true
		for _, route := range []Route{
			{PackageID: moved.FromPackage, PackageRoute: moved.FromRoute},
			{PackageID: moved.ToPackage, PackageRoute: moved.ToRoute},
		} {
			route.Method, route.LegacyPath, route.Version, route.Generation, route.Envelope = moved.Method, moved.LegacyPath, "4.0.0", 1, EnvelopePanel
			require.NoError(t, route.Validate(), route.PackageRoute)
		}
	}
	require.Len(t, seen, 28)
}
