package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/maintenance"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestMaintenanceWebSocketAuthenticationDurableAckAndTicket(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{}, &model.ForwardNode{})
	// Overlapping resource IDs must authenticate against the matching secret.
	require.NoError(t, db.Create(&model.Node{ID: 1, Name: "monitored", APIKey: "test-only-proxy-secret", Status: model.NodeStatusOnline}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 1, Name: "other execution plane", APIToken: "test-only-forward-secret"}).Error)
	h := &AgentHandler{db: db, wsUpgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}}
	router := gin.New()
	router.GET("/ws", h.AgentWebSocketUnified)
	server := httptest.NewServer(router)
	defer server.Close()
	headers := http.Header{"X-Node-Id": []string{"1"}, "X-Api-Key": []string{"test-only-proxy-secret"}}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", headers)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	now := time.Now().UTC()
	first := now.Add(-2 * time.Minute)
	event := maintenance.Event{SchemaVersion: 1, EventID: "ws-event-1", OccurredAt: now, Environment: "development", Source: "agent", NodeID: "1", PluginID: "machine-telemetry", InstanceID: "machine-telemetry", PluginVersion: "1.1.0", ErrorCode: "PLUGIN_PROCESS_EXITED", Severity: "P1", Status: "open", FirstFailedAt: &first, ConsecutiveFailures: 3, RedactedSummary: "password=secret-canary"}
	send := func(e maintenance.Event) maintenance.Acknowledgement {
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
		require.NoError(t, conn.WriteJSON(map[string]any{"type": "maintenance_events", "node_id": 1, "payload": map[string]any{"version": maintenance.WireVersion, "events": []maintenance.Event{e}}}))
		var envelope struct {
			Type    string                      `json:"type"`
			Payload maintenance.Acknowledgement `json:"payload"`
		}
		require.NoError(t, conn.ReadJSON(&envelope))
		require.Equal(t, "maintenance_ack", envelope.Type)
		return envelope.Payload
	}
	require.True(t, send(event).Events[0].Persisted)
	n, err := service.ProcessMaintenanceEvents(db, 50, time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.True(t, send(event).Events[0].Persisted)
	event.NodeID = "2"
	event.EventID = "forged-node"
	ack := send(event)
	require.False(t, ack.Events[0].Persisted)
	require.Equal(t, "identity_mismatch", ack.Events[0].Error)
	event.NodeID = "1"
	event.EventID = "db-failure"
	require.NoError(t, db.Migrator().DropTable(&model.MaintenanceEvent{}))
	require.False(t, send(event).Events[0].Persisted)
	var ticket model.MaintenanceTicket
	require.NoError(t, db.First(&ticket).Error)
	require.Contains(t, ticket.Title, "machine-telemetry")
	require.NotContains(t, ticket.Title, "secret-canary")
}

func TestMaintenanceTicketRoleClaimRecoveryCloseAndSafeDetails(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{}, &model.User{})
	now := time.Now().UTC()
	require.NoError(t, db.Create(&model.MaintenanceSettings{ID: 1, OwnerID: 10, TechnicianIDs: []uint{20}}).Error)
	first := now.Add(-time.Hour)
	ticket := model.MaintenanceTicket{StreamKey: "stream", NodeID: 1, Status: "open", FirstFailedAt: first, Title: "Readable incident"}
	require.NoError(t, db.Create(&ticket).Error)
	require.NoError(t, db.Create(&model.MaintenanceStream{Key: "stream", TicketID: &ticket.ID}).Error)
	h := &MaintenanceHandler{db: db}
	request := func(actor uint, method, path, body string, action gin.HandlerFunc) *httptest.ResponseRecorder {
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set("user_id", actor) })
		router.Handle(method, "/tickets/:id", action)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}
	require.Equal(t, 403, request(30, "POST", "/tickets/1", "", h.Claim).Code)
	require.Equal(t, 200, request(20, "POST", "/tickets/1", "", h.Claim).Code)
	require.Equal(t, 409, request(10, "POST", "/tickets/1", "", h.Claim).Code)
	require.Equal(t, 409, request(20, "POST", "/tickets/1", `{"note":"handled"}`, h.Close).Code)
	require.NoError(t, db.First(&ticket).Error)
	require.True(t, ticket.FirstFailedAt.Equal(first))
	require.NoError(t, db.Model(&ticket).Update("status", "recovered").Error)
	require.Equal(t, 400, request(20, "POST", "/tickets/1", `{"note":""}`, h.Close).Code)
	require.Equal(t, 200, request(20, "POST", "/tickets/1", `{"note":"Restarted instance; token=SECRET_CANARY"}`, h.Close).Code)
	detail := request(20, "GET", "/tickets/1", "", h.Detail)
	require.Equal(t, 200, detail.Code)
	require.NotContains(t, detail.Body.String(), "SECRET_CANARY")
	require.Contains(t, detail.Body.String(), "closed")
	var customerTickets int64
	if db.Migrator().HasTable(&model.Ticket{}) {
		require.NoError(t, db.Model(&model.Ticket{}).Count(&customerTickets).Error)
	}
	require.Zero(t, customerTickets)
}

func TestMaintenanceSettingsVerificationCannotBeForged(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.User{})
	now := time.Now().UTC()
	require.NoError(t, db.Create(&model.User{ID: 10, Email: "owner@example.invalid", Password: "test-only"}).Error)
	input := model.MaintenanceSettings{OwnerID: 10, Contacts: []model.MaintenanceContact{{UserID: 10, Email: "owner@example.invalid", TelegramChatID: "10"}}}
	_, err := service.SaveMaintenanceSettings(db, 10, false, input, now)
	require.Error(t, err)
	settings, err := service.SaveMaintenanceSettings(db, 10, true, input, now)
	require.NoError(t, err)
	settings.Enabled = true
	settings.OwnerEmailVerifiedAt = &now
	settings.OwnerTelegramVerifiedAt = &now
	_, err = service.SaveMaintenanceSettings(db, 10, true, settings, now)
	require.Error(t, err)
	require.NoError(t, service.QueueMaintenanceVerification(db, 10, "email", now))
	var delivery model.MaintenanceDelivery
	require.NoError(t, db.First(&delivery).Error)
	code := strings.TrimPrefix(delivery.DedupeKey, "verify:")
	require.Error(t, service.ConfirmMaintenanceVerification(db, 10, "email", code, now))
	require.NoError(t, db.Model(&delivery).Update("status", "sent").Error)
	require.NoError(t, service.ConfirmMaintenanceVerification(db, 10, "email", code, now))
	require.Error(t, service.ConfirmMaintenanceVerification(db, 10, "email", code, now))
	safe, _ := json.Marshal(delivery)
	require.NotContains(t, string(safe), code)
}

func TestMaintenanceBootstrapAndTechnicianCanLoadWorkbench(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	h := &MaintenanceHandler{db: db}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(10)); c.Set("is_admin", true) })
	router.GET("/me", h.Me)
	router.GET("/settings", h.Settings)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/me", nil))
	require.Equal(t, 200, rec.Code)
	require.Contains(t, rec.Body.String(), "bootstrap")
	require.NoError(t, db.Create(&model.MaintenanceSettings{ID: 1, OwnerID: 20, TechnicianIDs: []uint{10}}).Error)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/settings", nil))
	require.Equal(t, 200, rec.Code)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/me", nil))
	require.Equal(t, 200, rec.Code)
	require.Contains(t, rec.Body.String(), "technician")
}
