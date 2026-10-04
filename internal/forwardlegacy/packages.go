package forwardlegacy

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// adoptCapabilityPrefix is the manifest capability by which a package
// adopts an existing kernel table into its storage lease.
const adoptCapabilityPrefix = "kernel.storage.adopt:"

// PackageAdoption is an installed package release that still adopts flux
// tables. A storage lease fails as a whole when an adopted table is
// missing (packagestore.ErrGrantTargetMissing), so dropping a table such a
// release adopts would take the whole package down, the forward package's
// v4 API included, until a release without the flux routes (F5d) is
// installed.
type PackageAdoption struct {
	PluginID string   `json:"plugin_id"`
	Target   string   `json:"target"`
	Version  string   `json:"version"`
	Tables   []string `json:"tables"`
}

// packagesAdoptingDropTables answers the installed package releases (the
// desired and the observed version) whose manifests adopt a drop table.
func packagesAdoptingDropTables(ctx context.Context, db *gorm.DB) ([]PackageAdoption, error) {
	if !db.Migrator().HasTable(&model.PluginInstallation{}) || !db.Migrator().HasTable(&model.PluginRelease{}) {
		return nil, nil
	}
	var installations []model.PluginInstallation
	if err := db.WithContext(ctx).Order("plugin_id, target").Find(&installations).Error; err != nil {
		return nil, err
	}
	var adoptions []PackageAdoption
	for _, installation := range installations {
		seen := map[string]bool{}
		for _, version := range []string{installation.DesiredVersion, installation.ObservedVersion} {
			if version == "" || seen[version] {
				continue
			}
			seen[version] = true
			var releases []model.PluginRelease
			if err := db.WithContext(ctx).Where("plugin_id = ? AND version = ?", installation.PluginID, version).
				Limit(1).Find(&releases).Error; err != nil {
				return nil, err
			}
			if len(releases) == 0 {
				continue
			}
			var manifest struct {
				Capabilities []string `json:"capabilities"`
			}
			if err := json.Unmarshal([]byte(releases[0].ManifestJSON), &manifest); err != nil {
				return nil, fmt.Errorf("package %s %s: manifest: %w", installation.PluginID, version, err)
			}
			var tables []string
			for _, capability := range manifest.Capabilities {
				table, ok := strings.CutPrefix(capability, adoptCapabilityPrefix)
				if ok && isDropTable(table) {
					tables = append(tables, table)
				}
			}
			if len(tables) > 0 {
				sort.Strings(tables)
				adoptions = append(adoptions, PackageAdoption{PluginID: installation.PluginID, Target: installation.Target, Version: version, Tables: tables})
			}
		}
	}
	return adoptions, nil
}
