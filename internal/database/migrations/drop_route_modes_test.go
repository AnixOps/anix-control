package migrations_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 0004_drop_route_modes on a database the previous release left: the
// route-mode tables are dropped, every stored route-mode map leaves its
// Control package configuration (a new revision, written to the audit log
// with the map first), and the rest of every document is unchanged.
func TestDropRouteModesRemovesTheStoredModes(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		db := open(t)
		history := steps(t)
		var at int
		for index, step := range history {
			if step.Name == "drop_route_modes" {
				at = index
			}
		}
		require.NotZero(t, at, "0004_drop_route_modes is a step")
		_, err := migrations.RunSteps(ctx, db, history[:at], migrateOptions())
		require.NoError(t, err)

		now := time.Now().UTC().Truncate(time.Second)
		for _, row := range []struct {
			id                           uint
			pluginID, target, configJSON string
			revision                     int64
		}{
			{1, "knowledge", "control", `{"port":443,"ratio":1.50,"routes":{"knowledge.article.list":"legacy","knowledge.admin.knowledge.post":"shadow"}}`, 4},
			{2, "ticket", "control", `{"port":80}`, 2},
			{3, "identity-platform", "control", `{"routes":{}}`, 9},
			{4, "machine-telemetry", "agent", `{"routes":{"kept":"by an agent"}}`, 1},
			{5, "notification", "control", `[1,2]`, 1},
		} {
			require.NoError(t, db.Exec("INSERT INTO v3_kernel_plugin_installation (id, plugin_id, target, desired_version, state, enabled, lifecycle_generation, config_revision, created_at, updated_at) VALUES (?, ?, ?, '4.2.1', 'healthy', ?, 1, ?, ?, ?)",
				row.id, row.pluginID, row.target, true, row.revision, now, now).Error)
			require.NoError(t, db.Exec("INSERT INTO v3_kernel_plugin_configuration (installation_id, revision, config_json, config_hash, updated_by, created_at, updated_at) VALUES (?, ?, ?, 'old', 7, ?, ?)",
				row.id, row.revision, row.configJSON, now, now).Error)
		}
		require.NoError(t, db.Exec("INSERT INTO v4_kernel_route_mode_revision (group_id, package_id, route_id, action, from_mode, to_mode, config_revision, created_at) VALUES ('g', 'knowledge', 'knowledge.article.list', 'set', 'native', 'legacy', 4, ?)", now).Error)

		result, err := migrations.Migrate(ctx, db, migrateOptions())
		require.NoError(t, err)
		require.Contains(t, result.Applied, "0004_drop_route_modes")
		for _, table := range []string{"v4_kernel_route_mode_revision", "v4_kernel_shadow_mismatch_sample"} {
			assert.False(t, db.Migrator().HasTable(table), table)
		}

		type stored struct {
			ConfigJSON string
			ConfigHash string
			Revision   int64
			UpdatedBy  uint
		}
		read := func(id uint) (stored, int64) {
			var configuration stored
			require.NoError(t, db.Raw("SELECT config_json, config_hash, revision, updated_by FROM v3_kernel_plugin_configuration WHERE installation_id = ?", id).Scan(&configuration).Error)
			var installationRevision int64
			require.NoError(t, db.Raw("SELECT config_revision FROM v3_kernel_plugin_installation WHERE id = ?", id).Scan(&installationRevision).Error)
			return configuration, installationRevision
		}
		hash := func(document string) string {
			sum := sha256.Sum256([]byte(document))
			return hex.EncodeToString(sum[:])
		}

		knowledge, revision := read(1)
		assert.Equal(t, `{"port":443,"ratio":1.50}`, knowledge.ConfigJSON, "the rest of the document keeps its text")
		assert.Equal(t, hash(knowledge.ConfigJSON), knowledge.ConfigHash)
		assert.Equal(t, int64(5), knowledge.Revision)
		assert.Zero(t, knowledge.UpdatedBy)
		assert.Equal(t, int64(5), revision)
		identity, revision := read(3)
		assert.Equal(t, `{}`, identity.ConfigJSON)
		assert.Equal(t, int64(10), identity.Revision)
		assert.Equal(t, int64(10), revision)
		for id, want := range map[uint]string{2: `{"port":80}`, 4: `{"routes":{"kept":"by an agent"}}`, 5: `[1,2]`} {
			untouched, _ := read(id)
			assert.Equal(t, want, untouched.ConfigJSON, id)
			assert.Equal(t, "old", untouched.ConfigHash, id)
		}

		type audit struct {
			Username   string
			Action     string
			Module     string
			TargetType string
			TargetID   uint
			Content    string
		}
		var audits []audit
		require.NoError(t, db.Raw("SELECT username, action, module, target_type, target_id, content FROM v2_operation_log ORDER BY id").Scan(&audits).Error)
		require.Len(t, audits, 2)
		assert.Equal(t, audit{Username: "system/migration", Action: "route_modes_removed", Module: "kernel", TargetType: "plugin_installation", TargetID: 1, Content: audits[0].Content}, audits[0])
		var content struct {
			Migration    string            `json:"migration"`
			PluginID     string            `json:"plugin_id"`
			Routes       map[string]string `json:"routes"`
			FromRevision int64             `json:"from_revision"`
			ToRevision   int64             `json:"to_revision"`
		}
		require.NoError(t, json.Unmarshal([]byte(audits[0].Content), &content))
		assert.Equal(t, "0004_drop_route_modes", content.Migration)
		assert.Equal(t, "knowledge", content.PluginID)
		assert.Equal(t, map[string]string{"knowledge.article.list": "legacy", "knowledge.admin.knowledge.post": "shadow"}, content.Routes)
		assert.Equal(t, int64(4), content.FromRevision)
		assert.Equal(t, int64(5), content.ToRevision)
		assert.Equal(t, uint(3), audits[1].TargetID)
	})
}
