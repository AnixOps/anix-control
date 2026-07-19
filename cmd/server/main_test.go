package main

import (
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	appconfig "github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/handler"
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

func TestShouldStartForwardAgentBridgeWorkerDisabledByDefault(t *testing.T) {
	if shouldStartForwardAgentBridgeWorker(nil) {
		t.Fatal("nil config should not start forward agent bridge worker")
	}
	if shouldStartForwardAgentBridgeWorker(&appconfig.Config{}) {
		t.Fatal("legacy bridge worker should be opt-in so clean_agent direct pull owns pending jobs by default")
	}
}

func TestShouldStartForwardAgentBridgeWorkerWhenLegacyBridgeEnabled(t *testing.T) {
	cfg := &appconfig.Config{}
	cfg.ForwardRuntime.CleanAgent.LegacyBridgeEnabled = true
	if !shouldStartForwardAgentBridgeWorker(cfg) {
		t.Fatal("legacy bridge worker should start when clean_agent legacy_bridge_enabled is true")
	}
}

func TestStartGRPCServerDisabled(t *testing.T) {
	server, addr, err := startGRPCServer(&appconfig.Config{})
	if err != nil {
		t.Fatalf("startGRPCServer() error = %v", err)
	}
	if server != nil || addr != "" {
		t.Fatalf("disabled gRPC returned server=%v addr=%q", server, addr)
	}
}

func TestStartGRPCServerRejectsOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	port := listener.Addr().(*net.TCPAddr).Port
	cfg := &appconfig.Config{}
	cfg.GRPC.Enable = true
	cfg.GRPC.Host = "127.0.0.1"
	cfg.GRPC.Port = port

	server, addr, err := startGRPCServer(cfg)
	if err == nil {
		if server != nil {
			server.Stop()
		}
		t.Fatal("startGRPCServer() accepted an occupied port")
	}
	if server != nil || addr != "" {
		t.Fatalf("failed startup returned server=%v addr=%q", server, addr)
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("startGRPCServer() error = %v, want EADDRINUSE", err)
	}
}

func TestNewControlPluginHostManagerRejectsInvalidTimeout(t *testing.T) {
	cfg := &appconfig.Config{}
	cfg.Plugins.ControlHostRuntimeDir = t.TempDir()
	cfg.Plugins.ControlHostStartupTimeout = "not-a-duration"

	manager, err := newControlPluginHostManager(cfg)

	if err == nil {
		t.Fatalf("newControlPluginHostManager() = %v, nil error", manager)
	}
}

func TestNewControlPluginHostManagerRejectsInvalidWebSocketSessionTimeout(t *testing.T) {
	cfg := &appconfig.Config{}
	cfg.Plugins.ControlHostRuntimeDir = t.TempDir()
	cfg.Plugins.ControlHostWebSocketSessionTimeout = "not-a-duration"

	manager, err := newControlPluginHostManager(cfg)

	if err == nil {
		t.Fatalf("newControlPluginHostManager() = %v, nil error", manager)
	}
}

func TestNewControlPluginArtifactResolverValidatesTrustRootAndCreatesCleanup(t *testing.T) {
	_, cleanup, err := newControlPluginArtifactResolver(&appconfig.Config{})
	if err == nil {
		t.Fatal("newControlPluginArtifactResolver accepted an empty trust root")
	}
	if cleanup != nil {
		t.Fatal("invalid resolver returned cleanup")
	}

	cfg := &appconfig.Config{}
	cfg.Plugins.OfficialPublicKey = base64.StdEncoding.EncodeToString(make([]byte, 32))
	resolver, cleanup, err := newControlPluginArtifactResolver(cfg)
	if err != nil {
		t.Fatalf("newControlPluginArtifactResolver() error = %v", err)
	}
	if resolver == nil || cleanup == nil {
		t.Fatal("valid resolver must provide resolver and cleanup")
	}
	cleanup()
}

func TestCreateDefaultIndexWritesFile(t *testing.T) {
	dir := t.TempDir()
	if err := createDefaultIndex(dir); err != nil {
		t.Fatalf("createDefaultIndex() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !strings.Contains(string(content), "AnixOps Control") {
		t.Fatalf("index.html does not contain AnixOps Control marker")
	}

	info, err := os.Stat(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("stat index.html: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("index.html permissions = %v, want %v", got, want)
	}
}

func TestFrontendAPIProxyTargetFollowsSpecificBindAddress(t *testing.T) {
	cases := []struct {
		name string
		host string
		want string
	}{
		{name: "loopback", host: "127.0.0.1", want: "http://127.0.0.1:19080"},
		{name: "specific IPv4", host: "10.100.0.130", want: "http://10.100.0.130:19080"},
		{name: "all IPv4", host: "0.0.0.0", want: "http://127.0.0.1:19080"},
		{name: "all IPv6", host: "::", want: "http://127.0.0.1:19080"},
		{name: "specific IPv6", host: "2001:db8::10", want: "http://[2001:db8::10]:19080"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &appconfig.Config{}
			cfg.Server.Host = tc.host
			cfg.Server.Port = 19080
			if got := frontendAPIProxyTarget(cfg); got != tc.want {
				t.Fatalf("frontendAPIProxyTarget() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMainBuildInfoLdflagsPopulateHandlerBuildInfo(t *testing.T) {
	if os.Getenv("V2BOARD_BUILDINFO_LDFLAGS_CHILD") == "1" {
		syncBuildInfo()
		if handler.BuildVersion != "9.8.7 #456" {
			t.Fatalf("handler.BuildVersion = %q, want %q", handler.BuildVersion, "9.8.7 #456")
		}
		if handler.BuildTime != "2026-07-08T00:00:00Z" {
			t.Fatalf("handler.BuildTime = %q", handler.BuildTime)
		}
		if handler.BuildCode != "456" {
			t.Fatalf("handler.BuildCode = %q", handler.BuildCode)
		}
		if handler.BuildCommit != "abcdef123456" {
			t.Fatalf("handler.BuildCommit = %q", handler.BuildCommit)
		}
		return
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		goBin = "/usr/local/go/bin/go"
		if _, statErr := os.Stat(goBin); statErr != nil {
			t.Fatalf("go binary not found: %v", err)
		}
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(
		goBin,
		"test",
		"./cmd/server",
		"-run", "^TestMainBuildInfoLdflagsPopulateHandlerBuildInfo$",
		"-count=1",
		"-ldflags", "-X github.com/AnixOps/anix-control/v4/cmd/server.version=9.8.7 -X github.com/AnixOps/anix-control/v4/cmd/server.buildTime=2026-07-08T00:00:00Z -X github.com/AnixOps/anix-control/v4/cmd/server.buildCode=456 -X github.com/AnixOps/anix-control/v4/cmd/server.commit=abcdef123456",
	)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "V2BOARD_BUILDINFO_LDFLAGS_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child go test failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestReleaseBuildCommandsTargetMainBuildInfoVariables(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".github/workflows/ci.yml", "Dockerfile", "config/deploy/deploy_panel.sh"} {
		content, err := os.ReadFile(filepath.Join(repoRoot, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		for _, expected := range []string{"-X main.version=", "-X main.buildTime=", "-X main.buildCode=", "-X main.commit="} {
			if !strings.Contains(text, expected) {
				t.Fatalf("%s does not set %s in release build ldflags", path, expected)
			}
		}
		if strings.Contains(text, "internal/handler.BuildVersion") || strings.Contains(text, "internal/handler.BuildTime") {
			t.Fatalf("%s still targets handler build info variables that are overwritten by syncBuildInfo", path)
		}
	}
}
