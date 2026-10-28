package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// The forward node check, which the KernelNodeOps diagnose executor
// (internal/kernelnodeops, node-ops-service.md section 3.3) runs from
// Control.

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

// PublicHost resolves the host of a probe Control runs for a user and
// answers the address to connect to, or the refusal (publicProbeAddress):
// loopback, private and other non-public addresses are refused.
func (p DiagnosisProbes) PublicHost(ctx context.Context, host string) (netip.Addr, string) {
	return publicProbeAddressWith(ctx, p.lookup(), host)
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
