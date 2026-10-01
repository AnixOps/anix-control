package kernelnodeops

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/gost"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

// The agent capability a node vantage needs (decision D10): diag.v1.
const (
	DiagnosisAgentCapability        = "diag"
	DiagnosisAgentCapabilityVersion = "v1"
)

// AgentDirectory answers what nodes' agents advertise on their live Agent
// Control sessions.
type AgentDirectory interface {
	// Advertises reports whether node's agent holds a session whose Hello
	// lists the capability name at version.
	Advertises(node *kernelnodeopsv1.NodeRef, name, version string) bool
}

// Diagnosis executes the diagnose family's kinds (node-ops-service.md
// section 3.3, NO-8): CheckEndpoints, CollectNodeStats, DiagnoseForward and
// DiagnoseTunnel. Each runs the kernel's implementation of the legacy route
// (internal/service), which the legacy handlers run too, so its result is
// what the route computes. Results carry no credential: the nodes' tokens
// are named to the engine's scrubber, and diagnoses send none.
//
// The vantage (D10) is Control by default; it would be the node when the
// agent of every node a diagnosis concerns advertises diag.v1. The kernel
// does not run diagnoses on agents yet (A2-5), so it dials from Control and
// says so in the result's VantageReport. A package host is never a vantage.
type Diagnosis struct {
	// Probes is how the diagnoses reach the network. The zero value dials
	// from Control, with the system resolver, at most
	// service.DefaultDiagnosisConcurrency probes at once per operation.
	Probes service.DiagnosisProbes
	// Agents answers which nodes' agents advertise diag.v1; nil answers
	// none. No forward node holds an Agent Control session before A2-1, so
	// the kernel's default has none.
	Agents AgentDirectory
	// Now stamps the results; time.Now when nil.
	Now func() time.Time
}

func (d *Diagnosis) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// Register serves the diagnose kinds on registry.
func (d *Diagnosis) Register(registry *Registry) error {
	for _, served := range []struct {
		kind     string
		executor ExecutorFunc
	}{
		{KindDiagnoseEndpoints, d.checkEndpoints},
		{KindDiagnoseNodeStats, d.collectNodeStats},
		{KindDiagnoseForward, d.diagnoseForward},
		{KindDiagnoseTunnel, d.diagnoseTunnel},
	} {
		if err := registry.Register(served.kind, served.executor); err != nil {
			return err
		}
	}
	return nil
}

// init serves the diagnose kinds on DefaultExecutors. It runs after
// kinds.go's init registered the kinds: a package's init functions run in
// the order of its file names.
func init() {
	if err := (&Diagnosis{}).Register(DefaultExecutors); err != nil {
		panic(err)
	}
}

// vantage chooses where a diagnosis of nodes runs and reports it.
func (d *Diagnosis) vantage(requested kernelnodeopsv1.Vantage, nodes []*kernelnodeopsv1.NodeRef) *kernelnodeopsv1.VantageReport {
	control := kernelnodeopsv1.Vantage_VANTAGE_CONTROL
	report := &kernelnodeopsv1.VantageReport{Requested: requested, Selected: control, Used: control}
	switch {
	case requested == control:
	case d.advertised(nodes):
		report.Selected = kernelnodeopsv1.Vantage_VANTAGE_NODE
		report.Note = "the node's agent advertises diag.v1, but this kernel does not run diagnoses on agents yet: it dialled from Control"
	case requested == kernelnodeopsv1.Vantage_VANTAGE_NODE:
		report.Note = "the node's agent does not advertise diag.v1: the kernel dialled from Control"
	}
	return report
}

// advertised reports whether the agent of every node advertises diag.v1.
func (d *Diagnosis) advertised(nodes []*kernelnodeopsv1.NodeRef) bool {
	if d.Agents == nil || len(nodes) == 0 {
		return false
	}
	for _, node := range nodes {
		if !d.Agents.Advertises(node, DiagnosisAgentCapability, DiagnosisAgentCapabilityVersion) {
			return false
		}
	}
	return true
}

// begin records that the kernel took the operation on channel. It reports
// false, with the outcome to end on, when the operation ended meanwhile.
func begin(ctx context.Context, run *Run, channel kernelnodeopsv1.Channel) (Outcome, bool) {
	if err := run.Accept(ctx, Acceptance{Channel: channel}); err != nil {
		if errors.Is(err, ErrOperationEnded) || ctx.Err() != nil {
			return Cancelled("the operation ended before it ran").WithChannel(channel), false
		}
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "recording the operation's start failed", true), false
	}
	return Outcome{}, true
}

// nodeID converts a checked node or row id (at most 2^32-1) to a row id.
func nodeID(id uint64) uint {
	return uint(id) // #nosec G115 -- the kinds' checks bound ids to 32 bits.
}

func forwardNode(id uint) *kernelnodeopsv1.NodeRef {
	return nodeRef(kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD, uint64(id))
}

// checkEndpoints dials the nodes' service ports, as the forward node check
// does, and with record_status writes what it found to their rows.
func (d *Diagnosis) checkEndpoints(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetCheckEndpoints()
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_CONTROL_DIAL); !ok {
		return outcome
	}
	ids := make([]uint, 0, len(op.GetNodes()))
	for _, node := range op.GetNodes() {
		ids = append(ids, nodeID(node.GetId()))
	}
	checks, err := service.NewForwardNodeService(run.engine.DB).CheckEndpoints(ctx, d.Probes, ids, op.GetRecordStatus())
	if err != nil && ctx.Err() == nil {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "loading the forward nodes failed", true)
	}
	result := &kernelnodeopsv1.EndpointCheckResult{}
	var gone, failed []string
	for _, check := range checks {
		ref := forwardNode(check.NodeID)
		item := &kernelnodeopsv1.EndpointCheck{Node: ref, Vantage: d.vantage(op.GetVantage(), []*kernelnodeopsv1.NodeRef{ref})}
		if check.Node != nil {
			run.UseSecret(nodesecrets.ForwardNodeToken(run.engine.DB, check.Node))
		}
		switch {
		case check.Node == nil:
			item.Error = fmt.Sprintf("forward node %d not found", check.NodeID)
			gone = append(gone, strconv.FormatUint(uint64(check.NodeID), 10))
		case check.Result == nil:
			item.Error = check.Err.Error()
			failed = append(failed, fmt.Sprintf("forward node %d: %s", check.NodeID, item.Error))
		default:
			item.Reachable = check.Result.Status == model.ForwardNodeStatusOnline
			item.LatencyMs = check.Result.Latency
			item.Error = check.Result.Error
			item.CheckedAtUnixMs = check.Result.CheckTime.UnixMilli()
			if check.Err != nil {
				failed = append(failed, fmt.Sprintf("forward node %d: %s", check.NodeID, check.Err.Error()))
			}
		}
		result.Checks = append(result.Checks, item)
	}
	wrapped := &kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_EndpointCheck{EndpointCheck: result}}
	switch {
	case ctx.Err() != nil:
		return Cancelled("the check stopped before every node was checked; nothing was recorded").WithResult(wrapped)
	case len(gone) > 0:
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, "forward nodes not found: "+strings.Join(gone, ", "), false).WithResult(wrapped)
	case len(failed) > 0:
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, strings.Join(failed, "; "), true).WithResult(wrapped)
	}
	return Succeeded(wrapped)
}

// collectNodeStats reads a forward node's traffic, as the statistics sync
// does: a gost node's metrics endpoint, or an Ansible machine's panel-side
// counters.
func (d *Diagnosis) collectNodeStats(ctx context.Context, run *Run) Outcome {
	id := nodeID(run.Operation.GetCollectNodeStats().GetNode().GetId())
	db := run.engine.DB
	nodes := service.NewForwardNodeService(db)
	node, err := nodes.GetByID(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, fmt.Sprintf("forward node %d not found", id), false)
	}
	if err != nil {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "loading the forward node failed", true)
	}
	run.UseSecret(nodesecrets.ForwardNodeToken(db, node))
	ansible := true
	if _, err := nodes.GetByIDForInventoryScope(id, service.ForwardNodeInventoryScopeAnsible); errors.Is(err, gorm.ErrRecordNotFound) {
		ansible = false
	} else if err != nil {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "loading the forward node failed", true)
	}
	channel := kernelnodeopsv1.Channel_CHANNEL_CONTROL_DIAL
	if ansible {
		channel = kernelnodeopsv1.Channel_CHANNEL_LOCAL_ANSIBLE
	}
	if outcome, ok := begin(ctx, run, channel); !ok {
		return outcome
	}
	traffic, err := service.CollectForwardNodeTraffic(ctx, gost.NewManager(db), node, ansible)
	if err != nil {
		switch {
		case ctx.Err() != nil:
			return Cancelled("the statistics collection stopped before it completed")
		case node.APIPort == 0 || node.MetricsPort <= 0:
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, err.Error(), false)
		}
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, err.Error(), true)
	}
	stats := &kernelnodeopsv1.NodeStatsResult{Node: forwardNode(id), CollectedAtUnixMs: d.now().UnixMilli()}
	if traffic.Ansible {
		stats.UploadBytes = counter(traffic.TotalUpload)
		stats.DownloadBytes = counter(traffic.TotalDownload)
		stats.CurrentConnections = int64(traffic.CurrentConn)
	} else {
		names := make([]string, 0, len(traffic.Services))
		for name := range traffic.Services {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			totals := traffic.Services[name]
			service := &kernelnodeopsv1.ServiceTraffic{Service: name, InputBytes: counter(totals.InBytes), OutputBytes: counter(totals.OutBytes)}
			stats.Services = append(stats.Services, service)
			stats.UploadBytes += service.GetInputBytes()
			stats.DownloadBytes += service.GetOutputBytes()
		}
	}
	return Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_NodeStats{NodeStats: stats}})
}

// counter answers a byte counter; a negative one is 0.
func counter(value int64) uint64 {
	if value < 0 {
		return 0
	}
	return uint64(value)
}

// diagnoseForward dials a forward's targets.
//
// For a user's forward the kernel dials public targets only, as the legacy
// route does. Request bindings name the actor, but the kernel verifies them
// only from NO-4 on; until then it cannot tell an administrator's diagnosis
// from a user's, so it checks every forward's targets as a user's and never
// dials a private or loopback target for a package.
func (d *Diagnosis) diagnoseForward(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetDiagnoseForward()
	vantage := d.vantage(op.GetVantage(), run.Targets)
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_CONTROL_DIAL); !ok {
		return outcome
	}
	nameTargetTokens(run)
	report, err := service.NewPanelForwardService(run.engine.DB).DiagnoseForwardTargets(ctx, d.Probes, nodeID(op.GetForwardId()), true)
	var outcomes []service.DiagnosisOutcome
	if report != nil {
		outcomes = report.Results
	}
	return diagnosisOutcome(ctx, outcomes, vantage, err)
}

// diagnoseTunnel dials a tunnel's nodes.
func (d *Diagnosis) diagnoseTunnel(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetDiagnoseTunnel()
	vantage := d.vantage(op.GetVantage(), run.Targets)
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_CONTROL_DIAL); !ok {
		return outcome
	}
	nameTargetTokens(run)
	report, err := service.NewPanelForwardService(run.engine.DB).DiagnoseTunnelContext(ctx, d.Probes, nodeID(op.GetTunnelId()))
	var outcomes []service.DiagnosisOutcome
	if report != nil {
		outcomes = report.Results
	}
	return diagnosisOutcome(ctx, outcomes, vantage, err)
}

// nameTargetTokens names the tokens of the forward nodes a diagnosis
// concerns to the engine's scrubber. A diagnosis sends no token, but a
// node's answer could echo one.
func nameTargetTokens(run *Run) {
	nodes := service.NewForwardNodeService(run.engine.DB)
	for _, target := range run.Targets {
		if target.GetKind() != kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD {
			continue
		}
		if node, err := nodes.GetByID(nodeID(target.GetId())); err == nil {
			run.UseSecret(nodesecrets.ForwardNodeToken(run.engine.DB, node))
		}
	}
}

// diagnosisOutcome maps a diagnosis report. Its probes' failures are the
// result; the operation fails only when the diagnosis could not run.
func diagnosisOutcome(ctx context.Context, outcomes []service.DiagnosisOutcome, vantage *kernelnodeopsv1.VantageReport, err error) Outcome {
	diagnosis := &kernelnodeopsv1.DiagnosisResult{Vantage: vantage}
	for _, outcome := range outcomes {
		diagnosis.Outcomes = append(diagnosis.Outcomes, &kernelnodeopsv1.DiagnosisOutcome{
			Success: outcome.Success, Description: outcome.Description, NodeName: outcome.NodeName, NodeId: outcome.NodeID,
			TargetIp: outcome.TargetIP, TargetPort: port(outcome.TargetPort), AverageTimeMs: outcome.AverageTime,
			PacketLoss: outcome.PacketLoss, Message: outcome.Message,
		})
	}
	result := &kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_Diagnosis{Diagnosis: diagnosis}}
	switch {
	case err == nil:
		return Succeeded(result)
	case ctx.Err() != nil:
		return Cancelled("the diagnosis stopped before every probe completed").WithResult(result)
	case errors.Is(err, service.ErrPanelForwardNotFound), errors.Is(err, service.ErrPanelTunnelNotFound),
		errors.Is(err, service.ErrPanelForwardNodeNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, err.Error(), false)
	}
	return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, err.Error(), true)
}

// port answers a port; one out of range is 0.
func port(value int) int32 {
	if value < 0 || value > math.MaxInt32 {
		return 0
	}
	return int32(value)
}
