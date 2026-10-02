// Package routemodefixture installs signed v2 Control package releases with
// chosen compatibility routes, for the route-mode administration tests of
// the handler and the command line.
package routemodefixture

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Route is one compatibility route.
type Route struct {
	Method, Path, ID, Transport string
}

// Install registers, stores and installs an enabled v2 Control release of
// packageID (version 4.0.1, lifecycle generation 3) that declares routes,
// signed by privateKey.
func Install(t testing.TB, db *gorm.DB, privateKey ed25519.PrivateKey, packageID string, routes []Route) model.PluginInstallation {
	t.Helper()
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	require.True(t, ok)
	declared := make([]map[string]string, 0, len(routes))
	for _, route := range routes {
		envelope := "panel"
		if route.Transport == "websocket" {
			envelope = "websocket"
		}
		declared = append(declared, map[string]string{
			"method": route.Method, "legacy_path": route.Path, "package_route": route.ID, "envelope": envelope, "transport": route.Transport,
		})
	}
	routeDocument, err := json.Marshal(map[string]any{"api_version": "v2", "package_id": packageID, "routes": declared})
	require.NoError(t, err)
	entrypoint := []byte("#!/bin/sh\nexit 0\n")
	migrations := []byte(`{"format":"anixops.migrations/v1","migrations":[],"package_id":"` + packageID + `","version":"4.0.1"}`)
	files := map[string][]byte{"bin/control-host": entrypoint, "compat/v2-routes.json": routeDocument, "migrations/index.json": migrations}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range names {
		file, err := writer.Create(name)
		require.NoError(t, err)
		_, err = file.Write(files[name])
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	artifact := buffer.Bytes()
	digest := func(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
	manifest := service.PluginManifest{
		ID: packageID, Name: packageID, Version: "4.0.1", APIVersion: "v2", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: digest(artifact),
		ControlEntrypoint:   &service.PluginEntrypoint{Path: "bin/control-host", SHA256: digest(entrypoint)},
		Migrations:          &service.PluginMigrations{Index: "migrations/index.json", SHA256: digest(migrations)},
		CompatibilityRoutes: &service.PluginCompatibilityRoutes{Path: "compat/v2-routes.json", SHA256: digest(routeDocument)},
		RouteContractDigest: digest(routeDocument),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	installation := model.PluginInstallation{
		PluginID: packageID, Target: "control", DesiredVersion: "4.0.1", ObservedVersion: "4.0.1",
		State: "healthy", Enabled: true, LifecycleGeneration: 3,
	}
	require.NoError(t, db.Create(&installation).Error)
	return installation
}

// HTTP is an HTTP compatibility route.
func HTTP(method, path, id string) Route {
	return Route{Method: method, Path: path, ID: id, Transport: "http"}
}
