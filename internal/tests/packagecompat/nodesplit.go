package packagecompat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v5/internal/packagestore"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// NodeSplitModels are the node secret split's tables. A route on a table
// or view of the split's remainder (docs/architecture/node-ops-service.md
// section 4.6) lists them in Route.Models, so the shared databases keep
// them between cases.
func NodeSplitModels() []any {
	return []any{&model.ProtocolSecret{}, &model.NodeSecretSplit{}}
}

// FinalizeNodeSplit moves the secrets of the rows a seed wrote into tables
// (all split tables when none are named) to the new tables, as the
// kernel's writers do (nodesecrets.SeedForTest): their legacy secret
// columns then hold tombstones and redacted documents. It then creates the
// kernel API views (packagestore.EnsureKernelAPIViews). Seed functions
// call it after the rows are written, on both sides.
func FinalizeNodeSplit(t testing.TB, db *gorm.DB, tables ...string) {
	t.Helper()
	require.NoError(t, nodesecrets.SeedForTest(db, tables...))
	require.NoError(t, packagestore.EnsureKernelAPIViews(db))
}

// Lease answers what the kernel's storage lease grants the package whose
// manifest template is packages/<packageID>/manifest.template.json: the
// tables it adopts and the views it reads. It is a native service's
// Leased.
func Lease(packageID string) func(ctx context.Context, name string) bool {
	grants := service.StorageGrants(manifest(packageID))
	return func(_ context.Context, name string) bool {
		return slices.Contains(grants.AdoptTables, name) || slices.Contains(grants.Views, name)
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
