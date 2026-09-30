package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/stretchr/testify/assert"
)

// fakeHostStatsManager is a pluginhost.Manager that only reports stats.
type fakeHostStatsManager struct {
	stats []pluginhost.HostStats
}

func (*fakeHostStatsManager) Start(context.Context, pluginhost.ArtifactRef, uint64) error {
	return pluginhost.ErrHostUnavailable
}

func (*fakeHostStatsManager) Dispatch(context.Context, pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	return pluginhost.DispatchOutput{}, pluginhost.ErrHostUnavailable
}

func (*fakeHostStatsManager) Health(context.Context, string, string, uint64) (pluginhost.HostHealth, error) {
	return pluginhost.HostHealth{}, pluginhost.ErrHostUnavailable
}

func (*fakeHostStatsManager) Drain(context.Context, string, string, uint64, time.Time) error {
	return pluginhost.ErrHostUnavailable
}

func (*fakeHostStatsManager) Stop(context.Context, string, string, uint64) error {
	return pluginhost.ErrHostUnavailable
}

func (*fakeHostStatsManager) Rollback(context.Context, string, string, uint64) error {
	return pluginhost.ErrHostUnavailable
}

func (m *fakeHostStatsManager) Stats() []pluginhost.HostStats { return m.stats }

func (s *MetricsHandlerTestSuite) TestWritePluginHostMetrics() {
	var body strings.Builder
	writePluginHostMetrics(&body, []pluginhost.HostStats{
		{PackageID: "knowledge", State: pluginhost.HostStateRunning, Starts: 3, UnexpectedExits: 2, Restarts: 2},
		{PackageID: "ticket", State: pluginhost.HostStateFailed, Starts: 6, UnexpectedExits: 6, Restarts: 5, Failures: 1},
	})
	assert.Equal(s.T(), `# HELP anixops_plugin_host_starts_total Package host processes that became healthy.
# TYPE anixops_plugin_host_starts_total counter
anixops_plugin_host_starts_total{package="knowledge"} 3
anixops_plugin_host_starts_total{package="ticket"} 6
# HELP anixops_plugin_host_unexpected_exits_total Package host exits not caused by a lifecycle operation.
# TYPE anixops_plugin_host_unexpected_exits_total counter
anixops_plugin_host_unexpected_exits_total{package="knowledge"} 2
anixops_plugin_host_unexpected_exits_total{package="ticket"} 6
# HELP anixops_plugin_host_restarts_total Watchdog restart attempts of package hosts.
# TYPE anixops_plugin_host_restarts_total counter
anixops_plugin_host_restarts_total{package="knowledge"} 2
anixops_plugin_host_restarts_total{package="ticket"} 5
# HELP anixops_plugin_host_failures_total Times a package host exhausted its restart budget.
# TYPE anixops_plugin_host_failures_total counter
anixops_plugin_host_failures_total{package="knowledge"} 0
anixops_plugin_host_failures_total{package="ticket"} 1
# HELP anixops_plugin_host_state Current package host supervision state (1 for the current state).
# TYPE anixops_plugin_host_state gauge
anixops_plugin_host_state{package="knowledge",state="running"} 1
anixops_plugin_host_state{package="knowledge",state="restarting"} 0
anixops_plugin_host_state{package="knowledge",state="failed"} 0
anixops_plugin_host_state{package="knowledge",state="exited"} 0
anixops_plugin_host_state{package="knowledge",state="stopped"} 0
anixops_plugin_host_state{package="ticket",state="running"} 0
anixops_plugin_host_state{package="ticket",state="restarting"} 0
anixops_plugin_host_state{package="ticket",state="failed"} 1
anixops_plugin_host_state{package="ticket",state="exited"} 0
anixops_plugin_host_state{package="ticket",state="stopped"} 0
`, body.String())
	assert.Equal(s.T(), `a\"b\\c\n`, prometheusLabelValue("a\"b\\c\n"))
}

func (s *MetricsHandlerTestSuite) TestGetMetrics_IncludesPluginHostStatsWhenSupervised() {
	previous := pluginhost.DefaultManager()
	s.T().Cleanup(func() { pluginhost.SetDefaultManager(previous) })
	handler := NewMetricsHandler()
	s.router.GET("/metrics", handler.GetMetrics)
	get := func() string {
		recorder := httptest.NewRecorder()
		s.router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		assert.Equal(s.T(), http.StatusOK, recorder.Code)
		return recorder.Body.String()
	}

	pluginhost.SetDefaultManager(nil)
	assert.NotContains(s.T(), get(), "anixops_plugin_host_", "no supervisor, no host series")

	pluginhost.SetDefaultManager(&fakeHostStatsManager{stats: []pluginhost.HostStats{
		{PackageID: "knowledge", State: pluginhost.HostStateRestarting, Starts: 1, UnexpectedExits: 1, Restarts: 1},
	}})
	body := get()
	assert.Contains(s.T(), body, `anixops_plugin_host_restarts_total{package="knowledge"} 1`)
	assert.Contains(s.T(), body, `anixops_plugin_host_state{package="knowledge",state="restarting"} 1`)
}

func (s *MetricsHandlerTestSuite) TestWritePackageRouteMetrics() {
	var body strings.Builder
	writePackageRouteMetrics(&body, []pluginhost.HostStats{
		{PackageID: "ticket"},
		{PackageID: "broken", HealthDetailsJSON: `not json`},
		{PackageID: "knowledge", HealthDetailsJSON: `{"bridge":"required","config":{"status":"ok","revision":4},"routes":{
			"knowledge.article.list":{"mode":"shadow","effective":"shadow","shadow_total":9,"shadow_mismatch":2,"shadow_errors":1,"shadow_skipped":3},
			"knowledge.article.detail":{"mode":"native","effective":"legacy","mode_unsupported":true}}}`},
		{PackageID: "identity-platform", HealthDetailsJSON: `{"bridge":"required","config":{"status":"unsupported"}}`},
	})
	assert.Equal(s.T(), `# HELP anixops_package_config_status Route-mode configuration status reported by each package host (1 for the current status).
# TYPE anixops_package_config_status gauge
anixops_package_config_status{package="identity-platform",status="unsupported"} 1
anixops_package_config_status{package="knowledge",status="ok"} 1
# HELP anixops_package_route_mode Configured and effective mode of package routes that are not legacy or have native traffic.
# TYPE anixops_package_route_mode gauge
anixops_package_route_mode{package="knowledge",route="knowledge.article.detail",mode="native",effective="legacy"} 1
anixops_package_route_mode{package="knowledge",route="knowledge.article.list",mode="shadow",effective="shadow"} 1
# HELP anixops_package_native_requests_total Requests answered by a package-native route.
# TYPE anixops_package_native_requests_total counter
anixops_package_native_requests_total{package="knowledge",route="knowledge.article.detail"} 0
anixops_package_native_requests_total{package="knowledge",route="knowledge.article.list"} 0
# HELP anixops_package_native_errors_total Package-native route requests that failed.
# TYPE anixops_package_native_errors_total counter
anixops_package_native_errors_total{package="knowledge",route="knowledge.article.detail"} 0
anixops_package_native_errors_total{package="knowledge",route="knowledge.article.list"} 0
# HELP anixops_package_shadow_requests_total Completed shadow comparisons of package-native routes.
# TYPE anixops_package_shadow_requests_total counter
anixops_package_shadow_requests_total{package="knowledge",route="knowledge.article.detail"} 0
anixops_package_shadow_requests_total{package="knowledge",route="knowledge.article.list"} 9
# HELP anixops_package_shadow_mismatches_total Shadow comparisons whose native answer differed from legacy.
# TYPE anixops_package_shadow_mismatches_total counter
anixops_package_shadow_mismatches_total{package="knowledge",route="knowledge.article.detail"} 0
anixops_package_shadow_mismatches_total{package="knowledge",route="knowledge.article.list"} 2
# HELP anixops_package_shadow_errors_total Shadow runs whose native implementation failed.
# TYPE anixops_package_shadow_errors_total counter
anixops_package_shadow_errors_total{package="knowledge",route="knowledge.article.detail"} 0
anixops_package_shadow_errors_total{package="knowledge",route="knowledge.article.list"} 1
# HELP anixops_package_shadow_skipped_total Shadow runs skipped because all shadow slots were busy.
# TYPE anixops_package_shadow_skipped_total counter
anixops_package_shadow_skipped_total{package="knowledge",route="knowledge.article.detail"} 0
anixops_package_shadow_skipped_total{package="knowledge",route="knowledge.article.list"} 3
`, body.String())

	body.Reset()
	writePackageRouteMetrics(&body, []pluginhost.HostStats{{PackageID: "ticket"}})
	assert.Empty(s.T(), body.String(), "hosts without health details export no route series")
}
