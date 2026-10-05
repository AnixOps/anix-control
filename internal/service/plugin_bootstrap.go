package service

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"golang.org/x/mod/semver"
	"gorm.io/gorm"
)

const (
	identityBootstrapPackageID   = "identity-platform"
	identityBootstrapManifestExt = ".manifest.json"
	identityBootstrapArtifactExt = ".anxp"
	maxBootstrapManifestBytes    = 1 << 20
	maxBootstrapSignatureBytes   = 16 << 10
	maxBootstrapArtifactBytes    = MaxPluginArtifactBytes

	// bootstrapInstallationActor is the audit name of a change the bootstrap
	// makes on its own; bootstrapInstallationUpdateAction is its audit action.
	bootstrapInstallationActor        = "system/bootstrap"
	bootstrapInstallationUpdateAction = "bootstrap_installation_update"
)

// BootstrapIdentityPlatformPackage imports exactly one signed identity package
// before HTTP routes are exposed. This is the only package allowed through the
// unauthenticated cold-start path; every later package admission stays behind
// the authenticated kernel API.
//
// A bootstrap never changes an installation that exists, with one exception
// (moveRetiredRootIdentityInstallation): after the official signing root
// changed, an installation whose release is bound to a retired root moves to
// the bootstrap release, which has just been verified under the active root.
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
	if err := ensureBootstrapIdentityInstallation(db, *manifest, publicKey); err != nil {
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

func ensureBootstrapIdentityInstallation(db *gorm.DB, manifest PluginManifest, publicKey ed25519.PublicKey) error {
	var moved *bootstrapInstallationMove
	err := db.Transaction(func(tx *gorm.DB) error {
		moved = nil
		if err := LockPluginInstallationTarget(tx, "control"); err != nil {
			return err
		}
		var existing model.PluginInstallation
		err := tx.First(&existing, "plugin_id = ? AND target = ?", manifest.ID, "control").Error
		if err == nil {
			var moveErr error
			moved, moveErr = moveRetiredRootIdentityInstallation(tx, existing, manifest, publicKey)
			return moveErr
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
	if err != nil {
		return err
	}
	if moved != nil {
		log.Print(moved.warning())
	}
	return nil
}

// bootstrapReleaseIsNewer reports whether version is provably newer than
// installed: both must be full MAJOR.MINOR.PATCH semantic versions (the form
// of every release), and version must have the higher precedence. Anything
// else, a shorthand such as "4" or "4.2" included, is not provably newer.
func bootstrapReleaseIsNewer(installed, version string) bool {
	fullSemver := func(value string) (string, bool) {
		prefixed := "v" + value
		core := value
		if end := strings.IndexAny(core, "-+"); end >= 0 {
			core = core[:end]
		}
		return prefixed, semver.IsValid(prefixed) && strings.Count(core, ".") == 2
	}
	installedSemver, installedOK := fullSemver(installed)
	versionSemver, versionOK := fullSemver(version)
	return installedOK && versionOK && semver.Compare(versionSemver, installedSemver) > 0
}

// bootstrapInstallationMove records what moveRetiredRootIdentityInstallation
// changed, for the warning printed once the transaction committed.
type bootstrapInstallationMove struct {
	InstallationID               uint
	FromRelease, ToRelease       model.PluginRelease
	FromGeneration, ToGeneration int64
}

func (m bootstrapInstallationMove) warning() string {
	return fmt.Sprintf("WARNING: %s installation %d (target control) moved from release %s (trust root %s, retired) to the bootstrap release %s (trust root %s, active), lifecycle generation %d -> %d. "+
		"The installed release is bound to a retired official plugin trust root, so the installation could not start and nobody could sign in. "+
		"The bootstrap package under plugins.identity_bootstrap_package_dir verified under the active root. The lifecycle worker starts it on this boot. "+
		"No other package is touched: import them signed with the active root (docs/UPGRADE.md).",
		m.ToRelease.PluginID, m.InstallationID, m.FromRelease.Version, m.FromRelease.TrustRootKeyID,
		m.ToRelease.Version, m.ToRelease.TrustRootKeyID, m.FromGeneration, m.ToGeneration)
}

// moveRetiredRootIdentityInstallation is the one case where a bootstrap
// changes an installation that exists. The official signing root changed
// (docs/guide/release-root-rotation.md): the installation's release is bound
// to a retired root, so it fails closed and, because login is served by
// identity-platform, nobody can sign in. The bootstrap package has already
// been verified under the active root by BootstrapIdentityPlatformPackage,
// exactly as on a first bootstrap; this moves the installation to it.
//
// It acts only when all of these hold, and otherwise changes nothing:
//   - the installation is enabled (a disabled one is an administrator's call);
//   - its desired release exists and verifies as ErrPluginTrustRootRequired,
//     that is, it is bound to a root that is not the active one (an
//     installation on the active root is never upgraded by a bootstrap);
//   - the bootstrap release verifies under the active root, here again;
//   - its version is strictly newer by semantic version (a bootstrap package
//     never downgrades; the start goes on and says so);
//   - ValidatePluginInstallationPlan accepts it, as for an administrator's
//     update (a refusal is logged and the start goes on).
//
// The intent changes the way an administrator's update changes it: desired
// version, previous version, pending state and a new lifecycle generation. The
// kernel's lifecycle worker does the rest on this boot (restart
// reconciliation queues plugin.enable for the desired version and generation:
// host start, migration ledger run for the new generation, observed version).
// The dependency lifecycle plan of the handler is not needed: a bootstrap
// package has no dependencies, which BootstrapIdentityPlatformPackage
// enforces. The audit entry is written in the same transaction, so a move is
// never unaudited.
func moveRetiredRootIdentityInstallation(tx *gorm.DB, existing model.PluginInstallation, manifest PluginManifest, publicKey ed25519.PublicKey) (*bootstrapInstallationMove, error) {
	if !existing.Enabled || existing.DesiredVersion == manifest.Version {
		return nil, nil
	}
	var installed model.PluginRelease
	if err := tx.First(&installed, "plugin_id = ? AND version = ?", existing.PluginID, existing.DesiredVersion).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if _, err := VerifyStoredPluginRelease(tx, installed, publicKey); !errors.Is(err, ErrPluginTrustRootRequired) {
		return nil, nil
	}
	var target model.PluginRelease
	if err := tx.First(&target, "plugin_id = ? AND version = ?", manifest.ID, manifest.Version).Error; err != nil {
		return nil, err
	}
	if _, err := VerifyStoredPluginRelease(tx, target, publicKey); err != nil {
		return nil, fmt.Errorf("verify identity bootstrap release under the active trust root: %w", err)
	}

	if !bootstrapReleaseIsNewer(existing.DesiredVersion, manifest.Version) {
		log.Printf("WARNING: %s installation %d stays on release %s (trust root %s, retired): the bootstrap release %s (trust root %s, active) is not newer, "+
			"so the installation is not moved (a bootstrap package never downgrades). Nobody can sign in until an administrator moves the installation with a session token "+
			"issued before the restart, or a newer %s package signed with the active root is placed in plugins.identity_bootstrap_package_dir (docs/guide/release-root-rotation.md).",
			existing.PluginID, existing.ID, installed.Version, installed.TrustRootKeyID, target.Version, target.TrustRootKeyID, existing.PluginID)
		return nil, nil
	}
	if err := ValidatePluginInstallationPlan(tx, target, "control", true, existing.ID); err != nil {
		log.Printf("WARNING: %s installation %d stays on release %s (trust root %s, retired): the installation is not moved to the bootstrap release %s (trust root %s, active) "+
			"because validating it failed: %v. Nobody can sign in until an administrator resolves this and moves the installation (docs/guide/release-root-rotation.md).",
			existing.PluginID, existing.ID, installed.Version, installed.TrustRootKeyID, target.Version, target.TrustRootKeyID, err)
		return nil, nil
	}

	move := &bootstrapInstallationMove{
		InstallationID: existing.ID, FromRelease: installed, ToRelease: target, FromGeneration: existing.LifecycleGeneration,
	}
	previousVersion := existing.ObservedVersion
	if previousVersion == "" {
		previousVersion = existing.DesiredVersion
	}
	existing.DesiredVersion = target.Version
	if previousVersion != target.Version {
		existing.PreviousVersion = previousVersion
	}
	existing.Enabled, existing.State, existing.DisabledAt = true, "pending", nil
	existing.LifecycleGeneration++
	if existing.LifecycleGeneration <= 0 {
		existing.LifecycleGeneration = 1
	}
	move.ToGeneration = existing.LifecycleGeneration
	if err := tx.Save(&existing).Error; err != nil {
		return nil, err
	}

	content, err := json.Marshal(map[string]any{
		"plugin_id": existing.PluginID, "target": existing.Target, "source": "plugins.identity_bootstrap_package_dir",
		"from_version": installed.Version, "to_version": target.Version,
		"from_trust_root_fingerprint": installed.TrustRootFingerprint, "from_trust_root_key_id": installed.TrustRootKeyID,
		"to_trust_root_fingerprint": target.TrustRootFingerprint, "to_trust_root_key_id": target.TrustRootKeyID,
		"from_lifecycle_generation": move.FromGeneration, "to_lifecycle_generation": move.ToGeneration,
	})
	if err != nil {
		return nil, err
	}
	installationID := existing.ID
	if err := NewOperationLogService(tx).Record(&OperationLogInput{
		Username: bootstrapInstallationActor, Action: bootstrapInstallationUpdateAction, Module: "kernel",
		TargetType: "plugin_installation", TargetID: &installationID, Content: string(content), Status: 1,
	}); err != nil {
		return nil, fmt.Errorf("audit identity bootstrap installation update: %w", err)
	}
	return move, nil
}
