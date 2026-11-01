package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/agenttransport"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

// TestLegacyAgentPathsAreGone: the retired AnixOps-agent REST and WebSocket
// paths, UniProxy and the old subscription path no longer exist, whatever
// credentials they carry.
func TestLegacyAgentPathsAreGone(t *testing.T) {
	router, _, host := setupV2PackageRouter(t)
	require.NoError(t, database.GetDB().AutoMigrate(&model.Node{}, &model.ForwardNode{}, &model.AgentTransport{}))
	t.Cleanup(agenttransport.SetDefault(agenttransport.NewRecorder(database.Get)))
	node := model.Node{Name: "edge", Host: "edge.example.test"}
	require.NoError(t, database.GetDB().Create(&node).Error)
	nodeID := strconv.FormatUint(uint64(node.ID), 10)

	send := func(method, path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		request.Header.Set("X-Node-ID", nodeID)
		request.Header.Set("X-API-Key", "edge-api-key")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	for _, test := range []struct{ method, path string }{
		{http.MethodGet, "/api/v2/agent/tasks"},
		{http.MethodPost, "/api/v2/agent/heartbeat"},
		{http.MethodPost, "/api/v2/agent/register"},
		{http.MethodPost, "/api/v2/agent/result"},
		{http.MethodPost, "/api/v2/agent/monitor"},
		{http.MethodGet, "/api/v2/agent/ws"},
		{http.MethodPost, "/api/v2/node/heartbeat"},
		{http.MethodPost, "/api/v2/node/runtime-health"},
		{http.MethodPost, "/api/v2/node/register"},
		{http.MethodGet, "/api/v2/node/ws"},
		{http.MethodGet, "/api/v2/server/UniProxy/config"},
		{http.MethodGet, "/api/v2/server/UniProxy/user"},
		{http.MethodPost, "/api/v2/server/UniProxy/push"},
		{http.MethodGet, "/api/v1/server/UniProxy/config"},
		{http.MethodGet, "/api/v1/client/subscribe"},
	} {
		host.lastRouteID = ""
		answer := send(test.method, test.path)
		require.Equal(t, http.StatusNotFound, answer.Code, "%s %s: %s", test.method, test.path, answer.Body.String())
		require.Empty(t, answer.Header().Get("Deprecation"), test.path)
		require.Empty(t, host.lastRouteID, "%s reached the package host", test.path)
	}
	answer := send(http.MethodGet, "/api/v2/forward-agent/install.sh?node_id="+nodeID+"&token=edge-api-key")
	require.NotEqual(t, http.StatusForbidden, answer.Code)
	require.Empty(t, answer.Header().Get("Deprecation"))
}
