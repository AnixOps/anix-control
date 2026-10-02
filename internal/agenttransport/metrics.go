package agenttransport

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/AnixOps/anix-control/v4/internal/config"
)

// maxPathLabels bounds the path label of the legacy counters. Paths are
// route templates and gRPC method names, a fixed set; anything beyond the
// bound counts under otherPath.
const (
	maxPathLabels = 128
	otherPath     = "other"
)

// counterVec is a counter with one path label.
type counterVec struct {
	mu     sync.RWMutex
	values map[string]*atomic.Uint64
}

func newCounterVec() *counterVec { return &counterVec{values: map[string]*atomic.Uint64{}} }

func (c *counterVec) inc(path string) {
	if path == "" {
		path = otherPath
	}
	c.mu.RLock()
	counter := c.values[path]
	c.mu.RUnlock()
	if counter == nil {
		c.mu.Lock()
		if counter = c.values[path]; counter == nil {
			if len(c.values) >= maxPathLabels {
				path = otherPath
			}
			if counter = c.values[path]; counter == nil {
				counter = &atomic.Uint64{}
				c.values[path] = counter
			}
		}
		c.mu.Unlock()
	}
	counter.Add(1)
}

func (c *counterVec) get(path string) uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if counter := c.values[path]; counter != nil {
		return counter.Load()
	}
	return 0
}

func (c *counterVec) write(body *strings.Builder, name string) {
	c.mu.RLock()
	paths := make([]string, 0, len(c.values))
	for path := range c.values {
		paths = append(paths, path)
	}
	c.mu.RUnlock()
	sort.Strings(paths)
	for _, path := range paths {
		body.WriteString(name + `{path="` + escapeLabel(path) + `"} ` + strconv.FormatUint(c.get(path), 10) + "\n")
	}
}

func escapeLabel(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(value)
}

var (
	legacyRequests = newCounterVec()
	legacyRefused  = newCounterVec()
	// currentMode is the agent_control.mtls this process runs with, for
	// the mode gauge; SetMode sets it at startup.
	currentMode atomic.Value
)

// CountLegacy counts a request a legacy AnixOps Agent channel served, by
// path: a route template (/api/v2/agent/heartbeat) or a gRPC method.
func CountLegacy(path string) { legacyRequests.inc(path) }

// CountRefused counts a legacy request agent_control.mtls: required
// refused.
func CountRefused(path string) { legacyRefused.inc(path) }

// LegacyRequests returns the served counter of path (tests and the CLI).
func LegacyRequests(path string) uint64 { return legacyRequests.get(path) }

// RefusedRequests returns the refused counter of path.
func RefusedRequests(path string) uint64 { return legacyRefused.get(path) }

// SetMode records the agent_control.mtls this process enforces, for
// anixops_agent_mtls_mode.
func SetMode(mode string) { currentMode.Store(mode) }

// WritePrometheus renders the transition metrics in the Prometheus text
// format. The names follow the repository's anixops_ prefix (the design
// doc's anix_agent_legacy_requests_total predates it).
func WritePrometheus(body *strings.Builder) {
	body.WriteString("# HELP anixops_agent_legacy_requests_total Requests served on a legacy AnixOps Agent channel (node API key on the stream, the legacy agent HTTP paths, the agent WebSocket), by path.\n")
	body.WriteString("# TYPE anixops_agent_legacy_requests_total counter\n")
	legacyRequests.write(body, "anixops_agent_legacy_requests_total")
	body.WriteString("# HELP anixops_agent_legacy_refused_total Legacy AnixOps Agent requests refused by agent_control.mtls: required, by path.\n")
	body.WriteString("# TYPE anixops_agent_legacy_refused_total counter\n")
	legacyRefused.write(body, "anixops_agent_legacy_refused_total")
	mode, _ := currentMode.Load().(string)
	if mode == "" {
		return
	}
	body.WriteString("# HELP anixops_agent_mtls_mode The agent_control.mtls mode this process enforces (1 for the current mode).\n")
	body.WriteString("# TYPE anixops_agent_mtls_mode gauge\n")
	for _, candidate := range config.AgentMTLSModes {
		value := "0"
		if candidate == mode {
			value = "1"
		}
		body.WriteString(`anixops_agent_mtls_mode{mode="` + candidate + `"} ` + value + "\n")
	}
}
