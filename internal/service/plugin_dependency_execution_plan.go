package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

// PluginDependencyExecutionStep binds one graph vertex to the immutable
// release that an execution plan will use. Installation is nil when the
// package has not been installed on the target yet.
type PluginDependencyExecutionStep struct {
	PluginID     string                    `json:"plugin_id"`
	Release      model.PluginRelease       `json:"release"`
	Manifest     PluginManifest            `json:"manifest"`
	Installation *model.PluginInstallation `json:"installation,omitempty"`
}

// PluginDependencyExecutionPlan is the dependency-first closure used by the
// Control lifecycle worker. Unlike the compatibility resolver, it can select
// the latest registered target-compatible release for a missing dependency so
// an administrator can enable one root package without first hand-installing
// every package in its closure.
//
// An existing installation always pins its desired version. That makes an
// upgrade deterministic and prevents a newly published dependency release
// from silently changing an already configured package.
type PluginDependencyExecutionPlan struct {
	Target string                          `json:"target"`
	RootID string                          `json:"root_id"`
	Order  []string                        `json:"order"`
	Steps  []PluginDependencyExecutionStep `json:"steps"`
}

// ResolvePluginDependencyExecutionPlan resolves a root release and all of its
// transitive dependencies into a stable dependency-first execution plan. It
// validates the selected stored manifests, including registered trust roots,
// but does not mutate installation intent.
func ResolvePluginDependencyExecutionPlan(db *gorm.DB, rootRelease model.PluginRelease, target string) (*PluginDependencyExecutionPlan, error) {
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
	selected := map[string]PluginDependencyExecutionStep{
		rootManifest.ID: {PluginID: rootManifest.ID, Release: rootRelease, Manifest: *rootManifest},
	}
	queue := []string{rootManifest.ID}
	paths := map[string][]string{rootManifest.ID: {rootManifest.ID}}

	for index := 0; index < len(queue); index++ {
		pluginID := queue[index]
		step := selected[pluginID]
		dependencies := append([]string(nil), step.Manifest.Dependencies...)
		sort.Strings(dependencies)
		for _, dependencyID := range dependencies {
			if _, exists := selected[dependencyID]; exists {
				continue
			}
			dependencyPath := append(append([]string(nil), paths[pluginID]...), dependencyID)
			selection, err := selectPluginDependencyExecutionRelease(db, dependencyID, target)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, pluginDependencyGraphMissing(strings.Join(dependencyPath, " -> "))
				}
				return nil, fmt.Errorf("load dependency %s: %w", strings.Join(dependencyPath, " -> "), err)
			}
			selected[dependencyID] = selection
			paths[dependencyID] = dependencyPath
			queue = append(queue, dependencyID)
		}
	}

	manifests := make([]PluginManifest, 0, len(selected))
	for _, step := range selected {
		manifests = append(manifests, step.Manifest)
	}
	resolution, err := ResolvePluginDependencyGraph(manifests, []string{rootManifest.ID})
	if err != nil {
		return nil, err
	}
	steps := make([]PluginDependencyExecutionStep, 0, len(resolution.Order))
	for _, pluginID := range resolution.Order {
		steps = append(steps, selected[pluginID])
	}
	return &PluginDependencyExecutionPlan{
		Target: target, RootID: rootManifest.ID, Order: append([]string(nil), resolution.Order...), Steps: steps,
	}, nil
}

func selectPluginDependencyExecutionRelease(db *gorm.DB, pluginID, target string) (PluginDependencyExecutionStep, error) {
	var installation model.PluginInstallation
	err := db.Where("plugin_id = ? AND target = ?", pluginID, target).Order("id").First(&installation).Error
	if err == nil {
		if strings.TrimSpace(installation.DesiredVersion) == "" {
			return PluginDependencyExecutionStep{}, pluginDependencyGraphMissing(pluginID + " (desired version is empty)")
		}
		var release model.PluginRelease
		if err := db.Where("plugin_id = ? AND version = ?", pluginID, installation.DesiredVersion).First(&release).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return PluginDependencyExecutionStep{}, pluginDependencyGraphMissing(pluginID + "@" + installation.DesiredVersion)
			}
			return PluginDependencyExecutionStep{}, err
		}
		manifest, err := loadDependencyGraphReleaseManifest(db, release, target)
		if err != nil {
			return PluginDependencyExecutionStep{}, err
		}
		if manifest.ID != pluginID {
			return PluginDependencyExecutionStep{}, fmt.Errorf("%w: dependency release metadata is keyed as %s, manifest declares %s", ErrPluginDependencyGraphInvalid, pluginID, manifest.ID)
		}
		return PluginDependencyExecutionStep{PluginID: pluginID, Release: release, Manifest: *manifest, Installation: &installation}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return PluginDependencyExecutionStep{}, err
	}

	var plugin model.Plugin
	if err := db.First(&plugin, "id = ? AND official = ? AND publisher = ?", pluginID, true, "AnixOps").Error; err != nil {
		return PluginDependencyExecutionStep{}, err
	}
	var releases []model.PluginRelease
	if err := db.Where("plugin_id = ?", pluginID).Order("published_at DESC, id DESC").Find(&releases).Error; err != nil {
		return PluginDependencyExecutionStep{}, err
	}
	if len(releases) == 0 {
		return PluginDependencyExecutionStep{}, gorm.ErrRecordNotFound
	}
	for _, release := range releases {
		manifest, manifestErr := loadDependencyGraphReleaseManifest(db, release, target)
		if manifestErr == nil {
			return PluginDependencyExecutionStep{PluginID: pluginID, Release: release, Manifest: *manifest}, nil
		}
		// A release for another target is not a viable candidate. Metadata or
		// signature errors remain fail-closed rather than silently downgrading.
		if strings.Contains(manifestErr.Error(), "does not support "+target+" target") {
			continue
		}
		return PluginDependencyExecutionStep{}, manifestErr
	}
	return PluginDependencyExecutionStep{}, gorm.ErrRecordNotFound
}

// ValidatePluginDependencyExecutionPlan verifies every selected artifact and
// validates conflicts against already-enabled packages outside the closure.
// Callers hold PluginTargetLock while invoking it and applying the plan.
func ValidatePluginDependencyExecutionPlan(db *gorm.DB, plan *PluginDependencyExecutionPlan) error {
	if db == nil {
		return errors.New("database is not initialized")
	}
	if plan == nil || len(plan.Steps) == 0 {
		return ErrPluginDependencyGraphEmpty
	}
	if plan.Target != "control" && plan.Target != "agent" {
		return fmt.Errorf("unsupported target %q", plan.Target)
	}

	selected := make(map[string]PluginManifest, len(plan.Steps))
	for _, step := range plan.Steps {
		if step.PluginID == "" || step.PluginID != step.Manifest.ID || step.Release.PluginID != step.PluginID {
			return fmt.Errorf("%w: inconsistent execution selection for %s", ErrPluginDependencyGraphInvalid, step.PluginID)
		}
		if err := requireVerifiedPluginArtifact(db, step.Release); err != nil {
			return err
		}
		selected[step.PluginID] = step.Manifest
	}

	var enabled []model.PluginInstallation
	if err := db.Where("target = ? AND enabled = ?", plan.Target, true).Order("plugin_id").Find(&enabled).Error; err != nil {
		return err
	}
	for _, installation := range enabled {
		if _, included := selected[installation.PluginID]; included {
			continue
		}
		manifest, err := loadReleaseManifestForInstallation(db, installation)
		if err != nil {
			return fmt.Errorf("enabled plugin %s is invalid: %w", installation.PluginID, err)
		}
		for pluginID, selectedManifest := range selected {
			if pluginIDInList(installation.PluginID, selectedManifest.Conflicts) || pluginIDInList(pluginID, manifest.Conflicts) {
				return fmt.Errorf("%w: %s conflicts with enabled %s on %s", ErrPluginConflict, pluginID, installation.PluginID, plan.Target)
			}
		}
	}
	return nil
}
