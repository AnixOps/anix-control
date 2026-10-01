package service

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

	"github.com/AnixOps/anix-control/v4/internal/gost"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// The forward diagnoses: the forward node check, the node statistics, and
// the forward and tunnel diagnoses. The legacy routes and the KernelNodeOps
// diagnose executors (internal/kernelnodeops, node-ops-service.md section
// 3.3) run these same functions, from Control, so both compute the same
// result.

const (
	// forwardNodeCheckTimeout bounds the forward node check's dial.
	forwardNodeCheckTimeout = 5 * time.Second
	// DefaultDiagnosisConcurrency bounds the probes one diagnosis runs at
	// once.
	DefaultDiagnosisConcurrency = 8
)

// Errors of the diagnoses' lookups. Their texts are what the legacy routes
// answer.
var (
	ErrPanelForwardNotFound     = errors.New("转发不存在")
	ErrPanelTunnelNotFound      = errors.New("隧道不存在")
	ErrPanelForwardNodeNotFound = errors.New("节点不存在")
)

// DiagnosisProbes is how a diagnosis reaches the network. The zero value
// dials with net.Dialer and resolves names with the system resolver, as the
// legacy routes do; tests replace both.
type DiagnosisProbes struct {
	// Dial opens a probe's TCP connection within timeout.
	Dial func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error)
	// Lookup resolves the host name of a target a user chose.
	Lookup func(ctx context.Context, host string) ([]netip.Addr, error)
	// Concurrency bounds the probes in flight; DefaultDiagnosisConcurrency
	// when zero.
	Concurrency int
}

func (p DiagnosisProbes) dial(ctx context.Context, address string, timeout time.Duration) (net.Conn, error) {
	if p.Dial != nil {
		return p.Dial(ctx, "tcp", address, timeout)
	}
	dialer := net.Dialer{Timeout: timeout}
	return dialer.DialContext(ctx, "tcp", address)
}

func (p DiagnosisProbes) lookup() probeLookupFunc {
	if p.Lookup != nil {
		return p.Lookup
	}
	return probeLookup
}

// each calls probe for 0 to n-1, at most Concurrency at once. It starts no
// probe after ctx ended, waits for those it started, and returns ctx's
// error then.
func (p DiagnosisProbes) each(ctx context.Context, n int, probe func(i int)) error {
	limit := p.Concurrency
	if limit <= 0 {
		limit = DefaultDiagnosisConcurrency
	}
	slots := make(chan struct{}, limit)
	var running sync.WaitGroup
	for i := 0; i < n && ctx.Err() == nil; i++ {
		select {
		case <-ctx.Done():
			continue
		case slots <- struct{}{}:
		}
		running.Add(1)
		go func(i int) {
			defer running.Done()
			defer func() { <-slots }()
			probe(i)
		}(i)
	}
	running.Wait()
	return ctx.Err()
}

// ProbeEndpoint dials node's service port (host:port), as the forward node
// check does: online with the latency when it connects, offline with the
// dial's error when it does not. A check whose ctx ended is no check: ctx's
// error is returned.
func (p DiagnosisProbes) ProbeEndpoint(ctx context.Context, node *model.ForwardNode) (*HealthCheckResult, error) {
	start := time.Now()
	result := &HealthCheckResult{NodeID: node.ID, CheckTime: start}
	conn, err := p.dial(ctx, fmt.Sprintf("%s:%d", node.Host, node.Port), forwardNodeCheckTimeout)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		result.Status = model.ForwardNodeStatusOffline
		result.Error = err.Error()
		return result, nil
	}
	if err := conn.Close(); err != nil {
		return nil, fmt.Errorf("close health check connection: %w", err)
	}
	result.Status = model.ForwardNodeStatusOnline
	result.Latency = time.Since(start).Milliseconds()
	return result, nil
}

// RecordHealthCheck writes a check's result to the node's row, as the
// forward node check does: status, last check and latency, and the uptime
// when the node is online.
func (s *ForwardNodeService) RecordHealthCheck(node *model.ForwardNode, result *HealthCheckResult) error {
	now := time.Now()
	updates := map[string]any{
		"status":     result.Status,
		"last_check": now,
		"latency":    result.Latency,
	}

	if result.Status == model.ForwardNodeStatusOnline {
		// 计算在线率
		var stats struct {
			Total  int64
			Online int64
		}
		if err := s.db.Model(&model.ForwardNode{}).
			Where("id = ?", node.ID).
			Select("COUNT(*) as total, SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as online", model.ForwardNodeStatusOnline).
			Scan(&stats).Error; err != nil {
			return fmt.Errorf("calculate forward node uptime: %w", err)
		}
		if stats.Total > 0 {
			updates["uptime"] = float64(stats.Online) / float64(stats.Total) * 100
		}
	}

	if err := s.db.Model(&model.ForwardNode{}).Where("id = ?", node.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("update forward node health check: %w", err)
	}
	return nil
}

// CheckEndpoint checks one node's endpoint from Control and, with record,
// writes what it found to the node's row.
func (s *ForwardNodeService) CheckEndpoint(ctx context.Context, probes DiagnosisProbes, nodeID uint, record bool) (*HealthCheckResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	node, err := s.GetByID(nodeID)
	if err != nil {
		return nil, err
	}
	result, err := probes.ProbeEndpoint(ctx, node)
	if err != nil {
		return nil, err
	}
	if record {
		if err := s.RecordHealthCheck(node, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// EndpointCheck is one node's check in CheckEndpoints.
type EndpointCheck struct {
	NodeID uint
	// Node is the node's row; nil when the node does not exist.
	Node *model.ForwardNode
	// Result is the check's result; nil when it did not run or failed.
	Result *HealthCheckResult
	// Err is why there is no result: gorm.ErrRecordNotFound for a node that
	// does not exist, or the check's error.
	Err error
}

// CheckEndpoints checks the nodes' endpoints from Control, at most
// probes.Concurrency at once, and with record writes each result to its
// node's row. When ctx ends it returns the checks that completed and ctx's
// error, and records nothing. Another error is a failure to load the nodes.
func (s *ForwardNodeService) CheckEndpoints(ctx context.Context, probes DiagnosisProbes, nodeIDs []uint, record bool) ([]EndpointCheck, error) {
	checks := make([]EndpointCheck, len(nodeIDs))
	for i, id := range nodeIDs {
		checks[i].NodeID = id
		node, err := s.GetByID(id)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			checks[i].Err = err
			continue
		}
		checks[i].Node = node
	}
	completed := make([]bool, len(checks))
	err := probes.each(ctx, len(checks), func(i int) {
		if checks[i].Node == nil {
			completed[i] = true
			return
		}
		result, err := probes.ProbeEndpoint(ctx, checks[i].Node)
		if err != nil && ctx.Err() != nil {
			return
		}
		checks[i].Result, checks[i].Err, completed[i] = result, err, true
	})
	if err != nil {
		var done []EndpointCheck
		for i := range checks {
			if completed[i] {
				done = append(done, checks[i])
			}
		}
		return done, err
	}
	if record {
		for i := range checks {
			if checks[i].Result == nil {
				continue
			}
			if err := s.RecordHealthCheck(checks[i].Node, checks[i].Result); err != nil {
				checks[i].Err = err
			}
		}
	}
	return checks, nil
}

// ForwardNodeTraffic is what the statistics sync reads from a forward node.
type ForwardNodeTraffic struct {
	// Ansible is set for an Ansible machine: it exposes no gost metrics, so
	// its panel-side counters are kept.
	Ansible bool
	// Services are a gost node's services' totals from its metrics
	// endpoint.
	Services map[string]*gost.ServiceTrafficTotals
	// CurrentConn, TotalUpload and TotalDownload are an Ansible machine's
	// counters.
	CurrentConn   int
	TotalUpload   int64
	TotalDownload int64
}

// CollectForwardNodeTraffic reads node's traffic as the statistics sync
// does: an Ansible machine's panel-side counters, or every gost service's
// totals from the node's Prometheus metrics endpoint, through manager.
func CollectForwardNodeTraffic(ctx context.Context, manager *gost.Manager, node *model.ForwardNode, ansible bool) (*ForwardNodeTraffic, error) {
	if ansible {
		return &ForwardNodeTraffic{Ansible: true, CurrentConn: node.CurrentConn, TotalUpload: node.TotalUpload, TotalDownload: node.TotalDownload}, nil
	}
	services, err := manager.GetNodeTrafficTotals(ctx, node.ID)
	if err != nil {
		return nil, err
	}
	return &ForwardNodeTraffic{Services: services}, nil
}

// DiagnoseForward diagnoses an actor's forward from Control. It is
// DiagnoseForwardContext without a deadline.
func (s *PanelForwardService) DiagnoseForward(userID uint, isAdmin bool, forwardID uint) (*DiagnosisReport, error) {
	return s.DiagnoseForwardContext(context.Background(), DiagnosisProbes{}, userID, isAdmin, forwardID)
}

// DiagnoseForwardContext diagnoses the actor's forward from Control: it
// dials each of the forward's targets. A user's diagnosis dials public
// targets only (publicProbeAddress); an administrator's dials every target.
func (s *PanelForwardService) DiagnoseForwardContext(ctx context.Context, probes DiagnosisProbes, userID uint, isAdmin bool, forwardID uint) (*DiagnosisReport, error) {
	record, err := s.getForwardForActor(forwardID, userID, isAdmin)
	if err != nil {
		return nil, err
	}
	return s.diagnoseForward(ctx, probes, record, !isAdmin)
}

// DiagnoseForwardTargets diagnoses forward forwardID from Control for the
// kernel: the forward is not scoped to an actor, and with publicOnly only
// public targets are dialled, as for a user.
func (s *PanelForwardService) DiagnoseForwardTargets(ctx context.Context, probes DiagnosisProbes, forwardID uint, publicOnly bool) (*DiagnosisReport, error) {
	record, err := s.getForwardForActor(forwardID, 0, true)
	if err != nil {
		return nil, err
	}
	return s.diagnoseForward(ctx, probes, record, publicOnly)
}

// diagnoseForward dials record's targets. When ctx ends it returns the
// probes that completed and ctx's error.
func (s *PanelForwardService) diagnoseForward(ctx context.Context, probes DiagnosisProbes, record *model.Forward, publicOnly bool) (*DiagnosisReport, error) {
	if err := s.db.Preload("Tunnel").First(record, record.ID).Error; err != nil {
		return nil, err
	}
	tunnelName := resolveTunnelName(record.Tunnel)
	tunnelID := fmt.Sprintf("%d", record.TunnelID)
	var targets []string
	for _, raw := range strings.Split(record.RemoteAddr, ",") {
		if target := strings.TrimSpace(raw); target != "" {
			targets = append(targets, target)
		}
	}
	outcomes := make([]DiagnosisOutcome, len(targets))
	completed := make([]bool, len(targets))
	err := probes.each(ctx, len(targets), func(i int) {
		outcomes[i], completed[i] = probes.probeForwardTarget(ctx, targets[i], publicOnly)
		outcomes[i].Description = "转发->目标"
		outcomes[i].NodeName = tunnelName
		outcomes[i].NodeID = tunnelID
	})
	results := completedOutcomes(outcomes, completed)
	if err == nil && len(results) == 0 {
		results = append(results, DiagnosisOutcome{
			Success:     false,
			Description: "转发->目标",
			NodeName:    tunnelName,
			NodeID:      tunnelID,
			TargetIP:    "-",
			Message:     "没有可诊断的目标地址",
		})
	}
	return &DiagnosisReport{
		ForwardName: record.Name,
		Timestamp:   time.Now().UnixMilli(),
		Results:     results,
	}, err
}

// probeForwardTarget dials one of a forward's targets. It reports false
// when ctx ended before the probe completed.
func (p DiagnosisProbes) probeForwardTarget(ctx context.Context, target string, publicOnly bool) (DiagnosisOutcome, bool) {
	host, port, err := splitTarget(target)
	if err != nil {
		return DiagnosisOutcome{TargetIP: target, Message: err.Error()}, true
	}
	dialTarget := target
	if publicOnly {
		// The probe runs from Control: a user's target must not reach
		// Control's own network (see publicProbeAddress).
		address, refusal := publicProbeAddressWith(ctx, p.lookup(), host)
		if refusal != "" {
			if ctx.Err() != nil {
				return DiagnosisOutcome{}, false
			}
			return DiagnosisOutcome{TargetIP: host, TargetPort: port, Message: refusal}, true
		}
		dialTarget = net.JoinHostPort(address.String(), strconv.Itoa(port))
	}
	start := time.Now()
	conn, dialErr := p.dial(ctx, dialTarget, diagnosisTimeout)
	elapsed := time.Since(start)
	if dialErr == nil {
		_ = conn.Close()
		return DiagnosisOutcome{Success: true, TargetIP: host, TargetPort: port, AverageTime: float64(elapsed.Milliseconds()), PacketLoss: 0}, true
	}
	if ctx.Err() != nil {
		return DiagnosisOutcome{}, false
	}
	return DiagnosisOutcome{TargetIP: host, TargetPort: port, Message: dialErr.Error()}, true
}

func completedOutcomes(outcomes []DiagnosisOutcome, completed []bool) []DiagnosisOutcome {
	results := make([]DiagnosisOutcome, 0, len(outcomes))
	for i := range outcomes {
		if completed[i] {
			results = append(results, outcomes[i])
		}
	}
	return results
}

// DiagnoseTunnel diagnoses a tunnel from Control. It is
// DiagnoseTunnelContext without a deadline.
func (s *PanelForwardService) DiagnoseTunnel(id uint) (*TunnelDiagnosisReport, error) {
	return s.DiagnoseTunnelContext(context.Background(), DiagnosisProbes{}, id)
}

// tunnelCheck is one node a tunnel diagnosis dials.
type tunnelCheck struct {
	description string
	nodeLabel   string
	nodeID      uint
	host        string
	port        int
}

// DiagnoseTunnelContext diagnoses a tunnel from Control: it dials the
// tunnel's execution node (job backends), or its entry node and, for a
// two-hop tunnel, its exit node. When ctx ends it returns the probes that
// completed and ctx's error.
func (s *PanelForwardService) DiagnoseTunnelContext(ctx context.Context, probes DiagnosisProbes, id uint) (*TunnelDiagnosisReport, error) {
	tunnel, err := s.getTunnelByID(id)
	if err != nil {
		return nil, err
	}
	backend, err := s.resolveRuntimeBackend()
	if err != nil {
		return nil, err
	}

	checks := make([]tunnelCheck, 0, 2)
	if isForwardRuntimeExecutionNodeBackend(backend) {
		executionNodeID := storedPanelTunnelExecutionNodeID(tunnel)
		if executionNodeID > 0 {
			executionNode, err := s.getForwardNodeByID(executionNodeID)
			if err != nil {
				return nil, err
			}
			checks = append(checks, tunnelCheck{
				description: "管理端->中转执行节点",
				nodeLabel:   executionNode.Name,
				nodeID:      executionNode.ID,
				host:        strings.TrimSpace(executionNode.Host),
				port:        executionNode.Port,
			})
		}
	} else if tunnel.InNodeID > 0 {
		inNode, err := s.getForwardNodeByID(tunnel.InNodeID)
		if err != nil {
			return nil, err
		}
		checks = append(checks, tunnelCheck{
			description: "管理端->入口节点",
			nodeLabel:   inNode.Name,
			nodeID:      inNode.ID,
			host:        strings.TrimSpace(inNode.Host),
			port:        inNode.Port,
		})
	}

	if !isForwardRuntimeExecutionNodeBackend(backend) && tunnel.Type == 2 && tunnel.OutNodeID != nil {
		outNode, err := s.getForwardNodeByID(*tunnel.OutNodeID)
		if err != nil {
			return nil, err
		}
		checks = append(checks, tunnelCheck{
			description: "管理端->出口节点",
			nodeLabel:   outNode.Name,
			nodeID:      outNode.ID,
			host:        strings.TrimSpace(outNode.Host),
			port:        outNode.Port,
		})
	}

	outcomes := make([]DiagnosisOutcome, len(checks))
	completed := make([]bool, len(checks))
	err = probes.each(ctx, len(checks), func(i int) {
		outcomes[i], completed[i] = probes.probeTunnelNode(ctx, checks[i])
	})
	results := completedOutcomes(outcomes, completed)
	if err == nil && len(results) == 0 {
		results = append(results, DiagnosisOutcome{
			Success:     false,
			Description: "隧道诊断",
			NodeName:    resolveTunnelName(tunnel),
			NodeID:      fmt.Sprintf("%d", tunnel.ID),
			TargetIP:    "-",
			Message:     "没有可诊断的节点",
		})
	}

	return &TunnelDiagnosisReport{
		TunnelID:   tunnel.ID,
		TunnelName: tunnel.Name,
		TunnelType: resolvePanelTunnelTypeName(tunnel.Type),
		Timestamp:  time.Now().UnixMilli(),
		Results:    results,
	}, err
}

// probeTunnelNode dials one of a tunnel's nodes. It reports false when ctx
// ended before the probe completed.
func (p DiagnosisProbes) probeTunnelNode(ctx context.Context, check tunnelCheck) (DiagnosisOutcome, bool) {
	outcome := DiagnosisOutcome{
		Description: check.description,
		NodeName:    check.nodeLabel,
		NodeID:      fmt.Sprintf("%d", check.nodeID),
		TargetIP:    check.host,
		TargetPort:  check.port,
	}
	if check.host == "" || check.port <= 0 {
		outcome.Message = "节点地址未配置"
		return outcome, true
	}

	target := net.JoinHostPort(strings.Trim(check.host, "[]"), fmt.Sprintf("%d", check.port))
	start := time.Now()
	conn, dialErr := p.dial(ctx, target, diagnosisTimeout)
	elapsed := time.Since(start)
	if dialErr == nil {
		_ = conn.Close()
		outcome.Success = true
		outcome.AverageTime = float64(elapsed.Milliseconds())
		outcome.PacketLoss = 0
		return outcome, true
	}
	if ctx.Err() != nil {
		return DiagnosisOutcome{}, false
	}
	outcome.Message = dialErr.Error()
	return outcome, true
}
