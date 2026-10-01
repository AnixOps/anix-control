package main

import (
	"crypto/ed25519"
	"crypto/rand"
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
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/requestorigin"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
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

func TestSelectConfigPathFallsBackToBuiltInDefaults(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(explicit, []byte("env: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing.yaml")

	if path, err := selectConfigPath(explicit, true, ""); err != nil || path != explicit {
		t.Fatalf("explicit -config = %q, %v", path, err)
	}
	if _, err := selectConfigPath(missing, true, ""); err == nil {
		t.Fatal("a missing explicit -config must fail")
	}
	if path, err := selectConfigPath(defaultConfigPath, false, explicit); err != nil || path != explicit {
		t.Fatalf("ANIX_CONTROL_CONFIG = %q, %v", path, err)
	}
	if _, err := selectConfigPath(defaultConfigPath, false, missing); err == nil {
		t.Fatal("a missing ANIX_CONTROL_CONFIG must fail")
	}
	// The test runs in cmd/server, which has no config/config.yaml.
	if path, err := selectConfigPath(defaultConfigPath, false, ""); err != nil || path != "" {
		t.Fatalf("no config = %q, %v; want built-in defaults", path, err)
	}
}

func TestWriteEnvTableListsVariablesWithoutSecretValues(t *testing.T) {
	var out strings.Builder
	if err := writeEnvTable(&out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, expected := range []string{"ANIX_CONTROL_DATABASE_PASSWORD", "(secret)", "ANIX_CONTROL_SERVER_PORT", "8080", "_FILE"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("print-env output lacks %q", expected)
		}
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

func TestEnsureConfiguredPluginTrustRootRejectsInvalidConfiguration(t *testing.T) {
	if err := ensureConfiguredPluginTrustRoot(nil, ""); err == nil {
		t.Fatal("ensureConfiguredPluginTrustRoot accepted an empty configured root")
	}
	if err := ensureConfiguredPluginTrustRoot(nil, "not-base64"); err == nil {
		t.Fatal("ensureConfiguredPluginTrustRoot accepted a malformed configured root")
	}
}

func TestEnsureConfiguredPluginTrustRootActivatesOnlyConfiguredRoot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureKernelSchema(db); err != nil {
		t.Fatal(err)
	}
	firstPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secondPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureConfiguredPluginTrustRoot(db, base64.StdEncoding.EncodeToString(firstPublicKey)); err != nil {
		t.Fatal(err)
	}
	if err := ensureConfiguredPluginTrustRoot(db, base64.StdEncoding.EncodeToString(secondPublicKey)); err != nil {
		t.Fatal(err)
	}

	var activeRoots []model.PluginTrustRoot
	if err := db.Where("active = ?", true).Find(&activeRoots).Error; err != nil {
		t.Fatal(err)
	}
	if len(activeRoots) != 1 {
		t.Fatalf("active roots = %d, want 1", len(activeRoots))
	}
	if activeRoots[0].Fingerprint != service.PluginTrustRootFingerprint(secondPublicKey) {
		t.Fatalf("active root fingerprint = %q", activeRoots[0].Fingerprint)
	}
}

// clientIPFor serves one request on an engine configured with proxies and
// returns what Context.ClientIP saw.
func clientIPFor(t *testing.T, proxies []string, peer string, headers map[string]string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previous := requestorigin.Default()
	t.Cleanup(func() { requestorigin.SetDefault(previous) })
	r := gin.New()
	if err := applyTrustedProxies(r, proxies); err != nil {
		t.Fatalf("applyTrustedProxies() error = %v", err)
	}
	r.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = peer
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Body.String()
}

// The default (server.trusted_proxies unset) trusts loopback only: no other
// peer can set its address with X-Forwarded-For or X-Real-IP, which feed
// rate limits, login throttling and audit logs.
func TestApplyTrustedProxiesDefaultsToLoopbackOnly(t *testing.T) {
	spoof := map[string]string{"X-Forwarded-For": "198.51.100.8", "X-Real-IP": "198.51.100.9"}
	for _, peer := range []string{"203.0.113.10:12345", "10.0.0.7:12345", "192.168.1.2:12345", "172.17.0.1:12345"} {
		want, _, _ := net.SplitHostPort(peer)
		if got := clientIPFor(t, nil, peer, spoof); got != want {
			t.Fatalf("peer %s: ClientIP() = %q, want the peer", peer, got)
		}
	}
	if got := clientIPFor(t, nil, "127.0.0.1:12345", spoof); got != "198.51.100.8" {
		t.Fatalf("loopback proxy: ClientIP() = %q, want X-Forwarded-For", got)
	}
	if got := clientIPFor(t, nil, "[::1]:12345", spoof); got != "198.51.100.8" {
		t.Fatalf("IPv6 loopback proxy: ClientIP() = %q, want X-Forwarded-For", got)
	}
}

func TestApplyTrustedProxiesEmptyListTrustsNone(t *testing.T) {
	if got := clientIPFor(t, []string{}, "127.0.0.1:12345", map[string]string{"X-Forwarded-For": "198.51.100.8"}); got != "127.0.0.1" {
		t.Fatalf("ClientIP() = %q, want the peer when no proxy is trusted", got)
	}
}

func TestApplyTrustedProxiesHonorsConfiguredProxyChain(t *testing.T) {
	if got := clientIPFor(t, []string{"10.0.0.0/8"}, "10.1.2.3:12345", map[string]string{"X-Forwarded-For": "198.51.100.8"}); got != "198.51.100.8" {
		t.Fatalf("ClientIP() = %q, want %q when proxy is trusted", got, "198.51.100.8")
	}
	if got := clientIPFor(t, []string{"10.0.0.0/8"}, "127.0.0.1:12345", map[string]string{"X-Forwarded-For": "198.51.100.8"}); got != "127.0.0.1" {
		t.Fatalf("ClientIP() = %q: a configured list replaces the loopback default", got)
	}
}

func TestApplyTrustedProxiesSetsTheRequestOriginPolicy(t *testing.T) {
	previous := requestorigin.Default()
	t.Cleanup(func() { requestorigin.SetDefault(previous) })
	if err := applyTrustedProxies(gin.New(), []string{"10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	if got := requestorigin.Default().CIDRs(); len(got) != 1 || got[0] != "10.0.0.0/8" {
		t.Fatalf("request origin policy = %v", got)
	}
	if err := applyTrustedProxies(gin.New(), []string{"not-an-ip"}); err == nil {
		t.Fatal("an invalid trusted proxy was accepted")
	}
}

// The UI server proxies /api to the API listener, which trusts it as a
// loopback peer: it must pass on only the origin it resolved, never a
// client's forwarding headers.
func TestProxyAPIReplacesClientForwardingHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := requestorigin.Default()
	t.Cleanup(func() { requestorigin.SetDefault(previous) })
	defaultPolicy, err := requestorigin.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	requestorigin.SetDefault(defaultPolicy)

	var seen http.Header
	var seenHost, seenPeer string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, seenHost, seenPeer = r.Header.Clone(), r.Host, r.RemoteAddr
	}))
	defer upstream.Close()

	r := gin.New()
	r.Any("/api/*path", func(c *gin.Context) { proxyAPI(c, upstream.URL) })
	// A real server (the reverse proxy needs a CloseNotifier) that lets the
	// test choose the peer address the UI server sees.
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.RemoteAddr = req.Header.Get("X-Test-Peer")
		req.Header.Del("X-Test-Peer")
		r.ServeHTTP(w, req)
	}))
	defer front.Close()
	send := func(peer string) {
		req, err := http.NewRequest(http.MethodGet, front.URL+"/api/v2/forward-agent/install.sh", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "panel.example.test"
		req.Header.Set("X-Test-Peer", peer)
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Forwarded-Host", "evil.example")
		req.Header.Set("X-Forwarded-For", "198.51.100.66")
		req.Header.Set("X-Real-IP", "198.51.100.67")
		req.Header.Set("Forwarded", "host=evil.example")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}

	send("203.0.113.9:40000")
	if got := seen.Get("X-Forwarded-Proto"); got != "http" {
		t.Fatalf("X-Forwarded-Proto = %q, want http", got)
	}
	if got := seen.Get("X-Forwarded-Host"); got != "panel.example.test" || seenHost != "panel.example.test" {
		t.Fatalf("X-Forwarded-Host = %q, Host = %q, want panel.example.test", got, seenHost)
	}
	if got := seen.Get("X-Forwarded-For"); got != "203.0.113.9" {
		t.Fatalf("X-Forwarded-For = %q, want only the real peer", got)
	}
	if seen.Get("X-Real-IP") != "" || seen.Get("Forwarded") != "" {
		t.Fatalf("client forwarding headers passed on: %v", seen)
	}
	if !strings.HasPrefix(seenPeer, "127.0.0.1:") {
		t.Fatalf("upstream peer = %q", seenPeer)
	}

	send("127.0.0.1:40000")
	if seen.Get("X-Forwarded-Proto") != "https" || seen.Get("X-Forwarded-Host") != "evil.example" ||
		seen.Get("X-Forwarded-For") != "198.51.100.66, 127.0.0.1" {
		t.Fatalf("a trusted proxy's headers were not kept: %v", seen)
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
