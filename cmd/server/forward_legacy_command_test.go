package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type legacyFakeCleaner map[uint]forwardlegacy.PathResult

func (f legacyFakeCleaner) CleanNode(_ context.Context, node forwardlegacy.NodeInfo) forwardlegacy.PathResult {
	if result, ok := f[node.ID]; ok {
		return result
	}
	return forwardlegacy.PathResult{State: forwardlegacy.PathNotNeeded}
}

// The upgrade end to end on the command line: archive, check, abandon,
// the refused and the accepted drop, and a restart's schema steps that no
// longer create the dropped tables.
func TestForwardLegacyCommand(t *testing.T) {
	db := openSQLite(t)
	require.NoError(t, forwardlegacy.EnsureSchema(db))
	require.NoError(t, migrateSchema(db, "development"))
	require.NoError(t, db.AutoMigrate(&model.AgentTransport{}, &model.AgentCertificate{}))
	require.NoError(t, db.Create(&model.ForwardNode{ID: 1, Name: "hk", Type: "relay", Host: "192.0.2.1", APIToken: "node-secret-token-1", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 2, Name: "gone", Type: "exit", Host: "192.0.2.2", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.ForwardRule{Name: "r", RelayNodeID: 1, ListenPort: 30000, ExitNodeID: 2, TargetHost: "198.51.100.1", TargetPort: 80}).Error)

	dataDir := t.TempDir()
	savedDir, savedCleaners := forwardLegacyDataDir, forwardLegacyCleaners
	t.Cleanup(func() { forwardLegacyDataDir, forwardLegacyCleaners = savedDir, savedCleaners })
	forwardLegacyDataDir = dataDir
	forwardLegacyCleaners = func(*gorm.DB) (forwardlegacy.Cleaner, forwardlegacy.Cleaner) {
		return legacyFakeCleaner{1: {State: forwardlegacy.StateClean, Detail: "NodeX deleted 1 gost resource(s)"}},
			legacyFakeCleaner{2: {State: forwardlegacy.StateUnreachable, Detail: "192.0.2.2 did not answer over SSH"}}
	}
	ctx := context.Background()
	run := func(arguments ...string) (string, error) {
		var output bytes.Buffer
		err := runAdminCommand(ctx, nil, db, append([]string{"forward", "legacy"}, arguments...), &output)
		return output.String(), err
	}

	for _, arguments := range [][]string{{}, {"bogus"}, {"abandon"}, {"status", "extra"}} {
		_, err := run(arguments...)
		require.ErrorContains(t, err, "invalid forward legacy command", arguments)
	}

	output, err := run("status")
	require.NoError(t, err)
	assert.Contains(t, output, "forward-1  hk")
	assert.Contains(t, output, "2 node(s): 0 clean, 0 dirty, 0 unreachable, 0 abandoned, 2 unchecked.")
	assert.Contains(t, output, "no archive")

	output, err = run("archive")
	require.NoError(t, err)
	var archived struct {
		Archive model.ForwardLegacyArchive `json:"archive"`
		Audited bool                       `json:"audited"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &archived))
	assert.True(t, archived.Audited)
	assert.Equal(t, filepath.Join(dataDir, forwardlegacy.ArchiveDirName), filepath.Dir(archived.Archive.Path))
	content, err := os.ReadFile(archived.Archive.Path)
	require.NoError(t, err)
	assert.NotContains(t, string(content), "node-secret-token-1")
	output, err = run("archive", "-o", archived.Archive.Path)
	require.ErrorContains(t, err, "never overwrites", output)

	output, err = run("check")
	require.NoError(t, err)
	assert.Contains(t, output, "1 clean, 0 dirty, 1 unreachable")

	_, err = run("drop", "--confirm", "yes")
	require.ErrorContains(t, err, "confirmation phrase is wrong")
	require.ErrorContains(t, err, "forward-2 (gone) unreachable")

	output, err = run("abandon", "gone", "--reason", "returned to the provider")
	require.NoError(t, err)
	assert.Contains(t, output, `"state": "abandoned"`)

	output, err = run("status")
	require.NoError(t, err)
	assert.Contains(t, output, "abandoned by system/cli: returned to the provider")
	assert.Contains(t, output, `forward legacy drop --confirm "`+forwardlegacy.ConfirmPhrase+`"`)

	_, err = run("drop", "--confirm", forwardlegacy.ConfirmPhrase)
	require.ErrorContains(t, err, "no database backup")
	backup := filepath.Join(t.TempDir(), "anix.dump")
	require.NoError(t, os.WriteFile(backup, []byte("dump"), 0o600))
	output, err = run("drop", "--confirm", forwardlegacy.ConfirmPhrase, "--backup-taken", backup)
	require.NoError(t, err)
	assert.Contains(t, output, "Rolling back to 4.1 now needs the database backup "+backup)
	for _, table := range forwardlegacy.DropTables {
		assert.False(t, db.Migrator().HasTable(table), table)
	}
	assert.True(t, db.Migrator().HasTable("v2_forward_node"))

	// The next start (development or production) leaves them dropped.
	require.NoError(t, migrateSchema(db, "development"))
	require.NoError(t, migrateSchema(db, "production"))
	for _, step := range []func(*gorm.DB) error{
		forwardlegacy.UnlessDropped(func(db *gorm.DB) error { return db.AutoMigrate(&model.ForwardRuntimeJob{}) }),
	} {
		require.NoError(t, step(db))
	}
	for _, table := range forwardlegacy.DropTables {
		assert.False(t, db.Migrator().HasTable(table), table)
	}
	output, err = run("status")
	require.NoError(t, err)
	assert.Contains(t, output, "The flux tables were dropped at")

	var actions []string
	require.NoError(t, db.Model(&model.OperationLog{}).Order("id").Pluck("action", &actions).Error)
	assert.Equal(t, []string{"forward.legacy_archive", "forward.legacy_check", "forward.legacy_drop_refused",
		"forward.legacy_abandon", "forward.legacy_drop_refused", "forward.legacy_drop"}, actions)
}

func TestForwardLegacyArchiveDir(t *testing.T) {
	saved := forwardLegacyDataDir
	t.Cleanup(func() { forwardLegacyDataDir = saved })
	forwardLegacyDataDir = "/var/lib/anix"
	assert.Equal(t, "/var/lib/anix/forward-legacy", forwardLegacyArchiveDir(nil))
	cfg := testConfigWithDatabase("sqlite", "/srv/anix/config/data/v2board.db")
	assert.Equal(t, "/srv/anix/config/data", resolveDataDir(cfg, ""))
	cfg = testConfigWithDatabase("postgres", "anix")
	assert.True(t, strings.HasSuffix(resolveDataDir(cfg, "/etc/anix/config.yaml"), filepath.Join("config", "data")))
}

func testConfigWithDatabase(driver, database string) *config.Config {
	return &config.Config{Database: config.DatabaseConfig{Driver: driver, Database: database}}
}
