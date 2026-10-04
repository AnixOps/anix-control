package kernelforward

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/model"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	internalmodel "github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/protobuf/proto"
)

// Route diagnosis (forward-sdk.md section 7.6, F3c). DiagnoseRoute runs in
// stages, read only (it never takes the plan lock):
//
//  1. Control's records: the route paused or enforced; for every node of
//     every hop, its desired state holds the hop and its latest report runs
//     the desired generation, applied, without an error for the hop
//     (PROBE_KIND_CONFIG); every upstream's health and breaker in that
//     report (PROBE_KIND_HEALTH).
//  2. Node vantage, for each node whose Agent advertises agent.diagnostic
//     and diag.v1 (NodeChecker): the forward checks of agent.diagnostic on
//     each of its hops: forward.listen (LISTEN), forward.port_conflict
//     (PORT_CONFLICT), forward.connect (TCP_CONNECT, DELIVERY on the last
//     hop) and, for UDP routes, forward.udp_probe (UDP_EXCHANGE, DELIVERY
//     on the last hop). A last hop is never asked to dial a literal target
//     the route's policy refuses.
//  3. Control vantage, for what no node could probe: a TCP connect from
//     Control to the entry listeners and to the targets, public addresses
//     only whatever the route's policy (Control is not on the exit's
//     network). Relay and exit listeners admit only the previous hop, so
//     Control never dials them.
//
// Everything runs within one time budget; a probe the budget leaves no time
// for is SKIPPED with code "deadline".

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
	s        *Service
	route    *forwardv1.Route
	states   map[string]*forwardv1.NodeForwardState
	reports  map[string]*forwardv1.NodeForwardReport
	observed map[string]time.Time
	infos    map[string]*forwardv1.NodeInfo
	vantage  map[string]NodeVantage
	steps    []*forwardv1.DiagnoseStep
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
	vantageNode        = forwardv1.DiagnoseVantage_DIAGNOSE_VANTAGE_NODE
	protoTCP           = forwardv1.L4Protocol_L4_PROTOCOL_TCP
	protoUDP           = forwardv1.L4Protocol_L4_PROTOCOL_UDP
)

func (s *Service) diagnose(ctx context.Context, routeID string) (*forwardv1.DiagnoseRouteResponse, error) {
	started := s.now()
	route, err := s.GetRoute(ctx, routeID)
	if err != nil {
		return nil, err
	}
	d := &diagnosis{s: s, route: route, states: map[string]*forwardv1.NodeForwardState{}, reports: map[string]*forwardv1.NodeForwardReport{},
		observed: map[string]time.Time{}, infos: map[string]*forwardv1.NodeInfo{}, vantage: map[string]NodeVantage{}}
	refs := routeNodes(route)
	if err := d.load(ctx, refs); err != nil {
		return nil, err
	}
	enforced, err := s.Enforcement(ctx, []string{routeID})
	if err != nil {
		return nil, err
	}

	// Stage 1: Control's records.
	if route.GetPaused() {
		d.add(&forwardv1.DiagnoseStep{Kind: forwardv1.ProbeKind_PROBE_KIND_CONFIG, Vantage: vantageControl,
			Result: result(statusFailed, CodeRoutePaused, "the route is paused: its nodes drop its traffic")})
	}
	if reason := enforced[routeID]; reason != "" {
		d.add(&forwardv1.DiagnoseStep{Kind: forwardv1.ProbeKind_PROBE_KIND_CONFIG, Vantage: vantageControl,
			Result: result(statusFailed, CodeRouteEnforced, "Control pauses the route: "+reason)})
	}
	for index, hop := range route.GetHops() {
		for _, ref := range uniqueRefs(hop.GetNodeRefs()) {
			d.configStep(uint32(index), ref)  // #nosec G115 -- validate bounds hops to MaxHops.
			d.healthSteps(uint32(index), ref) // #nosec G115 -- as above.
		}
	}

	// Which nodes can probe.
	nodes := make([]*forwardv1.DiagnoseNode, 0, len(refs))
	for _, ref := range refs {
		v := NodeVantage{Note: "this process holds no Agent sessions (the command line): POST /api/v4/forward/routes/{id}/diagnose runs the node probes"}
		if node, err := agentcontrol.ParseAgentNode(ref); err == nil && s.Checks != nil {
			v = s.Checks.Vantage(node)
		} else if err != nil {
			v.Note = "not an Agent node reference"
		}
		d.vantage[ref] = v
		nodes = append(nodes, &forwardv1.DiagnoseNode{NodeRef: ref, Connected: v.Connected, NodeVantage: v.Capable, Note: v.Note})
	}

	// Stage 2 and 3.
	d.nodeProbes(ctx)
	d.controlProbes(ctx)

	answer := &forwardv1.DiagnoseRouteResponse{Ok: true, Steps: d.steps, RouteId: routeID, Nodes: nodes,
		StartedAtUnixMs: started.UnixMilli(), FinishedAtUnixMs: s.now().UnixMilli()}
	for _, step := range d.steps {
		if step.GetResult().GetStatus() == statusFailed {
			answer.Ok = false
		}
	}
	return answer, nil
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

func uniqueRefs(refs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, ref := range refs {
		if !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	return out
}

// load reads the nodes' desired states, latest reports and inventory.
func (d *diagnosis) load(ctx context.Context, refs []string) error {
	db, err := d.s.db(ctx)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}
	var states []internalmodel.KernelForwardNodeState
	if err := db.Where("node_ref IN ?", refs).Find(&states).Error; err != nil {
		return err
	}
	for _, row := range states {
		state := &forwardv1.NodeForwardState{}
		if err := jsonRead.Unmarshal([]byte(row.StateJSON), state); err != nil {
			return fmt.Errorf("kernel forward: state of %s: %w", row.NodeRef, err)
		}
		d.states[row.NodeRef] = state
	}
	var reports []internalmodel.KernelForwardNodeReport
	if err := db.Where("node_ref IN ?", refs).Find(&reports).Error; err != nil {
		return err
	}
	for _, row := range reports {
		report := &forwardv1.NodeForwardReport{}
		if err := jsonRead.Unmarshal([]byte(row.ReportJSON), report); err != nil {
			return fmt.Errorf("kernel forward: report of %s: %w", row.NodeRef, err)
		}
		d.reports[row.NodeRef] = report
		d.observed[row.NodeRef] = row.ObservedAt
	}
	inv, err := loadInventory(db)
	if err != nil {
		return err
	}
	for _, info := range inv.nodes {
		d.infos[info.GetNodeRef()] = info
	}
	return nil
}

// nodeHop is the node's desired hop of the route, nil when its state has
// none.
func (d *diagnosis) nodeHop(ref string, index uint32) *forwardv1.NodeHop {
	for _, hop := range d.states[ref].GetHops() {
		if hop.GetRouteId() == d.route.GetId() && hop.GetHopIndex() == index {
			return hop
		}
	}
	return nil
}

func (d *diagnosis) configStep(index uint32, ref string) {
	step := &forwardv1.DiagnoseStep{NodeRef: ref, HopIndex: index, Kind: forwardv1.ProbeKind_PROBE_KIND_CONFIG, Vantage: vantageControl}
	state, report := d.states[ref], d.reports[ref]
	if report != nil {
		defer func() { step.Result.ObservedAtUnixMs = d.observed[ref].UnixMilli() }()
	}
	switch {
	case d.nodeHop(ref, index) == nil:
		step.Result = result(statusFailed, CodeNotPlanned, "the node's desired state does not hold this hop: the route does not plan on the node (see GET /api/v4/forward/nodes/"+ref+")")
	case report == nil:
		step.Result = result(statusFailed, CodeNeverReported, fmt.Sprintf("the node never reported: Control wants generation %d", state.GetGeneration()))
	case report.GetGeneration() < state.GetGeneration():
		step.Result = result(statusFailed, CodeNotApplied, fmt.Sprintf("the node runs generation %d, Control wants %d", report.GetGeneration(), state.GetGeneration()))
	case hopError(report, d.route.GetId(), index) != "":
		step.Result = result(statusFailed, CodeHopError, "the node could not apply the hop: "+hopError(report, d.route.GetId(), index))
	case !report.GetApplied():
		step.Result = result(statusFailed, CodeApplyFailed, fmt.Sprintf("the node did not apply generation %d", report.GetGeneration()))
	default:
		step.Result = result(statusOK, "", fmt.Sprintf("generation %d applied", report.GetGeneration()))
	}
	d.add(step)
}

func hopError(report *forwardv1.NodeForwardReport, routeID string, index uint32) string {
	for _, hopErr := range report.GetErrors() {
		if hopErr.GetRouteId() == routeID && hopErr.GetHopIndex() == index {
			return hopErr.GetMessage()
		}
	}
	return ""
}

func (d *diagnosis) healthSteps(index uint32, ref string) {
	hop := d.nodeHop(ref, index)
	if hop == nil {
		return
	}
	report := d.reports[ref]
	for _, upstream := range hop.GetUpstreams() {
		target := hostPort(upstream.GetAddress(), upstream.GetPort())
		step := &forwardv1.DiagnoseStep{NodeRef: ref, HopIndex: index, Kind: forwardv1.ProbeKind_PROBE_KIND_HEALTH, Vantage: vantageControl, Target: target}
		var health *forwardv1.UpstreamHealth
		for _, h := range report.GetHealth() {
			if h.GetRouteId() == d.route.GetId() && h.GetHopIndex() == index && h.GetAddress() == upstream.GetAddress() && h.GetPort() == upstream.GetPort() {
				health = h
				break
			}
		}
		switch {
		case health == nil && hop.GetHealth().GetDisabled():
			step.Result = result(statusInconclusive, CodeNoHealth, "active health checks are disabled on this hop")
		case health == nil:
			step.Result = result(statusInconclusive, CodeNoHealth, "the node's latest report has no health for this upstream")
		case health.GetState() == forwardv1.HealthState_HEALTH_STATE_HEALTHY:
			step.Result = result(statusOK, CodeHealthy, "healthy")
		case health.GetState() == forwardv1.HealthState_HEALTH_STATE_CIRCUIT_OPEN:
			step.Result = result(statusFailed, CodeCircuitOpen, fmt.Sprintf("circuit breaker open after %d failures in a row, until %s",
				health.GetConsecutiveFailures(), time.UnixMilli(health.GetCircuitOpenUntilUnixMs()).UTC().Format(time.RFC3339)))
		case health.GetState() == forwardv1.HealthState_HEALTH_STATE_UNHEALTHY:
			step.Result = result(statusFailed, CodeUnhealthy, fmt.Sprintf("unhealthy: %d failures in a row", health.GetConsecutiveFailures()))
		default:
			step.Result = result(statusInconclusive, CodeNoHealth, "health state not reported")
		}
		if health != nil {
			step.Result.RttUs = health.GetRttUs()
			step.Result.ObservedAtUnixMs = health.GetCheckedAtUnixMs()
		}
		d.add(step)
	}
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

// targetRefusal answers why the policy refuses a literal target, "" when
// it does not or the host is a name (the Agent checks what names resolve
// to).
func targetRefusal(host string, policy model.TargetPolicy) string {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		if isLoopbackName(host) {
			return validate.ErrLoopbackTarget.Error()
		}
		return ""
	}
	if err := validate.CheckTargetAddress(addr, policy); err != nil {
		return err.Error()
	}
	return ""
}

func isLoopbackName(host string) bool {
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	return name == "localhost" || strings.HasSuffix(name, ".localhost")
}

// probe is one forward check to run on a node.
type probe struct {
	ref      string
	node     agentcontrol.AgentNode
	index    uint32
	kind     forwardv1.ProbeKind
	action   string
	protocol forwardv1.L4Protocol
	params   map[string]any
	// steps receives the check's steps, in order.
	steps []*forwardv1.DiagnoseStep
}

// nodeProbes runs stage 2, and marks the node steps of nodes that cannot
// probe as SKIPPED.
func (d *diagnosis) nodeProbes(ctx context.Context) {
	tcp, udp := routeProtocols(d.route)
	last := len(d.route.GetHops()) - 1
	policy := effectiveTargetPolicy(d.route)
	policyParam := service.ForwardTargetPolicyPublicOnly
	if policy == model.TargetPolicyAllowPrivate {
		policyParam = service.ForwardTargetPolicyAllowPrivate
	}
	var probes []*probe
	for i, hop := range d.route.GetHops() {
		index := uint32(i) // #nosec G115 -- validate bounds hops to MaxHops.
		for _, ref := range uniqueRefs(hop.GetNodeRefs()) {
			v := d.vantage[ref]
			type planned struct {
				kind     forwardv1.ProbeKind
				action   string
				protocol forwardv1.L4Protocol
			}
			plan := []planned{
				{forwardv1.ProbeKind_PROBE_KIND_LISTEN, service.ForwardCheckListen, 0},
				{forwardv1.ProbeKind_PROBE_KIND_PORT_CONFLICT, service.ForwardCheckPortConflict, 0},
			}
			connectKind, udpKind := forwardv1.ProbeKind_PROBE_KIND_TCP_CONNECT, forwardv1.ProbeKind_PROBE_KIND_UDP_EXCHANGE
			if i == last {
				connectKind, udpKind = forwardv1.ProbeKind_PROBE_KIND_DELIVERY, forwardv1.ProbeKind_PROBE_KIND_DELIVERY
			}
			if tcp {
				plan = append(plan, planned{connectKind, service.ForwardCheckConnect, protoTCP})
			}
			if udp {
				plan = append(plan, planned{udpKind, service.ForwardCheckUDPProbe, protoUDP})
			}
			node, _ := agentcontrol.ParseAgentNode(ref)
			for _, p := range plan {
				if !v.Capable {
					code, message := CodeNodeVantageUnavailable, "the node's Agent does not run forward checks (agent.diagnostic with diag.v1)"
					if !v.Connected && d.s.Checks != nil {
						code, message = CodeNodeOffline, "the node's Agent is not connected"
					}
					if v.Note != "" {
						message += ": " + v.Note
					}
					d.add(&forwardv1.DiagnoseStep{NodeRef: ref, HopIndex: index, Kind: p.kind, Vantage: vantageNode, Protocol: p.protocol,
						Result: result(statusSkipped, code, message)})
					continue
				}
				timeout := diagnoseCheckTimeout
				if p.action == service.ForwardCheckUDPProbe {
					timeout = diagnoseUDPTimeout
				}
				params := map[string]any{"route_id": d.route.GetId(), "hop_index": int64(index), "timeout_ms": timeout.Milliseconds()}
				if state := d.states[ref]; state != nil {
					params["generation"] = int64(min(state.GetGeneration(), 1<<53)) // #nosec G115 -- bounded above.
				}
				if p.action == service.ForwardCheckConnect || p.action == service.ForwardCheckUDPProbe {
					params["target_policy"] = policyParam
				}
				if i == last && p.protocol != 0 {
					// A last hop dials the targets: refuse the literal ones the
					// policy refuses here, and narrow the check to the others.
					var allowed []string
					for _, target := range d.route.GetTargets() {
						address := hostPort(target.GetHost(), target.GetPort())
						if reason := targetRefusal(target.GetHost(), policy); reason != "" {
							d.add(&forwardv1.DiagnoseStep{NodeRef: ref, HopIndex: index, Kind: p.kind, Vantage: vantageNode, Target: address, Protocol: p.protocol,
								Result: result(statusSkipped, CodeTargetNotAllowed, "not probed: "+reason)})
							continue
						}
						allowed = append(allowed, address)
					}
					if len(allowed) < len(d.route.GetTargets()) {
						for _, address := range allowed {
							narrowed := cloneParams(params)
							narrowed["upstream"] = address
							probes = append(probes, &probe{ref: ref, node: node, index: index, kind: p.kind, action: p.action, protocol: p.protocol, params: narrowed})
						}
						continue
					}
				}
				probes = append(probes, &probe{ref: ref, node: node, index: index, kind: p.kind, action: p.action, protocol: p.protocol, params: params})
			}
		}
	}
	if len(probes) == 0 {
		return
	}
	// An Agent runs its operations one at a time, in order: each node's
	// checks go out in sequence, the nodes in parallel.
	byNode := map[string][]*probe{}
	var order []string
	for _, p := range probes {
		if _, ok := byNode[p.ref]; !ok {
			order = append(order, p.ref)
		}
		byNode[p.ref] = append(byNode[p.ref], p)
	}
	slots := make(chan struct{}, diagnoseProbeConcurrency)
	var wg sync.WaitGroup
	for _, ref := range order {
		wg.Add(1)
		go func(queue []*probe) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
			}
			for _, p := range queue {
				p.steps = d.runCheck(ctx, p)
			}
		}(byNode[ref])
	}
	wg.Wait()
	for _, p := range probes {
		for _, step := range p.steps {
			d.add(step)
		}
	}
}

func cloneParams(params map[string]any) map[string]any {
	out := make(map[string]any, len(params)+1)
	for key, value := range params {
		out[key] = value
	}
	return out
}

// runCheck runs one forward check and converts the Agent's answer to steps.
func (d *diagnosis) runCheck(ctx context.Context, p *probe) []*forwardv1.DiagnoseStep {
	base := func(target string) *forwardv1.DiagnoseStep {
		return &forwardv1.DiagnoseStep{NodeRef: p.ref, HopIndex: p.index, Kind: p.kind, Vantage: vantageNode, Target: target, Protocol: p.protocol}
	}
	upstream, _ := p.params["upstream"].(string)
	if ctx.Err() != nil {
		step := base(upstream)
		step.Result = result(statusSkipped, CodeDeadline, "the diagnosis ran out of time before this check")
		return []*forwardv1.DiagnoseStep{step}
	}
	timeout := time.Duration(p.params["timeout_ms"].(int64)) * time.Millisecond
	checkCtx, cancel := context.WithTimeout(ctx, timeout+diagnoseCheckGrace)
	defer cancel()
	answer, err := d.s.Checks.Check(checkCtx, p.node, p.action, p.params)
	if err != nil {
		step := base(upstream)
		switch {
		case errors.Is(err, ErrCheckUnavailable):
			step.Result = result(statusSkipped, CodeNodeVantageUnavailable, err.Error())
		case errors.Is(err, context.DeadlineExceeded) || checkCtx.Err() != nil:
			step.Result = result(statusInconclusive, CodeAgentTimeout, "the Agent did not answer the check in time")
		default:
			step.Result = result(statusInconclusive, CodeAgentError, "the Agent failed the check: "+err.Error())
		}
		return []*forwardv1.DiagnoseStep{step}
	}
	if len(answer.Items) == 0 {
		step := base(upstream)
		step.Result = checkResult(answer.Status, answer.Code, answer.Message, 0)
		step.Result.ProbeId = answer.OperationID
		return []*forwardv1.DiagnoseStep{step}
	}
	steps := make([]*forwardv1.DiagnoseStep, 0, len(answer.Items))
	for _, item := range answer.Items {
		step := base(item.Target)
		if item.Protocol == "udp" {
			step.Protocol = protoUDP
		} else if item.Protocol == "tcp" {
			step.Protocol = protoTCP
		}
		message := item.Message
		if message == "" {
			message = answer.Message
		}
		step.Result = checkResult(item.Status, item.Code, message, item.RTTMicro)
		step.Result.ProbeId = answer.OperationID
		steps = append(steps, step)
	}
	return steps
}

// checkResult converts an Agent's verdict; an unknown one is
// INCONCLUSIVE.
func checkResult(status, code, message string, rtt uint32) *forwardv1.ProbeResult {
	verdict := statusInconclusive
	switch status {
	case CheckOK:
		verdict = statusOK
	case CheckFailed:
		verdict = statusFailed
	case CheckSkipped:
		verdict = statusSkipped
	}
	out := result(verdict, code, message)
	out.RttUs = rtt
	return out
}

// controlProbes runs stage 3: dials from Control for the entries and the
// last hop's targets that no node probes.
func (d *diagnosis) controlProbes(ctx context.Context) {
	hops := d.route.GetHops()
	if len(hops) == 0 {
		return
	}
	type dial struct {
		ref, address string
		index        uint32
		kind         forwardv1.ProbeKind
		step         *forwardv1.DiagnoseStep
	}
	var dials []*dial
	tcp, _ := routeProtocols(d.route)
	port := d.route.GetListen().GetPort()
	for _, ref := range uniqueRefs(hops[0].GetNodeRefs()) {
		if d.vantage[ref].Capable {
			continue
		}
		host := d.route.GetListen().GetAddress()
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = ""
			if hop := d.nodeHop(ref, 0); hop != nil && hop.GetListen().GetPort() != 0 {
				port = hop.GetListen().GetPort()
			}
			if addresses := d.infos[ref].GetAddresses(); len(addresses) > 0 {
				host = addresses[0]
			}
		}
		if host == "" || port == 0 {
			continue
		}
		address := hostPort(host, port)
		if !tcp {
			d.add(&forwardv1.DiagnoseStep{NodeRef: ref, Kind: forwardv1.ProbeKind_PROBE_KIND_LISTEN, Vantage: vantageControl, Target: address, Protocol: protoUDP,
				Result: result(statusSkipped, CodeControlUDP, "Control does not probe a UDP listener; the node's Agent does")})
			continue
		}
		dials = append(dials, &dial{ref: ref, address: address, kind: forwardv1.ProbeKind_PROBE_KIND_LISTEN})
	}
	last := uint32(len(hops) - 1) // #nosec G115 -- validate bounds hops to MaxHops.
	needTargets := false
	for _, ref := range uniqueRefs(hops[last].GetNodeRefs()) {
		if !d.vantage[ref].Capable {
			needTargets = true
		}
	}
	if needTargets && tcp {
		for _, target := range d.route.GetTargets() {
			dials = append(dials, &dial{address: hostPort(target.GetHost(), target.GetPort()), index: last, kind: forwardv1.ProbeKind_PROBE_KIND_TCP_CONNECT})
		}
	}
	if len(dials) == 0 {
		return
	}
	slots := make(chan struct{}, diagnoseProbeConcurrency)
	var wg sync.WaitGroup
	for _, x := range dials {
		wg.Add(1)
		go func(x *dial) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
			}
			x.step = &forwardv1.DiagnoseStep{NodeRef: x.ref, HopIndex: x.index, Kind: x.kind, Vantage: vantageControl, Target: x.address, Protocol: protoTCP}
			x.step.Result = d.controlDial(ctx, x.address)
		}(x)
	}
	wg.Wait()
	for _, x := range dials {
		d.add(x.step)
	}
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
