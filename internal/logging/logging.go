// Package logging configures process-wide log output from the log config
// block. The text format keeps the historical plain lines. The json format
// writes one JSON object per line to stdout for container log collectors:
// standard-library log lines, slog records, HTTP access logs and GORM logs all
// go through the same handler.
package logging

import (
	"context"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/gin-gonic/gin"
)

var (
	jsonEnabled atomic.Bool
	handlerMu   sync.RWMutex
	handler     slog.Handler
)

// Setup applies cfg to the process. It must run before the HTTP router and
// the database are initialised, so they pick up the chosen format.
func Setup(cfg config.LogConfig) {
	SetupTo(cfg, os.Stdout)
}

// SetupTo is Setup with an explicit destination, for tests.
func SetupTo(cfg config.LogConfig, out io.Writer) {
	if strings.TrimSpace(cfg.Format) != "json" {
		// The text format keeps the historical output unchanged.
		jsonEnabled.Store(false)
		return
	}
	jsonHandler := slog.NewJSONHandler(out, &slog.HandlerOptions{Level: parseLevel(cfg.Level)})
	handlerMu.Lock()
	handler = jsonHandler
	handlerMu.Unlock()
	jsonEnabled.Store(true)
	slog.SetDefault(slog.New(jsonHandler))
	// slog.SetDefault routes the log package through the handler at INFO,
	// which a level above info would drop. Standard-library lines are the
	// server's existing operational messages, including errors, so they are
	// always written.
	log.SetFlags(0)
	log.SetOutput(StdWriter())
}

// JSON reports whether the json format is active.
func JSON() bool { return jsonEnabled.Load() }

// StdWriter returns a writer that turns each written line into an INFO
// record of the active JSON handler, regardless of the configured level.
func StdWriter() io.Writer { return lineWriter{} }

type lineWriter struct{}

func (lineWriter) Write(p []byte) (int, error) {
	handlerMu.RLock()
	current := handler
	handlerMu.RUnlock()
	message := strings.TrimRight(string(p), "\r\n")
	if current == nil || message == "" {
		return len(p), nil
	}
	record := slog.NewRecord(time.Now(), slog.LevelInfo, message, 0)
	if err := current.Handle(context.Background(), record); err != nil {
		return 0, err
	}
	return len(p), nil
}

// AccessLog is the json replacement for gin.Logger: one record per request.
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		status := c.Writer.Status()
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", status),
			slog.Float64("latency_ms", float64(time.Since(start).Microseconds())/1000),
			slog.String("client_ip", c.ClientIP()),
			slog.Int("bytes", c.Writer.Size()),
		}
		if requestID, ok := c.Get("request_id"); ok {
			if value, ok := requestID.(string); ok && value != "" {
				attrs = append(attrs, slog.String("request_id", value))
			}
		}
		slog.LogAttrs(c.Request.Context(), level, "http request", attrs...)
	}
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
