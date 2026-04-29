package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	var resp map[string]any
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.IsType(s.T(), []any{}, resp["data"])
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

	payload := map[string]any{
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
	var resp map[string]any
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
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

	payload := map[string]any{
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
	var resp map[string]any
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

	ctx, w := s.newAdminContext("POST", "/admin/tunnel/delete", map[string]any{"id": tunnel.ID})
	s.handler.DeletePanelTunnel(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]any
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

	ctx, w := s.newAdminContext("POST", "/admin/tunnel/diagnose", map[string]any{"tunnelId": tunnel.ID})
	s.handler.DiagnosePanelTunnel(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]any
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), float64(tunnel.ID), data["tunnelId"])
}

func (s *ForwardPanelHandlerTestSuite) TestGetPanelRuntimeStatus_ProxiesNodeXStatus() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte("ok"))
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
	var resp map[string]any
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	config := data["config"].(map[string]any)
	runtimeStatus := data["runtimeStatus"].(map[string]any)
	reachability := data["reachability"].(map[string]any)
	runtimeReady := data["runtimeReady"].(map[string]any)
	assert.Equal(s.T(), "gost", config["backend"])
	assert.Equal(s.T(), true, config["nodeXMode"])
	assert.Equal(s.T(), "v0.0.17-test.5", runtimeStatus["version"])
	assert.Equal(s.T(), "/api/v2/internal/forward/runtime/status", runtimeStatus["statusPath"])
	assert.Equal(s.T(), true, reachability["ready"])
	assert.Equal(s.T(), true, runtimeReady["ready"])
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
	var resp map[string]any
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	config := data["config"].(map[string]any)
	health := data["health"].(map[string]any)
	status := data["runtimeStatus"].(map[string]any)
	reachability := data["reachability"].(map[string]any)
	runtimeReady := data["runtimeReady"].(map[string]any)
	commands := data["commands"].(map[string]any)
	assert.Equal(s.T(), "gost", config["backend"])
	assert.Equal(s.T(), true, config["nodeXMode"])
	assert.Equal(s.T(), true, health["ok"])
	assert.Equal(s.T(), true, status["ok"])
	assert.Equal(s.T(), "v0.0.17-test.5", status["version"])
	assert.Equal(s.T(), true, reachability["ready"])
	assert.Equal(s.T(), true, runtimeReady["ready"])
	assert.NotEmpty(s.T(), commands["powerShell"])
}

func (s *ForwardPanelHandlerTestSuite) TestGetNodeXRuntimeStatus_UsesDedicatedNodeXProbe() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte("ok"))
		case "/api/v2/internal/forward/runtime/status":
			assert.Equal(s.T(), "Bearer nodex-status-token", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"version":"v0.0.18-test.5","executePath":"/api/v2/internal/forward/runtime/execute","statusPath":"/api/v2/internal/forward/runtime/status","authRequired":true,"supports":{"resourceTypes":["panel_forward","legacy_rule"],"backends":["gost","iptables_ansible"],"actions":["create","update","delete","pause","resume","sync"]},"modes":{"gost":{"supported":true},"iptablesAnsible":{"supported":true,"ready":false,"command":"ansible-playbook","commandFound":false,"inventoryPath":"inventory.ini","inventoryExists":false,"applyPlaybookPath":"apply.yml","applyPlaybookExists":true,"removePlaybookPath":"remove.yml","removePlaybookExists":true,"workingDir":"playbooks","workingDirExists":true,"targetPattern":"relay","become":false,"timeoutSeconds":30,"issues":["inventory missing: inventory.ini"]}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configSvc := service.NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex.base_url", server.URL, "string", "forward", "dedicated NodeX status base url"))
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex.token", "nodex-status-token", "string", "forward", "dedicated NodeX status token"))
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex_mode", "false", "bool", "forward", "keep global backend on local ansible"))
	assert.NoError(s.T(), configSvc.Set("forward.runtime_backend", model.ForwardRuntimeBackendIptablesAnsible, "string", "forward", "local backend"))

	ctx, w := s.newAdminContext("GET", "/admin/forward/nodex/status", nil)
	s.handler.GetNodeXRuntimeStatus(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]any
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	config := data["config"].(map[string]any)
	status := data["runtimeStatus"].(map[string]any)
	assert.Equal(s.T(), true, config["nodeXMode"])
	assert.Equal(s.T(), "v0.0.18-test.5", status["version"])
}

func (s *ForwardPanelHandlerTestSuite) TestDiagnoseNodeXRuntime_ReturnsDedicatedCommands() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte("ok"))
		case "/api/v2/internal/forward/runtime/status":
			assert.Equal(s.T(), "Bearer nodex-doctor-token", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"version":"v0.0.18-test.5","executePath":"/api/v2/internal/forward/runtime/execute","statusPath":"/api/v2/internal/forward/runtime/status","authRequired":true,"supports":{"resourceTypes":["panel_forward","legacy_rule"],"backends":["gost","iptables_ansible"],"actions":["create","update","delete","pause","resume","sync"]},"modes":{"gost":{"supported":true},"iptablesAnsible":{"supported":true,"ready":true,"command":"ansible-playbook","commandFound":true,"inventoryPath":"inventory.ini","inventoryExists":true,"applyPlaybookPath":"apply.yml","applyPlaybookExists":true,"removePlaybookPath":"remove.yml","removePlaybookExists":true,"workingDir":"playbooks","workingDirExists":true,"targetPattern":"relay","become":false,"timeoutSeconds":30}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configSvc := service.NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex.base_url", server.URL, "string", "forward", "dedicated NodeX doctor base url"))
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex.token", "nodex-doctor-token", "string", "forward", "dedicated NodeX doctor token"))

	ctx, w := s.newAdminContext("GET", "/admin/forward/nodex/doctor", nil)
	s.handler.DiagnoseNodeXRuntime(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]any
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	commands := data["commands"].(map[string]any)
	references := commands["references"].([]any)
	assert.NotEmpty(s.T(), commands["powerShell"])
	assert.Contains(s.T(), references[2], "https://github.com/zdwtest/NodeX")
}

func (s *ForwardPanelHandlerTestSuite) TestGetLocalRuntimeStatus_UsesDedicatedLocalProbe() {
	tempDir := s.T().TempDir()
	inventoryPath := filepath.Join(tempDir, "inventory.ini")
	applyPath := filepath.Join(tempDir, "apply.yml")
	removePath := filepath.Join(tempDir, "remove.yml")

	assert.NoError(s.T(), os.WriteFile(inventoryPath, []byte("[forward_nodes]\nrelay ansible_host=127.0.0.1\n"), 0o600))
	assert.NoError(s.T(), os.WriteFile(applyPath, []byte("---\n- hosts: all\n"), 0o600))
	assert.NoError(s.T(), os.WriteFile(removePath, []byte("---\n- hosts: all\n"), 0o600))

	configSvc := service.NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex_mode", "true", "bool", "forward", "active NodeX mode should not affect local probe"))
	assert.NoError(s.T(), configSvc.Set("forward.runtime_backend", model.ForwardRuntimeBackendGost, "string", "forward", "active gost backend"))
	assert.NoError(s.T(), configSvc.Set("forward.runtime.ansible.backend", model.ForwardRuntimeBackendNftablesAnsible, "string", "forward", "dedicated local backend"))
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex.base_url", "http://127.0.0.1:18081", "string", "forward", "standby NodeX url"))
	assert.NoError(s.T(), configSvc.Set("forward.runtime.nodex.token", "standby-token", "string", "forward", "standby NodeX token"))
	assert.NoError(s.T(), configSvc.Set("forward.runtime.ansible.config", fmt.Sprintf(`{"inventory":%q,"playbookApply":%q,"playbookRemove":%q,"command":"go","workingDir":%q}`, inventoryPath, applyPath, removePath, tempDir), "json", "forward", "dedicated local config"))

	ctx, w := s.newAdminContext("GET", "/admin/forward/local/status", nil)
	s.handler.GetLocalRuntimeStatus(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]any
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	config := data["config"].(map[string]any)
	local := data["localAnsible"].(map[string]any)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, config["backend"])
	assert.Equal(s.T(), false, config["nodeXMode"])
	assert.Equal(s.T(), "go", local["command"])
	assert.Equal(s.T(), true, local["ready"])
}

func (s *ForwardPanelHandlerTestSuite) TestDiagnoseLocalRuntime_ReturnsDedicatedCommands() {
	tempDir := s.T().TempDir()
	inventoryPath := filepath.Join(tempDir, "inventory.ini")
	applyPath := filepath.Join(tempDir, "apply.yml")
	removePath := filepath.Join(tempDir, "remove.yml")

	assert.NoError(s.T(), os.WriteFile(inventoryPath, []byte("[forward_nodes]\nrelay ansible_host=127.0.0.1\n"), 0o600))
	assert.NoError(s.T(), os.WriteFile(applyPath, []byte("---\n- hosts: all\n"), 0o600))
	assert.NoError(s.T(), os.WriteFile(removePath, []byte("---\n- hosts: all\n"), 0o600))

	configSvc := service.NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configSvc.Set("forward.runtime.ansible.backend", model.ForwardRuntimeBackendNftablesAnsible, "string", "forward", "dedicated local doctor backend"))
	assert.NoError(s.T(), configSvc.Set("forward.runtime.ansible.config", fmt.Sprintf(`{"inventory":%q,"playbookApply":%q,"playbookRemove":%q,"command":"go","workingDir":%q}`, inventoryPath, applyPath, removePath, tempDir), "json", "forward", "dedicated local doctor config"))

	ctx, w := s.newAdminContext("GET", "/admin/forward/local/doctor", nil)
	s.handler.DiagnoseLocalRuntime(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]any
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	commands := data["commands"].(map[string]any)
	references := commands["references"].([]any)
	assert.NotEmpty(s.T(), commands["powerShell"])
	assert.Contains(s.T(), references[0], "docs/guide/forward-relay-onboarding.md")
}

func (s *ForwardPanelHandlerTestSuite) newAdminContext(method, path string, payload any) (*gin.Context, *httptest.ResponseRecorder) {
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
