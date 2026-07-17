package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/middleware"
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

func setupForwardFlowEnvelopeRouter(t *testing.T) *gin.Engine {
	t.Helper()

	initTestDB()
	r := gin.New()
	h := NewForwardHandler()
	r.POST("/forward/flow/report", h.ReportPanelForwardTraffic)
	r.POST("/forward/flow/snapshot", h.SnapshotPanelForwardTraffic)
	return r
}

func TestForwardFlowHandlers_ResponseEnvelope(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		body     string
		wantCode float64
		wantMsg  string
		wantData any
	}{
		{
			name:     "report success",
			path:     "/forward/flow/report",
			body:     `{"records":[]}`,
			wantCode: 0,
			wantMsg:  "操作成功",
			wantData: true,
		},
		{
			name:     "report invalid body",
			path:     "/forward/flow/report",
			body:     `{}`,
			wantCode: -1,
			wantMsg:  "参数错误",
			wantData: nil,
		},
		{
			name:     "report service error",
			path:     "/forward/flow/report",
			body:     `{"records":[{"forwardId":0,"upload":1,"download":0}]}`,
			wantCode: -1,
			wantMsg:  "forwardId is required",
			wantData: nil,
		},
		{
			name:     "snapshot success",
			path:     "/forward/flow/snapshot",
			body:     `{"records":[]}`,
			wantCode: 0,
			wantMsg:  "操作成功",
			wantData: true,
		},
		{
			name:     "snapshot invalid body",
			path:     "/forward/flow/snapshot",
			body:     `{}`,
			wantCode: -1,
			wantMsg:  "参数错误",
			wantData: nil,
		},
		{
			name:     "snapshot service error",
			path:     "/forward/flow/snapshot",
			body:     `{"records":[{"forwardId":0,"backend":"gost","uploadTotal":1,"downloadTotal":0}]}`,
			wantCode: -1,
			wantMsg:  "forwardId is required",
			wantData: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupForwardFlowEnvelopeRouter(t)

			req, err := http.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			resp := decodePanelTestResponse(t, w)
			assert.Equal(t, tt.wantCode, resp["code"])
			assert.Equal(t, tt.wantMsg, resp["msg"])
			assert.Equal(t, tt.wantData, resp["data"])
			assert.NotContains(t, w.Body.String(), "\"error\"")
		})
	}
}
