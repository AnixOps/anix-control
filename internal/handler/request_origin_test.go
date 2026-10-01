package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forgedOriginRequest is a request whose forwarding headers point at an
// attacker's host. httptest's default Host is example.com.
func forgedOriginRequest(target, peer string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Host = "panel.example.test"
	request.RemoteAddr = peer
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "evil.example")
	return request
}

func serveForOrigin(t *testing.T, request *http.Request, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/*path", handler)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func withCleanAgentPublicURL(t *testing.T, publicURL string) {
	t.Helper()
	previous := config.Get()
	t.Cleanup(func() { config.Set(previous) })
	cfg := &config.Config{}
	cfg.ForwardRuntime.CleanAgent.PublicURL = publicURL
	config.Set(cfg)
}

func TestCleanAgentInstallScriptIgnoresForgedForwardingHeaders(t *testing.T) {
	withCleanAgentPublicURL(t, "")
	handler := (&ForwardCleanAgentHandler{}).InstallScript

	body := serveForOrigin(t, forgedOriginRequest("/api/v2/forward-agent/install.sh", "203.0.113.9:40000"), handler).Body.String()
	assert.Contains(t, body, `PANEL_URL="${PANEL_URL:-http://panel.example.test}"`)
	assert.NotContains(t, body, "evil.example")

	// A reverse proxy on loopback (the default trusted proxy) is honoured.
	body = serveForOrigin(t, forgedOriginRequest("/api/v2/forward-agent/install.sh", "127.0.0.1:40000"), handler).Body.String()
	assert.Contains(t, body, `PANEL_URL="${PANEL_URL:-https://evil.example}"`)

	// An invalid Host leaves the placeholder rather than a broken URL.
	request := forgedOriginRequest("/api/v2/forward-agent/install.sh", "203.0.113.9:40000")
	request.Host = "panel.example.test/x"
	body = serveForOrigin(t, request, handler).Body.String()
	assert.Contains(t, body, `PANEL_URL="${PANEL_URL:-https://panel.example.com}"`)
}

func TestCleanAgentInstallScriptPrefersConfiguredPublicURL(t *testing.T) {
	withCleanAgentPublicURL(t, "https://control.example.org/")
	handler := (&ForwardCleanAgentHandler{}).InstallScript
	for _, peer := range []string{"203.0.113.9:40000", "127.0.0.1:40000"} {
		body := serveForOrigin(t, forgedOriginRequest("/api/v2/forward-agent/install.sh", peer), handler).Body.String()
		assert.Contains(t, body, `PANEL_URL="${PANEL_URL:-https://control.example.org}"`, peer)
	}
}

func TestSubscribeURLIgnoresForgedForwardingHeaders(t *testing.T) {
	h := &SubscribeHandler{}
	var link, domain string
	capture := func(c *gin.Context) { link, domain = h.buildSubscribeURL(c), h.buildSubscribeDomain(c) }

	serveForOrigin(t, forgedOriginRequest("/s/token?flag=1", "203.0.113.9:40000"), capture)
	assert.Equal(t, "http://panel.example.test/s/token?flag=1", link)
	assert.Equal(t, "panel.example.test", domain)

	serveForOrigin(t, forgedOriginRequest("/s/token", "127.0.0.1:40000"), capture)
	assert.Equal(t, "https://evil.example/s/token", link)

	request := forgedOriginRequest("/s/token", "203.0.113.9:40000")
	request.Host = "bad host"
	serveForOrigin(t, request, capture)
	assert.Empty(t, link, "an invalid host gives no link (the formatter's placeholder)")
}

func (s *SubscribeHandlerTestSuite) TestSubscribeURLPrefersConfiguredDomain() {
	require.NoError(s.T(), service.NewSystemConfigService(s.db).Set(service.SystemConfigKeySubscribeDomains, "sub.example.org", "string", "app", ""))
	h := NewSubscribeHandler(s.cfg)
	var link string
	capture := func(c *gin.Context) { link = h.buildSubscribeURL(c) }
	for _, peer := range []string{"203.0.113.9:40000", "127.0.0.1:40000"} {
		serveForOrigin(s.T(), forgedOriginRequest("/s/token", peer), capture)
		assert.True(s.T(), strings.HasSuffix(link, "://sub.example.org/s/token"), "peer %s: %s", peer, link)
	}
}
