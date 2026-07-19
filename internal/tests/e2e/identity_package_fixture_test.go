package e2e

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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/identitybridge"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
)

const (
	identityE2EPackageID  = "identity-platform"
	identityE2EVersion    = "4.0.0"
	identityE2EGeneration = uint64(7)
)

// packageE2EHost models the package-host side after the kernel has resolved a
// signed route. It deliberately uses the production bridge allowlist and HTTP
// adapter rather than invoking a legacy Gin route directly.
type packageE2EHost struct {
	allowlist *packagebridge.Allowlist
}

func (h *packageE2EHost) Start(context.Context, pluginhost.ArtifactRef, uint64) error {
	return pluginhost.ErrHostUnavailable
}

func (h *packageE2EHost) Dispatch(ctx context.Context, input pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	if h == nil || h.allowlist == nil || input.Version != identityE2EVersion || input.Generation != identityE2EGeneration {
		return pluginhost.DispatchOutput{}, pluginhost.ErrHostUnavailable
	}
	metadata, err := json.Marshal(input.Metadata)
	if err != nil {
		return pluginhost.DispatchOutput{}, fmt.Errorf("%w: encode package request metadata: %v", pluginhost.ErrHostIncompatible, err)
	}
	response, err := h.allowlist.Invoke(ctx, packagebridge.Call{
		Host: packagebridge.HostIdentity{
			PackageID: input.PackageID, Version: input.Version, Generation: input.Generation,
		},
		Request: packagebridge.Request{
			RequestID: input.RequestID, RouteID: input.RouteID, Method: input.Method,
			Body: input.Body, PrincipalJSON: input.PrincipalJSON, MetadataJSON: metadata, Deadline: input.Deadline,
		},
		Operation: input.RouteID, Payload: input.Body,
	})
	if err != nil {
		return pluginhost.DispatchOutput{}, fmt.Errorf("%w: invoke package bridge: %v", pluginhost.ErrHostUnavailable, err)
	}
	headers := make([]pluginhost.Header, len(response.Headers))
	for index, header := range response.Headers {
		headers[index] = pluginhost.Header{Name: header.Name, Value: header.Value}
	}
	return pluginhost.DispatchOutput{
		StatusCode: response.StatusCode, Body: response.Body, Headers: headers,
	}, nil
}

func (h *packageE2EHost) Health(context.Context, string, string, uint64) (pluginhost.HostHealth, error) {
	return pluginhost.HostHealth{}, pluginhost.ErrHostUnavailable
}

func (h *packageE2EHost) Drain(context.Context, string, string, uint64, time.Time) error {
	return pluginhost.ErrHostUnavailable
}

func (h *packageE2EHost) Stop(context.Context, string, string, uint64) error {
	return pluginhost.ErrHostUnavailable
}

func (h *packageE2EHost) Rollback(context.Context, string, string, uint64) error {
	return pluginhost.ErrHostUnavailable
}

func installIdentityPlatformE2EPackage(t testing.TB, cfg *config.Config) func() {
	t.Helper()
	require.NotNil(t, cfg)
	require.NotNil(t, database.Get())

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cfg.Plugins.OfficialPublicKey = base64.StdEncoding.EncodeToString(publicKey)
	cfg.Plugins.ControlExecutionEnabled = true
	config.Set(cfg)
	require.NoError(t, service.EnsureKernelSchema(database.Get()))

	routes := identityPackageFixtureFile(t, "compat", "v2-routes.json")
	migrations := identityPackageFixtureFile(t, "migrations", "index.json")
	migrationSQL := identityPackageFixtureFile(t, "migrations", "001_identity_platform.sql")
	installE2ESignedPackage(t, identityE2EPackageID, "Identity Platform", routes, migrations, map[string][]byte{
		"migrations/001_identity_platform.sql": migrationSQL,
	}, publicKey, privateKey)
	installTaskSixE2EPackages(t, publicKey, privateKey)
	installTaskSevenE2EPackages(t, publicKey, privateKey)
	installTaskEightE2EPackages(t, publicKey, privateKey)

	allowlist, err := identitybridge.NewAllowlist(cfg)
	require.NoError(t, err)
	previous := pluginhost.DefaultManager()
	pluginhost.SetDefaultManager(&packageE2EHost{allowlist: allowlist})
	return func() { pluginhost.SetDefaultManager(previous) }
}

func installTaskSixE2EPackages(t testing.TB, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) {
	t.Helper()
	for _, packageSpec := range []struct {
		id   string
		name string
	}{
		{id: "knowledge", name: "Knowledge"},
		{id: "ticket", name: "Ticket"},
		{id: "plan", name: "Plan"},
		{id: "notification", name: "Notification"},
	} {
		routes := packageFixtureFile(t, packageSpec.id, "compat", "v2-routes.json")
		migrations, err := json.Marshal(map[string]any{
			"format": "anixops.migrations/v1", "package_id": packageSpec.id, "version": identityE2EVersion, "migrations": []any{},
		})
		require.NoError(t, err)
		installE2ESignedPackage(t, packageSpec.id, packageSpec.name, routes, migrations, nil, publicKey, privateKey)
	}
}

func installTaskSevenE2EPackages(t testing.TB, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) {
	t.Helper()
	for _, packageSpec := range []struct {
		id   string
		name string
	}{
		{id: "order", name: "Order"},
		{id: "payment", name: "Payment"},
		{id: "subscription", name: "Subscription"},
	} {
		routes := packageFixtureFile(t, packageSpec.id, "compat", "v2-routes.json")
		migrations, err := json.Marshal(map[string]any{
			"format": "anixops.migrations/v1", "package_id": packageSpec.id, "version": identityE2EVersion, "migrations": []any{},
		})
		require.NoError(t, err)
		installE2ESignedPackage(t, packageSpec.id, packageSpec.name, routes, migrations, nil, publicKey, privateKey)
	}
}

func installTaskEightE2EPackages(t testing.TB, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) {
	t.Helper()
	for _, packageSpec := range []struct {
		id   string
		name string
	}{
		{id: "machine-telemetry", name: "Machine Telemetry"},
		{id: "proxy-node", name: "Proxy Node"},
		{id: "protocol-runtime", name: "Protocol Runtime"},
		{id: "wireguard", name: "WireGuard"},
		{id: "forward", name: "Forward"},
		{id: "gost-mesh", name: "Gost Mesh"},
	} {
		routes := packageFixtureFile(t, packageSpec.id, "compat", "v2-routes.json")
		migrations, err := json.Marshal(map[string]any{
			"format": "anixops.migrations/v1", "package_id": packageSpec.id, "version": identityE2EVersion, "migrations": []any{},
		})
		require.NoError(t, err)
		installE2ESignedPackage(t, packageSpec.id, packageSpec.name, routes, migrations, nil, publicKey, privateKey)
	}
}

func installE2ESignedPackage(t testing.TB, packageID, name string, routes, migrations []byte, files map[string][]byte, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) {
	t.Helper()
	entrypoint := []byte("#!/bin/sh\nexit 0\n")
	artifactFiles := map[string][]byte{
		"bin/control-host":      entrypoint,
		"compat/v2-routes.json": routes,
		"migrations/index.json": migrations,
	}
	for path, contents := range files {
		artifactFiles[path] = contents
	}
	artifact := identityPackageFixtureArchive(t, artifactFiles)
	artifactDigest := sha256.Sum256(artifact)
	entrypointDigest := sha256.Sum256(entrypoint)
	migrationsDigest := sha256.Sum256(migrations)
	routesDigest := sha256.Sum256(routes)
	manifest := service.PluginManifest{
		ID: packageID, Name: name, Version: identityE2EVersion,
		APIVersion: "v2", Publisher: "AnixOps", Targets: []string{"control"},
		ArtifactSHA256: hex.EncodeToString(artifactDigest[:]),
		ControlEntrypoint: &service.PluginEntrypoint{
			Path: "bin/control-host", SHA256: hex.EncodeToString(entrypointDigest[:]),
		},
		Migrations: &service.PluginMigrations{
			Index: "migrations/index.json", SHA256: hex.EncodeToString(migrationsDigest[:]),
		},
		CompatibilityRoutes: &service.PluginCompatibilityRoutes{
			Path: "compat/v2-routes.json", SHA256: hex.EncodeToString(routesDigest[:]),
		},
		RouteContractDigest: hex.EncodeToString(routesDigest[:]),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(
		database.Get(), string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey,
	)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(database.Get(), release.ID, artifact)
	require.NoError(t, err)
	require.NoError(t, database.Get().Create(&model.PluginInstallation{
		PluginID: packageID, Target: "control", DesiredVersion: identityE2EVersion,
		ObservedVersion: identityE2EVersion, State: "healthy", Enabled: true,
		LifecycleGeneration: int64(identityE2EGeneration),
	}).Error)
}

func identityPackageFixtureFile(t testing.TB, relativePath ...string) []byte {
	t.Helper()
	return packageFixtureFile(t, identityE2EPackageID, relativePath...)
}

func packageFixtureFile(t testing.TB, packageID string, relativePath ...string) []byte {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok)
	parts := append([]string{filepath.Dir(source), "..", "..", "..", "packages", packageID}, relativePath...)
	content, err := os.ReadFile(filepath.Join(parts...))
	require.NoError(t, err)
	return content
}

func identityPackageFixtureArchive(t testing.TB, files map[string][]byte) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		file, err := writer.Create(name)
		require.NoError(t, err)
		_, err = file.Write(files[name])
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return archive.Bytes()
}
