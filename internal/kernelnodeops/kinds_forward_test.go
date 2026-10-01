package kernelnodeops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// fakeNodeX is the NodeX control plane of these tests: it records every
// execute request and answers by the name of the forward or rule in it:
// "failing" is refused, "hang" waits for the request to end, "slow"
// answers after a while, "echo" refuses with the tokens it was sent, and
// anything else is applied.
type fakeNodeX struct {
	server  *httptest.Server
	mu      sync.Mutex
	bodies  []string
	started chan string
}

func newFakeNodeX(t *testing.T) *fakeNodeX {
	t.Helper()
	n := &fakeNodeX{started: make(chan string, 64)}
	n.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n.mu.Lock()
		n.bodies = append(n.bodies, string(body))
		n.mu.Unlock()
		select {
		case n.started <- string(body):
		default:
		}
		text := string(body)
		switch {
		case strings.Contains(text, `"name":"hang"`):
			<-r.Context().Done()
			return
		case strings.Contains(text, `"name":"slow"`):
			select {
			case <-time.After(300 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		case strings.Contains(text, `"name":"failing"`):
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"nodex refused the change"}`))
			return
		case strings.Contains(text, `"name":"echo"`):
			var request map[string]any
			_ = json.Unmarshal(body, &request)
			encoded, _ := json.Marshal(request)
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"nodex refused: ` + strings.ReplaceAll(string(encoded), `"`, `'`) + `"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"backend":"gost","status":2,"message":"nodex applied","result":"applied"}}`))
	}))
	t.Cleanup(n.server.Close)
	return n
}

func (n *fakeNodeX) received() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := append([]string(nil), n.bodies...)
	n.bodies = nil
	return out
}

func (n *fakeNodeX) waitStarted(t *testing.T) string {
	t.Helper()
	select {
	case body := <-n.started:
		return body
	case <-time.After(10 * time.Second):
		require.FailNow(t, "NodeX was not called")
		return ""
	}
}

// forwardHarness is an engine serving the forward kinds on the seeded
// database, with the forward nodes given API ports and tokens, the split
// tables installed and the tokens pinned, and the runtime on the gost
// backend through a fake NodeX.
type forwardHarness struct {
	*harness
	nodex   *fakeNodeX
	configs *service.SystemConfigService
}

func setConfig(t *testing.T, configs *service.SystemConfigService, values map[string]string) {
	t.Helper()
	for key, value := range values {
		require.NoError(t, configs.Set(key, value, "string", "forward", "forward kinds test"))
	}
}

func newForwardHarness(t *testing.T, db *gorm.DB, options ...func(*Engine)) *forwardHarness {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.SystemConfig{}, &model.ForwardRuntimeJob{}, &model.ForwardPortBinding{}, &model.ForwardUserTunnel{}, &model.SpeedLimit{}, &model.OperationLog{}))
	require.NoError(t, service.EnsureForwardRuntimeJobSchema(db))
	require.NoError(t, nodesecrets.EnsureSchema(db))
	for _, id := range []uint{10, 11, 12} {
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", id).Updates(map[string]any{
			"api_port": 18080, "api_token": fmt.Sprintf("relay-token-%d", id), "type": model.ForwardNodeTypeRelay, "enabled": true,
		}).Error)
	}
	require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 12).Update("type", model.ForwardNodeTypeExit).Error)
	require.NoError(t, nodesecrets.Sync(db, nodesecrets.TableForwardNode, 10, 11, 12))
	nodex := newFakeNodeX(t)
	configs := service.NewSystemConfigService(db)
	setConfig(t, configs, map[string]string{
		"forward.runtime_backend": model.ForwardRuntimeBackendGost, "forward.runtime.nodex.base_url": nodex.server.URL,
		"forward.runtime.nodex.token": "nodex-shared-token",
	})
	h := newHarness(t, db, options...)
	require.NoError(t, (&Forward{JobPoll: 20 * time.Millisecond}).Register(h.registry))
	return &forwardHarness{harness: h, nodex: nodex, configs: configs}
}

func (h *forwardHarness) forward(t *testing.T, id uint) model.Forward {
	t.Helper()
	var record model.Forward
	require.NoError(t, h.db.First(&record, id).Error)
	return record
}

func (h *forwardHarness) jobs(t *testing.T, forwardID uint) []model.ForwardRuntimeJob {
	t.Helper()
	var jobs []model.ForwardRuntimeJob
	require.NoError(t, h.db.Where("forward_id = ?", forwardID).Order("id").Find(&jobs).Error)
	return jobs
}

func (h *forwardHarness) rename(t *testing.T, forwardID uint, name string) {
	t.Helper()
	require.NoError(t, h.db.Model(&model.Forward{}).Where("id = ?", forwardID).Update("name", name).Error)
}

// ledgerText is everything the ledger holds, as JSON.
func (h *forwardHarness) ledgerText(t *testing.T) string {
	t.Helper()
	encoded, err := json.Marshal(h.rows(t))
	require.NoError(t, err)
	return string(encoded)
}

func legacyRule(id uint64, action kernelnodeopsv1.ForwardAction) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_ApplyLegacyRule{
		ApplyLegacyRule: &kernelnodeopsv1.ApplyLegacyRule{RuleId: id, Action: action},
	}}
}

func syncBackend(backend string) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_SyncForwardBackend{
		SyncForwardBackend: &kernelnodeopsv1.SyncForwardBackend{Backend: backend},
	}}
}

// The kernel serves the forward kinds and the connection test by default.
func TestTheKernelServesTheForwardKinds(t *testing.T) {
	forwardKinds := []string{KindForwardApply, KindForwardTunnel, KindForwardSyncBackend, KindForwardLegacyRule}
	require.Subset(t, DefaultExecutors.Kinds(), append(forwardKinds, KindDiagnoseForwardBackend))
	require.Equal(t, kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_DIAGNOSE, FamilyOfKind(KindDiagnoseForwardBackend))
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		engine := &Engine{DB: db, Executors: DefaultExecutors}
		capabilities, err := (&Server{Engine: engine, Authorizer: allow(service.CapabilityNodeOpsForward)}).For(forwardHost).
			GetCapabilities(context.Background(), &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.NoError(t, err)
		require.Subset(t, capabilities.GetKinds(), forwardKinds)
		h := newHarness(t, db)
		require.NoError(t, (&Forward{}).Register(h.registry))
		require.Error(t, (&Forward{}).Register(h.registry), "a kind is served once")
	})
}

// ApplyForward on the gost backend runs NodeX synchronously: the job row,
// the forward's runtime columns and the result record what NodeX answered,
// the stored payload carries no token, and NodeX got the ingress node's.
func TestApplyForwardThroughNodeX(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newForwardHarness(t, db)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsForward))

		op := submitWait(t, client, "forward.apply:40:create", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_CREATE))
		require.Equal(t, succeeded, op.GetState(), "%v", op.GetError())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_NODEX, op.GetChannel())
		result := op.GetResult().GetForwardApply()
		require.Equal(t, model.ForwardRuntimeBackendGost, result.GetBackend())
		require.EqualValues(t, model.ForwardRuntimeJobStatusSuccess, result.GetRuntimeStatus())
		require.Equal(t, "nodex applied", result.GetMessage())
		require.NotZero(t, result.GetRuntimeJobId())
		forward := h.forward(t, 40)
		require.Equal(t, model.ForwardRuntimeJobStatusSuccess, forward.RuntimeStatus)
		require.Equal(t, "nodex applied", forward.RuntimeMessage)
		require.NotNil(t, forward.RuntimeLastSyncAt)
		jobs := h.jobs(t, 40)
		require.Len(t, jobs, 1)
		require.EqualValues(t, result.GetRuntimeJobId(), jobs[0].ID)
		require.Equal(t, "create", jobs[0].Action)
		require.Equal(t, "applied", jobs[0].Result)
		require.NotContains(t, jobs[0].Payload, "apiToken", "a stored payload carries no token")
		require.NotContains(t, jobs[0].Payload, "relay-token-10")
		sent := h.nodex.received()
		require.Len(t, sent, 1)
		require.Contains(t, sent[0], `"apiToken":"relay-token-10"`, "NodeX gets the ingress node's token with the request")
		require.Contains(t, sent[0], `"action":"create"`)

		// A change NodeX refuses fails with its message; a forced deletion
		// succeeds anyway, so the package may delete the forward.
		h.rename(t, 41, "failing")
		op = submitWait(t, client, "forward.apply:41:update", applyForward(41, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE))
		require.Equal(t, failed, op.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, op.GetError().GetCode())
		require.Equal(t, "nodex refused the change", op.GetError().GetMessage())
		require.EqualValues(t, model.ForwardRuntimeJobStatusFailed, op.GetResult().GetForwardApply().GetRuntimeStatus())
		require.Equal(t, model.ForwardRuntimeJobStatusFailed, h.forward(t, 41).RuntimeStatus)
		op = submitWait(t, client, "forward.apply:41:force", applyForward(41, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_FORCE_DELETE))
		require.Equal(t, succeeded, op.GetState(), "%v", op.GetError())
		require.EqualValues(t, model.ForwardRuntimeJobStatusFailed, op.GetResult().GetForwardApply().GetRuntimeStatus())
		require.Equal(t, "nodex refused the change", op.GetResult().GetForwardApply().GetMessage())

		// A forward deleted since the submission is gone.
		op = submitWait(t, client, "forward.apply:40:gone", applyForwardAfter(t, h.db, 40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_SYNC))
		require.Equal(t, failed, op.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, op.GetError().GetCode())
	})
}

// applyForwardAfter is an ApplyForward whose forward is deleted once the
// operation is recorded: the kind's executor finds it gone.
func applyForwardAfter(t *testing.T, db *gorm.DB, id uint64, action kernelnodeopsv1.ForwardAction) *kernelnodeopsv1.OperationSpec {
	t.Helper()
	require.NoError(t, db.Delete(&model.Forward{}, id).Error)
	spec := applyForward(id, action)
	// The target still resolves through the tunnel: the forward row is
	// gone, so resolve fails NOT_FOUND. Recreate a bare row the executor
	// then finds without its tunnel.
	require.NoError(t, db.Create(&model.Forward{ID: uint(id), UserID: 1, Name: "ghost", TunnelID: 999, InPort: 1000, RemoteAddr: "203.0.113.1:80"}).Error)
	return spec
}

// An operation ends TIMED_OUT at its deadline while NodeX hangs, and
// CANCELLED when it is cancelled.
func TestApplyForwardTimesOutAndCancels(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newForwardHarness(t, db, func(e *Engine) { e.Timeout = 400 * time.Millisecond })
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsForward))
		h.rename(t, 40, "hang")

		op := submitWait(t, client, "forward.apply:40:hang", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE))
		require.Equal(t, timedOut, op.GetState(), "%v", op.GetError())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED, op.GetError().GetCode())
		deadline := time.Now().Add(5 * time.Second)
		for jobs := h.jobs(t, 40); ; jobs = h.jobs(t, 40) {
			require.Len(t, jobs, 1)
			if jobs[0].Status == model.ForwardRuntimeJobStatusFailed {
				break
			}
			require.True(t, time.Now().Before(deadline), "the job records the interrupted call")
			time.Sleep(10 * time.Millisecond)
		}

		h.rename(t, 41, "hang")
		h.engine.Timeout = time.Minute
		submitted := submit(t, client, "forward.apply:41:hang", applyForward(41, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE)).GetOperation()
		h.nodex.waitStarted(t)
		_, err := client.CancelOperation(context.Background(), &kernelnodeopsv1.CancelOperationRequest{OperationId: submitted.GetOperationId(), Reason: "operator"})
		require.NoError(t, err)
		op = eventually(t, get(t, client, submitted.GetOperationId()), inState(cancelled))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_CANCELLED, op.GetError().GetCode())
	})
}

// Two operations on one forward run one at a time, and a pending change is
// superseded by a newer one.
func TestConcurrentForwardOperationsAreSuperseded(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newForwardHarness(t, db)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsForward))
		h.rename(t, 40, "slow")

		first := submit(t, client, "forward.apply:40:1", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_CREATE)).GetOperation()
		h.nodex.waitStarted(t)
		update := submit(t, client, "forward.apply:40:2", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE)).GetOperation()
		pause := submit(t, client, "forward.apply:40:3", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_PAUSE)).GetOperation()
		require.Equal(t, superseded, get(t, client, update.GetOperationId())().GetState())
		require.Equal(t, running, get(t, client, first.GetOperationId())().GetState())
		eventually(t, get(t, client, first.GetOperationId()), inState(succeeded))
		eventually(t, get(t, client, pause.GetOperationId()), inState(succeeded))
		sent := h.nodex.received()
		require.Len(t, sent, 2, "the superseded change never reached NodeX")
		require.Contains(t, sent[0], `"action":"create"`)
		require.Contains(t, sent[1], `"action":"pause"`)
	})
}

// A fenced generation cannot submit a forward change.
func TestFencedGenerationsCannotApplyForwards(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newForwardHarness(t, db)
		h.start(t)
		grants := allow(service.CapabilityNodeOpsForward)
		grants.set("fenced", true)
		client := h.client(forwardHost, grants)
		_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "fenced", Operation: applyForward(40, forwardAction)})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Empty(t, h.rows(t))
		require.Empty(t, h.nodex.received())
	})
}

// ansibleRuntime configures the local Ansible backend with a fake
// ansible-playbook that writes its arguments and answers as behave says
// ("ok" succeeds, anything else fails with that text).
func ansibleRuntime(t *testing.T, h *forwardHarness, behave string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"inventory.ini", "apply.yml", "remove.yml"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("# "+name+"\n"), 0o600))
	}
	script := filepath.Join(dir, "ansible-playbook")
	body := "#!/bin/sh\necho \"$@\" > \"" + filepath.Join(dir, "args") + "\"\n"
	if behave == "ok" {
		body += "echo applied by ansible\n"
	} else {
		body += "echo \"" + behave + "\" >&2\nexit 1\n"
	}
	require.NoError(t, os.WriteFile(script, []byte(body), 0o700)) // #nosec G306 -- a test's executable fake.
	config, err := json.Marshal(map[string]any{
		"inventory": filepath.Join(dir, "inventory.ini"), "playbookApply": filepath.Join(dir, "apply.yml"), "playbookRemove": filepath.Join(dir, "remove.yml"),
		"command": script, "workingDir": dir, "targetPattern": "{{node.host}}",
	})
	require.NoError(t, err)
	setConfig(t, h.configs, map[string]string{
		"forward.runtime_backend": model.ForwardRuntimeBackendNftablesAnsible, "forward.runtime.ansible.backend": model.ForwardRuntimeBackendNftablesAnsible,
		"forward.runtime.ansible.config": string(config),
	})
	return dir
}

// runJobs runs the local Ansible job executor until the test ends.
func runJobs(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	executor := service.NewPanelForwardRuntimeJobExecutor(db)
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			_ = executor.RunPendingJobs(ctx)
			select {
			case <-ctx.Done():
			case <-time.After(30 * time.Millisecond):
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

// forwardOnTunnel31 adds forward 42 on tunnel 31, whose execution node is
// node 12.
func forwardOnTunnel31(t *testing.T, db *gorm.DB, backend string) {
	t.Helper()
	require.NoError(t, db.Create(&model.Forward{ID: 42, UserID: 1, Name: "c", TunnelID: 31, InPort: 1002, RemoteAddr: "203.0.113.4:80", RuntimeBackend: backend}).Error)
}

// ApplyForward on a local Ansible backend queues a job the local executor
// runs; the operation is ACCEPTED when the job is queued and ends with the
// job. The payload carries no token.
func TestApplyForwardThroughLocalAnsible(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newForwardHarness(t, db)
		dir := ansibleRuntime(t, h, "ok")
		forwardOnTunnel31(t, db, model.ForwardRuntimeBackendNftablesAnsible)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsForward))

		accepted, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "forward.apply:42:create", Operation: applyForward(42, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_CREATE),
			Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED, WaitTimeoutMs: 10000,
		})
		require.NoError(t, err)
		require.Equal(t, running, accepted.GetOperation().GetState())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_LOCAL_ANSIBLE, accepted.GetOperation().GetChannel())
		jobs := h.jobs(t, 42)
		require.Len(t, jobs, 1)
		require.Equal(t, model.ForwardRuntimeJobStatusPending, jobs[0].Status)
		require.NotContains(t, jobs[0].Payload, "apiToken")
		require.NotContains(t, jobs[0].Payload, "relay-token-12")
		require.Equal(t, running, get(t, client, accepted.GetOperation().GetOperationId())().GetState(), "the operation waits for the job")

		runJobs(t, db)
		op := eventually(t, get(t, client, accepted.GetOperation().GetOperationId()), inState(succeeded, failed))
		require.Equal(t, succeeded, op.GetState(), "%v", op.GetError())
		require.Equal(t, "applied by ansible", op.GetResult().GetForwardApply().GetMessage())
		args, err := os.ReadFile(filepath.Join(dir, "args"))
		require.NoError(t, err)
		require.Contains(t, string(args), "-i "+filepath.Join(dir, "inventory.ini"))
		require.NotContains(t, string(args), "relay-token", "Ansible gets no node token")
		require.Equal(t, model.ForwardRuntimeJobStatusSuccess, h.forward(t, 42).RuntimeStatus)

		// A failing playbook fails the operation with its output.
		ansibleRuntime(t, h, "nft refused")
		op = submitWait(t, client, "forward.apply:42:update", applyForward(42, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE))
		require.Equal(t, failed, op.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, op.GetError().GetCode())
		require.Contains(t, op.GetError().GetMessage(), "nft refused")
	})
}

// ApplyForward on the clean agent backend queues a job the node's agent
// claims and reports; the claim carries no token, for a new payload and
// for one written before NO-7.
func TestApplyForwardThroughACleanAgent(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newForwardHarness(t, db)
		setConfig(t, h.configs, map[string]string{"forward.runtime_backend": model.ForwardRuntimeBackendCleanAgent})
		forwardOnTunnel31(t, db, model.ForwardRuntimeBackendCleanAgent)
		node := uint(12)
		require.NoError(t, db.Create(&model.ForwardCleanAgent{ID: 21, NodeID: &node, Name: "agent-12", Token: "agent-12-token"}).Error)
		agents := service.NewForwardCleanAgentService(db)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsForward))

		accepted, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "forward.apply:42:create", Operation: applyForward(42, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_CREATE),
			Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED, WaitTimeoutMs: 10000,
		})
		require.NoError(t, err)
		require.Equal(t, running, accepted.GetOperation().GetState())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_CLEAN_AGENT_JOB, accepted.GetOperation().GetChannel())

		// An old payload, written before NO-7 with the node's token, is
		// queued for the same node on another forward.
		require.NoError(t, db.Create(&model.Forward{ID: 43, UserID: 1, Name: "old", TunnelID: 31, InPort: 1003, RemoteAddr: "203.0.113.5:80", RuntimeBackend: model.ForwardRuntimeBackendCleanAgent}).Error)
		old := uint(43)
		require.NoError(t, db.Create(&model.ForwardRuntimeJob{Backend: model.ForwardRuntimeBackendCleanAgent, Action: "create", ResourceType: "panel_forward",
			ResourceID: &old, ForwardID: &old, NodeID: &node, Status: model.ForwardRuntimeJobStatusPending,
			Payload: `{"resourceType":"panel_forward","backend":"clean_agent","action":"create","panelForward":{"ingressNode":{"id":12,"host":"198.51.100.12","apiPort":18080,"apiToken":"relay-token-12"}}}`}).Error)

		actions, err := agents.Heartbeat(service.ForwardCleanAgentHeartbeatInput{AgentID: 21, Token: "agent-12-token", Limit: 10})
		require.NoError(t, err)
		require.Len(t, actions, 2)
		for _, action := range actions {
			require.NotContains(t, string(action.Payload), "relay-token-12", "the claim serves no token")
			require.NotContains(t, string(action.Payload), `"apiToken":"r`)
			require.Contains(t, string(action.Payload), `"host":"198.51.100.12"`)
		}
		require.EqualValues(t, 42, *actions[0].ForwardID)
		yes := true
		require.NoError(t, agents.Report(service.ForwardCleanAgentReportInput{AgentID: 21, Token: "agent-12-token", JobID: actions[0].JobID, Success: &yes, Result: "agent applied"}))
		op := eventually(t, get(t, client, accepted.GetOperation().GetOperationId()), inState(succeeded, failed))
		require.Equal(t, succeeded, op.GetState(), "%v", op.GetError())
		require.Equal(t, "agent applied", op.GetResult().GetForwardApply().GetMessage())
		require.Equal(t, model.ForwardRuntimeJobStatusSuccess, h.forward(t, 42).RuntimeStatus)

		// A failure the agent reports fails the operation.
		accepted, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "forward.apply:42:update", Operation: applyForward(42, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE),
			Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED, WaitTimeoutMs: 10000,
		})
		require.NoError(t, err)
		actions, err = agents.Heartbeat(service.ForwardCleanAgentHeartbeatInput{AgentID: 21, Token: "agent-12-token", Limit: 10})
		require.NoError(t, err)
		require.Len(t, actions, 1)
		no := false
		require.NoError(t, agents.Report(service.ForwardCleanAgentReportInput{AgentID: 21, Token: "agent-12-token", JobID: actions[0].JobID, Success: &no, Error: "nft: permission denied"}))
		op = eventually(t, get(t, client, accepted.GetOperation().GetOperationId()), inState(succeeded, failed))
		require.Equal(t, failed, op.GetState())
		require.Equal(t, "nft: permission denied", op.GetError().GetMessage())
	})
}

// ApplyTunnel fans out one forward.apply UPDATE per active forward of the
// tunnel; the parent counts them.
func TestApplyTunnelFansOut(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newForwardHarness(t, db)
		require.NoError(t, db.Create(&model.Forward{ID: 44, UserID: 1, Name: "paused", TunnelID: 30, InPort: 1004, RemoteAddr: "203.0.113.6:80"}).Error)
		require.NoError(t, db.Model(&model.Forward{}).Where("id = ?", 44).Update("status", model.ForwardStatusPaused).Error)
		h.rename(t, 41, "failing")
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsForward))

		op := submitWait(t, client, "forward.tunnel:30", applyTunnel(30))
		require.Equal(t, failed, op.GetState(), "one child failed")
		require.EqualValues(t, 2, op.GetFanOut().GetTotal())
		require.EqualValues(t, 1, op.GetFanOut().GetSucceeded())
		require.EqualValues(t, 1, op.GetFanOut().GetFailed())
		require.Equal(t, "1 of 2 operations failed", op.GetError().GetMessage())
		var children []model.KernelNodeOperation
		require.NoError(t, db.Where("parent_operation_id = ?", op.GetOperationId()).Find(&children).Error)
		require.Len(t, children, 2)
		for _, child := range children {
			require.Equal(t, KindForwardApply, child.Kind)
			require.Contains(t, child.Operation, `"action":"FORWARD_ACTION_UPDATE"`)
		}
		sent := h.nodex.received()
		require.Len(t, sent, 2, "the paused forward is not re-applied")
		require.Equal(t, model.ForwardRuntimeJobStatusFailed, h.forward(t, 41).RuntimeStatus)
		require.Equal(t, model.ForwardStatusActive, h.forward(t, 41).Status, "the status column is the package's")

		// A tunnel without active forwards ends at once.
		op = submitWait(t, client, "forward.tunnel:31", applyTunnel(31))
		require.Equal(t, succeeded, op.GetState())
		require.Zero(t, op.GetFanOut().GetTotal())
	})
}

// SyncForwardBackend moves the active forwards not on the target backend
// to it and fans out one forward.apply SYNC per forward; the backend in
// force is the default target.
func TestSyncForwardBackendFansOut(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newForwardHarness(t, db)
		require.NoError(t, db.Model(&model.Forward{}).Where("id = ?", 40).Update("runtime_backend", model.ForwardRuntimeBackendCleanAgent).Error)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsForward))

		op := submitWait(t, client, "forward.sync_backend:gost", syncBackend(model.ForwardRuntimeBackendGost))
		require.Equal(t, succeeded, op.GetState(), "%v", op.GetError())
		require.EqualValues(t, 1, op.GetFanOut().GetTotal())
		require.Equal(t, model.ForwardRuntimeBackendGost, h.forward(t, 40).RuntimeBackend)
		sent := h.nodex.received()
		require.Len(t, sent, 1)
		require.Contains(t, sent[0], `"action":"sync"`)
		require.Contains(t, sent[0], `"id":40`)

		// Every forward is on the backend in force now: nothing to do.
		op = submitWait(t, client, "forward.sync_backend:default", syncBackend(""))
		require.Equal(t, succeeded, op.GetState())
		require.Zero(t, op.GetFanOut().GetTotal())
		require.Empty(t, h.nodex.received())
	})
}

// ApplyLegacyRule pushes the rule through NodeX with both nodes' tokens,
// resolved at send time.
func TestApplyLegacyRule(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newForwardHarness(t, db)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsForward))

		op := submitWait(t, client, "forward.legacy_rule:50:create", legacyRule(50, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_CREATE))
		require.Equal(t, succeeded, op.GetState(), "%v", op.GetError())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_NODEX, op.GetChannel())
		require.True(t, op.GetResult().GetLegacyRule().GetAppliedOnRelay())
		require.True(t, op.GetResult().GetLegacyRule().GetAppliedOnExit())
		sent := h.nodex.received()
		require.Len(t, sent, 1)
		require.Contains(t, sent[0], `"resourceType":"legacy_rule"`)
		require.Contains(t, sent[0], `"apiToken":"relay-token-10"`)
		require.Contains(t, sent[0], `"apiToken":"relay-token-12"`)

		for _, action := range []kernelnodeopsv1.ForwardAction{kernelnodeopsv1.ForwardAction_FORWARD_ACTION_PAUSE, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_RESUME, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_DELETE} {
			op = submitWait(t, client, "forward.legacy_rule:50:"+action.String(), legacyRule(50, action))
			require.Equal(t, succeeded, op.GetState(), "%v", op.GetError())
		}
		sent = h.nodex.received()
		require.Len(t, sent, 3)
		require.Contains(t, sent[0], `"action":"sync"`, "a pause syncs the row as it is")
		require.Contains(t, sent[2], `"action":"delete"`)

		require.NoError(t, db.Model(&model.ForwardRule{}).Where("id = ?", 50).Update("name", "failing").Error)
		op = submitWait(t, client, "forward.legacy_rule:50:failing", legacyRule(50, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE))
		require.Equal(t, failed, op.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, op.GetError().GetCode())
		require.Equal(t, "nodex refused the change", op.GetError().GetMessage())
		require.Equal(t, "nodex refused the change", op.GetResult().GetLegacyRule().GetMessage())
	})
}

// A forward node's token is pinned to its endpoint (section 3.8): a change
// of the row's address that did not go through an administrator's forward
// node update ends ENDPOINT_UNCONFIRMED and presents nothing; the
// administrator's update moves the pin; a node without an API port holds no
// pin and is never presented to an address.
func TestEndpointPinning(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newForwardHarness(t, db)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsForward))
		nodes := service.NewForwardNodeService(db)

		// A package writes the row's host: the next apply does not present
		// the token there.
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 10).Update("host", "203.0.113.99").Error)
		op := submitWait(t, client, "forward.apply:40:moved", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE))
		require.Equal(t, failed, op.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_ENDPOINT_UNCONFIRMED, op.GetError().GetCode())
		require.False(t, op.GetError().GetRetryable())
		require.NotContains(t, op.GetError().GetMessage(), "203.0.113.99", "the error names no address")
		require.Empty(t, h.nodex.received(), "nothing was sent")
		require.Equal(t, model.ForwardRuntimeJobStatusFailed, h.forward(t, 40).RuntimeStatus)
		require.NotContains(t, h.forward(t, 40).RuntimeMessage, "relay-token")
		require.NotZero(t, nodesecrets.PinCount(nodesecrets.PinUnconfirmed))

		// The administrator's own update of the node confirms the address.
		node, err := nodes.GetByID(10)
		require.NoError(t, err)
		require.NoError(t, nodes.Update(node))
		op = submitWait(t, client, "forward.apply:40:confirmed", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE))
		require.Equal(t, succeeded, op.GetState(), "%v", op.GetError())
		sent := h.nodex.received()
		require.Len(t, sent, 1)
		require.Contains(t, sent[0], `"host":"203.0.113.99"`)
		require.Contains(t, sent[0], `"apiToken":"relay-token-10"`)

		// A changed API port is a changed endpoint too, and so is a removed
		// one: a node without an API port is pinned to nothing.
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 10).Update("api_port", 18081).Error)
		op = submitWait(t, client, "forward.apply:40:port", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_ENDPOINT_UNCONFIRMED, op.GetError().GetCode())
		node, err = nodes.GetByID(10)
		require.NoError(t, err)
		node.APIPort = 0
		require.NoError(t, nodes.Update(node))
		op = submitWait(t, client, "forward.apply:40:noport", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_ENDPOINT_UNCONFIRMED, op.GetError().GetCode(), "no API port: the token is presented nowhere")
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 10).Update("api_port", 18080).Error)
		op = submitWait(t, client, "forward.apply:40:portback", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_ENDPOINT_UNCONFIRMED, op.GetError().GetCode(), "a port written by a package does not confirm")
		require.Empty(t, h.nodex.received())
		node, err = nodes.GetByID(10)
		require.NoError(t, err)
		require.NoError(t, nodes.Update(node))
		op = submitWait(t, client, "forward.apply:40:portconfirmed", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE))
		require.Equal(t, succeeded, op.GetState(), "%v", op.GetError())
		h.nodex.received()

		// A legacy rule needs both of its nodes confirmed.
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 12).Update("host", "203.0.113.98").Error)
		op = submitWait(t, client, "forward.legacy_rule:50:moved", legacyRule(50, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_ENDPOINT_UNCONFIRMED, op.GetError().GetCode())
		require.Empty(t, h.nodex.received())
	})
}

// Results and errors never carry a node token: NodeX's refusal echoing the
// request is scrubbed, and the ledger holds none.
func TestForwardResultsCarryNoToken(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newForwardHarness(t, db)
		h.start(t)
		client := h.client(forwardHost, allow(service.CapabilityNodeOpsForward))
		h.rename(t, 40, "echo")

		op := submitWait(t, client, "forward.apply:40:echo", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE))
		require.Equal(t, failed, op.GetState())
		require.Contains(t, op.GetError().GetMessage(), "nodex refused")
		require.NotContains(t, op.GetError().GetMessage(), "relay-token-10")
		require.Contains(t, op.GetError().GetMessage(), service.NodeSecretPlaceholder)
		ledger := h.ledgerText(t)
		require.NotContains(t, ledger, "relay-token-10")
		require.NotContains(t, ledger, "nodex-shared-token")
		var jobs []model.ForwardRuntimeJob
		require.NoError(t, db.Find(&jobs).Error)
		for _, job := range jobs {
			require.NotContains(t, job.Payload, "relay-token")
		}
	})
}
