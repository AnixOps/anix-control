package kernelnodeops

import (
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Once the v4.2 upgrade dropped the flux tables, a forward node's desired
// configuration has no legacy rules or tunnels instead of failing.
func TestForwardDesiredConfigWithoutFluxTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ForwardNode{}))
	require.NoError(t, db.Create(&model.ForwardNode{ID: 4, Name: "sg", Type: "exit", Host: "192.0.2.4", Enabled: true}).Error)
	document, err := buildForwardDesiredConfig(db, 4)
	require.NoError(t, err)
	require.Equal(t, agentcontrol.NodeKindForward, document["kind"])
	require.Empty(t, document["legacy_rules"])
	require.Empty(t, document["tunnels"])
}
