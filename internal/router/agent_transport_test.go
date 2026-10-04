package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestLegacyAgentPathsFollowAgentControlMTLS: the legacy AnixOps-agent
// paths answer deprecation signals by default (preferred) and are refused
// in required, while UniProxy, which third-party node software shares,
// stays open and is never signalled.
func TestLegacyAgentPathsFollowAgentControlMTLS(t *testing.T) {
	router, cfg, host := setupV2PackageRouter(t)
	require.NoError(t, database.GetDB().AutoMigrate(&model.Node{}, &model.ForwardNode{}, &model.AgentTransport{}))
	t.Cleanup(agenttransport.SetDefault(agenttransport.NewRecorder(database.Get)))
	node := model.Node{Name: "edge", Host: "edge.example.test", APIKey: "edge-api-key", APIKeyHash: sha256Hex("edge-api-key")}
	require.NoError(t, database.GetDB().Create(&node).Error)
	nodeID := strconv.FormatUint(uint64(node.ID), 10)

	send := func(router *gin.Engine, method, path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		request.Header.Set("X-Node-ID", nodeID)
		request.Header.Set("X-API-Key", "edge-api-key")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	// preferred, which the fixture sets explicitly (the 4.1 default).
	served := agenttransport.LegacyRequests("/api/v2/agent/tasks")
	answer := send(router, http.MethodGet, "/api/v2/agent/tasks")
	require.Equal(t, http.StatusOK, answer.Code, answer.Body.String())
	require.Equal(t, "true", answer.Header().Get("Deprecation"))
	require.Contains(t, answer.Header().Get("Link"), agenttransport.UpgradeGuideURL)
	require.Equal(t, served+1, agenttransport.LegacyRequests("/api/v2/agent/tasks"))
	var row model.AgentTransport
	require.NoError(t, database.GetDB().Where("node_kind = ? AND node_id = ? AND transport = ?", "proxy", node.ID, model.AgentTransportHTTPLegacy).First(&row).Error)
	answer = send(router, http.MethodGet, "/api/v2/server/UniProxy/config?node_id="+nodeID+"&token=edge-api-key")
	require.Empty(t, answer.Header().Get("Deprecation"), "UniProxy is not an AnixOps-only channel")

	// The 4.2 default (agent_control.mtls empty) is required.
	defaulted := *cfg
	defaulted.AgentControl = config.AgentControlConfig{}
	byDefault := gin.New()
	Setup(byDefault, &defaulted)
	answer = send(byDefault, http.MethodGet, "/api/v2/agent/tasks")
	require.Equal(t, http.StatusForbidden, answer.Code, answer.Body.String())
	require.Contains(t, answer.Body.String(), `"code":"agent_mtls_required"`)

	// required, with a sunset date.
	required := *cfg
	required.AgentControl = config.AgentControlConfig{MTLS: config.AgentMTLSRequired, LegacySunset: "2027-03-31"}
	strict := gin.New()
	Setup(strict, &required)
	for _, test := range []struct{ method, path string }{
		{http.MethodGet, "/api/v2/agent/tasks"},
		{http.MethodPost, "/api/v2/agent/heartbeat"},
		{http.MethodPost, "/api/v2/agent/register"},
		{http.MethodGet, "/api/v2/agent/ws"},
		{http.MethodPost, "/api/v2/node/heartbeat"},
		{http.MethodPost, "/api/v2/node/runtime-health"},
		{http.MethodPost, "/api/v2/node/register"},
		{http.MethodGet, "/api/v2/node/ws"},
		{http.MethodGet, "/api/v2/forward/agent/rules?node_id=" + nodeID},
		{http.MethodPost, "/api/v2/forward-agent/heartbeat"},
		{http.MethodPost, "/api/v2/forward-agent/register"},
		{http.MethodPost, "/api/v2/forward-agent/report"},
	} {
		host.lastRouteID = ""
		answer := send(strict, test.method, test.path)
		require.Equal(t, http.StatusForbidden, answer.Code, test.path)
		require.Contains(t, answer.Body.String(), `"code":"agent_mtls_required"`, test.path)
		require.Equal(t, "Wed, 31 Mar 2027 00:00:00 GMT", answer.Header().Get("Sunset"), test.path)
		require.Empty(t, host.lastRouteID, "%s reached the package host", test.path)
	}
	for _, path := range []string{"/api/v2/server/UniProxy/config", "/api/v1/server/UniProxy/config", "/api/v2/forward-agent/install.sh"} {
		answer := send(strict, http.MethodGet, path+"?node_id="+nodeID+"&token=edge-api-key")
		require.NotEqual(t, http.StatusForbidden, answer.Code, path)
		require.Empty(t, answer.Header().Get("Deprecation"), path)
	}
}
