package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	appconfig "github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/handler"
	"github.com/AnixOps/anix-control/v5/internal/requestorigin"
	"github.com/gin-gonic/gin"
)

const frontendTestIndex = "<!doctype html><title>spa-index</title>"

// frontendFixture starts the frontend listener (newFrontendServer) in front of
// api. It listens on 127.0.0.1 like the real one and lets a test choose the
// peer address the listener sees with the X-Test-Peer header. The returned
// config is the one the listener was built from.
func frontendFixture(t *testing.T, trusted []string, api http.Handler) (*httptest.Server, *appconfig.Config) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previous := requestorigin.Default()
	t.Cleanup(func() { requestorigin.SetDefault(previous) })

	apiServer := httptest.NewServer(api)
	t.Cleanup(apiServer.Close)
	apiURL, err := url.Parse(apiServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(apiURL.Port())
	if err != nil {
		t.Fatal(err)
	}

	webRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte(frontendTestIndex), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &appconfig.Config{}
	cfg.Server.Host = apiURL.Hostname()
	cfg.Server.Port = port
	cfg.Server.TrustedProxies = trusted
	cfg.Frontend.Enable = true
	cfg.Frontend.Path = webRoot

	srv, err := newFrontendServer(cfg)
	if err != nil {
		t.Fatalf("newFrontendServer() error = %v", err)
	}
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if peer := req.Header.Get("X-Test-Peer"); peer != "" {
			req.RemoteAddr = peer
			req.Header.Del("X-Test-Peer")
		}
		srv.Handler.ServeHTTP(w, req)
	}))
	t.Cleanup(front.Close)
	return front, cfg
}

func frontendGet(t *testing.T, base, path string, headers map[string]string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body)
}

// The documented reverse proxies send the whole host to the frontend
// listener. The install script, its signature, the agent metadata, the
// caller's address and the Agent release assets are API routes: they must
// reach the API listener, and only unknown paths get the SPA.
func TestFrontendListenerProxiesInstallRoutesToAPI(t *testing.T) {
	var seen []string
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		if strings.Contains(r.URL.Path, "/missing/") {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, "api:"+r.URL.Path)
	})
	front, _ := frontendFixture(t, nil, api)

	for _, path := range []string{
		"/install.sh",
		"/install.sh.sig",
		"/install/agent.env",
		"/install/ip",
		"/install/agent/v4.2.0/anix-agent-linux-64.zip",
	} {
		status, body := frontendGet(t, front.URL, path, nil)
		if status != http.StatusOK || body != "api:"+path {
			t.Errorf("GET %s = %d %q, want the API answer", path, status, body)
		}
		if strings.Contains(body, "spa-index") {
			t.Errorf("GET %s was answered with index.html", path)
		}
	}
	if len(seen) != 5 {
		t.Fatalf("the API saw %v, want the five install paths", seen)
	}

	// The API's own errors reach the client: a missing release asset is a
	// 404 from the API, not a 200 with HTML.
	status, body := frontendGet(t, front.URL, "/install/agent/missing/anix-agent-linux-64.zip", nil)
	if status != http.StatusNotFound || !strings.Contains(body, "not_found") {
		t.Errorf("a missing asset = %d %q, want the API's 404", status, body)
	}

	// Everything else keeps the SPA fallback and never reaches the API.
	before := len(seen)
	for _, path := range []string{"/", "/admin/login", "/install", "/install/", "/install/unknown", "/install.shx", "/dashboard/nodes"} {
		status, body := frontendGet(t, front.URL, path, nil)
		if status != http.StatusOK || body != frontendTestIndex {
			t.Errorf("GET %s = %d %q, want the SPA index", path, status, body)
		}
	}
	if len(seen) != before {
		t.Errorf("SPA paths reached the API: %v", seen[before:])
	}
}

// The API listener must see the real client behind the frontend listener:
// /install/ip hands the address to the join, and the public rate limit and
// login throttling key on it.
func TestFrontendListenerKeepsTheClientAddressForInstallRoutes(t *testing.T) {
	spoof := map[string]string{
		"X-Forwarded-For":   "198.51.100.66",
		"X-Real-IP":         "198.51.100.67",
		"X-Forwarded-Host":  "evil.example",
		"X-Forwarded-Proto": "https",
	}
	with := func(base map[string]string, extra map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range base {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	tests := []struct {
		name    string
		trusted []string
		peer    string
		headers map[string]string
		want    string
	}{
		{"direct client, spoofed headers ignored", nil, "203.0.113.9:40000", spoof, "203.0.113.9"},
		{"direct IPv6 client", nil, "[2001:db8::9]:40000", nil, "2001:db8::9"},
		{"loopback proxy, X-Forwarded-For", nil, "127.0.0.1:40000", map[string]string{"X-Forwarded-For": "198.51.100.7"}, "198.51.100.7"},
		{"loopback proxy, appended chain", nil, "127.0.0.1:40000", map[string]string{"X-Forwarded-For": "9.9.9.9, 198.51.100.7"}, "198.51.100.7"},
		{"loopback proxy, X-Real-IP only", nil, "127.0.0.1:40000", map[string]string{"X-Real-IP": "198.51.100.8"}, "198.51.100.8"},
		{"loopback proxy, IPv6 client", nil, "127.0.0.1:40000", map[string]string{"X-Forwarded-For": "2001:db8::7"}, "2001:db8::7"},
		{"loopback proxy without headers", nil, "127.0.0.1:40000", nil, "127.0.0.1"},
		{"listed proxy on another host", []string{"127.0.0.1/32", "10.0.5.20"}, "10.0.5.20:40000", map[string]string{"X-Forwarded-For": "198.51.100.7"}, "198.51.100.7"},
		// A list without loopback, as an operator with only a remote proxy
		// writes it, and an empty list: the hop from the frontend listener
		// to the API listener is Control's own and stays trusted.
		{"list without loopback, remote proxy", []string{"10.0.5.20"}, "10.0.5.20:40000", map[string]string{"X-Forwarded-For": "198.51.100.7"}, "198.51.100.7"},
		{"list without loopback, direct client", []string{"10.0.5.20"}, "203.0.113.9:40000", spoof, "203.0.113.9"},
		{"empty list, direct client", []string{}, "203.0.113.9:40000", spoof, "203.0.113.9"},
		{"untrusted peer chain", []string{"10.0.5.20"}, "203.0.113.9:40000", with(spoof, map[string]string{"X-Forwarded-For": "10.0.5.20"}), "203.0.113.9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var apiPeer string
			var apiEngine *gin.Engine
			api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				apiPeer = r.RemoteAddr
				apiEngine.ServeHTTP(w, r)
			})
			front, cfg := frontendFixture(t, tc.trusted, api)

			// The API listener as newAPIServer builds it, with the one route
			// under test: the same trust list, the real handler.
			apiEngine = gin.New()
			if err := applyTrustedProxies(apiEngine, serverTrustedProxies(cfg)); err != nil {
				t.Fatal(err)
			}
			apiEngine.GET("/install/ip", handler.NewAgentInstallHandler().ClientIP)

			headers := with(tc.headers, map[string]string{"X-Test-Peer": tc.peer})
			status, body := frontendGet(t, front.URL, "/install/ip", headers)
			if status != http.StatusOK {
				t.Fatalf("GET /install/ip = %d %q", status, body)
			}
			if got := strings.TrimSpace(body); got != tc.want {
				t.Fatalf("/install/ip = %q, want %q (the API listener's peer was %s)", got, tc.want, apiPeer)
			}
		})
	}
}

func TestServerTrustedProxiesAddsTheFrontendHop(t *testing.T) {
	cfgFor := func(enable bool, host string, trusted []string) *appconfig.Config {
		cfg := &appconfig.Config{}
		cfg.Server.Host = host
		cfg.Server.Port = 8080
		cfg.Server.TrustedProxies = trusted
		cfg.Frontend.Enable = enable
		return cfg
	}
	tests := []struct {
		name string
		cfg  *appconfig.Config
		want []string
	}{
		{"frontend disabled keeps the list", cfgFor(false, "0.0.0.0", []string{"10.0.5.20"}), []string{"10.0.5.20"}},
		{"frontend disabled keeps an empty list", cfgFor(false, "0.0.0.0", []string{}), []string{}},
		{"default list already covers loopback", cfgFor(true, "0.0.0.0", nil), nil},
		{"explicit loopback list is unchanged", cfgFor(true, "", []string{"127.0.0.1/32", "::1/128"}), []string{"127.0.0.1/32", "::1/128"}},
		{"a wider network that holds loopback is unchanged", cfgFor(true, "::", []string{"127.0.0.0/8"}), []string{"127.0.0.0/8"}},
		{"list without loopback gains the hop", cfgFor(true, "0.0.0.0", []string{"10.0.5.20"}), []string{"10.0.5.20", "127.0.0.1"}},
		{"empty list trusts only the hop", cfgFor(true, "0.0.0.0", []string{}), []string{"127.0.0.1"}},
		{"specific bind address is the hop", cfgFor(true, "10.0.0.5", nil), []string{"127.0.0.1/32", "::1/128", "10.0.0.5"}},
		{"specific bind address joins a list", cfgFor(true, "10.0.0.5", []string{"10.0.5.20"}), []string{"10.0.5.20", "10.0.0.5"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := serverTrustedProxies(tc.cfg)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("serverTrustedProxies() = %#v, want %#v", got, tc.want)
			}
			// The result is a valid list.
			if _, err := requestorigin.NewPolicy(got); err != nil {
				t.Fatalf("NewPolicy(%v) error = %v", got, err)
			}
		})
	}
}

func TestFrontendHopSourcesFollowTheProxyTarget(t *testing.T) {
	cfg := &appconfig.Config{}
	cfg.Server.Port = 8080
	for host, want := range map[string][]string{
		"":            {"127.0.0.1"},
		"0.0.0.0":     {"127.0.0.1"},
		"[::]":        {"127.0.0.1"},
		"127.0.0.1":   {"127.0.0.1"},
		"10.0.0.5":    {"10.0.0.5"},
		"2001:db8::5": {"2001:db8::5"},
	} {
		cfg.Server.Host = host
		got := frontendHopSources(cfg)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("host %q: frontendHopSources() = %v, want %v", host, got, want)
		}
	}
	// A host name resolves to its addresses.
	cfg.Server.Host = "localhost"
	for _, address := range frontendHopSources(cfg) {
		if ip := net.ParseIP(address); ip == nil || !ip.IsLoopback() {
			t.Errorf("localhost resolved to %q, want loopback addresses", address)
		}
	}
}
