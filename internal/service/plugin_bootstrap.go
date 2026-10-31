package service

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/builtinpkg"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"golang.org/x/mod/semver"
	"gorm.io/gorm"
)

// Built-in packages: the official packages embedded in the Control binary
// (internal/builtinpkg) are verified, registered, installed or upgraded and
// enabled at every start, before HTTP routes are exposed. The lifecycle
// worker then starts their hosts (restart reconciliation), running package
// migrations when the version changed. Every failure stays with its package:
// the package is reported (its result, its installation's state and a
// kernel alert) and Control starts anyway.
// docs/architecture/plugin-kernel-contract.md, "Built-in Packages".

const (
	// bootstrapInstallationActor is the audit name of a change the bootstrap
	// makes on its own; builtinInstallationAction is its audit action.
	bootstrapInstallationActor = "system/bootstrap"
	builtinInstallationAction  = "builtin_package_installation"
)

// Results of BootstrapBuiltinPackages for one package.
const (
	// BuiltinActionInstalled: a new installation, enabled.
	BuiltinActionInstalled = "installed"
	// BuiltinActionUpgraded: the installation moved to the built-in version.
	BuiltinActionUpgraded = "upgraded"
	// BuiltinActionEnabled: a disabled installation of the built-in version
	// was enabled again.
	BuiltinActionEnabled = "enabled"
	// BuiltinActionUnchanged: already enabled at the built-in version (a
	// same-version restart).
	BuiltinActionUnchanged = "unchanged"
	// BuiltinActionInvalid: the embedded package did not verify (signature,
	// digest, contents); nothing was written.
	BuiltinActionInvalid = "invalid"
	// BuiltinActionFailed: it verified but could not be registered or
	// installed, or a package it depends on failed.
	BuiltinActionFailed = "failed"
)

// BuiltinPackageResult is what the start-up bootstrap did with one package.
type BuiltinPackageResult struct {
	ID      string
	Version string
	Action  string
	Err     error
}

// builtinPackageRank orders packages without a dependency between them:
// identity first (logins), then the platform package, then by id.
func builtinPackageRank(id string) int {
	switch id {
	case "identity-platform":
		return 0
	case "platform":
		return 1
	default:
		return 2
	}
}

// BootstrapBuiltinPackages installs, upgrades and enables every package of
// set, in dependency order. It is idempotent: a restart with the same
// binary changes nothing. It never returns an error for a package: the
// results say what happened to each, and the caller logs them.
func BootstrapBuiltinPackages(db *gorm.DB, encodedPublicKey string, set *builtinpkg.Set) []BuiltinPackageResult {
	if set.Empty() {
		return nil
	}
	results := make(map[string]*BuiltinPackageResult, len(set.Packages))
	for _, embedded := range set.Packages {
		results[embedded.ID] = &BuiltinPackageResult{ID: embedded.ID, Version: embedded.Version}
	}
	publicKey, keyErr := ParseOfficialPluginPublicKey(encodedPublicKey)
	if keyErr == nil && db == nil {
		keyErr = errors.New("built-in packages need an initialized database")
	}
	manifests := map[string]*PluginManifest{}
	for _, embedded := range set.Packages {
		result := results[embedded.ID]
		if keyErr != nil {
			result.Action, result.Err = BuiltinActionInvalid, fmt.Errorf("verify with plugins.official_public_key: %w", keyErr)
			continue
		}
		manifest, err := VerifyBuiltinPackage(embedded, publicKey)
		if err != nil {
			result.Action, result.Err = BuiltinActionInvalid, err
			continue
		}
		manifests[embedded.ID] = manifest
	}
	if db != nil {
		retireBuiltinPackages(db)
	}
	order, orderErrs := orderBuiltinPackages(manifests)
	for id, err := range orderErrs {
		results[id].Action, results[id].Err = BuiltinActionFailed, err
	}
	for _, id := range order {
		result := results[id]
		manifest := manifests[id]
		if failed := failedBuiltinDependency(*manifest, results); failed != "" {
			result.Action, result.Err = BuiltinActionFailed, fmt.Errorf("it depends on %s, which failed", failed)
			continue
		}
		embedded, _ := set.Lookup(id)
		action, err := installBuiltinPackage(db, embedded, *manifest, publicKey)
		if err != nil {
			result.Action, result.Err = BuiltinActionFailed, err
			// An installation that wants a newer release is left as it is:
			// it may run that release (its host starts or fails on its own).
			if !errors.Is(err, errBuiltinOlderThanInstalled) {
				markBuiltinInstallationFailed(db, id, err)
			}
			continue
		}
		result.Action = action
	}
	out := make([]BuiltinPackageResult, 0, len(results))
	for _, id := range append(order, sortedKeys(orderErrs, results)...) {
		out = append(out, *results[id])
	}
	return out
}

// retiredBuiltinPackages are official packages a release removed. An
// enabled installation left behind by an older Control names capabilities
// this binary no longer knows, and installation validation reads every
// enabled installation: one such row fails every built-in package.
var retiredBuiltinPackages = []string{"gost-mesh", "nftables-forward", "nat-egress"}

// retireBuiltinPackages disables the installations of retired packages,
// once, with an audit entry; their rows and releases stay for the record.
func retireBuiltinPackages(db *gorm.DB) {
	for _, id := range retiredBuiltinPackages {
		_ = db.Transaction(func(tx *gorm.DB) error {
			var installation model.PluginInstallation
			if err := tx.First(&installation, "plugin_id = ? AND target = ? AND enabled = ?", id, "control", true).Error; err != nil {
				return err
			}
			now := time.Now()
			fromGeneration := installation.LifecycleGeneration
			installation.Enabled, installation.State, installation.DisabledAt = false, "disabled", &now
			installation.LastError = "retired: this release of Control no longer ships the package"
			installation.LifecycleGeneration++
			if err := tx.Save(&installation).Error; err != nil {
				return err
			}
			return recordBuiltinInstallation(tx, installation, installation.DesiredVersion, installation.DesiredVersion, fromGeneration, "retired")
		})
	}
}

// sortedKeys lists the packages that were not in the install order: those
// that did not verify or could not be ordered.
func sortedKeys(orderErrs map[string]error, results map[string]*BuiltinPackageResult) []string {
	var ids []string
	for id, result := range results {
		if result.Action == BuiltinActionInvalid || orderErrs[id] != nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func failedBuiltinDependency(manifest PluginManifest, results map[string]*BuiltinPackageResult) string {
	for _, dependency := range manifest.Dependencies {
		if result, ok := results[dependency]; ok && (result.Action == BuiltinActionInvalid || result.Action == BuiltinActionFailed) {
			return dependency
		}
	}
	return ""
}

// VerifyBuiltinPackage checks one embedded package against the official key
// before anything is written: the manifest signature, that the manifest is
// canonical and names this package and version, a v2 Control package whose
// host is built in, and the artifact with its migration index and
// compatibility routes.
func VerifyBuiltinPackage(embedded builtinpkg.Package, publicKey ed25519.PublicKey) (*PluginManifest, error) {
	manifest, err := VerifyPluginRelease(string(embedded.Manifest), embedded.Signature, publicKey)
	if err != nil {
		return nil, fmt.Errorf("verify the manifest: %w", err)
	}
	if manifest.ID != embedded.ID || manifest.Version != embedded.Version {
		return nil, fmt.Errorf("the manifest is %s %s, the index says %s %s", manifest.ID, manifest.Version, embedded.ID, embedded.Version)
	}
	canonical, err := CanonicalPluginManifest(*manifest)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(canonical, embedded.Manifest) {
		return nil, errors.New("the embedded manifest is not canonical")
	}
	if manifest.APIVersion != pluginManifestAPIVersionV2 || !manifestSupportsTarget(*manifest, "control") || !manifest.ControlEntrypoint.BuiltinHost() {
		return nil, errors.New("the manifest is not a v2 Control package with a built-in host")
	}
	if err := VerifyPluginArtifact(*manifest, embedded.Artifact); err != nil {
		return nil, fmt.Errorf("verify the artifact: %w", err)
	}
	if _, err := ParsePluginMigrationIndex(*manifest, embedded.Artifact); err != nil {
		return nil, fmt.Errorf("verify the migration index: %w", err)
	}
	routes, err := extractPluginArtifactFile(embedded.Artifact, manifest.CompatibilityRoutes.Path, maxPluginControlEntrypointBytes)
	if err != nil {
		return nil, fmt.Errorf("extract the compatibility routes: %w", err)
	}
	if !strings.EqualFold(sha256Bytes(routes), manifest.CompatibilityRoutes.SHA256) || !strings.EqualFold(sha256Bytes(routes), manifest.RouteContractDigest) {
		return nil, errors.New("the compatibility routes do not match the manifest")
	}
	return manifest, nil
}

// orderBuiltinPackages sorts the verified packages so that a package comes
// after the packages it depends on; packages without a dependency between
// them keep builtinPackageRank and then id order. A package whose
// dependency is not among them, or that is part of a cycle, fails.
func orderBuiltinPackages(manifests map[string]*PluginManifest) ([]string, map[string]error) {
	failed := map[string]error{}
	pending := map[string]map[string]bool{}
	for id, manifest := range manifests {
		pending[id] = map[string]bool{}
		for _, dependency := range manifest.Dependencies {
			pending[id][dependency] = true
		}
	}
	// A dependency that is not embedded (or did not verify) fails its
	// dependents, transitively.
	for changed := true; changed; {
		changed = false
		for id, dependencies := range pending {
			for dependency := range dependencies {
				if _, ok := pending[dependency]; !ok {
					if failed[dependency] == nil && manifests[dependency] == nil {
						failed[id] = fmt.Errorf("it depends on %s, which this binary does not carry or which did not verify", dependency)
					} else {
						failed[id] = fmt.Errorf("it depends on %s, which failed", dependency)
					}
					delete(pending, id)
					changed = true
					break
				}
			}
		}
	}
	var order []string
	for len(pending) > 0 {
		var ready []string
		for id, dependencies := range pending {
			if len(dependencies) == 0 {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			for id := range pending {
				failed[id] = errors.New("its dependencies form a cycle")
			}
			break
		}
		sort.Slice(ready, func(i, j int) bool {
			left, right := builtinPackageRank(ready[i]), builtinPackageRank(ready[j])
			if left != right {
				return left < right
			}
			return ready[i] < ready[j]
		})
		next := ready[0]
		order = append(order, next)
		delete(pending, next)
		for _, dependencies := range pending {
			delete(dependencies, next)
		}
	}
	return order, failed
}

func installBuiltinPackage(db *gorm.DB, embedded builtinpkg.Package, manifest PluginManifest, publicKey ed25519.PublicKey) (string, error) {
	release, err := ensureBuiltinRelease(db, manifest, embedded, publicKey)
	if err != nil {
		return "", err
	}
	if _, err := StorePluginArtifact(db, release.ID, embedded.Artifact); err != nil {
		return "", fmt.Errorf("store the artifact: %w", err)
	}
	return ensureBuiltinInstallation(db, manifest, *release)
}

func ensureBuiltinRelease(db *gorm.DB, manifest PluginManifest, embedded builtinpkg.Package, publicKey ed25519.PublicKey) (*model.PluginRelease, error) {
	var existing model.PluginRelease
	err := db.First(&existing, "plugin_id = ? AND version = ?", manifest.ID, manifest.Version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		release, registerErr := RegisterPluginRelease(db, string(embedded.Manifest), embedded.Signature, publicKey)
		if registerErr != nil {
			return nil, fmt.Errorf("register the release: %w", registerErr)
		}
		return release, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load the release: %w", err)
	}
	if existing.TrustRootFingerprint != PluginTrustRootFingerprint(publicKey) {
		return nil, fmt.Errorf("release %s %s is already registered under another trust root (key %s)", manifest.ID, manifest.Version, existing.TrustRootKeyID)
	}
	if existing.ManifestJSON != string(embedded.Manifest) || strings.TrimSpace(existing.Signature) != strings.TrimSpace(embedded.Signature) {
		return nil, fmt.Errorf("release %s %s is already registered with other contents", manifest.ID, manifest.Version)
	}
	if _, err := VerifyStoredPluginRelease(db, existing, publicKey); err != nil {
		return nil, fmt.Errorf("verify the registered release: %w", err)
	}
	return &existing, nil
}

// errBuiltinOlderThanInstalled refuses to move an installation to an older
// version than it wants.
var errBuiltinOlderThanInstalled = errors.New("the binary carries an older release than the installation")

// ensureBuiltinInstallation makes the package's Control installation enabled
// at the built-in version. A new installation starts at lifecycle
// generation 1; any change to an existing one is made the way an
// administrator's update makes it (desired and previous version, pending
// state, a new lifecycle generation) and is audited in the same
// transaction. A same-version restart changes nothing. The installation
// never moves to an older version than it wants: that is an older binary
// on a newer database, and the package fails instead.
func ensureBuiltinInstallation(db *gorm.DB, manifest PluginManifest, release model.PluginRelease) (string, error) {
	action := ""
	err := db.Transaction(func(tx *gorm.DB) error {
		action = ""
		if err := LockPluginInstallationTarget(tx, "control"); err != nil {
			return err
		}
		var existing model.PluginInstallation
		err := tx.First(&existing, "plugin_id = ? AND target = ?", manifest.ID, "control").Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := ValidatePluginInstallationPlan(tx, release, "control", true, 0); err != nil {
				return fmt.Errorf("validate the installation: %w", err)
			}
			installation := model.PluginInstallation{
				PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version,
				State: "pending", Enabled: true, LifecycleGeneration: 1,
			}
			if err := tx.Create(&installation).Error; err != nil {
				return err
			}
			action = BuiltinActionInstalled
			return recordBuiltinInstallation(tx, installation, "", manifest.Version, 0, action)
		}
		if err != nil {
			return err
		}
		if existing.Enabled && existing.DesiredVersion == manifest.Version {
			action = BuiltinActionUnchanged
			return nil
		}
		if existing.DesiredVersion != manifest.Version && bootstrapReleaseIsNewer(manifest.Version, existing.DesiredVersion) {
			return fmt.Errorf("%w: the installation wants %s, newer than the %s this binary carries; run %s or newer, or restore the backup taken before the upgrade", errBuiltinOlderThanInstalled, existing.DesiredVersion, manifest.Version, existing.DesiredVersion)
		}
		if err := ValidatePluginInstallationPlan(tx, release, "control", true, existing.ID); err != nil {
			return fmt.Errorf("validate the installation: %w", err)
		}
		fromVersion, fromGeneration := existing.DesiredVersion, existing.LifecycleGeneration
		previousVersion := existing.ObservedVersion
		if previousVersion == "" {
			previousVersion = existing.DesiredVersion
		}
		action = BuiltinActionEnabled
		if existing.DesiredVersion != manifest.Version {
			action = BuiltinActionUpgraded
			existing.DesiredVersion = manifest.Version
			if previousVersion != manifest.Version {
				existing.PreviousVersion = previousVersion
			}
		}
		existing.Enabled, existing.State, existing.DisabledAt, existing.LastError = true, "pending", nil, ""
		existing.LifecycleGeneration++
		if existing.LifecycleGeneration <= 0 {
			existing.LifecycleGeneration = 1
		}
		if err := tx.Save(&existing).Error; err != nil {
			return err
		}
		return recordBuiltinInstallation(tx, existing, fromVersion, manifest.Version, fromGeneration, action)
	})
	if err != nil {
		return "", err
	}
	return action, nil
}

func recordBuiltinInstallation(tx *gorm.DB, installation model.PluginInstallation, fromVersion, toVersion string, fromGeneration int64, action string) error {
	content, err := json.Marshal(map[string]any{
		"plugin_id": installation.PluginID, "target": installation.Target, "source": "built-in",
		"action": action, "from_version": fromVersion, "to_version": toVersion,
		"from_lifecycle_generation": fromGeneration, "to_lifecycle_generation": installation.LifecycleGeneration,
	})
	if err != nil {
		return err
	}
	installationID := installation.ID
	if err := NewOperationLogService(tx).Record(&OperationLogInput{
		Username: bootstrapInstallationActor, Action: builtinInstallationAction, Module: "kernel",
		TargetType: "plugin_installation", TargetID: &installationID, Content: string(content), Status: 1,
	}); err != nil {
		return fmt.Errorf("audit the installation: %w", err)
	}
	return nil
}

// markBuiltinInstallationFailed records a bootstrap failure on the package's
// existing installation, so the plugin center and the alert scan show it.
// A package without an installation is reported by the alert scan alone.
func markBuiltinInstallationFailed(db *gorm.DB, id string, cause error) {
	if db == nil || cause == nil {
		return
	}
	message := strings.TrimSpace("built-in package bootstrap: " + cause.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}
	_ = db.Model(&model.PluginInstallation{}).
		Where("plugin_id = ? AND target = ?", id, "control").
		Updates(map[string]any{"state": "failed", "last_error": message}).Error
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

// BuiltinPackageStatus is a built-in package that needs an administrator,
// as the alert scan sees it (internal/kernelalerts).
type BuiltinPackageStatus struct {
	ID      string
	Version string
	// Invalid: the embedded package did not verify.
	Invalid bool
	// State and LastError are its installation's, when it has one.
	State     string
	LastError string
	Reason    string
}

// BuiltinPackageProblems re-checks the embedded packages and their
// installations: a package that does not verify, one without an enabled
// installation at the built-in version, and one whose installation failed.
// It writes nothing. An installation that is still pending (its host is
// starting) is not a problem.
func BuiltinPackageProblems(db *gorm.DB, encodedPublicKey string, set *builtinpkg.Set, setErr error) ([]BuiltinPackageStatus, error) {
	if setErr != nil {
		return []BuiltinPackageStatus{{ID: "index", Invalid: true, Reason: setErr.Error()}}, nil
	}
	if set.Empty() {
		return nil, nil
	}
	publicKey, keyErr := ParseOfficialPluginPublicKey(encodedPublicKey)
	var problems []BuiltinPackageStatus
	for _, embedded := range set.Packages {
		if keyErr != nil {
			problems = append(problems, BuiltinPackageStatus{ID: embedded.ID, Version: embedded.Version, Invalid: true, Reason: keyErr.Error()})
			continue
		}
		if _, err := VerifyBuiltinPackage(embedded, publicKey); err != nil {
			problems = append(problems, BuiltinPackageStatus{ID: embedded.ID, Version: embedded.Version, Invalid: true, Reason: err.Error()})
			continue
		}
		var installations []model.PluginInstallation
		if err := db.Where("plugin_id = ? AND target = ?", embedded.ID, "control").Limit(1).Find(&installations).Error; err != nil {
			return nil, err
		}
		status := BuiltinPackageStatus{ID: embedded.ID, Version: embedded.Version}
		switch {
		case len(installations) == 0:
			status.Reason = "it is not installed: the start-up bootstrap failed (see the Control log)"
		case installations[0].State == "failed":
			status.State, status.LastError = installations[0].State, installations[0].LastError
			status.Reason = "its installation failed"
		case !installations[0].Enabled || installations[0].DesiredVersion != embedded.Version:
			status.State, status.LastError = installations[0].State, installations[0].LastError
			status.Reason = fmt.Sprintf("its installation is not enabled at %s (wants %s, enabled %t)", embedded.Version, installations[0].DesiredVersion, installations[0].Enabled)
		default:
			continue
		}
		problems = append(problems, status)
	}
	return problems, nil
}
