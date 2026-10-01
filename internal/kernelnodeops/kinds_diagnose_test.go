package kernelnodeops

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/gost"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gorm.io/gorm"
)

// fakeNetwork is the diagnoses' network: each address answers as behave
// says, and every dial is recorded.
type fakeNetwork struct {
	mu          sync.Mutex
	behave      map[string]string
	dialled     []string
	inFlight    int
	maxInFlight int
	started     chan string
	lookups     map[string][]netip.Addr
}

// Behaviours of a fake address: "ok" connects, "slow" connects after 30ms,
// "hang" waits for the probe's context to end, anything else is refused
// with that text (or the kernel's refusal text when empty).
func newFakeNetwork(behave map[string]string) *fakeNetwork {
	return &fakeNetwork{behave: behave, started: make(chan string, 256), lookups: map[string][]netip.Addr{}}
}

func (n *fakeNetwork) probes() service.DiagnosisProbes {
	return service.DiagnosisProbes{Dial: n.dial, Lookup: n.lookup}
}

func (n *fakeNetwork) dial(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
	n.mu.Lock()
	n.dialled = append(n.dialled, address)
	n.inFlight++
	n.maxInFlight = max(n.maxInFlight, n.inFlight)
	behaviour := n.behave[address]
	n.mu.Unlock()
	defer func() {
		n.mu.Lock()
		n.inFlight--
		n.mu.Unlock()
	}()
	select {
	case n.started <- address:
	default:
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("dial %s without a timeout", address)
	}
	switch behaviour {
	case "slow":
		select {
		case <-time.After(30 * time.Millisecond):
		case <-ctx.Done():
			return nil, &net.OpError{Op: "dial", Net: network, Err: ctx.Err()}
		}
		fallthrough
	case "ok":
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	case "hang":
		<-ctx.Done()
		return nil, &net.OpError{Op: "dial", Net: network, Err: ctx.Err()}
	case "":
		return nil, fmt.Errorf("dial tcp %s: connect: connection refused", address)
	}
	return nil, fmt.Errorf("%s", behaviour)
}

func (n *fakeNetwork) lookup(_ context.Context, host string) ([]netip.Addr, error) {
	if addrs, ok := n.lookups[host]; ok {
		return addrs, nil
	}
	return nil, fmt.Errorf("lookup %s: no such host", host)
}

func (n *fakeNetwork) dials() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.dialled...)
}

func (n *fakeNetwork) busy() (int, int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.inFlight, n.maxInFlight
}

// waitDial waits until address is dialled.
func (n *fakeNetwork) waitDial(t *testing.T, address string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case dialled := <-n.started:
			if dialled == address {
				return
			}
		case <-deadline:
			require.FailNow(t, "the address was not dialled", address)
		}
	}
}

// agents is a fake agent directory: the nodes whose agents advertise
// diag.v1.
type agents map[uint64]bool

func (a agents) Advertises(node *kernelnodeopsv1.NodeRef, name, version string) bool {
	return node.GetKind() == forwardKind && name == "diag" && version == "v1" && a[node.GetId()]
}

// diagnosisHarness is an engine serving the diagnose kinds on the fake
// network, on the seeded database with the system configuration table.
func diagnosisHarness(t *testing.T, db *gorm.DB, network *fakeNetwork, directory AgentDirectory, options ...func(*Engine)) *harness {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.SystemConfig{}))
	h := newHarness(t, db, options...)
	require.NoError(t, (&Diagnosis{Probes: network.probes(), Agents: directory}).Register(h.registry))
	return h
}

func submitWait(t *testing.T, client kernelnodeopsv1.KernelNodeOpsServer, requestID string, spec *kernelnodeopsv1.OperationSpec) *kernelnodeopsv1.Operation {
	t.Helper()
	response, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
		RequestId: requestID, Operation: spec, Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, WaitTimeoutMs: 20000,
	})
	require.NoError(t, err)
	return response.GetOperation()
}

func checkNodes(record bool, vantage kernelnodeopsv1.Vantage, ids ...uint64) *kernelnodeopsv1.OperationSpec {
	spec := checkEndpoints()
	for _, id := range ids {
		spec.GetCheckEndpoints().Nodes = append(spec.GetCheckEndpoints().Nodes, nodeRef(forwardKind, id))
	}
	spec.GetCheckEndpoints().RecordStatus = record
	spec.GetCheckEndpoints().Vantage = vantage
	return spec
}

func nodeStats(id uint64) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_CollectNodeStats{
		CollectNodeStats: &kernelnodeopsv1.CollectNodeStats{Node: nodeRef(forwardKind, id)},
	}}
}

func diagnoseForward(id uint64, vantage kernelnodeopsv1.Vantage) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_DiagnoseForward{
		DiagnoseForward: &kernelnodeopsv1.DiagnoseForward{ForwardId: id, Vantage: vantage},
	}}
}

func diagnoseTunnel(id uint64, vantage kernelnodeopsv1.Vantage) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_DiagnoseTunnel{
		DiagnoseTunnel: &kernelnodeopsv1.DiagnoseTunnel{TunnelId: id, Vantage: vantage},
	}}
}

func forwardNodeRow(t *testing.T, db *gorm.DB, id uint) model.ForwardNode {
	t.Helper()
	var node model.ForwardNode
	require.NoError(t, db.First(&node, id).Error)
	return node
}

// legacyOutcomes maps the legacy route's report into the contract's
// outcomes, without the probes' timings.
func legacyOutcomes(report []service.DiagnosisOutcome) []*kernelnodeopsv1.DiagnosisOutcome {
	outcomes := diagnosisOutcome(context.Background(), report, nil, nil).result.GetDiagnosis().GetOutcomes()
	for _, outcome := range outcomes {
		outcome.AverageTimeMs = 0
	}
	return outcomes
}

func withoutTimings(outcomes []*kernelnodeopsv1.DiagnosisOutcome) []*kernelnodeopsv1.DiagnosisOutcome {
	copied := make([]*kernelnodeopsv1.DiagnosisOutcome, 0, len(outcomes))
	for _, outcome := range outcomes {
		clone := proto.Clone(outcome).(*kernelnodeopsv1.DiagnosisOutcome)
		clone.AverageTimeMs = 0
		copied = append(copied, clone)
	}
	return copied
}

func requireProtoEqual[M proto.Message](t *testing.T, expected, actual []M) {
	t.Helper()
	require.Len(t, actual, len(expected))
	for i := range expected {
		require.True(t, proto.Equal(expected[i], actual[i]), "item %d:\nexpected %v\nactual   %v", i, expected[i], actual[i])
	}
}

// The kernel serves the four diagnosis kinds, and GetCapabilities lists
// them.
func TestTheKernelServesTheDiagnoseKinds(t *testing.T) {
	diagnosis := []string{KindDiagnoseEndpoints, KindDiagnoseForward, KindDiagnoseNodeStats, KindDiagnoseTunnel}
	require.Subset(t, DefaultExecutors.Kinds(), diagnosis)
	for _, kindName := range diagnosis {
		require.Equal(t, kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_DIAGNOSE, FamilyOfKind(kindName))
	}
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		engine := &Engine{DB: db, Executors: DefaultExecutors}
		capabilities, err := (&Server{Engine: engine, Authorizer: allow(service.CapabilityNodeOpsDiagnose)}).For(forwardHost).
			GetCapabilities(context.Background(), &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.NoError(t, err)
		require.Subset(t, capabilities.GetKinds(), diagnosis)
		require.Equal(t, []kernelnodeopsv1.OperationFamily{kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_DIAGNOSE}, capabilities.GetGrantedFamilies())

		h := newHarness(t, db)
		require.NoError(t, (&Diagnosis{}).Register(h.registry))
		require.Error(t, (&Diagnosis{}).Register(h.registry), "a kind is served once")
	})
}

// CheckEndpoints dials each node's service port, as the forward node check
// does, and records what it found with record_status. A node deleted since
// the submission fails the operation (TARGET_GONE) with the other checks.
func TestCheckEndpoints(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		network := newFakeNetwork(map[string]string{"198.51.100.10:22": "ok", "198.51.100.12:22": "ok"})
		h := diagnosisHarness(t, db, network, nil)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))

		operation := submitWait(t, client, "diagnose.endpoints:1", checkNodes(true, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED, 10, 11, 12))
		require.Equal(t, succeeded, operation.GetState(), "%v", operation.GetError())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_CONTROL_DIAL, operation.GetChannel())
		checks := operation.GetResult().GetEndpointCheck().GetChecks()
		require.Len(t, checks, 3)
		for i, id := range []uint64{10, 11, 12} {
			require.Equal(t, id, checks[i].GetNode().GetId())
			require.Equal(t, forwardKind, checks[i].GetNode().GetKind())
			require.NotZero(t, checks[i].GetCheckedAtUnixMs())
			require.True(t, proto.Equal(&kernelnodeopsv1.VantageReport{
				Selected: kernelnodeopsv1.Vantage_VANTAGE_CONTROL, Used: kernelnodeopsv1.Vantage_VANTAGE_CONTROL,
			}, checks[i].GetVantage()), "%v", checks[i].GetVantage())
		}
		require.True(t, checks[0].GetReachable())
		require.Empty(t, checks[0].GetError())
		require.False(t, checks[1].GetReachable())
		require.Equal(t, "dial tcp 198.51.100.11:22: connect: connection refused", checks[1].GetError())
		require.True(t, checks[2].GetReachable())

		// What the legacy check records, and answers.
		require.Equal(t, model.ForwardNodeStatusOnline, forwardNodeRow(t, db, 10).Status)
		require.False(t, forwardNodeRow(t, db, 10).LastCheck.IsZero())
		require.Equal(t, model.ForwardNodeStatusOffline, forwardNodeRow(t, db, 11).Status)
		legacy, err := service.NewForwardNodeService(db).CheckEndpoint(context.Background(), network.probes(), 11, false)
		require.NoError(t, err)
		require.Equal(t, legacy.Error, checks[1].GetError())
		require.Equal(t, legacy.Status == model.ForwardNodeStatusOnline, checks[1].GetReachable())

		// Without record_status the rows are left alone.
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 11).Updates(map[string]any{"status": model.ForwardNodeStatusOnline, "latency": 7}).Error)
		operation = submitWait(t, client, "diagnose.endpoints:2", checkNodes(false, kernelnodeopsv1.Vantage_VANTAGE_CONTROL, 11))
		require.Equal(t, succeeded, operation.GetState())
		require.False(t, operation.GetResult().GetEndpointCheck().GetChecks()[0].GetReachable())
		require.Equal(t, model.ForwardNodeStatusOnline, forwardNodeRow(t, db, 11).Status)
		require.Equal(t, 7, forwardNodeRow(t, db, 11).Latency)
	})
}

// A node deleted after the submission is TARGET_GONE; the operation keeps
// the checks of the other nodes (a partial failure).
func TestCheckEndpointsOfANodeGoneSinceTheSubmission(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		network := newFakeNetwork(map[string]string{"198.51.100.10:22": "ok"})
		h := diagnosisHarness(t, db, network, nil)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))
		submitted := submit(t, client, "diagnose.endpoints:gone", checkNodes(true, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED, 10, 12, 11))
		require.NoError(t, db.Delete(&model.ForwardNode{}, 12).Error)
		h.start(t)

		operation := eventually(t, get(t, client, submitted.GetOperation().GetOperationId()), inState(failed))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, operation.GetError().GetCode())
		require.Equal(t, "forward nodes not found: 12", operation.GetError().GetMessage())
		require.False(t, operation.GetError().GetRetryable())
		checks := operation.GetResult().GetEndpointCheck().GetChecks()
		require.Len(t, checks, 3)
		require.True(t, checks[0].GetReachable())
		require.Equal(t, "forward node 12 not found", checks[1].GetError())
		require.False(t, checks[2].GetReachable())
		require.Equal(t, model.ForwardNodeStatusOnline, forwardNodeRow(t, db, 10).Status, "the checks that ran are recorded")
	})
}

// A check past its deadline ends TIMED_OUT, promptly: the dials see their
// context end, and nothing is recorded.
func TestCheckEndpointsStopAtTheDeadline(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		network := newFakeNetwork(map[string]string{"198.51.100.10:22": "hang", "198.51.100.11:22": "ok"})
		h := diagnosisHarness(t, db, network, nil, func(e *Engine) { e.Timeout = 400 * time.Millisecond })
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))

		began := time.Now()
		operation := submitWait(t, client, "diagnose.endpoints:deadline", checkNodes(true, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED, 10, 11))
		require.Equal(t, timedOut, operation.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED, operation.GetError().GetCode())
		require.Less(t, time.Since(began), 5*time.Second)
		require.Eventually(t, func() bool { inFlight, _ := network.busy(); return inFlight == 0 }, 5*time.Second, 10*time.Millisecond)
		require.True(t, forwardNodeRow(t, db, 10).LastCheck.IsZero(), "a check that timed out records nothing")
		require.True(t, forwardNodeRow(t, db, 11).LastCheck.IsZero(), "a check that timed out records nothing")
	})
}

// A cancelled check ends CANCELLED promptly: its dials see their context
// end, and nothing is recorded.
func TestCheckEndpointsStopOnCancellation(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		network := newFakeNetwork(map[string]string{"198.51.100.10:22": "hang"})
		h := diagnosisHarness(t, db, network, nil)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))

		response, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "diagnose.endpoints:cancel", Operation: checkNodes(true, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED, 10),
			Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED,
		})
		require.NoError(t, err)
		require.Equal(t, running, response.GetOperation().GetState())
		network.waitDial(t, "198.51.100.10:22")
		began := time.Now()
		stopped, err := client.CancelOperation(context.Background(), &kernelnodeopsv1.CancelOperationRequest{
			OperationId: response.GetOperation().GetOperationId(), Reason: "the administrator left",
		})
		require.NoError(t, err)
		require.Equal(t, cancelled, stopped.GetOperation().GetState())
		require.Less(t, time.Since(began), 2*time.Second)
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_CANCELLED, stopped.GetOperation().GetError().GetCode())
		require.Eventually(t, func() bool { inFlight, _ := network.busy(); return inFlight == 0 }, 5*time.Second, 10*time.Millisecond)
		require.True(t, forwardNodeRow(t, db, 10).LastCheck.IsZero(), "a cancelled check records nothing")
	})
}

// metricsServer serves a gost node's Prometheus metrics.
func metricsServer(t *testing.T, handler http.HandlerFunc) int {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.Listener.Addr().(*net.TCPAddr).Port
}

const gostMetrics = "# HELP gost_service_transfer_input_bytes_total in\n" +
	`gost_service_transfer_input_bytes_total{client="198.51.100.1",service="svc-b"} 100` + "\n" +
	`gost_service_transfer_input_bytes_total{client="198.51.100.2",service="svc-b"} 23` + "\n" +
	`gost_service_transfer_output_bytes_total{client="198.51.100.1",service="svc-b"} 7` + "\n" +
	`gost_service_transfer_input_bytes_total{client="198.51.100.1",service="svc-a"} 5` + "\n" +
	`gost_service_transfer_output_bytes_total{client="198.51.100.1",service="svc-a"} 6` + "\n"

// CollectNodeStats reads a gost node's metrics endpoint, or an Ansible
// machine's panel-side counters, as the statistics sync does.
func TestCollectNodeStats(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		port := metricsServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/metrics" || r.Header.Get("Authorization") != "" {
				http.Error(w, "unexpected request", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(gostMetrics))
		})
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 10).Updates(map[string]any{
			"host": "127.0.0.1", "api_port": 18080, "api_token": "relay-node-token", "metrics_port": port, "type": model.ForwardNodeTypeRelay,
		}).Error)
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 11).Updates(map[string]any{"api_port": 18080, "api_token": "exit-node-token", "type": model.ForwardNodeTypeExit}).Error)
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 12).Updates(map[string]any{
			"type": model.ForwardNodeTypeRelay, "tags": `["ansible-machine"]`, "current_conn": 3, "total_upload": 1024, "total_download": 2048,
		}).Error)
		network := newFakeNetwork(nil)
		h := diagnosisHarness(t, db, network, nil)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))

		operation := submitWait(t, client, "diagnose.node_stats:10", nodeStats(10))
		require.Equal(t, succeeded, operation.GetState(), "%v", operation.GetError())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_CONTROL_DIAL, operation.GetChannel())
		stats := operation.GetResult().GetNodeStats()
		require.EqualValues(t, 10, stats.GetNode().GetId())
		require.EqualValues(t, 128, stats.GetUploadBytes())
		require.EqualValues(t, 13, stats.GetDownloadBytes())
		require.NotZero(t, stats.GetCollectedAtUnixMs())
		requireProtoEqual(t, []*kernelnodeopsv1.ServiceTraffic{
			{Service: "svc-a", InputBytes: 5, OutputBytes: 6}, {Service: "svc-b", InputBytes: 123, OutputBytes: 7},
		}, stats.GetServices())
		node := forwardNodeRow(t, db, 10)
		legacy, err := service.CollectForwardNodeTraffic(context.Background(), gost.NewManager(db), &node, false)
		require.NoError(t, err)
		require.Len(t, legacy.Services, len(stats.GetServices()))
		for _, item := range stats.GetServices() {
			require.EqualValues(t, legacy.Services[item.GetService()].InBytes, item.GetInputBytes())
			require.EqualValues(t, legacy.Services[item.GetService()].OutBytes, item.GetOutputBytes())
		}

		operation = submitWait(t, client, "diagnose.node_stats:12", nodeStats(12))
		require.Equal(t, succeeded, operation.GetState(), "%v", operation.GetError())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_LOCAL_ANSIBLE, operation.GetChannel())
		require.True(t, proto.Equal(&kernelnodeopsv1.NodeStatsResult{
			Node: nodeRef(forwardKind, 12), UploadBytes: 1024, DownloadBytes: 2048, CurrentConnections: 3,
			CollectedAtUnixMs: operation.GetResult().GetNodeStats().GetCollectedAtUnixMs(),
		}, operation.GetResult().GetNodeStats()), "%v", operation.GetResult().GetNodeStats())

		operation = submitWait(t, client, "diagnose.node_stats:11", nodeStats(11))
		require.Equal(t, failed, operation.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, operation.GetError().GetCode())
		require.Equal(t, "metrics endpoint not configured for node 11", operation.GetError().GetMessage())
		require.False(t, operation.GetError().GetRetryable())
		require.Nil(t, operation.GetResult())
		require.Empty(t, network.dials(), "statistics dial no service port")
	})
}

// A metrics endpoint that fails is BACKEND_FAILED, with what it answered
// scrubbed; one that does not answer in time ends TIMED_OUT.
func TestCollectNodeStatsFailures(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		failing := metricsServer(t, func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "denied for relay-node-token; api_token=relay-node-token", http.StatusForbidden)
		})
		stuck := make(chan struct{})
		t.Cleanup(func() { close(stuck) })
		hanging := metricsServer(t, func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-stuck:
			}
		})
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 10).Updates(map[string]any{
			"host": "127.0.0.1", "api_port": 18080, "api_token": "relay-node-token", "metrics_port": failing, "type": model.ForwardNodeTypeExit,
		}).Error)
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 11).Updates(map[string]any{
			"host": "127.0.0.1", "api_port": 18080, "api_token": "exit-node-token", "metrics_port": hanging, "type": model.ForwardNodeTypeExit,
		}).Error)
		h := diagnosisHarness(t, db, newFakeNetwork(nil), nil, func(e *Engine) { e.Timeout = 500 * time.Millisecond })
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))

		operation := submitWait(t, client, "diagnose.node_stats:failing", nodeStats(10))
		require.Equal(t, failed, operation.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, operation.GetError().GetCode())
		require.True(t, operation.GetError().GetRetryable())
		require.Equal(t, "metrics API error: 403 Forbidden - denied for ********; api_token=********\n", operation.GetError().GetMessage())

		began := time.Now()
		operation = submitWait(t, client, "diagnose.node_stats:hanging", nodeStats(11))
		require.Equal(t, timedOut, operation.GetState())
		require.Less(t, time.Since(began), 5*time.Second)
	})
}

// prepareForward points forward 40 at targets and makes tunnel 30 a two-hop
// tunnel from node 10 to node 11.
func prepareForward(t *testing.T, db *gorm.DB, targets ...string) {
	t.Helper()
	require.NoError(t, db.Model(&model.Forward{}).Where("id = ?", 40).Update("remote_addr", strings.Join(targets, ",")).Error)
	require.NoError(t, db.Model(&model.ForwardTunnel{}).Where("id = ?", 30).Update("type", 2).Error)
}

// DiagnoseForward dials a forward's targets as the legacy route does for a
// user: private and loopback targets, and names that resolve to them, are
// refused without a dial (the SSRF guard of #84). The kernel cannot tell an
// administrator's diagnosis from a user's before it verifies request
// bindings, so it applies the user's rule to every forward.
func TestDiagnoseForwardDialsPublicTargetsOnly(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		prepareForward(t, db, "203.0.113.1:80", "203.0.113.2:80", " ", "127.0.0.1:22", "10.0.0.5:80", "localhost:22",
			"[::1]:22", "169.254.169.254:80", "100.64.0.1:80", "internal.example.test:80", "public.example.test:443", "no-port")
		network := newFakeNetwork(map[string]string{"203.0.113.1:80": "ok", "203.0.113.7:443": "ok"})
		network.lookups["internal.example.test"] = []netip.Addr{netip.MustParseAddr("10.0.0.9")}
		network.lookups["public.example.test"] = []netip.Addr{netip.MustParseAddr("203.0.113.7")}
		h := diagnosisHarness(t, db, network, nil)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))

		operation := submitWait(t, client, "diagnose.forward:40", diagnoseForward(40, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED))
		require.Equal(t, succeeded, operation.GetState(), "%v", operation.GetError())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_CONTROL_DIAL, operation.GetChannel())
		diagnosis := operation.GetResult().GetDiagnosis()
		refused := "不能诊断内网或本机地址"
		outcome := func(success bool, ip string, port int32, message string) *kernelnodeopsv1.DiagnosisOutcome {
			return &kernelnodeopsv1.DiagnosisOutcome{Success: success, Description: "转发->目标", NodeName: "tunnel", NodeId: "30", TargetIp: ip, TargetPort: port, Message: message}
		}
		requireProtoEqual(t, []*kernelnodeopsv1.DiagnosisOutcome{
			outcome(true, "203.0.113.1", 80, ""),
			outcome(false, "203.0.113.2", 80, "dial tcp 203.0.113.2:80: connect: connection refused"),
			outcome(false, "127.0.0.1", 22, refused),
			outcome(false, "10.0.0.5", 80, refused),
			outcome(false, "localhost", 22, refused),
			outcome(false, "::1", 22, refused),
			outcome(false, "169.254.169.254", 80, refused),
			outcome(false, "100.64.0.1", 80, refused),
			outcome(false, "internal.example.test", 80, refused),
			outcome(true, "public.example.test", 443, ""),
			outcome(false, "no-port", 0, "无法解析目标地址"),
		}, withoutTimings(diagnosis.GetOutcomes()))
		require.ElementsMatch(t, []string{"203.0.113.1:80", "203.0.113.2:80", "203.0.113.7:443"}, network.dials(),
			"only public addresses are dialled, and a name is dialled at the address checked")
		require.True(t, proto.Equal(&kernelnodeopsv1.VantageReport{
			Selected: kernelnodeopsv1.Vantage_VANTAGE_CONTROL, Used: kernelnodeopsv1.Vantage_VANTAGE_CONTROL,
		}, diagnosis.GetVantage()))

		// The legacy route computes the same for the forward's owner.
		legacy, err := service.NewPanelForwardService(db).DiagnoseForwardContext(context.Background(), network.probes(), 1, false, 40)
		require.NoError(t, err)
		requireProtoEqual(t, legacyOutcomes(legacy.Results), withoutTimings(diagnosis.GetOutcomes()))

		// A forward deleted since the submission is TARGET_GONE.
		submitted := submit(t, client, "diagnose.forward:41", diagnoseForward(41, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED))
		require.NoError(t, db.Delete(&model.Forward{}, 41).Error)
		gone := eventually(t, get(t, client, submitted.GetOperation().GetOperationId()), inState(failed))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, gone.GetError().GetCode())
		require.Equal(t, "转发不存在", gone.GetError().GetMessage())
	})
}

// DiagnoseTunnel dials a tunnel's entry and exit nodes, as the legacy route
// does; a cancelled diagnosis keeps the probes that completed.
func TestDiagnoseTunnel(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		prepareForward(t, db, "203.0.113.1:80")
		network := newFakeNetwork(map[string]string{"198.51.100.10:22": "ok"})
		h := diagnosisHarness(t, db, network, nil)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))

		operation := submitWait(t, client, "diagnose.tunnel:30", diagnoseTunnel(30, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED))
		require.Equal(t, succeeded, operation.GetState(), "%v", operation.GetError())
		requireProtoEqual(t, []*kernelnodeopsv1.DiagnosisOutcome{
			{Success: true, Description: "管理端->入口节点", NodeName: "relay", NodeId: "10", TargetIp: "198.51.100.10", TargetPort: 22},
			{Description: "管理端->出口节点", NodeName: "exit", NodeId: "11", TargetIp: "198.51.100.11", TargetPort: 22, Message: "dial tcp 198.51.100.11:22: connect: connection refused"},
		}, withoutTimings(operation.GetResult().GetDiagnosis().GetOutcomes()))
		legacy, err := service.NewPanelForwardService(db).DiagnoseTunnelContext(context.Background(), network.probes(), 30)
		require.NoError(t, err)
		requireProtoEqual(t, legacyOutcomes(legacy.Results), withoutTimings(operation.GetResult().GetDiagnosis().GetOutcomes()))

		// Cancelled while the entry node hangs: the exit node's probe
		// completed and is kept.
		network.mu.Lock()
		network.behave["198.51.100.10:22"] = "hang"
		network.mu.Unlock()
		before := len(network.dials())
		response, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "diagnose.tunnel:30:cancel", Operation: diagnoseTunnel(30, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED),
			Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED,
		})
		require.NoError(t, err)
		// Both nodes were dialled, and only the hanging entry node's probe
		// is still in flight.
		require.Eventually(t, func() bool {
			inFlight, _ := network.busy()
			return len(network.dials()) == before+2 && inFlight == 1
		}, 5*time.Second, 5*time.Millisecond)
		stopped, err := client.CancelOperation(context.Background(), &kernelnodeopsv1.CancelOperationRequest{OperationId: response.GetOperation().GetOperationId()})
		require.NoError(t, err)
		require.Equal(t, cancelled, stopped.GetOperation().GetState())
		requireProtoEqual(t, []*kernelnodeopsv1.DiagnosisOutcome{
			{Description: "管理端->出口节点", NodeName: "exit", NodeId: "11", TargetIp: "198.51.100.11", TargetPort: 22, Message: "dial tcp 198.51.100.11:22: connect: connection refused"},
		}, withoutTimings(stopped.GetOperation().GetResult().GetDiagnosis().GetOutcomes()))

		// A tunnel deleted since the submission is TARGET_GONE.
		submitted := submit(t, client, "diagnose.tunnel:31", diagnoseTunnel(31, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED))
		require.NoError(t, db.Delete(&model.ForwardTunnel{}, 31).Error)
		gone := eventually(t, get(t, client, submitted.GetOperation().GetOperationId()), inState(failed))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, gone.GetError().GetCode())
		require.Equal(t, "隧道不存在", gone.GetError().GetMessage())
	})
}

// The vantage (D10): Control by default; the node when the agent of every
// node the diagnosis concerns advertises diag.v1 and Control was not asked
// for. The kernel does not run diagnoses on agents yet, so it dials from
// Control either way and says so.
func TestDiagnosisVantage(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		prepareForward(t, db, "203.0.113.1:80")
		network := newFakeNetwork(map[string]string{"203.0.113.1:80": "ok", "198.51.100.10:22": "ok", "198.51.100.11:22": "ok"})
		h := diagnosisHarness(t, db, network, agents{10: true, 11: true})
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))
		control, node, unspecified := kernelnodeopsv1.Vantage_VANTAGE_CONTROL, kernelnodeopsv1.Vantage_VANTAGE_NODE, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED
		fallback := "the node's agent advertises diag.v1, but this kernel does not run diagnoses on agents yet: it dialled from Control"
		missing := "the node's agent does not advertise diag.v1: the kernel dialled from Control"

		for i, c := range []struct {
			spec *kernelnodeopsv1.OperationSpec
			want *kernelnodeopsv1.VantageReport
		}{
			{diagnoseForward(40, unspecified), &kernelnodeopsv1.VantageReport{Requested: unspecified, Selected: node, Used: control, Note: fallback}},
			{diagnoseForward(40, node), &kernelnodeopsv1.VantageReport{Requested: node, Selected: node, Used: control, Note: fallback}},
			{diagnoseForward(40, control), &kernelnodeopsv1.VantageReport{Requested: control, Selected: control, Used: control}},
			{diagnoseTunnel(30, node), &kernelnodeopsv1.VantageReport{Requested: node, Selected: node, Used: control, Note: fallback}},
			// Tunnel 31's node 12 has no agent advertising diag.v1.
			{diagnoseTunnel(31, node), &kernelnodeopsv1.VantageReport{Requested: node, Selected: control, Used: control, Note: missing}},
			{diagnoseTunnel(31, unspecified), &kernelnodeopsv1.VantageReport{Requested: unspecified, Selected: control, Used: control}},
		} {
			operation := submitWait(t, client, fmt.Sprintf("diagnose:vantage:%d", i), c.spec)
			require.Equal(t, succeeded, operation.GetState(), "%v", operation.GetError())
			require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_CONTROL_DIAL, operation.GetChannel())
			require.True(t, proto.Equal(c.want, operation.GetResult().GetDiagnosis().GetVantage()), "case %d: %v", i, operation.GetResult().GetDiagnosis().GetVantage())
		}

		operation := submitWait(t, client, "diagnose.endpoints:vantage", checkNodes(false, node, 10, 12))
		require.Equal(t, succeeded, operation.GetState())
		checks := operation.GetResult().GetEndpointCheck().GetChecks()
		require.True(t, proto.Equal(&kernelnodeopsv1.VantageReport{Requested: node, Selected: node, Used: control, Note: fallback}, checks[0].GetVantage()))
		require.True(t, proto.Equal(&kernelnodeopsv1.VantageReport{Requested: node, Selected: control, Used: control, Note: missing}, checks[1].GetVantage()))
		require.True(t, checks[0].GetReachable(), "the node vantage still dials from Control")
	})
}

// A diagnosis runs a bounded number of probes at once.
func TestDiagnosisBoundsItsProbes(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		behave := map[string]string{}
		var targets []string
		for i := 1; i <= 24; i++ {
			target := fmt.Sprintf("203.0.113.%d:443", i)
			targets = append(targets, target)
			behave[target] = "slow"
		}
		prepareForward(t, db, targets...)
		for _, limit := range []int{3, 0} {
			t.Run(fmt.Sprintf("limit %d", limit), func(t *testing.T) {
				network := newFakeNetwork(behave)
				probes := network.probes()
				probes.Concurrency = limit
				h := newHarness(t, db)
				require.NoError(t, (&Diagnosis{Probes: probes}).Register(h.registry))
				require.NoError(t, db.AutoMigrate(&model.SystemConfig{}))
				h.start(t)
				client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))
				operation := submitWait(t, client, fmt.Sprintf("diagnose.forward:bounded:%d", limit), diagnoseForward(40, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED))
				require.Equal(t, succeeded, operation.GetState(), "%v", operation.GetError())
				require.Len(t, operation.GetResult().GetDiagnosis().GetOutcomes(), 24)
				for _, outcome := range operation.GetResult().GetDiagnosis().GetOutcomes() {
					require.True(t, outcome.GetSuccess(), "%v", outcome)
				}
				want := limit
				if want == 0 {
					want = service.DefaultDiagnosisConcurrency
				}
				_, most := network.busy()
				require.LessOrEqual(t, most, want)
				require.Greater(t, most, 1, "the probes run concurrently")
			})
		}
	})
}

// stringsOf returns every string and bytes value message holds.
func stringsOf(message protoreflect.Message) []string {
	var values []string
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		switch {
		case field.IsMap():
		case field.IsList():
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				switch field.Kind() {
				case protoreflect.StringKind:
					values = append(values, list.Get(i).String())
				case protoreflect.BytesKind:
					values = append(values, string(list.Get(i).Bytes()))
				case protoreflect.MessageKind:
					values = append(values, stringsOf(list.Get(i).Message())...)
				}
			}
		case field.Kind() == protoreflect.StringKind:
			values = append(values, value.String())
		case field.Kind() == protoreflect.BytesKind:
			values = append(values, string(value.Bytes()))
		case field.Kind() == protoreflect.MessageKind:
			values = append(values, stringsOf(value.Message())...)
		}
		return true
	})
	return values
}

// No diagnosis result, error or ledger row carries a node's token or a
// value at a secret key, even when what a node or its metrics endpoint
// answered echoes one: the walk reads every string of every operation.
func TestDiagnosisResultsCarryNoSecrets(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		port := metricsServer(t, func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "relay-SECRET-token-10 rejected; password=hunter22-SECRET", http.StatusUnauthorized)
		})
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 10).Updates(map[string]any{
			"host": "127.0.0.1", "api_port": 18080, "api_token": "relay-SECRET-token-10", "metrics_port": port, "type": model.ForwardNodeTypeExit,
		}).Error)
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 11).Updates(map[string]any{"api_port": 18080, "api_token": "exit-SECRET-token-11"}).Error)
		prepareForward(t, db, "203.0.113.1:80", "203.0.113.9:80")
		// What a node answers echoes the tokens of the nodes the operation
		// concerns, and values at secret keys.
		const leaks = "; api_key=key-SECRET-9 {\"private_key\":\"pk-SECRET\"}"
		network := newFakeNetwork(map[string]string{
			"198.51.100.11:22": "refused by exit-SECRET-token-11" + leaks,
			"127.0.0.1:22":     "refused by relay-SECRET-token-10" + leaks,
			"203.0.113.1:80":   "refused by exit-SECRET-token-11 and relay-SECRET-token-10" + leaks,
			"203.0.113.9:80":   "ok",
		})
		h := diagnosisHarness(t, db, network, nil)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))

		var operations []*kernelnodeopsv1.Operation
		for i, spec := range []*kernelnodeopsv1.OperationSpec{
			checkNodes(true, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED, 11, 12),
			nodeStats(10),
			diagnoseForward(40, kernelnodeopsv1.Vantage_VANTAGE_NODE),
			diagnoseTunnel(30, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED),
		} {
			operation := submitWait(t, client, fmt.Sprintf("diagnose:secrets:%d", i), spec)
			name, _ := stateName(operation.GetState())
			require.True(t, Terminal(name), "%v", operation)
			operations = append(operations, operation)
		}
		require.NotEmpty(t, operations[0].GetResult().GetEndpointCheck().GetChecks()[0].GetError())
		require.Contains(t, operations[1].GetError().GetMessage(), "********")
		require.Contains(t, operations[2].GetResult().GetDiagnosis().GetOutcomes()[0].GetMessage(), "********")

		secrets := []string{"relay-SECRET-token-10", "exit-SECRET-token-11", "hunter22-SECRET", "key-SECRET-9", "pk-SECRET"}
		for _, operation := range operations {
			for _, text := range stringsOf(operation.ProtoReflect()) {
				for _, secret := range secrets {
					require.NotContains(t, text, secret, "operation %s", operation.GetKind())
				}
			}
		}
		for _, row := range h.rows(t) {
			for _, text := range []string{row.Result, row.ErrorMessage, row.Evidence, row.Operation} {
				for _, secret := range secrets {
					require.NotContains(t, text, secret, "ledger row %s", row.Kind)
				}
			}
		}
	})
}
