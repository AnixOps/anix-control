package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
)

func TestBootstrapIdentityPlatformPackageImportsOnlyVerifiedColdStartPackage(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	directory := writeBootstrapIdentityPlatformPackage(t, publicKey, privateKey)

	require.NoError(t, BootstrapIdentityPlatformPackage(db, base64.StdEncoding.EncodeToString(publicKey), directory))

	var release model.PluginRelease
	require.NoError(t, db.First(&release, "plugin_id = ? AND version = ?", "identity-platform", "4.0.0").Error)
	require.Equal(t, PluginTrustRootFingerprint(publicKey), release.TrustRootFingerprint)
	artifact, err := GetPluginArtifact(db, release.ID)
	require.NoError(t, err)
	require.NotEmpty(t, artifact.Data)

	var installation model.PluginInstallation
	require.NoError(t, db.First(&installation, "plugin_id = ? AND target = ?", "identity-platform", "control").Error)
	require.Equal(t, "4.0.0", installation.DesiredVersion)
	require.True(t, installation.Enabled)
	require.Equal(t, "pending", installation.State)
	require.EqualValues(t, 1, installation.LifecycleGeneration)

	// A later restart must keep the original desired state and avoid duplicate
	// release/install records.
	require.NoError(t, BootstrapIdentityPlatformPackage(db, base64.StdEncoding.EncodeToString(publicKey), directory))
	var releaseCount, installationCount int64
	require.NoError(t, db.Model(&model.PluginRelease{}).Where("plugin_id = ?", "identity-platform").Count(&releaseCount).Error)
	require.NoError(t, db.Model(&model.PluginInstallation{}).Where("plugin_id = ? AND target = ?", "identity-platform", "control").Count(&installationCount).Error)
	require.EqualValues(t, 1, releaseCount)
	require.EqualValues(t, 1, installationCount)
}

func TestBootstrapIdentityPlatformPackageRejectsWrongRootAndPreservesExistingInstall(t *testing.T) {
	db := newKernelTestDB(t)
	trustedPublicKey, trustedPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	directory := writeBootstrapIdentityPlatformPackage(t, trustedPublicKey, trustedPrivateKey)
	wrongPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	err = BootstrapIdentityPlatformPackage(db, base64.StdEncoding.EncodeToString(wrongPublicKey), directory)
	require.Error(t, err)

	var releaseCount int64
	require.NoError(t, db.Model(&model.PluginRelease{}).Where("plugin_id = ?", "identity-platform").Count(&releaseCount).Error)
	require.Zero(t, releaseCount)

	require.NoError(t, BootstrapIdentityPlatformPackage(db, base64.StdEncoding.EncodeToString(trustedPublicKey), directory))
	var installation model.PluginInstallation
	require.NoError(t, db.First(&installation, "plugin_id = ? AND target = ?", "identity-platform", "control").Error)
	require.NoError(t, db.Model(&installation).Updates(map[string]any{
		"desired_version": "operator-selected", "enabled": false, "state": "disabled", "lifecycle_generation": 9,
	}).Error)

	require.NoError(t, BootstrapIdentityPlatformPackage(db, base64.StdEncoding.EncodeToString(trustedPublicKey), directory))
	require.NoError(t, db.First(&installation, installation.ID).Error)
	require.Equal(t, "operator-selected", installation.DesiredVersion)
	require.False(t, installation.Enabled)
	require.Equal(t, "disabled", installation.State)
	require.EqualValues(t, 9, installation.LifecycleGeneration)
}

func writeBootstrapIdentityPlatformPackage(t *testing.T, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) string {
	t.Helper()
	return writeBootstrapIdentityPlatformPackageVersion(t, privateKey, "4.0.0")
}

// writeBootstrapIdentityPlatformPackageVersion writes the bootstrap trio of
// identity-platform at version, signed by privateKey.
func writeBootstrapIdentityPlatformPackageVersion(t *testing.T, privateKey ed25519.PrivateKey, version string) string {
	t.Helper()
	signed := signedTestPackage(t, privateKey, "identity-platform", version, nil)
	directory := t.TempDir()
	stem := "identity-platform-" + version
	require.NoError(t, os.WriteFile(filepath.Join(directory, stem+".manifest.json"), []byte(signed.ManifestJSON), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(directory, stem+".manifest.sig"), []byte(signed.Signature+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(directory, stem+".anxp"), signed.Artifact, 0o644))
	return directory
}

// testSignedPackage is a v2 Control package: its canonical manifest, the
// base64 signature over it and the artifact the manifest hashes.
type testSignedPackage struct {
	Manifest     PluginManifest
	ManifestJSON string
	Signature    string
	Artifact     []byte
}

// signedTestPackage builds a minimal v2 Control package of pluginID at
// version, signed with privateKey (a test key; never the release key).
// mutate may adjust the manifest before it is signed.
func signedTestPackage(t *testing.T, privateKey ed25519.PrivateKey, pluginID, version string, mutate func(*PluginManifest)) testSignedPackage {
	t.Helper()
	entrypoint := []byte("#!/bin/sh\nexit 0\n")
	migrations := []byte(`{"format":"anixops.migrations/v1","migrations":[],"package_id":"` + pluginID + `","version":"` + version + `"}`)
	routes := []byte(`{"api_version":"v2","package_id":"` + pluginID + `","routes":[]}`)
	artifact := kernelTestV2Package(t, map[string][]byte{
		"bin/control-host":      entrypoint,
		"compat/v2-routes.json": routes,
		"migrations/index.json": migrations,
	})
	artifactDigest := sha256.Sum256(artifact)
	entrypointDigest := sha256.Sum256(entrypoint)
	migrationsDigest := sha256.Sum256(migrations)
	routesDigest := sha256.Sum256(routes)
	manifest := PluginManifest{
		ID: pluginID, Name: pluginID, Version: version, APIVersion: pluginManifestAPIVersionV2,
		Publisher: "AnixOps", Targets: []string{"control"}, ArtifactSHA256: hex.EncodeToString(artifactDigest[:]),
		ControlEntrypoint:   &PluginEntrypoint{Path: "bin/control-host", SHA256: hex.EncodeToString(entrypointDigest[:])},
		Migrations:          &PluginMigrations{Index: "migrations/index.json", SHA256: hex.EncodeToString(migrationsDigest[:])},
		CompatibilityRoutes: &PluginCompatibilityRoutes{Path: "compat/v2-routes.json", SHA256: hex.EncodeToString(routesDigest[:])},
		RouteContractDigest: hex.EncodeToString(routesDigest[:]),
	}
	if pluginID == "identity-platform" {
		manifest.Name = "Identity Platform"
	}
	if mutate != nil {
		mutate(&manifest)
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	return testSignedPackage{
		Manifest: manifest, ManifestJSON: string(canonical), Artifact: artifact,
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)),
	}
}
