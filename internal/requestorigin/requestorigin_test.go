package requestorigin

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func mustPolicy(t *testing.T, proxies []string) *Policy {
	t.Helper()
	policy, err := NewPolicy(proxies)
	if err != nil {
		t.Fatalf("NewPolicy(%v): %v", proxies, err)
	}
	return policy
}

func request(remoteAddr, host string, secure bool, headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = remoteAddr
	r.Host = host
	r.TLS = nil
	if secure {
		r.TLS = &tls.ConnectionState{}
	}
	for name, value := range headers {
		r.Header.Set(name, value)
	}
	return r
}

var forged = map[string]string{
	HeaderForwardedProto: "https",
	HeaderForwardedHost:  "evil.example",
	HeaderForwardedFor:   "198.51.100.66",
	HeaderRealIP:         "198.51.100.67",
}

func TestNewPolicyDefaultsAndValidation(t *testing.T) {
	if got := mustPolicy(t, nil).CIDRs(); len(got) != 2 || got[0] != "127.0.0.1/32" || got[1] != "::1/128" {
		t.Fatalf("default CIDRs = %v, want loopback only", got)
	}
	if got := mustPolicy(t, []string{}).CIDRs(); len(got) != 0 {
		t.Fatalf("empty list CIDRs = %v, want none", got)
	}
	if got := mustPolicy(t, []string{" 10.1.2.3 ", "", "fd00::1", "192.168.0.0/16"}).CIDRs(); len(got) != 3 ||
		got[0] != "10.1.2.3/32" || got[1] != "fd00::1/128" || got[2] != "192.168.0.0/16" {
		t.Fatalf("CIDRs = %v", got)
	}
	for _, bad := range []string{"proxy.example", "10.0.0.0/33", "1.2.3", "*"} {
		if _, err := NewPolicy([]string{bad}); err == nil {
			t.Errorf("NewPolicy(%q) accepted an invalid proxy", bad)
		}
	}
}

func TestResolveIgnoresForwardingHeadersFromUntrustedPeer(t *testing.T) {
	policy := mustPolicy(t, nil)
	for _, peer := range []string{"203.0.113.9:5000", "10.0.0.5:5000", "[2001:db8::1]:5000", "", "garbage"} {
		got := policy.Resolve(request(peer, "panel.example:8443", false, forged))
		if got.Scheme != "http" || got.Host != "panel.example:8443" || got.Forwarded {
			t.Errorf("peer %q: origin = %+v, want the connection's http://panel.example:8443", peer, got)
		}
		if got.BaseURL() != "http://panel.example:8443" {
			t.Errorf("peer %q: BaseURL = %q", peer, got.BaseURL())
		}
	}
	got := policy.Resolve(request("203.0.113.9:5000", "panel.example", true, map[string]string{HeaderForwardedProto: "http"}))
	if got.Scheme != "https" {
		t.Fatalf("TLS connection with a forged http proto: scheme = %q, want https", got.Scheme)
	}
	if ip := policy.Resolve(request("203.0.113.9:5000", "panel.example", false, forged)).ClientIP; ip != "203.0.113.9" {
		t.Fatalf("ClientIP = %q, want the untrusted peer", ip)
	}
}

func TestResolveHonoursTrustedProxy(t *testing.T) {
	policy := mustPolicy(t, []string{"127.0.0.1/32", "::1/128", "10.0.0.0/8"})
	for _, peer := range []string{"127.0.0.1:5000", "[::1]:5000", "10.9.8.7:5000"} {
		got := policy.Resolve(request(peer, "127.0.0.1:8080", false, forged))
		if got.Scheme != "https" || got.Host != "evil.example" || !got.Forwarded {
			t.Errorf("peer %q: origin = %+v, want the proxy's https://evil.example", peer, got)
		}
		if got.ClientIP != "198.51.100.66" {
			t.Errorf("peer %q: ClientIP = %q, want X-Forwarded-For", peer, got.ClientIP)
		}
	}

	// The nearest proxy's (last) value counts; invalid values are ignored.
	got := policy.Resolve(request("127.0.0.1:5000", "panel.example", false, map[string]string{
		HeaderForwardedProto: "http, https", HeaderForwardedHost: "evil.example, panel.example:443",
	}))
	if got.Scheme != "https" || got.Host != "panel.example:443" {
		t.Fatalf("multi-value origin = %+v", got)
	}
	got = policy.Resolve(request("127.0.0.1:5000", "panel.example", false, map[string]string{
		HeaderForwardedProto: "javascript", HeaderForwardedHost: "evil.example/path",
	}))
	if got.Scheme != "http" || got.Host != "panel.example" || got.Forwarded {
		t.Fatalf("invalid forwarded values were used: %+v", got)
	}
}

func TestResolveRejectsInvalidHost(t *testing.T) {
	got := mustPolicy(t, nil).Resolve(request("203.0.113.9:5000", "evil.example/x", false, nil))
	if got.Host != "" || got.BaseURL() != "" {
		t.Fatalf("invalid Host kept: %+v", got)
	}
}

func TestNormalizeHost(t *testing.T) {
	valid := map[string]string{
		"panel.example":      "panel.example",
		"Panel.Example:8443": "panel.example:8443",
		"10.0.0.1":           "10.0.0.1",
		"10.0.0.1:80":        "10.0.0.1:80",
		"[2001:db8::1]":      "[2001:db8::1]",
		"[2001:db8::1]:443":  "[2001:db8::1]:443",
		"localhost":          "localhost",
		"my_host.internal":   "my_host.internal",
	}
	for in, want := range valid {
		if got, ok := NormalizeHost(in); !ok || got != want {
			t.Errorf("NormalizeHost(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	invalid := []string{
		"", "https://evil.example", "evil.example/path", "evil.example?x", "evil.example#x",
		"user@evil.example", "evil example", "evil.example\r\nX-Injected: 1", "evil.example:",
		"evil.example:0", "evil.example:65536", "evil.example:http", "2001:db8::1", "[2001:db8::1",
		"[10.0.0.1]", "[2001:db8::1]x", "a..b", ".evil.example", "-evil.example", "evil.example:80:80",
		"%65vil.example", string(make([]byte, 256)),
	}
	for _, in := range invalid {
		if got, ok := NormalizeHost(in); ok {
			t.Errorf("NormalizeHost(%q) accepted: %q", in, got)
		}
	}
}

// TestClientIPMatchesGin checks the helper against gin's Context.ClientIP on
// an engine configured with ApplyTo, so the address package hosts receive
// and the one rate limits and audit logs use cannot disagree.
func TestClientIPMatchesGin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		peer    string
		headers map[string]string
		want    string
	}{
		{"203.0.113.9:1", map[string]string{HeaderForwardedFor: "198.51.100.1"}, "203.0.113.9"},
		{"203.0.113.9:1", map[string]string{HeaderRealIP: "198.51.100.1"}, "203.0.113.9"},
		{"127.0.0.1:1", map[string]string{HeaderForwardedFor: "198.51.100.1"}, "198.51.100.1"},
		{"127.0.0.1:1", map[string]string{HeaderForwardedFor: "198.51.100.1, 127.0.0.1"}, "198.51.100.1"},
		{"127.0.0.1:1", map[string]string{HeaderForwardedFor: "1.1.1.1, 198.51.100.2"}, "198.51.100.2"},
		{"127.0.0.1:1", map[string]string{HeaderRealIP: "198.51.100.3"}, "198.51.100.3"},
		{"127.0.0.1:1", map[string]string{HeaderForwardedFor: "bogus"}, "127.0.0.1"},
		{"127.0.0.1:1", nil, "127.0.0.1"},
		{"[::1]:1", map[string]string{HeaderForwardedFor: "2001:db8::5"}, "2001:db8::5"},
		{"10.0.0.1:1", map[string]string{HeaderForwardedFor: "198.51.100.1"}, "10.0.0.1"},
	}
	for _, proxies := range [][]string{nil, {}} {
		policy := mustPolicy(t, proxies)
		engine := gin.New()
		if err := policy.ApplyTo(engine); err != nil {
			t.Fatal(err)
		}
		var ginIP string
		engine.GET("/x", func(c *gin.Context) { ginIP = c.ClientIP() })
		for _, tc := range cases {
			r := request(tc.peer, "panel.example", false, tc.headers)
			engine.ServeHTTP(httptest.NewRecorder(), r)
			helperIP := policy.ClientIP(r)
			if helperIP != ginIP {
				t.Errorf("proxies %v peer %s %v: helper %q, gin %q", proxies, tc.peer, tc.headers, helperIP, ginIP)
			}
			if peerHost, _, _ := net.SplitHostPort(tc.peer); proxies != nil && helperIP != peerHost {
				t.Errorf("no trusted proxy, peer %s: ClientIP %q, want the peer", tc.peer, helperIP)
			}
			if proxies == nil && helperIP != tc.want {
				t.Errorf("peer %s %v: ClientIP %q, want %q", tc.peer, tc.headers, helperIP, tc.want)
			}
		}
	}
}

func TestDefaultPolicyAndIsForwardingHeader(t *testing.T) {
	previous := Default()
	t.Cleanup(func() { SetDefault(previous) })
	if !Default().TrustedPeer(request("127.0.0.1:1", "", false, nil)) {
		t.Fatal("the default policy must trust loopback")
	}
	SetDefault(mustPolicy(t, []string{}))
	if got := Resolve(request("127.0.0.1:1", "panel.example", false, forged)); got.Forwarded || got.Scheme != "http" {
		t.Fatalf("an empty trust list still honoured headers: %+v", got)
	}
	for _, name := range []string{"Forwarded", "x-forwarded-for", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Forwarded-Port", "X-Real-IP"} {
		if !IsForwardingHeader(name) {
			t.Errorf("IsForwardingHeader(%q) = false", name)
		}
	}
	for _, name := range []string{"Host", "User-Agent", "X-Request-ID", "X-Forwarded"} {
		if IsForwardingHeader(name) {
			t.Errorf("IsForwardingHeader(%q) = true", name)
		}
	}
}
