package packagecompat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// NodeSplitModels are the node credential split's tables, and the
// operation log its phase changes are audited in. A route on a table or
// view of the split's remainder (docs/architecture/node-ops-service.md
// section 4.6) lists them in Route.Models, so the shared databases keep
// them between cases.
func NodeSplitModels() []any {
	return []any{&model.NodeCredential{}, &model.ProtocolSecret{}, &model.NodeSecretSplit{}, &model.OperationLog{}}
}

// FinalizeNodeSplit takes tables (all split tables when none are named)
// through the split as an operator does (docs/UPGRADE.md): the state rows,
// a backfill, a matching verification, dual_read, then finalize. Their
// legacy secret columns then hold tombstones and redacted documents, and
// the kernel creates the views that wait for finalize
// (packagestore.EnsureKernelAPIViews). Seed functions call it after the
// rows are written, on both sides.
func FinalizeNodeSplit(t testing.TB, db *gorm.DB, tables ...string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, nodesecrets.EnsureSchema(db))
	nodesecrets.ForgetPhases(db)
	_, err := nodesecrets.Backfill(ctx, db, nodesecrets.BackfillOptions{Tables: tables, Restart: true})
	require.NoError(t, err)
	verified, err := nodesecrets.Verify(ctx, db, nodesecrets.VerifyOptions{Tables: tables})
	require.NoError(t, err, "%+v", verified)
	for _, result := range verified {
		require.True(t, result.Match, "%+v", result)
	}
	_, err = nodesecrets.SetPhase(ctx, db, nodesecrets.PhaseOptions{Tables: tables, Phase: nodesecrets.PhaseDualRead, Actor: "packagecompat"})
	require.NoError(t, err)
	_, err = nodesecrets.Finalize(ctx, db, nodesecrets.FinalizeOptions{Tables: tables, Confirm: true, Actor: "packagecompat"})
	require.NoError(t, err)
	nodesecrets.ForgetPhases(db)
	require.NoError(t, packagestore.EnsureKernelAPIViews(db))
}

// Lease answers what the kernel's storage lease grants the package whose
// manifest template is packages/<packageID>/manifest.template.json on db,
// as the kernel decides it when a host asks (service.EffectiveStorageGrants):
// an adoption of v2_node, v2_node_protocol or v2_forward_node, and a view of
// the split's remainder, only once finalized. The answer is computed when
// asked, so it sees what the case's seed did. It is a native service's
// Leased.
func Lease(db *gorm.DB, packageID string) func(ctx context.Context, name string) bool {
	grants := service.StorageGrants(manifest(packageID))
	return func(_ context.Context, name string) bool {
		effective, err := service.EffectiveStorageGrants(db, grants)
		if err != nil {
			return false
		}
		for _, granted := range [][]string{effective.AdoptTables, effective.Views} {
			for _, candidate := range granted {
				if candidate == name {
					return true
				}
			}
		}
		return false
	}
}

// manifest reads a package's manifest template; a package without one is
// a broken test.
func manifest(packageID string) service.PluginManifest {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("packagecompat: no caller")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "packages", packageID, "manifest.template.json")) // #nosec G304 -- a test reads the manifest template of a package it names.
	if err != nil {
		panic(err)
	}
	var manifest service.PluginManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		panic(err)
	}
	return manifest
}

// OpenSQLite is a fresh SQLite kernel database with models migrated, for
// a test that calls a native handler directly.
func OpenSQLite(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	_, db, err := createSQLite(t.TempDir(), "direct")
	require.NoError(t, err)
	closeOnCleanup(t, db)
	require.NoError(t, db.AutoMigrate(models...))
	return db
}
