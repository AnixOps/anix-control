package agenttransport

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// maxPathLabels bounds the path label of the refusal counter. Paths are
// gRPC method names, a fixed set; anything beyond the
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

var legacyRefused = newCounterVec()

// CountRefused counts an agent request refused for lacking a client
// certificate, by gRPC method.
func CountRefused(path string) { legacyRefused.inc(path) }

// RefusedRequests returns the refused counter of path.
func RefusedRequests(path string) uint64 { return legacyRefused.get(path) }

// WritePrometheus renders the refusal metric in the Prometheus text format.
func WritePrometheus(body *strings.Builder) {
	body.WriteString("# HELP anixops_agent_legacy_refused_total Agent requests refused for lacking a client certificate (agent_control.mtls: required), by gRPC method.\n")
	body.WriteString("# TYPE anixops_agent_legacy_refused_total counter\n")
	legacyRefused.write(body, "anixops_agent_legacy_refused_total")
}
