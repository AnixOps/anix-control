package protocolruntimecompat

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/protocol-runtime/native"
	mirror "github.com/AnixOps/anix-control/v4/packages/protocol-runtime/native/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The node protocol routes run through the whole path a deployment runs
// (packagecompat.RunKernelRead and RunKernelWrite): the kernel's gateway
// seals the secrets an administrator types into handles, the package host
// reads v2_node_protocol, which its lease adopts once the node credential
// split finalized it, and writes the rows; the kernel validates and stores
// the secrets (PutSecretDocument) and retires a deleted protocol's peers,
// links and secrets (RetireProtocol). Both sides start from the same rows,
// finalized; the answers are the same bytes and the rows, the split
// table's secrets, the peers and the links end the same.

// WireGuard key pairs, one for the seeded protocol and one an
// administrator types, generated once so both sides store the same.
var wireGuard = func() (pair struct{ private, public, otherPrivate, otherPublic string }) {
	var err error
	if pair.private, pair.public, err = service.GenerateWireGuardKeypair(); err != nil {
		panic(err)
	}
	if pair.otherPrivate, pair.otherPublic, err = service.GenerateWireGuardKeypair(); err != nil {
		panic(err)
	}
	return pair
}()

var protocolModels = []any{
	&model.Node{}, &model.NodeProtocol{}, &model.WireGuardPeer{}, &model.SubscriptionGroup{}, &model.SubscriptionTemplate{},
	&model.User{}, &model.AgentDiagnosticTask{},
}

func protocolCapabilities() []string {
	return []string{service.CapabilityNodeOpsNodeConfig, service.CapabilityNodeOpsAgents, service.CapabilityNodeOpsDiagnose}
}

// protocolRoute is a node protocol route on the kernel's path.
func protocolRoute(method, pattern, routeID string, legacy func(*handler.NodeHandler, *gin.Context)) packagecompat.KernelRoute {
	models := append(append(append([]any{}, protocolModels...), packagecompat.NodeSplitModels()...), model.KernelNodeOperationModels()...)
	return packagecompat.KernelRoute{
		Route: packagecompat.Route{
			Method: method, Pattern: pattern, RouteID: routeID, Models: models,
			// The legacy handler is built per request: it keeps the
			// database it was built with.
			Legacy: func(c *gin.Context) { legacy(handler.NewNodeHandler(), c) },
		},
		PackageID: "protocol-runtime", Capabilities: protocolCapabilities(),
		NativeService: func(db *gorm.DB, nodeOps kernelnodeopsv1.KernelNodeOpsClient) pluginhostsdk.NativeHandler {
			return (&native.Service{
				Open:    func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
				Leased:  packagecompat.Lease(db, "protocol-runtime"),
				NodeOps: nodeOps,
			}).Handlers()[routeID]
		},
	}
}

func ptr[T any](value T) *T { return &value }

func wireGuardSettings(private, public string) string {
	return fmt.Sprintf(`{"cidr":"10.8.0.0/24","server_address":"10.8.0.1/24","server_private_key":%q,"server_public_key":%q,"tunnel_type":"quic",`+
		`"relay":{"server":"relay.example.test","server_port":8443}}`, private, public)
}

// seedNodes writes nodes 1 to 3 and the protocols of nodes 1 and 2:
// Reality with its private key, a WireGuard server with its key pair and
// its users' peers, TLS with a key, a hidden one without secrets; two
// subscription groups linked to them. Times are fixed.
func seedNodes(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.Create(&[]model.Node{
		{ID: 1, Name: "edge-1", Host: "edge-1.example.test", APIKey: "fake-key-1", Secret: "fake-secret-1", CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, Name: "edge-2", Host: "edge-2.example.test", APIKey: "fake-key-2", Secret: "fake-secret-2", CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 3, Name: "edge-3", Host: "edge-3.example.test", APIKey: "fake-key-3", Secret: "fake-secret-3", CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "admin@example.test", Token: "token-1", UUID: "uuid-1", CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, Email: "user@example.test", Token: "token-2", UUID: "uuid-2", CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	protocols := []model.NodeProtocol{
		{ID: 1, NodeID: 1, Name: "reality", Type: model.ProtocolVLESS, Port: 443, Sort: 2, TLS: 2, Host: ptr("cdn.example.test"),
			Settings:        ptr(`{"clients":[{"id":"u","flow":"xtls-rprx-vision"}],"decryption":"none"}`),
			RealitySettings: ptr(`{"dest":"www.example.test:443","private_key":"fake-reality-private-1","public_key":"pub-1","short_ids":["ab"]}`)},
		{ID: 2, NodeID: 1, Name: "wg", Type: model.ProtocolWireGuard, Port: 51820, Sort: 1,
			Settings: ptr(wireGuardSettings(wireGuard.private, wireGuard.public))},
		{ID: 3, NodeID: 1, Name: "trojan", Type: model.ProtocolTrojan, Port: 8443, Sort: 1, TLS: 1,
			TLSSettings:  ptr(`{"server_name":"edge-1.example.test","private_key":"fake-tls-key","key_file":"/etc/x.pem"}`),
			CustomConfig: ptr(`{"log":{"level":"warning"},"password":"fake-custom-password"}`)},
		{ID: 4, NodeID: 2, Name: "plain", Type: model.ProtocolVMess, Port: 10086, Transport: ptr("ws"), TransportSettings: ptr(`{"path":"/ws"}`)},
	}
	for i := range protocols {
		protocols[i].CreatedAt, protocols[i].UpdatedAt = seeded, seeded.Add(time.Minute)
	}
	require.NoError(t, db.Create(&protocols).Error)
	require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", 4).UpdateColumn("show", 0).Error)
	require.NoError(t, db.Create(&[]model.WireGuardPeer{
		{ID: 1, NodeProtocolID: 2, UserID: 1, PeerIP: "10.8.0.2", PrivateKey: "fake-peer-private-1", PublicKey: "peer-pub-1", PresharedKey: "fake-psk-1", CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, NodeProtocolID: 2, UserID: 2, PeerIP: "10.8.0.3", PrivateKey: "fake-peer-private-2", PublicKey: "peer-pub-2", PresharedKey: "fake-psk-2", CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	require.NoError(t, db.Create(&[]model.SubscriptionGroup{
		{ID: 1, Name: "default", Enable: 1, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, Name: "premium", Enable: 1, CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	require.NoError(t, db.Exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (1, 1), (1, 2), (2, 2), (2, 4)").Error)
	syncSequences(t, db, "v2_node", "v2_user", "v2_node_protocol", "v2_wireguard_peer", "v2_subscription_group")
}

// seedFinalized is seedNodes with v2_node_protocol (and the WireGuard
// peers, whose keys it holds) finalized.
func seedFinalized(t testing.TB, db *gorm.DB) {
	seedNodes(t, db)
	packagecompat.FinalizeNodeSplit(t, db, nodesecrets.TableNodeProtocol, nodesecrets.TableWireGuardPeer)
}

func syncSequences(t testing.TB, db *gorm.DB, tables ...string) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range tables {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

// protocolState is everything the protocol routes can change: the rows as
// stored (finalized: redacted), the secrets the kernel keeps for them, the
// WireGuard peers and the group links. Times the handlers take from their
// clock read as "<clock>".
func protocolState(t testing.TB, db *gorm.DB) any {
	var protocols []model.NodeProtocol
	require.NoError(t, db.Order("id").Find(&protocols).Error)
	rows := make([]map[string]any, 0, len(protocols))
	for _, protocol := range protocols {
		created, updated := protocol.CreatedAt, protocol.UpdatedAt
		protocol.CreatedAt, protocol.UpdatedAt = time.Time{}, time.Time{}
		rows = append(rows, map[string]any{"row": protocol, "created_at": clock(created), "updated_at": clock(updated)})
	}
	var secrets []struct {
		Scope, ColumnName, JSONPointer, Value string
		OwnerID                               uint
	}
	require.NoError(t, db.Model(&model.ProtocolSecret{}).Where("scope <> ?", "legacy_original").
		Order("scope, owner_id, column_name, json_pointer").Find(&secrets).Error)
	var peers []struct {
		ID, NodeProtocolID, UserID uint
		PeerIP                     string
	}
	require.NoError(t, db.Model(&model.WireGuardPeer{}).Order("id").Find(&peers).Error)
	var links []struct{ SubscriptionGroupID, NodeProtocolID uint }
	require.NoError(t, db.Table("v2_subscription_group_node_protocols").Order("subscription_group_id, node_protocol_id").Find(&links).Error)
	return map[string]any{"protocols": rows, "secrets": secrets, "peers": peers, "links": links}
}

// clock is a time a handler takes from its clock, or a seeded one.
func clock(value time.Time) string {
	if value.Sub(seeded) >= 0 && value.Sub(seeded) <= time.Hour {
		return value.UTC().Format(time.RFC3339)
	}
	return "<clock>"
}

func kernelRead(t *testing.T, r packagecompat.KernelRoute, cases []packagecompat.Case) {
	for _, c := range cases {
		c.Principal = admin
		if c.Seed == nil {
			c.Seed = seedFinalized
		}
		packagecompat.RunKernelRead(t, r, c)
	}
}

func kernelWrite(t *testing.T, r packagecompat.KernelRoute, cases []packagecompat.Case) {
	for _, c := range cases {
		c.Principal = admin
		if c.Seed == nil {
			c.Seed = seedFinalized
		}
		c.Snapshot = protocolState
		packagecompat.RunKernelWrite(t, r, c)
	}
}

func TestNodeProtocolsReadParity(t *testing.T) {
	path := func(id string) string { return "/api/v2/admin/nodes/" + id + "/protocols" }
	kernelRead(t, protocolRoute("GET", "/api/v2/admin/nodes/:id/protocols", native.ProtocolsRouteID, (*handler.NodeHandler).GetProtocols),
		[]packagecompat.Case{
			{Name: "by sort and id, secrets masked", Path: path("1")},
			{Name: "a hidden protocol without secrets", Path: path("2")},
			{Name: "a node without protocols", Path: path("3")},
			{Name: "an unknown node", Path: path("99")},
			{Name: "not a number", Path: path("x")},
			{Name: "beyond 32 bits", Path: path("4294967297")},
			{Name: "before finalize the kernel answers", Path: path("1"), Seed: seedNodes, Fallback: true},
		})
}

func TestNodeProtocolCreateParity(t *testing.T) {
	pattern := "/api/v2/admin/nodes/:id/protocols"
	path := func(id string) string { return "/api/v2/admin/nodes/" + id + "/protocols" }
	stamped := []string{"data.created_at", "data.updated_at"}
	body := func(value map[string]any) []byte {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		return encoded
	}
	kernelWrite(t, protocolRoute("POST", pattern, native.CreateProtocolRouteID, (*handler.NodeHandler).CreateProtocol), []packagecompat.Case{
		{Name: "reality with a typed private key", Path: path("2"), Mask: stamped, Body: body(map[string]any{
			"name": "new-reality", "type": "vless", "port": 2443, "tls": 2, "sort": 3,
			"reality_settings": `{"dest":"www.example.test:443","private_key":"typed-reality-private","public_key":"pub-new","short_ids":["cd"]}`,
			"settings":         `{"clients":[{"id":"x","password":"typed-client-password"}]}`,
		})},
		{Name: "a placeholder stores nothing", Path: path("2"), Mask: stamped, Body: body(map[string]any{
			"name": "masked", "type": "trojan", "port": 3443,
			"tls_settings": `{"server_name":"x.example.test","private_key":"` + service.NodeSecretPlaceholder + `"}`,
		})},
		{Name: "every secret column", Path: path("3"), Mask: stamped, Body: body(map[string]any{
			"name": "all", "type": "vless", "port": 4443, "enable": 0, "show": 0, "host": "h.example.test", "alpn": "h2", "transport": "grpc",
			"settings":           `{"password":"p1"}`,
			"tls_settings":       `{"private_key":"p2"}`,
			"transport_settings": `{"service_name":"grpc","token":"p3"}`,
			"reality_settings":   `{"private_key":"p4","short_ids":["ef"]}`,
			"custom_config":      `{"secret":"p5","inbounds":[]}`,
		})},
		{Name: "a WireGuard server with its key pair", Path: path("2"), Mask: stamped, Body: body(map[string]any{
			"name": "wg-new", "type": "wireguard", "port": 51821, "settings": wireGuardSettings(wireGuard.otherPrivate, wireGuard.otherPublic),
		})},
		{Name: "a WireGuard key pair that does not match", Path: path("2"), Body: body(map[string]any{
			"name": "wg-bad", "type": "wireguard", "port": 51822, "settings": wireGuardSettings(wireGuard.otherPrivate, wireGuard.public),
		})},
		{Name: "a WireGuard server without its private key", Path: path("2"), Body: body(map[string]any{
			"name": "wg-none", "type": "wireguard", "port": 51823, "settings": wireGuardSettings(service.NodeSecretPlaceholder, wireGuard.public),
		})},
		{Name: "a WireGuard port out of range", Path: path("2"), Body: body(map[string]any{
			"name": "wg-port", "type": "wireguard", "port": 0, "settings": wireGuardSettings(wireGuard.otherPrivate, wireGuard.otherPublic),
		})},
		{Name: "the body's node and groups are not written", Path: path("2"), Mask: stamped, Body: body(map[string]any{
			"name": "nested", "type": "vmess", "port": 5443, "id": 77, "node_id": 3,
			"node": map[string]any{"id": 9, "name": "planted"}, "subscription_groups": []map[string]any{{"id": 1}, {"name": "new-group"}},
		})},
		{Name: "an unknown node", Path: path("99"), Body: body(map[string]any{"name": "x", "type": "vless", "port": 1})},
		{Name: "a node id that is not a number", Path: path("x"), Body: body(map[string]any{"name": "x"})},
		{Name: "a port that is a string", Path: path("2"), Body: []byte(`{"name":"x","port":"443"}`)},
		{Name: "a type that is a number", Path: path("2"), Body: []byte(`{"name":"x","type":7}`)},
		// A body the gateway cannot seal is served by the kernel's legacy
		// handler, without the package host.
		{Name: "not JSON", Path: path("2"), Body: []byte(`{"name":`), Fallback: true},
		{Name: "no body", Path: path("2")},
		{Name: "a custom configuration that is not JSON is the kernel's", Path: path("2"), Mask: stamped, Fallback: true,
			Body: body(map[string]any{"name": "raw", "type": "vless", "port": 6443, "custom_config": "not json"})},
		{Name: "before finalize the kernel answers", Path: path("2"), Mask: stamped, Seed: seedNodes, Fallback: true,
			Body: body(map[string]any{"name": "early", "type": "vless", "port": 7443, "reality_settings": `{"private_key":"typed-early"}`})},
	})
}

func TestNodeProtocolUpdateParity(t *testing.T) {
	pattern := "/api/v2/admin/nodes/:id/protocols/:protocol_id"
	path := func(id string) string { return "/api/v2/admin/nodes/1/protocols/" + id }
	body := func(value map[string]any) []byte {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		return encoded
	}
	kernelWrite(t, protocolRoute("PUT", pattern, native.UpdateProtocolRouteID, (*handler.NodeHandler).UpdateProtocol), []packagecompat.Case{
		{Name: "a field without secrets", Path: path("1"), Body: body(map[string]any{"name": "renamed", "sort": 9})},
		{Name: "the placeholder keeps the private key", Path: path("1"), Body: body(map[string]any{
			"reality_settings": map[string]any{"dest": "other.example.test:443", "private_key": service.NodeSecretPlaceholder, "public_key": "pub-2", "short_ids": []string{"ab"}},
		})},
		{Name: "a typed private key", Path: path("1"), Body: body(map[string]any{
			"reality_settings": `{"dest":"www.example.test:443","private_key":"typed-reality-private-2","public_key":"pub-1"}`,
		})},
		{Name: "a document without the secret clears it", Path: path("3"), Body: body(map[string]any{"tls_settings": `{"server_name":"edge-1.example.test"}`})},
		{Name: "null clears a column", Path: path("3"), Body: body(map[string]any{"custom_config": nil, "tls_settings": nil})},
		{Name: "a WireGuard server keeps its private key", Path: path("2"), Body: body(map[string]any{
			"port": 51830, "settings": wireGuardSettings(service.NodeSecretPlaceholder, wireGuard.public),
		})},
		{Name: "a kept private key that does not match a new public key", Path: path("2"), Body: body(map[string]any{
			"settings": wireGuardSettings(service.NodeSecretPlaceholder, wireGuard.otherPublic),
		})},
		{Name: "a typed WireGuard key pair", Path: path("2"), Body: body(map[string]any{
			"settings": wireGuardSettings(wireGuard.otherPrivate, wireGuard.otherPublic),
		})},
		{Name: "a WireGuard port out of range", Path: path("2"), Body: body(map[string]any{"port": 70000})},
		{Name: "the id and node are never changed", Path: path("4"), Body: body(map[string]any{"id": 9, "NodeID": 3, "node_id": 2, "Name": "kept"})},
		{Name: "a key named twice", Path: path("4"), Body: body(map[string]any{"name": "a", "Name": "b"})},
		{Name: "a value of the wrong type", Path: path("4"), Body: body(map[string]any{"port": "x"})},
		{Name: "an unknown protocol", Path: path("99"), Body: body(map[string]any{"name": "x"})},
		// The gateway binds handles to the protocol the path names: a path
		// that names none is served by the kernel's legacy handler.
		{Name: "a protocol id that is not a number", Path: path("x"), Body: body(map[string]any{"name": "x"}), Fallback: true},
		{Name: "not JSON", Path: path("1"), Body: []byte(`{"name":`), Fallback: true},
		{Name: "a document that is not JSON is the kernel's", Path: path("4"), Fallback: true, Body: body(map[string]any{"custom_config": "not json"})},
		{Name: "before finalize the kernel answers", Path: path("1"), Seed: seedNodes, Fallback: true, Body: body(map[string]any{"name": "early"})},
	})
}

func TestNodeProtocolDeleteParity(t *testing.T) {
	pattern := "/api/v2/admin/nodes/:id/protocols/:protocol_id"
	path := func(id string) string { return "/api/v2/admin/nodes/1/protocols/" + id }
	kernelWrite(t, protocolRoute("DELETE", pattern, native.DeleteProtocolRouteID, (*handler.NodeHandler).DeleteProtocol), []packagecompat.Case{
		{Name: "with its peers, links and secrets", Path: path("2")},
		{Name: "with its secrets", Path: path("1")},
		{Name: "without secrets", Path: path("4")},
		{Name: "an unknown protocol", Path: path("99")},
		{Name: "a protocol id that is not a number", Path: path("x")},
		{Name: "before finalize the kernel answers", Path: path("2"), Seed: seedNodes, Fallback: true},
	})
}

// No typed secret reaches the package: the native handlers of the writes
// are sent handles only, and the kernel holds none once a request ended.
func TestTypedSecretsNeverReachThePackage(t *testing.T) {
	db := packagecompat.OpenSQLite(t, protocolRoute("POST", "/", native.CreateProtocolRouteID, nil).Models...)
	seedFinalized(t, db)
	route := protocolRoute("POST", "/api/v2/admin/nodes/:id/protocols", native.CreateProtocolRouteID, (*handler.NodeHandler).CreateProtocol)
	kernel := packagecompat.StartKernel(t, packagecompat.KernelOptions{
		DB: db, PackageID: route.PackageID, Capabilities: route.Capabilities,
		Routes: []packagecompat.KernelHostRoute{{Method: route.Method, Pattern: route.Pattern, RouteID: route.RouteID, Legacy: route.Legacy,
			Native: func(nodeOps kernelnodeopsv1.KernelNodeOpsClient) pluginhostsdk.NativeHandler {
				return route.NativeService(db, nodeOps)
			}}},
	})
	answer := kernel.Do(t, "POST", "/api/v2/admin/nodes/2/protocols",
		[]byte(`{"name":"r","type":"vless","port":1443,"reality_settings":"{\"private_key\":\"typed-secret-7\"}"}`), nil)
	require.Equal(t, 200, answer.StatusCode, "%s", answer.Body)
	require.NotContains(t, string(answer.Body), "typed-secret-7")
	received := kernel.Received(route.RouteID)
	require.Len(t, received, 1)
	require.NotContains(t, string(received[0].Body), "typed-secret-7", "the package reads a handle")
	require.True(t, strings.Contains(string(received[0].Body), "anix-sealed:v1:"))
	var stored model.NodeProtocol
	require.NoError(t, db.Order("id DESC").First(&stored).Error)
	require.NotContains(t, *stored.RealitySettings, "typed-secret-7", "a finalized column holds the placeholder")
	var secret model.ProtocolSecret
	require.NoError(t, db.Where("owner_id = ? AND column_name = ?", stored.ID, "reality_settings").First(&secret).Error)
	require.Equal(t, `"typed-secret-7"`, secret.Value, "the kernel keeps the secret")
	var ledger []model.KernelNodeOperation
	require.NoError(t, db.Find(&ledger).Error)
	encoded, err := json.Marshal(ledger)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "typed-secret-7")
	require.NotContains(t, string(encoded), "anix-sealed:v1:"+"x", "the ledger keeps no handle")
	require.Zero(t, kernel.PendingHandles())
}

// The package's model mirrors the kernel's: the same type names, fields,
// JSON tags, and for the adopted v2_node_protocol the same gorm tags, so it
// writes what the kernel writes and binds what the kernel binds.
func TestModelMirrorsTheKernelModel(t *testing.T) {
	pairs := []struct {
		kernel, mirror any
		gorm           bool
	}{
		{kernel: model.NodeProtocol{}, mirror: mirror.NodeProtocol{}, gorm: true},
		{kernel: model.Node{}, mirror: mirror.Node{}},
		{kernel: model.SubscriptionGroup{}, mirror: mirror.SubscriptionGroup{}, gorm: true},
		{kernel: model.SubscriptionTemplate{}, mirror: mirror.SubscriptionTemplate{}, gorm: true},
	}
	for _, pair := range pairs {
		kernel, copied := reflect.TypeOf(pair.kernel), reflect.TypeOf(pair.mirror)
		require.Equal(t, kernel.String(), copied.String())
		var kernelFields, mirrorFields []string
		for i := range kernel.NumField() {
			field := kernel.Field(i)
			if field.Tag.Get("json") == "-" {
				continue
			}
			kernelFields = append(kernelFields, field.Name)
			other, ok := copied.FieldByName(field.Name)
			require.True(t, ok, "%s.%s", kernel, field.Name)
			require.Equal(t, field.Tag.Get("json"), other.Tag.Get("json"), "%s.%s", kernel, field.Name)
			require.Equal(t, field.Type.String(), other.Type.String(), "%s.%s", kernel, field.Name)
			if pair.gorm {
				require.Equal(t, field.Tag.Get("gorm"), other.Tag.Get("gorm"), "%s.%s", kernel, field.Name)
			}
		}
		for i := range copied.NumField() {
			mirrorFields = append(mirrorFields, copied.Field(i).Name)
		}
		require.Equal(t, kernelFields, mirrorFields, "%s fields in order", kernel)
	}
	require.Equal(t, reflect.TypeOf(model.ProtocolType("")).String(), reflect.TypeOf(mirror.ProtocolType("")).String())
	require.Equal(t, reflect.TypeOf(model.NodeStatus(0)).String(), reflect.TypeOf(mirror.NodeStatus(0)).String())
}
