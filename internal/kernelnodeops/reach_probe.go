package kernelnodeops

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
)

// The reachability probe of the agent.diagnostic operation
// (sdk/api/agent/v1/PROTOCOL.md, "Reachability probe"): a proxy node's
// Agent that lists the capability reach.probe opens a TCP connection to
// each port of the targets from the node's network, closes it at once and
// answers per port whether it connected, was refused or got no answer.
const (
	ReachProbeAction     = "reach.probe"
	ReachProbeCapability = "reach.probe"
)

// ErrReachProbeUnavailable is the error of a probe the node's Agent cannot
// run: not connected, or it does not list reach.probe.
var ErrReachProbeUnavailable = errors.New("the node's Agent does not run reach.probe")

// ReachTarget is one target of a probe: a host (address or name) and its
// ports.
type ReachTarget struct {
	Host  string `json:"host"`
	Ports []int  `json:"ports"`
}

// ReachProbes runs reach.probe on the Agent Control stream of a proxy node.
// It is not an administrator task: it is not recorded in
// v2_agent_diagnostic_task.
type ReachProbes struct {
	// Sources are the live sessions; DefaultAgentSources when nil.
	Sources SourcesFunc
}

// Capable answers whether node's Agent is connected and runs the probe.
func (p ReachProbes) Capable(node agentcontrol.AgentNode) bool {
	streams := RealityProbes{Sources: p.Sources}.streams()
	if streams == nil {
		return false
	}
	session, ok := streams.Session(node)
	return ok && hasCapability(session.Capabilities, AgentDiagnosticOperation) && hasCapability(session.Capabilities, ReachProbeCapability)
}

// Probe runs reach.probe for targets on node, each connection bounded by
// timeout, and answers the Agent's result (the probe's JSON).
func (p ReachProbes) Probe(ctx context.Context, node agentcontrol.AgentNode, targets []ReachTarget, timeout time.Duration) (json.RawMessage, error) {
	if !p.Capable(node) {
		return nil, ErrReachProbeUnavailable
	}
	streams := RealityProbes{Sources: p.Sources}.streams()
	params := map[string]any{"targets": targets, "timeout_ms": timeout.Milliseconds()}
	// The Agent runs 16 connections at a time: the slack covers the rounds.
	rounds := time.Duration(len(targets)*4/16 + 1)
	return runCheck(ctx, streams, node, ReachProbeAction, "reach-", params, timeout, rounds*timeout+5*time.Second, ErrReachProbeUnavailable)
}
