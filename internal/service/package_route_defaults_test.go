package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// rehearsedPackages are the packages the owner signed off after the staging
// rehearsal (H7), by batch: 15, less gost-mesh and forward, whose rehearsed
// routes went with the v4.1 forwarding runtime.
var rehearsedPackages = [][]string{
	{"knowledge", "ticket"},
	{"notification", "platform", "machine-telemetry", "protocol-runtime"},
	{"plan", "order", "payment", "affiliate"},
	{"subscription", "proxy-node", "wireguard"},
}

// TestPackageRouteDefaultsGate is the gate on the rehearsed list: it is exactly
// the rehearsed list (testdata/rehearsed-routes.json, the R5 record), every
// route is native-flagged for its package in the extraction map, and none is
// identity's, kernel-owned or a WebSocket route. A route that becomes
// native-flagged later fails nothing here and does not run natively: it
// joins the list only by an explicit change of both files.
func TestPackageRouteDefaultsGate(t *testing.T) {
	defaults, err := LoadPackageRouteDefaults()
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join("testdata", "rehearsed-routes.json"))
	require.NoError(t, err)
	var rehearsed [][3]string
	require.NoError(t, json.Unmarshal(raw, &rehearsed))
	// 130 rehearsed, less the four backup routes removed with the backup
	// settings (docs/reference/backups.md) and the three v3 forward routes
	// removed with the v4.1 forwarding runtime.
	require.Len(t, rehearsed, 118)
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
	assert.Equal(t, 118, defaults.RouteCount())

	// The packages and batches the owner signed off, at the rehearsed
	// release.
	require.Len(t, defaults.Packages, 13)
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
			assert.Contains(t, []string{RouteCatalogNativeFlagged, RouteCatalogNative}, entry.Mode, "%s must be native-flagged or native", route)
			assert.NotEqual(t, RouteCatalogKernelOwned, entry.Mode, route)
			assert.NotContains(t, IdentityGroupARoutes, route)
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

func TestPackageRouteDefaultsSummary(t *testing.T) {
	summary, err := PackageRouteDefaultsSummary()
	require.NoError(t, err)
	assert.Contains(t, summary, "the 118 rehearsed routes of 13 packages")
	assert.Contains(t, summary, "every other route runs its legacy handler")
	assert.NotContains(t, summary, "default_mode")
}

// TestResolveEffectivePackageRouteModesOnlyDefaultsTheSet resolves every
// package of the extraction map at a new release: exactly the 118 rehearsed
// routes come out native; every other native-flagged route (identity's
// among them) stays legacy.
func TestResolveEffectivePackageRouteModesOnlyDefaultsTheSet(t *testing.T) {
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
		modes, err := ResolveEffectivePackageRouteModes(db, packageID, "9.9.9")
		require.NoError(t, err)
		for route, mode := range modes {
			require.Equal(t, packagebridge.RouteModeNative, mode, route)
			require.True(t, defaults.Includes(packageID, route), "%s runs natively but is not in the list", route)
			native++
		}
	}
	assert.Equal(t, 118, native)

	// Identity group A follows the authority only.
	modes, err := ResolveEffectivePackageRouteModes(db, IdentityPlatformPackageID, "9.9.9")
	require.NoError(t, err)
	assert.Empty(t, modes)
}

// testCompatibilityRoute is one compatibility route a test package declares.
type testCompatibilityRoute struct {
	Method, LegacyPath, PackageRoute, Transport string
}

// knowledgeRoutes are knowledge's six compatibility routes as the rehearsed
// release declares them (all in the list), plus one the list does not hold.
var knowledgeRoutes = []testCompatibilityRoute{
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

func nativeModes(routes ...string) map[string]string {
	modes := map[string]string{}
	for _, route := range routes {
		modes[route] = packagebridge.RouteModeNative
	}
	return modes
}

// seedRouteModePackageVersion registers, stores and installs a signed v2
// Control release version of packageID that declares routes.
func seedRouteModePackageVersion(t *testing.T, db *gorm.DB, packageID, version string, routes []testCompatibilityRoute) (ed25519.PublicKey, model.PluginInstallation) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	declared := make([]map[string]string, 0, len(routes))
	for _, route := range routes {
		envelope := "panel"
		if route.Transport == "websocket" {
			envelope = "websocket"
		}
		declared = append(declared, map[string]string{
			"method": route.Method, "legacy_path": route.LegacyPath, "package_route": route.PackageRoute,
			"envelope": envelope, "transport": route.Transport,
		})
	}
	routeDocument, err := json.Marshal(map[string]any{"api_version": "v2", "package_id": packageID, "routes": declared})
	require.NoError(t, err)
	entrypoint := []byte("#!/bin/sh\nexit 0\n")
	migrations := []byte(`{"format":"anixops.migrations/v1","migrations":[],"package_id":"` + packageID + `","version":"` + version + `"}`)
	artifact := kernelTestV2Package(t, map[string][]byte{
		"bin/control-host": entrypoint, "compat/v2-routes.json": routeDocument, "migrations/index.json": migrations,
	})
	digest := func(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
	manifest := PluginManifest{
		ID: packageID, Name: packageID, Version: version, APIVersion: pluginManifestAPIVersionV2, Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: digest(artifact),
		ControlEntrypoint:   &PluginEntrypoint{Path: "bin/control-host", SHA256: digest(entrypoint)},
		Migrations:          &PluginMigrations{Index: "migrations/index.json", SHA256: digest(migrations)},
		CompatibilityRoutes: &PluginCompatibilityRoutes{Path: "compat/v2-routes.json", SHA256: digest(routeDocument)},
		RouteContractDigest: digest(routeDocument),
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	installation := model.PluginInstallation{
		PluginID: packageID, Target: "control", DesiredVersion: version, ObservedVersion: version,
		State: "healthy", Enabled: true, LifecycleGeneration: 7,
	}
	require.NoError(t, db.Create(&installation).Error)
	return publicKey, installation
}

func prepareRouteModeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, MigrateKernelModelsForTest(db))
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.OperationLog{}))
}

// openRouteModePostgres opens ANIX_TEST_POSTGRES_DSN, or POSTGRES_TEST_DSN,
// on a throwaway schema; it refuses a database whose name lacks "test".
func openRouteModePostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	}
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN and POSTGRES_TEST_DSN are not set")
	}
	admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminDB.Close() })
	var databaseName string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	schema := "route_modes_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func openRouteModeSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestRouteModeDefaultsSQLite(t *testing.T) {
	runRouteModeDefaultsSuite(t, openRouteModeSQLite)
}

// TestPostgresRouteModeDefaults runs the route-mode suite on PostgreSQL;
// CI's PostgreSQL regression job selects it by its name.
func TestPostgresRouteModeDefaults(t *testing.T) {
	runRouteModeDefaultsSuite(t, openRouteModePostgres)
}

func runRouteModeDefaultsSuite(t *testing.T, open func(*testing.T) *gorm.DB) {
	t.Run("rehearsed release", func(t *testing.T) { testRouteModeDefaultsRehearsedRelease(t, open(t)) })
	t.Run("package too old", func(t *testing.T) { testRouteModeDefaultsTooOld(t, open(t)) })
	t.Run("identity group A follows the authority", func(t *testing.T) { testRouteModeDefaultsIdentity(t, open(t)) })
}

func seedKnowledge(t *testing.T, db *gorm.DB, version string) PackageHostOperations {
	t.Helper()
	prepareRouteModeDB(t, db)
	seedRouteModePackageVersion(t, db, "knowledge", version, knowledgeRoutes)
	return PackageHostOperations{DB: db}
}

func hostModes(t *testing.T, operations PackageHostOperations, version string) map[string]string {
	t.Helper()
	config, err := operations.PackageConfig(context.Background(), packagebridge.HostIdentity{PackageID: "knowledge", Version: version, Generation: 7})
	require.NoError(t, err)
	return config.RouteModes
}

// At the rehearsed release, and at any later one, the host is told to run
// exactly the listed routes natively; the others stay legacy.
func testRouteModeDefaultsRehearsedRelease(t *testing.T, db *gorm.DB) {
	operations := seedKnowledge(t, db, "4.1.0-rc.5")
	assert.Equal(t, nativeModes(knowledgeDefaultRoutes...), hostModes(t, operations, "4.1.0-rc.5"))

	// The final release clears the rehearsed rc.
	modes, err := ResolveEffectivePackageRouteModes(db, "knowledge", "4.1.0")
	require.NoError(t, err)
	assert.Equal(t, nativeModes(knowledgeDefaultRoutes...), modes)
}

// A release older than the rehearsed one stays legacy.
func testRouteModeDefaultsTooOld(t *testing.T, db *gorm.DB) {
	operations := seedKnowledge(t, db, "4.0.0")
	assert.Empty(t, hostModes(t, operations, "4.0.0"))
}

// Identity's routes are not in the list: identity group A follows the
// authority only, and the rest of identity-platform stays legacy.
func testRouteModeDefaultsIdentity(t *testing.T, db *gorm.DB) {
	prepareRouteModeDB(t, db)
	routes := []testCompatibilityRoute{{Method: "GET", LegacyPath: "/api/v2/admin/invite-codes", PackageRoute: "identity.admin.invite_codes.get", Transport: "http"}}
	for i, route := range IdentityGroupARoutes {
		routes = append(routes, testCompatibilityRoute{Method: "POST", LegacyPath: "/api/v2/group-a/" + string(rune('a'+i)), PackageRoute: route, Transport: "http"})
	}
	seedRouteModePackageVersion(t, db, IdentityPlatformPackageID, "4.1.0-rc.5", routes)
	host := packagebridge.HostIdentity{PackageID: IdentityPlatformPackageID, Version: "4.1.0-rc.5", Generation: 7}
	config, err := PackageHostOperations{DB: db}.PackageConfig(context.Background(), host)
	require.NoError(t, err)
	assert.Empty(t, config.RouteModes, "before the cutover every identity route is legacy")

	for _, state := range []string{model.IdentityAuthorityIdentity, model.IdentityAuthorityFinalized} {
		require.NoError(t, db.Save(&model.IdentityAuthority{ID: 1, State: state}).Error)
		config, err = PackageHostOperations{DB: db}.PackageConfig(context.Background(), host)
		require.NoError(t, err)
		assert.Len(t, config.RouteModes, len(IdentityGroupARoutes), state)
		for route, mode := range config.RouteModes {
			assert.True(t, slices.Contains(IdentityGroupARoutes, route), route)
			assert.Equal(t, packagebridge.RouteModeNative, mode, route)
		}
	}
	require.NoError(t, db.Save(&model.IdentityAuthority{ID: 1, State: model.IdentityAuthorityImporting}).Error)
	config, err = PackageHostOperations{DB: db}.PackageConfig(context.Background(), host)
	require.NoError(t, err)
	assert.Empty(t, config.RouteModes, "a rollback returns group A to legacy")
}

func TestPackageRouteCatalogParsesTheEmbeddedMap(t *testing.T) {
	catalog, err := PackageRouteCatalog()
	require.NoError(t, err)
	assert.Equal(t, RouteCatalogNativeFlagged, catalog["knowledge.article.list"].Mode)
	assert.Equal(t, "knowledge", catalog["knowledge.article.list"].PackageID)
	assert.Equal(t, RouteCatalogKernelOwned, catalog["platform.admin.system.configs.get"].Mode)
	_, err = parsePackageRouteCatalog([]byte(`{`))
	require.Error(t, err)
}
