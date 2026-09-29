package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/control-center/internal/core/plugin"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestRouter() *gin.Engine {
	h := NewHandlers(nil)
	router := gin.New()

	// Health
	router.GET("/health", h.HealthCheck)
	router.GET("/ready", h.ReadinessCheck)

	// Auth
	router.POST("/login", h.Login)
	router.POST("/logout", h.Logout)
	router.POST("/refresh", h.RefreshToken)
	router.GET("/me", h.GetCurrentUser)

	// Plugins
	router.GET("/plugins", h.ListPlugins)
	router.GET("/plugins/:name", h.GetPlugin)
	router.POST("/plugins/:name/execute", h.ExecutePlugin)
	router.GET("/plugins/:name/status", h.GetPluginStatus)
	router.GET("/plugins/:name/capabilities", h.GetPluginCapabilities)
	router.POST("/plugins/:name/start", h.StartPlugin)
	router.POST("/plugins/:name/stop", h.StopPlugin)
	router.POST("/plugins/:name/restart", h.RestartPlugin)

	// Nodes
	router.GET("/nodes", h.ListNodes)
	router.POST("/nodes", h.CreateNode)
	router.GET("/nodes/:id", h.GetNode)
	router.PUT("/nodes/:id", h.UpdateNode)
	router.DELETE("/nodes/:id", h.DeleteNode)
	router.GET("/nodes/:id/stats", h.GetNodeStats)

	// Agents
	router.GET("/agents", h.ListAgents)
	router.POST("/agents/:id/connect", h.ConnectAgent)
	router.POST("/agents/:id/disconnect", h.DisconnectAgent)
	router.POST("/agents/:id/exec", h.ExecAgent)
	router.GET("/agents/:id/info", h.GetAgentInfo)

	// Playbooks
	router.GET("/playbooks", h.ListPlaybooks)
	router.GET("/playbooks/:name", h.GetPlaybook)
	router.POST("/playbooks/:name/run", h.RunPlaybook)
	router.POST("/playbooks/:name/validate", h.ValidatePlaybook)

	// Inventory
	router.GET("/inventory", h.GetInventory)
	router.GET("/hosts", h.ListHosts)

	// Users
	router.GET("/users", h.ListUsers)
	router.POST("/users", h.CreateUser)
	router.GET("/users/:id", h.GetUser)
	router.PUT("/users/:id", h.UpdateUser)
	router.DELETE("/users/:id", h.DeleteUser)
	router.POST("/users/:id/ban", h.BanUser)
	router.POST("/users/:id/unban", h.UnbanUser)
	router.GET("/users/:id/subscriptions", h.GetUserSubscriptions)

	// Orders
	router.GET("/orders", h.ListOrders)
	router.GET("/orders/:id", h.GetOrder)

	// Plans
	router.GET("/plans", h.ListPlans)
	router.GET("/plans/:id", h.GetPlan)

	// Dashboard
	router.GET("/dashboard", h.GetDashboard)
	router.GET("/dashboard/stats", h.GetDashboardStats)

	// Logs
	router.GET("/logs", h.GetLogs)
	router.GET("/logs/stream", h.StreamLogs)

	// Settings
	router.GET("/settings", h.GetSettings)
	router.PUT("/settings", h.UpdateSettings)

	// Events
	router.GET("/events/stream", h.StreamEvents)

	// Admin
	router.GET("/admin/stats", h.GetAdminStats)
	router.GET("/admin/audit-logs", h.GetAuditLogs)
	router.GET("/admin/system-info", h.GetSystemInfo)
	router.POST("/admin/backup", h.CreateBackup)
	router.GET("/admin/backups", h.ListBackups)

	return router
}

func TestHealthCheck(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %v", response["status"])
	}
}

func TestReadinessCheck(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["status"] != "ready" {
		t.Errorf("expected status 'ready', got %v", response["status"])
	}
}

func TestLogin_Success(t *testing.T) {
	router := setupTestRouter()

	body := LoginRequest{
		Email:    "admin@example.com",
		Password: "admin123456",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/login", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["access_token"] != "demo-token" {
		t.Errorf("expected access_token 'demo-token', got %v", response["access_token"])
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	router := setupTestRouter()

	body := LoginRequest{
		Email:    "wrong@example.com",
		Password: "wrongpass",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/login", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestLogin_InvalidRequest(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/login", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestLogout(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/logout", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRefreshToken(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/refresh", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["access_token"] != "new-token" {
		t.Errorf("expected access_token 'new-token', got %v", response["access_token"])
	}
}

func TestGetCurrentUser(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestListPlugins(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/plugins", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGetPlugin(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/plugins/ansible", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["name"] != "ansible" {
		t.Errorf("expected name 'ansible', got %v", response["name"])
	}
}

func TestExecutePlugin(t *testing.T) {
	router := setupTestRouter()

	body := map[string]interface{}{
		"action": "test",
		"params": map[string]interface{}{"key": "value"},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/plugins/test/execute", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestExecutePlugin_InvalidBody(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/plugins/test/execute", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestGetPluginStatus(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/plugins/test/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGetPluginCapabilities(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/plugins/test/capabilities", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestStartPlugin(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/plugins/test/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestStopPlugin(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/plugins/test/stop", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRestartPlugin(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/plugins/test/restart", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestListNodes(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/nodes", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestCreateNode(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/nodes", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}
}

func TestGetNode(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/nodes/123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["id"] != "123" {
		t.Errorf("expected id '123', got %v", response["id"])
	}
}

func TestUpdateNode(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("PUT", "/nodes/123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestDeleteNode(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("DELETE", "/nodes/123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGetNodeStats(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/nodes/123/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestListAgents(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/agents", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestConnectAgent(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/agents/123/connect", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestDisconnectAgent(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/agents/123/disconnect", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestExecAgent(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/agents/123/exec", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGetAgentInfo(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/agents/123/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestListPlaybooks(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/playbooks", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGetPlaybook(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/playbooks/deploy", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["name"] != "deploy" {
		t.Errorf("expected name 'deploy', got %v", response["name"])
	}
}

func TestRunPlaybook(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/playbooks/test/run", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestValidatePlaybook(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/playbooks/test/validate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["valid"] != true {
		t.Errorf("expected valid true, got %v", response["valid"])
	}
}

func TestGetInventory(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/inventory", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestListHosts(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/hosts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestListUsers(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestListUsers_WithPagination(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/users?page=2&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["page"] != float64(2) {
		t.Errorf("expected page 2, got %v", response["page"])
	}
	if response["per_page"] != float64(10) {
		t.Errorf("expected per_page 10, got %v", response["per_page"])
	}
}

func TestCreateUser(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}
}

func TestGetUser(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/users/456", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["id"] != "456" {
		t.Errorf("expected id '456', got %v", response["id"])
	}
}

func TestUpdateUser(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("PUT", "/users/456", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestDeleteUser(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("DELETE", "/users/456", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestBanUser(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/users/456/ban", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestUnbanUser(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/users/456/unban", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGetUserSubscriptions(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/users/456/subscriptions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestListOrders(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/orders", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGetOrder(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/orders/789", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["id"] != "789" {
		t.Errorf("expected id '789', got %v", response["id"])
	}
}

func TestListPlans(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/plans", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGetPlan(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/plans/101", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["id"] != "101" {
		t.Errorf("expected id '101', got %v", response["id"])
	}
}

func TestGetDashboard(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["nodes"] == nil {
		t.Error("expected nodes in response")
	}
}

func TestGetDashboardStats(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/dashboard/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGetLogs(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/logs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestStreamLogs(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/logs/stream", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGetSettings(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestUpdateSettings(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("PUT", "/settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestStreamEvents(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/events/stream", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Check SSE headers
	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type 'text/event-stream', got '%s'", w.Header().Get("Content-Type"))
	}
}

func TestGetAdminStats(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/admin/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGetAuditLogs(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/admin/audit-logs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGetSystemInfo(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/admin/system-info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestCreateBackup(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("POST", "/admin/backup", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestListBackups(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/admin/backups", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandlers_WithPluginManager(t *testing.T) {
	pm := plugin.NewManager()
	h := NewHandlers(pm)

	if h.pluginMgr != pm {
		t.Error("expected plugin manager to be set")
	}
}

func TestLoginRequest(t *testing.T) {
	req := LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
	}

	if req.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got '%s'", req.Email)
	}
	if req.Password != "password123" {
		t.Errorf("expected password 'password123', got '%s'", req.Password)
	}
}
