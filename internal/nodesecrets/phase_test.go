package nodesecrets

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// phases answers the stored phase of every split table.
func phases(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	rows, err := Status(context.Background(), db)
	require.NoError(t, err)
	out := map[string]string{}
	for _, row := range rows {
		out[row.Table] = row.Phase
	}
	return out
}

func audits(t *testing.T, db *gorm.DB) []model.OperationLog {
	t.Helper()
	var rows []model.OperationLog
	require.NoError(t, db.Where("module = ?", "node_secrets").Order("id").Find(&rows).Error)
	return rows
}

// allPhases is every split table in one phase.
func allPhases(phase string) map[string]string {
	out := map[string]string{}
	for _, table := range Tables() {
		out[table] = phase
	}
	return out
}

func TestPhaseGateRefusesWithoutACleanRecentVerify(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		f := seedLegacy(t, db)
		dualRead := PhaseOptions{Phase: PhaseDualRead, Actor: "operator"}

		// Never verified.
		changes, err := SetPhase(ctx, db, dualRead)
		require.ErrorIs(t, err, ErrPhaseRefused)
		require.Len(t, changes, len(Tables()))
		for _, change := range changes {
			require.False(t, change.Changed)
			require.Contains(t, change.Refused, "no verification has matched")
		}
		require.Equal(t, allPhases(PhaseDualWrite), phases(t, db))

		// Verified with mismatches: the legacy rows were never copied.
		_, err = Verify(ctx, db, VerifyOptions{})
		require.ErrorIs(t, err, ErrMismatch)
		changes, err = SetPhase(ctx, db, PhaseOptions{Tables: []string{TableNode}, Phase: PhaseDualRead})
		require.ErrorIs(t, err, ErrPhaseRefused)
		require.Contains(t, changes[0].Refused, "no verification has matched")

		// A clean verify opens the gate, table by table.
		_, err = Backfill(ctx, db, BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		changes, err = SetPhase(ctx, db, PhaseOptions{Tables: []string{TableNode}, Phase: PhaseDualRead, Actor: "operator"})
		require.NoError(t, err)
		require.Equal(t, []PhaseChange{{Table: TableNode, From: PhaseDualWrite, To: PhaseDualRead, Changed: true,
			VerifiedAt: changes[0].VerifiedAt, Digest: changes[0].Digest}}, changes)
		require.NotNil(t, changes[0].VerifiedAt)
		require.Len(t, changes[0].Digest, 64)
		require.Equal(t, PhaseDualRead, ReadPhase(db, TableNode), "this process reads the change at once")
		require.Equal(t, PhaseDualWrite, ReadPhase(db, TableAuthorizedKey))

		// The same phase again changes nothing and audits nothing.
		changes, err = SetPhase(ctx, db, PhaseOptions{Tables: []string{TableNode}, Phase: PhaseDualRead})
		require.NoError(t, err)
		require.False(t, changes[0].Changed)
		require.Len(t, audits(t, db), 1)

		// A verification older than VerifyMaxAge no longer opens it.
		changes, err = SetPhase(ctx, db, PhaseOptions{Tables: []string{TableAuthorizedKey}, Phase: PhaseDualRead,
			Now: time.Now().Add(VerifyMaxAge + time.Minute)})
		require.ErrorIs(t, err, ErrPhaseRefused)
		require.Contains(t, changes[0].Refused, "older than")

		// A failing verification after a clean one closes it again.
		require.NoError(t, db.Where("subject_kind = ? AND subject_id = ?", SubjectForward, f.forwards[0].ID).Delete(&model.NodeCredential{}).Error)
		_, err = Verify(ctx, db, VerifyOptions{Tables: []string{TableForwardNode}})
		require.ErrorIs(t, err, ErrMismatch)
		changes, err = SetPhase(ctx, db, PhaseOptions{Tables: []string{TableForwardNode}, Phase: PhaseDualRead})
		require.ErrorIs(t, err, ErrPhaseRefused)
		require.Contains(t, changes[0].Refused, "mismatches")

		// "all" moves every table or none: the forward nodes refuse, so the
		// registration keys stay in dual_write.
		changes, err = SetPhase(ctx, db, dualRead)
		require.ErrorIs(t, err, ErrPhaseRefused)
		for _, change := range changes {
			require.False(t, change.Changed, change.Table)
			if change.Table == TableForwardNode {
				require.NotEmpty(t, change.Refused)
			} else {
				require.Empty(t, change.Refused, change.Table)
			}
		}
		want := allPhases(PhaseDualWrite)
		want[TableNode] = PhaseDualRead
		require.Equal(t, want, phases(t, db))
		require.Len(t, audits(t, db), 1)

		// Backfilled and verified again, every table moves.
		_, err = Backfill(ctx, db, BackfillOptions{Tables: []string{TableForwardNode}})
		require.NoError(t, err)
		requireVerified(t, db)
		changes, err = SetPhase(ctx, db, dualRead)
		require.NoError(t, err)
		require.Equal(t, allPhases(PhaseDualRead), phases(t, db))
		changed := 0
		for _, change := range changes {
			if change.Changed {
				changed++
			}
		}
		require.Equal(t, len(Tables())-1, changed)

		// Each change is audited: who, which table, from and to, and the
		// verification it relied on; never a secret.
		entries := audits(t, db)
		require.Len(t, entries, len(Tables()))
		for _, entry := range entries {
			require.Equal(t, "operator", entry.Username)
			require.Equal(t, "phase_dual_read", entry.Action)
			require.Equal(t, "node_secret_split", entry.TargetType)
			require.Contains(t, entry.Content, `"to":"dual_read"`)
			require.Contains(t, entry.Content, `"digest":"`)
			require.NotContains(t, entry.Content, "fake-")
		}
	})
}

func TestPhaseRollbackAndRefusedPhases(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		f := seedLegacy(t, db)
		_, err := Backfill(ctx, db, BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		_, err = SetPhase(ctx, db, PhaseOptions{Phase: PhaseDualRead})
		require.NoError(t, err)

		// The way back needs no verification, even a failing one.
		require.NoError(t, db.Where("subject_kind = ?", SubjectProxy).Delete(&model.NodeCredential{}).Error)
		_, err = Verify(ctx, db, VerifyOptions{Tables: []string{TableNode}})
		require.ErrorIs(t, err, ErrMismatch)
		changes, err := SetPhase(ctx, db, PhaseOptions{Tables: []string{TableNode}, Phase: PhaseDualWrite})
		require.NoError(t, err)
		require.True(t, changes[0].Changed)
		require.Nil(t, changes[0].VerifiedAt)
		require.Equal(t, PhaseDualWrite, ReadPhase(db, TableNode))
		entries := audits(t, db)
		last := entries[len(entries)-1]
		require.Equal(t, "phase_dual_write", last.Action)
		require.Equal(t, "cli", last.Username, "an unnamed actor")
		require.Contains(t, last.Content, `"from":"dual_read"`)

		// Rolled back, the readers read the legacy columns again.
		node := f.nodes[0]
		before := FallbackCount(TableNode, KindNodeAPIKey, "")
		require.True(t, NodeAPIKeyMatches(db, &node, "fake-node-key-1"))
		require.Equal(t, before, FallbackCount(TableNode, KindNodeAPIKey, ""), "dual_write never reads the new table")

		// Only dual_read and dual_write can be asked for, and a table in
		// another phase is not moved.
		for _, phase := range []string{PhaseLegacy, PhaseFinalized, "", "dual-read"} {
			_, err := SetPhase(ctx, db, PhaseOptions{Phase: phase})
			require.Error(t, err, phase)
			require.False(t, errors.Is(err, ErrPhaseRefused), phase)
		}
		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", TableWireGuardPeer).Update("phase", PhaseFinalized).Error)
		changes, err = SetPhase(ctx, db, PhaseOptions{Tables: []string{TableWireGuardPeer}, Phase: PhaseDualWrite})
		require.ErrorIs(t, err, ErrPhaseRefused)
		require.Contains(t, changes[0].Refused, `"finalized"`)
		_, err = SetPhase(ctx, db, PhaseOptions{Tables: []string{"v2_user"}, Phase: PhaseDualWrite})
		require.Error(t, err)
	})
}

func TestSetPhaseNeedsTheAuditLog(t *testing.T) {
	db := openSQLite(t)
	require.NoError(t, db.AutoMigrate(legacyModels...))
	require.NoError(t, EnsureSchema(db))
	_, err := SetPhase(context.Background(), db, PhaseOptions{Phase: PhaseDualWrite})
	require.ErrorContains(t, err, "v2_operation_log")
}

func TestReadPhaseDefaultsToTheLegacyColumns(t *testing.T) {
	require.Equal(t, PhaseDualWrite, ReadPhase(nil, TableNode))
	// No split tables: the legacy columns.
	db := openSQLite(t)
	require.NoError(t, db.AutoMigrate(legacyModels...))
	require.Equal(t, PhaseDualWrite, ReadPhase(db, TableNode))
	// An unknown phase reads as dual_write.
	require.NoError(t, EnsureSchema(db))
	require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", TableNode).Update("phase", "sideways").Error)
	invalidatePhases(db)
	require.Equal(t, PhaseDualWrite, ReadPhase(db, TableNode))
	// The phase is cached, and a change made elsewhere is read within
	// PhaseCacheTTL.
	require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", TableNode).Update("phase", PhaseDualRead).Error)
	require.Equal(t, PhaseDualWrite, ReadPhase(db, TableNode), "cached")
	phaseMu.Lock()
	entry := phaseCache[phaseCacheKey(db)]
	entry.loaded = entry.loaded.Add(-PhaseCacheTTL)
	phaseCache[phaseCacheKey(db)] = entry
	phaseMu.Unlock()
	require.Equal(t, PhaseDualRead, ReadPhase(db, TableNode))
	// Sessions and transactions of one database share the cache.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		require.Equal(t, PhaseDualRead, ReadPhase(tx.WithContext(context.Background()), TableNode))
		return nil
	}))
}
