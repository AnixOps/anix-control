package regionrelay

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	if forward.Name != "forward-1" || forward.MaxConnections != DefaultMaxConnections || forward.DialTimeout != DefaultDialTimeout || forward.KeepAlive != DefaultKeepAlive || !forward.ProxyProtocol ||
		forward.MaxConnectionsPerSource != DefaultMaxPerSource || forward.HandshakeTimeout != DefaultHandshakeTimeout {
		t.Fatalf("forward = %+v", forward)
	}
	if config.Cache.MaxBytes != DefaultCacheMaxBytes || config.Cache.MaxFetches != DefaultCacheFetches ||
		config.Cache.MaxRequestsPerSource != DefaultCacheRequestsPerSource || config.Cache.MaxConnections != DefaultCacheMaxConnections || len(config.Cache.RedirectHosts) != len(DefaultRedirectHosts) {
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
	waitAccepted(t, up, 1)
	second, err := net.Dial("tcp", forward.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	// The second connection speaks too, so only the cap can stop it.
	_, _ = second.Write([]byte("x"))
	requireClosedByPeer(t, second)
	time.Sleep(100 * time.Millisecond)
	if got := up.accepted.Load(); got != 1 {
		t.Fatalf("the upstream accepted %d connections with max_connections 1", got)
	}
}

func TestForwardRefusesOverMaxConnectionsPerSource(t *testing.T) {
	up := newUpstream(t, func(conn net.Conn) {
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(io.Discard, conn)
	})
	forward := startForward(t, ForwardConfig{Upstreams: []string{up.listener.Addr().String()}, MaxConnections: 10, MaxConnectionsPerSource: 1})
	first, err := net.Dial("tcp", forward.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	if _, err := first.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	waitAccepted(t, up, 1)
	second, err := net.Dial("tcp", forward.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	_, _ = second.Write([]byte("x"))
	requireClosedByPeer(t, second)
	if got := up.accepted.Load(); got != 1 {
		t.Fatalf("the upstream accepted %d connections from one address with max_connections_per_source 1", got)
	}
	// The slot comes back when the first connection ends.
	_ = first.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		third, err := net.Dial("tcp", forward.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		_, _ = third.Write([]byte("x"))
		time.Sleep(50 * time.Millisecond)
		_ = third.Close()
		if up.accepted.Load() == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the address's slot was not released")
		}
	}
}

// An upstream that never answers is not held open past handshake_timeout,
// even while the node keeps quiet too (audit C1).
func TestForwardClosesAnUnansweredHandshake(t *testing.T) {
	up := newUpstream(t, func(conn net.Conn) {
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(io.Discard, conn)
	})
	forward := startForward(t, ForwardConfig{Upstreams: []string{up.listener.Addr().String()}, HandshakeTimeout: 200 * time.Millisecond})
	node, err := net.Dial("tcp", forward.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = node.Close() }()
	if _, err := node.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	requireClosedByPeer(t, node)
}

func waitAccepted(t *testing.T, up *upstream, n int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for up.accepted.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("the upstream accepted %d connections, want %d", up.accepted.Load(), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// requireClosedByPeer reads conn until the relay closes it, and fails when
// it stays open (a read deadline is not a close).
func requireClosedByPeer(t *testing.T, conn net.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err := io.Copy(io.Discard, conn)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatal("the connection stayed open")
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
	requireClosedByPeer(t, node)
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
	for _, address := range []string{"127.0.0.1:443", "10.0.0.1:443", "192.168.1.1:443", "169.254.169.254:80", "[::1]:443", "[fc00::1]:443", "100.64.0.1:443", "0.0.0.0:443",
		"198.18.0.1:443", "192.0.0.8:443", "240.0.0.1:443", "[64:ff9b::a00:1]:443", "[2002:a00:1::1]:443"} {
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

// A source whose SHA-256 the relay has learned is not downloaded again for
// another hash: random hashes with a large allowed source cost one download,
// not one each (audit C2).
func TestCacheRefusesAKnownSourceWithAnotherHash(t *testing.T) {
	var hits atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, "release-bytes")
	}))
	defer origin.Close()
	cache := testCache(t, origin, 1<<20)
	source := origin.URL + "/releases/big.zip"
	if recorder := cacheGet(cache, sumOf("one"), source); recorder.Code != http.StatusBadGateway {
		t.Fatalf("first wrong hash: %d", recorder.Code)
	}
	for _, other := range []string{"two", "three", "four"} {
		if recorder := cacheGet(cache, sumOf(other), source); recorder.Code != http.StatusNotFound {
			t.Fatalf("hash of %q: %d, want 404 without a download", other, recorder.Code)
		}
	}
	if recorder := cacheGet(cache, sumOf("release-bytes"), source); recorder.Code != http.StatusOK {
		t.Fatalf("the right hash: %d", recorder.Code)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("origin downloaded %d times, want 2", got)
	}
}

// Spellings of one source share its learned SHA-256, and a query or a
// fragment cannot make a new source (re-audit N1).
func TestCacheCanonicalizesSources(t *testing.T) {
	var hits atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, "release-bytes")
	}))
	defer origin.Close()
	cache := testCache(t, origin, 1<<20)
	source := origin.URL + "/releases/big.zip"
	for _, refused := range []string{source + "?x=1", source + "?", source + "#frag", source + "/../big.zip"} {
		if recorder := cacheGet(cache, sumOf("one"), refused); recorder.Code != http.StatusForbidden {
			t.Fatalf("%s: code %d", refused, recorder.Code)
		}
	}
	if hits.Load() != 0 {
		t.Fatal("the origin was contacted")
	}
	if recorder := cacheGet(cache, sumOf("one"), source); recorder.Code != http.StatusBadGateway {
		t.Fatalf("first wrong hash: %d", recorder.Code)
	}
	for _, spelling := range []string{
		origin.URL + "/%72eleases/big.zip",
		origin.URL + "/releases/%62ig.zip",
		strings.Replace(origin.URL, "https://", "HTTPS://", 1) + "/releases/big.zip",
	} {
		if recorder := cacheGet(cache, sumOf("two"), spelling); recorder.Code != http.StatusNotFound {
			t.Fatalf("%s: %d, want 404 without a download", spelling, recorder.Code)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("origin downloaded %d times, want 1", got)
	}
}

// A learned SHA-256 expires, so a replaced release can be fetched again
// (re-audit N3).
func TestCacheForgetsAKnownSumAfterItsLifetime(t *testing.T) {
	var hits atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, "release-bytes")
	}))
	defer origin.Close()
	cache := testCache(t, origin, 1<<20)
	clock := time.Unix(1_700_000_000, 0)
	cache.now = func() time.Time { return clock }
	source := origin.URL + "/releases/big.zip"
	if recorder := cacheGet(cache, sumOf("one"), source); recorder.Code != http.StatusBadGateway {
		t.Fatalf("first wrong hash: %d", recorder.Code)
	}
	clock = clock.Add(knownSumLifetime - time.Second)
	if recorder := cacheGet(cache, sumOf("two"), source); recorder.Code != http.StatusNotFound {
		t.Fatalf("inside the lifetime: %d", recorder.Code)
	}
	clock = clock.Add(2 * time.Second)
	if recorder := cacheGet(cache, sumOf("two"), source); recorder.Code != http.StatusBadGateway {
		t.Fatalf("after the lifetime: %d, want a new download", recorder.Code)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("origin downloaded %d times, want 2", got)
	}
}

// Concurrent requests for one file share one download.
func TestCacheDedupesConcurrentDownloads(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release
		_, _ = io.WriteString(w, "release-bytes")
	}))
	defer origin.Close()
	cache := testCache(t, origin, 1<<20)
	sum, source := sumOf("release-bytes"), origin.URL+"/releases/a.zip"
	codes := make(chan int, 5)
	for i := 0; i < 5; i++ {
		go func() { codes <- cacheGet(cache, sum, source).Code }()
	}
	deadline := time.Now().Add(5 * time.Second)
	for hits.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	for i := 0; i < 5; i++ {
		if code := <-codes; code != http.StatusOK {
			t.Fatalf("request answered %d", code)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("origin downloaded %d times for concurrent requests", got)
	}
}

func TestCacheServesOnlyAllowedSources(t *testing.T) {
	config := CacheConfig{Dir: t.TempDir(), Origins: []string{"https://github.com/AnixOps/"}, AllowedSources: []string{"10.0.0.0/8", "203.0.113.7"},
		MaxBytes: 1 << 20, MaxObjectBytes: 1 << 20, MaxFetches: 1, FetchTimeout: time.Second}
	cache, err := NewCache(config, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	for remote, want := range map[string]int{"192.0.2.1:1234": http.StatusForbidden, "10.1.2.3:1234": http.StatusNotFound, "203.0.113.7:1": http.StatusNotFound} {
		request := httptest.NewRequest(http.MethodGet, CachePathPrefix+sumOf("x"), nil)
		request.RemoteAddr = remote
		recorder := httptest.NewRecorder()
		cache.ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Fatalf("%s: %d, want %d", remote, recorder.Code, want)
		}
	}
}

func TestCacheBoundsRequestsPerSource(t *testing.T) {
	config := CacheConfig{Dir: t.TempDir(), Origins: []string{"https://github.com/AnixOps/"}, MaxRequestsPerSource: 2,
		MaxBytes: 1 << 20, MaxObjectBytes: 1 << 20, MaxFetches: 1, FetchTimeout: time.Second}
	cache, err := NewCache(config, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	ip := remoteIP("192.0.2.1:1234")
	for i := 0; i < 2; i++ {
		if !cache.claimRequest(ip) {
			t.Fatalf("request %d refused within the limit", i+1)
		}
	}
	if cache.claimRequest(ip) {
		t.Fatal("a third request from one address was admitted")
	}
	if !cache.claimRequest(remoteIP("192.0.2.2:1")) {
		t.Fatal("another address was refused")
	}
	cache.releaseRequest(ip)
	if !cache.claimRequest(ip) {
		t.Fatal("the released slot did not come back")
	}
}

// Redirects stay https and on the origins' hosts or redirect_hosts.
func TestRedirectPolicy(t *testing.T) {
	policy := redirectPolicy(CacheConfig{Origins: []string{"https://github.com/AnixOps/"}, RedirectHosts: DefaultRedirectHosts})
	for target, ok := range map[string]bool{
		"https://github.com/AnixOps/x":                   true,
		"https://objects.githubusercontent.com/a":        true,
		"https://release-assets.githubusercontent.com/b": true,
		"http://objects.githubusercontent.com/a":         false,
		"https://evil.example/a":                         false,
		"https://user@objects.githubusercontent.com/a":   false,
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		if err := policy(request, nil); (err == nil) != ok {
			t.Fatalf("%s: err = %v, want allowed %v", target, err, ok)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "https://github.com/AnixOps/x", nil)
	if err := policy(request, make([]*http.Request, 5)); err == nil {
		t.Fatal("a sixth redirect was allowed")
	}
}

// One IPv6 host holds a /64: its addresses share one limit (re-audit N6).
func TestSourceGroup(t *testing.T) {
	for address, want := range map[string]string{
		"203.0.113.10:443":            "203.0.113.10",
		"[::ffff:203.0.113.10]:443":   "203.0.113.10",
		"[2001:db8:1:2::1]:443":       "2001:db8:1:2::/64",
		"[2001:db8:1:2:ffff::9]:5000": "2001:db8:1:2::/64",
		"[fe80::1%eth0]:443":          "fe80::/64",
	} {
		if got := sourceGroup(remoteIP(address)); got != want {
			t.Fatalf("%s: %s, want %s", address, got, want)
		}
	}
	addr := &net.TCPAddr{IP: net.ParseIP("2001:db8:1:2::abcd"), Port: 1}
	if got := sourceKey(addr); got != "2001:db8:1:2::/64" {
		t.Fatalf("sourceKey = %s", got)
	}
}

// The cache listener holds at most max_connections open; a closed one
// frees its slot, and Close ends a waiting Accept (re-audit N7).
func TestLimitListener(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := limitListener(inner, 1)
	accepted := make(chan net.Conn, 2)
	acceptErr := make(chan error, 1)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				acceptErr <- err
				return
			}
			accepted <- conn
		}
	}()
	dial := func() net.Conn {
		conn, err := net.Dial("tcp", inner.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	dial()
	first := <-accepted
	dial()
	select {
	case <-accepted:
		t.Fatal("a second connection was accepted over max_connections 1")
	case <-time.After(200 * time.Millisecond):
	}
	_ = first.Close()
	_ = first.Close() // a second Close does not free another slot
	select {
	case second := <-accepted:
		_ = second.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("the closed connection's slot was not freed")
	}
	_ = listener.Close()
	select {
	case err := <-acceptErr:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Accept after Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Accept did not return after Close")
	}
}

func TestCacheWriteTimeoutCoversTheDownload(t *testing.T) {
	if got := cacheWriteTimeout(DefaultCacheFetchTimeout); got <= DefaultCacheFetchTimeout+time.Minute {
		t.Fatalf("write timeout %s does not leave time to transfer after a download", got)
	}
}
