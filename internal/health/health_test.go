package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func probe(t *testing.T, state *State, path string) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	state.Register(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return recorder.Code, body
}

func TestProbesFollowTheProcessLifecycle(t *testing.T) {
	state := &State{}
	pingErr := error(nil)
	state.SetDatabasePinger(func(context.Context) error { return pingErr })

	code, body := probe(t, state, "/readyz")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "starting", body["status"])
	code, _ = probe(t, state, "/livez")
	assert.Equal(t, http.StatusOK, code, "live while starting")

	state.MarkStarted()
	code, body = probe(t, state, "/readyz")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]any{"database": "ok"}, body["checks"])

	pingErr = errors.New("connection refused")
	code, body = probe(t, state, "/readyz")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, map[string]any{"database": "unreachable"}, body["checks"], "the error text is not exposed")
	code, _ = probe(t, state, "/health")
	assert.Equal(t, http.StatusOK, code, "/health keeps its legacy meaning: the process is up")

	pingErr = nil
	state.MarkDraining()
	code, body = probe(t, state, "/readyz")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "draining", body["status"])
	code, body = probe(t, state, "/health")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, map[string]any{"status": "draining"}, body)
	code, _ = probe(t, state, "/livez")
	assert.Equal(t, http.StatusOK, code, "draining is not a liveness failure")
}

func TestHealthKeepsTheLegacyResponseShape(t *testing.T) {
	code, body := probe(t, &State{}, "/health")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]any{"status": "ok"}, body)
}
