package identitycutover

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/identity/account"
	"github.com/AnixOps/anix-control/identity/secretbox"
	"github.com/AnixOps/anix-control/identity/server"
	identityv1 "github.com/AnixOps/anix-control/sdk/api/identity/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	compatv2 "github.com/AnixOps/anix-control/v4/internal/compat/v2"
	"github.com/AnixOps/anix-control/v4/internal/identityimport"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	return db
}

// fakeHosts reports the identity-platform host's route modes: the ones the
// kernel hands out (service.PackageHostOperations), unless stuck.
type fakeHosts struct {
	db    *gorm.DB
	mu    sync.Mutex
	stuck bool
}

func (h *fakeHosts) Health(ctx context.Context, packageID, version string, generation uint64) (pluginhost.HostHealth, error) {
	h.mu.Lock()
	stuck := h.stuck
	h.mu.Unlock()
	config := packagebridge.PackageConfig{}
	if !stuck {
		var err error
		config, err = service.PackageHostOperations{DB: h.db}.PackageConfig(ctx,
			packagebridge.HostIdentity{PackageID: packageID, Version: version, Generation: generation})
		if err != nil {
			return pluginhost.HostHealth{}, err
		}
	}
	routes := map[string]map[string]string{}
	for route, mode := range config.RouteModes {
		routes[route] = map[string]string{"mode": mode, "effective": mode}
	}
	details, err := json.Marshal(map[string]any{"config": map[string]any{"revision": config.Revision}, "routes": routes})
	return pluginhost.HostHealth{Healthy: true, DetailsJSON: string(details)}, err
}

type fixture struct {
	kernel    *gorm.DB
	accounts  *account.Store
	hosts     *fakeHosts
	freeze    *compatv2.RouteFreeze
	publicKey ed25519.PublicKey
	service   *Service
	now       time.Time
	finalized int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	kernel := openSQLite(t)
	require.NoError(t, service.EnsureKernelSchema(kernel))
	require.NoError(t, kernel.AutoMigrate(&model.User{}, &model.UserMFA{}))
	publicKey := seedIdentityPlatformRelease(t, kernel)

	identityDB := openSQLite(t)
	schema, err := os.ReadFile("../../packages/identity-platform/migrations/003_accounts.sql")
	require.NoError(t, err)
	prefixed := &packagestoresdk.Store{Lease: packagebridgesdk.StorageLease{TablePrefix: "t_"}}
	require.NoError(t, identityDB.Exec(prefixed.ExpandScript(string(schema))).Error)
	box, err := secretbox.New([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	accounts := &account.Store{DB: identityDB, Secrets: box, Tables: account.Tables{
		Account: "t_account", MFA: "t_mfa", ImportRun: "t_import_run",
	}}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	identityv1.RegisterIdentityServiceServer(grpcServer, &server.Server{Accounts: accounts})
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := grpc.NewClient("passthrough:///identity", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	f := &fixture{kernel: kernel, accounts: accounts, hosts: &fakeHosts{db: kernel}, freeze: &compatv2.RouteFreeze{}, publicKey: publicKey, now: time.Now()}
	f.service = &Service{
		DB:          kernel,
		Importer:    &identityimport.Importer{DB: kernel, Connect: func() (grpc.ClientConnInterface, error) { return conn, nil }},
		PublicKey:   func() (ed25519.PublicKey, error) { return publicKey, nil },
		RefreshKeys: func(context.Context) error { return nil },
		Hosts:       f.hosts, Freeze: f.freeze, Settle: 50 * time.Millisecond, Hold: time.Millisecond,
		OnFinalized: func() { f.finalized++ },
		Now:         func() time.Time { return f.now },
	}
	require.NoError(t, kernel.Create(&[]model.User{
		{ID: 1, Email: "admin@example.test", Password: "$2a$10$admin", UUID: "u1", Token: "t1", IsAdmin: 1},
		{ID: 2, Email: "member@example.test", Password: "$2a$10$member", UUID: "u2", Token: "t2"},
	}).Error)
	require.NoError(t, kernel.Create(&model.UserMFA{UserID: 2, Enabled: true, TOTPSecret: "JBSWY3DPEHPK3PXP"}).Error)
	return f
}

// seedIdentityPlatformRelease registers a signed identity-platform release
// with the package's real compatibility routes and installs it.
func seedIdentityPlatformRelease(t *testing.T, db *gorm.DB) ed25519.PublicKey {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	routes, err := os.ReadFile("../../packages/identity-platform/compat/v2-routes.json")
	require.NoError(t, err)
	entrypoint := []byte("#!/bin/sh\nexit 0\n")
	migrations := []byte(`{"format":"anixops.migrations/v1","migrations":[],"package_id":"identity-platform","version":"4.1.0"}`)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range []struct {
		name string
		data []byte
	}{{"bin/control-host", entrypoint}, {"compat/v2-routes.json", routes}, {"migrations/index.json", migrations}} {
		entry, err := writer.Create(file.name)
		require.NoError(t, err)
		_, err = entry.Write(file.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	artifact := buffer.Bytes()
	digest := func(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
	manifest := service.PluginManifest{
		ID: service.IdentityPlatformPackageID, Name: "Identity platform", Version: "4.1.0", APIVersion: "v2", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: digest(artifact),
		ControlEntrypoint:   &service.PluginEntrypoint{Path: "bin/control-host", SHA256: digest(entrypoint)},
		Migrations:          &service.PluginMigrations{Index: "migrations/index.json", SHA256: digest(migrations)},
		CompatibilityRoutes: &service.PluginCompatibilityRoutes{Path: "compat/v2-routes.json", SHA256: digest(routes)},
		RouteContractDigest: digest(routes),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: service.IdentityPlatformPackageID, Target: "control", DesiredVersion: "4.1.0", ObservedVersion: "4.1.0",
		State: "healthy", Enabled: true, LifecycleGeneration: 3,
	}).Error)
	return publicKey
}

func (f *fixture) state(t *testing.T) string {
	t.Helper()
	state, err := service.IdentityAuthorityState(f.kernel)
	require.NoError(t, err)
	return state
}

// groupAModes returns the configured mode of every group A route.
func (f *fixture) groupAModes(t *testing.T) map[string]int {
	t.Helper()
	config, err := service.PackageHostOperations{DB: f.kernel}.PackageConfig(context.Background(),
		packagebridge.HostIdentity{PackageID: service.IdentityPlatformPackageID, Version: "4.1.0", Generation: 3})
	require.NoError(t, err)
	counts := map[string]int{}
	for _, route := range service.IdentityGroupARoutes {
		mode := config.RouteModes[route]
		if mode == "" {
			mode = packagebridge.RouteModeLegacy
		}
		counts[mode]++
	}
	return counts
}

func (f *fixture) events(t *testing.T) []string {
	t.Helper()
	var actions []string
	require.NoError(t, f.kernel.Model(&model.IdentityCutoverEvent{}).Order("id").Pluck("action", &actions).Error)
	return actions
}

func allNative(t *testing.T) string {
	t.Helper()
	modes := map[string]string{}
	for _, route := range service.IdentityGroupARoutes {
		modes[route] = packagebridge.RouteModeNative
	}
	document, err := json.Marshal(map[string]any{"routes": modes})
	require.NoError(t, err)
	return string(document)
}

func (f *fixture) setModes(document string) error {
	var installation model.PluginInstallation
	if err := f.kernel.Take(&installation, "plugin_id = ?", service.IdentityPlatformPackageID).Error; err != nil {
		return err
	}
	_, err := service.UpdatePluginConfigurationWithValidatorAndHook(f.kernel, f.publicKey, installation.ID, document, nil, 1, nil, nil)
	return err
}

func TestCutoverNeedsACompletedFullImport(t *testing.T) {
	f := newFixture(t)
	_, err := f.service.Cutover(context.Background(), 1)
	require.ErrorIs(t, err, ErrNotReady)
	require.Equal(t, model.IdentityAuthorityKernel, f.state(t))
}

func TestCutoverHandsGroupAToIdentityWithTheLatestLegacyChanges(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	require.NoError(t, f.kernel.Model(&model.User{}).Where("1 = 1").Update("updated_at", f.now.Add(-2*time.Hour)).Error)
	require.NoError(t, f.kernel.Model(&model.UserMFA{}).Where("1 = 1").Update("updated_at", f.now.Add(-2*time.Hour)).Error)
	f.service.Importer.Now = func() time.Time { return f.now.Add(-time.Hour) }
	_, err := f.service.Importer.Run(ctx, false)
	require.NoError(t, err)
	f.service.Importer.Now = nil

	// Changed in legacy after the full import: a new user and a ban.
	require.NoError(t, f.kernel.Create(&model.User{ID: 3, Email: "late@example.test", Password: "$2a$10$late", UUID: "u3", Token: "t3"}).Error)
	require.NoError(t, f.kernel.Model(&model.User{}).Where("id = ?", 2).Updates(map[string]any{"banned": 1, "updated_at": time.Now()}).Error)

	report, err := f.service.Cutover(ctx, 1)
	require.NoError(t, err)
	// The catch-up delta carries both changes; the final one may repeat
	// them, as deltas overlap by the second their predecessor started.
	require.GreaterOrEqual(t, report.Imported, uint64(2))
	require.LessOrEqual(t, report.Imported, uint64(4))
	require.Equal(t, model.IdentityAuthorityIdentity, f.state(t))
	require.Equal(t, map[string]int{packagebridge.RouteModeNative: len(service.IdentityGroupARoutes)}, f.groupAModes(t))
	require.Equal(t, []string{model.IdentityCutoverActionCutover}, f.events(t))
	require.False(t, f.freeze.Frozen(service.IdentityPlatformPackageID, "identity.auth.login"), "group A is served again")

	accounts, err := f.accounts.Get(ctx, []uint64{2, 3})
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.True(t, accounts[0].Banned)

	// Group A cannot be switched back behind the authority's back.
	require.ErrorContains(t, f.setModes(`{"routes":{}}`), "stays native")
	require.ErrorContains(t, f.setModes(`{"routes":{"identity.auth.login":"native"}}`), "switch together")
	_, err = f.service.Importer.Run(ctx, true)
	require.ErrorIs(t, err, identityimport.ErrNotImportable)
	_, err = f.service.Cutover(ctx, 1)
	require.ErrorIs(t, err, ErrNotReady)
}

func TestCutoverIsUndoneWhenTheHostsDoNotServeNatively(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, err := f.service.Importer.Run(ctx, false)
	require.NoError(t, err)
	f.hosts.stuck = true

	_, err = f.service.Cutover(ctx, 1)
	require.ErrorContains(t, err, "did not serve group A natively")
	require.Equal(t, model.IdentityAuthorityImporting, f.state(t))
	require.Equal(t, map[string]int{packagebridge.RouteModeLegacy: len(service.IdentityGroupARoutes)}, f.groupAModes(t))
	require.Equal(t, []string{model.IdentityCutoverActionCutover, model.IdentityCutoverActionAborted}, f.events(t))
	require.False(t, f.freeze.Frozen(service.IdentityPlatformPackageID, "identity.auth.login"))

	f.hosts.stuck = false
	_, err = f.service.Cutover(ctx, 1)
	require.NoError(t, err, "a later cutover succeeds")
}

func TestRollbackReturnsGroupAToLegacyUntilFinalize(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, err := f.service.Importer.Run(ctx, false)
	require.NoError(t, err)
	_, err = f.service.Cutover(ctx, 1)
	require.NoError(t, err)

	report, err := f.service.Rollback(ctx, 1)
	require.NoError(t, err)
	require.Empty(t, report.Warning)
	require.Equal(t, model.IdentityAuthorityImporting, f.state(t))
	require.Equal(t, map[string]int{packagebridge.RouteModeLegacy: len(service.IdentityGroupARoutes)}, f.groupAModes(t))
	_, err = f.service.Rollback(ctx, 1)
	require.ErrorIs(t, err, ErrNotReady)
	require.ErrorContains(t, f.setModes(allNative(t)), "only once identity is authoritative")

	_, err = f.service.Cutover(ctx, 1)
	require.NoError(t, err, "cutting over again re-imports what changed meanwhile")
	require.Equal(t, []string{"cutover", "rollback", "cutover"}, f.events(t))
}

func TestFinalizeRemovesLegacyCredentialsADayAfterTheCutover(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	require.ErrorIs(t, f.service.Finalize(ctx, 1, true), ErrNotReady)
	_, err := f.service.Importer.Run(ctx, false)
	require.NoError(t, err)
	_, err = f.service.Cutover(ctx, 1)
	require.NoError(t, err)

	f.now = f.now.Add(time.Hour)
	require.ErrorIs(t, f.service.Finalize(ctx, 1, false), ErrTooEarly)
	f.now = f.now.Add(24 * time.Hour)
	require.NoError(t, f.service.Finalize(ctx, 1, false))
	require.Equal(t, model.IdentityAuthorityFinalized, f.state(t))
	require.Equal(t, 1, f.finalized)

	var passwords []string
	require.NoError(t, f.kernel.Model(&model.User{}).Distinct().Pluck("password", &passwords).Error)
	require.Equal(t, []string{model.UnusableLegacyPassword}, passwords)
	var mfas int64
	require.NoError(t, f.kernel.Model(&model.UserMFA{}).Count(&mfas).Error)
	require.Zero(t, mfas)

	_, err = f.service.Rollback(ctx, 1)
	require.ErrorIs(t, err, ErrFinal)
	require.ErrorIs(t, f.service.Finalize(ctx, 1, true), ErrFinal)
	require.ErrorContains(t, f.setModes(`{"routes":{}}`), "stays native")
}

func TestOneAuthorityChangeAtATime(t *testing.T) {
	f := newFixture(t)
	f.service.running.Lock()
	defer f.service.running.Unlock()
	_, err := f.service.Cutover(context.Background(), 1)
	require.ErrorIs(t, err, ErrBusy)
	_, err = f.service.Rollback(context.Background(), 1)
	require.ErrorIs(t, err, ErrBusy)
	require.ErrorIs(t, f.service.Finalize(context.Background(), 1, true), ErrBusy)
}

func TestStartRunsTheChangeInTheBackground(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, err := f.service.Importer.Run(ctx, false)
	require.NoError(t, err)
	require.Nil(t, f.service.Latest())
	require.Error(t, f.service.Start(ctx, "finalize", 1), "finalize runs in the foreground")

	require.NoError(t, f.service.Start(ctx, model.IdentityCutoverActionCutover, 1))
	require.Eventually(t, func() bool { return !f.service.Latest().Running }, 5*time.Second, 5*time.Millisecond)
	latest := f.service.Latest()
	require.Empty(t, latest.Error)
	require.Equal(t, model.IdentityCutoverActionCutover, latest.Report.Action)
	require.Equal(t, model.IdentityAuthorityIdentity, f.state(t))

	require.NoError(t, f.service.Start(ctx, model.IdentityCutoverActionCutover, 1))
	require.Eventually(t, func() bool { return !f.service.Latest().Running }, 5*time.Second, 5*time.Millisecond)
	require.Contains(t, f.service.Latest().Error, "already authoritative")
}

// routesWith is a route configuration: group A in groupA's mode (legacy
// leaves it out) plus extra.
func routesWith(t *testing.T, groupA string, extra map[string]string) string {
	t.Helper()
	modes := map[string]string{}
	if groupA != packagebridge.RouteModeLegacy {
		for _, route := range service.IdentityGroupARoutes {
			modes[route] = groupA
		}
	}
	for route, mode := range extra {
		modes[route] = mode
	}
	document, err := json.Marshal(map[string]any{"routes": modes})
	require.NoError(t, err)
	return string(document)
}

// The account reads leave legacy only while identity is authoritative, on
// their own; the rollback returns them to legacy. The resets, which touch
// only the subscriber, switch at any time.
func TestAccountReadsFollowTheAuthority(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	profile, detail := "identity.user.profile.get", "identity.admin.users.id.get"
	list, stats := "identity.admin.users.get", "identity.admin.users.stats.get"
	require.Subset(t, service.IdentityAccountReadRoutes, []string{profile, detail, list, stats})
	resets := map[string]string{
		"identity.admin.users.id.reset_traffic.post":   packagebridge.RouteModeNative,
		"identity.admin.users.id.reset_subscribe.post": packagebridge.RouteModeNative,
	}
	require.NoError(t, f.setModes(routesWith(t, packagebridge.RouteModeLegacy, resets)), "resets switch before any cutover")
	for _, route := range []string{profile, list, stats} {
		for _, mode := range []string{packagebridge.RouteModeNative, packagebridge.RouteModeShadow} {
			require.ErrorContains(t, f.setModes(routesWith(t, packagebridge.RouteModeLegacy, map[string]string{route: mode})),
				"leaves legacy mode only once identity is authoritative", route)
		}
	}
	_, err := f.service.Importer.Run(ctx, false)
	require.NoError(t, err)
	for _, route := range []string{detail, list, stats} {
		require.ErrorContains(t, f.setModes(routesWith(t, packagebridge.RouteModeLegacy, map[string]string{route: packagebridge.RouteModeNative})),
			"leaves legacy mode only once identity is authoritative", "an import is not authority")
	}

	_, err = f.service.Cutover(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, map[string]int{packagebridge.RouteModeLegacy: len(service.IdentityAccountReadRoutes)}, f.modesOf(t, service.IdentityAccountReadRoutes),
		"the cutover leaves the reads to the operator")
	reads := map[string]string{
		profile: packagebridge.RouteModeNative, detail: packagebridge.RouteModeShadow,
		list: packagebridge.RouteModeNative, stats: packagebridge.RouteModeShadow,
	}
	for route, mode := range resets {
		reads[route] = mode
	}
	require.NoError(t, f.setModes(routesWith(t, packagebridge.RouteModeNative, reads)))
	require.ErrorContains(t, f.setModes(routesWith(t, packagebridge.RouteModeNative, map[string]string{profile: packagebridge.RouteModeNative, "identity.auth.login": packagebridge.RouteModeLegacy})),
		"switch together", "the reads do not count as group A")

	_, err = f.service.Rollback(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, map[string]int{packagebridge.RouteModeLegacy: len(service.IdentityAccountReadRoutes)}, f.modesOf(t, service.IdentityAccountReadRoutes))
	require.Equal(t, map[string]int{packagebridge.RouteModeLegacy: len(service.IdentityGroupARoutes)}, f.groupAModes(t))
	require.Equal(t, map[string]int{packagebridge.RouteModeNative: len(resets)}, f.modesOf(t, []string{
		"identity.admin.users.id.reset_traffic.post", "identity.admin.users.id.reset_subscribe.post",
	}), "the rollback leaves the resets alone")
}

// modesOf counts the configured modes of routes.
func (f *fixture) modesOf(t *testing.T, routes []string) map[string]int {
	t.Helper()
	config, err := service.PackageHostOperations{DB: f.kernel}.PackageConfig(context.Background(),
		packagebridge.HostIdentity{PackageID: service.IdentityPlatformPackageID, Version: "4.1.0", Generation: 3})
	require.NoError(t, err)
	counts := map[string]int{}
	for _, route := range routes {
		mode := config.RouteModes[route]
		if mode == "" {
			mode = packagebridge.RouteModeLegacy
		}
		counts[mode]++
	}
	return counts
}
