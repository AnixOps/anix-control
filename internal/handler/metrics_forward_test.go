package handler

import (
	"net/http"
	"net/http/httptest"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// /metrics carries the forwarding gauges (internal/kernelforward).
func (s *MetricsHandlerTestSuite) TestGetMetrics_IncludesForwardGauges() {
	require.NoError(s.T(), s.db.AutoMigrate(model.KernelForwardModels()...))
	handler := NewMetricsHandler()
	s.router.GET("/metrics", handler.GetMetrics)
	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Equal(s.T(), http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	for _, name := range []string{
		"anixops_forward_nodes", "anixops_forward_lagging_nodes", "anixops_forward_generation_lag_max",
		"anixops_forward_hop_errors", "anixops_forward_plan_refused",
	} {
		assert.Contains(s.T(), body, "# TYPE "+name+" gauge\n")
	}
}
