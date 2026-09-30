package logging

import (
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func restoreDefaults(t *testing.T) {
	t.Helper()
	previous := slog.Default()
	flags := log.Flags()
	t.Cleanup(func() {
		slog.SetDefault(previous)
		log.SetFlags(flags)
		log.SetOutput(os.Stderr)
		jsonEnabled.Store(false)
		handlerMu.Lock()
		handler = nil
		handlerMu.Unlock()
	})
}

func decodeLines(t *testing.T, out *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record), line)
		records = append(records, record)
	}
	return records
}

func TestJSONFormatWritesEveryLogSourceAsJSONLines(t *testing.T) {
	restoreDefaults(t)
	var out bytes.Buffer
	SetupTo(config.LogConfig{Format: "json", Level: "warn"}, &out)
	require.True(t, JSON())

	log.Printf("plugin host %s restarted", "identity-platform")
	slog.Info("filtered by level")
	slog.Warn("kept by level", "component", "test")

	records := decodeLines(t, &out)
	require.Len(t, records, 2)
	assert.Equal(t, "plugin host identity-platform restarted", records[0]["msg"])
	assert.Equal(t, "INFO", records[0]["level"], "standard log lines are always written")
	assert.Equal(t, "kept by level", records[1]["msg"])
	assert.Equal(t, "test", records[1]["component"])
}

func TestAccessLogRecordsRequestsWithoutQueryStrings(t *testing.T) {
	restoreDefaults(t)
	var out bytes.Buffer
	SetupTo(config.LogConfig{Format: "json"}, &out)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AccessLog())
	router.GET("/api/v2/user/info", func(c *gin.Context) { c.Status(http.StatusTeapot) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v2/user/info?token=secret", nil))

	records := decodeLines(t, &out)
	require.Len(t, records, 1)
	assert.Equal(t, "http request", records[0]["msg"])
	assert.Equal(t, "/api/v2/user/info", records[0]["path"])
	assert.EqualValues(t, http.StatusTeapot, records[0]["status"])
	assert.Equal(t, "WARN", records[0]["level"])
	assert.NotContains(t, out.String(), "secret")
}

func TestTextFormatLeavesLoggingUnchanged(t *testing.T) {
	restoreDefaults(t)
	before := slog.Default()
	SetupTo(config.LogConfig{Format: "text", Level: "error"}, &bytes.Buffer{})
	assert.False(t, JSON())
	assert.Same(t, before, slog.Default())
}
