package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
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
	require.Equal(t, config.AgentMTLSPreferred, body.Data.Mode)
	require.Equal(t, agenttransport.Summary{Total: 2, MTLS: 1, Legacy: 1}, body.Data.Summary)

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
	initTestDB()
	db := database.Get()
	require.NoError(t, db.AutoMigrate(&model.AgentTransport{}))
	t.Cleanup(agenttransport.SetDefault(agenttransport.NewRecorder(database.Get)))
	forward := model.ForwardNode{ID: 7701, Name: "ws-relay", Host: "198.51.100.77", Port: 22, APIToken: "ws-relay-token", Enabled: true}
	require.NoError(t, db.Create(&forward).Error)
	t.Cleanup(func() { db.Delete(&model.ForwardNode{}, forward.ID) })

	h := NewAgentHandler()
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
