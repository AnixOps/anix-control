package v2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayDispatcherStub struct {
	input  pluginhost.DispatchInput
	output pluginhost.DispatchOutput
	err    error
}

func (s *gatewayDispatcherStub) Dispatch(_ context.Context, input pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	s.input = input
	return s.output, s.err
}

func TestGatewayUsesPackageDataEnvelopeAndPropagatesRequestIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dispatcher := &gatewayDispatcherStub{output: pluginhost.DispatchOutput{
		StatusCode: http.StatusOK,
		Body:       []byte(`{"items":[]}`),
	}}
	gateway := Gateway{
		Registry: NewRegistry(registrySourceStub{route: Route{
			Method:       http.MethodGet,
			LegacyPath:   "/api/v2/user/knowledge",
			PackageID:    "knowledge",
			Version:      "4.0.0",
			Generation:   7,
			PackageRoute: "knowledge.article.list",
			Envelope:     EnvelopeData,
		}}),
		Dispatcher: dispatcher,
		Timeout:    time.Second,
	}

	router := gin.New()
	router.GET("/api/v2/user/knowledge", func(c *gin.Context) {
		c.Set("user_id", uint(42))
		c.Set("is_admin", false)
		c.Set("request_id", "request-42")
		gateway.Serve(c)
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v2/user/knowledge?tag=stable&tag=v4", nil)
	request.Header.Set("Idempotency-Key", "idem-42")
	request.Header.Set("User-Agent", "AnixOps-Test/1.0")
	request.RemoteAddr = "198.51.100.42:443"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"data":{"items":[]}}`, recorder.Body.String())
	require.Equal(t, "knowledge.article.list", dispatcher.input.RouteID)
	require.Equal(t, "request-42", dispatcher.input.RequestID)
	require.Equal(t, "idem-42", dispatcher.input.IdempotencyKey)
	require.Equal(t, "knowledge", dispatcher.input.PackageID)
	require.JSONEq(t, `{"actor_id":42,"admin":false,"package_id":"knowledge"}`, string(dispatcher.input.PrincipalJSON))
	require.Equal(t, "/api/v2/user/knowledge", dispatcher.input.Metadata.Path)
	require.Equal(t, []string{"stable", "v4"}, dispatcher.input.Metadata.Query["tag"])
	require.Equal(t, "198.51.100.42", dispatcher.input.Metadata.ClientIP)
	require.Equal(t, "AnixOps-Test/1.0", dispatcher.input.Metadata.UserAgent)
}

func TestRequestMetadataRemovesAgentWebSocketCredentialsAfterKernelPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var metadata pluginhost.RequestMetadata
	router.GET("/api/v2/agent/ws", func(c *gin.Context) {
		c.Set("anixops.agent_ws.trusted", true)
		c.Set("anixops.agent_ws.forward_node", true)
		c.Set("node_id", uint(17))
		metadata = requestMetadata(c)
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/agent/ws?node_id=17&api_key=secret-token&token=second-secret&tag=stable", nil))

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Equal(t, []string{"stable"}, metadata.Query["tag"])
	require.NotContains(t, metadata.Query, "node_id")
	require.NotContains(t, metadata.Query, "api_key")
	require.NotContains(t, metadata.Query, "token")
	encoded, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.JSONEq(t, `{"path":"/api/v2/agent/ws","query":{"tag":["stable"]},"client_ip":"192.0.2.1","node_id":17,"trusted_agent_websocket_auth":true,"trusted_agent_websocket_forward_node":true}`, string(encoded))
}

func TestGatewayFailsClosedWhenPackageIsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := Gateway{Registry: NewRegistry(registrySourceStub{err: ErrPackageUnavailable})}
	router := gin.New()
	router.GET("/api/v2/user/knowledge", gateway.Serve)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/user/knowledge", nil))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "legacy")
}
