package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAgentTransportsHandlerListsTheInventory(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{}, &model.ForwardNode{}, &model.ForwardCleanAgent{}, &model.AgentTransport{}, &model.AgentCertificate{})
	now := time.Now().UTC()
	require.NoError(t, db.Create(&[]model.Node{{ID: 1, Name: "enrolled", APIKey: "a"}, {ID: 2, Name: "legacy", APIKey: "b"}}).Error)
	require.NoError(t, db.Create(&[]model.AgentTransport{
		{NodeKind: "proxy", NodeID: 1, Transport: model.AgentTransportMTLSStream, AgentVersion: "2.0.0", FirstSeenAt: now, LastSeenAt: now},
		{NodeKind: "proxy", NodeID: 2, Transport: model.AgentTransportWebSocket, FirstSeenAt: now, LastSeenAt: now},
	}).Error)
	live := agenttransport.NewRecorder(nil)
	handler := &AgentTransportsHandler{
		policy: agenttransport.PolicyFrom(config.AgentControlConfig{}),
		db:     func() *gorm.DB { return db },
		live:   func() *agenttransport.Recorder { return live },
	}
	route := "/api/v4/kernel/agents/transports"

	recorder := performKernelHandlerRequest(t, http.MethodGet, route, "", route, handler.List)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var body struct {
		Data agenttransport.Inventory `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, config.AgentMTLSRequired, body.Data.Mode, "the 4.2 default")
	summary := body.Data.Summary
	require.Equal(t, [3]int{2, 1, 1}, [3]int{summary.Total, summary.MTLS, summary.Legacy})
	require.False(t, summary.ReadyForRequired)
	require.Len(t, summary.RequiredReasons, 1)
	require.Contains(t, summary.RequiredReasons[0], "1 enabled node(s) still on a legacy AnixOps Agent channel (1 seen within the last 7 days)")
	require.Len(t, summary.RequiredBlockers, 1)
	require.Equal(t, "proxy-2", summary.RequiredBlockers[0].Node)
	require.Contains(t, recorder.Body.String(), `"ready_for_required":false`)

	recorder = performKernelHandlerRequest(t, http.MethodGet, route+"?legacy_only=true", "", route, handler.List)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Len(t, body.Data.Nodes, 1)
	require.Equal(t, "proxy-2", body.Data.Nodes[0].Node)
	require.Equal(t, model.AgentTransportWebSocket, body.Data.Nodes[0].Transport)

	recorder = performKernelHandlerRequest(t, http.MethodGet, route+"?legacy_only=maybe", "", route, handler.List)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid_request")

	handler.db = func() *gorm.DB { return nil }
	recorder = performKernelHandlerRequest(t, http.MethodGet, route, "", route, handler.List)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)

	broken := newKernelHandlerTestDB(t)
	handler.db = func() *gorm.DB { return broken }
	recorder = performKernelHandlerRequest(t, http.MethodGet, route, "", route, handler.List)
	require.Equal(t, http.StatusInternalServerError, recorder.Code, "the tables are missing")

	require.NotNil(t, NewAgentTransportsHandler(agenttransport.Policy{Mode: config.AgentMTLSOptional}))
}

// TestAgentWebSocketCarriesDeprecationAndIsRecorded: the legacy path
// middleware's signals reach the WebSocket handshake answer, and the
// session is recorded in the transport inventory.
func TestAgentWebSocketCarriesDeprecationAndIsRecorded(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{}, &model.ForwardNode{}, &model.AgentTransport{})
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(agenttransport.SetDefault(agenttransport.NewRecorder(func() *gorm.DB { return db })))
	forward := model.ForwardNode{ID: 7701, Name: "ws-relay", Host: "198.51.100.77", Port: 22, APIToken: "ws-relay-token", Enabled: true}
	require.NoError(t, db.Create(&forward).Error)

	h := NewAgentHandler()
	h.db = db
	h.wsUpgrader.CheckOrigin = func(*http.Request) bool { return true }
	router := gin.New()
	router.GET("/api/v2/agent/ws",
		agenttransport.LegacyHTTP(agenttransport.Policy{Mode: config.AgentMTLSPreferred}, model.AgentTransportWebSocket),
		h.AgentWebSocketUnified)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	header := http.Header{}
	header.Set("X-Node-ID", "7701")
	header.Set("X-API-Key", "ws-relay-token")
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/v2/agent/ws", header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.Equal(t, "true", response.Header.Get("Deprecation"))
	require.Contains(t, response.Header.Get("Link"), agenttransport.UpgradeGuideURL)
	require.NoError(t, conn.WriteJSON(map[string]any{"type": "heartbeat", "node_id": 7701}))

	require.Eventually(t, func() bool {
		var rows []model.AgentTransport
		db.Where("node_kind = ? AND node_id = ? AND transport = ?", "forward", 7701, model.AgentTransportWebSocket).Find(&rows)
		return len(rows) == 1
	}, 2*time.Second, 20*time.Millisecond)

	require.Nil(t, legacyAgentUpgradeHeader(http.Header{}), "nothing to carry without signals")
	recordWebSocketSighting(t.Context(), nil)
}

// TestAgentTransportsHandlerShowsCertificatesAndConnections: the inventory
// answer carries each node's certificate state, certificate times and
// connection type, and node=... narrows it to the named nodes.
func TestAgentTransportsHandlerShowsCertificatesAndConnections(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{}, &model.ForwardNode{}, &model.ForwardCleanAgent{}, &model.AgentTransport{}, &model.AgentCertificate{})
	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, db.Create(&[]model.Node{{ID: 1, Name: "enrolled", APIKey: "a"}, {ID: 2, Name: "revoked", APIKey: "b"}, {ID: 3, Name: "silent", APIKey: "c"}}).Error)
	revokedAt := now.Add(-time.Hour)
	require.NoError(t, db.Create(&[]model.AgentCertificate{
		{Serial: "live", NodeKind: "proxy", NodeID: 1, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", CreatedAt: now.Add(-24 * time.Hour), NotAfter: now.Add(6 * 24 * time.Hour)},
		{Serial: "ended", NodeKind: "proxy", NodeID: 2, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", CreatedAt: now.Add(-24 * time.Hour), NotAfter: now.Add(6 * 24 * time.Hour), RevokedAt: &revokedAt, RevokeReason: "node_disabled"},
	}).Error)
	require.NoError(t, db.Create(&[]model.AgentTransport{
		{NodeKind: "proxy", NodeID: 1, Transport: model.AgentTransportMTLSStream, AgentVersion: "2.0.0", FirstSeenAt: now, LastSeenAt: now},
		{NodeKind: "proxy", NodeID: 2, Transport: model.AgentTransportWebSocket, FirstSeenAt: now, LastSeenAt: now.Add(-time.Minute)},
	}).Error)
	handler := &AgentTransportsHandler{
		policy: agenttransport.PolicyFrom(config.AgentControlConfig{}),
		db:     func() *gorm.DB { return db },
		live:   func() *agenttransport.Recorder { return agenttransport.NewRecorder(nil) },
	}
	route := "/api/v4/kernel/agents/transports"
	type answer struct {
		Data struct {
			Nodes []map[string]any `json:"nodes"`
		} `json:"data"`
	}
	nodes := func(query string) map[string]map[string]any {
		t.Helper()
		recorder := performKernelHandlerRequest(t, http.MethodGet, route+query, "", route, handler.List)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var body answer
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		byNode := map[string]map[string]any{}
		for _, node := range body.Data.Nodes {
			byNode[node["node"].(string)] = node
		}
		return byNode
	}

	all := nodes("")
	require.Len(t, all, 3)
	live := all["proxy-1"]
	require.Equal(t, "valid", live["certificate_state"])
	certificate := live["certificate"].(map[string]any)
	require.Equal(t, "live", certificate["serial"])
	require.Equal(t, now.Add(-24*time.Hour).Format(time.RFC3339), certificate["issued_at"])
	require.Equal(t, now.Add(6*24*time.Hour).Format(time.RFC3339), certificate["not_after"])
	// Seven days of lifetime, issued a day ago: renewed after 4 days 16 hours.
	require.Equal(t, now.Add(-24*time.Hour+7*24*time.Hour*2/3).Format(time.RFC3339), certificate["renew_after"])
	require.Nil(t, certificate["revoked_at"])
	require.Equal(t, "mtls_stream", live["connection"].(map[string]any)["type"], "no session provider: the stream sighting decides")

	revoked := all["proxy-2"]
	require.Equal(t, "revoked", revoked["certificate_state"])
	require.Nil(t, revoked["certificate"])
	last := revoked["last_certificate"].(map[string]any)
	require.Equal(t, "ended", last["serial"])
	require.Equal(t, "node_disabled", last["revoke_reason"])
	require.NotNil(t, last["revoked_at"])
	require.Equal(t, "legacy", revoked["connection"].(map[string]any)["type"])
	require.Equal(t, "websocket", revoked["connection"].(map[string]any)["transport"])

	silent := all["proxy-3"]
	require.Equal(t, "none", silent["certificate_state"])
	require.Nil(t, silent["last_certificate"])
	require.Equal(t, "offline", silent["connection"].(map[string]any)["type"])
	require.Nil(t, silent["connection"].(map[string]any)["last_seen_at"])

	one := nodes("?node=proxy-2")
	require.Len(t, one, 1)
	require.Contains(t, one, "proxy-2")
	two := nodes("?node=proxy-1,proxy-3&node=proxy-99")
	require.Len(t, two, 2, "a repeated and a comma separated value; an unknown node is not listed")
	require.Contains(t, two, "proxy-1")
	require.Contains(t, two, "proxy-3")
	require.Len(t, nodes("?node="), 3, "an empty value names nobody")

	for _, query := range []string{"?node=node-1", "?node=proxy-0", "?node=proxy-x", "?node=proxy-1,,,bad"} {
		recorder := performKernelHandlerRequest(t, http.MethodGet, route+query, "", route, handler.List)
		require.Equal(t, http.StatusBadRequest, recorder.Code, query)
		require.Contains(t, recorder.Body.String(), "invalid_request")
	}
	many := make([]string, 0, maxNodeFilter+1)
	for i := 1; i <= maxNodeFilter+1; i++ {
		many = append(many, "proxy-"+strconv.Itoa(i))
	}
	recorder := performKernelHandlerRequest(t, http.MethodGet, route+"?node="+strings.Join(many, ","), "", route, handler.List)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "at most 200")
}
