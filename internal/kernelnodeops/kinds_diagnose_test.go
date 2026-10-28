package kernelnodeops

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
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

func forwardNodeRow(t *testing.T, db *gorm.DB, id uint) model.ForwardNode {
	t.Helper()
	var node model.ForwardNode
	require.NoError(t, db.First(&node, id).Error)
	return node
}

// legacyOutcomes maps the legacy route's report into the contract's
// outcomes, without the probes' timings.
// The kernel serves the four diagnosis kinds, and GetCapabilities lists
// them.
func TestTheKernelServesTheDiagnoseKinds(t *testing.T) {
	diagnosis := []string{KindDiagnoseEndpoints}
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
func TestDiagnosisVantage(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		network := newFakeNetwork(map[string]string{"198.51.100.10:22": "ok", "198.51.100.11:22": "ok"})
		h := diagnosisHarness(t, db, network, agents{10: true, 11: true})
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))
		control, node := kernelnodeopsv1.Vantage_VANTAGE_CONTROL, kernelnodeopsv1.Vantage_VANTAGE_NODE
		fallback := "the node's agent advertises diag.v1, but this kernel does not run diagnoses on agents yet: it dialled from Control"
		missing := "the node's agent does not advertise diag.v1: the kernel dialled from Control"

		operation := submitWait(t, client, "diagnose.endpoints:vantage", checkNodes(false, node, 10, 12))
		require.Equal(t, succeeded, operation.GetState())
		checks := operation.GetResult().GetEndpointCheck().GetChecks()
		require.True(t, proto.Equal(&kernelnodeopsv1.VantageReport{Requested: node, Selected: node, Used: control, Note: fallback}, checks[0].GetVantage()))
		require.True(t, proto.Equal(&kernelnodeopsv1.VantageReport{Requested: node, Selected: control, Used: control, Note: missing}, checks[1].GetVantage()))
		require.True(t, checks[0].GetReachable(), "the node vantage still dials from Control")
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
// value at a secret key, even when what a node answered echoes one: the walk reads every string of every operation.
func TestDiagnosisResultsCarryNoSecrets(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 11).Updates(map[string]any{"api_port": 18080, "api_token": "exit-SECRET-token-11"}).Error)
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 12).Updates(map[string]any{"api_port": 18080, "api_token": "relay-SECRET-token-12"}).Error)
		// What a node answers echoes the tokens of the nodes the operation
		// concerns, and values at secret keys.
		const leaks = "; api_key=key-SECRET-9 {\"private_key\":\"pk-SECRET\"}"
		network := newFakeNetwork(map[string]string{
			"198.51.100.11:22": "refused by exit-SECRET-token-11 and relay-SECRET-token-12" + leaks,
		})
		h := diagnosisHarness(t, db, network, nil)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))

		var operations []*kernelnodeopsv1.Operation
		for i, spec := range []*kernelnodeopsv1.OperationSpec{
			checkNodes(true, kernelnodeopsv1.Vantage_VANTAGE_UNSPECIFIED, 11, 12),
		} {
			operation := submitWait(t, client, fmt.Sprintf("diagnose:secrets:%d", i), spec)
			name, _ := stateName(operation.GetState())
			require.True(t, Terminal(name), "%v", operation)
			operations = append(operations, operation)
		}
		require.Contains(t, operations[0].GetResult().GetEndpointCheck().GetChecks()[0].GetError(), "********")

		secrets := []string{"relay-SECRET-token-12", "exit-SECRET-token-11", "key-SECRET-9", "pk-SECRET"}
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
