package service

import (
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The production start adds the node group columns to the existing node
// tables (it never runs the full AutoMigrate there) and creates the node
// group table with its partial unique name index. The existing nodes are
// approved.
func TestProductionSchemaStepsAddTheNodeGroupColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "schema.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	// Tables as an earlier release created them.
	require.NoError(t, db.Exec(`CREATE TABLE v2_forward_node (id integer primary key, name text not null, type text not null default 'exit',
		host text not null, port integer not null, api_port integer, api_token text, enabled numeric default true,
		created_at datetime, updated_at datetime)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO v2_forward_node (id, name, host, port) VALUES (3, 'relay', '203.0.113.3', 0)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE v2_node (id integer primary key, name text, host text, created_at datetime, updated_at datetime)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO v2_node (id, name, host) VALUES (4, 'proxy', '203.0.113.4')`).Error)

	for range 2 { // idempotent
		require.NoError(t, EnsureNodeRuntimeHealthSchema(db))
		require.NoError(t, EnsureForwardNodeColumns(db))
		require.NoError(t, EnsureKernelSchema(db))
	}
	migrator := db.Migrator()
	for _, column := range []string{"metrics_port", "node_group_id", "machine_fingerprint", "approval", "last_public_address"} {
		assert.True(t, migrator.HasColumn(&model.ForwardNode{}, column), "v2_forward_node."+column)
	}
	for _, column := range []string{"node_group_id", "machine_fingerprint", "approval", "last_public_address"} {
		assert.True(t, migrator.HasColumn(&model.Node{}, column), "v2_node."+column)
	}
	assert.True(t, migrator.HasIndex(&model.ForwardNode{}, "NodeGroupID"))
	assert.True(t, migrator.HasIndex(&model.ForwardNode{}, "MachineFingerprint"))

	var forwardApproval, proxyApproval string
	require.NoError(t, db.Raw(`SELECT approval FROM v2_forward_node WHERE id = 3`).Scan(&forwardApproval).Error)
	require.NoError(t, db.Raw(`SELECT approval FROM v2_node WHERE id = 4`).Scan(&proxyApproval).Error)
	assert.Equal(t, model.NodeApprovalApproved, forwardApproval, "existing forward nodes are approved")
	assert.Equal(t, model.NodeApprovalApproved, proxyApproval, "existing proxy nodes are approved")

	assert.True(t, migrator.HasTable(&model.NodeGroup{}))
	assert.True(t, migrator.HasIndex(&model.NodeGroup{}, model.NodeGroupLiveNameIndex))
	var definition string
	require.NoError(t, db.Raw(`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?`, model.NodeGroupLiveNameIndex).Scan(&definition).Error)
	assert.Contains(t, definition, "WHERE deleted = 0", "the name is unique among live groups only")
}
