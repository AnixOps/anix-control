package migrations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// dropRouteModesVersion is the number of 0004_drop_route_modes. Should the
// step move to another number before it is released, change it here and in
// the two SQL file names.
const dropRouteModesVersion = 4

// routeModesConfigKey is the key of a Control package configuration that
// held its stored route modes.
const routeModesConfigKey = "routes"

// dropStoredRouteModes is the data part of 0004_drop_route_modes: it removes
// the stored route-mode map ("routes") from every Control package
// configuration, so only the rehearsed list and identity group A decide a
// route's mode. Each document that held one gets a new revision without the
// key (the installation's config_revision follows), and the removed map is
// written to the audit log first, with the installation and the revisions.
// A document that is not a JSON object holds no map and is left as it is.
//
// The step works on the tables as 0001_baseline defines them, with plain
// SQL, so later model changes do not change what it does.
func dropStoredRouteModes(ctx context.Context, tx *gorm.DB, _ string) error {
	type configuration struct {
		InstallationID uint
		Revision       int64
		ConfigJSON     string
		PluginID       string
	}
	var rows []configuration
	if err := tx.WithContext(ctx).Raw(
		"SELECT c.installation_id AS installation_id, c.revision AS revision, c.config_json AS config_json, i.plugin_id AS plugin_id " +
			"FROM v3_kernel_plugin_configuration c JOIN v3_kernel_plugin_installation i ON i.id = c.installation_id " +
			"WHERE i.target = 'control' ORDER BY c.installation_id").Scan(&rows).Error; err != nil {
		return fmt.Errorf("read the package configurations: %w", err)
	}
	now := time.Now().UTC()
	for _, row := range rows {
		document, removed, ok := withoutRouteModes(row.ConfigJSON)
		if !ok {
			continue
		}
		canonical, err := json.Marshal(document)
		if err != nil {
			return fmt.Errorf("installation %d: encode the configuration: %w", row.InstallationID, err)
		}
		digest := sha256.Sum256(canonical)
		revision := row.Revision + 1
		content, err := json.Marshal(map[string]any{
			"migration": "0004_drop_route_modes", "plugin_id": row.PluginID, "routes": removed,
			"from_revision": row.Revision, "to_revision": revision,
			"reason": "route modes come only from the rehearsed list (config/package-route-defaults.json); identity group A follows the identity authority",
		})
		if err != nil {
			return fmt.Errorf("installation %d: encode the audit entry: %w", row.InstallationID, err)
		}
		if err := tx.Exec("INSERT INTO v2_operation_log (username, action, module, target_type, target_id, content, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			"system/migration", "route_modes_removed", "kernel", "plugin_installation", row.InstallationID, string(content), 1, now).Error; err != nil {
			return fmt.Errorf("installation %d: audit the removed route modes: %w", row.InstallationID, err)
		}
		if err := tx.Exec("UPDATE v3_kernel_plugin_configuration SET config_json = ?, config_hash = ?, revision = ?, updated_by = 0, updated_at = ? WHERE installation_id = ?",
			string(canonical), hex.EncodeToString(digest[:]), revision, now, row.InstallationID).Error; err != nil {
			return fmt.Errorf("installation %d: store the configuration: %w", row.InstallationID, err)
		}
		if err := tx.Exec("UPDATE v3_kernel_plugin_installation SET config_revision = ? WHERE id = ?", revision, row.InstallationID).Error; err != nil {
			return fmt.Errorf("installation %d: record the configuration revision: %w", row.InstallationID, err)
		}
	}
	return nil
}

// withoutRouteModes returns the configuration document without its stored
// route-mode map, and the map, when the document is a JSON object that has
// one. Numbers keep their text, so the rest of the document is unchanged.
func withoutRouteModes(raw string) (map[string]any, json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil || document == nil {
		return nil, nil, false
	}
	value, ok := document[routeModesConfigKey]
	if !ok {
		return nil, nil, false
	}
	removed, err := json.Marshal(value)
	if err != nil {
		return nil, nil, false
	}
	delete(document, routeModesConfigKey)
	return document, removed, true
}
