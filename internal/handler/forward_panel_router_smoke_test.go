package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/middleware"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"
)

type ForwardPanelRouterSmokeTestSuite struct {
	HandlerTestSuite
	adminToken string
}

func (s *ForwardPanelRouterSmokeTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.router = gin.New()
	config.Set(s.cfg)
	s.router.Use(middleware.Recovery())
	authHandler := NewAuthHandler(s.cfg)
	systemHandler := NewSystemHandler()
	forwardHandler := NewForwardHandler()

	v2 := s.router.Group("/api/v2")
	v2.POST("/login", authHandler.Login)

	admin := v2.Group("/admin")
	admin.Use(middleware.JWTAuth())
	admin.Use(middleware.AdminAuth())
	admin.PUT("/system/configs/:key", systemHandler.SetConfig)
	admin.POST("/forward/nodes", forwardHandler.CreateNode)
	admin.POST("/forward/create", forwardHandler.CreatePanelForward)
	admin.POST("/forward/list", forwardHandler.ListPanelForwards)
	admin.GET("/forward/runtime/jobs", forwardHandler.ListPanelRuntimeJobs)
	admin.GET("/forward/runtime/status", forwardHandler.GetPanelRuntimeStatus)
	admin.GET("/forward/runtime/doctor", forwardHandler.DiagnosePanelRuntime)
	admin.POST("/forward/pause", forwardHandler.PausePanelForward)
	admin.POST("/forward/resume", forwardHandler.ResumePanelForward)
	admin.POST("/forward/force-delete", forwardHandler.ForceDeletePanelForward)
	admin.POST("/tunnel/create", forwardHandler.CreatePanelTunnel)

	s.adminToken = s.loginAdmin("smoke-admin@example.com", "SmokePass123!")
}

func (s *ForwardPanelRouterSmokeTestSuite) TestNftablesAnsibleFullChain() {
	tempDir := s.T().TempDir()
	s.mustWriteFile(filepath.Join(tempDir, "inventory.ini"), "[forward_nodes]\nrelay ansible_host=192.0.2.10\n")
	s.mustWriteFile(filepath.Join(tempDir, "forward_apply.yml"), "---\n- hosts: all\n  tasks:\n    - debug: msg=\"apply\"\n")
	s.mustWriteFile(filepath.Join(tempDir, "forward_remove.yml"), "---\n- hosts: all\n  tasks:\n    - debug: msg=\"remove\"\n")
	s.mustWriteFile(filepath.Join(tempDir, "ansible.cfg"), "[defaults]\nhost_key_checking = False\n")
	fakeCommand := s.mustWriteFakeCommand(tempDir, "ansible-playbook", "smoke-runtime")

	s.mustSetSystemConfig("forward.runtime.nodex_mode", false, "bool", "Enable NodeX forward runtime mode")
	s.mustSetSystemConfig("forward.runtime_backend", model.ForwardRuntimeBackendNftablesAnsible, "string", "Forward runtime backend")
	s.mustSetSystemConfig("forward.runtime.ansible.backend", model.ForwardRuntimeBackendNftablesAnsible, "string", "Forward runtime local backend")
	s.mustSetSystemConfig("forward.runtime.ansible.config", map[string]any{
		"inventory":      "inventory.ini",
		"playbookApply":  "forward_apply.yml",
		"playbookRemove": "forward_remove.yml",
		"command":        fakeCommand,
		"workingDir":     tempDir,
		"targetPattern":  "{{node.host}}",
		"timeoutSeconds": 30,
		"environment": map[string]string{
			"ANSIBLE_CONFIG": "ansible.cfg",
		},
	}, "json", "Forward runtime ansible config")

	statusBody := s.mustPanelRequest(http.MethodGet, "/api/v2/admin/forward/runtime/status", nil)
	statusData := s.mustMap(statusBody["data"])
	statusConfig := s.mustMap(statusData["config"])
	statusReady := s.mustMap(statusData["runtimeReady"])
	statusLocal := s.mustMap(statusData["localAnsible"])
	s.Equal(model.ForwardRuntimeBackendNftablesAnsible, s.mustString(statusConfig["backend"]))
	s.False(s.mustBool(statusConfig["nodeXMode"]))
	s.True(s.mustBool(statusReady["ready"]))
	s.True(s.mustBool(statusLocal["commandFound"]))
	s.True(s.mustBool(statusLocal["inventoryExists"]))
	s.True(s.mustBool(statusLocal["applyPlaybookExists"]))
	s.True(s.mustBool(statusLocal["removePlaybookExists"]))

	doctorBody := s.mustPanelRequest(http.MethodGet, "/api/v2/admin/forward/runtime/doctor", nil)
	doctorData := s.mustMap(doctorBody["data"])
	doctorCommands := s.mustMap(doctorData["commands"])
	s.NotEmpty(s.mustSlice(doctorCommands["powerShell"]))
	s.NotEmpty(s.mustSlice(doctorCommands["bash"]))

	nodeID := s.mustCreateForwardNode("Ansible Relay", model.ForwardNodeTypeRelay, "192.0.2.10")
	tunnelID := s.mustCreatePanelTunnel(map[string]any{
		"name":          "Ansible Tunnel",
		"inNodeId":      nodeID,
		"type":          1,
		"flow":          1,
		"trafficRatio":  1.0,
		"tcpListenAddr": "[::]",
		"udpListenAddr": "[::]",
		"protocol":      "tcp",
	})
	forwardID := s.mustCreatePanelForward(map[string]any{
		"name":          "Ansible Forward",
		"tunnelId":      tunnelID,
		"inPort":        21001,
		"remoteAddr":    "127.0.0.1:443",
		"interfaceName": "eth0",
		"strategy":      "fifo",
	})

	jobs := s.mustListRuntimeJobs(map[string]string{
		"backend":    model.ForwardRuntimeBackendNftablesAnsible,
		"forward_id": fmt.Sprintf("%d", forwardID),
		"limit":      "10",
	})
	s.Len(jobs, 1)
	s.Equal(model.ForwardRuntimeJobActionCreate, s.mustString(jobs[0]["action"]))
	s.Equal(model.ForwardRuntimeJobStatusPending, s.mustInt(jobs[0]["status"]))

	s.mustRunLocalExecutor()

	jobs = s.mustListRuntimeJobs(map[string]string{
		"backend":    model.ForwardRuntimeBackendNftablesAnsible,
		"forward_id": fmt.Sprintf("%d", forwardID),
		"limit":      "10",
	})
	s.Len(jobs, 1)
	s.Equal(model.ForwardRuntimeJobStatusSuccess, s.mustInt(jobs[0]["status"]))
	s.Contains(s.mustString(jobs[0]["result"]), "smoke-runtime")

	forwards := s.mustListPanelForwards()
	s.Require().Len(forwards, 1)
	s.Equal(forwardID, s.mustUint(forwards[0]["id"]))
	s.Equal(model.ForwardRuntimeBackendNftablesAnsible, s.mustString(forwards[0]["runtimeBackend"]))
	s.Equal(model.ForwardRuntimeJobStatusSuccess, s.mustInt(forwards[0]["runtimeStatus"]))
	s.Equal(model.ForwardStatusActive, s.mustInt(forwards[0]["status"]))

	s.mustPanelRequest(http.MethodPost, "/api/v2/admin/forward/pause", map[string]any{"id": forwardID})
	s.mustRunLocalExecutor()
	jobs = s.mustListRuntimeJobs(map[string]string{
		"backend":    model.ForwardRuntimeBackendNftablesAnsible,
		"forward_id": fmt.Sprintf("%d", forwardID),
		"limit":      "10",
	})
	s.Len(jobs, 2)
	s.Equal(model.ForwardRuntimeJobActionPause, s.mustString(jobs[0]["action"]))
	s.Equal(model.ForwardRuntimeJobStatusSuccess, s.mustInt(jobs[0]["status"]))

	forwards = s.mustListPanelForwards()
	s.Require().Len(forwards, 1)
	s.Equal(model.ForwardStatusPaused, s.mustInt(forwards[0]["status"]))

	s.mustPanelRequest(http.MethodPost, "/api/v2/admin/forward/resume", map[string]any{"id": forwardID})
	s.mustRunLocalExecutor()
	jobs = s.mustListRuntimeJobs(map[string]string{
		"backend":    model.ForwardRuntimeBackendNftablesAnsible,
		"forward_id": fmt.Sprintf("%d", forwardID),
		"limit":      "10",
	})
	s.Len(jobs, 3)
	s.Equal(model.ForwardRuntimeJobActionResume, s.mustString(jobs[0]["action"]))
	s.Equal(model.ForwardRuntimeJobStatusSuccess, s.mustInt(jobs[0]["status"]))

	forwards = s.mustListPanelForwards()
	s.Require().Len(forwards, 1)
	s.Equal(model.ForwardStatusActive, s.mustInt(forwards[0]["status"]))

	s.mustPanelRequest(http.MethodPost, "/api/v2/admin/forward/force-delete", map[string]any{"id": forwardID})
	s.mustRunLocalExecutor()
	jobs = s.mustListRuntimeJobs(map[string]string{
		"backend":    model.ForwardRuntimeBackendNftablesAnsible,
		"forward_id": fmt.Sprintf("%d", forwardID),
		"limit":      "10",
	})
	s.Len(jobs, 4)
	s.Equal(model.ForwardRuntimeJobActionDelete, s.mustString(jobs[0]["action"]))
	s.Equal(model.ForwardRuntimeJobStatusSuccess, s.mustInt(jobs[0]["status"]))
	s.Empty(s.mustListPanelForwards())
}

func (s *ForwardPanelRouterSmokeTestSuite) TestNodeXGostFullChain() {
	mock := newNodeXRuntimeMockServer(s.T(), "nodex-smoke-token")
	defer mock.Close()

	s.mustSetSystemConfig("forward.runtime.nodex_mode", true, "bool", "Enable NodeX forward runtime mode")
	s.mustSetSystemConfig("forward.runtime_backend", model.ForwardRuntimeBackendGost, "string", "Forward runtime backend")
	s.mustSetSystemConfig("forward.runtime.nodex.base_url", mock.URL, "string", "Forward runtime NodeX base URL")
	s.mustSetSystemConfig("forward.runtime.nodex.token", "nodex-smoke-token", "string", "Forward runtime NodeX token")
	s.mustSetSystemConfig("forward.runtime.nodex.timeout_seconds", 15, "int", "Forward runtime NodeX timeout")

	statusBody := s.mustPanelRequest(http.MethodGet, "/api/v2/admin/forward/runtime/status", nil)
	statusData := s.mustMap(statusBody["data"])
	statusConfig := s.mustMap(statusData["config"])
	statusRemote := s.mustMap(statusData["runtimeStatus"])
	statusReady := s.mustMap(statusData["runtimeReady"])
	s.Equal(model.ForwardRuntimeBackendGost, s.mustString(statusConfig["backend"]))
	s.True(s.mustBool(statusConfig["nodeXMode"]))
	s.Equal("v0.0.18-smoke", s.mustString(statusRemote["version"]))
	s.True(s.mustBool(statusReady["ready"]))

	doctorBody := s.mustPanelRequest(http.MethodGet, "/api/v2/admin/forward/runtime/doctor", nil)
	doctorData := s.mustMap(doctorBody["data"])
	doctorCommands := s.mustMap(doctorData["commands"])
	s.NotEmpty(s.mustSlice(doctorCommands["bash"]))

	nodeID := s.mustCreateForwardNode("NodeX Relay", model.ForwardNodeTypeRelay, "198.51.100.10")
	tunnelID := s.mustCreatePanelTunnel(map[string]any{
		"name":          "NodeX Tunnel",
		"inNodeId":      nodeID,
		"type":          1,
		"flow":          1,
		"trafficRatio":  1.0,
		"tcpListenAddr": "[::]",
		"udpListenAddr": "[::]",
		"protocol":      "tcp",
	})
	forwardID := s.mustCreatePanelForward(map[string]any{
		"name":          "NodeX Forward",
		"tunnelId":      tunnelID,
		"inPort":        22001,
		"remoteAddr":    "127.0.0.1:8443",
		"interfaceName": "eth0",
		"strategy":      "fifo",
	})

	s.mustPanelRequest(http.MethodPost, "/api/v2/admin/forward/pause", map[string]any{"id": forwardID})
	s.mustPanelRequest(http.MethodPost, "/api/v2/admin/forward/resume", map[string]any{"id": forwardID})
	s.mustPanelRequest(http.MethodPost, "/api/v2/admin/forward/force-delete", map[string]any{"id": forwardID})

	jobs := s.mustListRuntimeJobs(map[string]string{
		"backend":    model.ForwardRuntimeBackendGost,
		"forward_id": fmt.Sprintf("%d", forwardID),
		"limit":      "10",
	})
	s.Len(jobs, 4)
	for _, job := range jobs {
		s.Equal(model.ForwardRuntimeJobStatusSuccess, s.mustInt(job["status"]))
	}
	s.Empty(s.mustListPanelForwards())
	s.Equal([]string{
		model.ForwardRuntimeJobActionCreate,
		model.ForwardRuntimeJobActionPause,
		model.ForwardRuntimeJobActionResume,
		model.ForwardRuntimeJobActionDelete,
	}, mock.Actions())
}

func (s *ForwardPanelRouterSmokeTestSuite) loginAdmin(email, password string) string {
	s.T().Helper()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	s.Require().NoError(err)
	admin := &model.User{
		Email:    email,
		Password: string(hashedPassword),
		UUID:     uuid.NewString(),
		Token:    uuid.NewString(),
		IsAdmin:  1,
		Banned:   0,
	}
	s.Require().NoError(database.Get().Create(admin).Error)

	body := s.mustRequest(http.MethodPost, "/api/v2/login", map[string]any{
		"email":    email,
		"password": password,
	}, "")
	data := s.mustMap(body["data"])
	return s.mustString(data["token"])
}

func (s *ForwardPanelRouterSmokeTestSuite) mustSetSystemConfig(key string, value any, valueType, description string) {
	s.T().Helper()
	body := s.mustRequest(http.MethodPut, "/api/v2/admin/system/configs/"+key, map[string]any{
		"value":       value,
		"type":        valueType,
		"group":       "forward",
		"description": description,
	}, s.adminToken)
	message := body["message"]
	if data, ok := body["data"].(map[string]any); ok && data["message"] != nil {
		message = data["message"]
	}
	s.Equal("config updated", s.mustString(message))
}

func (s *ForwardPanelRouterSmokeTestSuite) mustCreateForwardNode(name, nodeType, host string) uint {
	s.T().Helper()
	body := s.mustRequest(http.MethodPost, "/api/v2/admin/forward/nodes", map[string]any{
		"name":      name,
		"type":      nodeType,
		"host":      host,
		"port":      22,
		"api_port":  18080,
		"api_token": "relay-token",
		"region":    "test",
		"isp":       "local",
	}, s.adminToken)
	data := s.mustMap(body["data"])
	return s.mustUint(data["id"])
}

func (s *ForwardPanelRouterSmokeTestSuite) mustCreatePanelTunnel(payload map[string]any) uint {
	s.T().Helper()
	body := s.mustPanelRequest(http.MethodPost, "/api/v2/admin/tunnel/create", payload)
	data := s.mustMap(body["data"])
	return s.mustUint(data["id"])
}

func (s *ForwardPanelRouterSmokeTestSuite) mustCreatePanelForward(payload map[string]any) uint {
	s.T().Helper()
	body := s.mustPanelRequest(http.MethodPost, "/api/v2/admin/forward/create", payload)
	data := s.mustMap(body["data"])
	return s.mustUint(data["id"])
}

func (s *ForwardPanelRouterSmokeTestSuite) mustListRuntimeJobs(filters map[string]string) []map[string]any {
	s.T().Helper()
	query := url.Values{}
	for key, value := range filters {
		if strings.TrimSpace(value) != "" {
			query.Set(key, value)
		}
	}
	path := "/api/v2/admin/forward/runtime/jobs"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	body := s.mustPanelRequest(http.MethodGet, path, nil)
	data := s.mustMap(body["data"])
	return s.toMapSlice(data["list"])
}

func (s *ForwardPanelRouterSmokeTestSuite) mustListPanelForwards() []map[string]any {
	s.T().Helper()
	body := s.mustPanelRequest(http.MethodPost, "/api/v2/admin/forward/list", map[string]any{})
	return s.toMapSlice(body["data"])
}

func (s *ForwardPanelRouterSmokeTestSuite) mustRunLocalExecutor() {
	s.T().Helper()
	executor := service.NewPanelForwardRuntimeJobExecutor(database.Get())
	s.Require().NoError(executor.RunPendingJobs(context.Background()))
}

func (s *ForwardPanelRouterSmokeTestSuite) mustPanelRequest(method, path string, payload any) map[string]any {
	s.T().Helper()
	body := s.mustRequest(method, path, payload, s.adminToken)
	s.Equal(float64(0), body["code"], "unexpected panel response: %#v", body)
	return body
}

func (s *ForwardPanelRouterSmokeTestSuite) mustRequest(method, path string, payload any, token string) map[string]any {
	s.T().Helper()

	var requestBody *bytes.Reader
	if payload == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(payload)
		s.Require().NoError(err)
		requestBody = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, requestBody)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, req)
	s.Equal(http.StatusOK, recorder.Code, "unexpected status for %s %s: %s", method, path, recorder.Body.String())

	if recorder.Body.Len() == 0 {
		return map[string]any{}
	}

	var body map[string]any
	s.Require().NoError(json.Unmarshal(recorder.Body.Bytes(), &body), "invalid json response for %s %s: %s", method, path, recorder.Body.String())
	return body
}

func (s *ForwardPanelRouterSmokeTestSuite) mustWriteFile(path, content string) {
	s.T().Helper()
	s.Require().NoError(os.WriteFile(path, []byte(content), 0o600))
}

func (s *ForwardPanelRouterSmokeTestSuite) mustWriteFakeCommand(dir, baseName, marker string) string {
	s.T().Helper()

	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, baseName+".cmd")
		s.Require().NoError(os.WriteFile(path, []byte("@echo off\r\necho "+marker+" %*\r\nexit /b 0\r\n"), 0o600))
		return path
	}

	path := filepath.Join(dir, baseName)
	script := "#!/bin/sh\n" +
		"echo " + marker + " \"$@\"\n"
	s.Require().NoError(os.WriteFile(path, []byte(script), 0o700))
	s.Require().NoError(os.Chmod(path, 0o700))
	return path
}

func (s *ForwardPanelRouterSmokeTestSuite) mustMap(value any) map[string]any {
	s.T().Helper()
	result, ok := value.(map[string]any)
	s.Require().True(ok, "expected map, got %T", value)
	return result
}

func (s *ForwardPanelRouterSmokeTestSuite) mustSlice(value any) []any {
	s.T().Helper()
	result, ok := value.([]any)
	s.Require().True(ok, "expected slice, got %T", value)
	return result
}

func (s *ForwardPanelRouterSmokeTestSuite) toMapSlice(value any) []map[string]any {
	s.T().Helper()
	items := s.mustSlice(value)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, s.mustMap(item))
	}
	return result
}

func (s *ForwardPanelRouterSmokeTestSuite) mustUint(value any) uint {
	s.T().Helper()
	switch typed := value.(type) {
	case float64:
		return uint(typed)
	case int:
		return uint(typed)
	case int64:
		return uint(typed)
	case uint:
		return typed
	default:
		s.T().Fatalf("expected uint-compatible value, got %T", value)
		return 0
	}
}

func (s *ForwardPanelRouterSmokeTestSuite) mustInt(value any) int {
	s.T().Helper()
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	default:
		s.T().Fatalf("expected int-compatible value, got %T", value)
		return 0
	}
}

func (s *ForwardPanelRouterSmokeTestSuite) mustString(value any) string {
	s.T().Helper()
	result, ok := value.(string)
	s.Require().True(ok, "expected string, got %T", value)
	return result
}

func (s *ForwardPanelRouterSmokeTestSuite) mustBool(value any) bool {
	s.T().Helper()
	result, ok := value.(bool)
	s.Require().True(ok, "expected bool, got %T", value)
	return result
}

type nodeXRuntimeMockServer struct {
	*httptest.Server
	token   string
	mu      sync.Mutex
	actions []string
}

func newNodeXRuntimeMockServer(t *testing.T, token string) *nodeXRuntimeMockServer {
	t.Helper()

	mock := &nodeXRuntimeMockServer{token: token}
	mock.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte("ok"))
		case "/api/v2/internal/forward/runtime/status":
			if got := r.Header.Get("Authorization"); got != "Bearer "+mock.token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"version":"v0.0.18-smoke","executePath":"/api/v2/internal/forward/runtime/execute","statusPath":"/api/v2/internal/forward/runtime/status","authRequired":true,"supports":{"resourceTypes":["panel_forward"],"backends":["gost","iptables_ansible"],"actions":["create","update","delete","pause","resume","sync"]},"modes":{"gost":{"supported":true},"iptablesAnsible":{"supported":true,"ready":true,"command":"ansible-playbook","commandFound":true,"inventoryPath":"inventory.ini","inventoryExists":true,"applyPlaybookPath":"apply.yml","applyPlaybookExists":true,"removePlaybookPath":"remove.yml","removePlaybookExists":true,"workingDir":"playbooks","workingDirExists":true,"targetPattern":"relay","become":false,"timeoutSeconds":30}}}}`))
		case "/api/v2/internal/forward/runtime/execute":
			if got := r.Header.Get("Authorization"); got != "Bearer "+mock.token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			defer func() {
				if err := r.Body.Close(); err != nil {
					t.Errorf("close runtime execute request body: %v", err)
				}
			}()
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			action, _ := payload["action"].(string)
			mock.mu.Lock()
			mock.actions = append(mock.actions, action)
			mock.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"backend":"gost","status":2,"message":"mock runtime ok","result":"ok","async":false}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	return mock
}

func (m *nodeXRuntimeMockServer) Actions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.actions))
	copy(result, m.actions)
	return result
}

func TestForwardPanelRouterSmokeTestSuite(t *testing.T) {
	suite.Run(t, new(ForwardPanelRouterSmokeTestSuite))
}
