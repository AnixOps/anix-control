package grpc

import (
	"math"
	"regexp"
	"time"
)

// The Agent's own health metrics on its heartbeat (Heartbeat.metrics):
// agent_control_* (the stream), agent_identity_* (enrollment and the
// certificate) and agent_dataplane_* (spool depth and drops, apply
// failures). Plugin telemetry (plugin.*) is persisted per plugin
// (NodeService.RecordPluginTelemetry); these are kept with the live session
// instead, the latest heartbeat's set, so the session views (the
// agent-control status, the transport inventory) can show them.
const maxAgentSessionMetrics = 64

var agentSessionMetricName = regexp.MustCompile(`^agent_(control|identity|dataplane)_[a-z0-9_]{1,96}$`)

// agentSessionMetrics keeps the Agent health metrics of a heartbeat: known
// prefixes, finite values, at most maxAgentSessionMetrics (the rest
// dropped). Nil when there are none.
func agentSessionMetrics(metrics map[string]float64) map[string]float64 {
	var kept map[string]float64
	for name, value := range metrics {
		if !agentSessionMetricName.MatchString(name) || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		if kept == nil {
			kept = make(map[string]float64)
		}
		if len(kept) >= maxAgentSessionMetrics {
			break
		}
		kept[name] = value
	}
	return kept
}

// recordAgentMetrics replaces the session's Agent health metrics with a
// heartbeat's, when it carries any.
func (c *AgentControlConnection) recordAgentMetrics(metrics map[string]float64, at time.Time) {
	kept := agentSessionMetrics(metrics)
	if kept == nil {
		return
	}
	c.stateMu.Lock()
	c.agentMetrics, c.agentMetricsAt = kept, at
	c.stateMu.Unlock()
}

// agentMetricsCopy returns the session's Agent health metrics and when they
// were reported; the caller holds stateMu.
func (c *AgentControlConnection) agentMetricsCopyLocked() (map[string]float64, *time.Time) {
	if c.agentMetrics == nil {
		return nil, nil
	}
	copied := make(map[string]float64, len(c.agentMetrics))
	for name, value := range c.agentMetrics {
		copied[name] = value
	}
	at := c.agentMetricsAt
	return copied, &at
}
