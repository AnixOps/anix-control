package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/clientreport"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A report on the subscription's address is taken once per window, an
// unknown token is 404, a bad body 400; the summary answers per node.
func TestClientReportAPI(t *testing.T) {
	env := newDiagnosticsEnv(t)
	require.NoError(t, env.db.AutoMigrate(&model.ClientFailureReport{}))
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	handler := &ClientReportHandler{db: func() *gorm.DB { return env.db }, limiter: clientreport.NewLimiter(), now: func() time.Time { return now }}
	router := gin.New()
	router.POST("/s/:token/report", handler.Report)
	router.GET("/client-failures", handler.Summary)

	body := `{"client":"anixops-net/0.3","platform":"ios","network":"cellular","window_secs":600,"future":1,
		"nodes":[{"node":"hk-1","server":"hk.example.com","proto":"anx","ok":3,"reset":1,"after_handshake":1,"stopped":false}]}`
	response := serveJSON(router, http.MethodPost, "/s/nope/report", body)
	require.Equal(t, http.StatusNotFound, response.Code)
	response = serveJSON(router, http.MethodPost, "/s/token-7/report", `{"window_secs":1,"nodes":[]}`)
	require.Equal(t, http.StatusBadRequest, response.Code)
	response = serveJSON(router, http.MethodPost, "/s/token-7/report", `not json`)
	require.Equal(t, http.StatusBadRequest, response.Code)

	response = serveJSON(router, http.MethodPost, "/s/token-7/report", body)
	require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
	response = serveJSON(router, http.MethodPost, "/s/token-7.yaml/report", body)
	require.Equal(t, http.StatusTooManyRequests, response.Code)
	assert.Equal(t, "600", response.Header().Get("Retry-After"))

	response = serveJSON(router, http.MethodGet, "/client-failures?hours=6", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"node":"hk-1"`)
	assert.Contains(t, response.Body.String(), `"after_handshake":1`)
	assert.NotContains(t, response.Body.String(), "token-7")
	response = serveJSON(router, http.MethodGet, "/client-failures?hours=9999", "")
	require.Equal(t, http.StatusBadRequest, response.Code)
}
