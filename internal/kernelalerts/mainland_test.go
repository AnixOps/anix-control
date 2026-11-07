package kernelalerts

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

// An exit that refuses mainland destinations and refused some within a day
// is a warning on the node; an older refusal or a node with the block off is
// none.
func TestExitMainlandDestinations(t *testing.T) {
	db := openSQLite(t)
	node := addProxyNode(t, db, "tokyo", model.NodeStatusOnline)
	other := addProxyNode(t, db, "osaka", model.NodeStatusOnline)
	recent, old := start.Add(-time.Hour), start.Add(-25*time.Hour)
	require.NoError(t, db.Create(&model.NodeExitPolicy{NodeID: node.ID, BlockMainland: true, MainlandBlockedTotal: 12, MainlandBlockedAt: &recent, UpdatedAt: start}).Error)
	require.NoError(t, db.Create(&model.NodeExitPolicy{NodeID: other.ID, BlockMainland: true, MainlandBlockedTotal: 3, MainlandBlockedAt: &old, UpdatedAt: start}).Error)

	found := kinds(scan(t, db, defaultSettings(), start))
	finding, ok := found[KindExitMainlandDestinations+"/proxy-"+itoa(node.ID)]
	require.True(t, ok, found)
	require.Equal(t, model.KernelAlertWarning, finding.Severity)
	require.Equal(t, SubjectNode, finding.SubjectKind)
	require.Contains(t, finding.Message, "proxy-"+itoa(node.ID)+" (tokyo) refused connections to mainland-China destinations (12 so far")
	require.Equal(t, int64(12), finding.Detail["total"])
	require.NotContains(t, found, KindExitMainlandDestinations+"/proxy-"+itoa(other.ID), "the refusal is older than a day")

	require.NoError(t, db.Model(&model.NodeExitPolicy{}).Where("node_id = ?", node.ID).Update("block_mainland", false).Error)
	require.NotContains(t, kinds(scan(t, db, defaultSettings(), start)), KindExitMainlandDestinations+"/proxy-"+itoa(node.ID))
}
