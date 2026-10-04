package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
