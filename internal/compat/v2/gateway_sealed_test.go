package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	configtables "github.com/AnixOps/anix-control/v5/config"
	"github.com/AnixOps/anix-control/v5/internal/pluginhost"
	"github.com/AnixOps/anix-control/v5/internal/sealedsecrets"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sealingHost stands for a package host: it records what it was sent and
// answers with answer(input), which may mint handles as an executor does.
type sealingHost struct {
	inputs []pluginhost.DispatchInput
	answer func(pluginhost.DispatchInput) pluginhost.DispatchOutput
	legacy []pluginhost.DispatchInput
	// legacyErr, when set, is the kernel's legacy dispatch failure.
	legacyErr error
}

func (h *sealingHost) Dispatch(_ context.Context, input pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	h.inputs = append(h.inputs, input)
	return h.answer(input), nil
}

// legacySealingHost also serves the kernel's legacy dispatch.
type legacySealingHost struct{ *sealingHost }

func (h legacySealingHost) DispatchLegacy(_ context.Context, input pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	if h.legacyErr != nil {
		return pluginhost.DispatchOutput{}, h.legacyErr
	}
	h.legacy = append(h.legacy, input)
	return pluginhost.DispatchOutput{StatusCode: http.StatusOK, Body: []byte(`{"code":0,"data":{"legacy":true},"msg":"ok","ts":1}`)}, nil
}

type sealedGateway struct {
	t       *testing.T
	store   *sealedsecrets.Store
	metrics *GatewayMetrics
	router  *gin.Engine
}

func newSealedGateway(t *testing.T, route Route, dispatcher Dispatcher) *sealedGateway {
	t.Helper()
	gin.SetMode(gin.TestMode)
	list := strings.TrimRight(string(configtables.NodeSecretFields), " \n")
	end := strings.LastIndex(list, "]")
	require.Positive(t, end)
	table, err := sealedsecrets.ParseTable([]byte(strings.TrimRight(list[:end], " \n") + answeringRoute))
	require.NoError(t, err)
	store := sealedsecrets.NewStore(nil)
	metrics := NewGatewayMetrics()
	gateway := Gateway{
		Registry: NewRegistry(patternSourceStub{route: route}), Dispatcher: dispatcher, Metrics: metrics,
		Sealer: sealedsecrets.NewSealer(table, nil, store),
	}
	router := gin.New()
	router.Handle(route.Method, route.LegacyPath, gateway.Serve)
	return &sealedGateway{t: t, store: store, metrics: metrics, router: router}
}

func (g *sealedGateway) do(method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("X-Request-ID", "req-1")
	g.router.ServeHTTP(recorder, request)
	return recorder
}

func bindingOf(input pluginhost.DispatchInput) sealedsecrets.Binding {
	return sealedsecrets.Binding{Key: input.SealedRequest, PackageID: input.PackageID, Generation: input.Generation, RequestID: input.RequestID, RouteID: input.RouteID}
}

// answeringRoute is a field list row the tests add to the embedded list: a
// proxy node creation whose answer shows a generated credential (no listed
// route answers one today).
const answeringRoute = `,
    {"route_id": "fixture.proxy.nodes.post", "target": {"kind": "proxy", "new": true}, "request": [{"pointer": "/raw_config", "kind": "document"}], "answer": [{"pointer": "/data/credential", "name": "credential"}]}
  ]
}`

var createNodeRoute = Route{
	Method: http.MethodPost, LegacyPath: "/api/v2/admin/nodes", PackageID: "proxy-node", Version: "4.1.0",
	Generation: 5, PackageRoute: "proxy.admin.nodes.post", Envelope: EnvelopePanel,
}

// The host reads the body with its secrets sealed, the capability keeps the
// original for the legacy handler, and the handles the kernel minted for
// the answer are expanded on the way to the client. The request's handles
// die with it.
func TestGatewaySealsRequestsAndExpandsTheirAnswers(t *testing.T) {
	var minted []string
	host := &sealingHost{}
	route := createNodeRoute
	route.PackageRoute = "fixture.proxy.nodes.post"
	gateway := newSealedGateway(t, route, host)
	host.answer = func(input pluginhost.DispatchInput) pluginhost.DispatchOutput {
		credential, _, err := gateway.store.Mint(bindingOf(input), "credential", "generated-credential")
		require.NoError(t, err)
		minted = append(minted, credential)
		return pluginhost.DispatchOutput{StatusCode: http.StatusOK, Body: []byte(`{"code":0,"data":{"credential":"` + credential + `","node_id":3},"msg":"ok","ts":1}`)}
	}
	body := `{"name":"edge","raw_config":{"private_key":"typed-private-key","public_key":"pub"}}`
	recorder := gateway.do(http.MethodPost, "/api/v2/admin/nodes", body)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, `{"code":0,"data":{"credential":"generated-credential","node_id":3},"msg":"ok","ts":1}`, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), sealedsecrets.Prefix, "a handle never reaches a client")

	require.Len(t, host.inputs, 1)
	input := host.inputs[0]
	assert.NotContains(t, string(input.Body), "typed-private-key", "a secret never reaches the package")
	assert.Contains(t, string(input.Body), `"public_key":"pub"`)
	assert.Equal(t, body, string(input.BridgeBody), "the legacy handler reads the request as sent")
	assert.NotEmpty(t, input.SealedRequest)
	assert.Zero(t, gateway.store.Pending(), "the request's handles die with it")
	for _, handle := range minted {
		assert.False(t, gateway.store.Live(handle))
	}
	rendered := renderGatewayMetrics(t, gateway.metrics)
	assert.Contains(t, rendered, `anixops_v2_gateway_sealed_secrets_total{package="proxy-node",route="fixture.proxy.nodes.post",stage="request",result="sealed",reason="none"} 1`)
	assert.Contains(t, rendered, `anixops_v2_gateway_sealed_secrets_total{package="proxy-node",route="fixture.proxy.nodes.post",stage="answer",result="expanded",reason="none"} 1`)
	assert.NotContains(t, rendered, sealedsecrets.Prefix, "handles never become label values")
	assert.NotContains(t, rendered, "typed-private-key")
}

// A request whose secrets cannot be sealed is served by the kernel's legacy
// handler without the package host, with a metric; without one it is
// refused. Either way nothing reaches the package.
func TestGatewayFailsClosedToLegacy(t *testing.T) {
	route := createNodeRoute
	// A route whose target is a path parameter: a proxy node.
	byID := Route{
		Method: http.MethodPut, LegacyPath: "/api/v2/admin/nodes/:id", PackageID: "proxy-node", Version: "4.1.0",
		Generation: 5, PackageRoute: "proxy.admin.nodes.id.put", Envelope: EnvelopePanel,
	}
	answer := func(pluginhost.DispatchInput) pluginhost.DispatchOutput {
		return pluginhost.DispatchOutput{StatusCode: http.StatusOK, Body: []byte(`{"code":0,"data":null,"msg":"ok","ts":1}`)}
	}
	for name, body := range map[string]string{
		"not json": `raw_config=typed-private-key`,
	} {
		t.Run(name, func(t *testing.T) {
			host := &sealingHost{answer: answer}
			gateway := newSealedGateway(t, route, legacySealingHost{host})
			recorder := gateway.do(http.MethodPost, "/api/v2/admin/nodes", body)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), `"legacy":true`)
			assert.Empty(t, host.inputs, "the package host never saw the request")
			require.Len(t, host.legacy, 1)
			assert.Equal(t, body, string(host.legacy[0].Body), "the legacy handler reads the request as sent")
			assert.Empty(t, host.legacy[0].SealedRequest)
			assert.Contains(t, renderGatewayMetrics(t, gateway.metrics), `stage="request",result="legacy_fallback",reason=`)
			assert.Zero(t, gateway.store.Pending())
		})
	}

	t.Run("a target that is not an id", func(t *testing.T) {
		host := &sealingHost{answer: answer}
		gateway := newSealedGateway(t, byID, legacySealingHost{host})
		recorder := gateway.do(http.MethodPut, "/api/v2/admin/nodes/x7", `{"raw_config":{"private_key":"typed-key"}}`)
		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Empty(t, host.inputs)
		assert.Contains(t, renderGatewayMetrics(t, gateway.metrics), `result="legacy_fallback",reason="target_invalid"`)
	})

	t.Run("no legacy dispatcher", func(t *testing.T) {
		host := &sealingHost{answer: answer}
		gateway := newSealedGateway(t, route, host)
		recorder := gateway.do(http.MethodPost, "/api/v2/admin/nodes", `{"raw_config":`)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		assert.Contains(t, recorder.Body.String(), codeSealedSecretUnavailable)
		assert.Empty(t, host.inputs)
		assert.Contains(t, renderGatewayMetrics(t, gateway.metrics), `stage="request",result="refused",reason="not_json"`)
	})

	t.Run("no legacy handler for the route", func(t *testing.T) {
		host := &sealingHost{answer: answer, legacyErr: pluginhost.ErrLegacyUnavailable}
		gateway := newSealedGateway(t, route, legacySealingHost{host})
		recorder := gateway.do(http.MethodPost, "/api/v2/admin/nodes", `{"raw_config":`)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		assert.Contains(t, recorder.Body.String(), codeSealedSecretUnavailable)
		assert.NotContains(t, recorder.Body.String(), "raw_config")
		assert.Empty(t, host.inputs)
	})
}

// An answer that would show a handle the kernel cannot expand there is
// refused, and the handle never reaches the client.
func TestGatewayRefusesAnswersWithHandlesItCannotShow(t *testing.T) {
	host := &sealingHost{}
	gateway := newSealedGateway(t, createNodeRoute, legacySealingHost{host})
	host.answer = func(input pluginhost.DispatchInput) pluginhost.DispatchOutput {
		// The package echoes the handle it was sent for the typed key.
		start := strings.Index(string(input.Body), sealedsecrets.Prefix)
		handle := string(input.Body)[start : start+len(sealedsecrets.Prefix)+43]
		return pluginhost.DispatchOutput{StatusCode: http.StatusOK, Body: []byte(`{"code":0,"data":{"credential":"` + handle + `"},"msg":"ok","ts":1}`)}
	}
	recorder := gateway.do(http.MethodPost, "/api/v2/admin/nodes", `{"raw_config":{"private_key":"typed-private-key"}}`)
	require.Equal(t, http.StatusBadGateway, recorder.Code)
	assert.Contains(t, recorder.Body.String(), codeSealedSecretRefused)
	assert.NotContains(t, recorder.Body.String(), sealedsecrets.Prefix)
	assert.NotContains(t, recorder.Body.String(), "typed-private-key")
	assert.Empty(t, host.legacy, "the native handler may have acted: the request is not served again")
	assert.Contains(t, renderGatewayMetrics(t, gateway.metrics), `stage="answer",result="refused",reason="answer_handle"`)
	assert.Zero(t, gateway.store.Pending())
}

// An unlisted route's body reaches the host as sent.
func TestGatewayLeavesUnlistedRoutesAlone(t *testing.T) {
	route := Route{
		Method: http.MethodPost, LegacyPath: "/api/v2/user/ticket", PackageID: "ticket", Version: "4.0.0",
		Generation: 7, PackageRoute: "ticket.user.ticket.post", Envelope: EnvelopeData,
	}
	host := &sealingHost{answer: func(pluginhost.DispatchInput) pluginhost.DispatchOutput {
		return pluginhost.DispatchOutput{StatusCode: http.StatusOK, Body: []byte(`{"ok":true}`)}
	}}
	gateway := newSealedGateway(t, route, host)
	recorder := gateway.do(http.MethodPost, "/api/v2/user/ticket", `{"password":"not a node secret"}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, host.inputs, 1)
	assert.Equal(t, `{"password":"not a node secret"}`, string(host.inputs[0].Body))
	assert.Nil(t, host.inputs[0].BridgeBody)
	assert.Empty(t, host.inputs[0].SealedRequest)
	assert.NotContains(t, renderGatewayMetrics(t, gateway.metrics), "anixops_v2_gateway_sealed_secrets_total{")
}
