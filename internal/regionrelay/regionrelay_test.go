package regionrelay

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/proxyproto"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestParseConfigFillsDefaults(t *testing.T) {
	config, err := ParseConfig([]byte(`
forwards:
  - listen: ":50051"
    upstreams: ["panel.example.com:50051"]
    proxy_protocol: true
cache:
  listen: ":8443"
  tls_cert_file: /etc/anix-relay/cache.pem
  tls_key_file: /etc/anix-relay/cache.key
  dir: /var/cache/anix-relay
  origins: ["https://github.com/AnixOps/"]
`))
	if err != nil {
		t.Fatal(err)
	}
	forward := config.Forwards[0]
	if forward.Name != "forward-1" || forward.MaxConnections != DefaultMaxConnections || forward.DialTimeout != DefaultDialTimeout || forward.KeepAlive != DefaultKeepAlive || !forward.ProxyProtocol {
		t.Fatalf("forward = %+v", forward)
	}
	if config.Cache.MaxBytes != DefaultCacheMaxBytes || config.Cache.MaxFetches != DefaultCacheFetches {
		t.Fatalf("cache = %+v", config.Cache)
	}
}

func TestParseConfigRefuses(t *testing.T) {
	cache := "cache:\n  listen: \":8443\"\n  tls_cert_file: /c\n  tls_key_file: /k\n  dir: /var/cache/r\n"
	cases := map[string]string{
		"empty":            "{}",
		"unknown key":      "forwards: []\napi: true\n",
		"no upstream":      "forwards:\n  - listen: \":50051\"\n",
		"bad upstream":     "forwards:\n  - listen: \":50051\"\n    upstreams: [\"panel\"]\n",
		"bad listen":       "forwards:\n  - listen: \"50051\"\n    upstreams: [\"p:1\"]\n",
		"same listen":      "forwards:\n  - listen: \":1\"\n    upstreams: [\"p:1\"]\n  - listen: \":1\"\n    upstreams: [\"p:1\"]\n",
		"same name":        "forwards:\n  - name: a\n    listen: \":1\"\n    upstreams: [\"p:1\"]\n  - name: a\n    listen: \":2\"\n    upstreams: [\"p:1\"]\n",
		"cache no origins": cache,
		"cache http":       cache + "  origins: [\"http://github.com/\"]\n",
		"cache no slash":   cache + "  origins: [\"https://github.com/AnixOps\"]\n",
		"cache relative":   strings.Replace(cache, "/var/cache/r", "cache", 1) + "  origins: [\"https://github.com/\"]\n",
		"cache no tls":     "cache:\n  listen: \":8443\"\n  dir: /var/cache/r\n  origins: [\"https://github.com/\"]\n",
		"cache object":     cache + "  origins: [\"https://github.com/\"]\n  max_bytes: 10\n  max_object_bytes: 11\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConfig([]byte(text)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

// upstream accepts connections and records what each one sent first.
type upstream struct {
	listener net.Listener
	accepted atomic.Int32
}

func newUpstream(t *testing.T, handle func(net.Conn)) *upstream {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	u := &upstream{listener: listener}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			u.accepted.Add(1)
			go handle(conn)
		}
	}()
	return u
}

func startForward(t *testing.T, config ForwardConfig) *Forward {
	t.Helper()
	if config.Listen == "" {
		config.Listen = "127.0.0.1:0"
	}
	if config.DialTimeout == 0 {
		config.DialTimeout = time.Second
	}
	if config.KeepAlive == 0 {
		config.KeepAlive = time.Second
	}
	if config.MaxConnections == 0 {
		config.MaxConnections = 10
	}
	forward, err := ListenForward(config, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = forward.Serve() }()
	t.Cleanup(func() { _ = forward.Close() })
	return forward
}

func TestForwardPassesBytesWithTheProxyHeader(t *testing.T) {
	headers := make(chan proxyproto.Header, 1)
	up := newUpstream(t, func(conn net.Conn) {
		defer func() { _ = conn.Close() }()
		reader := bufio.NewReader(conn)
		header, err := proxyproto.Read(reader)
		if err != nil {
			return
		}
		headers <- header
		_, _ = io.Copy(conn, reader) // echo
	})
	forward := startForward(t, ForwardConfig{Name: "control", Upstreams: []string{up.listener.Addr().String()}, ProxyProtocol: true})

	node, err := net.Dial("tcp", forward.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = node.Close() }()
	if _, err := node.Write([]byte("tls bytes")); err != nil {
		t.Fatal(err)
	}
	echo := make([]byte, len("tls bytes"))
	if _, err := io.ReadFull(node, echo); err != nil || string(echo) != "tls bytes" {
		t.Fatalf("echo = %q, %v", echo, err)
	}
	header := <-headers
	if header.Source.String() != node.LocalAddr().String() {
		t.Fatalf("header source = %s, want the node %s", header.Source, node.LocalAddr())
	}
}

func TestForwardWithoutProxyProtocolSendsTheStreamOnly(t *testing.T) {
	first := make(chan []byte, 1)
	up := newUpstream(t, func(conn net.Conn) {
		defer func() { _ = conn.Close() }()
		buffer := make([]byte, 5)
		_, _ = io.ReadFull(conn, buffer)
		first <- buffer
	})
	forward := startForward(t, ForwardConfig{Upstreams: []string{up.listener.Addr().String()}})
	node, err := net.Dial("tcp", forward.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = node.Close() }()
	_, _ = node.Write([]byte("hello"))
	if got := <-first; string(got) != "hello" {
		t.Fatalf("upstream got %q", got)
	}
}

func TestForwardFailsOverAndStaysOnTheWorkingUpstream(t *testing.T) {
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadAddress := dead.Addr().String()
	_ = dead.Close()
	up := newUpstream(t, func(conn net.Conn) {
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(conn, conn)
	})
	forward := startForward(t, ForwardConfig{Upstreams: []string{deadAddress, up.listener.Addr().String()}})
	for i := 0; i < 2; i++ {
		node, err := net.Dial("tcp", forward.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		_, _ = node.Write([]byte("x"))
		buffer := make([]byte, 1)
		if _, err := io.ReadFull(node, buffer); err != nil {
			t.Fatalf("connection %d: %v", i, err)
		}
		_ = node.Close()
	}
	if got := forward.preferred.Load(); got != 1 {
		t.Fatalf("preferred upstream = %d, want 1", got)
	}
	if got := up.accepted.Load(); got != 2 {
		t.Fatalf("working upstream accepted %d connections", got)
	}
}

func TestForwardRefusesOverMaxConnections(t *testing.T) {
	up := newUpstream(t, func(conn net.Conn) {
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(io.Discard, conn)
	})
	forward := startForward(t, ForwardConfig{Upstreams: []string{up.listener.Addr().String()}, MaxConnections: 1})
	first, err := net.Dial("tcp", forward.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	if _, err := first.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for up.accepted.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	second, err := net.Dial("tcp", forward.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	_ = second.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := second.Read(make([]byte, 1)); err == nil {
		t.Fatal("the connection over max_connections was served")
	}
}

// A connection that sends nothing (the Agent timing the relay) never
// opens one to the upstream, whether it closes or stays silent.
func TestForwardDialsTheUpstreamOnlyAfterTheNodeSpeaks(t *testing.T) {
	saved := firstBytesTimeout
	firstBytesTimeout = 200 * time.Millisecond
	t.Cleanup(func() { firstBytesTimeout = saved })
	up := newUpstream(t, func(conn net.Conn) {
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(conn, conn)
	})
	forward := startForward(t, ForwardConfig{Upstreams: []string{up.listener.Addr().String()}})
	closed, err := net.Dial("tcp", forward.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	silent, err := net.Dial("tcp", forward.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = silent.Close() }()
	_ = silent.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := silent.Read(make([]byte, 1)); err == nil {
		t.Fatal("a silent node connection was served")
	}
	if got := up.accepted.Load(); got != 0 {
		t.Fatalf("the upstream accepted %d connections from nodes that sent nothing", got)
	}
}

func TestForwardCloseEndsOpenConnections(t *testing.T) {
	up := newUpstream(t, func(conn net.Conn) {
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(io.Discard, conn)
	})
	config := ForwardConfig{Listen: "127.0.0.1:0", Upstreams: []string{up.listener.Addr().String()}, DialTimeout: time.Second, KeepAlive: time.Second, MaxConnections: 10}
	forward, err := ListenForward(config, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = forward.Serve() }()
	node, err := net.Dial("tcp", forward.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = node.Close() }()
	if _, err := node.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for up.accepted.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	done := make(chan struct{})
	go func() { _ = forward.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return")
	}
	_ = node.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := node.Read(make([]byte, 1)); err == nil {
		t.Fatal("the node connection stayed open")
	}
}

// testCache returns a cache that downloads from origin's test server.
func testCache(t *testing.T, origin *httptest.Server, maxBytes int64) *Cache {
	t.Helper()
	config := CacheConfig{
		Dir:                 t.TempDir(),
		Origins:             []string{origin.URL + "/releases/"},
		AllowPrivateOrigins: true,
		MaxBytes:            maxBytes,
		MaxObjectBytes:      maxBytes,
		MaxFetches:          2,
		FetchTimeout:        10 * time.Second,
	}
	cache, err := NewCache(config, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	cache.client = origin.Client()
	return cache
}

func sumOf(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

func cacheGet(cache *Cache, sum, source string) *httptest.ResponseRecorder {
	target := CachePathPrefix + sum
	if source != "" {
		target += "?" + CacheSourceParam + "=" + url.QueryEscape(source)
	}
	recorder := httptest.NewRecorder()
	cache.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func TestCacheDownloadsOnceAndServesByHash(t *testing.T) {
	var hits atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, "release-bytes")
	}))
	defer origin.Close()
	cache := testCache(t, origin, 1<<20)
	sum := sumOf("release-bytes")
	source := origin.URL + "/releases/anix-agent.zip"

	for i := 0; i < 3; i++ {
		recorder := cacheGet(cache, sum, source)
		if recorder.Code != http.StatusOK || recorder.Body.String() != "release-bytes" {
			t.Fatalf("request %d: %d %q", i, recorder.Code, recorder.Body.String())
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("origin downloaded %d times", hits.Load())
	}
	// A cached file needs no source.
	if recorder := cacheGet(cache, sum, ""); recorder.Code != http.StatusOK {
		t.Fatalf("cached file without src: %d", recorder.Code)
	}
}

func TestCacheRefusesAWrongHashAndKeepsNothing(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "tampered")
	}))
	defer origin.Close()
	cache := testCache(t, origin, 1<<20)
	sum := sumOf("release-bytes")
	if recorder := cacheGet(cache, sum, origin.URL+"/releases/a.zip"); recorder.Code != http.StatusBadGateway {
		t.Fatalf("code = %d", recorder.Code)
	}
	if _, err := os.Stat(cache.path(sum)); err == nil {
		t.Fatal("a file with the wrong hash was kept")
	}
	entries, _ := os.ReadDir(filepath.Join(cache.config.Dir, "tmp"))
	if len(entries) != 0 {
		t.Fatalf("partial downloads left: %d", len(entries))
	}
}

func TestCacheRefusesSourcesOutsideTheOrigins(t *testing.T) {
	var hits atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer origin.Close()
	cache := testCache(t, origin, 1<<20)
	sum := sumOf("x")
	for _, source := range []string{
		origin.URL + "/other/a.zip",
		origin.URL + "/releases/../other/a.zip",
		strings.Replace(origin.URL, "https://", "http://", 1) + "/releases/a.zip",
		"https://user:pass@" + strings.TrimPrefix(origin.URL, "https://") + "/releases/a.zip",
		"https://example.invalid/releases/a.zip",
	} {
		if recorder := cacheGet(cache, sum, source); recorder.Code != http.StatusForbidden {
			t.Fatalf("%s: code %d", source, recorder.Code)
		}
	}
	if hits.Load() != 0 {
		t.Fatal("the origin was contacted")
	}
}

func TestCacheServesNothingElse(t *testing.T) {
	origin := httptest.NewTLSServer(http.NotFoundHandler())
	defer origin.Close()
	cache := testCache(t, origin, 1<<20)
	for _, target := range []string{"/", "/v1/sha256/", "/v1/sha256/abc", "/v1/sha256/" + strings.ToUpper(sumOf("x")), "/api", "/metrics"} {
		recorder := httptest.NewRecorder()
		cache.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", target, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	cache.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, CachePathPrefix+sumOf("x"), strings.NewReader("x")))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT: %d", recorder.Code)
	}
}

func TestCacheRefusesOversizedFiles(t *testing.T) {
	body := strings.Repeat("a", 2048)
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
	defer origin.Close()
	cache := testCache(t, origin, 1024)
	if recorder := cacheGet(cache, sumOf(body), origin.URL+"/releases/big"); recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("code = %d", recorder.Code)
	}
}

func TestCacheEvictsTheLeastRecentlyUsed(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat(strings.TrimPrefix(r.URL.Path, "/releases/"), 400))
	}))
	defer origin.Close()
	cache := testCache(t, origin, 1000)
	clock := time.Unix(1_700_000_000, 0)
	cache.now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	a, b, c := strings.Repeat("a", 400), strings.Repeat("b", 400), strings.Repeat("c", 400)
	for _, name := range []string{"a", "b"} {
		if recorder := cacheGet(cache, sumOf(strings.Repeat(name, 400)), origin.URL+"/releases/"+name); recorder.Code != http.StatusOK {
			t.Fatalf("%s: %d", name, recorder.Code)
		}
	}
	// Use a again, so b is the least recently used.
	if recorder := cacheGet(cache, sumOf(a), ""); recorder.Code != http.StatusOK {
		t.Fatal("a was not cached")
	}
	if recorder := cacheGet(cache, sumOf(c), origin.URL+"/releases/c"); recorder.Code != http.StatusOK {
		t.Fatalf("c: %d", recorder.Code)
	}
	if _, err := os.Stat(cache.path(sumOf(b))); err == nil {
		t.Fatal("b was kept")
	}
	for _, kept := range []string{a, c} {
		if _, err := os.Stat(cache.path(sumOf(kept))); err != nil {
			t.Fatalf("%c was evicted", kept[0])
		}
	}
}

func TestRefusePrivate(t *testing.T) {
	for _, address := range []string{"127.0.0.1:443", "10.0.0.1:443", "192.168.1.1:443", "169.254.169.254:80", "[::1]:443", "[fc00::1]:443", "100.64.0.1:443", "0.0.0.0:443"} {
		if err := refusePrivate(address); err == nil {
			t.Fatalf("%s allowed", address)
		}
	}
	for _, address := range []string{"140.82.112.3:443", "[2606:50c0:8000::153]:443"} {
		if err := refusePrivate(address); err != nil {
			t.Fatalf("%s refused: %v", address, err)
		}
	}
}
