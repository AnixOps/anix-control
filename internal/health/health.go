// Package health serves the process probes shared by the API and frontend
// servers: /livez (the process is running), /readyz (the process can serve
// traffic), and the legacy /health.
package health

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// readyzPingTimeout bounds the database check of one /readyz request.
const readyzPingTimeout = 2 * time.Second

// State tracks what the probes report. The zero value is live, not ready.
type State struct {
	started  atomic.Bool
	draining atomic.Bool

	mu   sync.RWMutex
	ping func(context.Context) error
}

// Default is the process-wide probe state.
var Default = &State{}

// SetDatabasePinger installs the database check used by /readyz.
func (s *State) SetDatabasePinger(ping func(context.Context) error) {
	s.mu.Lock()
	s.ping = ping
	s.mu.Unlock()
}

// MarkStarted reports that startup finished and the servers accept traffic.
func (s *State) MarkStarted() { s.started.Store(true) }

// MarkDraining reports that shutdown began: /readyz and /health fail so load
// balancers stop sending new requests, while /livez keeps succeeding.
func (s *State) MarkDraining() { s.draining.Store(true) }

// Draining reports whether shutdown began.
func (s *State) Draining() bool { return s.draining.Load() }

// Register adds /livez, /readyz and /health to r.
func (s *State) Register(r gin.IRoutes) {
	r.GET("/livez", s.Live)
	r.GET("/readyz", s.Ready)
	r.GET("/health", s.Health)
}

// Live always succeeds while the process can answer HTTP.
func (s *State) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready succeeds when startup finished, shutdown has not begun, and the
// database answers a ping.
func (s *State) Ready(c *gin.Context) {
	status, body := s.readiness(c.Request.Context())
	if status != http.StatusOK {
		c.Header("Retry-After", "5")
	}
	c.JSON(status, body)
}

// Health keeps the legacy response shape ({"status":"ok"}) and fails with
// {"status":"draining"} during shutdown.
func (s *State) Health(c *gin.Context) {
	if s.Draining() {
		c.Header("Retry-After", "30")
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "draining"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *State) readiness(ctx context.Context) (int, gin.H) {
	switch {
	case s.Draining():
		return http.StatusServiceUnavailable, gin.H{"status": "draining"}
	case !s.started.Load():
		return http.StatusServiceUnavailable, gin.H{"status": "starting"}
	}
	s.mu.RLock()
	ping := s.ping
	s.mu.RUnlock()
	if ping != nil {
		pingCtx, cancel := context.WithTimeout(ctx, readyzPingTimeout)
		defer cancel()
		if err := ping(pingCtx); err != nil {
			return http.StatusServiceUnavailable, gin.H{"status": "unavailable", "checks": gin.H{"database": "unreachable"}}
		}
	}
	return http.StatusOK, gin.H{"status": "ok", "checks": gin.H{"database": "ok"}}
}
