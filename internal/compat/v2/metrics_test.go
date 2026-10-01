package v2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func renderGatewayMetrics(t *testing.T, metrics *GatewayMetrics) string {
	t.Helper()
	var builder strings.Builder
	require.NoError(t, metrics.WritePrometheus(&builder))
	return builder.String()
}

func TestGatewayMetricsRenderPrometheusText(t *testing.T) {
	metrics := NewGatewayMetrics()
	metrics.Observe("knowledge", "knowledge.article.list", http.StatusOK, "", 3*time.Millisecond)
	metrics.Observe("knowledge", "knowledge.article.list", http.StatusOK, "", 200*time.Millisecond)
	metrics.Observe("knowledge", "knowledge.article.list", http.StatusBadGateway, codePluginResponseTooLarge, 20*time.Second)
	metrics.Observe("", "", http.StatusNotFound, codeRouteNotFound, time.Millisecond)
	metrics.ObserveSealed("proxy-node", "proxy.admin.nodes.post", sealedStageRequest, sealedResultLegacy, "not_json")
	metrics.ObserveSealed("proxy-node", "proxy.admin.nodes.post", sealedStageAnswer, sealedResultExpanded, "")

	require.Equal(t, `# HELP anixops_v2_gateway_requests_total Requests handled by the v2 package gateways.
# TYPE anixops_v2_gateway_requests_total counter
anixops_v2_gateway_requests_total{package="knowledge",route="knowledge.article.list",code_class="2xx"} 2
anixops_v2_gateway_requests_total{package="knowledge",route="knowledge.article.list",code_class="5xx"} 1
anixops_v2_gateway_requests_total{package="unresolved",route="unresolved",code_class="4xx"} 1
# HELP anixops_v2_gateway_errors_total Gateway-generated error responses by error code.
# TYPE anixops_v2_gateway_errors_total counter
anixops_v2_gateway_errors_total{package="knowledge",route="knowledge.article.list",code="plugin_response_too_large"} 1
anixops_v2_gateway_errors_total{package="unresolved",route="unresolved",code="package_route_not_found"} 1
# HELP anixops_v2_gateway_request_duration_seconds Time from gateway entry to response, by package.
# TYPE anixops_v2_gateway_request_duration_seconds histogram
anixops_v2_gateway_request_duration_seconds_bucket{package="knowledge",le="0.005"} 1
anixops_v2_gateway_request_duration_seconds_bucket{package="knowledge",le="0.025"} 1
anixops_v2_gateway_request_duration_seconds_bucket{package="knowledge",le="0.1"} 1
anixops_v2_gateway_request_duration_seconds_bucket{package="knowledge",le="0.25"} 2
anixops_v2_gateway_request_duration_seconds_bucket{package="knowledge",le="1"} 2
anixops_v2_gateway_request_duration_seconds_bucket{package="knowledge",le="2.5"} 2
anixops_v2_gateway_request_duration_seconds_bucket{package="knowledge",le="10"} 2
anixops_v2_gateway_request_duration_seconds_bucket{package="knowledge",le="+Inf"} 3
anixops_v2_gateway_request_duration_seconds_sum{package="knowledge"} 20.203
anixops_v2_gateway_request_duration_seconds_count{package="knowledge"} 3
anixops_v2_gateway_request_duration_seconds_bucket{package="unresolved",le="0.005"} 1
anixops_v2_gateway_request_duration_seconds_bucket{package="unresolved",le="0.025"} 1
anixops_v2_gateway_request_duration_seconds_bucket{package="unresolved",le="0.1"} 1
anixops_v2_gateway_request_duration_seconds_bucket{package="unresolved",le="0.25"} 1
anixops_v2_gateway_request_duration_seconds_bucket{package="unresolved",le="1"} 1
anixops_v2_gateway_request_duration_seconds_bucket{package="unresolved",le="2.5"} 1
anixops_v2_gateway_request_duration_seconds_bucket{package="unresolved",le="10"} 1
anixops_v2_gateway_request_duration_seconds_bucket{package="unresolved",le="+Inf"} 1
anixops_v2_gateway_request_duration_seconds_sum{package="unresolved"} 0.001
anixops_v2_gateway_request_duration_seconds_count{package="unresolved"} 1
# HELP anixops_v2_gateway_sealed_secrets_total Node secrets the v2 gateway sealed in requests and expanded in answers, and the requests it served legacy or refused because it could not.
# TYPE anixops_v2_gateway_sealed_secrets_total counter
anixops_v2_gateway_sealed_secrets_total{package="proxy-node",route="proxy.admin.nodes.post",stage="answer",result="expanded",reason="none"} 1
anixops_v2_gateway_sealed_secrets_total{package="proxy-node",route="proxy.admin.nodes.post",stage="request",result="legacy_fallback",reason="not_json"} 1
`, renderGatewayMetrics(t, metrics))
}

func TestGatewayMetricsBoundSeriesAndEscapeLabels(t *testing.T) {
	metrics := NewGatewayMetrics()
	metrics.seriesLimit = 2
	for index := range 5 {
		metrics.Observe(fmt.Sprintf("package-%d", index), "route", http.StatusOK, codePluginHostUnavailable, time.Millisecond)
	}
	metrics.Observe("quote\"back\\slash\nline", "route", http.StatusOK, "", time.Millisecond)
	rendered := renderGatewayMetrics(t, metrics)
	require.Len(t, metrics.requests, 3, "two real series plus one overflow series")
	require.Contains(t, rendered, `anixops_v2_gateway_requests_total{package="_overflow",route="_overflow",code_class="2xx"} 4`)
	require.Contains(t, rendered, `anixops_v2_gateway_errors_total{package="_overflow",route="_overflow",code="plugin_host_unavailable"} 3`)
	require.Contains(t, rendered, `anixops_v2_gateway_request_duration_seconds_count{package="_overflow"} 4`)
	require.Equal(t, `quote\"back\\slash\nline`, escapeLabel("quote\"back\\slash\nline"))
}

func TestGatewayMetricsAreConcurrencySafe(t *testing.T) {
	metrics := NewGatewayMetrics()
	var group sync.WaitGroup
	for worker := range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 100 {
				metrics.Observe("knowledge", fmt.Sprintf("route-%d", worker%2), http.StatusOK, "", time.Millisecond)
				var builder strings.Builder
				_ = metrics.WritePrometheus(&builder)
			}
		}()
	}
	group.Wait()
	require.Equal(t, uint64(800), metrics.latency["knowledge"].count)
}

func TestGatewayRecordsOutcomeMetricsByDeclaredRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	route := Route{
		Method: http.MethodGet, LegacyPath: "/api/v2/user/knowledge/:id", PackageID: "knowledge", Version: "4.0.0",
		Generation: 7, PackageRoute: "knowledge.user.knowledge.id.get", Envelope: EnvelopeData,
	}
	tests := []struct {
		name       string
		dispatcher *gatewayDispatcherStub
		path       string
		wantStatus int
		wantCode   string
	}{
		{name: "success", dispatcher: &gatewayDispatcherStub{output: pluginhost.DispatchOutput{StatusCode: http.StatusOK, Body: []byte(`{}`)}}, path: "/api/v2/user/knowledge/1", wantStatus: http.StatusOK},
		{name: "response too large", dispatcher: &gatewayDispatcherStub{err: fmt.Errorf("%w: %w", pluginhost.ErrHostUnavailable, pluginhost.ErrResponseTooLarge)}, path: "/api/v2/user/knowledge/2", wantStatus: http.StatusBadGateway, wantCode: codePluginResponseTooLarge},
		{name: "host incompatible", dispatcher: &gatewayDispatcherStub{err: pluginhost.ErrHostIncompatible}, path: "/api/v2/user/knowledge/3", wantStatus: http.StatusBadGateway, wantCode: codePluginHostIncompatible},
		{name: "host unavailable", dispatcher: &gatewayDispatcherStub{err: errors.New("down")}, path: "/api/v2/user/knowledge/4", wantStatus: http.StatusBadGateway, wantCode: codePluginHostUnavailable},
		{name: "undeclared", dispatcher: &gatewayDispatcherStub{}, path: "/api/v2/user/other", wantStatus: http.StatusServiceUnavailable, wantCode: codePackageUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metrics := NewGatewayMetrics()
			gateway := Gateway{Registry: NewRegistry(patternSourceStub{route: route}), Dispatcher: test.dispatcher, Metrics: metrics}
			router := gin.New()
			router.Any("/api/v2/*path", gateway.Serve)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			require.Equal(t, test.wantStatus, recorder.Code)
			if test.wantCode != "" {
				require.Contains(t, recorder.Body.String(), `"code":"`+test.wantCode+`"`)
			}

			rendered := renderGatewayMetrics(t, metrics)
			packageID, routeID := "knowledge", "knowledge.user.knowledge.id.get"
			if test.name == "undeclared" {
				packageID, routeID = "unresolved", "unresolved"
			}
			require.Contains(t, rendered, fmt.Sprintf(`anixops_v2_gateway_requests_total{package=%q,route=%q,code_class="%dxx"} 1`, packageID, routeID, test.wantStatus/100))
			require.NotContains(t, rendered, test.path, "raw request paths must never become label values")
			if test.wantCode != "" {
				require.Contains(t, rendered, fmt.Sprintf(`anixops_v2_gateway_errors_total{package=%q,route=%q,code=%q} 1`, packageID, routeID, test.wantCode))
			} else {
				require.NotContains(t, rendered, "anixops_v2_gateway_errors_total{")
			}
		})
	}
}

func TestGatewayRejectsRequestAboveConfiguredLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	route := Route{
		Method: http.MethodPost, LegacyPath: "/api/v2/user/ticket", PackageID: "ticket", Version: "4.0.0",
		Generation: 7, PackageRoute: "ticket.user.ticket.post", Envelope: EnvelopeData,
	}
	dispatcher := &gatewayDispatcherStub{output: pluginhost.DispatchOutput{StatusCode: http.StatusOK, Body: []byte(`{}`)}}
	gateway := Gateway{Registry: NewRegistry(patternSourceStub{route: route}), Dispatcher: dispatcher, RequestBodyLimit: 3 << 20, Metrics: NewGatewayMetrics()}
	router := gin.New()
	router.POST("/api/v2/user/ticket", gateway.Serve)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v2/user/ticket", strings.NewReader(strings.Repeat("a", 2<<20))))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, dispatcher.input.Body, 2<<20)

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v2/user/ticket", strings.NewReader(strings.Repeat("a", 3<<20+1))))
	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.Contains(t, recorder.Body.String(), codePluginRequestTooLarge)
}

// patternSourceStub resolves one declared pattern like the verified source.
type patternSourceStub struct {
	route Route
}

func (s patternSourceStub) ResolveV2Route(_ context.Context, method, path string) (Route, error) {
	if method != s.route.Method || !routeMatches(s.route.LegacyPath, path) {
		return Route{}, ErrPackageUnavailable
	}
	return s.route, nil
}
