package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type maintenanceE2ESender struct{ channels map[string]int }

func (s *maintenanceE2ESender) Send(_ context.Context, d model.MaintenanceDelivery) error {
	s.channels[d.Channel]++
	return nil
}
func TestMaintenanceCrossRepositoryAgentToTicket(t *testing.T) {
	binary := os.Getenv("ANIXOPS_AGENT_E2E_BINARY")
	if binary == "" {
		t.Skip("requires CI-built Agent node test binary")
	}
	db := newKernelHandlerTestDB(t, &model.Node{}, &model.ForwardNode{})
	require.NoError(t, db.Create(&model.Node{ID: 1, Name: "E2E fixture", APIKey: "test-key", Status: model.NodeStatusOnline}).Error)
	now := time.Now().UTC()
	require.NoError(t, db.Create(&model.MaintenanceSettings{ID: 1, OwnerID: 10, Enabled: true, Revision: 1, OwnerEmailVerifiedAt: &now, OwnerTelegramVerifiedAt: &now, Contacts: []model.MaintenanceContact{{UserID: 10, Email: "owner@example.invalid", TelegramChatID: "10"}}}).Error)
	h := &AgentHandler{db: db, wsUpgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}}
	router := gin.New()
	router.GET("/api/v2/node/ws", h.AgentWebSocketUnified)
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-test.run=^TestMaintenanceExternalControl$", "-test.v")
	command.Env = append(os.Environ(), "ANIXOPS_MAINTENANCE_E2E_URL="+server.URL)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "Agent integration: %s", output)
	sender := &maintenanceE2ESender{channels: map[string]int{}}
	require.NoError(t, (service.MaintenanceEventWorker{DB: db, Sender: sender}).RunOnce(ctx, time.Now()))
	var ticket model.MaintenanceTicket
	require.NoError(t, db.First(&ticket).Error)
	require.Equal(t, "machine-telemetry", ticket.PluginID)
	require.Equal(t, "PLUGIN_PROCESS_EXITED", ticket.ErrorCode)
	require.Equal(t, 1, sender.channels["email"])
	require.Equal(t, 1, sender.channels["telegram"])
	require.NoError(t, (service.MaintenanceEventWorker{DB: db, Sender: sender}).RunOnce(ctx, time.Now()))
	require.Equal(t, 1, sender.channels["email"])
	require.Equal(t, 1, sender.channels["telegram"])
}
