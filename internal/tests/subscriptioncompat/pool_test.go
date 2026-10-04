package subscriptioncompat

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/subscription/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A group's protocols and the protocol pool read kapi_node_protocol_public_v1
// and kapi_node_public_v1, which exist, and which the kernel grants the
// package, only once v2_node_protocol and v2_node are finalized
// (docs/architecture/node-ops-service.md section 4.3). The native side's
// lease is what the kernel grants the package's manifest on its database
// (packagecompat.Lease), so these cases also prove the grant.

// poolRoute is a protocol pool route: both databases have the node
// credential split's tables.
func poolRoute(method, pattern, routeID string, legacy func(*handler.SubscriptionAdminHandler, *gin.Context)) packagecompat.Route {
	r := route(method, pattern, routeID, legacy)
	r.Models = append(r.Models, packagecompat.NodeSplitModels()...)
	r.Native = func(db *gorm.DB) pluginhostsdk.NativeHandler {
		return poolService(db).Handlers()[routeID]
	}
	return r
}

func poolService(db *gorm.DB) *native.Service {
	return &native.Service{
		Open:   func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
		Leased: packagecompat.Lease(db, "subscription"),
	}
}

// seedPool is seed with secrets in every protocol column and in the nodes'
// raw configurations and a protocol hidden from subscriptions, then the two
// tables finalized.
func seedPool(t testing.TB, db *gorm.DB) {
	seed(t, db)
	require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", 1).UpdateColumns(map[string]any{
		"settings":           `{"clients":[{"id":"u","password":"pw"}],"decryption":"none"}`,
		"tls_settings":       `{"server_name":"edge.example.test","private_key":"tls-key","key_file":"/etc/k.pem"}`,
		"transport_settings": `{"path":"/ws","headers":{"Host":"edge"}}`,
		"reality_settings":   `{"private_key":"reality-private","public_key":"reality-public","short_ids":["ab"]}`,
		"custom_config":      `not json`,
		"host":               "cdn.example.test", "alpn": "h2", "tls": 2, "transport": "ws", "group_id": 2,
	}).Error)
	require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", 3).UpdateColumns(map[string]any{
		"settings": `{"server_private_key":"wg-private","server_public_key":"wg-public","peers":[{"preshared_key":"psk"}]}`, "sort": 2,
	}).Error)
	require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", 2).UpdateColumn("show", 0).Error)
	require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", 5).UpdateColumn("sort", -1).Error)
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", 1).UpdateColumns(map[string]any{
		"raw_config": `{"wireguard":{"private_key":"raw-private","listen_port":51820},"log":{"level":"info"}}`,
		"tags":       `["hk"]`, "group_id": 2, "parent_id": 3, "monthly_limit": int64(1 << 40), "server_os": "linux",
	}).Error)
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", 2).UpdateColumn("raw_config", `{"log":{"level":"debug"}}`).Error)
	// seed's report times are relative to its clock, which the two sides
	// read a moment apart.
	require.NoError(t, db.Model(&model.Node{}).Where("last_check_at IS NOT NULL").UpdateColumn("last_check_at", seeded.Unix()).Error)
	require.NoError(t, db.Exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (3, 4)").Error)
	packagecompat.FinalizeNodeSplit(t, db, "v2_node", "v2_node_protocol")
	syncSequences(t, db)
}

func TestGroupProtocolsParity(t *testing.T) {
	path := func(id string) string { return "/api/v2/admin/subscription/groups/" + id + "/protocols" }
	read(t, poolRoute("GET", "/api/v2/admin/subscription/groups/:id/protocols", native.GroupProtocolsRouteID,
		(*handler.SubscriptionAdminHandler).GetGroupProtocols),
		[]packagecompat.Case{
			{Name: "secrets and raw configuration redacted", Path: path("1"), Seed: seedPool},
			{Name: "a hidden protocol is linked too", Path: path("2"), Seed: seedPool},
			{Name: "a disabled group", Path: path("3"), Seed: seedPool},
			{Name: "no protocols", Path: path("4"), Seed: seedPool},
			{Name: "unknown group", Path: path("99"), Seed: seedPool},
			{Name: "not a number", Path: path("x"), Seed: seedPool},
			{Name: "beyond 32 bits", Path: path("4294967297"), Seed: seedPool},
		})
}

func TestAvailableProtocolsParity(t *testing.T) {
	path := "/api/v2/admin/subscription/protocols/available"
	read(t, poolRoute("GET", path, native.AvailableProtocolsRouteID, (*handler.SubscriptionAdminHandler).GetAvailableProtocols),
		[]packagecompat.Case{
			{Name: "shown protocols by node and sort, with nodes and groups", Path: path, Seed: seedPool},
			{Name: "no protocols", Path: path, Seed: func(t testing.TB, db *gorm.DB) {
				packagecompat.FinalizeNodeSplit(t, db, "v2_node", "v2_node_protocol")
			}},
		})
}

// Before the two tables are finalized the kernel neither creates the views
// nor grants them, although on SQLite the package shares the database file
// that holds the tables: the routes answer from the legacy handler.
func TestProtocolPoolStaysLegacyUntilFinalized(t *testing.T) {
	for name, seedCase := range map[string]func(testing.TB, *gorm.DB){
		"dual_write": seed,
		"only v2_node_protocol finalized": func(t testing.TB, db *gorm.DB) {
			seed(t, db)
			packagecompat.FinalizeNodeSplit(t, db, "v2_node_protocol")
		},
	} {
		t.Run(name, func(t *testing.T) {
			db := packagecompat.OpenSQLite(t, poolRoute("GET", "/", native.GroupProtocolsRouteID, nil).Models...)
			seedCase(t, db)
			handlers := poolService(db).Handlers()
			for _, routeID := range []string{native.GroupProtocolsRouteID, native.AvailableProtocolsRouteID} {
				_, err := handlers[routeID](context.Background(), pluginhostsdk.NativeRequest{
					RouteID: routeID, Metadata: pluginhostsdk.RequestMetadata{PathParams: map[string]string{"id": "1"}},
				})
				require.ErrorIs(t, err, pluginhostsdk.ErrNativeUnavailable, routeID)
			}
		})
	}
}
