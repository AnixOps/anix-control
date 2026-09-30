package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const routeModeTestRoutes = `{"api_version":"v2","package_id":"knowledge","routes":[` +
	`{"method":"GET","legacy_path":"/api/v2/user/knowledge","package_route":"knowledge.article.list","envelope":"panel","transport":"http"},` +
	`{"method":"POST","legacy_path":"/api/v2/admin/knowledge","package_route":"knowledge.admin.knowledge.post","envelope":"panel","transport":"http"},` +
	`{"method":"GET","legacy_path":"/api/v2/ws","package_route":"knowledge.ws","envelope":"websocket","transport":"websocket"}]}`

// seedRouteModeRelease registers and stores a signed v2 control release of
// "knowledge" with routeModeTestRoutes and returns its installation.
func seedRouteModeRelease(t *testing.T, db *gorm.DB, schema string) (ed25519.PublicKey, model.PluginInstallation) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	entrypoint := []byte("#!/bin/sh\nexit 0\n")
	migrations := []byte(`{"format":"anixops.migrations/v1","migrations":[],"package_id":"knowledge","version":"4.0.1"}`)
	routes := []byte(routeModeTestRoutes)
	artifact := kernelTestV2Package(t, map[string][]byte{
		"bin/control-host": entrypoint, "compat/v2-routes.json": routes, "migrations/index.json": migrations,
	})
	digest := func(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
	manifest := PluginManifest{
		ID: "knowledge", Name: "Knowledge", Version: "4.0.1", APIVersion: pluginManifestAPIVersionV2, Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: digest(artifact),
		ControlEntrypoint:   &PluginEntrypoint{Path: "bin/control-host", SHA256: digest(entrypoint)},
		Migrations:          &PluginMigrations{Index: "migrations/index.json", SHA256: digest(migrations)},
		CompatibilityRoutes: &PluginCompatibilityRoutes{Path: "compat/v2-routes.json", SHA256: digest(routes)},
		RouteContractDigest: digest(routes),
	}
	if schema != "" {
		manifest.ConfigSchema = json.RawMessage(schema)
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	installation := model.PluginInstallation{
		PluginID: "knowledge", Target: "control", DesiredVersion: "4.0.1", ObservedVersion: "4.0.1",
		State: "healthy", Enabled: true, LifecycleGeneration: 7,
	}
	require.NoError(t, db.Create(&installation).Error)
	return publicKey, installation
}

func TestSplitPackageRouteModes(t *testing.T) {
	rest, modes, present, err := splitPackageRouteModes(`{"port":443,"routes":{"a.b":"native"}}`)
	require.NoError(t, err)
	assert.True(t, present)
	assert.Equal(t, `{"port":443}`, rest)
	assert.Equal(t, map[string]string{"a.b": "native"}, modes)

	rest, modes, present, err = splitPackageRouteModes(`{"port":443}`)
	require.NoError(t, err)
	assert.False(t, present)
	assert.Nil(t, modes)
	assert.Equal(t, `{"port":443}`, rest)

	_, _, _, err = splitPackageRouteModes(`{"routes":["native"]}`)
	require.Error(t, err)
	_, _, _, err = splitPackageRouteModes(`{"routes":{"a":1}}`)
	require.Error(t, err)

	rest, _, present, err = splitPackageRouteModes(`[1,2]`)
	require.NoError(t, err)
	assert.False(t, present)
	assert.Equal(t, `[1,2]`, rest)
}

func TestConfigurationRouteModesAreValidatedAndHiddenFromThePackageSchema(t *testing.T) {
	db := newKernelTestDB(t)
	// A strict package schema that knows nothing about the reserved key.
	publicKey, installation := seedRouteModeRelease(t, db, `{"type":"object","additionalProperties":false,"properties":{"port":{"type":"integer"}}}`)

	config, err := UpdatePluginConfigurationWithValidatorAndHook(db, publicKey, installation.ID,
		`{"port":443,"routes":{"knowledge.article.list":"shadow","knowledge.admin.knowledge.post":"native"}}`, nil, 1, nil, nil)
	require.NoError(t, err)
	assert.Contains(t, config.ConfigJSON, `"routes"`, "the stored document keeps the route modes")

	invalid := map[string]string{
		"foreign route":      `{"routes":{"ticket.list":"native"}}`,
		"unknown mode":       `{"routes":{"knowledge.article.list":"canary"}}`,
		"shadow on POST":     `{"routes":{"knowledge.admin.knowledge.post":"shadow"}}`,
		"native WebSocket":   `{"routes":{"knowledge.ws":"native"}}`,
		"routes not object":  `{"routes":"native"}`,
		"schema still holds": `{"port":"https","routes":{}}`,
	}
	for name, document := range invalid {
		_, err := UpdatePluginConfigurationWithValidatorAndHook(db, publicKey, installation.ID, document, nil, 1, nil, nil)
		assert.Error(t, err, name)
	}
}

func TestPackageHostOperationsReturnActiveModesOnlyToTheCurrentGeneration(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, installation := seedRouteModeRelease(t, db, "")
	_, err := UpdatePluginConfigurationWithValidatorAndHook(db, publicKey, installation.ID,
		`{"routes":{"knowledge.article.list":"shadow","knowledge.admin.knowledge.post":"legacy"}}`, nil, 1, nil, nil)
	require.NoError(t, err)

	operations := PackageHostOperations{DB: db}
	current := packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.0.1", Generation: 7}
	config, err := operations.PackageConfig(context.Background(), current)
	require.NoError(t, err)
	assert.Equal(t, int64(1), config.Revision)
	assert.NotEmpty(t, config.ConfigHash)
	assert.Equal(t, map[string]string{"knowledge.article.list": "shadow"}, config.RouteModes, "legacy entries are omitted")

	fenced := map[string]packagebridge.HostIdentity{
		"stale generation": {PackageID: "knowledge", Version: "4.0.1", Generation: 6},
		"other version":    {PackageID: "knowledge", Version: "4.0.0", Generation: 7},
		"unknown package":  {PackageID: "ticket", Version: "4.0.1", Generation: 7},
	}
	for name, host := range fenced {
		_, err := operations.PackageConfig(context.Background(), host)
		assert.ErrorIs(t, err, packagebridge.ErrHostFenced, name)
	}

	require.NoError(t, db.Model(&model.PluginInstallation{}).Where("id = ?", installation.ID).Update("enabled", false).Error)
	_, err = operations.PackageConfig(context.Background(), current)
	assert.ErrorIs(t, err, packagebridge.ErrHostFenced, "a disabled package is fenced")
}
