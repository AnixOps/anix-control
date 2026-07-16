package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

// ResolvePluginInstallationDependencyGraph loads the selected desired release
// for every dependency from the Control database, verifies the stored release
// metadata, and then delegates graph validation/order to
// ResolvePluginDependencyGraph. A dependency installation may be disabled:
// this function is a preflight graph check, not an enable operation. The
// existing ValidatePluginInstallationPlan check still enforces that every
// dependency is enabled before production intent is accepted.
//
// Registered releases carry a trust-root fingerprint and are re-verified from
// the stored trust-root row. Rows created by the pre-signature 3.x migration
// without a fingerprint retain the existing compatibility behavior and are
// checked through DecodePluginReleaseManifest.
func ResolvePluginInstallationDependencyGraph(db *gorm.DB, rootRelease model.PluginRelease, target string) (*PluginDependencyResolution, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if target != "control" && target != "agent" {
		return nil, fmt.Errorf("unsupported target %q", target)
	}

	rootManifest, err := loadDependencyGraphReleaseManifest(db, rootRelease, target)
	if err != nil {
		return nil, err
	}

	manifests := []PluginManifest{*rootManifest}
	byID := map[string]PluginManifest{rootManifest.ID: *rootManifest}
	queue := []string{rootManifest.ID}
	paths := map[string][]string{rootManifest.ID: []string{rootManifest.ID}}
	for index := 0; index < len(queue); index++ {
		pluginID := queue[index]
		manifest := byID[pluginID]
		dependencies := append([]string(nil), manifest.Dependencies...)
		sort.Strings(dependencies)
		for _, dependencyID := range dependencies {
			if _, alreadyLoaded := byID[dependencyID]; alreadyLoaded {
				continue
			}
			dependencyPath := append(append([]string(nil), paths[pluginID]...), dependencyID)
			pathText := strings.Join(dependencyPath, " -> ")

			var installation model.PluginInstallation
			err := db.Where("plugin_id = ? AND target = ?", dependencyID, target).
				Order("id").First(&installation).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, pluginDependencyGraphMissing(pathText)
			}
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(installation.DesiredVersion) == "" {
				return nil, pluginDependencyGraphMissing(pathText + " (desired version is empty)")
			}

			var release model.PluginRelease
			err = db.Where("plugin_id = ? AND version = ?", dependencyID, installation.DesiredVersion).
				First(&release).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				dependencyPath[len(dependencyPath)-1] = dependencyID + "@" + installation.DesiredVersion
				return nil, pluginDependencyGraphMissing(strings.Join(dependencyPath, " -> "))
			}
			if err != nil {
				return nil, err
			}
			dependencyManifest, err := loadDependencyGraphReleaseManifest(db, release, target)
			if err != nil {
				return nil, fmt.Errorf("load dependency %s@%s: %w", dependencyID, installation.DesiredVersion, err)
			}
			if dependencyManifest.ID != dependencyID {
				return nil, fmt.Errorf("%w: dependency release metadata is keyed as %s, manifest declares %s", ErrPluginDependencyGraphInvalid, dependencyID, dependencyManifest.ID)
			}
			byID[dependencyID] = *dependencyManifest
			paths[dependencyID] = dependencyPath
			manifests = append(manifests, *dependencyManifest)
			queue = append(queue, dependencyID)
		}
	}

	return ResolvePluginDependencyGraph(manifests, []string{rootManifest.ID})
}

// ResolvePluginDependencyGraphFromDB is a descriptive alias for callers that
// do not have an installation object yet and are resolving from a release.
func ResolvePluginDependencyGraphFromDB(db *gorm.DB, rootRelease model.PluginRelease, target string) (*PluginDependencyResolution, error) {
	return ResolvePluginInstallationDependencyGraph(db, rootRelease, target)
}

func loadDependencyGraphReleaseManifest(db *gorm.DB, release model.PluginRelease, target string) (*PluginManifest, error) {
	if err := ValidatePluginReleaseTarget(release, target); err != nil {
		return nil, err
	}
	manifest, err := DecodePluginReleaseManifest(release)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(release.TrustRootFingerprint) != "" {
		verified, verifyErr := VerifyStoredPluginRelease(db, release, nil)
		if verifyErr != nil {
			return nil, verifyErr
		}
		manifest = verified
	}
	if err := validatePluginDependencyGraphManifest(*manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func pluginDependencyGraphMissing(path string) error {
	return fmt.Errorf("%w: %w: %s", ErrPluginDependencyGraphMissing, ErrPluginDependencyUnsatisfied, path)
}
