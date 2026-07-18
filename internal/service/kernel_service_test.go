package service

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

func newKernelTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, EnsureKernelSchema(db))
	return db
}

func TestEnsureKernelSchemaSeedsOfficialServiceScopes(t *testing.T) {
	db := newKernelTestDB(t)
	require.NoError(t, EnsureKernelSchema(db))

	var scopes []model.ServiceScope
	require.NoError(t, db.Order("id").Find(&scopes).Error)
	require.Len(t, scopes, 4)

	owners := make(map[string]string, len(scopes))
	for _, scope := range scopes {
		owners[scope.ID] = scope.PluginID
	}
	require.Equal(t, map[string]string{
		"forward":      "forward",
		"monitoring":   "machine-telemetry",
		"proxy":        "protocol-runtime",
		"subscription": "subscription",
	}, owners)
}

func seedKernelTestRelease(t *testing.T, db *gorm.DB, pluginID, version, target string) {
	t.Helper()
	manifest := PluginManifest{
		ID: pluginID, Name: pluginID, Version: version, APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{target}, ArtifactSHA256: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
	}
	manifestJSON, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginRelease{
		PluginID: pluginID, Version: version, APIVersion: "v1", ManifestJSON: string(manifestJSON),
		ArtifactSHA256: manifest.ArtifactSHA256, Signature: "test",
	}).Error)
}

func seedKernelTestManifestRelease(t *testing.T, db *gorm.DB, manifest PluginManifest) model.PluginRelease {
	t.Helper()
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	require.NoError(t, db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.Plugin{
		ID: manifest.ID, Name: manifest.Name, Publisher: manifest.Publisher, Official: true,
	}).Error)
	release := model.PluginRelease{
		PluginID: manifest.ID, Version: manifest.Version, APIVersion: manifest.APIVersion,
		ManifestJSON: string(canonical), ArtifactSHA256: manifest.ArtifactSHA256, Signature: "test",
	}
	require.NoError(t, db.Create(&release).Error)
	return release
}

func kernelTestWebUIManifest(pluginID string) PluginManifest {
	permission := pluginID + ".view"
	route := "/admin/extensions/" + pluginID
	return PluginManifest{
		ID: pluginID, Name: "Test Extension", Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: strings.Repeat("a", 64),
		Permissions: []string{permission}, ConfigSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		WebUI: &PluginWebUI{
			Bundle:      PluginWebUIBundle{Path: "webui/index.mjs", SHA256: strings.Repeat("b", 64)},
			Permissions: []string{permission},
			Routes:      []PluginWebUIRoute{{ID: pluginID + ".main", Path: route, Export: "default", Permission: permission}},
			Menus:       []PluginWebUIMenu{{ID: pluginID + ".main", Parent: "services", Label: "Test Extension", Icon: "box", Route: route, Permission: permission, Order: 100}},
		},
	}
}

func kernelTestControlRouteManifest(pluginID string, artifact []byte) PluginManifest {
	return PluginManifest{
		ID: pluginID, Name: "Control Route Plugin", Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: kernelTestArtifactSHA256(artifact),
		Permissions:   []string{PluginAPIPermission(pluginID)},
		ControlRoutes: []string{"/api/v3/plugins/" + pluginID + "/status", "/api/v3/plugins/" + pluginID + "/admin/*"},
	}
}

func kernelTestArtifact(pluginID string) []byte {
	return []byte("anixops-test-artifact:" + pluginID)
}

func kernelTestArtifactSHA256(artifact []byte) string {
	digest := sha256.Sum256(artifact)
	return hex.EncodeToString(digest[:])
}

func kernelTestWebUIPackage(t *testing.T, manifest *PluginManifest, source string) []byte {
	t.Helper()
	require.NotNil(t, manifest.WebUI)
	bundleBytes := []byte(source)
	manifest.WebUI.Bundle.SHA256 = kernelTestArtifactSHA256(bundleBytes)
	manifest.FrontendSHA256 = manifest.WebUI.Bundle.SHA256
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	bundleWriter, err := writer.Create(manifest.WebUI.Bundle.Path)
	require.NoError(t, err)
	_, err = bundleWriter.Write(bundleBytes)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	artifact := buf.Bytes()
	manifest.ArtifactSHA256 = kernelTestArtifactSHA256(artifact)
	return artifact
}

func readControlManifestGolden(t *testing.T) []byte {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "contracts", "plugin", "v1", "manifest-golden.json"))
	require.NoError(t, err)
	return bytes.TrimSpace(raw)
}

func readControlManifestNegativeFixtures(t *testing.T) []struct {
	Name      string          `json:"name"`
	WantError string          `json:"want_error"`
	Manifest  json.RawMessage `json:"manifest"`
} {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "contracts", "plugin", "v1", "manifest-negative.json"))
	require.NoError(t, err)
	var suite struct {
		APIVersion string `json:"api_version"`
		Cases      []struct {
			Name      string          `json:"name"`
			WantError string          `json:"want_error"`
			Manifest  json.RawMessage `json:"manifest"`
		} `json:"cases"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&suite))
	require.Equal(t, "anixops.plugin.negative/v1", suite.APIVersion)
	require.NotEmpty(t, suite.Cases)
	return suite.Cases
}

func TestResolveEffectiveAccessUnionsMembershipWithinScope(t *testing.T) {
	db := newKernelTestDB(t)
	forwardDirect := model.AccessGroup{ScopeID: "forward", Name: "direct", Enabled: true}
	forwardPlan := model.AccessGroup{ScopeID: "forward", Name: "plan", Enabled: true}
	proxy := model.AccessGroup{ScopeID: "proxy", Name: "proxy", Enabled: true}
	require.NoError(t, db.Create(&[]*model.AccessGroup{&forwardDirect, &forwardPlan, &proxy}).Error)
	require.NoError(t, db.Create(&model.AccessGroupUser{GroupID: forwardDirect.ID, UserID: 7}).Error)
	require.NoError(t, db.Create(&model.AccessGroupPlan{GroupID: forwardPlan.ID, PlanID: 9}).Error)
	require.NoError(t, db.Create(&model.AccessGroupUser{GroupID: proxy.ID, UserID: 7}).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{GroupID: forwardDirect.ID, ResourceType: "node", ResourceID: "1", Permissions: `["use"]`}).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{GroupID: forwardPlan.ID, ResourceType: "topology", ResourceID: "2", Permissions: `["use"]`}).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{GroupID: proxy.ID, ResourceType: "node", ResourceID: "3", Permissions: `["use"]`}).Error)

	planID := uint(9)
	access, err := ResolveEffectiveAccess(db, 7, &planID, "forward")
	require.NoError(t, err)
	require.Len(t, access.Groups, 2)
	require.Len(t, access.Grants, 2)
	for _, grant := range access.Grants {
		require.NotEqual(t, "3", grant.ResourceID, "proxy grants must not leak into forward scope")
	}
}

func TestResolveActorPluginAccessUsesPerPluginLegacyFallback(t *testing.T) {
	db := newKernelTestDB(t)

	legacyAdmin, err := ResolveActorPluginAccess(db, 7, true)
	require.NoError(t, err)
	require.True(t, legacyAdmin.Unrestricted)
	require.Equal(t, PluginPermissionModeLegacy, legacyAdmin.PermissionMode())
	require.Nil(t, legacyAdmin.ProfilePermissions())
	require.Empty(t, legacyAdmin.RestrictedPluginList())
	require.True(t, legacyAdmin.Allows("example-ui", "example-ui.view"))

	regularActor, err := ResolveActorPluginAccess(db, 8, false)
	require.NoError(t, err)
	require.False(t, regularActor.Unrestricted)
	require.Equal(t, PluginPermissionModeAuthoritative, regularActor.PermissionMode())
	require.Empty(t, regularActor.ProfilePermissions())

	operators := model.AccessGroup{ScopeID: "example-ui", Name: "operators", Enabled: true}
	viewers := model.AccessGroup{ScopeID: "example-ui", Name: "viewers", Enabled: true}
	dirty := model.AccessGroup{ScopeID: "dirty-ui", Name: "dirty", Enabled: true}
	require.NoError(t, db.Create(&operators).Error)
	require.NoError(t, db.Create(&viewers).Error)
	require.NoError(t, db.Create(&dirty).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{
		GroupID: operators.ID, ResourceType: PluginAPIGrantResourceType, ResourceID: "example-ui",
		Permissions: `{"example-ui.manage":true,"example-ui.disabled":false,"example-ui.view":"1","other-ui.escape":true}`,
	}).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{
		GroupID: viewers.ID, ResourceType: PluginAPIGrantResourceType, ResourceID: "example-ui",
		Permissions: `["example-ui.view","example-ui.audit","example-ui.view"]`,
	}).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{
		GroupID: operators.ID, ResourceType: PluginAPIGrantResourceType, ResourceID: "other-ui",
		Permissions: `["*"]`,
	}).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{
		GroupID: dirty.ID, ResourceType: PluginAPIGrantResourceType, ResourceID: "dirty-ui",
		Permissions: `["other-ui.escape"]`,
	}).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{
		GroupID: dirty.ID, ResourceType: PluginAPIGrantResourceType, ResourceID: "empty-ui",
		Permissions: `{"empty-ui.view":false}`,
	}).Error)

	unassignedAdmin, err := ResolveActorPluginAccess(db, 7, true)
	require.NoError(t, err)
	require.False(t, unassignedAdmin.Unrestricted)
	require.Equal(t, PluginPermissionModeMixed, unassignedAdmin.PermissionMode())
	require.Equal(t, []string{"example-ui", "other-ui"}, unassignedAdmin.RestrictedPluginList())
	require.Empty(t, unassignedAdmin.ProfilePermissions())
	require.False(t, unassignedAdmin.Allows("example-ui", "example-ui.view"))
	require.True(t, unassignedAdmin.Allows("legacy-ui", "legacy-ui.view"), "legacy admins retain fallback for plugins without grants")
	require.True(t, unassignedAdmin.Allows("dirty-ui", "dirty-ui.view"), "cross-plugin-only grants must not lock a plugin")
	require.True(t, unassignedAdmin.Allows("empty-ui", "empty-ui.view"), "grants with no enabled permissions must not lock a plugin")

	regularWithGrants, err := ResolveActorPluginAccess(db, 8, false)
	require.NoError(t, err)
	require.Empty(t, regularWithGrants.ProfileRestrictedPluginList(), "non-admin profiles must not enumerate configured plugin grants")

	require.NoError(t, db.Create(&model.AccessGroupUser{GroupID: operators.ID, UserID: 7}).Error)
	require.NoError(t, db.Create(&model.AccessGroupUser{GroupID: viewers.ID, UserID: 7}).Error)
	assignedAdmin, err := ResolveActorPluginAccess(db, 7, true)
	require.NoError(t, err)
	require.Equal(t, PluginPermissionModeMixed, assignedAdmin.PermissionMode())
	require.Equal(t, []string{"example-ui.audit", "example-ui.manage", "example-ui.view", "other-ui.*"}, assignedAdmin.ProfilePermissions())
	require.True(t, assignedAdmin.Allows("example-ui", "example-ui.manage"))
	require.False(t, assignedAdmin.Allows("example-ui", "other-ui.manage"), "a grant cannot escape its resource_id plugin")
	require.False(t, assignedAdmin.Allows("example-ui", "other-ui.escape"), "cross-plugin permission strings must be discarded")
	require.NotContains(t, assignedAdmin.ProfilePermissions(), "other-ui.escape")
	require.True(t, assignedAdmin.Allows("other-ui", "other-ui.anything"))

	require.NoError(t, db.Model(&model.AccessGroup{}).Where("id IN ?", []uint{operators.ID, viewers.ID}).Update("enabled", false).Error)
	disabledGrantAdmin, err := ResolveActorPluginAccess(db, 9, true)
	require.NoError(t, err)
	require.False(t, disabledGrantAdmin.Unrestricted, "stored grants keep per-plugin restriction mode active even when their groups are disabled")
	require.Equal(t, PluginPermissionModeMixed, disabledGrantAdmin.PermissionMode())
	require.Empty(t, disabledGrantAdmin.ProfilePermissions())
}

func TestResolveActorPluginAccessWithoutKernelSchemaPreservesV2Profile(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	regular, err := ResolveActorPluginAccess(db, 8, false)
	require.NoError(t, err)
	require.Equal(t, PluginPermissionModeAuthoritative, regular.PermissionMode())
	require.Empty(t, regular.ProfilePermissions())
	require.False(t, regular.Allows("machine-telemetry", "machine-telemetry.view"))

	admin, err := ResolveActorPluginAccess(db, 7, true)
	require.NoError(t, err)
	require.Equal(t, PluginPermissionModeLegacy, admin.PermissionMode())
	require.Nil(t, admin.ProfilePermissions())
	require.True(t, admin.Allows("machine-telemetry", "machine-telemetry.view"))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	_, err = ResolveActorPluginAccess(db, 7, true)
	require.Error(t, err, "closed database errors must not be mistaken for an unmigrated Kernel schema")
}

func TestVerifyPluginReleaseRejectsTampering(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := PluginManifest{
		ID: "wireguard", Name: "WireGuard", Version: "1.0.0", APIVersion: "v1",
		Publisher: "AnixOps", Targets: []string{"agent"},
		ArtifactSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))

	verified, err := VerifyPluginRelease(string(canonical), signature, publicKey)
	require.NoError(t, err)
	require.Equal(t, "wireguard", verified.ID)

	manifest.Version = "1.0.1"
	tampered, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	_, err = VerifyPluginRelease(string(tampered), signature, publicKey)
	require.ErrorContains(t, err, "signature verification failed")
}

func TestPluginManifestGoldenContract(t *testing.T) {
	fixture := readControlManifestGolden(t)
	var manifest PluginManifest
	require.NoError(t, json.Unmarshal(fixture, &manifest))
	require.NoError(t, manifest.Validate())

	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	require.Equal(t, string(fixture), string(canonical), "golden fixture must be the exact signed canonical bytes")

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	verified, err := VerifyPluginRelease(string(fixture), signature, publicKey)
	require.NoError(t, err)
	require.Equal(t, "machine-telemetry", verified.ID)
	require.NotNil(t, verified.WebUI)
	require.Equal(t, "webui/index.mjs", verified.WebUI.Bundle.Path)
}

func TestPluginManifestSharedNegativeFixtures(t *testing.T) {
	for _, fixture := range readControlManifestNegativeFixtures(t) {
		t.Run(fixture.Name, func(t *testing.T) {
			var manifest PluginManifest
			decoder := json.NewDecoder(bytes.NewReader(fixture.Manifest))
			decoder.DisallowUnknownFields()
			err := decoder.Decode(&manifest)
			if err == nil {
				err = manifest.Validate()
			}
			require.ErrorContains(t, err, fixture.WantError)
		})
	}
}

func TestPluginManifestValidatesControlAgentContract(t *testing.T) {
	base := PluginManifest{
		ID: "gost-mesh", Name: "GOST Mesh", Version: "1.0.0", APIVersion: pluginManifestAPIVersion,
		Publisher: "AnixOps", Targets: []string{"agent"}, Architectures: []string{"linux/amd64"},
		ArtifactSHA256: strings.Repeat("a", sha256.Size*2), Dependencies: []string{"machine-telemetry"},
		Conflicts: []string{"legacy-gost"}, ConfigSchema: json.RawMessage(`{"type":"object"}`),
		Entrypoints: map[string]string{"health": "bin/health"}, FrontendSHA256: strings.Repeat("b", 64),
	}
	require.NoError(t, base.Validate())

	tests := []struct {
		name    string
		mutate  func(*PluginManifest)
		wantErr string
	}{
		{name: "API version", mutate: func(m *PluginManifest) { m.APIVersion = "v2" }, wantErr: "API version"},
		{name: "unsafe version", mutate: func(m *PluginManifest) { m.Version = "../1.0.0" }, wantErr: "safe path"},
		{name: "duplicate target", mutate: func(m *PluginManifest) { m.Targets = []string{"agent", "agent"} }, wantErr: "duplicate target"},
		{name: "invalid architecture", mutate: func(m *PluginManifest) { m.Architectures = []string{"linux/../amd64"} }, wantErr: "invalid plugin architecture"},
		{name: "duplicate architecture", mutate: func(m *PluginManifest) { m.Architectures = []string{"linux/amd64", "linux/amd64"} }, wantErr: "duplicate plugin architecture"},
		{name: "self dependency", mutate: func(m *PluginManifest) { m.Dependencies = []string{"gost-mesh"} }, wantErr: "depend on itself"},
		{name: "duplicate dependency", mutate: func(m *PluginManifest) { m.Dependencies = []string{"wireguard", "wireguard"} }, wantErr: "duplicate plugin dependency"},
		{name: "dependency conflict overlap", mutate: func(m *PluginManifest) { m.Dependencies, m.Conflicts = []string{"wireguard"}, []string{"wireguard"} }, wantErr: "both a dependency"},
		{name: "entrypoint traversal", mutate: func(m *PluginManifest) { m.Entrypoints = map[string]string{"health": "../bin/health"} }, wantErr: "relative path"},
		{name: "invalid frontend hash", mutate: func(m *PluginManifest) { m.FrontendSHA256 = "not-a-hash" }, wantErr: "frontend_sha256"},
		{name: "negative migration", mutate: func(m *PluginManifest) { m.Migration = -1 }, wantErr: "migration_version"},
		{name: "array config schema", mutate: func(m *PluginManifest) { m.ConfigSchema = json.RawMessage(`[]`) }, wantErr: "config_schema"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := base
			test.mutate(&manifest)
			require.ErrorContains(t, manifest.Validate(), test.wantErr)
		})
	}
}

func TestPluginManifestValidatesControlRoutes(t *testing.T) {
	artifact := kernelTestArtifact("machine-telemetry")
	base := kernelTestControlRouteManifest("machine-telemetry", artifact)
	require.NoError(t, base.Validate())

	v4 := base
	v4.ControlRoutes = []string{"/api/v4/plugins/machine-telemetry/status"}
	require.NoError(t, v4.Validate())

	tests := []struct {
		name    string
		mutate  func(*PluginManifest)
		wantErr string
	}{
		{name: "without control target", mutate: func(m *PluginManifest) { m.Targets = []string{"agent"} }, wantErr: "control target"},
		{name: "missing api permission", mutate: func(m *PluginManifest) { m.Permissions = []string{"machine-telemetry.view"} }, wantErr: "machine-telemetry.api"},
		{name: "permission outside namespace", mutate: func(m *PluginManifest) { m.Permissions = append(m.Permissions, "admin.users") }, wantErr: "outside the plugin namespace"},
		{name: "outside plugin namespace", mutate: func(m *PluginManifest) { m.ControlRoutes = []string{"/api/v3/plugins/wireguard/status"} }, wantErr: "outside"},
		{name: "query string", mutate: func(m *PluginManifest) { m.ControlRoutes = []string{"/api/v3/plugins/machine-telemetry/status?x=1"} }, wantErr: "unsafe"},
		{name: "middle wildcard", mutate: func(m *PluginManifest) { m.ControlRoutes = []string{"/api/v3/plugins/machine-telemetry/*/status"} }, wantErr: "wildcard"},
		{name: "duplicate route", mutate: func(m *PluginManifest) {
			m.ControlRoutes = []string{"/api/v3/plugins/machine-telemetry/status", "/api/v3/plugins/machine-telemetry/status"}
		}, wantErr: "duplicate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := base
			manifest.Permissions = append([]string{}, base.Permissions...)
			manifest.ControlRoutes = append([]string{}, base.ControlRoutes...)
			manifest.Targets = append([]string{}, base.Targets...)
			test.mutate(&manifest)
			require.ErrorContains(t, manifest.Validate(), test.wantErr)
		})
	}
}

func TestRegisterPluginReleaseCreatesOnlyVerifiedOfficialCatalogEntry(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := PluginManifest{
		ID: "example-official", Name: "Example Official", Version: "1.0.0", APIVersion: "v1",
		Publisher: "AnixOps", Targets: []string{"control"},
		ArtifactSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	manifestJSON, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, manifestJSON))

	release, err := RegisterPluginRelease(db, string(manifestJSON), signature, publicKey)
	require.NoError(t, err)
	require.Equal(t, manifest.ID, release.PluginID)
	require.Equal(t, PluginTrustRootFingerprint(publicKey), release.TrustRootFingerprint)
	require.Equal(t, PluginTrustRootKeyID(publicKey), release.TrustRootKeyID)

	var plugin model.Plugin
	require.NoError(t, db.First(&plugin, "id = ?", manifest.ID).Error)
	require.True(t, plugin.Official)
	require.Equal(t, "AnixOps", plugin.Publisher)
	require.Equal(t, string(manifestJSON), release.ManifestJSON, "only canonical signed metadata should be persisted")
	var trustRoot model.PluginTrustRoot
	require.NoError(t, db.First(&trustRoot, "fingerprint = ?", release.TrustRootFingerprint).Error)
	require.True(t, trustRoot.Active)
	require.Equal(t, base64.StdEncoding.EncodeToString(publicKey), trustRoot.PublicKey)

	_, err = RegisterPluginRelease(db, string(manifestJSON), signature, publicKey)
	require.Error(t, err, "a release version cannot be overwritten")
}

func TestRegisterPluginReleaseTracksTrustRootRotation(t *testing.T) {
	db := newKernelTestDB(t)
	register := func(version string) {
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		manifest := PluginManifest{
			ID: "rotating-plugin", Name: "Rotating Plugin", Version: version, APIVersion: "v1",
			Publisher: "AnixOps", Targets: []string{"control"},
			ArtifactSHA256: strings.Repeat("b", 64),
		}
		manifestJSON, err := CanonicalPluginManifest(manifest)
		require.NoError(t, err)
		signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, manifestJSON))
		release, err := RegisterPluginRelease(db, string(manifestJSON), signature, publicKey)
		require.NoError(t, err)
		require.Equal(t, PluginTrustRootFingerprint(publicKey), release.TrustRootFingerprint)
	}

	register("1.0.0")
	register("1.0.1")

	var count int64
	require.NoError(t, db.Model(&model.PluginTrustRoot{}).Where("active = ?", true).Count(&count).Error)
	require.EqualValues(t, 2, count)
}

func TestRegisterPluginReleaseCanonicalizesEquivalentInput(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := PluginManifest{
		ID: "canonical-release", Name: "Canonical Release", Version: "1.0.0", APIVersion: "v1",
		Publisher: "AnixOps", Targets: []string{"control"},
		ArtifactSHA256: strings.Repeat("a", 64),
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	pretty := "\n  " + strings.ReplaceAll(string(canonical), ",", ",\n  ") + "\n"

	release, err := RegisterPluginRelease(db, pretty, signature, publicKey)
	require.NoError(t, err)
	require.Equal(t, string(canonical), release.ManifestJSON)
}

func TestKernelBooleanFieldsPreserveExplicitFalse(t *testing.T) {
	db := newKernelTestDB(t)
	group := model.AccessGroup{ScopeID: "forward", Name: "disabled", Enabled: false}
	require.NoError(t, db.Create(&group).Error)
	assignment := model.NodeServiceAssignment{NodeID: 1, ServiceScope: "forward", PluginID: "nftables-forward", Role: "entry", Enabled: false}
	require.NoError(t, db.Create(&assignment).Error)

	var storedGroup model.AccessGroup
	require.NoError(t, db.First(&storedGroup, group.ID).Error)
	require.False(t, storedGroup.Enabled)
	var storedAssignment model.NodeServiceAssignment
	require.NoError(t, db.First(&storedAssignment, assignment.ID).Error)
	require.False(t, storedAssignment.Enabled)
}

func TestResolvePluginControlRouteUsesSignedRouteAndGrantPermissions(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	artifact := kernelTestArtifact("machine-telemetry")
	manifest := kernelTestControlRouteManifest("machine-telemetry", artifact)
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	installation := model.PluginInstallation{
		PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version, ObservedVersion: manifest.Version,
		State: "healthy", Enabled: true, LifecycleGeneration: 7,
	}
	require.NoError(t, db.Create(&installation).Error)

	resolution, err := ResolvePluginControlRoute(db, publicKey, manifest.ID, "/api/v3/plugins/machine-telemetry/status", 7, true)
	require.NoError(t, err)
	require.Equal(t, "/api/v3/plugins/machine-telemetry/status", resolution.MatchedRoute)
	require.Equal(t, PluginAPIPermission(manifest.ID), resolution.Permission)
	require.EqualValues(t, 7, resolution.Generation)

	_, err = ResolvePluginControlRoute(db, publicKey, manifest.ID, "/api/v3/plugins/machine-telemetry/status", 7, false)
	require.ErrorIs(t, err, ErrPluginRouteForbidden)

	group := model.AccessGroup{ScopeID: manifest.ID, Name: "operators", Enabled: true}
	require.NoError(t, db.Create(&group).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{
		GroupID: group.ID, ResourceType: PluginAPIGrantResourceType, ResourceID: manifest.ID,
		Permissions: `["` + PluginAPIPermission(manifest.ID) + `"]`,
	}).Error)

	_, err = ResolvePluginControlRoute(db, publicKey, manifest.ID, "/api/v3/plugins/machine-telemetry/admin/nodes/1", 7, true)
	require.ErrorIs(t, err, ErrPluginRouteForbidden)

	require.NoError(t, db.Create(&model.AccessGroupUser{GroupID: group.ID, UserID: 7}).Error)
	resolution, err = ResolvePluginControlRoute(db, publicKey, manifest.ID, "/api/v3/plugins/machine-telemetry/admin/nodes/1", 7, true)
	require.NoError(t, err)
	require.Equal(t, "/api/v3/plugins/machine-telemetry/admin/*", resolution.MatchedRoute)

	_, err = ResolvePluginControlRoute(db, publicKey, manifest.ID, "/api/v3/plugins/machine-telemetry/missing", 7, true)
	require.ErrorIs(t, err, ErrPluginRouteNotFound)
}

func TestVerifyPluginReleaseRejectsUnknownManifestFields(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, err = VerifyPluginRelease(`{"id":"wireguard","name":"WireGuard","version":"1.0.0","api_version":"v1","publisher":"AnixOps","targets":["agent"],"artifact_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","unexpected":true}`, "", publicKey)
	require.ErrorContains(t, err, "unknown field")
}

func TestPluginManifestValidatesSafeWebUIMetadata(t *testing.T) {
	valid := kernelTestWebUIManifest("example-ui")
	require.NoError(t, valid.Validate())

	tests := []struct {
		name   string
		mutate func(*PluginManifest)
	}{
		{"remote bundle", func(manifest *PluginManifest) { manifest.WebUI.Bundle.Path = "https://evil.example/plugin.mjs" }},
		{"protocol relative bundle", func(manifest *PluginManifest) { manifest.WebUI.Bundle.Path = "//evil.example/plugin.mjs" }},
		{"bundle traversal", func(manifest *PluginManifest) { manifest.WebUI.Bundle.Path = "webui/../plugin.mjs" }},
		{"encoded bundle traversal", func(manifest *PluginManifest) { manifest.WebUI.Bundle.Path = "webui/%2e%2e/plugin.mjs" }},
		{"route outside namespace", func(manifest *PluginManifest) { manifest.WebUI.Routes[0].Path = "/admin/users" }},
		{"route query", func(manifest *PluginManifest) { manifest.WebUI.Routes[0].Path += "?script=https://evil.example/x.js" }},
		{"core permission", func(manifest *PluginManifest) { manifest.WebUI.Permissions[0] = "admin.users" }},
		{"undeclared menu route", func(manifest *PluginManifest) { manifest.WebUI.Menus[0].Route += "/missing" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := kernelTestWebUIManifest("example-ui")
			test.mutate(&manifest)
			require.Error(t, manifest.Validate())
		})
	}
}

func TestListEnabledWebUIExtensionsRequiresVerifiedVersionBoundInstallation(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := kernelTestWebUIManifest("example-ui")
	artifact := kernelTestWebUIPackage(t, &manifest, `export const anixopsExtension = {}; export default {};`)
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	release, err := RegisterPluginRelease(db, string(canonical), signature, publicKey)
	require.NoError(t, err)
	_, err = StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	installation := model.PluginInstallation{
		PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version,
		ObservedVersion: manifest.Version, State: "healthy", Enabled: true,
	}
	require.NoError(t, db.Create(&installation).Error)

	extensions, err := ListEnabledWebUIExtensions(db, publicKey)
	require.NoError(t, err)
	require.Len(t, extensions, 1)
	extension := extensions[0]
	require.Equal(t, manifest.ID, extension.PluginID)
	require.Equal(t, installation.ID, extension.InstallationID)
	require.Equal(t, "webui/index.mjs", extension.Bundle.Path)
	require.Equal(t, manifest.WebUI.Bundle.SHA256, extension.Bundle.SHA256)
	require.Equal(t, PluginWebUIAssetURL(manifest.ID, manifest.Version, manifest.WebUI.Bundle.SHA256, manifest.WebUI.Bundle.Path), extension.Bundle.URL)
	require.Equal(t, manifest.WebUI.Menus, extension.Menus)
	require.JSONEq(t, string(manifest.ConfigSchema), string(extension.ConfigSchema))

	require.NoError(t, db.Model(&installation).Updates(map[string]any{"state": "pending"}).Error)
	extensions, err = ListEnabledWebUIExtensions(db, publicKey)
	require.NoError(t, err)
	require.Empty(t, extensions)
	require.NoError(t, db.Model(&installation).Updates(map[string]any{"state": "healthy", "desired_version": "2.0.0"}).Error)
	_, err = ListEnabledWebUIExtensions(db, publicKey)
	require.ErrorIs(t, err, ErrExtensionCatalogIntegrity)

	require.NoError(t, db.Model(&installation).Update("desired_version", manifest.Version).Error)
	require.NoError(t, db.Model(release).Update("signature", "tampered").Error)
	_, err = ListEnabledWebUIExtensions(db, publicKey)
	require.ErrorIs(t, err, ErrExtensionCatalogIntegrity)
}

func TestListEnabledWebUIExtensionsNormalizesUnknownMenuParent(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := kernelTestWebUIManifest("legacy-menu-ui")
	manifest.WebUI.Menus[0].Parent = "legacy-forward"
	artifact := kernelTestWebUIPackage(t, &manifest, `export const anixopsExtension = {}; export default {};`)
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version,
		ObservedVersion: manifest.Version, State: "healthy", Enabled: true,
	}).Error)

	extensions, err := ListEnabledWebUIExtensions(db, publicKey)
	require.NoError(t, err)
	require.Len(t, extensions, 1)
	require.Len(t, extensions[0].Menus, 1)
	require.Equal(t, PluginWebUIMenuParentExtensions, extensions[0].Menus[0].Parent)
}

func TestListEnabledWebUIExtensionsFiltersCatalogToActorWebUIPermissions(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := kernelTestWebUIManifest("filtered-ui")
	manifest.Permissions = append(manifest.Permissions, "filtered-ui.manage")
	manifest.WebUI.Permissions = append(manifest.WebUI.Permissions, "filtered-ui.manage")
	manifest.WebUI.Routes = append(manifest.WebUI.Routes, PluginWebUIRoute{
		ID: "filtered-ui.manage", Path: "/admin/extensions/filtered-ui/manage", Export: "Manage", Permission: "filtered-ui.manage",
	})
	manifest.WebUI.Menus = append(manifest.WebUI.Menus, PluginWebUIMenu{
		ID: "filtered-ui.manage", Parent: "services", Label: "Manage", Icon: "box",
		Route: "/admin/extensions/filtered-ui/manage", Permission: "filtered-ui.manage", Order: 101,
	})
	artifact := kernelTestWebUIPackage(t, &manifest, `export const anixopsExtension = {}; export default {};`)
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version,
		ObservedVersion: manifest.Version, State: "healthy", Enabled: true,
	}).Error)

	group := model.AccessGroup{ScopeID: manifest.ID, Name: "viewers", Enabled: true}
	require.NoError(t, db.Create(&group).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{
		GroupID: group.ID, ResourceType: PluginAPIGrantResourceType, ResourceID: manifest.ID,
		Permissions: `["filtered-ui.view"]`,
	}).Error)
	require.NoError(t, db.Create(&model.AccessGroupUser{GroupID: group.ID, UserID: 7}).Error)

	extensions, err := ListEnabledWebUIExtensionsForActor(db, publicKey, 7, true)
	require.NoError(t, err)
	require.Len(t, extensions, 1)
	require.Equal(t, []string{"filtered-ui.view"}, extensions[0].Permissions)
	require.Len(t, extensions[0].Routes, 1)
	require.Equal(t, "filtered-ui.main", extensions[0].Routes[0].ID)
	require.Len(t, extensions[0].Menus, 1)
	require.Equal(t, "filtered-ui.main", extensions[0].Menus[0].ID)

	extensions, err = ListEnabledWebUIExtensionsForActor(db, publicKey, 8, true)
	require.NoError(t, err)
	require.Empty(t, extensions, "once grants exist an unassigned administrator must fail closed")
}

func TestListEnabledWebUIExtensionsForActorQuarantinesInvalidPlugin(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	for _, pluginID := range []string{"catalog-broken", "catalog-valid"} {
		manifest := kernelTestWebUIManifest(pluginID)
		artifact := kernelTestWebUIPackage(t, &manifest, `export const anixopsExtension = {}; export default {};`)
		canonical, err := CanonicalPluginManifest(manifest)
		require.NoError(t, err)
		release, err := RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
		require.NoError(t, err)
		_, err = StorePluginArtifact(db, release.ID, artifact)
		require.NoError(t, err)
		require.NoError(t, db.Create(&model.PluginInstallation{
			PluginID: pluginID, Target: "control", DesiredVersion: manifest.Version,
			ObservedVersion: manifest.Version, State: "healthy", Enabled: true,
		}).Error)
		if pluginID == "catalog-broken" {
			require.NoError(t, db.Model(release).Update("signature", "tampered").Error)
		}
	}

	extensions, err := ListEnabledWebUIExtensionsForActor(db, publicKey, 7, true)
	require.NoError(t, err)
	require.Len(t, extensions, 1)
	require.Equal(t, "catalog-valid", extensions[0].PluginID)

	_, err = ListEnabledWebUIExtensions(db, publicKey)
	require.ErrorIs(t, err, ErrExtensionCatalogIntegrity, "strict diagnostics must still report quarantined catalog corruption")
}

func TestListEnabledWebUIExtensionsRequiresTrustRootOnlyForActiveCandidates(t *testing.T) {
	db := newKernelTestDB(t)
	extensions, err := ListEnabledWebUIExtensions(db, nil)
	require.NoError(t, err)
	require.NotNil(t, extensions)
	require.Empty(t, extensions)

	manifest := kernelTestWebUIManifest("trust-root-ui")
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginRelease{
		PluginID: manifest.ID, Version: manifest.Version, APIVersion: manifest.APIVersion,
		ManifestJSON: string(canonical), ArtifactSHA256: manifest.ArtifactSHA256, Signature: "unverified",
	}).Error)
	require.NoError(t, db.Create(&model.Plugin{ID: manifest.ID, Name: manifest.Name, Publisher: "AnixOps", Official: true}).Error)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version,
		ObservedVersion: manifest.Version, State: "enabled", Enabled: true,
	}).Error)
	_, err = ListEnabledWebUIExtensions(db, nil)
	require.ErrorIs(t, err, ErrPluginTrustRootRequired)
}

func TestStorePluginArtifactExtractsVerifiedWebUIBundle(t *testing.T) {
	db := newKernelTestDB(t)
	manifest := kernelTestWebUIManifest("asset-ui")
	source := `export const anixopsExtension = {}; export default {};`
	artifact := kernelTestWebUIPackage(t, &manifest, source)
	release := seedKernelTestManifestRelease(t, db, manifest)

	_, err := StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	asset, err := GetPluginWebUIAsset(db, manifest.ID, manifest.Version, manifest.WebUI.Bundle.SHA256)
	require.NoError(t, err)
	require.Equal(t, release.ID, asset.ReleaseID)
	require.Equal(t, manifest.WebUI.Bundle.Path, asset.BundlePath)
	require.Equal(t, int64(len(source)), asset.SizeBytes)
	require.Equal(t, []byte(source), asset.Data)
	require.Equal(t, PluginWebUIAssetStorageKey(manifest.ID, manifest.Version, manifest.WebUI.Bundle.SHA256, manifest.WebUI.Bundle.Path), asset.StorageKey)

	manifest.Version = "1.0.1"
	manifest.WebUI.Bundle.SHA256 = strings.Repeat("c", 64)
	manifest.FrontendSHA256 = manifest.WebUI.Bundle.SHA256
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	rejected := model.PluginRelease{
		PluginID: manifest.ID, Version: manifest.Version, APIVersion: manifest.APIVersion,
		ManifestJSON: string(canonical), ArtifactSHA256: manifest.ArtifactSHA256, Signature: "test",
	}
	require.NoError(t, db.Create(&rejected).Error)
	_, err = StorePluginArtifact(db, rejected.ID, artifact)
	require.ErrorContains(t, err, "webui bundle hash mismatch")
}

func TestStorePluginArtifactVerifiesHashAndImmutability(t *testing.T) {
	db := newKernelTestDB(t)
	artifact := kernelTestArtifact("artifact-plugin")
	manifest := PluginManifest{
		ID: "artifact-plugin", Name: "Artifact Plugin", Version: "1.0.0", APIVersion: pluginManifestAPIVersion,
		Publisher: "AnixOps", Targets: []string{"control"}, ArtifactSHA256: kernelTestArtifactSHA256(artifact),
	}
	release := seedKernelTestManifestRelease(t, db, manifest)

	_, err := StorePluginArtifact(db, release.ID, []byte("tampered"))
	require.ErrorContains(t, err, "hash mismatch")

	stored, err := StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	require.Equal(t, release.ID, stored.ReleaseID)
	require.Equal(t, int64(len(artifact)), stored.SizeBytes)
	require.Equal(t, PluginArtifactStorageKey(manifest.ID, manifest.Version, manifest.ArtifactSHA256), stored.StorageKey)

	storedAgain, err := StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	require.Equal(t, stored.ID, storedAgain.ID)

	require.NoError(t, db.Model(&model.PluginArtifact{}).Where("id = ?", stored.ID).Update("data", []byte("corrupted")).Error)
	_, err = StorePluginArtifact(db, release.ID, artifact)
	require.ErrorIs(t, err, ErrPluginArtifactImmutable)
}

func TestValidatePluginInstallationPlanEnforcesDependenciesAndConflicts(t *testing.T) {
	db := newKernelTestDB(t)
	baseRelease := func(id string) (PluginManifest, []byte) {
		artifact := kernelTestArtifact(id)
		return PluginManifest{
			ID: id, Name: id, Version: "1.0.0", APIVersion: pluginManifestAPIVersion, Publisher: "AnixOps",
			Targets: []string{"control"}, ArtifactSHA256: kernelTestArtifactSHA256(artifact),
		}, artifact
	}
	dependencyManifest, _ := baseRelease("machine-telemetry")
	dependency := seedKernelTestManifestRelease(t, db, dependencyManifest)
	conflictManifest, _ := baseRelease("legacy-gost")
	conflict := seedKernelTestManifestRelease(t, db, conflictManifest)
	pluginManifest, pluginArtifact := baseRelease("gost-mesh")
	pluginManifest.Dependencies = []string{"machine-telemetry"}
	pluginManifest.Conflicts = []string{"legacy-gost"}
	plugin := seedKernelTestManifestRelease(t, db, pluginManifest)

	err := ValidatePluginInstallationPlan(db, plugin, "control", true, 0)
	require.ErrorIs(t, err, ErrPluginArtifactRequired)
	_, err = StorePluginArtifact(db, plugin.ID, pluginArtifact)
	require.NoError(t, err)

	err = ValidatePluginInstallationPlan(db, plugin, "control", true, 0)
	require.ErrorIs(t, err, ErrPluginDependencyUnsatisfied)

	dependencyInstall := model.PluginInstallation{
		PluginID: dependency.PluginID, Target: "control", DesiredVersion: dependency.Version,
		ObservedVersion: dependency.Version, State: "healthy", Enabled: true,
	}
	require.NoError(t, db.Create(&dependencyInstall).Error)
	require.NoError(t, ValidatePluginInstallationPlan(db, plugin, "control", true, 0))

	conflictInstall := model.PluginInstallation{
		PluginID: conflict.PluginID, Target: "control", DesiredVersion: conflict.Version,
		ObservedVersion: conflict.Version, State: "healthy", Enabled: true,
	}
	require.NoError(t, db.Create(&conflictInstall).Error)
	err = ValidatePluginInstallationPlan(db, plugin, "control", true, 0)
	require.ErrorIs(t, err, ErrPluginConflict)

	require.NoError(t, db.Model(&conflictInstall).Update("enabled", false).Error)
	pluginInstall := model.PluginInstallation{
		PluginID: plugin.PluginID, Target: "control", DesiredVersion: plugin.Version,
		ObservedVersion: plugin.Version, State: "healthy", Enabled: true,
	}
	require.NoError(t, db.Create(&pluginInstall).Error)
	err = ValidatePluginInstallationPlan(db, dependency, "control", false, dependencyInstall.ID)
	require.ErrorIs(t, err, ErrPluginDependencyUnsatisfied)
}

func TestUpdatePluginConfigurationVerifiesReleaseSchemaAndRevision(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := kernelTestWebUIManifest("config-ui")
	manifest.ConfigSchema = json.RawMessage(`{"type":"object","required":["port"],"additionalProperties":false,"properties":{"port":{"type":"integer","minimum":1,"maximum":65535}}}`)
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	_, err = RegisterPluginRelease(db, string(canonical), signature, publicKey)
	require.NoError(t, err)
	installation := model.PluginInstallation{PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version, State: "disabled"}
	require.NoError(t, db.Create(&installation).Error)

	expected := int64(0)
	configuration, err := UpdatePluginConfiguration(db, publicKey, installation.ID, ` { "port" : 443 } `, &expected, 42)
	require.NoError(t, err)
	require.Equal(t, int64(1), configuration.Revision)
	require.Equal(t, `{"port":443}`, configuration.ConfigJSON)
	require.Equal(t, uint(42), configuration.UpdatedBy)

	loaded, err := GetPluginConfiguration(db, installation.ID)
	require.NoError(t, err)
	require.Equal(t, configuration.ConfigHash, loaded.ConfigHash)

	_, err = UpdatePluginConfiguration(db, publicKey, installation.ID, `{"port":8443}`, &expected, 42)
	require.ErrorIs(t, err, ErrPluginConfigurationConflict)
	_, err = UpdatePluginConfiguration(db, publicKey, installation.ID, `{"port":0}`, nil, 42)
	require.ErrorContains(t, err, "does not satisfy its schema")
	_, err = UpdatePluginConfiguration(db, publicKey, installation.ID, `{"port":443,"unexpected":true}`, nil, 42)
	require.ErrorContains(t, err, "does not satisfy its schema")

	var storedInstallation model.PluginInstallation
	require.NoError(t, db.First(&storedInstallation, installation.ID).Error)
	require.Equal(t, int64(1), storedInstallation.ConfigRevision)
}

func TestGetPluginConfigurationReturnsQuietDefaultWhenDocumentIsMissing(t *testing.T) {
	var databaseLogs bytes.Buffer
	testLogger := logger.New(log.New(&databaseLogs, "", 0), logger.Config{LogLevel: logger.Warn})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: testLogger})
	require.NoError(t, err)
	require.NoError(t, EnsureKernelSchema(db))

	installation := model.PluginInstallation{
		PluginID: "machine-telemetry", Target: "agent", DesiredVersion: "1.1.0", State: "pending",
	}
	require.NoError(t, db.Create(&installation).Error)
	databaseLogs.Reset()

	configuration, err := GetPluginConfiguration(db, installation.ID)
	require.NoError(t, err)
	require.Equal(t, installation.ID, configuration.InstallationID)
	require.Equal(t, int64(0), configuration.Revision)
	require.JSONEq(t, `{}`, configuration.ConfigJSON)
	require.Len(t, configuration.ConfigHash, sha256.Size*2)
	require.NotContains(t, databaseLogs.String(), "record not found")
}

func TestUpdatePluginConfigurationRunsVersionBoundSemanticValidatorInsideSave(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := kernelTestWebUIManifest("config-semantic")
	manifest.ConfigSchema = json.RawMessage(`{"type":"object","required":["port"],"properties":{"port":{"type":"integer"}}}`)
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	_, err = RegisterPluginRelease(db, string(canonical), signature, publicKey)
	require.NoError(t, err)
	installation := model.PluginInstallation{PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version, State: "disabled"}
	require.NoError(t, db.Create(&installation).Error)

	semanticErr := errors.New("semantic contract rejected configuration")
	_, err = UpdatePluginConfigurationWithValidator(
		db, publicKey, installation.ID, ` { "port" : 443 } `, nil, 7,
		func(pluginID, version string, canonicalConfig json.RawMessage) error {
			require.Equal(t, manifest.ID, pluginID)
			require.Equal(t, manifest.Version, version)
			require.JSONEq(t, `{"port":443}`, string(canonicalConfig))
			return semanticErr
		},
	)
	require.ErrorIs(t, err, semanticErr)
	var count int64
	require.NoError(t, db.Model(&model.PluginConfiguration{}).Where("installation_id = ?", installation.ID).Count(&count).Error)
	require.Zero(t, count)
	var storedInstallation model.PluginInstallation
	require.NoError(t, db.First(&storedInstallation, installation.ID).Error)
	require.Zero(t, storedInstallation.ConfigRevision)

	configuration, err := UpdatePluginConfigurationWithValidator(
		db, publicKey, installation.ID, ` { "port" : 443 } `, nil, 7,
		func(pluginID, version string, canonicalConfig json.RawMessage) error {
			require.Equal(t, manifest.ID, pluginID)
			require.Equal(t, manifest.Version, version)
			require.Equal(t, `{"port":443}`, string(canonicalConfig))
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), configuration.Revision)
}

func TestValidateTopologyDetectsCycleConflictAndMissingSecret(t *testing.T) {
	nodeID := uint(1)
	input := TopologyRevisionInput{
		Vertices: []model.TopologyVertex{
			{Key: "cn", Kind: "entry", NodeID: &nodeID, ConfigJSON: `{"port":443,"protocol":"tcp","mtu":500}`},
			{Key: "exit", Kind: "exit", NodeID: &nodeID, ConfigJSON: `{"port":443,"protocol":"tcp"}`},
		},
		Edges: []model.TopologyEdge{
			{SourceKey: "cn", TargetKey: "exit", Protocol: "wss", ConfigJSON: `{}`},
			{SourceKey: "exit", TargetKey: "cn", Protocol: "tcp", ConfigJSON: `{}`},
		},
	}
	issues := ValidateTopology(input)
	codes := make(map[string]bool)
	for _, issue := range issues {
		codes[issue.Code] = true
	}
	require.True(t, codes["invalid_mtu"])
	require.True(t, codes["port_conflict"])
	require.True(t, codes["secret_required"])
	require.True(t, codes["cycle"])
}

func TestValidateTopologyRejectsInlineSecretButAllowsReference(t *testing.T) {
	inline := TopologyRevisionInput{Vertices: []model.TopologyVertex{{Key: "entry", Kind: "entry", ConfigJSON: `{"private_key":"do-not-store-me"}`}}}
	issues := ValidateTopology(inline)
	require.Len(t, issues, 1)
	require.Equal(t, "inline_secret", issues[0].Code)
	camelCase := TopologyRevisionInput{Vertices: []model.TopologyVertex{{Key: "entry", Kind: "entry", ConfigJSON: `{"apiKey":"do-not-store-me"}`}}}
	require.Equal(t, "inline_secret", ValidateTopology(camelCase)[0].Code)
	clientSecret := TopologyRevisionInput{Vertices: []model.TopologyVertex{{Key: "entry", Kind: "entry", ConfigJSON: `{"client_secret":"do-not-store-me"}`}}}
	require.Equal(t, "inline_secret", ValidateTopology(clientSecret)[0].Code)

	referenced := TopologyRevisionInput{Vertices: []model.TopologyVertex{{Key: "entry", Kind: "entry", ConfigJSON: `{"private_key_ref":"secret/wireguard-entry"}`}}}
	require.Empty(t, ValidateTopology(referenced))
}

func TestValidateTopologyRejectsIncompleteNetworkContract(t *testing.T) {
	input := TopologyRevisionInput{
		Vertices: []model.TopologyVertex{{Key: "entry", ConfigJSON: `{"port":70000,"address_family":"ipx"}`}},
		Edges:    []model.TopologyEdge{{SourceKey: "entry", TargetKey: "entry"}},
	}
	codes := make(map[string]bool)
	for _, issue := range ValidateTopology(input) {
		codes[issue.Code] = true
	}
	require.True(t, codes["vertex_kind_required"])
	require.True(t, codes["invalid_port"])
	require.True(t, codes["invalid_address_family"])
	require.True(t, codes["edge_protocol_required"])
}

func TestCreateTopologyRevisionStoresImmutableSnapshot(t *testing.T) {
	db := newKernelTestDB(t)
	topology := model.Topology{Name: "cn-to-exit", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	input := TopologyRevisionInput{
		Vertices: []model.TopologyVertex{{Key: "cn", Kind: "entry", ConfigJSON: `{}`}, {Key: "exit", Kind: "nat", ConfigJSON: `{}`}},
		Edges:    []model.TopologyEdge{{SourceKey: "cn", TargetKey: "exit", Protocol: "tcp", ConfigJSON: `{}`}},
	}
	first, err := CreateTopologyRevision(db, topology.ID, 1, input)
	require.NoError(t, err)
	second, err := CreateTopologyRevision(db, topology.ID, 1, input)
	require.NoError(t, err)
	require.Equal(t, int64(1), first.Revision)
	require.Equal(t, int64(2), second.Revision)
	require.Equal(t, first.ContentHash, second.ContentHash)

	var count int64
	require.NoError(t, db.Model(&model.TopologyVertex{}).Where("revision_id = ?", first.ID).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestCreateTopologyRevisionRequiresNodePluginAssignment(t *testing.T) {
	db := newKernelTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	node := model.Node{Name: "cn-entry", Host: "127.0.0.1"}
	require.NoError(t, db.Create(&node).Error)
	topology := model.Topology{Name: "assigned-forward", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	input := TopologyRevisionInput{Vertices: []model.TopologyVertex{{Key: "entry", Kind: "plugin", NodeID: &node.ID, PluginID: "nftables-forward", Role: "cn_dedicated_nftables", ConfigJSON: `{}`}}}

	_, err := CreateTopologyRevision(db, topology.ID, 1, input)
	require.ErrorContains(t, err, "no enabled assignment")
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "forward", PluginID: "nftables-forward", Role: "cn_dedicated_nftables", Enabled: true,
	}).Error)

	revision, err := CreateTopologyRevision(db, topology.ID, 1, input)
	require.NoError(t, err)
	require.Equal(t, int64(1), revision.Revision)
}

func TestCreateKernelOperationIsPersistentAndIdempotent(t *testing.T) {
	db := newKernelTestDB(t)
	seedKernelTestRelease(t, db, "wireguard", "1.0.0", "control")
	deadline := time.Now().Add(time.Minute)
	operation := model.KernelOperation{
		ID:             "1c5b99ca-d878-454f-ab2a-88e6e1c971ac",
		IdempotencyKey: "node-1-config-7",
		PluginID:       "wireguard",
		TargetVersion:  "1.0.0",
		Kind:           "plugin.configure",
		Revision:       7,
		ConfigJSON:     ` { "peer" : "example" } `,
		DeadlineAt:     &deadline,
	}
	created, reused, err := CreateKernelOperation(db, operation)
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, "pending", created.State)
	require.Equal(t, `{"peer":"example"}`, created.ConfigJSON)
	require.NotEmpty(t, created.ConfigHash)

	again, reused, err := CreateKernelOperation(db, operation)
	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, created.ID, again.ID)

	cancelled, err := CancelKernelOperation(db, created.ID, time.Time{})
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.State)
	require.NotNil(t, cancelled.CancelAt)
}

func TestCreateKernelOperationAdvancesNodeRevisionAndExpiresDeadline(t *testing.T) {
	db := newKernelTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	node := model.Node{Name: "operation-node", Host: "127.0.0.1"}
	require.NoError(t, db.Create(&node).Error)
	seedKernelTestRelease(t, db, "wireguard", "1.0.0", "agent")
	deadline := time.Now().Add(time.Minute)
	base := model.KernelOperation{
		ID: "e85b8480-3113-464c-b204-3977fc3f20e6", IdempotencyKey: "node-op-1", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.configure", ConfigJSON: `{}`,
		DeadlineAt: &deadline,
	}
	_, _, err := CreateKernelOperation(db, base)
	require.NoError(t, err)
	base.ID, base.IdempotencyKey, base.Revision = "686609eb-3b2a-470a-9f92-27ac15861565", "node-op-2", 1
	_, _, err = CreateKernelOperation(db, base)
	require.ErrorContains(t, err, "must be the next")

	past := time.Now().Add(-time.Minute)
	expired := model.KernelOperation{
		ID: "17f3601c-a2de-44b2-99a2-0ab6cbffb266", IdempotencyKey: "expired-op", SessionID: "session-2",
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.health", Revision: 1,
		ConfigHash: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", State: "pending", DeadlineAt: &past,
	}
	require.NoError(t, db.Create(&expired).Error)
	updated, err := ExpireKernelOperations(db, time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(1), updated)
	require.NoError(t, db.First(&expired, "id = ?", expired.ID).Error)
	require.Equal(t, "timed_out", expired.State)
}

func TestCancelKernelOperationRejectsTerminalStateAndIsIdempotentWhileRequested(t *testing.T) {
	db := newKernelTestDB(t)
	now := time.Now()
	deadline := now.Add(time.Minute)
	operation := model.KernelOperation{
		ID: "4efea128-cd2e-4c6b-a486-8ac0108ca233", IdempotencyKey: "cancel-terminal", SessionID: "session",
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.health", Revision: 1,
		ConfigHash: strings.Repeat("a", 64), State: "completed", DeadlineAt: &deadline,
	}
	require.NoError(t, db.Create(&operation).Error)
	_, err := CancelKernelOperation(db, operation.ID, now)
	require.ErrorIs(t, err, ErrKernelOperationNotPending)

	require.NoError(t, db.Model(&operation).Update("state", "running").Error)
	requested, err := CancelKernelOperation(db, operation.ID, now)
	require.NoError(t, err)
	require.Equal(t, "cancel_requested", requested.State)
	again, err := CancelKernelOperation(db, operation.ID, now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, "cancel_requested", again.State)
}
