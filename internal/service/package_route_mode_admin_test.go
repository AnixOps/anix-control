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
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// routeModeAdminRoutes are the compatibility routes of the test package
// "knowledge": native-flagged GET and POST routes, a bridged and a
// kernel-owned route, and a WebSocket route the catalog does not list.
var routeModeAdminRoutes = []compatibilityRoute{
	{Method: "GET", LegacyPath: "/api/v2/user/knowledge", PackageRoute: "knowledge.article.list", Transport: "http"},
	{Method: "POST", LegacyPath: "/api/v2/admin/knowledge", PackageRoute: "knowledge.admin.knowledge.post", Transport: "http"},
	{Method: "GET", LegacyPath: "/api/v2/admin/knowledge/bridged", PackageRoute: "knowledge.bridged.get", Transport: "http"},
	{Method: "GET", LegacyPath: "/api/v2/admin/knowledge/kernel", PackageRoute: "knowledge.kernel.get", Transport: "http"},
	{Method: "GET", LegacyPath: "/api/v2/ws", PackageRoute: "knowledge.ws", Transport: "websocket"},
}

var routeModeAdminCatalog = map[string]RouteCatalogEntry{
	"knowledge.article.list":         {PackageID: "knowledge", RouteID: "knowledge.article.list", Mode: RouteCatalogNativeFlagged},
	"knowledge.admin.knowledge.post": {PackageID: "knowledge", RouteID: "knowledge.admin.knowledge.post", Mode: RouteCatalogNativeFlagged},
	"knowledge.bridged.get":          {PackageID: "knowledge", RouteID: "knowledge.bridged.get", Mode: RouteCatalogBridged},
	"knowledge.kernel.get":           {PackageID: "knowledge", RouteID: "knowledge.kernel.get", Mode: RouteCatalogKernelOwned},
}

// seedRouteModePackage registers, stores and installs a signed v2 Control
// release 4.0.1 of packageID that declares routes. 4.0.1 is older than any
// rehearsed release, so no route of it defaults to native.
func seedRouteModePackage(t *testing.T, db *gorm.DB, packageID string, routes []compatibilityRoute) (ed25519.PublicKey, model.PluginInstallation) {
	t.Helper()
	return seedRouteModePackageVersion(t, db, packageID, "4.0.1", routes)
}

// seedRouteModePackageVersion is seedRouteModePackage at version.
func seedRouteModePackageVersion(t *testing.T, db *gorm.DB, packageID, version string, routes []compatibilityRoute) (ed25519.PublicKey, model.PluginInstallation) {
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
	require.NoError(t, EnsureKernelSchema(db))
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

func TestRouteModeAdminSQLite(t *testing.T) {
	runRouteModeAdminSuite(t, openRouteModeSQLite)
}

// TestPostgresRouteModeAdmin runs the suite on PostgreSQL; CI's PostgreSQL
// regression job selects it by its name.
func TestPostgresRouteModeAdmin(t *testing.T) {
	runRouteModeAdminSuite(t, openRouteModePostgres)
}

func runRouteModeAdminSuite(t *testing.T, open func(*testing.T) *gorm.DB) {
	t.Run("list", func(t *testing.T) { testRouteModeAdminList(t, open(t)) })
	t.Run("set and rollback", func(t *testing.T) { testRouteModeAdminSetAndRollback(t, open(t)) })
	t.Run("rules", func(t *testing.T) { testRouteModeAdminRules(t, open(t)) })
	t.Run("identity group A", func(t *testing.T) { testRouteModeAdminIdentityGroupA(t, open(t)) })
	t.Run("super admin", func(t *testing.T) { testRouteModeSuperAdmin(t, open(t)) })
}

var cliActor = RouteModeActor{Name: RouteModeCLIActor}

func routeModeEntry(t *testing.T, packages []PackageRouteModes, route string) RouteModeEntry {
	t.Helper()
	for _, pkg := range packages {
		for _, entry := range pkg.Routes {
			if entry.RouteID == route {
				return entry
			}
		}
	}
	t.Fatalf("route %s is not listed", route)
	return RouteModeEntry{}
}

func testRouteModeAdminList(t *testing.T, db *gorm.DB) {
	prepareRouteModeDB(t, db)
	publicKey, _ := seedRouteModePackage(t, db, "knowledge", routeModeAdminRoutes)
	admin := &RouteModeAdmin{DB: db, PublicKey: publicKey, Catalog: routeModeAdminCatalog}
	ctx := context.Background()

	_, err := admin.List(ctx, "ticket", nil)
	require.ErrorIs(t, err, ErrRouteModePackageNotInstalled)

	hosts := map[string]map[string]RouteHostObservation{"knowledge": {
		"knowledge.article.list": {Mode: "shadow", Effective: "shadow", ShadowTotal: 9, ShadowMismatch: 2},
	}}
	packages, err := admin.List(ctx, "", hosts)
	require.NoError(t, err)
	require.Len(t, packages, 1)
	assert.Equal(t, "knowledge", packages[0].PackageID)
	assert.Empty(t, packages[0].Error)
	require.Len(t, packages[0].Routes, len(routeModeAdminRoutes))

	list := routeModeEntry(t, packages, "knowledge.article.list")
	assert.Equal(t, []string{"legacy", "shadow", "native"}, list.AllowedModes)
	assert.Equal(t, "legacy", list.Configured)
	assert.Equal(t, "legacy", list.Effective)
	require.NotNil(t, list.Host)
	assert.Equal(t, uint64(2), list.Host.ShadowMismatch)
	assert.Equal(t, []string{"legacy", "native"}, routeModeEntry(t, packages, "knowledge.admin.knowledge.post").AllowedModes)
	assert.Equal(t, []string{"legacy"}, routeModeEntry(t, packages, "knowledge.bridged.get").AllowedModes)
	kernel := routeModeEntry(t, packages, "knowledge.kernel.get")
	assert.Empty(t, kernel.AllowedModes)
	assert.Equal(t, RouteModeLockKernelOwned, kernel.Locked)
	assert.Equal(t, RouteModeLockWebSocket, routeModeEntry(t, packages, "knowledge.ws").Locked)
}

func testRouteModeAdminSetAndRollback(t *testing.T, db *gorm.DB) {
	prepareRouteModeDB(t, db)
	publicKey, installation := seedRouteModePackage(t, db, "knowledge", routeModeAdminRoutes)
	admin := &RouteModeAdmin{DB: db, PublicKey: publicKey, Catalog: routeModeAdminCatalog}
	ctx := context.Background()
	operator := RouteModeActor{UserID: 9, Name: "root@example.com", IP: "192.0.2.1"}

	// Native needs the confirmation and a reason.
	for _, request := range []RouteModeChangeRequest{
		{PackageID: "knowledge", Mode: "native"},
		{PackageID: "knowledge", Mode: "native", Confirm: true},
		{PackageID: "knowledge", Mode: "native", Reason: "batch 1"},
		{PackageID: "knowledge", Mode: "native", Confirm: true, Reason: "   "},
	} {
		_, err := admin.Set(ctx, request, operator)
		require.ErrorIs(t, err, ErrRouteModeConfirmationRequired)
	}

	shadow, err := admin.Set(ctx, RouteModeChangeRequest{PackageID: "knowledge", Routes: []string{"knowledge.article.list"}, Mode: "shadow"}, operator)
	require.NoError(t, err)
	require.NotEmpty(t, shadow.GroupID)
	assert.Equal(t, []RouteModeChange{{RouteID: "knowledge.article.list", From: "legacy", To: "shadow"}}, shadow.Changes)
	assert.Equal(t, int64(1), shadow.ConfigRevision)

	// Repeating it changes nothing and records nothing.
	again, err := admin.Set(ctx, RouteModeChangeRequest{PackageID: "knowledge", Routes: []string{"knowledge.article.list"}, Mode: "shadow"}, operator)
	require.NoError(t, err)
	assert.Empty(t, again.GroupID)
	assert.Empty(t, again.Changes)

	// The whole package to native: the native-flagged routes switch, the
	// others are reported.
	native, err := admin.Set(ctx, RouteModeChangeRequest{PackageID: "knowledge", Mode: "native", Confirm: true, Reason: "batch 1 passed shadow"}, operator)
	require.NoError(t, err)
	assert.ElementsMatch(t, []RouteModeChange{
		{RouteID: "knowledge.admin.knowledge.post", From: "legacy", To: "native"},
		{RouteID: "knowledge.article.list", From: "shadow", To: "native"},
	}, native.Changes)
	skipped := map[string]bool{}
	for _, skip := range native.Skipped {
		skipped[skip.RouteID] = true
	}
	assert.Equal(t, map[string]bool{"knowledge.bridged.get": true, "knowledge.kernel.get": true, "knowledge.ws": true}, skipped)

	// The host picks the change up through its configuration RPC.
	operations := PackageHostOperations{DB: db}
	hostConfig, err := operations.PackageConfig(ctx, packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.0.1", Generation: 7})
	require.NoError(t, err)
	assert.Equal(t, native.ConfigRevision, hostConfig.Revision)
	assert.Equal(t, map[string]string{"knowledge.article.list": "native", "knowledge.admin.knowledge.post": "native"}, hostConfig.RouteModes)

	rollback, err := admin.Rollback(ctx, "knowledge", "", cliActor)
	require.NoError(t, err)
	assert.Equal(t, model.RouteModeRevisionActionRollback, rollback.Action)
	assert.Len(t, rollback.Changes, 2)
	hostConfig, err = operations.PackageConfig(ctx, packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.0.1", Generation: 7})
	require.NoError(t, err)
	assert.Empty(t, hostConfig.RouteModes)

	revisions, err := admin.Revisions(ctx, "knowledge", 0)
	require.NoError(t, err)
	require.Len(t, revisions, 5)
	assert.Equal(t, model.RouteModeRevisionActionRollback, revisions[0].Action)
	assert.Equal(t, RouteModeCLIActor, revisions[0].Actor)
	assert.Equal(t, uint(0), revisions[0].ActorUserID)
	assert.Equal(t, "native", revisions[0].FromMode)
	assert.Equal(t, "legacy", revisions[0].ToMode)
	last := revisions[len(revisions)-1]
	assert.Equal(t, "knowledge.article.list", last.RouteID)
	assert.Equal(t, uint(9), last.ActorUserID)
	assert.Equal(t, int64(1), last.ConfigRevision)
	assert.Equal(t, "batch 1 passed shadow", revisions[2].Reason)
	other, err := admin.Revisions(ctx, "ticket", 0)
	require.NoError(t, err)
	assert.Empty(t, other)

	var audits []model.OperationLog
	require.NoError(t, db.Order("id").Find(&audits).Error)
	require.Len(t, audits, 3, "one audit entry per committed switch")
	assert.Equal(t, "route_mode_set", audits[0].Action)
	assert.Equal(t, "kernel", audits[0].Module)
	require.NotNil(t, audits[0].UserID)
	assert.Equal(t, uint(9), *audits[0].UserID)
	assert.Equal(t, "root@example.com", audits[0].Username)
	require.NotNil(t, audits[0].TargetID)
	assert.Equal(t, installation.ID, *audits[0].TargetID)
	assert.Contains(t, audits[1].Content, "batch 1 passed shadow")
	assert.Equal(t, "route_mode_rollback", audits[2].Action)
	assert.Nil(t, audits[2].UserID)
	assert.Equal(t, RouteModeCLIActor, audits[2].Username)

	// A rollback with nothing to roll back changes nothing.
	idle, err := admin.Rollback(ctx, "knowledge", "", cliActor)
	require.NoError(t, err)
	assert.Empty(t, idle.Changes)
}

func testRouteModeAdminRules(t *testing.T, db *gorm.DB) {
	prepareRouteModeDB(t, db)
	publicKey, _ := seedRouteModePackage(t, db, "knowledge", routeModeAdminRoutes)
	admin := &RouteModeAdmin{DB: db, PublicKey: publicKey, Catalog: routeModeAdminCatalog}
	ctx := context.Background()
	native := func(routes ...string) RouteModeChangeRequest {
		return RouteModeChangeRequest{PackageID: "knowledge", Routes: routes, Mode: "native", Confirm: true, Reason: "test"}
	}
	rejected := map[string]RouteModeChangeRequest{
		"not native-flagged":        native("knowledge.bridged.get"),
		"kernel-owned":              {PackageID: "knowledge", Routes: []string{"knowledge.kernel.get"}, Mode: "legacy"},
		"websocket":                 native("knowledge.ws"),
		"shadow on POST":            {PackageID: "knowledge", Routes: []string{"knowledge.admin.knowledge.post"}, Mode: "shadow"},
		"foreign route":             native("ticket.list"),
		"unknown mode":              {PackageID: "knowledge", Mode: "canary"},
		"one bad route rejects all": native("knowledge.article.list", "knowledge.bridged.get"),
		"no route named":            {PackageID: "knowledge", Routes: []string{" "}, Mode: "legacy"},
		"reason too long":           {PackageID: "knowledge", Mode: "legacy", Reason: strings.Repeat("x", maxRouteModeReason+1)},
	}
	for name, request := range rejected {
		_, err := admin.Set(ctx, request, cliActor)
		require.ErrorIs(t, err, ErrRouteModeRejected, name)
	}
	_, err := admin.Set(ctx, native("knowledge.article.list"), cliActor)
	require.NoError(t, err)
	_, err = admin.Set(ctx, RouteModeChangeRequest{PackageID: "ticket", Mode: "legacy"}, cliActor)
	require.ErrorIs(t, err, ErrRouteModePackageNotInstalled)

	var count int64
	require.NoError(t, db.Model(&model.RouteModeRevision{}).Count(&count).Error)
	assert.Equal(t, int64(1), count, "rejected changes record nothing")
	require.NoError(t, db.Model(&model.OperationLog{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	// Without the trust root nothing is written.
	_, err = (&RouteModeAdmin{DB: db, Catalog: routeModeAdminCatalog}).Rollback(ctx, "knowledge", "", cliActor)
	require.ErrorIs(t, err, ErrPluginTrustRootRequired)
}

func testRouteModeAdminIdentityGroupA(t *testing.T, db *gorm.DB) {
	prepareRouteModeDB(t, db)
	routes := []compatibilityRoute{
		{Method: "GET", LegacyPath: "/api/v2/user/info", PackageRoute: IdentityAccountReadRoutes[0], Transport: "http"},
		{Method: "GET", LegacyPath: "/api/v2/admin/invite-codes", PackageRoute: "identity.admin.invite_codes.get", Transport: "http"},
	}
	catalog := map[string]RouteCatalogEntry{
		IdentityAccountReadRoutes[0]:      {PackageID: IdentityPlatformPackageID, Mode: RouteCatalogNativeFlagged},
		"identity.admin.invite_codes.get": {PackageID: IdentityPlatformPackageID, Mode: RouteCatalogNativeFlagged},
	}
	for i, route := range IdentityGroupARoutes {
		method := "POST"
		if strings.HasSuffix(route, ".get") {
			method = "GET"
		}
		routes = append(routes, compatibilityRoute{Method: method, LegacyPath: "/api/v2/group-a/" + string(rune('a'+i)), PackageRoute: route, Transport: "http"})
		catalog[route] = RouteCatalogEntry{PackageID: IdentityPlatformPackageID, Mode: RouteCatalogNativeFlagged}
	}
	publicKey, _ := seedRouteModePackage(t, db, IdentityPlatformPackageID, routes)
	admin := &RouteModeAdmin{DB: db, PublicKey: publicKey, Catalog: catalog}
	ctx := context.Background()

	// Group A switches only with the identity cutover, partly or whole.
	for _, named := range [][]string{IdentityGroupARoutes[:1], IdentityGroupARoutes} {
		_, err := admin.Set(ctx, RouteModeChangeRequest{PackageID: IdentityPlatformPackageID, Routes: named, Mode: "native", Confirm: true, Reason: "x"}, cliActor)
		require.ErrorIs(t, err, ErrRouteModeRejected)
		assert.Contains(t, err.Error(), "identity cutover")
	}
	// The account reads leave legacy only once identity is authoritative.
	_, err := admin.Set(ctx, RouteModeChangeRequest{PackageID: IdentityPlatformPackageID, Routes: IdentityAccountReadRoutes[:1], Mode: "shadow"}, cliActor)
	require.ErrorIs(t, err, ErrRouteModeRejected)

	// The whole package skips group A and the account read.
	result, err := admin.Set(ctx, RouteModeChangeRequest{PackageID: IdentityPlatformPackageID, Mode: "native", Confirm: true, Reason: "x"}, cliActor)
	require.NoError(t, err)
	assert.Equal(t, []RouteModeChange{{RouteID: "identity.admin.invite_codes.get", From: "legacy", To: "native"}}, result.Changes)
	assert.Len(t, result.Skipped, len(IdentityGroupARoutes)+1)

	packages, err := admin.List(ctx, IdentityPlatformPackageID, nil)
	require.NoError(t, err)
	groupA := routeModeEntry(t, packages, IdentityGroupARoutes[0])
	assert.Equal(t, RouteModeLockIdentityGroupA, groupA.Locked)
	assert.Empty(t, groupA.AllowedModes)

	rollback, err := admin.Rollback(ctx, IdentityPlatformPackageID, "", cliActor)
	require.NoError(t, err)
	assert.Equal(t, []RouteModeChange{{RouteID: "identity.admin.invite_codes.get", From: "native", To: "legacy"}}, rollback.Changes)
}

func testRouteModeSuperAdmin(t *testing.T, db *gorm.DB) {
	prepareRouteModeDB(t, db)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "root@x", Token: "t1", UUID: "u1", IsAdmin: 1},
		{ID: 2, Email: "staff@x", Token: "t2", UUID: "u2", IsAdmin: 1, IsStaff: 1},
		{ID: 3, Email: "banned@x", Token: "t3", UUID: "u3", IsAdmin: 1, Banned: 1},
		{ID: 4, Email: "user@x", Token: "t4", UUID: "u4"},
	}).Error)
	for id, want := range map[uint]bool{0: false, 1: true, 2: false, 3: false, 4: false, 5: false} {
		got, err := IsSuperAdmin(db, id)
		require.NoError(t, err)
		assert.Equal(t, want, got, id)
	}
	assert.Equal(t, "root@x", RouteModeActorName(db, 1))
	assert.Equal(t, "user:5", RouteModeActorName(db, 5))
}

func TestPackageRouteCatalogParsesTheEmbeddedMap(t *testing.T) {
	catalog, err := PackageRouteCatalog()
	require.NoError(t, err)
	assert.Equal(t, RouteCatalogNativeFlagged, catalog["knowledge.article.list"].Mode)
	assert.Equal(t, "knowledge", catalog["knowledge.article.list"].PackageID)
	assert.Equal(t, RouteCatalogKernelOwned, catalog["platform.admin.system.backup.post"].Mode)
	_, err = parsePackageRouteCatalog([]byte(`{`))
	require.Error(t, err)
}
