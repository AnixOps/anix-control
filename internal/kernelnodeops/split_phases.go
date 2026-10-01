package kernelnodeops

import (
	"context"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"gorm.io/gorm"
)

// SplitPhasesFrom answers GetCapabilities.tables from the node credential
// split's state in db (v4_kernel_node_secret_split), read uncached.
//
//   - A table answers its phase. One being finalized (finalized, without
//     finalized_at yet) answers DUAL_READ: FINALIZED is the promise that the
//     legacy columns hold no secret and the table may be adopted.
//   - v2_forward_runtime_job is not a split table: its payloads carry no
//     token since NO-7. It answers LEGACY, as before.
//   - A database without the split tables answers LEGACY for every table.
func SplitPhasesFrom(db *gorm.DB) func(ctx context.Context) ([]*kernelnodeopsv1.TableSplitState, error) {
	return func(ctx context.Context) ([]*kernelnodeopsv1.TableSplitState, error) {
		rows, err := nodesecrets.Status(ctx, db)
		if err != nil {
			rows = nil
		}
		phases := make(map[string]kernelnodeopsv1.SecretSplitPhase, len(rows))
		for _, row := range rows {
			switch {
			case row.Phase == nodesecrets.PhaseFinalized && row.FinalizedAt != nil:
				phases[row.Table] = kernelnodeopsv1.SecretSplitPhase_SECRET_SPLIT_PHASE_FINALIZED
			case row.Phase == nodesecrets.PhaseFinalized, row.Phase == nodesecrets.PhaseDualRead:
				phases[row.Table] = kernelnodeopsv1.SecretSplitPhase_SECRET_SPLIT_PHASE_DUAL_READ
			case row.Phase == nodesecrets.PhaseDualWrite:
				phases[row.Table] = kernelnodeopsv1.SecretSplitPhase_SECRET_SPLIT_PHASE_DUAL_WRITE
			}
		}
		tables := make([]*kernelnodeopsv1.TableSplitState, 0, len(splitTables))
		for _, table := range splitTables {
			phase, ok := phases[table]
			if !ok {
				phase = kernelnodeopsv1.SecretSplitPhase_SECRET_SPLIT_PHASE_LEGACY
			}
			tables = append(tables, &kernelnodeopsv1.TableSplitState{Table: table, Phase: phase})
		}
		return tables, nil
	}
}
