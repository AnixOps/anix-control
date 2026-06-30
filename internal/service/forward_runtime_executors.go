package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/model"
)

// forwardRuntimeNodeRole 表示某个后端执行时需要解析的节点角色。
// gost 走入口节点 (ingress)，本地 ansible / clean_agent 走执行节点 (execution)。
type forwardRuntimeNodeRole int

const (
	forwardNodeRoleIngress   forwardRuntimeNodeRole = iota // gost：入口节点
	forwardNodeRoleExecution                              // ansible / clean_agent：执行节点
)

// forwardRuntimeExecContext 携带一次转发运行时执行所需的全部上下文。
type forwardRuntimeExecContext struct {
	action  string
	forward *model.Forward
	tunnel  *model.ForwardTunnel
	req     nodeXForwardExecuteRequest
	nodeID  *uint
}

// forwardRuntimeExecutor 是转发运行时后端的统一插件接口。
// 新增后端只需实现该接口并在 NewPanelForwardRuntimeService 注册，无需改动 Apply 主流程。
// 同步后端 (gost) 在 run 内直接执行并回写终态；异步后端 (ansible/clean_agent) 入队 pending 后立即返回。
type forwardRuntimeExecutor interface {
	backend() string
	nodeRole() forwardRuntimeNodeRole
	validate(action string) error
	run(ctx context.Context, ec forwardRuntimeExecContext) (*panelForwardRuntimeResult, error)
}

// ===== gost：同步执行（NodeX HTTP API），支持加密/协议中转 =====

type gostForwardExecutor struct {
	s *PanelForwardRuntimeService
}

func (e *gostForwardExecutor) backend() string                  { return model.ForwardRuntimeBackendGost }
func (e *gostForwardExecutor) nodeRole() forwardRuntimeNodeRole { return forwardNodeRoleIngress }

func (e *gostForwardExecutor) validate(action string) error {
	return e.s.validateBackendConfig(model.ForwardRuntimeBackendGost)
}

func (e *gostForwardExecutor) run(ctx context.Context, ec forwardRuntimeExecContext) (*panelForwardRuntimeResult, error) {
	s := e.s
	backend := model.ForwardRuntimeBackendGost

	payloadJSON, err := json.Marshal(ec.req)
	if err != nil {
		return failedPanelForwardRuntimeResult(backend, err), err
	}

	job := &model.ForwardRuntimeJob{
		Backend:      backend,
		Action:       ec.action,
		ResourceType: nodeXForwardResourceTypePanelForward,
		ResourceID:   uintPtr(ec.forward.ID),
		ForwardID:    uintPtr(ec.forward.ID),
		TunnelID:     uintPtr(ec.tunnel.ID),
		NodeID:       ec.nodeID,
		Payload:      string(payloadJSON),
	}

	startedAt := time.Now()
	job.Status = model.ForwardRuntimeJobStatusRunning
	job.StartedAt = &startedAt
	if err := s.db.Create(job).Error; err != nil {
		return failedPanelForwardRuntimeResult(backend, err), err
	}

	// 入口节点未配置时，删除/暂停动作直接视为成功（无规则可下发）。
	if ec.nodeID == nil &&
		(ec.action == model.ForwardRuntimeJobActionDelete || ec.action == model.ForwardRuntimeJobActionPause) {
		result := &panelForwardRuntimeResult{
			Backend: backend,
			Status:  model.ForwardRuntimeJobStatusSuccess,
			Message: "gost runtime skipped because ingress node is not configured",
		}
		completedAt := time.Now()
		if err := s.db.Model(&model.ForwardRuntimeJob{}).Where("id = ?", job.ID).Updates(map[string]any{
			"status":       result.Status,
			"result":       result.Message,
			"error":        "",
			"completed_at": &completedAt,
		}).Error; err != nil {
			result.Message = strings.TrimSpace(result.Message + "; local audit update failed: " + err.Error())
		}
		return result, nil
	}

	execResult, execErr := s.client.Execute(ctx, ec.req)
	result := buildPanelForwardRuntimeResult(backend, ec.action, execResult, execErr)
	if execErr == nil && result.Status == model.ForwardRuntimeJobStatusFailed {
		execErr = errors.New(result.Message)
	}

	updateValues := map[string]any{
		"status": result.Status,
		"result": "",
		"error":  "",
	}
	if execResult != nil {
		updateValues["result"] = strings.TrimSpace(execResult.Result)
	}
	if execErr != nil {
		updateValues["error"] = execErr.Error()
	} else if result.Status == model.ForwardRuntimeJobStatusFailed {
		updateValues["error"] = strings.TrimSpace(result.Message)
	}
	if isForwardRuntimeTerminalStatus(result.Status) {
		completedAt := time.Now()
		updateValues["completed_at"] = &completedAt
	} else {
		updateValues["completed_at"] = nil
	}

	if err := s.db.Model(&model.ForwardRuntimeJob{}).Where("id = ?", job.ID).Updates(updateValues).Error; err != nil {
		if execErr != nil {
			return result, execErr
		}
		result.Message = strings.TrimSpace(result.Message + "; local audit update failed: " + err.Error())
	}

	return result, execErr
}

// ===== nftables_ansible：异步入队，由 job executor 跑 ansible playbook 配 nftables 四层 NAT =====

type nftablesForwardExecutor struct {
	s *PanelForwardRuntimeService
}

func (e *nftablesForwardExecutor) backend() string {
	return model.ForwardRuntimeBackendNftablesAnsible
}
func (e *nftablesForwardExecutor) nodeRole() forwardRuntimeNodeRole { return forwardNodeRoleExecution }

func (e *nftablesForwardExecutor) validate(action string) error {
	return e.s.validateAnsiblePaths(model.ForwardRuntimeBackendNftablesAnsible, action)
}

func (e *nftablesForwardExecutor) run(ctx context.Context, ec forwardRuntimeExecContext) (*panelForwardRuntimeResult, error) {
	return e.s.enqueueLocalAnsibleJob(ec.action, ec.forward, ec.tunnel, ec.req, ec.nodeID)
}

// ===== clean_agent：异步入队，由 NAT 后节点的 agent 主动拉取执行 =====

type cleanAgentForwardExecutor struct {
	s *PanelForwardRuntimeService
}

func (e *cleanAgentForwardExecutor) backend() string {
	return model.ForwardRuntimeBackendCleanAgent
}
func (e *cleanAgentForwardExecutor) nodeRole() forwardRuntimeNodeRole { return forwardNodeRoleExecution }

func (e *cleanAgentForwardExecutor) validate(action string) error { return nil }

func (e *cleanAgentForwardExecutor) run(ctx context.Context, ec forwardRuntimeExecContext) (*panelForwardRuntimeResult, error) {
	return e.s.enqueueCleanAgentJob(ec.action, ec.forward, ec.tunnel, ec.req, ec.nodeID)
}

// buildForwardRuntimeExecutors 构造后端注册表。
// iptables_ansible 也注册并指向 nftables executor —— 兜底兼容旧数据，行为与 nftables 一致。
func buildForwardRuntimeExecutors(s *PanelForwardRuntimeService) map[string]forwardRuntimeExecutor {
	nft := &nftablesForwardExecutor{s: s}
	return map[string]forwardRuntimeExecutor{
		model.ForwardRuntimeBackendGost:            &gostForwardExecutor{s: s},
		model.ForwardRuntimeBackendNftablesAnsible: nft,
		model.ForwardRuntimeBackendIptablesAnsible: nft,
		model.ForwardRuntimeBackendCleanAgent:      &cleanAgentForwardExecutor{s: s},
	}
}
