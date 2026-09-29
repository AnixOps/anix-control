package v2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func precedenceRoute(path, id string) Route {
	return Route{Method: "GET", LegacyPath: path, PackageID: "proxy-node", PackageRoute: id}
}

func TestSelectMostSpecificRoutePrefersStaticSegmentsOverParams(t *testing.T) {
	// Declaration order puts the parameter route first, as in the real
	// proxy-node, order and identity-platform catalogues.
	routes := []Route{
		precedenceRoute("/api/v2/admin/nodes", "nodes.get"),
		precedenceRoute("/api/v2/admin/nodes/:id", "nodes.id.get"),
		precedenceRoute("/api/v2/admin/nodes/:id/logs", "nodes.id.logs.get"),
		precedenceRoute("/api/v2/admin/nodes/stats", "nodes.stats.get"),
	}

	route, ok := selectMostSpecificRoute(routes, "GET", "/api/v2/admin/nodes/stats")
	require.True(t, ok)
	require.Equal(t, "nodes.stats.get", route.PackageRoute)

	route, ok = selectMostSpecificRoute(routes, "GET", "/api/v2/admin/nodes/42")
	require.True(t, ok)
	require.Equal(t, "nodes.id.get", route.PackageRoute)

	route, ok = selectMostSpecificRoute(routes, "GET", "/api/v2/admin/nodes/42/logs")
	require.True(t, ok)
	require.Equal(t, "nodes.id.logs.get", route.PackageRoute)

	_, ok = selectMostSpecificRoute(routes, "POST", "/api/v2/admin/nodes/stats")
	require.False(t, ok)
}

func TestMostSpecificRouteComparesSegmentsLeftToRight(t *testing.T) {
	// For /a/c/b: /a/c/:y has a static second segment, so it wins over /a/:x/b.
	best, ambiguous := mostSpecificRoute([]Route{
		precedenceRoute("/a/:x/b", "left"),
		precedenceRoute("/a/c/:y", "right"),
	})
	require.False(t, ambiguous)
	require.Equal(t, "right", best.PackageRoute)

	_, ambiguous = mostSpecificRoute([]Route{
		precedenceRoute("/a/:x", "one"),
		precedenceRoute("/a/:y", "two"),
	})
	require.True(t, ambiguous, "equally specific patterns stay a conflict")
}
