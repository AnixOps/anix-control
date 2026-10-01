package nodesecretsplit

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/middleware"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fleet is every kind of subject the readers authenticate or present a
// secret of, written by the kernel's own writers (and one row written
// before P1). Every secret is fake.
type fleet struct {
	plain           model.Node // written before P1: no hash
	created         model.Node // CreateNode: a Reality protocol
	registered      model.NodeRegisterResponse
	registrationKey string
	rawNode         model.Node // a raw configuration with a private key
	rawPrivate      string
	reality         model.NodeProtocol
	realityPrivate  string
	forward         model.ForwardNode
	agent           model.ForwardCleanAgent
	agentToken      string
	wireGuard       model.NodeProtocol
	user            model.User
	peer            model.WireGuardPeer
}

func base64Key(t *testing.T, encoding *base64.Encoding) string {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return encoding.EncodeToString(key)
}

func newFleet(t *testing.T, db *gorm.DB) *fleet {
	t.Helper()
	f := &fleet{}
	nodes := service.NewNodeService()

	f.plain = model.Node{Name: "plain", Host: "203.0.113.50", Port: 443, APIKey: "fake-plain-key"}
	require.NoError(t, db.Create(&f.plain).Error)

	f.created = model.Node{Name: "created", Host: "203.0.113.51", Port: 443}
	require.NoError(t, nodes.CreateNode(&f.created))
	f.realityPrivate = base64Key(t, base64.RawURLEncoding)
	f.reality = model.NodeProtocol{NodeID: f.created.ID, Name: "reality", Type: model.ProtocolVLESS, Port: 8443, TLS: 2, Enable: 1,
		RealitySettings: ptr(`{"private_key":"` + f.realityPrivate + `","short_id":"6ba85179","dest":"www.example.test:443"}`)}
	require.NoError(t, nodes.CreateProtocol(&f.reality))

	var err error
	_, f.registrationKey, err = nodes.GenerateAuthKey("fleet", 0)
	require.NoError(t, err)
	registered, err := nodes.RegisterNode(&model.NodeRegisterRequest{AuthKey: f.registrationKey, Name: "registered", Port: 443}, "192.0.2.50")
	require.NoError(t, err)
	f.registered = *registered

	f.rawNode = model.Node{Name: "raw", Host: "203.0.113.52", Port: 443}
	require.NoError(t, nodes.CreateNode(&f.rawNode))
	f.rawPrivate = base64Key(t, base64.RawURLEncoding)
	raw := `{"node_type":"vless","type":"vless","server_port":443,"tls":2,"tls_settings":{"private_key":"` + f.rawPrivate + `","short_id":"01"}}`
	require.NoError(t, nodes.UpdateNode(f.rawNode.ID, map[string]any{"raw_config": raw}))

	// The agent channels resolve a node id against the forward nodes first,
	// so the forward node's id stays clear of the proxy nodes'.
	f.forward = model.ForwardNode{ID: 900, Name: "relay", Type: "relay", Host: "198.51.100.50", Port: 443, APIPort: 9000, APIToken: "fake-fleet-forward-token", Enabled: true}
	require.NoError(t, service.NewForwardNodeService(db).Create(&f.forward))
	issued, err := service.NewForwardCleanAgentService(db).CreateToken(service.ForwardCleanAgentCreateInput{Name: "agent", NodeID: &f.forward.ID})
	require.NoError(t, err)
	f.agent, f.agentToken = *issued.Agent, issued.Token

	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	// Disabled, so it needs no relay; its peers are kept all the same.
	f.wireGuard = model.NodeProtocol{NodeID: f.created.ID, Name: "wg", Type: model.ProtocolWireGuard, Port: 51820,
		Settings: ptr(`{"cidr":"10.66.0.0/24","server_address":"10.66.0.1/24","server_private_key":"` + base64.StdEncoding.EncodeToString(private.Bytes()) + `"}`)}
	require.NoError(t, nodes.CreateProtocol(&f.wireGuard))
	f.user = model.User{Email: "fleet@example.test", Token: "fake-fleet-user-token", UUID: "00000000-0000-4000-8000-000000000050"}
	require.NoError(t, db.Create(&f.user).Error)
	peer, err := service.NewSubscriptionService().GetOrCreateWireGuardPeer(f.wireGuard.ID, f.user.ID, "10.66.0.0/24")
	require.NoError(t, err)
	f.peer = *peer
	return f
}

// readerResults runs every kernel reader of a moved column once and
// reports, per reader, whether it authenticated the subject or answered
// its real secret.
func (f *fleet) readerResults(t *testing.T, db *gorm.DB, round int) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ok := func(c *gin.Context) { c.String(http.StatusOK, "%d", c.GetUint("node_id")) }
	router.GET("/uniproxy", middleware.NodeAuth(), ok)
	router.GET("/node", middleware.NodeAPIKeyAuth(), ok)
	router.GET("/package", middleware.NodeAPIKeyHeaderAuth(), ok)
	router.POST("/signed", middleware.NodeAPIKeyAuth(), middleware.SignatureAuth(), ok)
	agents := handler.NewAgentHandler()
	router.GET("/agent", agents.RequireAgentNode, ok)
	router.GET("/forward-rules", agents.AgentGetForwardRules)
	router.GET("/credentials/:id", handler.NewNodeHandler().GetNodeCredentials)
	router.GET("/config", middleware.NodeAuth(), handler.NewUniProxyHandler().GetConfig)
	serve := func(method, path string, nodeID uint, key string, body string, header map[string]string) *httptest.ResponseRecorder {
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}
		req := httptest.NewRequest(method, path+separator+"node_id="+strconv.FormatUint(uint64(nodeID), 10), strings.NewReader(body))
		req.Header.Set("X-API-Key", key)
		for name, value := range header {
			req.Header.Set(name, value)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	authenticated := func(rec *httptest.ResponseRecorder, nodeID uint) bool {
		return rec.Code == http.StatusOK && rec.Body.String() == strconv.FormatUint(uint64(nodeID), 10)
	}

	results := map[string]bool{}
	proxies := []struct {
		name string
		id   uint
		key  string
	}{
		{"plain", f.plain.ID, "fake-plain-key"},
		{"created", f.created.ID, f.created.APIKey},
		{"registered", f.registered.NodeID, f.registered.APIKey},
	}
	for _, proxy := range proxies {
		for _, path := range []string{"/uniproxy", "/node", "/package", "/agent"} {
			results[proxy.name+" "+path] = authenticated(serve(http.MethodGet, path, proxy.id, proxy.key, "", nil), proxy.id)
		}
	}
	// The gRPC listener and the Agent Control stream look nodes up by their
	// key's hash.
	for _, proxy := range proxies[1:] {
		found, err := service.NewNodeService().GetNodeByAPIKey(proxy.key)
		results[proxy.name+" grpc"] = err == nil && found.ID == proxy.id
	}

	// The request signature with the node's shared secret.
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	body := `{"round":` + strconv.Itoa(round) + `}`
	mac := hmac.New(sha256.New, []byte(f.created.Secret))
	mac.Write([]byte(timestamp + http.MethodPost + "/signed" + body))
	signature := hex.EncodeToString(mac.Sum(nil))
	results["created signature"] = authenticated(serve(http.MethodPost, "/signed", f.created.ID, f.created.APIKey, body,
		map[string]string{middleware.SignatureHeader: signature, middleware.TimestampHeader: timestamp}), f.created.ID)

	// The administrators' credentials route shows the stored key and secret.
	rec := serve(http.MethodGet, "/credentials/"+strconv.FormatUint(uint64(f.created.ID), 10), 0, "", "", nil)
	var credentials struct {
		Data struct {
			APIKey string `json:"api_key"`
			Secret string `json:"secret"`
		} `json:"data"`
	}
	results["created credentials"] = rec.Code == http.StatusOK && json.Unmarshal(rec.Body.Bytes(), &credentials) == nil &&
		credentials.Data.APIKey == f.created.APIKey && credentials.Data.Secret == f.created.Secret

	// The node configuration: the Reality private key of a protocol, and
	// the private key of a raw configuration.
	configKey := func(nodeID uint, key string) string {
		rec := serve(http.MethodGet, "/config?node_type=vless", nodeID, key, "", nil)
		var config struct {
			TLSSettings map[string]any `json:"tls_settings"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &config) != nil {
			return ""
		}
		value, _ := config.TLSSettings["private_key"].(string)
		return value
	}
	results["created config"] = configKey(f.created.ID, f.created.APIKey) == f.realityPrivate
	results["raw config"] = configKey(f.rawNode.ID, f.rawNode.APIKey) == f.rawPrivate

	// Registration with the registration key.
	_, err := service.NewNodeService().RegisterNode(&model.NodeRegisterRequest{AuthKey: f.registrationKey, Name: fmt.Sprintf("again-%d", round), Port: 443}, "192.0.2.51")
	results["registration"] = err == nil

	// The forward node's agent, and its clean agent.
	results["forward agent"] = serve(http.MethodGet, "/agent", f.forward.ID, "fake-fleet-forward-token", "", nil).Code == http.StatusOK
	results["forward rules"] = serve(http.MethodGet, "/forward-rules", f.forward.ID, "fake-fleet-forward-token", "", nil).Code == http.StatusOK
	cleanAgents := service.NewForwardCleanAgentService(db)
	_, err = cleanAgents.Heartbeat(service.ForwardCleanAgentHeartbeatInput{AgentID: f.agent.ID, Token: f.agentToken})
	results["clean agent heartbeat"] = err == nil
	_, err = cleanAgents.Register(service.ForwardCleanAgentRegisterInput{Token: f.agentToken, NodeID: &f.forward.ID})
	results["clean agent register"] = err == nil

	// The WireGuard peer in a subscription and in the node's user list.
	peer, err := service.NewSubscriptionService().GetOrCreateWireGuardPeer(f.wireGuard.ID, f.user.ID, "10.66.0.0/24")
	results["wireguard subscription"] = err == nil && peer.PrivateKey == f.peer.PrivateKey && peer.PresharedKey == f.peer.PresharedKey
	var protocol model.NodeProtocol
	require.NoError(t, db.First(&protocol, f.wireGuard.ID).Error)
	extras, err := service.NewSubscriptionService().BuildWireGuardRuntimeUserExtras(&protocol, []*model.User{&f.user})
	results["wireguard users"] = err == nil && extras[f.user.ID]["wireguard_preshared_key"] == f.peer.PresharedKey
	return results
}

func requireAll(t *testing.T, results map[string]bool, want bool) {
	t.Helper()
	for reader, got := range results {
		require.Equal(t, want, got, reader)
	}
}

// totalFallbacks sums every fallback counter of this process.
func totalFallbacks() uint64 {
	var total uint64
	for _, table := range nodesecrets.Tables() {
		for _, kind := range []string{nodesecrets.KindNodeAPIKey, nodesecrets.KindNodeSharedSecret, nodesecrets.KindRegistrationKey,
			nodesecrets.KindForwardNodeToken, nodesecrets.KindCleanAgentToken, "raw_config", "settings", "tls_settings",
			"transport_settings", "reality_settings", "custom_config", "private_key", "preshared_key"} {
			total += nodesecrets.FallbackCount(table, kind, "")
		}
	}
	return total
}

func setPhase(t *testing.T, db *gorm.DB, phase string) {
	t.Helper()
	changes, err := nodesecrets.SetPhase(context.Background(), db, nodesecrets.PhaseOptions{Phase: phase, Actor: "nodesecretsplit"})
	require.NoError(t, err, "%+v", changes)
}

// olderBinaryAuthenticates is the release before NO-3 authenticating a
// node: it reads only the legacy columns.
func olderBinaryAuthenticates(t *testing.T, db *gorm.DB, f *fleet) {
	t.Helper()
	for _, proxy := range []struct {
		id  uint
		key string
	}{{f.plain.ID, "fake-plain-key"}, {f.created.ID, f.created.APIKey}, {f.registered.NodeID, f.registered.APIKey}} {
		var node model.Node
		require.NoError(t, db.First(&node, proxy.id).Error)
		require.True(t, node.APIKeyHash == sha256Hex(proxy.key) || (node.APIKeyHash == "" && node.APIKey == proxy.key), "node %d", proxy.id)
		require.Equal(t, proxy.key, node.APIKey)
	}
	var byHash model.Node
	require.NoError(t, db.Where("api_key_hash = ?", sha256Hex(f.created.APIKey)).First(&byHash).Error)
	require.Equal(t, f.created.ID, byHash.ID)
	var key model.AuthorizedKey
	require.NoError(t, db.Where("key_hash = ?", sha256Hex(f.registrationKey)).First(&key).Error)
	var forward model.ForwardNode
	require.NoError(t, db.First(&forward, f.forward.ID).Error)
	require.Equal(t, "fake-fleet-forward-token", forward.APIToken)
	var agent model.ForwardCleanAgent
	require.NoError(t, db.Where("id = ? AND token = ?", f.agent.ID, f.agentToken).First(&agent).Error)
	var protocol model.NodeProtocol
	require.NoError(t, db.First(&protocol, f.reality.ID).Error)
	require.Contains(t, *protocol.RealitySettings, f.realityPrivate)
	var peer model.WireGuardPeer
	require.NoError(t, db.First(&peer, f.peer.ID).Error)
	require.Equal(t, f.peer.PrivateKey, peer.PrivateKey)
}

// TestEveryReaderInEachPhase: every reader authenticates every subject and
// answers every secret in dual_write and in dual_read; in dual_read a
// missing new row falls back with the metric; the way back is a phase
// change; and the phase change never touches a legacy column, so an older
// binary still authenticates every node.
func TestEveryReaderInEachPhase(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		ctx := context.Background()
		f := newFleet(t, db)
		_, err := nodesecrets.Backfill(ctx, db, nodesecrets.BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		before := totalFallbacks()

		requireAll(t, f.readerResults(t, db, 1), true)
		require.Equal(t, before, totalFallbacks(), "dual_write reads only the legacy columns")

		columns := legacyColumns(t, db)
		setPhase(t, db, nodesecrets.PhaseDualRead)
		require.Equal(t, columns, legacyColumns(t, db), "the phase change never touches a legacy column")
		olderBinaryAuthenticates(t, db, f)

		requireAll(t, f.readerResults(t, db, 2), true)
		require.Equal(t, before, totalFallbacks(), "every secret was read from the new tables")
		olderBinaryAuthenticates(t, db, f)
		requireVerified(t, db)

		// Rows missing from the new tables: the readers fall back to the
		// legacy columns, and count it.
		require.NoError(t, db.Where("subject_kind = ? AND subject_id = ?", nodesecrets.SubjectProxy, f.created.ID).Delete(&model.NodeCredential{}).Error)
		require.NoError(t, db.Where("subject_kind IN ?", []string{nodesecrets.SubjectForward, nodesecrets.SubjectCleanAgent, nodesecrets.SubjectRegistrationKey}).
			Delete(&model.NodeCredential{}).Error)
		require.NoError(t, db.Where("owner_id IN ? AND scope = ?", []uint{f.reality.ID, f.wireGuard.ID}, nodesecrets.ScopeNodeProtocol).Delete(&model.ProtocolSecret{}).Error)
		require.NoError(t, db.Where("scope IN ?", []string{nodesecrets.ScopeWireGuardPeer, nodesecrets.ScopeNodeRawConfig}).Delete(&model.ProtocolSecret{}).Error)
		apiKeys := nodesecrets.FallbackCount(nodesecrets.TableNode, nodesecrets.KindNodeAPIKey, nodesecrets.FallbackMissing)
		requireAll(t, f.readerResults(t, db, 3), true)
		require.Greater(t, nodesecrets.FallbackCount(nodesecrets.TableNode, nodesecrets.KindNodeAPIKey, nodesecrets.FallbackMissing), apiKeys)
		for _, counted := range [][2]string{
			{nodesecrets.TableNode, nodesecrets.KindNodeSharedSecret}, {nodesecrets.TableNode, "raw_config"},
			{nodesecrets.TableAuthorizedKey, nodesecrets.KindRegistrationKey},
			{nodesecrets.TableForwardNode, nodesecrets.KindForwardNodeToken}, {nodesecrets.TableCleanAgent, nodesecrets.KindCleanAgentToken},
			{nodesecrets.TableNodeProtocol, "reality_settings"}, {nodesecrets.TableWireGuardPeer, "private_key"},
		} {
			require.NotZero(t, nodesecrets.FallbackCount(counted[0], counted[1], nodesecrets.FallbackMissing), "%v", counted)
		}

		// The way back: dual_write reads the legacy columns again and
		// counts nothing more.
		setPhase(t, db, nodesecrets.PhaseDualWrite)
		after := totalFallbacks()
		requireAll(t, f.readerResults(t, db, 4), true)
		require.Equal(t, after, totalFallbacks())
		olderBinaryAuthenticates(t, db, f)
	})
}

// moveLegacySecrets writes into the legacy columns what P3 leaves there
// (section 4.4), bypassing the writers.
func (f *fleet) moveLegacySecrets(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, id := range []uint{f.plain.ID, f.created.ID, f.registered.NodeID} {
		require.NoError(t, db.Model(&model.Node{}).Where("id = ?", id).
			UpdateColumns(map[string]any{"api_key": nodesecrets.Tombstone(uint64(id)), "api_key_hash": "", "secret": ""}).Error)
	}
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", f.rawNode.ID).
		UpdateColumn("raw_config", service.RedactNodeSecretsJSON(`{"node_type":"vless","type":"vless","server_port":443,"tls":2,"tls_settings":{"private_key":"`+f.rawPrivate+`","short_id":"01"}}`)).Error)
	var keys []model.AuthorizedKey
	require.NoError(t, db.Find(&keys).Error)
	for _, key := range keys {
		require.NoError(t, db.Model(&model.AuthorizedKey{}).Where("id = ?", key.ID).
			UpdateColumns(map[string]any{"key": nodesecrets.Tombstone(uint64(key.ID)), "key_hash": ""}).Error)
	}
	require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", f.forward.ID).UpdateColumn("api_token", "").Error)
	require.NoError(t, db.Model(&model.ForwardCleanAgent{}).Where("id = ?", f.agent.ID).UpdateColumn("token", nodesecrets.Tombstone(uint64(f.agent.ID))).Error)
	var protocols []model.NodeProtocol
	require.NoError(t, db.Where("id IN ?", []uint{f.reality.ID, f.wireGuard.ID}).Find(&protocols).Error)
	for _, protocol := range protocols {
		redacted := protocol
		service.RedactNodeProtocol(&redacted)
		require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", protocol.ID).
			UpdateColumns(map[string]any{"settings": redacted.Settings, "reality_settings": redacted.RealitySettings}).Error)
	}
	require.NoError(t, db.Model(&model.WireGuardPeer{}).Where("id = ?", f.peer.ID).UpdateColumns(map[string]any{"private_key": "", "preshared_key": ""}).Error)
}

// TestDualReadReadsTheNewTables: with only the new tables holding the
// secrets, as after P3, every reader in dual_read still authenticates and
// answers every secret, without one fallback. In dual_write the same
// readers find nothing: the tombstones and the placeholder authenticate no
// one, which also proves the phase switches every reader.
func TestDualReadReadsTheNewTables(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		f := newFleet(t, db)
		_, err := nodesecrets.Backfill(context.Background(), db, nodesecrets.BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		setPhase(t, db, nodesecrets.PhaseDualRead)
		f.moveLegacySecrets(t, db)

		before := totalFallbacks()
		requireAll(t, f.readerResults(t, db, 1), true)
		require.Equal(t, before, totalFallbacks())

		setPhase(t, db, nodesecrets.PhaseDualWrite)
		results := f.readerResults(t, db, 2)
		requireAll(t, results, false)
		require.Equal(t, before, totalFallbacks())
	})
}

// TestValidateOnBuildReportsWithoutExcluding: a protocol whose secret fails
// validation is reported when a node's configuration is built from it,
// with the metric and a log line naming the node, protocol and field, and
// is still in the configuration (D7: report-only). The scan reports the
// same rows and excludes nothing.
func TestValidateOnBuildReportsWithoutExcluding(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		nodes := service.NewNodeService()
		node := &model.Node{Name: "invalid", Host: "203.0.113.60", Port: 443}
		require.NoError(t, nodes.CreateNode(node))
		// Written around the kernel's write checks, as an import or a
		// package could: a masked Reality key and a malformed WireGuard key.
		masked := model.NodeProtocol{NodeID: node.ID, Name: "masked", Type: model.ProtocolVLESS, Port: 8443, TLS: 2, Enable: 1,
			RealitySettings: ptr(`{"private_key":"********","short_id":"01"}`)}
		malformed := model.NodeProtocol{NodeID: node.ID, Name: "malformed", Type: model.ProtocolWireGuard, Port: 51820, Enable: 1,
			Settings: ptr(`{"cidr":"10.66.0.0/24","server_private_key":"fake-not-a-key"}`)}
		require.NoError(t, db.Create(&masked).Error)
		require.NoError(t, db.Create(&malformed).Error)

		nodesecrets.ForgetLogged()
		var logs bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
		t.Cleanup(func() { slog.SetDefault(previous) })
		placeholders := nodesecrets.InvalidCount(nodesecrets.TableNodeProtocol, service.NodeSecretReasonPlaceholder)
		wireGuardKeys := nodesecrets.InvalidCount(nodesecrets.TableNodeProtocol, service.NodeSecretReasonWireGuardKey)

		var stored model.Node
		require.NoError(t, db.First(&stored, node.ID).Error)
		config := service.BuildNodeProtocolConfig(&stored, &masked)
		require.Equal(t, "********", config["tls_settings"].(map[string]any)["private_key"], "reported, not excluded")
		config = service.BuildNodeProtocolConfig(&stored, &malformed)
		require.Equal(t, "fake-not-a-key", config["server_private_key"], "reported, not excluded")
		require.EqualValues(t, 1, nodesecrets.InvalidCount(nodesecrets.TableNodeProtocol, service.NodeSecretReasonPlaceholder)-placeholders)
		require.EqualValues(t, 1, nodesecrets.InvalidCount(nodesecrets.TableNodeProtocol, service.NodeSecretReasonWireGuardKey)-wireGuardKeys)
		output := logs.String()
		require.Contains(t, output, fmt.Sprintf(`subject="node %d protocol %d reality_settings /private_key" reason=placeholder`, node.ID, masked.ID))
		require.Contains(t, output, fmt.Sprintf(`subject="node %d protocol %d settings /server_private_key" reason=wireguard_key`, node.ID, malformed.ID))
		require.NotContains(t, output, "fake-not-a-key")

		report, err := service.ScanNodeSecrets(context.Background(), db)
		require.NoError(t, err)
		require.Zero(t, report.Excluded)
		require.EqualValues(t, 3, report.Protocols, "the default protocol and the two written above")
		require.Equal(t, []service.NodeSecretFinding{
			{NodeID: node.ID, ProtocolID: masked.ID, Type: "vless", Enabled: true, Column: "reality_settings", Field: "/private_key", Reason: service.NodeSecretReasonPlaceholder},
			{NodeID: node.ID, ProtocolID: malformed.ID, Type: "wireguard", Enabled: true, Column: "settings", Field: "/server_private_key", Reason: service.NodeSecretReasonWireGuardKey},
		}, report.Findings)
		var count int64
		require.NoError(t, db.Model(&model.NodeProtocol{}).Where("node_id = ?", node.ID).Count(&count).Error)
		require.EqualValues(t, 3, count, "validation changes nothing")
	})
}
