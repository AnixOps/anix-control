package kernelforward

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/model"
	"google.golang.org/protobuf/proto"
)

// Route diagnosis (forward-sdk.md section 7.6, F3c). DiagnoseRoute is read
// only (it never takes the plan lock). Every route runs on the network
// model and is diagnosed there (network_diagnose.go), after the steps of a
// paused or enforced route; a stored route left on a removed engine is not
// planned, so its diagnosis is those steps alone. Everything runs within
// one time budget; a probe the budget leaves no time for is SKIPPED with
// code "deadline".

// Diagnosis bounds.
const (
	// DefaultDiagnoseTimeout is a diagnosis's budget when the request names
	// none; MaxDiagnoseTimeout caps the budget a request asks for, below
	// the package host's request timeout (30 s).
	DefaultDiagnoseTimeout = 15 * time.Second
	MaxDiagnoseTimeout     = 25 * time.Second
	minDiagnoseTimeout     = time.Second
	// DiagnoseCacheTTL is how long a finished diagnosis is answered again
	// (cached) for the same route, which also rate-limits each route.
	DiagnoseCacheTTL = 10 * time.Second
	// MaxConcurrentDiagnoses bounds the routes diagnosed at once in one
	// Control process; one more is ErrDiagnoseBusy.
	MaxConcurrentDiagnoses = 4
	// diagnoseProbeConcurrency bounds the probes in flight per diagnosis.
	diagnoseProbeConcurrency = 8
	// diagnoseProbeTimeout bounds a dial from Control.
	diagnoseProbeTimeout = 3 * time.Second
	// diagnoseCheckTimeout and diagnoseUDPTimeout are the timeout_ms a
	// forward check names for the Agent: each dial (the Agent dials a hop's
	// upstreams at once), or the wait for a UDP reply.
	diagnoseCheckTimeout = 2 * time.Second
	diagnoseUDPTimeout   = 1500 * time.Millisecond
	// diagnoseCheckGrace is how much longer than a forward check's own
	// timeout Control waits for the Agent's answer.
	diagnoseCheckGrace = 2 * time.Second
)

// ErrDiagnoseBusy refuses a diagnosis while MaxConcurrentDiagnoses run
// (RESOURCE_EXHAUSTED).
var ErrDiagnoseBusy = errors.New("too many route diagnoses are running; retry shortly")

// Stable step codes (ProbeResult.code).
const (
	CodeRoutePaused            = "route_paused"
	CodeRouteEnforced          = "route_enforced"
	CodeNotPlanned             = "not_planned"
	CodeNeverReported          = "never_reported"
	CodeNotApplied             = "not_applied"
	CodeApplyFailed            = "apply_failed"
	CodeHopError               = "hop_error"
	CodeHealthy                = "healthy"
	CodeUnhealthy              = "unhealthy"
	CodeCircuitOpen            = "circuit_open"
	CodeNoHealth               = "no_health"
	CodeNodeVantageUnavailable = "node_vantage_unavailable"
	CodeNodeOffline            = "node_offline"
	CodeTargetNotAllowed       = "target_not_allowed"
	CodeNotPublic              = "not_public"
	CodeDeadline               = "deadline"
	CodeAgentTimeout           = "agent_timeout"
	CodeAgentError             = "agent_error"
	CodeReachable              = "reachable"
	CodeUnreachable            = "unreachable"
	CodeControlUDP             = "control_udp_not_probed"
)

// Forward check statuses, as the Agent answers them (CheckResult.Status,
// CheckItem.Status).
const (
	CheckOK           = "ok"
	CheckFailed       = "failed"
	CheckInconclusive = "inconclusive"
	CheckSkipped      = "skipped"
)

// NodeVantage is whether a node can run the forward checks.
type NodeVantage struct {
	// Connected: the node's Agent holds an Agent Control session.
	Connected bool
	// Capable: the session advertises agent.diagnostic and diag.v1.
	Capable bool
	// Note says why the node cannot probe, for people.
	Note string
}

// CheckResult is the structured answer of one forward check: the "result"
// member of the agent.diagnostic state (sdk/api/agent/v1/PROTOCOL.md,
// "Forward diagnostic checks").
type CheckResult struct {
	Check      string      `json:"check"`
	RouteID    string      `json:"route_id"`
	HopIndex   uint32      `json:"hop_index"`
	Generation uint64      `json:"generation"`
	Status     string      `json:"status"`
	Code       string      `json:"code"`
	Message    string      `json:"message"`
	Items      []CheckItem `json:"items"`
	// OperationID is the agent.diagnostic operation that carried the
	// check; set by the NodeChecker.
	OperationID string `json:"-"`
}

// CheckItem is one listener, port claim or upstream a check looked at.
type CheckItem struct {
	// Target is "host:port"; Protocol "tcp" or "udp".
	Target   string `json:"target"`
	Protocol string `json:"protocol"`
	Status   string `json:"status"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	RTTMicro uint32 `json:"rtt_us"`
}

// NodeChecker runs the forward checks of the agent.diagnostic operation on
// a node's Agent (kernelnodeops.ForwardChecks serves it on the Agent
// Control streams).
type NodeChecker interface {
	// Vantage answers whether node can run the checks.
	Vantage(node agentcontrol.AgentNode) NodeVantage
	// Check runs one check (service.ForwardDiagnosticChecks) with params
	// and waits for the Agent's answer until ctx ends. An error means the
	// check gave no answer: ErrCheckUnavailable when the node cannot run
	// it, context.DeadlineExceeded when the Agent did not answer in time,
	// anything else when the Agent failed it.
	Check(ctx context.Context, node agentcontrol.AgentNode, action string, params map[string]any) (*CheckResult, error)
}

// ErrCheckUnavailable: the node's Agent is not connected or does not run
// the forward checks.
var ErrCheckUnavailable = errors.New("the node's Agent cannot run forward checks")

// DiagnoseRoute diagnoses a stored route within timeout (0 for
// DefaultDiagnoseTimeout, at most MaxDiagnoseTimeout, and never past ctx's
// deadline less a second). A diagnosis of the same route that finished
// within DiagnoseCacheTTL, or that is running, is answered (cached).
// ErrNotFound for an unknown route, ErrDiagnoseBusy when too many run.
func (s *Service) DiagnoseRoute(ctx context.Context, routeID string, timeout time.Duration) (*forwardv1.DiagnoseRouteResponse, error) {
	if routeID == "" {
		return nil, fmt.Errorf("%w: route_id is required", ErrInvalidRequest)
	}
	budget := DefaultDiagnoseTimeout
	if timeout > 0 {
		budget = min(max(timeout, minDiagnoseTimeout), MaxDiagnoseTimeout)
	}
	if deadline, ok := ctx.Deadline(); ok {
		budget = min(budget, time.Until(deadline)-time.Second)
		if budget < minDiagnoseTimeout/2 {
			return nil, context.DeadlineExceeded
		}
	}
	return diagnoses.run(ctx, s, routeID, budget)
}

// diagnoseCache answers recent diagnoses again and bounds those running.
type diagnoseCache struct {
	mu      sync.Mutex
	running int
	entries map[string]*diagnoseEntry
}

type diagnoseEntry struct {
	done     chan struct{}
	answer   *forwardv1.DiagnoseRouteResponse
	err      error
	finished time.Time
}

// diagnoses is the process's cache: the kernel's ForwardControl server and
// the command line each hold one Service per process.
var diagnoses = newDiagnoseCache()

func newDiagnoseCache() *diagnoseCache {
	return &diagnoseCache{entries: map[string]*diagnoseEntry{}}
}

// resetDiagnosesForTest empties the cache.
func resetDiagnosesForTest() { diagnoses = newDiagnoseCache() }

func (c *diagnoseCache) run(ctx context.Context, s *Service, routeID string, budget time.Duration) (*forwardv1.DiagnoseRouteResponse, error) {
	now := time.Now()
	c.mu.Lock()
	for key, entry := range c.entries {
		if !entry.finished.IsZero() && now.Sub(entry.finished) >= DiagnoseCacheTTL {
			delete(c.entries, key)
		}
	}
	if entry, ok := c.entries[routeID]; ok {
		c.mu.Unlock()
		select {
		case <-entry.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if entry.err != nil {
			return nil, entry.err
		}
		answer := proto.Clone(entry.answer).(*forwardv1.DiagnoseRouteResponse)
		answer.Cached = true
		return answer, nil
	}
	if c.running >= MaxConcurrentDiagnoses {
		c.mu.Unlock()
		return nil, ErrDiagnoseBusy
	}
	entry := &diagnoseEntry{done: make(chan struct{})}
	c.entries[routeID] = entry
	c.running++
	c.mu.Unlock()

	// The diagnosis outlives a caller that gives up: others may be waiting.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), budget)
	answer, err := s.diagnose(runCtx, routeID)
	cancel()

	c.mu.Lock()
	c.running--
	entry.answer, entry.err, entry.finished = answer, err, time.Now()
	if err != nil {
		// A refusal or failure is not remembered: the next call retries.
		delete(c.entries, routeID)
	}
	close(entry.done)
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return proto.Clone(answer).(*forwardv1.DiagnoseRouteResponse), nil
}

// diagnosis is one run's state.
type diagnosis struct {
	s     *Service
	route *forwardv1.Route
	steps []*forwardv1.DiagnoseStep
}

func (d *diagnosis) add(step *forwardv1.DiagnoseStep) {
	if step.GetResult() != nil {
		step.Result.Ok = step.GetResult().GetStatus() == forwardv1.ProbeStatus_PROBE_STATUS_OK
		if step.Result.ObservedAtUnixMs == 0 {
			step.Result.ObservedAtUnixMs = d.s.now().UnixMilli()
		}
	}
	d.steps = append(d.steps, step)
}

func result(status forwardv1.ProbeStatus, code, message string) *forwardv1.ProbeResult {
	return &forwardv1.ProbeResult{Status: status, Code: code, Message: message}
}

const (
	statusOK           = forwardv1.ProbeStatus_PROBE_STATUS_OK
	statusFailed       = forwardv1.ProbeStatus_PROBE_STATUS_FAILED
	statusInconclusive = forwardv1.ProbeStatus_PROBE_STATUS_INCONCLUSIVE
	statusSkipped      = forwardv1.ProbeStatus_PROBE_STATUS_SKIPPED
	vantageControl     = forwardv1.DiagnoseVantage_DIAGNOSE_VANTAGE_CONTROL
	protoTCP           = forwardv1.L4Protocol_L4_PROTOCOL_TCP
	protoUDP           = forwardv1.L4Protocol_L4_PROTOCOL_UDP
)

func (s *Service) diagnose(ctx context.Context, routeID string) (*forwardv1.DiagnoseRouteResponse, error) {
	started := s.now()
	route, err := s.GetRoute(ctx, routeID)
	if err != nil {
		return nil, err
	}
	d := &diagnosis{s: s, route: route}
	enforced, err := s.Enforcement(ctx, []string{routeID})
	if err != nil {
		return nil, err
	}

	// Stage 1: Control's records.
	d.routeSteps(enforced[routeID])
	if _, ok := networkForward(route); ok {
		return d.diagnoseNetwork(ctx, started)
	}
	// A route left on a removed engine is not planned: there is nothing
	// on the nodes to probe.
	return d.answer(started, nil), nil
}

// routeSteps adds the steps of a paused or enforced route.
func (d *diagnosis) routeSteps(enforced string) {
	if d.route.GetPaused() {
		d.add(&forwardv1.DiagnoseStep{Kind: forwardv1.ProbeKind_PROBE_KIND_CONFIG, Vantage: vantageControl,
			Result: result(statusFailed, CodeRoutePaused, "the route is paused: its nodes drop its traffic")})
	}
	if enforced != "" {
		d.add(&forwardv1.DiagnoseStep{Kind: forwardv1.ProbeKind_PROBE_KIND_CONFIG, Vantage: vantageControl,
			Result: result(statusFailed, CodeRouteEnforced, "Control pauses the route: "+enforced)})
	}
}

// answer closes the diagnosis: ok unless a step failed.
func (d *diagnosis) answer(started time.Time, nodes []*forwardv1.DiagnoseNode) *forwardv1.DiagnoseRouteResponse {
	answer := &forwardv1.DiagnoseRouteResponse{Ok: true, Steps: d.steps, RouteId: d.route.GetId(), Nodes: nodes,
		StartedAtUnixMs: started.UnixMilli(), FinishedAtUnixMs: d.s.now().UnixMilli()}
	for _, step := range d.steps {
		if step.GetResult().GetStatus() == statusFailed {
			answer.Ok = false
		}
	}
	return answer
}

// routeNodes lists the route's nodes in hop order, once each.
func routeNodes(route *forwardv1.Route) []string {
	var refs []string
	seen := map[string]bool{}
	for _, hop := range route.GetHops() {
		for _, ref := range hop.GetNodeRefs() {
			if !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
	}
	return refs
}

func hopError(report *forwardv1.NodeForwardReport, routeID string, index uint32) string {
	for _, hopErr := range report.GetErrors() {
		if hopErr.GetRouteId() == routeID && hopErr.GetHopIndex() == index {
			return hopErr.GetMessage()
		}
	}
	return ""
}

// healthResult is the verdict of an upstream's reported health; healthy
// is the message when it is.
func healthResult(health *forwardv1.UpstreamHealth, healthy string) *forwardv1.ProbeResult {
	var out *forwardv1.ProbeResult
	switch health.GetState() {
	case forwardv1.HealthState_HEALTH_STATE_HEALTHY:
		out = result(statusOK, CodeHealthy, healthy)
	case forwardv1.HealthState_HEALTH_STATE_CIRCUIT_OPEN:
		out = result(statusFailed, CodeCircuitOpen, fmt.Sprintf("circuit breaker open after %d failures in a row, until %s",
			health.GetConsecutiveFailures(), time.UnixMilli(health.GetCircuitOpenUntilUnixMs()).UTC().Format(time.RFC3339)))
	case forwardv1.HealthState_HEALTH_STATE_UNHEALTHY:
		out = result(statusFailed, CodeUnhealthy, fmt.Sprintf("unhealthy: %d failures in a row", health.GetConsecutiveFailures()))
	default:
		out = result(statusInconclusive, CodeNoHealth, "health state not reported")
	}
	out.RttUs = health.GetRttUs()
	out.ObservedAtUnixMs = health.GetCheckedAtUnixMs()
	return out
}

func hostPort(host string, port uint32) string {
	return net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10))
}

// routeProtocols answers whether the route carries TCP and UDP.
func routeProtocols(route *forwardv1.Route) (tcp, udp bool) {
	switch route.GetListen().GetProtocol() {
	case forwardv1.L4Protocol_L4_PROTOCOL_UDP:
		return false, true
	case forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP:
		return true, true
	}
	return true, false
}

// effectiveTargetPolicy is the most a probe may dial among the targets:
// ALLOW_PRIVATE only on an administrator's route that asks for it.
func effectiveTargetPolicy(route *forwardv1.Route) model.TargetPolicy {
	if route.GetOwner() == "admin" && route.GetPolicy().GetTargetPolicy() == forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE {
		return model.TargetPolicyAllowPrivate
	}
	return model.TargetPolicyPublicOnly
}

// controlDial connects from Control to a public address.
func (d *diagnosis) controlDial(ctx context.Context, address string) *forwardv1.ProbeResult {
	if ctx.Err() != nil {
		return result(statusSkipped, CodeDeadline, "the diagnosis ran out of time before this dial")
	}
	host, port, _ := net.SplitHostPort(address)
	addr, refusal := d.s.Probes.PublicHost(ctx, host)
	if refusal != "" {
		return result(statusSkipped, CodeNotPublic, "Control dials public addresses only: "+refusal)
	}
	timeout := diagnoseProbeTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline))
	}
	dial := d.s.Probes.Dial
	if dial == nil {
		dial = func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
			dialer := net.Dialer{Timeout: timeout}
			return dialer.DialContext(ctx, network, address)
		}
	}
	began := time.Now()
	conn, err := dial(ctx, "tcp", net.JoinHostPort(addr.String(), port), timeout)
	if err != nil {
		if ctx.Err() != nil {
			return result(statusInconclusive, CodeDeadline, "the diagnosis ran out of time during the dial")
		}
		return result(statusFailed, CodeUnreachable, "Control could not connect: "+err.Error())
	}
	_ = conn.Close()
	out := result(statusOK, CodeReachable, "Control connected to "+addr.String())
	out.RttUs = uint32(min(time.Since(began).Microseconds(), 1<<32-1)) // #nosec G115 -- bounded above.
	return out
}
