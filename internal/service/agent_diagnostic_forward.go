package service

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
)

// The forward checks of the agent.diagnostic operation (forward-sdk.md
// section 7.6, F3c): node-vantage probes of one hop of one route, which
// only the kernel's route diagnosis (kernelforward.DiagnoseRoute) sends.
// The Agent resolves the hop's listener and upstreams from the
// NodeForwardState it has applied and probes only those, so a check names
// a hop, never an address to dial; the optional upstream narrows the check
// to one of the hop's upstreams. sdk/api/agent/v1/PROTOCOL.md, "Forward
// diagnostic checks", is the Agent's reference.
const (
	// ForwardCheckListen: the hop's own listener holds its port.
	ForwardCheckListen = "forward.listen"
	// ForwardCheckPortConflict: no foreign process or nftables rule claims
	// the hop's listen port.
	ForwardCheckPortConflict = "forward.port_conflict"
	// ForwardCheckConnect: a TCP connect to each of the hop's upstreams.
	ForwardCheckConnect = "forward.connect"
	// ForwardCheckUDPProbe: a short datagram to each of the hop's upstreams,
	// waiting for any reply; no reply is inconclusive, not a failure.
	ForwardCheckUDPProbe = "forward.udp_probe"

	// ForwardCheckDefaultTimeoutMS is a check's timeout when it names none.
	ForwardCheckDefaultTimeoutMS = 3000
	// ForwardCheckMinTimeoutMS and ForwardCheckMaxTimeoutMS bound it.
	ForwardCheckMinTimeoutMS = 100
	ForwardCheckMaxTimeoutMS = 10000
	// forwardCheckMaxHopIndex is validate.MaxHops - 1.
	forwardCheckMaxHopIndex = 7
	// forwardCheckMaxRouteID bounds a route id (Control's are 26-character
	// ULIDs).
	forwardCheckMaxRouteID = 64

	// ForwardTargetPolicyPublicOnly and ForwardTargetPolicyAllowPrivate are
	// the target_policy parameter: the most a check may dial among a last
	// hop's targets. The Agent applies the stricter of it and the hop's own
	// policy.
	ForwardTargetPolicyPublicOnly   = "public_only"
	ForwardTargetPolicyAllowPrivate = "allow_private"
)

// ForwardDiagnosticChecks lists the forward checks with their parameters.
// None changes the node.
var ForwardDiagnosticChecks = map[string]AgentDiagnosticActionSpec{
	ForwardCheckListen:       {Params: []string{"route_id", "hop_index", "generation", "timeout_ms"}},
	ForwardCheckPortConflict: {Params: []string{"route_id", "hop_index", "generation", "timeout_ms"}},
	ForwardCheckConnect:      {Params: []string{"route_id", "hop_index", "generation", "timeout_ms", "upstream", "target_policy"}},
	ForwardCheckUDPProbe:     {Params: []string{"route_id", "hop_index", "generation", "timeout_ms", "upstream", "target_policy"}},
}

// IsForwardDiagnosticCheck reports whether action is a forward check.
func IsForwardDiagnosticCheck(action string) bool {
	_, ok := ForwardDiagnosticChecks[action]
	return ok
}

// ValidateForwardDiagnosticCheck checks a forward check's parameters and
// answers them normalized: only the check's parameters, hop_index and
// generation as integers, timeout_ms clamped (ForwardCheckDefaultTimeoutMS
// when absent), target_policy public_only when absent.
func ValidateForwardDiagnosticCheck(action string, params map[string]any) (map[string]any, error) {
	spec, ok := ForwardDiagnosticChecks[action]
	if !ok {
		return nil, fmt.Errorf("action %q is not a forward diagnostic check", action)
	}
	normalized := map[string]any{}
	for _, key := range spec.Params {
		raw, present := params[key]
		switch key {
		case "route_id":
			id, _ := raw.(string)
			if id == "" || len(id) > forwardCheckMaxRouteID || strings.IndexFunc(id, func(r rune) bool { return r <= ' ' || r > '~' }) >= 0 {
				return nil, fmt.Errorf("route_id must be 1 to %d printable characters", forwardCheckMaxRouteID)
			}
			normalized[key] = id
		case "hop_index":
			index, ok := wholeNumber(raw)
			if !ok || index < 0 || index > forwardCheckMaxHopIndex {
				return nil, fmt.Errorf("hop_index must be an integer from 0 to %d", forwardCheckMaxHopIndex)
			}
			normalized[key] = index
		case "generation":
			if !present {
				continue
			}
			generation, ok := wholeNumber(raw)
			if !ok || generation < 0 {
				return nil, fmt.Errorf("generation must be a non-negative integer")
			}
			normalized[key] = generation
		case "timeout_ms":
			timeout := int64(ForwardCheckDefaultTimeoutMS)
			if present {
				value, ok := wholeNumber(raw)
				if !ok {
					return nil, fmt.Errorf("timeout_ms must be an integer")
				}
				timeout = min(max(value, ForwardCheckMinTimeoutMS), ForwardCheckMaxTimeoutMS)
			}
			normalized[key] = timeout
		case "upstream":
			if !present {
				continue
			}
			upstream, _ := raw.(string)
			host, port, err := net.SplitHostPort(upstream)
			if err != nil || host == "" {
				return nil, fmt.Errorf("upstream must be host:port")
			}
			if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
				return nil, fmt.Errorf("upstream must be host:port with a port from 1 to 65535")
			}
			normalized[key] = upstream
		case "target_policy":
			policy, _ := raw.(string)
			switch policy {
			case "":
				policy = ForwardTargetPolicyPublicOnly
			case ForwardTargetPolicyPublicOnly, ForwardTargetPolicyAllowPrivate:
			default:
				return nil, fmt.Errorf("target_policy must be %q or %q", ForwardTargetPolicyPublicOnly, ForwardTargetPolicyAllowPrivate)
			}
			normalized[key] = policy
		}
	}
	return normalized, nil
}

// wholeNumber reads a JSON number (float64 after decoding) or a Go
// integer as an int64.
func wholeNumber(raw any) (int64, bool) {
	switch v := raw.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case uint32:
		return int64(v), true
	case float64:
		if v != math.Trunc(v) || v < math.MinInt64 || v > math.MaxInt64 {
			return 0, false
		}
		return int64(v), true
	}
	return 0, false
}
