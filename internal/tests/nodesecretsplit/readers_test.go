package nodesecretsplit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/middleware"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// legacyColumns reads every moved column, the state an older binary
// authenticates from.
func legacyColumns(t *testing.T, db *gorm.DB) map[string][]map[string]any {
	t.Helper()
	columns := map[string][]string{
		"v2_node":                {"id", "api_key", "api_key_hash", "secret", "raw_config"},
		"v2_authorized_key":      {"id", "key", "key_hash", "expire_at", "used"},
		"v2_forward_node":        {"id", "host", "api_port", "api_token"},
		"v2_forward_clean_agent": {"id", "token", "status"},
		"v2_node_protocol":       {"id", "settings", "tls_settings", "transport_settings", "reality_settings", "custom_config"},
		"v2_wireguard_peer":      {"id", "private_key", "public_key", "preshared_key"},
	}
	out := map[string][]map[string]any{}
	for table, selected := range columns {
		var rows []map[string]any
		require.NoError(t, db.Table(table).Select(selected).Order("id").Find(&rows).Error)
		out[table] = rows
	}
	return out
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// TestOlderReadersAuthenticateEveryNode: P1 never changes a legacy column,
// so the readers an older binary runs (UniProxy, the node API, package
// downloads, the gRPC node service, registration, clean agents) still
// authenticate every node after the backfill and the verification, whether
// the node was written before P1 or by a P1 writer.
func TestOlderReadersAuthenticateEveryNode(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		ctx := context.Background()
		nodes := service.NewNodeService()

		// Written before P1: legacy rows only, one of them without a hash.
		before := []model.Node{
			{Name: "legacy", Host: "203.0.113.30", Port: 443, APIKey: "fake-legacy-key", APIKeyHash: sha256Hex("fake-legacy-key"), Secret: "fake-legacy-secret"},
			{Name: "unhashed", Host: "203.0.113.31", Port: 443, APIKey: "fake-unhashed-key"},
		}
		require.NoError(t, db.Create(&before).Error)
		// Written by the P1 writers.
		created := &model.Node{Name: "created", Host: "203.0.113.32", Port: 443}
		require.NoError(t, nodes.CreateNode(created))
		_, registrationKey, err := nodes.GenerateAuthKey("readers", 0)
		require.NoError(t, err)
		registered, err := nodes.RegisterNode(&model.NodeRegisterRequest{AuthKey: registrationKey, Name: "registered", Port: 443}, "192.0.2.30")
		require.NoError(t, err)
		forward := &model.ForwardNode{Name: "relay", Type: "relay", Host: "198.51.100.30", Port: 443, APIPort: 9000, APIToken: "fake-forward-token", Enabled: true}
		require.NoError(t, service.NewForwardNodeService(db).Create(forward))
		agents := service.NewForwardCleanAgentService(db)
		agent, err := service.CreateCleanAgentTokenTx(db, service.ForwardCleanAgentCreateInput{Name: "agent", NodeID: &forward.ID})
		require.NoError(t, err)

		columns := legacyColumns(t, db)
		_, err = nodesecrets.Backfill(ctx, db, nodesecrets.BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		require.Equal(t, columns, legacyColumns(t, db), "the split never changes a legacy column")

		gin.SetMode(gin.TestMode)
		router := gin.New()
		ok := func(c *gin.Context) { c.String(http.StatusOK, "%d", c.GetUint("node_id")) }
		router.GET("/uniproxy", middleware.NodeAuth(), ok)
		router.GET("/node", middleware.NodeAPIKeyAuth(), ok)
		router.GET("/package", middleware.NodeAPIKeyHeaderAuth(), ok)
		request := func(path string, nodeID uint, key string) *httptest.ResponseRecorder {
			t.Helper()
			req := httptest.NewRequest(http.MethodGet, path+"?node_id="+strconv.FormatUint(uint64(nodeID), 10), nil)
			req.Header.Set("X-API-Key", key)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			return rec
		}

		proxies := []struct {
			id     uint
			key    string
			hashed bool
		}{
			{before[0].ID, "fake-legacy-key", true},
			{before[1].ID, "fake-unhashed-key", false},
			{created.ID, created.APIKey, true},
			{registered.NodeID, registered.APIKey, true},
		}
		for _, proxy := range proxies {
			for _, path := range []string{"/uniproxy", "/node", "/package"} {
				rec := request(path, proxy.id, proxy.key)
				require.Equal(t, http.StatusOK, rec.Code, "%s node %d: %s", path, proxy.id, rec.Body.String())
				require.Equal(t, strconv.FormatUint(uint64(proxy.id), 10), rec.Body.String())
				require.Equal(t, http.StatusUnauthorized, request(path, proxy.id, "fake-wrong-key").Code)
			}
			// The gRPC node service looks keys up by their stored hash.
			if proxy.hashed {
				found, err := nodes.GetNodeByAPIKey(proxy.key)
				require.NoError(t, err)
				require.Equal(t, proxy.id, found.ID)
			}
		}

		// The registration key still registers, the clean agent still
		// authenticates, and the forward node's token is where its agent
		// and the gost manager read it.
		_, err = nodes.RegisterNode(&model.NodeRegisterRequest{AuthKey: registrationKey, Name: "again", Port: 443}, "192.0.2.31")
		require.NoError(t, err)
		_, err = agents.Heartbeat(service.ForwardCleanAgentHeartbeatInput{AgentID: agent.Agent.ID, Token: agent.Token})
		require.NoError(t, err)
		_, err = agents.Heartbeat(service.ForwardCleanAgentHeartbeatInput{AgentID: agent.Agent.ID, Token: "fake-wrong-token"})
		require.ErrorIs(t, err, service.ErrForwardCleanAgentUnauthorized)
		var stored model.ForwardNode
		require.NoError(t, db.First(&stored, forward.ID).Error)
		require.Equal(t, "fake-forward-token", stored.APIToken)
	})
}
