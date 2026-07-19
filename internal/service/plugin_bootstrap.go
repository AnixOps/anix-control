package service

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

const (
	identityBootstrapPackageID   = "identity-platform"
	identityBootstrapManifestExt = ".manifest.json"
	identityBootstrapArtifactExt = ".anxp"
	maxBootstrapManifestBytes    = 1 << 20
	maxBootstrapSignatureBytes   = 16 << 10
	maxBootstrapArtifactBytes    = MaxPluginArtifactBytes
)

// BootstrapIdentityPlatformPackage imports exactly one signed identity package
// before HTTP routes are exposed. This is the only package allowed through the
// unauthenticated cold-start path; every later package admission stays behind
// the authenticated kernel API.
func BootstrapIdentityPlatformPackage(db *gorm.DB, encodedPublicKey, directory string) error {
	if strings.TrimSpace(directory) == "" {
		return nil
	}
	if db == nil {
		return errors.New("identity bootstrap requires an initialized database")
	}
	publicKey, err := ParseOfficialPluginPublicKey(encodedPublicKey)
	if err != nil {
		return fmt.Errorf("parse identity bootstrap trust root: %w", err)
	}
	root, err := openIdentityBootstrapRoot(directory)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	manifestName, version, err := identityBootstrapManifestName(root)
	if err != nil {
		return err
	}
	stem := identityBootstrapPackageID + "-" + version
	manifestBytes, err := readIdentityBootstrapFile(root, manifestName, maxBootstrapManifestBytes)
	if err != nil {
		return err
	}
	signatureBytes, err := readIdentityBootstrapFile(root, stem+".manifest.sig", maxBootstrapSignatureBytes)
	if err != nil {
		return err
	}
	artifact, err := readIdentityBootstrapFile(root, stem+identityBootstrapArtifactExt, maxBootstrapArtifactBytes)
	if err != nil {
		return err
	}

	manifest, err := VerifyPluginRelease(string(manifestBytes), strings.TrimSpace(string(signatureBytes)), publicKey)
	if err != nil {
		return fmt.Errorf("verify identity bootstrap package: %w", err)
	}
	if manifest.ID != identityBootstrapPackageID || manifest.Version != version {
		return errors.New("identity bootstrap manifest does not match its file name")
	}
	if manifest.APIVersion != pluginManifestAPIVersionV2 || !manifestSupportsTarget(*manifest, "control") {
		return errors.New("identity bootstrap package must be a v2 Control package")
	}
	if len(manifest.Dependencies) != 0 {
		return errors.New("identity bootstrap package must not depend on another package")
	}
	if err := VerifyPluginArtifact(*manifest, artifact); err != nil {
		return fmt.Errorf("verify identity bootstrap artifact: %w", err)
	}

	release, err := ensureBootstrapIdentityRelease(db, *manifest, string(manifestBytes), strings.TrimSpace(string(signatureBytes)), publicKey)
	if err != nil {
		return err
	}
	if _, err := StorePluginArtifact(db, release.ID, artifact); err != nil {
		return fmt.Errorf("store identity bootstrap artifact: %w", err)
	}
	if err := ensureBootstrapIdentityInstallation(db, *manifest); err != nil {
		return err
	}
	return nil
}

func openIdentityBootstrapRoot(directory string) (*os.Root, error) {
	rootPath := filepath.Clean(strings.TrimSpace(directory))
	if !filepath.IsAbs(rootPath) {
		return nil, errors.New("plugins.identity_bootstrap_package_dir must be absolute")
	}
	info, err := os.Lstat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("inspect identity bootstrap directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("identity bootstrap package directory must be a directory, not a symlink")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return nil, errors.New("identity bootstrap package directory must not be writable by group or others")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, fmt.Errorf("open identity bootstrap directory: %w", err)
	}
	return root, nil
}

func identityBootstrapManifestName(root *os.Root) (string, string, error) {
	if root == nil {
		return "", "", errors.New("identity bootstrap directory is unavailable")
	}
	directory, err := root.Open(".")
	if err != nil {
		return "", "", fmt.Errorf("read identity bootstrap directory: %w", err)
	}
	defer func() { _ = directory.Close() }()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return "", "", fmt.Errorf("list identity bootstrap directory: %w", err)
	}
	prefix := identityBootstrapPackageID + "-"
	manifestName := ""
	version := ""
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, identityBootstrapManifestExt) {
			continue
		}
		candidateVersion := strings.TrimSuffix(strings.TrimPrefix(name, prefix), identityBootstrapManifestExt)
		if !safePluginSegment(candidateVersion) {
			return "", "", errors.New("identity bootstrap manifest file name has an invalid version")
		}
		if manifestName != "" {
			return "", "", errors.New("identity bootstrap directory contains multiple identity manifests")
		}
		manifestName, version = name, candidateVersion
	}
	if manifestName == "" {
		return "", "", errors.New("identity bootstrap package manifest is missing")
	}
	return manifestName, version, nil
}

func readIdentityBootstrapFile(root *os.Root, name string, maximum int64) ([]byte, error) {
	if root == nil || name == "" || maximum <= 0 {
		return nil, errors.New("identity bootstrap file request is invalid")
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("inspect identity bootstrap file %q: %w", name, err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return nil, fmt.Errorf("identity bootstrap file %q must be regular", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open identity bootstrap file %q: %w", name, err)
	}
	defer func() { _ = file.Close() }()
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, fmt.Errorf("identity bootstrap file %q changed before open", name)
	}
	contents, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, fmt.Errorf("read identity bootstrap file %q: %w", name, err)
	}
	if int64(len(contents)) > maximum {
		return nil, fmt.Errorf("identity bootstrap file %q exceeds its maximum size", name)
	}
	return contents, nil
}

func ensureBootstrapIdentityRelease(db *gorm.DB, manifest PluginManifest, manifestJSON, signature string, publicKey ed25519.PublicKey) (*model.PluginRelease, error) {
	var existing model.PluginRelease
	err := db.First(&existing, "plugin_id = ? AND version = ?", manifest.ID, manifest.Version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		release, registerErr := RegisterPluginRelease(db, manifestJSON, signature, publicKey)
		if registerErr != nil {
			return nil, fmt.Errorf("register identity bootstrap release: %w", registerErr)
		}
		return release, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load identity bootstrap release: %w", err)
	}
	if existing.TrustRootFingerprint != PluginTrustRootFingerprint(publicKey) {
		return nil, errors.New("existing identity release uses a different trust root")
	}
	canonical, err := CanonicalPluginManifest(manifest)
	if err != nil {
		return nil, err
	}
	if existing.ManifestJSON != string(canonical) || strings.TrimSpace(existing.Signature) != strings.TrimSpace(signature) {
		return nil, errors.New("existing identity release does not match the bootstrap package")
	}
	if _, err := VerifyStoredPluginRelease(db, existing, publicKey); err != nil {
		return nil, fmt.Errorf("verify existing identity bootstrap release: %w", err)
	}
	return &existing, nil
}

func ensureBootstrapIdentityInstallation(db *gorm.DB, manifest PluginManifest) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := LockPluginInstallationTarget(tx, "control"); err != nil {
			return err
		}
		var existing model.PluginInstallation
		err := tx.First(&existing, "plugin_id = ? AND target = ?", manifest.ID, "control").Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var release model.PluginRelease
		if err := tx.First(&release, "plugin_id = ? AND version = ?", manifest.ID, manifest.Version).Error; err != nil {
			return err
		}
		if err := ValidatePluginInstallationPlan(tx, release, "control", true, 0); err != nil {
			return fmt.Errorf("validate identity bootstrap installation: %w", err)
		}
		installation := model.PluginInstallation{
			PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version,
			State: "pending", Enabled: true, LifecycleGeneration: 1,
		}
		if err := tx.Create(&installation).Error; err != nil {
			return err
		}
		return nil
	})
}
