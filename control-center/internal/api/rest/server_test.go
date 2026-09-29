package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/control-center/internal/core/plugin"
	"github.com/AnixOps/anix-control/control-center/internal/security/auth"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestServerConfig(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        "test",
		PluginMgr:   nil,
		JWTManager:  nil,
		RBACManager: nil,
	}

	if cfg.Mode != "test" {
		t.Errorf("expected mode 'test', got '%s'", cfg.Mode)
	}
}

func TestNewServer(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.router == nil {
		t.Error("router not initialized")
	}
}

func TestServer_Router(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)
	router := s.Router()

	if router == nil {
		t.Error("Router returned nil")
	}
}

func TestServer_Stop(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)
	err := s.Stop()

	// Stop currently returns nil (TODO implementation)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestServer_AuthRequired_NoToken(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)

	// Create a test route with AuthRequired middleware
	s.router.GET("/protected", s.AuthRequired(), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestServer_AuthRequired_InvalidToken(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)
	s.router.GET("/protected", s.AuthRequired(), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestServer_AuthRequired_ValidToken(t *testing.T) {
	jwtManager := auth.NewJWTManager("test-secret", 3600, 86400, "test")
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  jwtManager,
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)
	s.router.GET("/protected", s.AuthRequired(), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Generate a valid token
	token, err := jwtManager.GenerateAccessToken("user-123", "admin", []string{"read", "write"}, "local")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestServer_RequireRole(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)

	// Create a test route with RequireRole middleware
	s.router.GET("/admin-only", func(c *gin.Context) {
		c.Set("role", "user")
		c.Next()
	}, s.RequireRole("admin"), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/admin-only", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

func TestServer_RequireRole_Admin(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)
	s.router.GET("/admin-only", func(c *gin.Context) {
		c.Set("role", "admin")
		c.Next()
	}, s.RequireRole("admin"), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/admin-only", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestServer_RequirePermission(t *testing.T) {
	rbacManager := auth.NewRBACManager()
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: rbacManager,
	}

	s := NewServer(cfg)
	s.router.GET("/resource", func(c *gin.Context) {
		c.Set("role", "guest")
		c.Next()
	}, s.RequirePermission("resource", "read"), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/resource", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

func TestServer_ExtractToken_AuthHeader(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)

	// Test token extraction from header
	var extractedToken string
	s.router.GET("/test", func(c *gin.Context) {
		extractedToken = s.extractToken(c)
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer my-token")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if extractedToken != "my-token" {
		t.Errorf("expected token 'my-token', got '%s'", extractedToken)
	}
}

func TestServer_ExtractToken_QueryParam(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)

	var extractedToken string
	s.router.GET("/test", func(c *gin.Context) {
		extractedToken = s.extractToken(c)
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/test?token=query-token", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if extractedToken != "query-token" {
		t.Errorf("expected token 'query-token', got '%s'", extractedToken)
	}
}

func TestServer_ExtractToken_Cookie(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)

	var extractedToken string
	s.router.GET("/test", func(c *gin.Context) {
		extractedToken = s.extractToken(c)
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: "cookie-token"})
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if extractedToken != "cookie-token" {
		t.Errorf("expected token 'cookie-token', got '%s'", extractedToken)
	}
}

func TestServer_HealthEndpoint(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestServer_ReadyEndpoint(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)

	req, _ := http.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestServer_LoginEndpoint(t *testing.T) {
	cfg := &ServerConfig{
		Mode:        gin.TestMode,
		PluginMgr:   plugin.NewManager(),
		JWTManager:  auth.NewJWTManager("test-secret", 3600, 86400, "test"),
		RBACManager: auth.NewRBACManager(),
	}

	s := NewServer(cfg)

	// Login endpoint should be accessible without auth
	req, _ := http.NewRequest("POST", "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Should not return 401 (even if 400 for missing body)
	if w.Code == http.StatusUnauthorized {
		t.Error("login endpoint should not require authentication")
	}
}
