package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// rehearsedPackages are the 15 packages the owner signed off after the
// staging rehearsal (H7), by batch.
var rehearsedPackages = [][]string{
	{"knowledge", "ticket"},
	{"notification", "platform", "machine-telemetry", "protocol-runtime"},
	{"plan", "order", "payment", "affiliate"},
	{"subscription", "forward", "proxy-node", "gost-mesh", "wireguard"},
}

// useRouteDefaultPolicy sets the process's default policy for one test.
func useRouteDefaultPolicy(t *testing.T, policy string) {
	t.Helper()
	previous := PackageRouteDefaultPolicy()
	SetPackageRouteDefaultPolicy(policy)
	t.Cleanup(func() { SetPackageRouteDefaultPolicy(previous) })
}

// TestPackageRouteDefaultsGate is the gate on the default set: it is exactly
// the rehearsed list (testdata/rehearsed-routes.json, the R5 record), every
// route is native-flagged for its package in the extraction map, and none is
// identity's, kernel-owned or a WebSocket route. A route that becomes
// native-flagged later fails nothing here and does not default to native:
// it joins the set only by an explicit change of both files.
func TestPackageRouteDefaultsGate(t *testing.T) {
	defaults, err := LoadPackageRouteDefaults()
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join("testdata", "rehearsed-routes.json"))
	require.NoError(t, err)
	var rehearsed [][3]string
	require.NoError(t, json.Unmarshal(raw, &rehearsed))
	require.Len(t, rehearsed, 151)
	want := make([]string, 0, len(rehearsed))
	for _, row := range rehearsed {
		require.Contains(t, []string{"read", "write"}, row[2], row[1])
		want = append(want, row[0]+" "+row[1])
	}
	got := []string{}
	for _, pkg := range defaults.Packages {
		for _, route := range pkg.Routes {
			got = append(got, pkg.PackageID+" "+route)
		}
	}
	sort.Strings(want)
	sort.Strings(got)
	assert.Equal(t, want, got, "config/package-route-defaults.json must list exactly the rehearsed routes")
	assert.Equal(t, 151, defaults.RouteCount())

	// The packages and batches the owner signed off, at the rehearsed
	// release.
	require.Len(t, defaults.Packages, 15)
	for _, pkg := range defaults.Packages {
		require.GreaterOrEqual(t, pkg.Batch, 1, pkg.PackageID)
		require.LessOrEqual(t, pkg.Batch, len(rehearsedPackages), pkg.PackageID)
		assert.Contains(t, rehearsedPackages[pkg.Batch-1], pkg.PackageID)
		assert.Equal(t, "4.1.0-rc.5", pkg.MinVersion, pkg.PackageID)
	}

	catalog, err := PackageRouteCatalog()
	require.NoError(t, err)
	inventoryRaw, err := os.ReadFile(filepath.Join("..", "..", "config", "v2-package-route-catalog.json"))
	require.NoError(t, err)
	var inventory []struct {
		RouteID   string `json:"route_id"`
		Transport string `json:"transport"`
	}
	require.NoError(t, json.Unmarshal(inventoryRaw, &inventory))
	transports := map[string]string{}
	for _, route := range inventory {
		transports[route.RouteID] = route.Transport
	}
	for _, pkg := range defaults.Packages {
		assert.NotEqual(t, IdentityPlatformPackageID, pkg.PackageID)
		for _, route := range pkg.Routes {
			entry, listed := catalog[route]
			require.True(t, listed, "%s is not in package-extraction.json", route)
			assert.Equal(t, pkg.PackageID, entry.PackageID, route)
			assert.Equal(t, RouteCatalogNativeFlagged, entry.Mode, "%s must be native-flagged", route)
			assert.NotEqual(t, RouteCatalogKernelOwned, entry.Mode, route)
			assert.NotContains(t, IdentityGroupARoutes, route)
			assert.NotContains(t, IdentityAccountReadRoutes, route)
			transport, known := transports[route]
			require.True(t, known, "%s is not in v2-package-route-catalog.json", route)
			assert.NotEqual(t, "websocket", transport, route)
		}
	}
}

func TestPackageRouteDefaultsParse(t *testing.T) {
	for name, document := range map[string]string{
		"format":        `{"format":"x","default_mode":"native","packages":[]}`,
		"mode":          `{"format":"anixops.package-route-defaults/v1","default_mode":"shadow","packages":[]}`,
		"version":       `{"format":"anixops.package-route-defaults/v1","default_mode":"native","packages":[{"package_id":"a","min_version":"latest","routes":["a.b"]}]}`,
		"duplicate":     `{"format":"anixops.package-route-defaults/v1","default_mode":"native","packages":[{"package_id":"a","min_version":"4.1.0","routes":["a.b"]},{"package_id":"c","min_version":"4.1.0","routes":["a.b"]}]}`,
		"empty package": `{"format":"anixops.package-route-defaults/v1","default_mode":"native","packages":[{"package_id":"a","min_version":"4.1.0"}]}`,
	} {
		_, err := parsePackageRouteDefaults([]byte(document))
		assert.Error(t, err, name)
	}
}

func TestPackageReleaseAtLeast(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"4.1.0-rc.5", true},
		{"4.1.0", true}, // the final release clears the rehearsed rc
		{"v4.1.0", true},
		{"4.1.1", true},
		{"4.2.0-rc.1", true},
		{"4.1.0-rc.10", true},
		{"4.1.0-rc.4", false},
		{"4.1.0-rc.1", false},
		{"4.0.0", false},
		{"4.0.1", false},
		{"dev", false},
		{"", false},
	} {
		assert.Equal(t, tc.want, PackageReleaseAtLeast(tc.version, "4.1.0-rc.5"), tc.version)
	}
}

func TestPackageRouteDefaultPolicy(t *testing.T) {
	useRouteDefaultPolicy(t, PackageRouteDefaultRehearsed)
	assert.Equal(t, PackageRouteDefaultRehearsed, PackageRouteDefaultPolicy())
	summary, err := PackageRouteDefaultsSummary()
	require.NoError(t, err)
	assert.Contains(t, summary, "policy rehearsed")
	assert.Contains(t, summary, "151 routes of 15 packages default to native")

	SetPackageRouteDefaultPolicy(" Legacy ")
	assert.Equal(t, PackageRouteDefaultLegacy, PackageRouteDefaultPolicy())
	summary, err = PackageRouteDefaultsSummary()
	require.NoError(t, err)
	assert.Contains(t, summary, "policy legacy")
	assert.Contains(t, summary, "0 routes default to native")

	SetPackageRouteDefaultPolicy("anything else")
	assert.Equal(t, PackageRouteDefaultRehearsed, PackageRouteDefaultPolicy())
}

// TestResolveEffectivePackageRouteModesOnlyDefaultsTheSet resolves every
// package of the extraction map at a new release with nothing stored:
// exactly the 151 rehearsed routes come out native; every other
// native-flagged route (identity's among them) stays legacy.
func TestResolveEffectivePackageRouteModesOnlyDefaultsTheSet(t *testing.T) {
	useRouteDefaultPolicy(t, PackageRouteDefaultRehearsed)
	db := openRouteModeSQLite(t)
	prepareRouteModeDB(t, db)
	defaults, err := LoadPackageRouteDefaults()
	require.NoError(t, err)
	catalog, err := PackageRouteCatalog()
	require.NoError(t, err)
	packages := map[string]bool{}
	for _, entry := range catalog {
		packages[entry.PackageID] = true
	}
	native := 0
	for packageID := range packages {
		modes, sources, err := ResolveEffectivePackageRouteModes(db, packageID, "9.9.9", map[string]string{})
		require.NoError(t, err)
		for route, mode := range modes {
			require.Equal(t, packagebridge.RouteModeNative, mode, route)
			require.True(t, defaults.Includes(packageID, route), "%s defaults to native but is not in the set", route)
			require.Equal(t, RouteModeSourceDefault, sources[route])
			native++
		}
	}
	assert.Equal(t, 151, native)

	// Identity group A follows the authority only.
	modes, _, err := ResolveEffectivePackageRouteModes(db, IdentityPlatformPackageID, "9.9.9", map[string]string{})
	require.NoError(t, err)
	assert.Empty(t, modes)
}

// knowledgeRoutes are knowledge's six compatibility routes as the rehearsed
// release declares them (all in the default set), plus one the default set
// does not hold.
var knowledgeRoutes = []compatibilityRoute{
	{Method: "GET", LegacyPath: "/api/v2/admin/knowledge", PackageRoute: "knowledge.admin.knowledge.get", Transport: "http"},
	{Method: "POST", LegacyPath: "/api/v2/admin/knowledge", PackageRoute: "knowledge.admin.knowledge.post", Transport: "http"},
	{Method: "DELETE", LegacyPath: "/api/v2/admin/knowledge/:id", PackageRoute: "knowledge.admin.knowledge.id.delete", Transport: "http"},
	{Method: "PUT", LegacyPath: "/api/v2/admin/knowledge/:id", PackageRoute: "knowledge.admin.knowledge.id.put", Transport: "http"},
	{Method: "GET", LegacyPath: "/api/v2/user/knowledge", PackageRoute: "knowledge.article.list", Transport: "http"},
	{Method: "GET", LegacyPath: "/api/v2/user/knowledge/:id", PackageRoute: "knowledge.user.knowledge.id.get", Transport: "http"},
	{Method: "GET", LegacyPath: "/api/v2/admin/knowledge/bridged", PackageRoute: "knowledge.bridged.get", Transport: "http"},
}

var knowledgeDefaultRoutes = []string{
	"knowledge.admin.knowledge.get", "knowledge.admin.knowledge.id.delete", "knowledge.admin.knowledge.id.put",
	"knowledge.admin.knowledge.post", "knowledge.article.list", "knowledge.user.knowledge.id.get",
}

func knowledgeCatalog() map[string]RouteCatalogEntry {
	catalog := map[string]RouteCatalogEntry{
		"knowledge.bridged.get": {PackageID: "knowledge", RouteID: "knowledge.bridged.get", Mode: RouteCatalogBridged},
	}
	for _, route := range knowledgeDefaultRoutes {
		catalog[route] = RouteCatalogEntry{PackageID: "knowledge", RouteID: route, Mode: RouteCatalogNativeFlagged}
	}
	return catalog
}

func nativeModes(routes ...string) map[string]string {
	modes := map[string]string{}
	for _, route := range routes {
		modes[route] = packagebridge.RouteModeNative
	}
	return modes
}

func storedRouteModes(t *testing.T, db *gorm.DB, installationID uint) map[string]string {
	t.Helper()
	configuration, err := GetPluginConfiguration(db, installationID)
	require.NoError(t, err)
	_, stored, _, err := splitPackageRouteModes(configuration.ConfigJSON)
	require.NoError(t, err)
	return stored
}

func TestRouteModeDefaultsSQLite(t *testing.T) {
	runRouteModeDefaultsSuite(t, openRouteModeSQLite)
}

// TestPostgresRouteModeDefaults runs the default-mode suite on PostgreSQL;
// CI's PostgreSQL regression job selects it by its name.
func TestPostgresRouteModeDefaults(t *testing.T) {
	runRouteModeDefaultsSuite(t, openRouteModePostgres)
}

func runRouteModeDefaultsSuite(t *testing.T, open func(*testing.T) *gorm.DB) {
	t.Run("rc.5 install without routes", func(t *testing.T) { testRouteModeDefaultsFreshInstall(t, open(t)) })
	t.Run("explicit legacy", func(t *testing.T) { testRouteModeDefaultsExplicitLegacy(t, open(t)) })
	t.Run("rollback", func(t *testing.T) { testRouteModeDefaultsRollback(t, open(t)) })
	t.Run("whole package set", func(t *testing.T) { testRouteModeDefaultsWholePackage(t, open(t)) })
	t.Run("kill switch", func(t *testing.T) { testRouteModeDefaultsKillSwitch(t, open(t)) })
	t.Run("package too old", func(t *testing.T) { testRouteModeDefaultsTooOld(t, open(t)) })
	t.Run("identity untouched", func(t *testing.T) { testRouteModeDefaultsIdentity(t, open(t)) })
}

func seedKnowledgeRC5(t *testing.T, db *gorm.DB, version string) (*RouteModeAdmin, PackageHostOperations, model.PluginInstallation) {
	t.Helper()
	prepareRouteModeDB(t, db)
	publicKey, installation := seedRouteModePackageVersion(t, db, "knowledge", version, knowledgeRoutes)
	return &RouteModeAdmin{DB: db, PublicKey: publicKey, Catalog: knowledgeCatalog()}, PackageHostOperations{DB: db}, installation
}

func hostModes(t *testing.T, operations PackageHostOperations, version string) map[string]string {
	t.Helper()
	config, err := operations.PackageConfig(context.Background(), packagebridge.HostIdentity{PackageID: "knowledge", Version: version, Generation: 7})
	require.NoError(t, err)
	return config.RouteModes
}

// An rc.5-shaped installation stores no routes key: the rehearsed routes
// run natively from the default, the others stay legacy, and the host gets
// the resolved map.
func testRouteModeDefaultsFreshInstall(t *testing.T, db *gorm.DB) {
	useRouteDefaultPolicy(t, PackageRouteDefaultRehearsed)
	admin, operations, installation := seedKnowledgeRC5(t, db, "4.1.0-rc.5")
	configuration, err := GetPluginConfiguration(db, installation.ID)
	require.NoError(t, err)
	assert.NotContains(t, configuration.ConfigJSON, `"routes"`)

	packages, err := admin.List(context.Background(), "knowledge", nil)
	require.NoError(t, err)
	require.Len(t, packages, 1)
	assert.Equal(t, PackageRouteDefaultState{Policy: "rehearsed", Routes: 6, MinVersion: "4.1.0-rc.5", Source: RouteModeSourceDefault}, packages[0].Defaults)
	for _, route := range knowledgeDefaultRoutes {
		entry := routeModeEntry(t, packages, route)
		assert.Equal(t, "native", entry.Effective, route)
		assert.Equal(t, "native", entry.Configured, route)
		assert.Empty(t, entry.Stored, route)
		assert.Equal(t, RouteModeSourceDefault, entry.Source, route)
	}
	bridged := routeModeEntry(t, packages, "knowledge.bridged.get")
	assert.Equal(t, "legacy", bridged.Effective)
	assert.Equal(t, RouteModeSourceUnset, bridged.Source)
	assert.Equal(t, nativeModes(knowledgeDefaultRoutes...), hostModes(t, operations, "4.1.0-rc.5"))

	// The final release clears the rehearsed rc.
	_, sources, err := ResolveEffectivePackageRouteModes(db, "knowledge", "4.1.0", nil)
	require.NoError(t, err)
	assert.Equal(t, RouteModeSourceDefault, sources["knowledge.article.list"])

	// Switching a defaulted route to native changes nothing.
	result, err := admin.Set(context.Background(), RouteModeChangeRequest{PackageID: "knowledge", Routes: []string{"knowledge.article.list"}, Mode: "native", Confirm: true, Reason: "x"}, cliActor)
	require.NoError(t, err)
	assert.Empty(t, result.Changes)
	assert.Empty(t, result.GroupID)
}

// set --mode legacy on a defaulted route stores an explicit legacy, which
// stays legacy; the other defaulted routes stay native.
func testRouteModeDefaultsExplicitLegacy(t *testing.T, db *gorm.DB) {
	useRouteDefaultPolicy(t, PackageRouteDefaultRehearsed)
	admin, operations, installation := seedKnowledgeRC5(t, db, "4.1.0-rc.5")
	ctx := context.Background()

	result, err := admin.Set(ctx, RouteModeChangeRequest{PackageID: "knowledge", Routes: []string{"knowledge.article.list"}, Mode: "legacy", Reason: "keep legacy"}, cliActor)
	require.NoError(t, err)
	require.NotEmpty(t, result.GroupID)
	assert.Equal(t, []RouteModeChange{{RouteID: "knowledge.article.list", From: "native", To: "legacy"}}, result.Changes)
	assert.Equal(t, map[string]string{"knowledge.article.list": "legacy"}, storedRouteModes(t, db, installation.ID))

	packages, err := admin.List(ctx, "knowledge", nil)
	require.NoError(t, err)
	entry := routeModeEntry(t, packages, "knowledge.article.list")
	assert.Equal(t, "legacy", entry.Effective)
	assert.Equal(t, "legacy", entry.Configured)
	assert.Equal(t, "legacy", entry.Stored)
	assert.Equal(t, RouteModeSourceStored, entry.Source)
	modes := hostModes(t, operations, "4.1.0-rc.5")
	assert.NotContains(t, modes, "knowledge.article.list")
	assert.Len(t, modes, 5)

	// Repeating it changes nothing.
	again, err := admin.Set(ctx, RouteModeChangeRequest{PackageID: "knowledge", Routes: []string{"knowledge.article.list"}, Mode: "legacy"}, cliActor)
	require.NoError(t, err)
	assert.Empty(t, again.Changes)

	// Back to native: the stored mode is native.
	back, err := admin.Set(ctx, RouteModeChangeRequest{PackageID: "knowledge", Routes: []string{"knowledge.article.list"}, Mode: "native", Confirm: true, Reason: "again"}, cliActor)
	require.NoError(t, err)
	assert.Equal(t, []RouteModeChange{{RouteID: "knowledge.article.list", From: "legacy", To: "native"}}, back.Changes)
	assert.Equal(t, nativeModes(knowledgeDefaultRoutes...), hostModes(t, operations, "4.1.0-rc.5"))

	revisions, err := admin.Revisions(ctx, "knowledge", 0)
	require.NoError(t, err)
	require.Len(t, revisions, 2)
}

// A rollback of a defaulted package writes an explicit legacy for each
// defaulted route, with revision rows and an audit entry.
func testRouteModeDefaultsRollback(t *testing.T, db *gorm.DB) {
	useRouteDefaultPolicy(t, PackageRouteDefaultRehearsed)
	admin, operations, installation := seedKnowledgeRC5(t, db, "4.1.0-rc.5")
	ctx := context.Background()

	rollback, err := admin.Rollback(ctx, "knowledge", "incident 42", cliActor)
	require.NoError(t, err)
	require.NotEmpty(t, rollback.GroupID)
	require.Len(t, rollback.Changes, len(knowledgeDefaultRoutes))
	for _, change := range rollback.Changes {
		assert.Equal(t, "native", change.From, change.RouteID)
		assert.Equal(t, "legacy", change.To, change.RouteID)
	}
	assert.Empty(t, hostModes(t, operations, "4.1.0-rc.5"))
	stored := storedRouteModes(t, db, installation.ID)
	assert.Len(t, stored, len(knowledgeDefaultRoutes))
	for _, route := range knowledgeDefaultRoutes {
		assert.Equal(t, "legacy", stored[route], route)
	}

	packages, err := admin.List(ctx, "knowledge", nil)
	require.NoError(t, err)
	for _, entry := range packages[0].Routes {
		assert.Equal(t, "legacy", entry.Effective, entry.RouteID)
	}

	revisions, err := admin.Revisions(ctx, "knowledge", 0)
	require.NoError(t, err)
	require.Len(t, revisions, len(knowledgeDefaultRoutes))
	for _, revision := range revisions {
		assert.Equal(t, model.RouteModeRevisionActionRollback, revision.Action)
		assert.Equal(t, "native", revision.FromMode)
		assert.Equal(t, "legacy", revision.ToMode)
		assert.Equal(t, "incident 42", revision.Reason)
		assert.Equal(t, rollback.GroupID, revision.GroupID)
	}
	var audits []model.OperationLog
	require.NoError(t, db.Find(&audits).Error)
	require.Len(t, audits, 1)
	assert.Equal(t, "route_mode_rollback", audits[0].Action)

	// The pinned legacy survives a later kill switch flip.
	SetPackageRouteDefaultPolicy(PackageRouteDefaultLegacy)
	SetPackageRouteDefaultPolicy(PackageRouteDefaultRehearsed)
	assert.Empty(t, hostModes(t, operations, "4.1.0-rc.5"))

	idle, err := admin.Rollback(ctx, "knowledge", "", cliActor)
	require.NoError(t, err)
	assert.Empty(t, idle.Changes)
	assert.Empty(t, idle.GroupID)
}

// The whole-package set compares against the effective modes.
func testRouteModeDefaultsWholePackage(t *testing.T, db *gorm.DB) {
	useRouteDefaultPolicy(t, PackageRouteDefaultRehearsed)
	admin, operations, _ := seedKnowledgeRC5(t, db, "4.1.0-rc.5")
	ctx := context.Background()

	native, err := admin.Set(ctx, RouteModeChangeRequest{PackageID: "knowledge", Mode: "native", Confirm: true, Reason: "x"}, cliActor)
	require.NoError(t, err)
	assert.Empty(t, native.Changes, "every native-capable route already runs natively by default")
	assert.Equal(t, []RouteModeSkip{{RouteID: "knowledge.bridged.get", Reason: "bridged: the package has no native implementation yet"}}, native.Skipped)

	legacy, err := admin.Set(ctx, RouteModeChangeRequest{PackageID: "knowledge", Mode: "legacy"}, cliActor)
	require.NoError(t, err)
	assert.Len(t, legacy.Changes, len(knowledgeDefaultRoutes))
	assert.Empty(t, hostModes(t, operations, "4.1.0-rc.5"))
}

// The kill switch turns every default off; a stored mode still wins.
func testRouteModeDefaultsKillSwitch(t *testing.T, db *gorm.DB) {
	useRouteDefaultPolicy(t, PackageRouteDefaultLegacy)
	admin, operations, _ := seedKnowledgeRC5(t, db, "4.1.0-rc.5")
	ctx := context.Background()

	assert.Empty(t, hostModes(t, operations, "4.1.0-rc.5"))
	packages, err := admin.List(ctx, "knowledge", nil)
	require.NoError(t, err)
	assert.Equal(t, RouteModeSourceKillSwitch, packages[0].Defaults.Source)
	assert.Contains(t, packages[0].Defaults.Note, "default_mode is legacy")
	for _, route := range knowledgeDefaultRoutes {
		entry := routeModeEntry(t, packages, route)
		assert.Equal(t, "legacy", entry.Effective, route)
		assert.Equal(t, "legacy", entry.Configured, route)
		assert.Equal(t, RouteModeSourceKillSwitch, entry.Source, route)
	}
	// A rollback has nothing to do.
	idle, err := admin.Rollback(ctx, "knowledge", "", cliActor)
	require.NoError(t, err)
	assert.Empty(t, idle.Changes)

	// An operator's stored native still applies.
	set, err := admin.Set(ctx, RouteModeChangeRequest{PackageID: "knowledge", Routes: []string{"knowledge.article.list"}, Mode: "native", Confirm: true, Reason: "x"}, cliActor)
	require.NoError(t, err)
	assert.Equal(t, []RouteModeChange{{RouteID: "knowledge.article.list", From: "legacy", To: "native"}}, set.Changes)
	assert.Equal(t, nativeModes("knowledge.article.list"), hostModes(t, operations, "4.1.0-rc.5"))

	// Lifting the switch restores the defaults.
	SetPackageRouteDefaultPolicy(PackageRouteDefaultRehearsed)
	assert.Equal(t, nativeModes(knowledgeDefaultRoutes...), hostModes(t, operations, "4.1.0-rc.5"))
}

// A release older than the rehearsed one stays legacy and the list says
// why.
func testRouteModeDefaultsTooOld(t *testing.T, db *gorm.DB) {
	useRouteDefaultPolicy(t, PackageRouteDefaultRehearsed)
	admin, operations, _ := seedKnowledgeRC5(t, db, "4.0.0")
	assert.Empty(t, hostModes(t, operations, "4.0.0"))
	packages, err := admin.List(context.Background(), "knowledge", nil)
	require.NoError(t, err)
	assert.Equal(t, RouteModeSourcePackageTooOld, packages[0].Defaults.Source)
	assert.Contains(t, packages[0].Defaults.Note, "4.0.0 is older than 4.1.0-rc.5")
	for _, route := range knowledgeDefaultRoutes {
		entry := routeModeEntry(t, packages, route)
		assert.Equal(t, "legacy", entry.Effective, route)
		assert.Equal(t, RouteModeSourcePackageTooOld, entry.Source, route)
	}
}

// Identity's routes are not in the default set: identity group A follows
// the authority only, and the rest of identity-platform stays legacy.
func testRouteModeDefaultsIdentity(t *testing.T, db *gorm.DB) {
	useRouteDefaultPolicy(t, PackageRouteDefaultRehearsed)
	prepareRouteModeDB(t, db)
	routes := []compatibilityRoute{{Method: "GET", LegacyPath: "/api/v2/admin/invite-codes", PackageRoute: "identity.admin.invite_codes.get", Transport: "http"}}
	catalog := map[string]RouteCatalogEntry{"identity.admin.invite_codes.get": {PackageID: IdentityPlatformPackageID, Mode: RouteCatalogNativeFlagged}}
	for i, route := range IdentityGroupARoutes {
		routes = append(routes, compatibilityRoute{Method: "POST", LegacyPath: "/api/v2/group-a/" + string(rune('a'+i)), PackageRoute: route, Transport: "http"})
		catalog[route] = RouteCatalogEntry{PackageID: IdentityPlatformPackageID, Mode: RouteCatalogNativeFlagged}
	}
	publicKey, _ := seedRouteModePackageVersion(t, db, IdentityPlatformPackageID, "4.1.0-rc.5", routes)
	admin := &RouteModeAdmin{DB: db, PublicKey: publicKey, Catalog: catalog}
	packages, err := admin.List(context.Background(), IdentityPlatformPackageID, nil)
	require.NoError(t, err)
	assert.Equal(t, PackageRouteDefaultState{Policy: "rehearsed"}, packages[0].Defaults)
	for _, entry := range packages[0].Routes {
		assert.Equal(t, "legacy", entry.Effective, entry.RouteID)
		assert.Equal(t, RouteModeSourceUnset, entry.Source, entry.RouteID)
	}

	// Once identity is authoritative group A is native from the authority,
	// never from the default set.
	require.NoError(t, db.Save(&model.IdentityAuthority{ID: 1, State: model.IdentityAuthorityIdentity}).Error)
	modes, sources, err := ResolveEffectivePackageRouteModes(db, IdentityPlatformPackageID, "4.1.0-rc.5", map[string]string{})
	require.NoError(t, err)
	assert.Len(t, modes, len(IdentityGroupARoutes))
	for route := range modes {
		assert.True(t, slices.Contains(IdentityGroupARoutes, route), route)
		assert.Equal(t, RouteModeSourceIdentityAuthority, sources[route])
	}
}
