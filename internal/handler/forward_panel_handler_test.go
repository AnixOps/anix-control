package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ForwardPanelHandlerTestSuite struct {
	HandlerTestSuite
	handler *ForwardHandler
}

func (s *ForwardPanelHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.handler = NewForwardHandler()
}

func (s *ForwardPanelHandlerTestSuite) TestListPanelTunnels_SucceedsAsAdmin() {
	ctx, w := s.newAdminContext("POST", "/admin/tunnel/list", nil)
	s.handler.ListPanelTunnels(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.IsType(s.T(), []interface{}{}, resp["data"])
}

func (s *ForwardPanelHandlerTestSuite) TestCreatePanelTunnel_WorksWithValidNode() {
	node := model.ForwardNode{
		Name:    "Ingress Relay",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "10.0.0.1",
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(&node).Error)

	payload := map[string]interface{}{
		"name":          "Create Tunnel",
		"inNodeId":      node.ID,
		"type":          1,
		"flow":          1,
		"trafficRatio":  1.2,
		"tcpListenAddr": "[::]",
		"udpListenAddr": "[::]",
		"protocol":      "tcp",
	}

	ctx, w := s.newAdminContext("POST", "/admin/tunnel/create", payload)
	s.handler.CreatePanelTunnel(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]interface{})
	assert.Equal(s.T(), "Create Tunnel", data["name"])
}

func (s *ForwardPanelHandlerTestSuite) TestUpdatePanelTunnel_AllowsRuntimeFieldsToChange() {
	configSvc := service.NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex_mode", "true", "bool", "forward", "enable NodeX mode for tunnel updates"))

	node := model.ForwardNode{
		Name:    "Init Relay",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "10.0.0.2",
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(&node).Error)
	exitNode := model.ForwardNode{
		Name:    "Init Exit",
		Type:    model.ForwardNodeTypeExit,
		Host:    "10.0.0.5",
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(&exitNode).Error)
	tunnel := model.ForwardTunnel{
		Name:          "Update Tunnel",
		InNodeID:      node.ID,
		OutNodeID:     &exitNode.ID,
		InIP:          node.Host,
		OutIP:         exitNode.Host,
		Type:          2,
		Flow:          1,
		Protocol:      "tcp",
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), database.Get().Create(&tunnel).Error)

	payload := map[string]interface{}{
		"id":            tunnel.ID,
		"name":          "Updated Name",
		"flow":          2,
		"trafficRatio":  2.5,
		"protocol":      "ws",
		"tcpListenAddr": "0.0.0.0",
		"udpListenAddr": "[::1]",
		"interfaceName": "eth1",
	}
	ctx, w := s.newAdminContext("POST", "/admin/tunnel/update", payload)
	s.handler.UpdatePanelTunnel(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	var updated model.ForwardTunnel
	assert.NoError(s.T(), database.Get().First(&updated, tunnel.ID).Error)
	assert.Equal(s.T(), "Updated Name", updated.Name)
	assert.Equal(s.T(), 2, updated.Flow)
	assert.Equal(s.T(), "ws", updated.Protocol)
}

func (s *ForwardPanelHandlerTestSuite) TestDeletePanelTunnel_RemovesRecord() {
	node := model.ForwardNode{
		Name:    "Delete Relay",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "10.0.0.3",
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(&node).Error)
	tunnel := model.ForwardTunnel{
		Name:          "Delete Tunnel",
		InNodeID:      node.ID,
		InIP:          node.Host,
		Type:          1,
		Flow:          1,
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), database.Get().Create(&tunnel).Error)

	ctx, w := s.newAdminContext("POST", "/admin/tunnel/delete", map[string]interface{}{"id": tunnel.ID})
	s.handler.DeletePanelTunnel(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	var count int64
	assert.NoError(s.T(), database.Get().Model(&model.ForwardTunnel{}).Where("id = ?", tunnel.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
}

func (s *ForwardPanelHandlerTestSuite) TestDiagnosePanelTunnel_ReturnsReport() {
	node := model.ForwardNode{
		Name:    "Diag Relay",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "10.0.0.4",
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
		Port:    9000,
	}
	assert.NoError(s.T(), database.Get().Create(&node).Error)
	tunnel := model.ForwardTunnel{
		Name:          "Diag Tunnel",
		InNodeID:      node.ID,
		InIP:          node.Host,
		Type:          1,
		Flow:          1,
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), database.Get().Create(&tunnel).Error)

	ctx, w := s.newAdminContext("POST", "/admin/tunnel/diagnose", map[string]interface{}{"tunnelId": tunnel.ID})
	s.handler.DiagnosePanelTunnel(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]interface{})
	assert.Equal(s.T(), float64(tunnel.ID), data["tunnelId"])
}

func (s *ForwardPanelHandlerTestSuite) TestGetPanelRuntimeStatus_ProxiesNodeXStatus() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/internal/forward/runtime/status":
			assert.Equal(s.T(), "Bearer handler-token", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"version":"v0.0.17-test.5","executePath":"/api/v2/internal/forward/runtime/execute","statusPath":"/api/v2/internal/forward/runtime/status","authRequired":true,"supports":{"resourceTypes":["panel_forward"],"backends":["gost"],"actions":["create","update","delete"]},"modes":{"gost":{"supported":true},"iptablesAnsible":{"supported":true,"ready":true,"command":"ansible-playbook","commandFound":true,"inventoryPath":"inventory.ini","inventoryExists":true,"applyPlaybookPath":"apply.yml","applyPlaybookExists":true,"removePlaybookPath":"remove.yml","removePlaybookExists":true,"workingDir":"playbooks","workingDirExists":true,"targetPattern":"relay","become":false,"timeoutSeconds":30}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configSvc := service.NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex.base_url", server.URL, "string", "forward", "handler base url"))
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex.token", "handler-token", "string", "forward", "handler token"))

	ctx, w := s.newAdminContext("GET", "/admin/forward/runtime/status", nil)
	s.handler.GetPanelRuntimeStatus(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]interface{})
	assert.Equal(s.T(), "v0.0.17-test.5", data["version"])
	assert.Equal(s.T(), "/api/v2/internal/forward/runtime/status", data["statusPath"])
}

func (s *ForwardPanelHandlerTestSuite) TestDiagnosePanelRuntime_ReturnsDoctorSummary() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte("ok"))
		case "/api/v2/internal/forward/runtime/status":
			assert.Equal(s.T(), "Bearer doctor-token", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"version":"v0.0.17-test.5","executePath":"/api/v2/internal/forward/runtime/execute","statusPath":"/api/v2/internal/forward/runtime/status","authRequired":true,"supports":{"resourceTypes":["panel_forward","legacy_rule"],"backends":["gost","iptables_ansible"],"actions":["create","update","delete","pause","resume","sync"]},"modes":{"gost":{"supported":true},"iptablesAnsible":{"supported":true,"ready":false,"command":"ansible-playbook","commandFound":false,"inventoryPath":"inventory.ini","inventoryExists":false,"applyPlaybookPath":"apply.yml","applyPlaybookExists":true,"removePlaybookPath":"remove.yml","removePlaybookExists":true,"workingDir":"playbooks","workingDirExists":true,"targetPattern":"relay","become":false,"timeoutSeconds":30,"issues":["inventory missing: inventory.ini"]}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configSvc := service.NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex.base_url", server.URL, "string", "forward", "doctor base url"))
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex.token", "doctor-token", "string", "forward", "doctor token"))

	ctx, w := s.newAdminContext("GET", "/admin/forward/runtime/doctor", nil)
	s.handler.DiagnosePanelRuntime(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]interface{})
	health := data["health"].(map[string]interface{})
	status := data["runtimeStatus"].(map[string]interface{})
	commands := data["commands"].(map[string]interface{})
	assert.Equal(s.T(), true, health["ok"])
	assert.Equal(s.T(), true, status["ok"])
	assert.Equal(s.T(), "v0.0.17-test.5", status["version"])
	assert.NotEmpty(s.T(), commands["powerShell"])
}

func (s *ForwardPanelHandlerTestSuite) newAdminContext(method, path string, payload interface{}) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	var body bytes.Buffer
	if payload != nil {
		_ = json.NewEncoder(&body).Encode(payload)
	}
	req := httptest.NewRequest(method, path, &body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req
	ctx.Set("user_id", uint(1))
	ctx.Set("is_admin", true)
	return ctx, rec
}

func TestForwardPanelHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(ForwardPanelHandlerTestSuite))
}
