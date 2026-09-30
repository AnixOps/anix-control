package v2

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// signedRouteFixture is an isolated SQLite database holding signed v2 control
// packages, for exercising verifiedRouteSource end to end.
type signedRouteFixture struct {
	db            *gorm.DB
	publicKey     ed25519.PublicKey
	privateKey    ed25519.PrivateKey
	encodedKey    string
	artifactReads atomic.Int64
}

func newSignedRouteFixture(tb testing.TB, onDisk bool) *signedRouteFixture {
	tb.Helper()
	dsn := ":memory:"
	if onDisk {
		dsn = filepath.Join(tb.TempDir(), "routes.db")
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(tb, err)
	sqlDB, err := db.DB()
	require.NoError(tb, err)
	// One connection keeps an in-memory database shared by every query.
	sqlDB.SetMaxOpenConns(1)
	tb.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(tb, service.EnsureKernelSchema(db))

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(tb, err)
	fixture := &signedRouteFixture{
		db: db, publicKey: publicKey, privateKey: privateKey, encodedKey: base64.StdEncoding.EncodeToString(publicKey),
	}
	artifactTable := model.PluginArtifact{}.TableName()
	require.NoError(tb, db.Callback().Query().After("gorm:query").Register("test:count_artifact_reads", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == artifactTable {
			fixture.artifactReads.Add(1)
		}
	}))
	return fixture
}

// addRelease registers and stores one signed release. payloadBytes of random
// data are added to the archive to model a realistic artifact size.
func (f *signedRouteFixture) addRelease(tb testing.TB, packageID, version, routes string, payloadBytes int) *model.PluginRelease {
	tb.Helper()
	release := f.registerRelease(tb, packageID, version, routes, payloadBytes)
	// Storing checks for an existing artifact; only resolver reads count.
	reads := f.artifactReads.Load()
	_, err := service.StorePluginArtifact(f.db, release.ID, release.artifact)
	require.NoError(tb, err)
	f.artifactReads.Store(reads)
	return release.PluginRelease
}

type registeredRelease struct {
	*model.PluginRelease
	artifact []byte
}

func (f *signedRouteFixture) registerRelease(tb testing.TB, packageID, version, routes string, payloadBytes int) registeredRelease {
	tb.Helper()
	entrypoint := []byte("#!/bin/sh\nexit 0\n")
	migrations := []byte(`{"format":"anixops.migrations/v1","migrations":[],"package_id":"` + packageID + `","version":"` + version + `"}`)
	files := []struct {
		name string
		data []byte
	}{
		{"bin/control-host", entrypoint},
		{"compat/v2-routes.json", []byte(routes)},
		{"migrations/index.json", migrations},
	}
	if payloadBytes > 0 {
		payload := make([]byte, payloadBytes)
		_, err := rand.Read(payload)
		require.NoError(tb, err)
		files = append(files, struct {
			name string
			data []byte
		}{"share/payload.bin", payload})
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range files {
		member, err := writer.CreateHeader(&zip.FileHeader{Name: file.name, Method: zip.Store})
		require.NoError(tb, err)
		_, err = member.Write(file.data)
		require.NoError(tb, err)
	}
	require.NoError(tb, writer.Close())
	artifact := buffer.Bytes()

	manifest := service.PluginManifest{
		ID: packageID, Name: packageID, Version: version, APIVersion: "v2", Publisher: "AnixOps", Targets: []string{"control"},
		ArtifactSHA256:    digestHex(artifact),
		ControlEntrypoint: &service.PluginEntrypoint{Path: "bin/control-host", SHA256: digestHex(entrypoint)},
		Migrations:        &service.PluginMigrations{Index: "migrations/index.json", SHA256: digestHex(migrations)},
		CompatibilityRoutes: &service.PluginCompatibilityRoutes{
			Path: "compat/v2-routes.json", SHA256: digestHex([]byte(routes)),
		},
		RouteContractDigest: digestHex([]byte(routes)),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(tb, err)
	release, err := service.RegisterPluginRelease(f.db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(f.privateKey, canonical)), f.publicKey)
	require.NoError(tb, err)
	return registeredRelease{PluginRelease: release, artifact: artifact}
}

func (f *signedRouteFixture) install(tb testing.TB, packageID, version string, generation int64) {
	tb.Helper()
	require.NoError(tb, f.db.Create(&model.PluginInstallation{
		PluginID: packageID, Target: "control", DesiredVersion: version, ObservedVersion: version,
		State: "healthy", Enabled: true, LifecycleGeneration: generation,
	}).Error)
}

func (f *signedRouteFixture) updateInstallation(tb testing.TB, packageID string, values map[string]any) {
	tb.Helper()
	require.NoError(tb, f.db.Model(&model.PluginInstallation{}).
		Where("plugin_id = ? AND target = ?", packageID, "control").Updates(values).Error)
}

func digestHex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func knowledgeRoutes(path, routeID string) string {
	return `{"api_version":"v2","package_id":"knowledge","routes":[{"method":"GET","legacy_path":"` + path +
		`","package_route":"` + routeID + `","envelope":"data"}]}`
}
