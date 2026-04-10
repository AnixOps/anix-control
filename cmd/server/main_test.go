package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestResolveConfigPathPrefersExplicitFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("env: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveConfigPath(configPath)
	if err != nil {
		t.Fatalf("resolveConfigPath() error = %v", err)
	}
	if resolved != configPath {
		t.Fatalf("resolveConfigPath() = %q, want %q", resolved, configPath)
	}
}

func TestResolveRuntimePathPrefersProjectRootWhenConfigIsUnderConfigDir(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := filepath.Join(tempDir, "repo")
	configDir := filepath.Join(projectRoot, "config")
	targetPath := filepath.Join(projectRoot, "config", "deploy", "ansible", "inventory.ini")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("[forward_nodes]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved := resolveRuntimePath(
		filepath.Join("config", "deploy", "ansible", "inventory.ini"),
		filepath.Join(configDir, "config.yaml"),
	)
	if resolved != targetPath {
		t.Fatalf("resolveRuntimePath() = %q, want %q", resolved, targetPath)
	}
}

func TestApplyTrustedProxiesDisabledByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := applyTrustedProxies(r, nil); err != nil {
		t.Fatalf("applyTrustedProxies() error = %v", err)
	}
	r.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.8")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Body.String(); got != "203.0.113.10" {
		t.Fatalf("ClientIP() = %q, want %q when trusted proxies are disabled", got, "203.0.113.10")
	}
}

func TestApplyTrustedProxiesHonorsConfiguredProxyChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := applyTrustedProxies(r, []string{"127.0.0.1"}); err != nil {
		t.Fatalf("applyTrustedProxies() error = %v", err)
	}
	r.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.8")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Body.String(); got != "198.51.100.8" {
		t.Fatalf("ClientIP() = %q, want %q when proxy is trusted", got, "198.51.100.8")
	}
}
