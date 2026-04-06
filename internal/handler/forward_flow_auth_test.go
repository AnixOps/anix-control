package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupForwardFlowAuthRouter(t *testing.T, apiToken string, register func(group *gin.RouterGroup, h *ForwardHandler)) *gin.Engine {
	t.Helper()

	initTestDB()
	config.Set(&config.Config{
		Env: "test",
		App: config.AppConfig{
			APIToken: apiToken,
		},
	})

	r := gin.New()
	group := r.Group("/api/v2/internal")
	group.Use(middleware.AppTokenAuth())
	register(group, NewForwardHandler())
	return r
}

func TestForwardFlowHandlers_AppTokenAuth(t *testing.T) {
	endpoints := []struct {
		name          string
		path          string
		body          string
		successMarker string
		registerRoute func(group *gin.RouterGroup, h *ForwardHandler)
	}{
		{
			name:          "upload panel flow",
			path:          "/api/v2/internal/forward/flow/upload",
			body:          `{"n":"web_api","u":1,"d":2}`,
			successMarker: "ok",
			registerRoute: func(group *gin.RouterGroup, h *ForwardHandler) {
				group.POST("/forward/flow/upload", h.UploadPanelFlowData)
			},
		},
		{
			name:          "report panel forward traffic",
			path:          "/api/v2/internal/forward/flow/report",
			body:          `{}`,
			successMarker: `"code":-1`,
			registerRoute: func(group *gin.RouterGroup, h *ForwardHandler) {
				group.POST("/forward/flow/report", h.ReportPanelForwardTraffic)
			},
		},
		{
			name:          "snapshot panel forward traffic",
			path:          "/api/v2/internal/forward/flow/snapshot",
			body:          `{}`,
			successMarker: `"code":-1`,
			registerRoute: func(group *gin.RouterGroup, h *ForwardHandler) {
				group.POST("/forward/flow/snapshot", h.SnapshotPanelForwardTraffic)
			},
		},
	}

	cases := []struct {
		name          string
		configToken   string
		pathSuffix    string
		headers       map[string]string
		wantStatus    int
		wantSubstring string
	}{
		{
			name:          "token not configured",
			configToken:   "",
			headers:       map[string]string{"X-API-Key": "panel-secret"},
			wantStatus:    http.StatusServiceUnavailable,
			wantSubstring: "internal api token is not configured",
		},
		{
			name:          "missing token",
			configToken:   "panel-secret",
			wantStatus:    http.StatusUnauthorized,
			wantSubstring: "invalid internal api token",
		},
		{
			name:          "wrong x api key",
			configToken:   "panel-secret",
			headers:       map[string]string{"X-API-Key": "wrong-secret"},
			wantStatus:    http.StatusUnauthorized,
			wantSubstring: "invalid internal api token",
		},
		{
			name:        "correct x api key",
			configToken: "panel-secret",
			headers:     map[string]string{"X-API-Key": "panel-secret"},
			wantStatus:  http.StatusOK,
		},
		{
			name:        "correct query secret",
			configToken: "panel-secret",
			pathSuffix:  "?secret=panel-secret",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "correct bearer token",
			configToken: "panel-secret",
			headers:     map[string]string{"Authorization": "Bearer panel-secret"},
			wantStatus:  http.StatusOK,
		},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					router := setupForwardFlowAuthRouter(t, tc.configToken, endpoint.registerRoute)

					req, err := http.NewRequest(http.MethodPost, endpoint.path+tc.pathSuffix, strings.NewReader(endpoint.body))
					require.NoError(t, err)
					req.Header.Set("Content-Type", "application/json")
					for key, value := range tc.headers {
						req.Header.Set(key, value)
					}

					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)

					assert.Equal(t, tc.wantStatus, w.Code)
					if tc.wantSubstring != "" {
						assert.Contains(t, w.Body.String(), tc.wantSubstring)
					} else {
						assert.Contains(t, w.Body.String(), endpoint.successMarker)
					}
				})
			}
		})
	}
}
