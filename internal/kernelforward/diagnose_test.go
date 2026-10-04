package kernelforward

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeChecks is a NodeChecker: per node, its vantage and how it answers.
type fakeChecks struct {
	mu       sync.Mutex
	vantage  map[string]NodeVantage
	answer   func(ctx context.Context, node agentcontrol.AgentNode, action string, params map[string]any) (*CheckResult, error)
	requests []fakeCheck
}

type fakeCheck struct {
	node   string
	action string
	params map[string]any
}

func (c *fakeChecks) Vantage(node agentcontrol.AgentNode) NodeVantage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.vantage[node.String()]
}

func (c *fakeChecks) Check(ctx context.Context, node agentcontrol.AgentNode, action string, params map[string]any) (*CheckResult, error) {
	c.mu.Lock()
	c.requests = append(c.requests, fakeCheck{node: node.String(), action: action, params: params})
	answer := c.answer
	c.mu.Unlock()
	return answer(ctx, node, action, params)
}

func (c *fakeChecks) calls() []fakeCheck {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]fakeCheck(nil), c.requests...)
}

// allOK answers every check with one OK item per upstream (or the hop's
// listener).
func allOK(_ context.Context, _ agentcontrol.AgentNode, action string, params map[string]any) (*CheckResult, error) {
	target := "listener"
	if upstream, ok := params["upstream"].(string); ok {
		target = upstream
	}
	return &CheckResult{Check: action, Status: CheckOK, Items: []CheckItem{{Target: target, Protocol: "tcp", Status: CheckOK, RTTMicro: 700}}}, nil
}

func capable(refs ...string) map[string]NodeVantage {
	out := map[string]NodeVantage{}
	for _, ref := range refs {
		out[ref] = NodeVantage{Connected: true, Capable: true}
	}
	return out
}

// steps answers the steps of a kind, optionally of one node.
func steps(answer *forwardv1.DiagnoseRouteResponse, kind forwardv1.ProbeKind, node string) []*forwardv1.DiagnoseStep {
	var out []*forwardv1.DiagnoseStep
	for _, step := range answer.GetSteps() {
		if step.GetKind() == kind && (node == "*" || step.GetNodeRef() == node) {
			out = append(out, step)
		}
	}
	return out
}

func (f *fixture) applied(node agentcontrol.AgentNode, at time.Time, health ...*forwardv1.UpstreamHealth) {
	f.t.Helper()
	state := f.state(node)
	_, err := f.service.RecordReport(f.ctx, node, &forwardv1.NodeForwardReport{
		NodeRef: node.String(), Generation: state.GetGeneration(), StateHash: state.GetStateHash(), Applied: true, Health: health,
	}, at, at)
	require.NoError(f.t, err)
}

func (f *fixture) diagnose(routeID string, timeout time.Duration) *forwardv1.DiagnoseRouteResponse {
	f.t.Helper()
	answer, err := f.service.DiagnoseRoute(f.ctx, routeID, timeout)
	require.NoError(f.t, err)
	return answer
}

// Without node probes: Control's records, node steps SKIPPED and clearly
// marked, and Control's own dials of the entry and the public target.
func TestDiagnoseControlOnly(t *testing.T)         { runDiagnoseControlOnly(t, openSQLite(t)) }
func TestPostgresDiagnoseControlOnly(t *testing.T) { runDiagnoseControlOnly(t, openPostgres(t)) }

func runDiagnoseControlOnly(t *testing.T, db *gorm.DB) {
	resetDiagnosesForTest()
	f := newFixture(t, db)
	route := f.create("c1", twoHop(31000))
	var dialled []string
	var mu sync.Mutex
	f.service.Probes.Dial = func(_ context.Context, network, address string, _ time.Duration) (net.Conn, error) {
		mu.Lock()
		dialled = append(dialled, network+" "+address)
		mu.Unlock()
		if address == "192.0.2.11:31000" {
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		}
		return nil, errors.New("connection refused")
	}

	answer := f.diagnose(route.GetId(), 0)
	assert.False(t, answer.GetOk())
	assert.Equal(t, route.GetId(), answer.GetRouteId())
	assert.False(t, answer.GetCached())
	require.Len(t, answer.GetNodes(), 2)
	for _, node := range answer.GetNodes() {
		assert.False(t, node.GetNodeVantage())
		assert.Contains(t, node.GetNote(), "no Agent sessions")
	}
	config := steps(answer, forwardv1.ProbeKind_PROBE_KIND_CONFIG, "*")
	require.Len(t, config, 2)
	for _, step := range config {
		assert.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_FAILED, step.GetResult().GetStatus())
		assert.Equal(t, CodeNeverReported, step.GetResult().GetCode())
		assert.Equal(t, forwardv1.DiagnoseVantage_DIAGNOSE_VANTAGE_CONTROL, step.GetVantage())
	}
	health := steps(answer, forwardv1.ProbeKind_PROBE_KIND_HEALTH, "*")
	require.Len(t, health, 2, "one upstream per hop")
	for _, step := range health {
		assert.Equal(t, CodeNoHealth, step.GetResult().GetCode())
		assert.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_INCONCLUSIVE, step.GetResult().GetStatus())
	}
	for _, kind := range []forwardv1.ProbeKind{forwardv1.ProbeKind_PROBE_KIND_PORT_CONFLICT, forwardv1.ProbeKind_PROBE_KIND_DELIVERY} {
		for _, step := range steps(answer, kind, "*") {
			assert.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_SKIPPED, step.GetResult().GetStatus(), kind)
			assert.Equal(t, CodeNodeVantageUnavailable, step.GetResult().GetCode())
			assert.Equal(t, forwardv1.DiagnoseVantage_DIAGNOSE_VANTAGE_NODE, step.GetVantage())
		}
	}
	// Control's dials: the entry listener (reachable) and the target.
	var listen, connect *forwardv1.DiagnoseStep
	for _, step := range answer.GetSteps() {
		if step.GetVantage() != forwardv1.DiagnoseVantage_DIAGNOSE_VANTAGE_CONTROL {
			continue
		}
		switch step.GetKind() {
		case forwardv1.ProbeKind_PROBE_KIND_LISTEN:
			listen = step
		case forwardv1.ProbeKind_PROBE_KIND_TCP_CONNECT:
			connect = step
		}
	}
	require.NotNil(t, listen)
	assert.Equal(t, "forward-11", listen.GetNodeRef())
	assert.Equal(t, "192.0.2.11:31000", listen.GetTarget())
	assert.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_OK, listen.GetResult().GetStatus())
	assert.True(t, listen.GetResult().GetOk())
	require.NotNil(t, connect)
	assert.Empty(t, connect.GetNodeRef())
	assert.EqualValues(t, 1, connect.GetHopIndex())
	assert.Equal(t, "198.51.100.10:443", connect.GetTarget())
	assert.Equal(t, CodeUnreachable, connect.GetResult().GetCode())
	assert.ElementsMatch(t, []string{"tcp 192.0.2.11:31000", "tcp 198.51.100.10:443"}, dialled, "Control never dials the exit's listener")

	// A diagnosis of the same route moments later is answered again.
	again := f.diagnose(route.GetId(), 0)
	assert.True(t, again.GetCached())
	assert.Len(t, again.GetSteps(), len(answer.GetSteps()))
	assert.Len(t, dialled, 2, "nothing dialled again")

	_, err := f.service.DiagnoseRoute(f.ctx, "01JF1A000000000000000000ZZ", 0)
	require.ErrorIs(t, err, ErrNotFound)
}

// With node probes on every node: the checks Control asks for, and their
// answers as NODE steps.
func TestDiagnoseNodeVantage(t *testing.T)         { runDiagnoseNodeVantage(t, openSQLite(t)) }
func TestPostgresDiagnoseNodeVantage(t *testing.T) { runDiagnoseNodeVantage(t, openPostgres(t)) }

func runDiagnoseNodeVantage(t *testing.T, db *gorm.DB) {
	resetDiagnosesForTest()
	f := newFixture(t, db)
	route := twoHop(31000)
	route.Listen.Protocol = forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP
	id := f.create("c1", route).GetId()
	at := start.Add(time.Minute)
	exitPort := f.state(exit).GetHops()[0].GetListen().GetPort()
	f.applied(entry, at, &forwardv1.UpstreamHealth{RouteId: id, Address: "192.0.2.12", Port: exitPort, State: forwardv1.HealthState_HEALTH_STATE_HEALTHY, RttUs: 800, CheckedAtUnixMs: at.UnixMilli()})
	f.applied(exit, at, &forwardv1.UpstreamHealth{RouteId: id, HopIndex: 1, Address: "198.51.100.10", Port: 443, State: forwardv1.HealthState_HEALTH_STATE_HEALTHY})
	checks := &fakeChecks{vantage: capable("forward-11", "forward-12"), answer: func(ctx context.Context, node agentcontrol.AgentNode, action string, params map[string]any) (*CheckResult, error) {
		if action == "forward.udp_probe" {
			return &CheckResult{Status: CheckInconclusive, Items: []CheckItem{{Target: "x:1", Protocol: "udp", Status: CheckInconclusive, Code: "no_reply", Message: "no reply"}}}, nil
		}
		return allOK(ctx, node, action, params)
	}}
	f.service.Checks = checks
	f.service.Probes.Dial = func(context.Context, string, string, time.Duration) (net.Conn, error) {
		t.Error("Control dials nothing when every node probes")
		return nil, errors.New("no")
	}

	answer := f.diagnose(id, 0)
	assert.True(t, answer.GetOk(), "inconclusive UDP does not fail the diagnosis")
	for _, node := range answer.GetNodes() {
		assert.True(t, node.GetNodeVantage())
		assert.True(t, node.GetConnected())
	}
	for _, step := range steps(answer, forwardv1.ProbeKind_PROBE_KIND_CONFIG, "*") {
		assert.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_OK, step.GetResult().GetStatus())
		assert.Equal(t, at.UnixMilli(), step.GetResult().GetObservedAtUnixMs())
	}
	healthy := steps(answer, forwardv1.ProbeKind_PROBE_KIND_HEALTH, "forward-11")
	require.Len(t, healthy, 1)
	assert.EqualValues(t, 800, healthy[0].GetResult().GetRttUs())
	assert.Equal(t, CodeHealthy, healthy[0].GetResult().GetCode())

	calls := checks.calls()
	actions := map[string][]string{}
	for _, call := range calls {
		actions[call.node] = append(actions[call.node], call.action)
		assert.Equal(t, id, call.params["route_id"])
		assert.NotZero(t, call.params["generation"])
		assert.NotZero(t, call.params["timeout_ms"])
	}
	want := []string{"forward.listen", "forward.port_conflict", "forward.connect", "forward.udp_probe"}
	assert.Equal(t, want, actions["forward-11"], "a node's checks go out in order")
	assert.Equal(t, want, actions["forward-12"])
	for _, call := range calls {
		if call.action == "forward.connect" || call.action == "forward.udp_probe" {
			assert.Equal(t, "public_only", call.params["target_policy"])
			assert.Nil(t, call.params["upstream"], "every target allowed: no narrowing")
		}
		if call.node == "forward-12" {
			assert.EqualValues(t, 1, call.params["hop_index"])
		}
	}
	// The entry's connect is TCP_CONNECT; the exit's is DELIVERY, with UDP.
	connect := steps(answer, forwardv1.ProbeKind_PROBE_KIND_TCP_CONNECT, "forward-11")
	require.Len(t, connect, 1)
	assert.Equal(t, forwardv1.DiagnoseVantage_DIAGNOSE_VANTAGE_NODE, connect[0].GetVantage())
	assert.EqualValues(t, 700, connect[0].GetResult().GetRttUs())
	exchange := steps(answer, forwardv1.ProbeKind_PROBE_KIND_UDP_EXCHANGE, "forward-11")
	require.Len(t, exchange, 1)
	assert.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_INCONCLUSIVE, exchange[0].GetResult().GetStatus())
	assert.Equal(t, "no_reply", exchange[0].GetResult().GetCode())
	assert.Equal(t, forwardv1.L4Protocol_L4_PROTOCOL_UDP, exchange[0].GetProtocol())
	delivery := steps(answer, forwardv1.ProbeKind_PROBE_KIND_DELIVERY, "forward-12")
	require.Len(t, delivery, 2, "TCP and UDP")
	assert.Equal(t, forwardv1.L4Protocol_L4_PROTOCOL_TCP, delivery[0].GetProtocol())

	// A failed check fails the diagnosis; the cache expires first.
	resetDiagnosesForTest()
	checks.answer = func(ctx context.Context, node agentcontrol.AgentNode, action string, params map[string]any) (*CheckResult, error) {
		if action == "forward.port_conflict" && node.ID == 12 {
			return &CheckResult{Status: CheckFailed, Code: "conflict", Message: "table ip nat chain PREROUTING rewrites destination port"}, nil
		}
		return allOK(ctx, node, action, params)
	}
	answer = f.diagnose(id, 0)
	assert.False(t, answer.GetOk())
	conflict := steps(answer, forwardv1.ProbeKind_PROBE_KIND_PORT_CONFLICT, "forward-12")
	require.Len(t, conflict, 1)
	assert.Equal(t, "conflict", conflict[0].GetResult().GetCode())
	assert.Contains(t, conflict[0].GetResult().GetMessage(), "PREROUTING")
}

// Faults Control sees in its records: a node behind its generation, a hop
// error, an open breaker, a paused route.
func TestDiagnoseConfigFaults(t *testing.T)         { runDiagnoseConfigFaults(t, openSQLite(t)) }
func TestPostgresDiagnoseConfigFaults(t *testing.T) { runDiagnoseConfigFaults(t, openPostgres(t)) }

func runDiagnoseConfigFaults(t *testing.T, db *gorm.DB) {
	resetDiagnosesForTest()
	f := newFixture(t, db)
	id := f.create("c1", twoHop(31000)).GetId()
	at := start.Add(time.Minute)
	state := f.state(entry)
	_, err := f.service.RecordReport(f.ctx, entry, &forwardv1.NodeForwardReport{
		NodeRef: "forward-11", Generation: state.GetGeneration() - 1, Applied: true,
	}, at, at)
	require.NoError(t, err)
	exitState := f.state(exit)
	_, err = f.service.RecordReport(f.ctx, exit, &forwardv1.NodeForwardReport{
		NodeRef: "forward-12", Generation: exitState.GetGeneration(), StateHash: exitState.GetStateHash(), Applied: false,
		Errors: []*forwardv1.HopError{{RouteId: id, HopIndex: 1, Engine: forwardv1.Engine_ENGINE_NFTABLES, Message: "nft: port in use"}},
		Health: []*forwardv1.UpstreamHealth{{RouteId: id, HopIndex: 1, Address: "198.51.100.10", Port: 443,
			State: forwardv1.HealthState_HEALTH_STATE_CIRCUIT_OPEN, ConsecutiveFailures: 3, CircuitOpenUntilUnixMs: at.Add(30 * time.Second).UnixMilli()}},
	}, at, at)
	require.NoError(t, err)

	answer := f.diagnose(id, 2*time.Second)
	assert.False(t, answer.GetOk())
	lag := steps(answer, forwardv1.ProbeKind_PROBE_KIND_CONFIG, "forward-11")
	require.Len(t, lag, 1)
	assert.Equal(t, CodeNotApplied, lag[0].GetResult().GetCode())
	hopErr := steps(answer, forwardv1.ProbeKind_PROBE_KIND_CONFIG, "forward-12")
	require.Len(t, hopErr, 1)
	assert.Equal(t, CodeHopError, hopErr[0].GetResult().GetCode())
	assert.Contains(t, hopErr[0].GetResult().GetMessage(), "port in use")
	breaker := steps(answer, forwardv1.ProbeKind_PROBE_KIND_HEALTH, "forward-12")
	require.Len(t, breaker, 1)
	assert.Equal(t, CodeCircuitOpen, breaker[0].GetResult().GetCode())
	assert.Equal(t, "198.51.100.10:443", breaker[0].GetTarget())

	// Paused: a route-wide step says so.
	resetDiagnosesForTest()
	stored, err := f.service.GetRoute(f.ctx, id)
	require.NoError(t, err)
	stored.Paused = true
	_, err = f.service.UpdateRoute(f.ctx, "u1", stored, stored.GetRevision())
	require.NoError(t, err)
	answer = f.diagnose(id, 2*time.Second)
	paused := answer.GetSteps()[0]
	assert.Equal(t, forwardv1.ProbeKind_PROBE_KIND_CONFIG, paused.GetKind())
	assert.Empty(t, paused.GetNodeRef())
	assert.Equal(t, CodeRoutePaused, paused.GetResult().GetCode())
}

// A node whose Agent cannot probe, or is offline, is marked; Control dials
// for it. A check that never answers is INCONCLUSIVE and the budget holds.
func TestDiagnoseIncapableAndTimeouts(t *testing.T) {
	resetDiagnosesForTest()
	f := newFixture(t, openSQLite(t))
	id := f.create("c1", twoHop(31000)).GetId()
	checks := &fakeChecks{
		vantage: map[string]NodeVantage{
			"forward-11": {Connected: true, Note: "the node's Agent does not advertise agent.diagnostic and diag.v1 (Agent 4.1.9)"},
			"forward-12": {},
		},
		answer: func(context.Context, agentcontrol.AgentNode, string, map[string]any) (*CheckResult, error) {
			t.Error("no check goes to a node that cannot run it")
			return nil, ErrCheckUnavailable
		},
	}
	f.service.Checks = checks
	answer := f.diagnose(id, 2*time.Second)
	assert.Empty(t, checks.calls())
	entrySteps := steps(answer, forwardv1.ProbeKind_PROBE_KIND_LISTEN, "forward-11")
	require.Len(t, entrySteps, 2, "the skipped node check and Control's dial")
	assert.Equal(t, CodeNodeVantageUnavailable, entrySteps[0].GetResult().GetCode())
	assert.Contains(t, entrySteps[0].GetResult().GetMessage(), "4.1.9")
	assert.Equal(t, forwardv1.DiagnoseVantage_DIAGNOSE_VANTAGE_CONTROL, entrySteps[1].GetVantage())
	assert.Equal(t, CodeUnreachable, entrySteps[1].GetResult().GetCode(), "refused by the test dialer")
	offline := steps(answer, forwardv1.ProbeKind_PROBE_KIND_LISTEN, "forward-12")
	require.Len(t, offline, 1, "Control never dials a relay or exit listener")
	assert.Equal(t, CodeNodeOffline, offline[0].GetResult().GetCode())
	nodes := answer.GetNodes()
	require.Len(t, nodes, 2)
	assert.True(t, nodes[0].GetConnected())
	assert.False(t, nodes[0].GetNodeVantage())
	assert.False(t, nodes[1].GetConnected())

	// Checks that hang until their deadline.
	resetDiagnosesForTest()
	checks.vantage = capable("forward-11", "forward-12")
	checks.answer = func(ctx context.Context, _ agentcontrol.AgentNode, _ string, _ map[string]any) (*CheckResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	began := time.Now()
	answer = f.diagnose(id, time.Second)
	assert.Less(t, time.Since(began), 3*time.Second, "the budget bounds the diagnosis")
	var timedOut, deadline int
	for _, step := range answer.GetSteps() {
		switch step.GetResult().GetCode() {
		case CodeAgentTimeout:
			timedOut++
			assert.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_INCONCLUSIVE, step.GetResult().GetStatus())
		case CodeDeadline:
			deadline++
			assert.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_SKIPPED, step.GetResult().GetStatus())
		}
	}
	assert.Equal(t, 2, timedOut, "the first check of each node")
	assert.Equal(t, 4, deadline, "the rest")

	// An Agent that fails a check.
	resetDiagnosesForTest()
	checks.answer = func(context.Context, agentcontrol.AgentNode, string, map[string]any) (*CheckResult, error) {
		return nil, errors.New("unsupported desired operation")
	}
	answer = f.diagnose(id, 2*time.Second)
	failed := steps(answer, forwardv1.ProbeKind_PROBE_KIND_LISTEN, "forward-11")
	require.Len(t, failed, 1)
	assert.Equal(t, CodeAgentError, failed[0].GetResult().GetCode())
}

// The public-targets rule: Control never dials a private target; a node
// is never asked to dial a literal target the route's policy refuses; an
// administrator's ALLOW_PRIVATE route lets the exit probe its own network.
func TestDiagnosePrivateTargets(t *testing.T) {
	resetDiagnosesForTest()
	f := newFixture(t, openSQLite(t))
	private := twoHop(31000)
	private.Policy = &forwardv1.Policy{TargetPolicy: forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE}
	private.Targets = []*forwardv1.Target{{Host: "10.0.0.5", Port: 22}}
	id := f.create("c1", private).GetId()
	checks := &fakeChecks{vantage: capable("forward-11"), answer: allOK}
	f.service.Checks = checks
	var dialled []string
	f.service.Probes.Dial = func(_ context.Context, _, address string, _ time.Duration) (net.Conn, error) {
		dialled = append(dialled, address)
		return nil, errors.New("refused")
	}
	answer := f.diagnose(id, 2*time.Second)
	assert.NotContains(t, dialled, "10.0.0.5:22")
	var refused *forwardv1.DiagnoseStep
	for _, step := range answer.GetSteps() {
		if step.GetVantage() == forwardv1.DiagnoseVantage_DIAGNOSE_VANTAGE_CONTROL && step.GetTarget() == "10.0.0.5:22" {
			refused = step
		}
	}
	require.NotNil(t, refused, "the exit cannot probe, so Control considers the target")
	assert.Equal(t, CodeNotPublic, refused.GetResult().GetCode())
	assert.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_SKIPPED, refused.GetResult().GetStatus())

	resetDiagnosesForTest()
	checks.vantage = capable("forward-11", "forward-12")
	answer = f.diagnose(id, 2*time.Second)
	for _, call := range checks.calls() {
		if call.node == "forward-12" && call.action == "forward.connect" {
			assert.Equal(t, "allow_private", call.params["target_policy"])
		}
	}
	assert.Len(t, steps(answer, forwardv1.ProbeKind_PROBE_KIND_DELIVERY, "forward-12"), 1)

	// A stored route whose policy no longer allows one of its targets (as
	// written before a policy change): that target is not probed, the
	// other is, alone.
	resetDiagnosesForTest()
	mixed := twoHop(31001)
	mixed.Targets = append(mixed.Targets, &forwardv1.Target{Host: "198.51.100.11", Port: 443})
	mixedID := f.create("c2", mixed).GetId()
	stored, err := f.service.GetRoute(f.ctx, mixedID)
	require.NoError(t, err)
	stored.Targets[1].Host = "192.168.1.9"
	encoded, err := encodeRoute(stored)
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&model.KernelForwardRoute{}).Where("id = ?", mixedID).Update("route_json", encoded).Error)
	before := len(checks.calls())
	answer = f.diagnose(mixedID, 2*time.Second)
	var notAllowed int
	for _, step := range steps(answer, forwardv1.ProbeKind_PROBE_KIND_DELIVERY, "forward-12") {
		if step.GetResult().GetCode() == CodeTargetNotAllowed {
			notAllowed++
			assert.Equal(t, "192.168.1.9:443", step.GetTarget())
		}
	}
	assert.Equal(t, 1, notAllowed)
	for _, call := range checks.calls()[before:] {
		if call.node == "forward-12" && call.action == "forward.connect" {
			assert.Equal(t, "198.51.100.10:443", call.params["upstream"])
			assert.Equal(t, "public_only", call.params["target_policy"])
		}
	}
}

// Too many diagnoses at once are refused; a caller of a running diagnosis
// shares it.
func TestDiagnoseBusyAndShared(t *testing.T) {
	resetDiagnosesForTest()
	f := newFixture(t, openSQLite(t))
	id := f.create("c1", twoHop(31000)).GetId()
	diagnoses.running = MaxConcurrentDiagnoses
	_, err := f.service.DiagnoseRoute(f.ctx, id, 0)
	require.ErrorIs(t, err, ErrDiagnoseBusy)
	diagnoses.running = 0

	release := make(chan struct{})
	started := make(chan struct{}, 8)
	f.service.Checks = &fakeChecks{vantage: capable("forward-11", "forward-12"), answer: func(ctx context.Context, node agentcontrol.AgentNode, action string, params map[string]any) (*CheckResult, error) {
		started <- struct{}{}
		<-release
		return allOK(ctx, node, action, params)
	}}
	first := make(chan *forwardv1.DiagnoseRouteResponse, 1)
	go func() {
		answer, _ := f.service.DiagnoseRoute(context.Background(), id, 0)
		first <- answer
	}()
	<-started
	second := make(chan *forwardv1.DiagnoseRouteResponse, 1)
	go func() {
		answer, _ := f.service.DiagnoseRoute(context.Background(), id, 0)
		second <- answer
	}()
	time.Sleep(50 * time.Millisecond)
	close(release)
	a, b := <-first, <-second
	require.NotNil(t, a)
	require.NotNil(t, b)
	assert.False(t, a.GetCached())
	assert.True(t, b.GetCached(), "the second caller shares the running diagnosis")
	assert.Equal(t, a.GetStartedAtUnixMs(), b.GetStartedAtUnixMs())
}
