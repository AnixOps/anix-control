package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

const (
	defaultForwardRuntimeJobPollInterval = 5 * time.Second
	defaultForwardRuntimeJobBatchSize    = 10
	defaultForwardRuntimeJobTimeout      = 2 * time.Minute
	defaultAnsibleCommand                = "ansible-playbook"
)

type panelForwardRuntimeCommandRunner interface {
	Run(ctx context.Context, command string, args []string, workdir string, env map[string]string) (string, error)
}

type osExecPanelForwardRuntimeCommandRunner struct{}

func (r osExecPanelForwardRuntimeCommandRunner) Run(ctx context.Context, command string, args []string, workdir string, env map[string]string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if strings.TrimSpace(workdir) != "" {
		cmd.Dir = workdir
	}
	if len(env) > 0 {
		cmd.Env = os.Environ()
		for key, value := range env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}

type PanelForwardRuntimeJobExecutor struct {
	db           *gorm.DB
	runner       panelForwardRuntimeCommandRunner
	pollInterval time.Duration
	batchSize    int
	jobTimeout   time.Duration
}

type panelForwardAnsibleTargetPayload struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}

type panelForwardAnsibleForwardPayload struct {
	ID            uint   `json:"id"`
	UserID        uint   `json:"userId"`
	Name          string `json:"name"`
	InPort        int    `json:"inPort"`
	RemoteAddr    string `json:"remoteAddr"`
	InterfaceName string `json:"interfaceName"`
	Strategy      string `json:"strategy"`
	Status        int    `json:"status"`
}

type panelForwardAnsibleTunnelPayload struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	InNodeID      uint   `json:"inNodeId"`
	Protocol      string `json:"protocol"`
	TCPListenAddr string `json:"tcpListenAddr"`
	UDPListenAddr string `json:"udpListenAddr"`
	InterfaceName string `json:"interfaceName"`
}

type panelForwardAnsibleNodePayload struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	APIPort int    `json:"apiPort"`
}

type panelForwardAnsibleRuntimePayload struct {
	Action         string                             `json:"action"`
	Inventory      string                             `json:"inventory"`
	Playbook       string                             `json:"playbook"`
	Become         bool                               `json:"become"`
	ExtraVars      map[string]interface{}             `json:"extraVars,omitempty"`
	Forward        panelForwardAnsibleForwardPayload  `json:"forward"`
	Tunnel         panelForwardAnsibleTunnelPayload   `json:"tunnel"`
	Node           panelForwardAnsibleNodePayload     `json:"node"`
	Limiter        *panelForwardLimiterPayload        `json:"limiter,omitempty"`
	Targets        []panelForwardAnsibleTargetPayload `json:"targets"`
	Command        string                             `json:"command,omitempty"`
	WorkingDir     string                             `json:"workingDir,omitempty"`
	TargetPattern  string                             `json:"targetPattern,omitempty"`
	Environment    map[string]string                  `json:"environment,omitempty"`
	TimeoutSeconds int                                `json:"timeoutSeconds,omitempty"`
}

func NewPanelForwardRuntimeJobExecutor(db *gorm.DB) *PanelForwardRuntimeJobExecutor {
	return &PanelForwardRuntimeJobExecutor{
		db:           db,
		runner:       osExecPanelForwardRuntimeCommandRunner{},
		pollInterval: defaultForwardRuntimeJobPollInterval,
		batchSize:    defaultForwardRuntimeJobBatchSize,
		jobTimeout:   defaultForwardRuntimeJobTimeout,
	}
}

func (e *PanelForwardRuntimeJobExecutor) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := e.requeueRunningJobs(); err != nil {
		log.Printf("forward runtime executor requeue failed: %v", err)
	}
	if err := e.RunPendingJobs(ctx); err != nil {
		log.Printf("forward runtime executor initial run failed: %v", err)
	}

	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.RunPendingJobs(ctx); err != nil {
				log.Printf("forward runtime executor cycle failed: %v", err)
			}
		}
	}
}

func (e *PanelForwardRuntimeJobExecutor) RunPendingJobs(ctx context.Context) error {
	for {
		processed, err := e.processNext(ctx)
		if err != nil {
			return err
		}
		if !processed {
			return nil
		}
	}
}

func (e *PanelForwardRuntimeJobExecutor) processNext(ctx context.Context) (bool, error) {
	var jobs []model.ForwardRuntimeJob
	if err := e.db.
		Where("backend = ? AND status = ?", model.ForwardRuntimeBackendIptablesAnsible, model.ForwardRuntimeJobStatusPending).
		Order("id ASC").
		Limit(e.batchSize).
		Find(&jobs).Error; err != nil {
		return false, err
	}

	for idx := range jobs {
		claimed, err := e.claimJob(&jobs[idx])
		if err != nil {
			return false, err
		}
		if !claimed {
			continue
		}
		return true, e.executeClaimedJob(ctx, &jobs[idx])
	}

	return false, nil
}

func (e *PanelForwardRuntimeJobExecutor) claimJob(job *model.ForwardRuntimeJob) (bool, error) {
	now := time.Now()
	result := e.db.Model(&model.ForwardRuntimeJob{}).
		Where("id = ? AND status = ?", job.ID, model.ForwardRuntimeJobStatusPending).
		Updates(map[string]interface{}{
			"status":     model.ForwardRuntimeJobStatusRunning,
			"started_at": &now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}

	job.Status = model.ForwardRuntimeJobStatusRunning
	job.StartedAt = &now
	return true, nil
}

func (e *PanelForwardRuntimeJobExecutor) executeClaimedJob(ctx context.Context, job *model.ForwardRuntimeJob) error {
	payload := &panelForwardAnsibleRuntimePayload{}
	if err := json.Unmarshal([]byte(job.Payload), payload); err != nil {
		return e.finishJobFailure(job, payload, fmt.Sprintf("invalid ansible payload: %v", err), "")
	}

	args, err := payload.commandArgs()
	if err != nil {
		return e.finishJobFailure(job, payload, err.Error(), "")
	}

	jobCtx, cancel := context.WithTimeout(ctx, payload.timeout(e.jobTimeout))
	defer cancel()

	output, runErr := e.runner.Run(jobCtx, payload.commandName(), args, payload.workingDirectory(), payload.environment())
	if runErr != nil {
		message := strings.TrimSpace(output)
		if message == "" {
			message = runErr.Error()
		} else {
			message = fmt.Sprintf("%s\n%s", runErr.Error(), message)
		}
		return e.finishJobFailure(job, payload, message, output)
	}

	return e.finishJobSuccess(job, payload, strings.TrimSpace(output))
}

func (e *PanelForwardRuntimeJobExecutor) finishJobSuccess(job *model.ForwardRuntimeJob, payload *panelForwardAnsibleRuntimePayload, output string) error {
	now := time.Now()
	message := payload.successMessage()
	if err := e.db.Model(&model.ForwardRuntimeJob{}).
		Where("id = ?", job.ID).
		Updates(map[string]interface{}{
			"status":       model.ForwardRuntimeJobStatusSuccess,
			"result":       output,
			"error":        "",
			"completed_at": &now,
		}).Error; err != nil {
		return err
	}

	return e.updateForwardRuntimeState(job, payload, model.ForwardRuntimeJobStatusSuccess, message, successForwardStatusForAction(job.Action), &now)
}

func (e *PanelForwardRuntimeJobExecutor) finishJobFailure(job *model.ForwardRuntimeJob, payload *panelForwardAnsibleRuntimePayload, message, output string) error {
	now := time.Now()
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		trimmed = "ansible runtime execution failed"
	}
	if err := e.db.Model(&model.ForwardRuntimeJob{}).
		Where("id = ?", job.ID).
		Updates(map[string]interface{}{
			"status":       model.ForwardRuntimeJobStatusFailed,
			"result":       strings.TrimSpace(output),
			"error":        trimmed,
			"completed_at": &now,
		}).Error; err != nil {
		return err
	}

	return e.updateForwardRuntimeState(job, payload, model.ForwardRuntimeJobStatusFailed, trimmed, failedForwardStatusForAction(job.Action), &now)
}

func (e *PanelForwardRuntimeJobExecutor) updateForwardRuntimeState(job *model.ForwardRuntimeJob, payload *panelForwardAnsibleRuntimePayload, runtimeStatus int, message string, forwardStatus *int, syncedAt *time.Time) error {
	if job.ForwardID == nil || *job.ForwardID == 0 {
		return nil
	}

	updates := map[string]interface{}{
		"runtime_backend":      model.ForwardRuntimeBackendIptablesAnsible,
		"runtime_status":       runtimeStatus,
		"runtime_message":      strings.TrimSpace(message),
		"runtime_last_sync_at": syncedAt,
	}
	if forwardStatus != nil {
		updates["status"] = *forwardStatus
	}

	result := e.db.Model(&model.Forward{}).Where("id = ?", *job.ForwardID).Updates(updates)
	return result.Error
}

func (e *PanelForwardRuntimeJobExecutor) requeueRunningJobs() error {
	return e.db.Model(&model.ForwardRuntimeJob{}).
		Where("backend = ? AND status = ?", model.ForwardRuntimeBackendIptablesAnsible, model.ForwardRuntimeJobStatusRunning).
		Updates(map[string]interface{}{
			"status":     model.ForwardRuntimeJobStatusPending,
			"started_at": nil,
		}).Error
}

func (p *panelForwardAnsibleRuntimePayload) commandName() string {
	if strings.TrimSpace(p.Command) == "" {
		return defaultAnsibleCommand
	}
	return strings.TrimSpace(p.Command)
}

func (p *panelForwardAnsibleRuntimePayload) workingDirectory() string {
	return strings.TrimSpace(p.WorkingDir)
}

func (p *panelForwardAnsibleRuntimePayload) environment() map[string]string {
	if len(p.Environment) == 0 {
		return nil
	}
	result := make(map[string]string, len(p.Environment))
	for key, value := range p.Environment {
		result[key] = value
	}
	return result
}

func (p *panelForwardAnsibleRuntimePayload) timeout(fallback time.Duration) time.Duration {
	if p.TimeoutSeconds > 0 {
		return time.Duration(p.TimeoutSeconds) * time.Second
	}
	return fallback
}

func (p *panelForwardAnsibleRuntimePayload) commandArgs() ([]string, error) {
	if strings.TrimSpace(p.Inventory) == "" {
		return nil, fmt.Errorf("inventory is required")
	}
	if strings.TrimSpace(p.Playbook) == "" {
		return nil, fmt.Errorf("playbook is required")
	}

	extraVars, err := json.Marshal(p.buildExtraVars())
	if err != nil {
		return nil, fmt.Errorf("marshal ansible extra vars: %w", err)
	}

	args := []string{"-i", strings.TrimSpace(p.Inventory)}
	if limit := p.limitPattern(); limit != "" {
		args = append(args, "--limit", limit)
	}
	if p.Become {
		args = append(args, "--become")
	}
	args = append(args, "--extra-vars", string(extraVars), strings.TrimSpace(p.Playbook))
	return args, nil
}

func (p *panelForwardAnsibleRuntimePayload) buildExtraVars() map[string]interface{} {
	result := map[string]interface{}{}
	for key, value := range p.ExtraVars {
		result[key] = value
	}
	result["runtimeAction"] = p.Action
	result["forward"] = p.Forward
	result["tunnel"] = p.Tunnel
	result["node"] = p.Node
	if p.Limiter != nil {
		result["limiter"] = p.Limiter
	}
	result["targets"] = p.Targets
	result["forwardId"] = p.Forward.ID
	result["tunnelId"] = p.Tunnel.ID
	result["nodeId"] = p.Node.ID
	if limit := p.limitPattern(); limit != "" {
		result["ansibleLimit"] = limit
	}
	return result
}

func (p *panelForwardAnsibleRuntimePayload) limitPattern() string {
	pattern := strings.TrimSpace(p.TargetPattern)
	if pattern == "" {
		pattern = strings.TrimSpace(p.Node.Host)
	}
	if pattern == "" {
		return ""
	}

	replacer := strings.NewReplacer(
		"{{node.host}}", p.Node.Host,
		"{{node.name}}", p.Node.Name,
		"{{node.id}}", fmt.Sprintf("%d", p.Node.ID),
		"{{forward.id}}", fmt.Sprintf("%d", p.Forward.ID),
		"{{tunnel.id}}", fmt.Sprintf("%d", p.Tunnel.ID),
	)
	return strings.TrimSpace(replacer.Replace(pattern))
}

func (p *panelForwardAnsibleRuntimePayload) successMessage() string {
	if p.Action == model.ForwardRuntimeJobActionDelete || p.Action == model.ForwardRuntimeJobActionPause {
		return "ansible runtime removed"
	}
	return "ansible runtime synchronized"
}

func successForwardStatusForAction(action string) *int {
	var status int
	switch action {
	case model.ForwardRuntimeJobActionPause:
		status = model.ForwardStatusPaused
		return &status
	case model.ForwardRuntimeJobActionCreate, model.ForwardRuntimeJobActionUpdate, model.ForwardRuntimeJobActionResume, model.ForwardRuntimeJobActionSync:
		status = model.ForwardStatusActive
		return &status
	default:
		return nil
	}
}

func failedForwardStatusForAction(action string) *int {
	if action == model.ForwardRuntimeJobActionDelete {
		return nil
	}
	status := model.ForwardStatusError
	return &status
}
