package nodesecretsplit

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v5/internal/packagestore"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// This file tests phase P3 of the split (section 4.3) through the kernel's
// own writers and readers: finalize, the readers on the new tables only,
// an older binary failing closed, the writers of a finalized table,
// unsplit, the moved inventory check, and the views and adoption that wait
// for finalize (section 4.6).

func finalize(t *testing.T, db *gorm.DB, tables ...string) {
	t.Helper()
	results, err := nodesecrets.Finalize(context.Background(), db, nodesecrets.FinalizeOptions{Tables: tables, Confirm: true, Actor: "nodesecretsplit"})
	require.NoError(t, err, "%+v", results)
}

// setStoredPhase writes a phase into the split's state table directly, as
// a binary that does not know the phase would act, and drops the cached
// phases.
func setStoredPhase(t *testing.T, db *gorm.DB, phase string) {
	t.Helper()
	require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("1 = 1").Update("phase", phase).Error)
	nodesecrets.ForgetPhases(db)
}

// secretsOf lists every secret value of the fleet; call it before
// finalize, while the legacy columns still hold them.
func (f *fleet) secretsOf(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var wireGuard model.NodeProtocol
	require.NoError(t, db.First(&wireGuard, f.wireGuard.ID).Error)
	values := []string{
		f.rawPrivate, f.realityPrivate,
		f.peer.PrivateKey, f.peer.PresharedKey,
	}
	for _, position := range nodesecrets.SecretPositions(*wireGuard.Settings) {
		values = append(values, strings.Trim(position.Value, `"`))
	}
	for _, value := range values {
		require.NotEmpty(t, value)
	}
	return values
}

// TestFinalizeThroughTheKernel: after finalize every kernel reader
// authenticates every node and presents every secret from the new tables
// only, without a fallback, and no tombstone authenticates; a binary that
// reads the legacy columns authenticates no one (fails closed); the
// kernel's writers keep working, writing tombstones.
func TestFinalizeThroughTheKernel(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		ctx := context.Background()
		require.NoError(t, db.AutoMigrate(&model.AgentCertificate{}, &model.AgentEnrollment{}))
		f := newFleet(t, db)
		secrets := f.secretsOf(t, db)
		_, err := nodesecrets.Backfill(ctx, db, nodesecrets.BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		setPhase(t, db, nodesecrets.PhaseDualRead)
		finalize(t, db)

		before := totalFallbacks()
		requireAll(t, f.readerResults(t, db, 1), true)
		require.Equal(t, before, totalFallbacks(), "a finalized reader never falls back")
		requireVerified(t, db)

		// No legacy column holds a secret any more.
		columns := fmt.Sprint(legacyColumns(t, db))
		for _, secret := range secrets {
			require.NotContains(t, columns, secret)
		}

		// A binary that reads the legacy columns fails closed: every reader
		// refuses.
		setStoredPhase(t, db, nodesecrets.PhaseDualWrite)
		requireAll(t, f.readerResults(t, db, 2), false)
		setStoredPhase(t, db, nodesecrets.PhaseFinalized)
		requireAll(t, f.readerResults(t, db, 3), true)

		// The writers of a finalized table keep working.
		require.NoError(t, service.NewNodeService().UpdateNode(f.rawNode.ID, map[string]any{"name": "raw-renamed"}))
		requireAll(t, f.readerResults(t, db, 4), true)

		requireVerified(t, db)
	})
}

// TestUnsplitThroughTheKernel: unsplit writes every legacy column back as
// it was before finalize, so an older binary authenticates every node
// again, and the readers in dual_read answer as before.
func TestUnsplitThroughTheKernel(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		ctx := context.Background()
		f := newFleet(t, db)
		_, err := nodesecrets.Backfill(ctx, db, nodesecrets.BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		setPhase(t, db, nodesecrets.PhaseDualRead)
		columns := legacyColumns(t, db)
		finalize(t, db)
		require.NotEqual(t, columns, legacyColumns(t, db))

		results, err := nodesecrets.Unsplit(ctx, db, nodesecrets.UnsplitOptions{Confirm: true, Actor: "nodesecretsplit", DropViews: packagestore.DropFinalizedViews})
		require.NoError(t, err, "%+v", results)
		require.Equal(t, columns, legacyColumns(t, db), "the legacy columns as they were")
		olderBinaryAuthenticates(t, db, f)
		before := totalFallbacks()
		requireAll(t, f.readerResults(t, db, 1), true)
		require.Equal(t, before, totalFallbacks())
		requireVerified(t, db)
	})
}

// splitViews are the views of section 4.6 that wait for finalize.
var splitViews = []string{
	"kapi_node_public_v1", "kapi_node_protocol_public_v1", "kapi_wireguard_peer_v1",
}

// movedColumns are the columns no view may show.
var movedColumns = map[string]bool{
	"api_key": true, "api_key_hash": true, "secret": true, "key": true, "key_hash": true, "api_token": true,
	"token": true, "private_key": true, "preshared_key": true, "value": true,
}

func viewExists(t *testing.T, db *gorm.DB, view string) bool {
	t.Helper()
	query := "SELECT count(*) FROM sqlite_master WHERE type = 'view' AND name = ?"
	if db.Name() == "postgres" {
		query = "SELECT count(*) FROM pg_views WHERE schemaname = current_schema() AND viewname = ?"
	}
	var count int64
	require.NoError(t, db.Raw(query, view).Scan(&count).Error)
	return count > 0
}

// TestSplitViewsAndAdoptionWaitForFinalize: the views of the split's
// remainder do not exist before finalize, and a lease leaves them and the
// adoption of a split table out; after finalize they exist, show no moved
// column and no secret, and the lease honours the grants; unsplit drops
// the views that showed a formerly secret column, and refuses adoption
// again.
func TestSplitViewsAndAdoptionWaitForFinalize(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		ctx := context.Background()
		f := newFleet(t, db)
		secrets := f.secretsOf(t, db)
		requested := service.PackageStorageGrants{Storage: true,
			AdoptTables: []string{"v2_forward_tunnel", "v2_node", "v2_node_protocol"}, Views: append([]string{"kapi_forward_node_v1"}, splitViews...)}
		grants := func() service.PackageStorageGrants {
			t.Helper()
			effective, err := service.EffectiveStorageGrants(db, requested)
			require.NoError(t, err)
			return effective
		}

		require.NoError(t, packagestore.EnsureKernelAPIViews(db))
		for _, view := range splitViews {
			require.False(t, viewExists(t, db, view), view)
		}
		require.Equal(t, service.PackageStorageGrants{Storage: true, AdoptTables: []string{"v2_forward_tunnel"},
			Views: []string{"kapi_forward_node_v1"}}, grants(), "before finalize the lease leaves them out")

		_, err := nodesecrets.Backfill(ctx, db, nodesecrets.BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		setPhase(t, db, nodesecrets.PhaseDualRead)
		require.NoError(t, packagestore.EnsureKernelAPIViews(db))
		for _, view := range splitViews {
			require.False(t, viewExists(t, db, view), "%s in dual_read", view)
		}
		require.Equal(t, []string{"v2_forward_tunnel"}, grants().AdoptTables)

		finalize(t, db, nodesecrets.TableNode)
		require.Equal(t, []string{"v2_forward_tunnel", "v2_node"}, grants().AdoptTables, "one table finalized, one adoption")
		finalize(t, db)
		require.NoError(t, packagestore.EnsureKernelAPIViews(db))
		require.Equal(t, requested, grants())

		for _, view := range splitViews {
			require.True(t, viewExists(t, db, view), view)
			var rows []map[string]any
			require.NoError(t, db.Table(view).Find(&rows).Error)
			require.NotEmpty(t, rows, view)
			for _, row := range rows {
				for column, value := range row {
					require.False(t, movedColumns[column], "%s shows %s", view, column)
					text := fmt.Sprint(value)
					if bytes, ok := value.([]byte); ok {
						text = string(bytes)
					}
					for _, secret := range secrets {
						require.NotContains(t, text, secret, "%s.%s", view, column)
					}
					require.False(t, nodesecrets.IsTombstone(text) && column != "raw_config", "%s.%s", view, column)
				}
			}
		}
		_, err = nodesecrets.Unsplit(ctx, db, nodesecrets.UnsplitOptions{Confirm: true, DropViews: packagestore.DropFinalizedViews})
		require.NoError(t, err)
		for _, view := range splitViews {
			require.False(t, viewExists(t, db, view), "%s after unsplit", view)
		}
		require.Equal(t, []string{"v2_forward_tunnel"}, grants().AdoptTables, "adoption refused again")
	})
}
