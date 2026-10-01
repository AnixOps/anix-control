package kernelnodeops

import (
	"context"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestSplitPhasesFrom: GetCapabilities answers each split table's phase;
// FINALIZED only once finalize completed, so a package never adopts a table
// still being finalized.
func TestSplitPhasesFrom(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		answer := func() map[string]kernelnodeopsv1.SecretSplitPhase {
			t.Helper()
			tables, err := SplitPhasesFrom(db)(ctx)
			require.NoError(t, err)
			require.Len(t, tables, len(splitTables))
			out := map[string]kernelnodeopsv1.SecretSplitPhase{}
			for _, table := range tables {
				out[table.GetTable()] = table.GetPhase()
			}
			return out
		}
		require.NoError(t, db.Migrator().DropTable(&model.NodeSecretSplit{}))
		for table, phase := range answer() {
			require.Equal(t, kernelnodeopsv1.SecretSplitPhase_SECRET_SPLIT_PHASE_LEGACY, phase, table)
		}
		require.NoError(t, nodesecrets.EnsureSchema(db))
		phases := answer()
		require.Equal(t, kernelnodeopsv1.SecretSplitPhase_SECRET_SPLIT_PHASE_DUAL_WRITE, phases["v2_node"])
		require.Equal(t, kernelnodeopsv1.SecretSplitPhase_SECRET_SPLIT_PHASE_LEGACY, phases["v2_forward_runtime_job"], "not a split table")

		update := func(table string, values map[string]any) {
			require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", table).Updates(values).Error)
		}
		update("v2_node", map[string]any{"phase": nodesecrets.PhaseDualRead})
		update("v2_forward_node", map[string]any{"phase": nodesecrets.PhaseFinalized})
		update("v2_node_protocol", map[string]any{"phase": nodesecrets.PhaseFinalized, "finalized_at": time.Now().UTC()})
		phases = answer()
		require.Equal(t, kernelnodeopsv1.SecretSplitPhase_SECRET_SPLIT_PHASE_DUAL_READ, phases["v2_node"])
		require.Equal(t, kernelnodeopsv1.SecretSplitPhase_SECRET_SPLIT_PHASE_DUAL_READ, phases["v2_forward_node"], "still being finalized")
		require.Equal(t, kernelnodeopsv1.SecretSplitPhase_SECRET_SPLIT_PHASE_FINALIZED, phases["v2_node_protocol"])
	})
}
