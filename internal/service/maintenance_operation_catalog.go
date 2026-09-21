package service

import (
	"errors"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

type MaintenanceCatalogNode struct {
	ID              uint     `json:"id"`
	Name            string   `json:"name"`
	DesiredVersion  string   `json:"desired_version"`
	ActiveVersion   string   `json:"active_version"`
	ActiveEnabled   bool     `json:"active_enabled"`
	RestoreVersions []string `json:"restore_versions"`
}
type MaintenanceCatalogRelease struct {
	Version string `json:"version"`
}
type MaintenanceOperationCatalog struct {
	Nodes      []MaintenanceCatalogNode    `json:"nodes"`
	Releases   []MaintenanceCatalogRelease `json:"releases"`
	NextNodeID uint                        `json:"next_node_id"`
}

// Only the specific operation choices cross this boundary. Node credentials,
// hosts, command output and package/config bodies are deliberately omitted.
func GetMaintenanceOperationCatalog(db *gorm.DB, afterID uint) (MaintenanceOperationCatalog, error) {
	result := MaintenanceOperationCatalog{Nodes: []MaintenanceCatalogNode{}, Releases: []MaintenanceCatalogRelease{}}
	var releases []model.PluginRelease
	if err := db.Where("plugin_id = ?", "machine-telemetry").Order("created_at DESC").Limit(100).Find(&releases).Error; err != nil {
		return result, err
	}
	verified := map[string]bool{}
	for _, release := range releases {
		manifest, err := VerifyStoredPluginRelease(db, release, nil)
		if err != nil || !manifestSupportsTarget(*manifest, "agent") {
			continue
		}
		var artifact model.PluginArtifact
		if err := db.Select("id, artifact_sha256, size_bytes").First(&artifact, "release_id = ?", release.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return result, err
		}
		if !strings.EqualFold(artifact.ArtifactSHA256, release.ArtifactSHA256) || artifact.SizeBytes <= 0 {
			continue
		}
		verified[release.Version] = true
		result.Releases = append(result.Releases, MaintenanceCatalogRelease{Version: release.Version})
	}
	var nodes []struct {
		ID   uint
		Name string
	}
	assigned := db.Model(&model.NodeServiceAssignment{}).Select("node_id").Where("plugin_id = ? AND delete_pending = ?", "machine-telemetry", false)
	if err := db.Model(&model.Node{}).Select("id,name").Where("id > ? AND id IN (?)", afterID, assigned).Order("id").Limit(101).Find(&nodes).Error; err != nil {
		return result, err
	}
	if len(nodes) > 100 {
		nodes = nodes[:100]
		result.NextNodeID = nodes[99].ID
	}
	for _, node := range nodes {
		item := MaintenanceCatalogNode{ID: node.ID, Name: node.Name, RestoreVersions: []string{}}
		var lifecycle model.NodePluginLifecycle
		if err := db.First(&lifecycle, "node_id = ? AND plugin_id = ?", node.ID, "machine-telemetry").Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return result, err
		}
		item.DesiredVersion, item.ActiveVersion, item.ActiveEnabled = lifecycle.DesiredVersion, lifecycle.ActiveVersion, lifecycle.ActiveEnabled
		var history []string
		if err := db.Model(&model.KernelOperation{}).Where("node_id = ? AND plugin_id = ? AND state IN ?", node.ID, "machine-telemetry", []string{"succeeded", "completed"}).Distinct("target_version").Limit(100).Pluck("target_version", &history).Error; err != nil {
			return result, err
		}
		for _, version := range history {
			if verified[version] {
				if _, err := maintenanceVerifiedConfig(db, node.ID, "machine-telemetry", version); err == nil {
					item.RestoreVersions = append(item.RestoreVersions, version)
				}
			}
		}
		result.Nodes = append(result.Nodes, item)
	}
	return result, nil
}
