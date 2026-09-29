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
