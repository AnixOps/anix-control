package nodesecretsplit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/middleware"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
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
		f.created.APIKey, f.created.Secret, f.registered.APIKey, f.registered.Secret, f.rawNode.APIKey, f.rawNode.Secret,
		"fake-plain-key", f.registrationKey, f.rawPrivate, f.realityPrivate, "fake-fleet-forward-token", f.agentToken,
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

		// A tombstone presented as a key authenticates nothing.
		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.GET("/node", middleware.NodeAPIKeyAuth(), func(c *gin.Context) { c.Status(http.StatusOK) })
		for _, id := range []uint{f.plain.ID, f.created.ID, f.registered.NodeID} {
			req := httptest.NewRequest(http.MethodGet, "/node?node_id="+strconv.FormatUint(uint64(id), 10), nil)
			req.Header.Set("X-API-Key", nodesecrets.Tombstone(uint64(id)))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.NotEqual(t, http.StatusOK, rec.Code, "node %d", id)
		}

		// A binary that reads the legacy columns fails closed: every reader
		// refuses, and the legacy lookups by hash find nothing.
		setStoredPhase(t, db, nodesecrets.PhaseDualWrite)
		requireAll(t, f.readerResults(t, db, 2), false)
		var count int64
		require.NoError(t, db.Model(&model.Node{}).Where("api_key_hash IN ?", []string{sha256Hex(f.created.APIKey), sha256Hex("fake-plain-key")}).Count(&count).Error)
		require.Zero(t, count)
		require.NoError(t, db.Model(&model.AuthorizedKey{}).Where("key_hash = ?", sha256Hex(f.registrationKey)).Count(&count).Error)
		require.Zero(t, count)
		setStoredPhase(t, db, nodesecrets.PhaseFinalized)
		requireAll(t, f.readerResults(t, db, 3), true)

		// The writers of a finalized table: a new node authenticates, its
		// row keeps tombstones.
		nodes := service.NewNodeService()
		added := &model.Node{Name: "after", Host: "203.0.113.70", Port: 443}
		require.NoError(t, nodes.CreateNode(added))
		var stored model.Node
		require.NoError(t, db.First(&stored, added.ID).Error)
		require.Equal(t, nodesecrets.Tombstone(uint64(added.ID)), stored.APIKey)
		require.Equal(t, nodesecrets.Tombstone(uint64(added.ID)), stored.Secret)
		found, err := nodes.GetNodeByAPIKey(added.APIKey)
		require.NoError(t, err)
		require.Equal(t, added.ID, found.ID)

		// A forward node saved back with its tombstone (the update route
		// with the placeholder) keeps its token and its agents.
		require.NoError(t, db.Create(&model.AgentCertificate{Serial: "fake-serial-1", NodeKind: "forward", NodeID: f.forward.ID,
			Cluster: "test", EnrollmentID: "e1", IssuerKeyID: "k1", NotAfter: time.Now().Add(time.Hour)}).Error)
		forwards := service.NewForwardNodeService(db)
		forward, err := forwards.GetByID(f.forward.ID)
		require.NoError(t, err)
		require.True(t, nodesecrets.IsTombstone(forward.APIToken))
		forward.Name = "relay-renamed"
		require.NoError(t, forwards.Update(forward))
		var certificate model.AgentCertificate
		require.NoError(t, db.First(&certificate, "serial = ?", "fake-serial-1").Error)
		require.Nil(t, certificate.RevokedAt, "an unchanged token revokes no agent")
		require.NoError(t, db.First(forward, f.forward.ID).Error)
		require.Equal(t, "fake-fleet-forward-token", nodesecrets.ForwardNodeToken(db, forward))
		requireAll(t, f.readerResults(t, db, 4), true)

		// The default registration key from the environment is found
		// through the split: created once.
		t.Setenv("NODE_DEFAULT_AUTH_KEY", "fake-default-auth-key")
		service.InitDefaultAuthKeyFromEnv()
		service.InitDefaultAuthKeyFromEnv()
		require.NoError(t, db.Model(&model.AuthorizedKey{}).Where("name = ?", "Default (from env)").Count(&count).Error)
		require.EqualValues(t, 1, count)
		requireVerified(t, db)
	})
}

// TestInventoryReadsTokenPresenceThroughTheSplit: whether a forward node
// has a token (an Ansible machine has none) comes from the legacy column
// before dual_read, and from a live credential row since; a tombstone is
// a token, an emptied column is none.
func TestInventoryReadsTokenPresenceThroughTheSplit(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		ctx := context.Background()
		forwards := service.NewForwardNodeService(db)
		withToken := &model.ForwardNode{Name: "tokened", Type: model.ForwardNodeTypeRelay, Host: "198.51.100.60", Port: 443, APIToken: "fake-inventory-token"}
		machine := &model.ForwardNode{Name: "machine", Type: model.ForwardNodeTypeRelay, Host: "198.51.100.61", Port: 443}
		require.NoError(t, forwards.Create(withToken))
		require.NoError(t, forwards.Create(machine))
		inventory := func() (ansible, nodex []uint) {
			t.Helper()
			for scope, out := range map[string]*[]uint{service.ForwardNodeInventoryScopeAnsible: &ansible, service.ForwardNodeInventoryScopeNodeX: &nodex} {
				nodes, total, err := forwards.ListByInventoryScope(scope, "", nil, 1, 50)
				require.NoError(t, err)
				require.EqualValues(t, len(nodes), total)
				for _, node := range nodes {
					*out = append(*out, node.ID)
				}
			}
			return ansible, nodex
		}
		check := func(stage string) {
			t.Helper()
			ansible, nodex := inventory()
			require.Equal(t, []uint{machine.ID}, ansible, stage)
			require.Equal(t, []uint{withToken.ID}, nodex, stage)
			unpinned, err := nodesecrets.ForwardNodesWithoutAPIPort(ctx, db)
			require.NoError(t, err)
			require.Equal(t, []nodesecrets.UnpinnedForwardNode{{ID: withToken.ID, Name: withToken.Name}}, unpinned, stage)
		}
		check("dual_write")
		_, err := nodesecrets.Backfill(ctx, db, nodesecrets.BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		setPhase(t, db, nodesecrets.PhaseDualRead)
		check("dual_read")
		// In dual_read the credential row decides, not the column.
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", machine.ID).UpdateColumn("api_token", "fake-column-only").Error)
		check("dual_read, a value only the legacy column holds")
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", machine.ID).UpdateColumn("api_token", "").Error)
		finalize(t, db)
		check("finalized")
		// The token cleared by a writer: the node joins the machines.
		node, err := forwards.GetByID(withToken.ID)
		require.NoError(t, err)
		node.APIToken = ""
		require.NoError(t, forwards.Update(node))
		ansible, _ := inventory()
		require.ElementsMatch(t, []uint{machine.ID, withToken.ID}, ansible)
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
	"kapi_node_public_v1", "kapi_node_protocol_public_v1", "kapi_node_credential_status_v1",
	"kapi_registration_key_v1", "kapi_forward_clean_agent_v1", "kapi_wireguard_peer_v1",
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
			AdoptTables: []string{"v2_forward_tunnel", "v2_node", "v2_node_protocol", "v2_forward_node"}, Views: append([]string{"kapi_forward_node_v1"}, splitViews...)}
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
		var status []struct {
			SubjectKind string
			HasValue    bool
		}
		require.NoError(t, db.Table("kapi_node_credential_status_v1").Where("subject_kind = ?", "forward").Find(&status).Error)
		require.Equal(t, 1, len(status))
		require.True(t, status[0].HasValue, "a forward node's token shows as present")
		var registration []struct{ HasKey bool }
		require.NoError(t, db.Table("kapi_registration_key_v1").Find(&registration).Error)
		require.True(t, registration[0].HasKey)

		_, err = nodesecrets.Unsplit(ctx, db, nodesecrets.UnsplitOptions{Confirm: true, DropViews: packagestore.DropFinalizedViews})
		require.NoError(t, err)
		for _, view := range splitViews {
			require.Equal(t, view == "kapi_node_credential_status_v1", viewExists(t, db, view), "%s after unsplit", view)
		}
		require.Equal(t, []string{"v2_forward_tunnel"}, grants().AdoptTables, "adoption refused again")
	})
}
