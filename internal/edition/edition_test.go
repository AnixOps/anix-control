package edition

import (
	"os"
	"path/filepath"
	"testing"

	configtables "github.com/AnixOps/anix-control/v4/config"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedTablesAreValid(t *testing.T) {
	table, routes, err := parse(configtables.Editions, configtables.PackageExtraction)
	require.NoError(t, err)
	require.Equal(t, config.EditionCommunity, table.Default)
	require.NotEmpty(t, table.CommercialPackages)

	packages := map[string]bool{}
	routeIDs := map[string]bool{}
	for _, route := range routes {
		packages[route.PackageID] = true
		routeIDs[route.RouteID] = true
	}
	for _, id := range table.CommercialPackages {
		require.True(t, packages[id], "commercial package %s owns no /api/v2 route", id)
		_, err := os.Stat(filepath.Join("..", "..", "packages", id, "manifest.template.json"))
		require.NoError(t, err, "commercial package %s is not under packages/", id)
	}
	for _, id := range table.CommercialRoutes {
		require.True(t, routeIDs[id], "commercial route %s is not in config/package-extraction.json", id)
	}
	// Plans stay in community as subscription templates: only the purchase
	// side is commercial.
	require.NotContains(t, table.CommercialPackages, "plan")
}

func TestParseRejectsInvalidTables(t *testing.T) {
	_, _, err := parse([]byte(`{"format":"other","default":"community"}`), configtables.PackageExtraction)
	require.Error(t, err)
	_, _, err = parse([]byte(`{"format":"anixops.editions/v1","default":"enterprise"}`), configtables.PackageExtraction)
	require.Error(t, err)
	_, _, err = parse([]byte(`{"format":"anixops.editions/v1","default":""}`), configtables.PackageExtraction)
	require.Error(t, err)
}

func TestPolicyFollowsTheConfiguredEdition(t *testing.T) {
	require.Equal(t, config.EditionCommunity, For(nil).Name())
	require.Equal(t, config.EditionCommunity, For(&config.Config{}).Name())
	require.Equal(t, config.EditionCommunity, New("bogus").Name())
	commercial := For(&config.Config{App: config.AppConfig{Edition: "Commercial"}})
	require.True(t, commercial.Commercial())
	require.False(t, commercial.Hides("payment", "payment.payment.methods.get"))

	community := For(&config.Config{App: config.AppConfig{Edition: config.EditionCommunity}})
	require.False(t, community.Commercial())
	require.True(t, community.Hides("payment", "payment.payment.methods.get"))
	require.True(t, community.Hides("plan", "plan.user.plan.get"))
	require.False(t, community.Hides("plan", "plan.admin.plans.post"))
	require.True(t, community.HidesRequest("get", "/api/v2/user/order"))
	require.False(t, community.HidesRequest("GET", "/api/v2/user/subscription"))
	require.False(t, community.HidesRequest("GET", ""))
}
