package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

const (
	defaultForwardRuntimeNodeXHealthPath = "/health"
	defaultForwardRuntimeNodeXStatusPath = "/api/v2/internal/forward/runtime/status"

	panelForwardRuntimeAttachmentModelNodeXGost         = "nodex_gost_stateful"
	panelForwardRuntimeAttachmentModelCleanAgent        = "clean_agent_pull"
	panelForwardRuntimeIgnoredNodeXConfigWarning        = "NodeX base_url/token are configured but ignored while runtime backend is a local ansible backend"
	panelForwardRuntimeMissingNodeXBaseURLReason        = "NodeX mode requires forward.runtime.nodex.base_url before the panel can probe the control plane"
	panelForwardRuntimeMissingNodeXTokenReason          = "NodeX mode requires forward.runtime.nodex.token before runtime readiness can be confirmed" // #nosec G101 -- this names a configuration key, not a hardcoded credential.
	panelForwardRuntimeNodeXHealthSuccessReason         = "NodeX /health responded with ok from the panel host"
	panelForwardRuntimeLocalExecutorReachableReason     = "ansible-playbook is available on the panel host"
	panelForwardRuntimeLocalExecutorReadyReason         = "Local ansible executor resolved inventory/playbooks and is ready to queue jobs"
	panelForwardRuntimeCleanAgentReadyReason            = "At least one clean forward agent is online and can pull queued jobs"
	panelForwardRuntimeCleanAgentWaitingReason          = "No online clean forward agent is registered yet"
	panelForwardRuntimeNodeXRuntimeReadyReason          = "NodeX runtime status responded and advertises gost support"
	panelForwardRuntimeNodeXRuntimeMissingBackendReason = "NodeX runtime status responded, but gost support is not advertised yet"
)

type nodeXForwardRuntimeStatusEnvelope struct {
	Data    *nodeXForwardRuntimeStatus `json:"data,omitempty"`
	Error   string                     `json:"error,omitempty"`
	Msg     string                     `json:"msg,omitempty"`
	Message string                     `json:"message,omitempty"`
}

type nodeXForwardRuntimeStatus struct {
	Version      string                          `json:"version"`
	ExecutePath  string                          `json:"executePath"`
	StatusPath   string                          `json:"statusPath"`
	AuthRequired bool                            `json:"authRequired"`
	Supports     nodeXForwardRuntimeSupportState `json:"supports"`
	Modes        nodeXForwardRuntimeModes        `json:"modes"`
}

type nodeXForwardRuntimeSupportState struct {
	ResourceTypes []string `json:"resourceTypes"`
	Backends      []string `json:"backends"`
	Actions       []string `json:"actions"`
}

type nodeXForwardRuntimeModes struct {
	Gost            nodeXForwardRuntimeModeStatus    `json:"gost"`
	IptablesAnsible nodeXForwardRuntimeAnsibleStatus `json:"iptablesAnsible"`
}

type nodeXForwardRuntimeModeStatus struct {
	Supported bool `json:"supported"`
}

type nodeXForwardRuntimeAnsibleStatus struct {
	Backend              string   `json:"backend,omitempty"`
	FirewallDriver       string   `json:"firewallDriver,omitempty"`
	Supported            bool     `json:"supported"`
	Ready                bool     `json:"ready"`
	Command              string   `json:"command"`
	CommandFound         bool     `json:"commandFound"`
	InventoryPath        string   `json:"inventoryPath"`
	InventoryExists      bool     `json:"inventoryExists"`
	ApplyPlaybookPath    string   `json:"applyPlaybookPath"`
	ApplyPlaybookExists  bool     `json:"applyPlaybookExists"`
	RemovePlaybookPath   string   `json:"removePlaybookPath"`
	RemovePlaybookExists bool     `json:"removePlaybookExists"`
	WorkingDir           string   `json:"workingDir"`
	WorkingDirExists     bool     `json:"workingDirExists"`
	AnsibleConfigPath    string   `json:"ansibleConfigPath,omitempty"`
	AnsibleConfigExists  bool     `json:"ansibleConfigExists"`
	TargetPattern        string   `json:"targetPattern"`
	Become               bool     `json:"become"`
	TimeoutSeconds       int64    `json:"timeoutSeconds"`
	Issues               []string `json:"issues,omitempty"`
}

type PanelForwardRuntimeProbe struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"statusCode,omitempty"`
	Body       string `json:"body,omitempty"`
	Error      string `json:"error,omitempty"`
}

type PanelForwardRuntimeStatusSnapshot struct {
	OK           bool                             `json:"ok"`
	StatusCode   int                              `json:"statusCode,omitempty"`
	Error        string                           `json:"error,omitempty"`
	Version      string                           `json:"version,omitempty"`
	AuthRequired *bool                            `json:"authRequired,omitempty"`
	ExecutePath  string                           `json:"executePath,omitempty"`
	StatusPath   string                           `json:"statusPath,omitempty"`
	Supports     *nodeXForwardRuntimeSupportState `json:"supports,omitempty"`
	Modes        *nodeXForwardRuntimeModes        `json:"modes,omitempty"`
	Issues       []string                         `json:"issues,omitempty"`
}

type PanelForwardRuntimeCommandHints struct {
	PowerShell []string `json:"powerShell"`
	Bash       []string `json:"bash"`
	Upgrade    []string `json:"upgrade"`
	References []string `json:"references"`
}

type PanelForwardRuntimeReadiness struct {
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`
}

type PanelForwardRuntimeConfigState struct {
	Backend           string `json:"backend"`
	NodeXMode         bool   `json:"nodeXMode"`
	BaseURL           string `json:"baseUrl,omitempty"`
	BaseURLConfigured bool   `json:"baseUrlConfigured"`
	TokenConfigured   bool   `json:"tokenConfigured"`
	TimeoutSeconds    int64  `json:"timeoutSeconds"`
}

type PanelForwardRuntimeAttachmentState struct {
	Model       string `json:"model"`
	Description string `json:"description"`
}

type PanelForwardRuntimeStatusSummary struct {
	BaseURL       string                             `json:"baseUrl,omitempty"`
	CheckedAt     string                             `json:"checkedAt"`
	Config        PanelForwardRuntimeConfigState     `json:"config"`
	Attachment    PanelForwardRuntimeAttachmentState `json:"attachment"`
	Reachability  PanelForwardRuntimeReadiness       `json:"reachability"`
	RuntimeReady  PanelForwardRuntimeReadiness       `json:"runtimeReady"`
	Summary       string                             `json:"summary,omitempty"`
	Warnings      []string                           `json:"warnings,omitempty"`
	Health        PanelForwardRuntimeProbe           `json:"health"`
	RuntimeStatus PanelForwardRuntimeStatusSnapshot  `json:"runtimeStatus"`
	LocalAnsible  *nodeXForwardRuntimeAnsibleStatus  `json:"localAnsible,omitempty"`
}

type PanelForwardRuntimeDoctorSummary struct {
	PanelForwardRuntimeStatusSummary
	Commands PanelForwardRuntimeCommandHints `json:"commands"`
}

func (s *PanelForwardRuntimeService) nodeXDiagnosticsClient() *nodeXForwardRuntimeClient {
	if client, ok := s.client.(*nodeXForwardRuntimeClient); ok && client != nil {
		return client
	}
	return newNodeXForwardRuntimeClient(s.configService)
}

func (s *PanelForwardRuntimeService) GetNodeXRuntimeStatus(ctx context.Context) (*nodeXForwardRuntimeStatus, error) {
	return s.nodeXDiagnosticsClient().Status(ctx)
}

func (s *PanelForwardRuntimeService) GetNodeXOperatorStatus(ctx context.Context) (*PanelForwardRuntimeStatusSummary, error) {
	doctor, err := s.nodeXDiagnosticsClient().Doctor(ctx)
	if err != nil {
		return nil, err
	}
	if doctor == nil {
		return nil, nil
	}
	summary := doctor.PanelForwardRuntimeStatusSummary
	return &summary, nil
}

func (s *PanelForwardRuntimeService) DiagnoseNodeXOperator(ctx context.Context) (*PanelForwardRuntimeDoctorSummary, error) {
	return s.nodeXDiagnosticsClient().Doctor(ctx)
}

func (s *PanelForwardRuntimeService) GetLocalOperatorStatus(ctx context.Context) (*PanelForwardRuntimeStatusSummary, error) {
	summary, err := s.buildLocalOperatorStatusSummary(ctx)
	if err != nil {
		return nil, err
	}
	if summary == nil {
		return nil, nil
	}
	base := summary.PanelForwardRuntimeStatusSummary
	return &base, nil
}

func (s *PanelForwardRuntimeService) DiagnoseLocalOperator(ctx context.Context) (*PanelForwardRuntimeDoctorSummary, error) {
	return s.buildLocalOperatorStatusSummary(ctx)
}

func (s *PanelForwardRuntimeService) GetPanelRuntimeStatus(ctx context.Context) (*PanelForwardRuntimeStatusSummary, error) {
	return s.buildRuntimeStatusSummary(ctx)
}

func (s *PanelForwardRuntimeService) DiagnosePanelRuntime(ctx context.Context) (*PanelForwardRuntimeDoctorSummary, error) {
	summary, err := s.buildRuntimeStatusSummary(ctx)
	if err != nil {
		return nil, err
	}
	return &PanelForwardRuntimeDoctorSummary{
		PanelForwardRuntimeStatusSummary: *summary,
		Commands:                         buildPanelForwardRuntimeCommands(summary.Config.Backend, summary.BaseURL),
	}, nil
}

func (s *PanelForwardService) GetRuntimeStatus(ctx context.Context) (*PanelForwardRuntimeStatusSummary, error) {
	if s.runtimeService == nil {
		return nil, fmt.Errorf("forward runtime service is not configured")
	}
	return s.runtimeService.GetPanelRuntimeStatus(ctx)
}

func (s *PanelForwardService) GetNodeXOperatorStatus(ctx context.Context) (*PanelForwardRuntimeStatusSummary, error) {
	if s.runtimeService == nil {
		return nil, fmt.Errorf("forward runtime service is not configured")
	}
	return s.runtimeService.GetNodeXOperatorStatus(ctx)
}

func (s *PanelForwardService) GetLocalOperatorStatus(ctx context.Context) (*PanelForwardRuntimeStatusSummary, error) {
	if s.runtimeService == nil {
		return nil, fmt.Errorf("forward runtime service is not configured")
	}
	return s.runtimeService.GetLocalOperatorStatus(ctx)
}

func (s *PanelForwardService) DiagnoseRuntime(ctx context.Context) (*PanelForwardRuntimeDoctorSummary, error) {
	if s.runtimeService == nil {
		return nil, fmt.Errorf("forward runtime service is not configured")
	}
	return s.runtimeService.DiagnosePanelRuntime(ctx)
}

func (s *PanelForwardService) DiagnoseNodeXOperator(ctx context.Context) (*PanelForwardRuntimeDoctorSummary, error) {
	if s.runtimeService == nil {
		return nil, fmt.Errorf("forward runtime service is not configured")
	}
	return s.runtimeService.DiagnoseNodeXOperator(ctx)
}

func (s *PanelForwardService) DiagnoseLocalOperator(ctx context.Context) (*PanelForwardRuntimeDoctorSummary, error) {
	if s.runtimeService == nil {
		return nil, fmt.Errorf("forward runtime service is not configured")
	}
	return s.runtimeService.DiagnoseLocalOperator(ctx)
}

func (s *PanelForwardRuntimeService) buildRuntimeStatusSummary(ctx context.Context) (*PanelForwardRuntimeStatusSummary, error) {
	client := s.nodeXDiagnosticsClient()
	settings, err := client.loadSettings()
	if err != nil {
		return nil, err
	}

	backend, err := s.resolveBackend()
	if err != nil {
		return nil, err
	}

	summary := &PanelForwardRuntimeStatusSummary{
		BaseURL:      settings.BaseURL,
		CheckedAt:    time.Now().Format(time.RFC3339),
		Config:       buildPanelForwardRuntimeConfigState(backend, settings),
		Attachment:   buildPanelForwardRuntimeAttachmentState(backend),
		Reachability: PanelForwardRuntimeReadiness{Ready: false},
		RuntimeReady: PanelForwardRuntimeReadiness{Ready: false},
	}

	if isForwardRuntimeLocalAnsibleBackend(backend) {
		summary.LocalAnsible = s.inspectLocalAnsibleRuntime(backend)
		summary.Reachability = buildLocalAnsibleReachability(summary.LocalAnsible)
		summary.RuntimeReady = buildLocalAnsibleRuntimeReady(summary.LocalAnsible)
		if summary.Config.BaseURLConfigured || summary.Config.TokenConfigured {
			summary.Warnings = append(summary.Warnings, panelForwardRuntimeIgnoredNodeXConfigWarning)
		}
	} else if backend == model.ForwardRuntimeBackendCleanAgent {
		summary.Reachability = s.buildCleanAgentRuntimeReady()
		summary.RuntimeReady = summary.Reachability
		if summary.Config.BaseURLConfigured || summary.Config.TokenConfigured {
			summary.Warnings = append(summary.Warnings, "NodeX base_url/token are configured but ignored while runtime backend is clean_agent")
		}
	} else {
		client.populateNodeXRuntimeSummary(ctx, settings, summary)
	}

	summary.Warnings = uniqueNonEmptyStrings(summary.Warnings, summary.RuntimeStatus.Issues)
	summary.Summary = buildPanelForwardRuntimeSummary(summary)
	return summary, nil
}

func (s *PanelForwardRuntimeService) buildLocalOperatorStatusSummary(_ context.Context) (*PanelForwardRuntimeDoctorSummary, error) {
	client := s.nodeXDiagnosticsClient()
	settings, err := client.loadSettings()
	if err != nil {
		return nil, err
	}
	backend, err := s.resolveLocalAnsibleBackend()
	if err != nil {
		return nil, err
	}

	summary := &PanelForwardRuntimeDoctorSummary{
		PanelForwardRuntimeStatusSummary: PanelForwardRuntimeStatusSummary{
			BaseURL:      settings.BaseURL,
			CheckedAt:    time.Now().Format(time.RFC3339),
			Config:       buildPanelForwardRuntimeConfigState(backend, settings),
			Attachment:   buildPanelForwardRuntimeAttachmentState(backend),
			Reachability: PanelForwardRuntimeReadiness{Ready: false},
			RuntimeReady: PanelForwardRuntimeReadiness{Ready: false},
		},
		Commands: buildPanelForwardRuntimeCommands(backend, ""),
	}

	summary.LocalAnsible = s.inspectLocalAnsibleRuntime(backend)
	summary.Reachability = buildLocalAnsibleReachability(summary.LocalAnsible)
	summary.RuntimeReady = buildLocalAnsibleRuntimeReady(summary.LocalAnsible)
	if summary.Config.BaseURLConfigured || summary.Config.TokenConfigured {
		summary.Warnings = append(summary.Warnings, panelForwardRuntimeIgnoredNodeXConfigWarning)
	}
	summary.Warnings = uniqueNonEmptyStrings(summary.Warnings, summary.RuntimeStatus.Issues)
	summary.Summary = buildPanelForwardRuntimeSummary(&summary.PanelForwardRuntimeStatusSummary)
	return summary, nil
}

func (s *PanelForwardRuntimeService) inspectLocalAnsibleRuntime(backend string) *nodeXForwardRuntimeAnsibleStatus {
	backend = normalizeForwardRuntimeLocalBackendOrDefault(backend)
	status := &nodeXForwardRuntimeAnsibleStatus{
		Backend:        backend,
		FirewallDriver: forwardRuntimeLocalFirewallDriver(backend),
		Supported:      true,
	}

	cfg, err := s.loadPanelForwardAnsibleConfigForDiagnosticsWithBackend(backend)
	if err != nil {
		status.Issues = append(status.Issues, err.Error())
		return status
	}

	status.Command = strings.TrimSpace(cfg.Command)
	if status.Command == "" {
		status.Command = defaultAnsibleCommand
	}
	if _, err := exec.LookPath(status.Command); err == nil {
		status.CommandFound = true
	} else {
		status.Issues = append(status.Issues, fmt.Sprintf("command not found: %s", status.Command))
	}

	status.WorkingDir = resolveForwardRuntimeWorkingDir(cfg.WorkingDir)
	status.WorkingDirExists = pathExists(status.WorkingDir)
	if !status.WorkingDirExists {
		status.Issues = append(status.Issues, fmt.Sprintf("working directory missing: %s", status.WorkingDir))
	}

	status.InventoryPath = resolveForwardRuntimeFilePath(cfg.WorkingDir, cfg.Inventory)
	status.InventoryExists = pathExists(status.InventoryPath)
	if !status.InventoryExists {
		status.Issues = append(status.Issues, fmt.Sprintf("inventory missing: %s", status.InventoryPath))
	}

	status.ApplyPlaybookPath = resolveForwardRuntimeFilePath(cfg.WorkingDir, cfg.ApplyPlaybook)
	status.ApplyPlaybookExists = pathExists(status.ApplyPlaybookPath)
	if !status.ApplyPlaybookExists {
		status.Issues = append(status.Issues, fmt.Sprintf("apply playbook missing: %s", status.ApplyPlaybookPath))
	}

	status.RemovePlaybookPath = resolveForwardRuntimeFilePath(cfg.WorkingDir, cfg.RemovePlaybook)
	status.RemovePlaybookExists = pathExists(status.RemovePlaybookPath)
	if !status.RemovePlaybookExists {
		status.Issues = append(status.Issues, fmt.Sprintf("remove playbook missing: %s", status.RemovePlaybookPath))
	}

	ansibleConfigPath := resolveForwardRuntimeEnvPath(cfg.WorkingDir, cfg.Environment["ANSIBLE_CONFIG"])
	if strings.TrimSpace(ansibleConfigPath) != "" {
		status.AnsibleConfigPath = ansibleConfigPath
		status.AnsibleConfigExists = pathExists(ansibleConfigPath)
		if !status.AnsibleConfigExists {
			status.Issues = append(status.Issues, fmt.Sprintf("ansible config missing: %s", ansibleConfigPath))
		}
	}

	status.TargetPattern = strings.TrimSpace(cfg.TargetPattern)
	status.Become = cfg.Become
	status.TimeoutSeconds = int64(cfg.TimeoutSeconds)
	status.Ready = status.CommandFound &&
		status.WorkingDirExists &&
		status.InventoryExists &&
		status.ApplyPlaybookExists &&
		status.RemovePlaybookExists

	return status
}

func buildPanelForwardRuntimeConfigState(backend string, settings *nodeXForwardRuntimeSettings) PanelForwardRuntimeConfigState {
	timeoutSeconds := int64(defaultForwardRuntimeNodeXTimeout / time.Second)
	if settings != nil && settings.Timeout > 0 {
		timeoutSeconds = int64(settings.Timeout / time.Second)
	}

	baseURL := ""
	tokenConfigured := false
	if settings != nil {
		baseURL = strings.TrimSpace(settings.BaseURL)
		tokenConfigured = strings.TrimSpace(settings.Token) != ""
	}

	return PanelForwardRuntimeConfigState{
		Backend:           backend,
		NodeXMode:         backend == model.ForwardRuntimeBackendGost,
		BaseURL:           baseURL,
		BaseURLConfigured: baseURL != "",
		TokenConfigured:   tokenConfigured,
		TimeoutSeconds:    timeoutSeconds,
	}
}

func buildPanelForwardRuntimeAttachmentState(backend string) PanelForwardRuntimeAttachmentState {
	if backend == model.ForwardRuntimeBackendCleanAgent {
		return PanelForwardRuntimeAttachmentState{
			Model:       panelForwardRuntimeAttachmentModelCleanAgent,
			Description: "Clean-room pull agent path. The panel stores desired runtime jobs; v2forward-agent polls, applies local firewall state, and reports results back.",
		}
	}
	if isForwardRuntimeLocalAnsibleBackend(backend) {
		attachmentModel := "local_nftables_ansible_stateless"
		description := "Stateless nftables/Ansible path. Only the execution node identity is stored on the tunnel; SSH access comes from the configured ansible inventory."
		if backend == model.ForwardRuntimeBackendIptablesAnsible {
			attachmentModel = "local_iptables_ansible_stateless"
			description = "Stateless iptables/Ansible path. Only the execution node identity is stored on the tunnel; SSH access comes from the configured ansible inventory."
		}
		return PanelForwardRuntimeAttachmentState{
			Model:       attachmentModel,
			Description: description,
		}
	}
	return PanelForwardRuntimeAttachmentState{
		Model:       panelForwardRuntimeAttachmentModelNodeXGost,
		Description: "Stateful NodeX/gost path. The panel talks to the NodeX control plane, and actual relay attachment only exists after the gost runtime job succeeds.",
	}
}

func buildNodeXReachability(probe PanelForwardRuntimeProbe) PanelForwardRuntimeReadiness {
	if probe.OK {
		return PanelForwardRuntimeReadiness{
			Ready:  true,
			Reason: panelForwardRuntimeNodeXHealthSuccessReason,
		}
	}
	if strings.TrimSpace(probe.Error) != "" {
		return PanelForwardRuntimeReadiness{
			Ready:  false,
			Reason: probe.Error,
		}
	}
	if probe.StatusCode > 0 {
		return PanelForwardRuntimeReadiness{
			Ready:  false,
			Reason: fmt.Sprintf("NodeX /health returned HTTP %d", probe.StatusCode),
		}
	}
	return PanelForwardRuntimeReadiness{
		Ready:  false,
		Reason: "NodeX health probe did not succeed",
	}
}

func buildGostRuntimeReady(snapshot PanelForwardRuntimeStatusSnapshot) PanelForwardRuntimeReadiness {
	if !snapshot.OK {
		if strings.TrimSpace(snapshot.Error) != "" {
			return PanelForwardRuntimeReadiness{Ready: false, Reason: snapshot.Error}
		}
		if snapshot.StatusCode > 0 {
			return PanelForwardRuntimeReadiness{
				Ready:  false,
				Reason: fmt.Sprintf("NodeX runtime status returned HTTP %d", snapshot.StatusCode),
			}
		}
		return PanelForwardRuntimeReadiness{
			Ready:  false,
			Reason: "NodeX runtime status is not available yet",
		}
	}

	supportsGost := snapshot.Supports != nil && containsString(snapshot.Supports.Backends, model.ForwardRuntimeBackendGost)
	if snapshot.Modes != nil && snapshot.Modes.Gost.Supported {
		supportsGost = true
	}
	if !supportsGost {
		return PanelForwardRuntimeReadiness{
			Ready:  false,
			Reason: panelForwardRuntimeNodeXRuntimeMissingBackendReason,
		}
	}

	return PanelForwardRuntimeReadiness{
		Ready:  true,
		Reason: panelForwardRuntimeNodeXRuntimeReadyReason,
	}
}

func buildLocalAnsibleReachability(status *nodeXForwardRuntimeAnsibleStatus) PanelForwardRuntimeReadiness {
	if status != nil && status.CommandFound {
		return PanelForwardRuntimeReadiness{
			Ready:  true,
			Reason: panelForwardRuntimeLocalExecutorReachableReason,
		}
	}
	return PanelForwardRuntimeReadiness{
		Ready:  false,
		Reason: firstRuntimeIssue(status, "ansible-playbook is not available on the panel host"),
	}
}

func buildLocalAnsibleRuntimeReady(status *nodeXForwardRuntimeAnsibleStatus) PanelForwardRuntimeReadiness {
	if status != nil && status.Ready {
		return PanelForwardRuntimeReadiness{
			Ready:  true,
			Reason: panelForwardRuntimeLocalExecutorReadyReason,
		}
	}
	return PanelForwardRuntimeReadiness{
		Ready:  false,
		Reason: firstRuntimeIssue(status, "Local ansible runtime is not ready yet"),
	}
}

func (s *PanelForwardRuntimeService) buildCleanAgentRuntimeReady() PanelForwardRuntimeReadiness {
	if s == nil || s.db == nil {
		return PanelForwardRuntimeReadiness{Ready: false, Reason: "database is not configured"}
	}

	var onlineCount int64
	if err := s.db.Model(&model.ForwardCleanAgent{}).
		Where("status = ? AND revoked_at IS NULL", model.ForwardCleanAgentStatusOnline).
		Count(&onlineCount).Error; err != nil {
		return PanelForwardRuntimeReadiness{Ready: false, Reason: err.Error()}
	}
	if onlineCount > 0 {
		return PanelForwardRuntimeReadiness{Ready: true, Reason: panelForwardRuntimeCleanAgentReadyReason}
	}
	return PanelForwardRuntimeReadiness{Ready: false, Reason: panelForwardRuntimeCleanAgentWaitingReason}
}

func firstRuntimeIssue(status *nodeXForwardRuntimeAnsibleStatus, fallback string) string {
	if status != nil {
		if issue := firstNonEmpty(status.Issues...); issue != "" {
			return issue
		}
	}
	return fallback
}

func buildPanelForwardRuntimeSummary(summary *PanelForwardRuntimeStatusSummary) string {
	if summary == nil {
		return ""
	}
	if isForwardRuntimeLocalAnsibleBackend(summary.Config.Backend) {
		label := forwardRuntimeLocalBackendLabel(summary.Config.Backend)
		if summary.RuntimeReady.Ready {
			return fmt.Sprintf("Local %s executor is ready. This path stays stateless and still requires queued jobs to finish successfully before a forward actually exists.", label)
		}
		if summary.Reachability.Ready {
			return "ansible-playbook is reachable on the panel host, but inventory/playbooks or related runtime files are not ready yet."
		}
		return fmt.Sprintf("Local %s executor is not ready on the panel host yet.", label)
	}

	if summary.Config.Backend == model.ForwardRuntimeBackendCleanAgent {
		if summary.RuntimeReady.Ready {
			return "Clean agent runtime is selected and at least one agent is online. Forward changes will be queued until the bound agent pulls and reports each job."
		}
		return "Clean agent runtime is selected, but no online clean agent is registered yet."
	}

	if !summary.Config.BaseURLConfigured {
		return "NodeX mode is enabled, but the control plane base URL is still missing."
	}
	if !summary.Config.TokenConfigured {
		return "NodeX control plane address is configured, but the shared token is still missing."
	}
	if summary.RuntimeReady.Ready {
		return "NodeX control plane is configured and runtime status confirms gost support. Actual relay attachment still depends on each runtime job succeeding."
	}
	if summary.Reachability.Ready {
		return "NodeX control plane responded, but gost runtime readiness is not confirmed yet."
	}
	return "NodeX mode is enabled, but the control plane is not reachable from the panel host."
}

func buildPanelForwardRuntimeCommands(backend, baseURL string) PanelForwardRuntimeCommandHints {
	if backend == model.ForwardRuntimeBackendCleanAgent {
		return PanelForwardRuntimeCommandHints{
			PowerShell: []string{
				"Invoke-WebRequest '/api/v2/admin/forward/agents' -Method POST -Body '{\"name\":\"relay-1\",\"nodeId\":1}'",
				"Invoke-WebRequest '/api/v2/forward-agent/install.sh'",
			},
			Bash: []string{
				"curl -fsSL \"$PANEL_URL/api/v2/forward-agent/install.sh\" -o install-v2forward-agent.sh",
				"sudo AGENT_TOKEN='<TOKEN>' NODE_ID='<FORWARD_NODE_ID>' bash install-v2forward-agent.sh",
			},
			Upgrade: []string{
				"go test ./internal/service -run TestForwardCleanAgent",
				"go build -o v2board ./cmd/server",
			},
			References: []string{
				"docs/forward-clean-room/spec.md",
				"docs/forward-clean-room/provenance.md",
			},
		}
	}
	if isForwardRuntimeLocalAnsibleBackend(backend) {
		applyPlaybook := defaultForwardApplyPlaybookPathForBackend(backend)
		removePlaybook := defaultForwardRemovePlaybookPathForBackend(backend)
		firewallCommand := "nft --version"
		firewallListCommand := "nft list tables"
		if backend == model.ForwardRuntimeBackendIptablesAnsible {
			firewallCommand = "iptables --version"
			firewallListCommand = "iptables -t nat -S"
		}
		return PanelForwardRuntimeCommandHints{
			PowerShell: []string{
				"Get-Command ansible-playbook",
				"ansible-playbook --version",
				"Get-Content .\\config\\deploy\\ansible\\inventory.ini",
				firewallCommand,
			},
			Bash: []string{
				"command -v ansible-playbook",
				"ansible-playbook --version",
				"cat ./config/deploy/ansible/inventory.ini",
				firewallListCommand,
			},
			Upgrade: []string{
				"git pull --ff-only",
				fmt.Sprintf("cat ./%s", strings.ReplaceAll(applyPlaybook, "\\", "/")),
				fmt.Sprintf("cat ./%s", strings.ReplaceAll(removePlaybook, "\\", "/")),
				"go test ./internal/service/... -run ForwardRuntime",
			},
			References: []string{
				"docs/guide/forward-relay-onboarding.md",
				"docs/guide/forward-tunnel-runtime-ops.md",
				"docs/guide/forward-tunnel-smoke-test.md",
			},
		}
	}
	return buildNodeXOperatorCommands(baseURL)
}

func uniqueNonEmptyStrings(values ...[]string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, group := range values {
		for _, value := range group {
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			result = append(result, trimmed)
		}
	}
	return result
}

func containsString(values []string, target string) bool {
	normalizedTarget := strings.TrimSpace(strings.ToLower(target))
	for _, value := range values {
		if strings.TrimSpace(strings.ToLower(value)) == normalizedTarget {
			return true
		}
	}
	return false
}

func (c *nodeXForwardRuntimeClient) populateNodeXRuntimeSummary(ctx context.Context, settings *nodeXForwardRuntimeSettings, summary *PanelForwardRuntimeStatusSummary) {
	if summary == nil {
		return
	}

	if !summary.Config.BaseURLConfigured {
		summary.Reachability = PanelForwardRuntimeReadiness{
			Ready:  false,
			Reason: panelForwardRuntimeMissingNodeXBaseURLReason,
		}
		summary.RuntimeReady = PanelForwardRuntimeReadiness{
			Ready:  false,
			Reason: panelForwardRuntimeMissingNodeXBaseURLReason,
		}
		summary.Warnings = append(summary.Warnings, panelForwardRuntimeMissingNodeXBaseURLReason)
		return
	}

	summary.Health = c.fetchHealth(ctx, settings.BaseURL, settings.Timeout)
	summary.Reachability = buildNodeXReachability(summary.Health)

	if !summary.Config.TokenConfigured {
		summary.RuntimeReady = PanelForwardRuntimeReadiness{
			Ready:  false,
			Reason: panelForwardRuntimeMissingNodeXTokenReason,
		}
		summary.Warnings = append(summary.Warnings, panelForwardRuntimeMissingNodeXTokenReason)
		return
	}

	_, summary.RuntimeStatus = c.fetchRuntimeStatus(ctx, settings.BaseURL, settings.Token, settings.Timeout)
	summary.RuntimeReady = buildGostRuntimeReady(summary.RuntimeStatus)
	if summary.RuntimeStatus.Error != "" {
		summary.Warnings = append(summary.Warnings, summary.RuntimeStatus.Error)
	}
}

func (c *nodeXForwardRuntimeClient) Status(ctx context.Context) (*nodeXForwardRuntimeStatus, error) {
	settings, err := c.loadSettings()
	if err != nil {
		return nil, err
	}

	baseURL, err := resolveNodeXBaseURL(nil, settings, true)
	if err != nil {
		return nil, err
	}
	token, err := resolveNodeXToken(nil, settings, true)
	if err != nil {
		return nil, err
	}

	status, snapshot := c.fetchRuntimeStatus(ctx, baseURL, token, settings.Timeout)
	if !snapshot.OK {
		if snapshot.Error != "" {
			return nil, errors.New(snapshot.Error)
		}
		return nil, fmt.Errorf("NodeX runtime status probe failed with status %d", snapshot.StatusCode)
	}
	return status, nil
}

func (c *nodeXForwardRuntimeClient) Doctor(ctx context.Context) (*PanelForwardRuntimeDoctorSummary, error) {
	settings, err := c.loadSettings()
	if err != nil {
		return nil, err
	}

	baseURL, err := resolveNodeXBaseURL(nil, settings, true)
	if err != nil {
		return nil, err
	}
	token, err := resolveNodeXToken(nil, settings, true)
	if err != nil {
		return nil, err
	}

	summary := &PanelForwardRuntimeDoctorSummary{
		PanelForwardRuntimeStatusSummary: PanelForwardRuntimeStatusSummary{
			BaseURL:      baseURL,
			CheckedAt:    time.Now().Format(time.RFC3339),
			Config:       buildPanelForwardRuntimeConfigState(model.ForwardRuntimeBackendGost, settings),
			Attachment:   buildPanelForwardRuntimeAttachmentState(model.ForwardRuntimeBackendGost),
			Reachability: PanelForwardRuntimeReadiness{Ready: false},
			RuntimeReady: PanelForwardRuntimeReadiness{Ready: false},
		},
		Commands: buildPanelForwardRuntimeCommands(model.ForwardRuntimeBackendGost, baseURL),
	}

	summary.Health = c.fetchHealth(ctx, baseURL, settings.Timeout)
	summary.Reachability = buildNodeXReachability(summary.Health)
	_, summary.RuntimeStatus = c.fetchRuntimeStatus(ctx, baseURL, token, settings.Timeout)
	summary.RuntimeReady = buildGostRuntimeReady(summary.RuntimeStatus)
	summary.Warnings = uniqueNonEmptyStrings(summary.RuntimeStatus.Issues, []string{summary.RuntimeStatus.Error})
	summary.Summary = buildPanelForwardRuntimeSummary(&summary.PanelForwardRuntimeStatusSummary)
	return summary, nil
}

func (c *nodeXForwardRuntimeClient) fetchHealth(ctx context.Context, baseURL string, timeout time.Duration) PanelForwardRuntimeProbe {
	body, probe := c.performNodeXRequest(ctx, http.MethodGet, baseURL+defaultForwardRuntimeNodeXHealthPath, "", timeout)
	probe.Body = strings.TrimSpace(string(body))
	probe.OK = probe.OK && strings.EqualFold(probe.Body, "ok")
	if !probe.OK && probe.Error == "" && probe.StatusCode >= http.StatusBadRequest && probe.Body != "" {
		probe.Error = probe.Body
	}
	return probe
}

func (c *nodeXForwardRuntimeClient) fetchRuntimeStatus(ctx context.Context, baseURL, token string, timeout time.Duration) (*nodeXForwardRuntimeStatus, PanelForwardRuntimeStatusSnapshot) {
	body, probe := c.performNodeXRequest(ctx, http.MethodGet, baseURL+defaultForwardRuntimeNodeXStatusPath, token, timeout)
	snapshot := PanelForwardRuntimeStatusSnapshot{
		OK:         false,
		StatusCode: probe.StatusCode,
		Error:      probe.Error,
	}

	if len(bytes.TrimSpace(body)) == 0 {
		if snapshot.Error == "" {
			snapshot.Error = "empty runtime status response"
		}
		return nil, snapshot
	}

	var envelope nodeXForwardRuntimeStatusEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		snapshot.Error = fmt.Sprintf("decode NodeX runtime status response: %v", err)
		return nil, snapshot
	}

	if !probe.OK {
		snapshot.Error = firstNonEmpty(snapshot.Error, strings.TrimSpace(envelope.Error), strings.TrimSpace(envelope.Msg), strings.TrimSpace(envelope.Message), strings.TrimSpace(string(body)))
		return nil, snapshot
	}

	if envelope.Data == nil {
		snapshot.Error = firstNonEmpty(strings.TrimSpace(envelope.Error), strings.TrimSpace(envelope.Msg), strings.TrimSpace(envelope.Message), "runtime status data is empty")
		return nil, snapshot
	}

	authRequired := envelope.Data.AuthRequired
	snapshot.OK = true
	snapshot.Error = ""
	snapshot.Version = strings.TrimSpace(envelope.Data.Version)
	snapshot.AuthRequired = &authRequired
	snapshot.ExecutePath = strings.TrimSpace(envelope.Data.ExecutePath)
	snapshot.StatusPath = strings.TrimSpace(envelope.Data.StatusPath)
	snapshot.Supports = &envelope.Data.Supports
	snapshot.Modes = &envelope.Data.Modes
	snapshot.Issues = append(snapshot.Issues, envelope.Data.Modes.IptablesAnsible.Issues...)

	return envelope.Data, snapshot
}

func (c *nodeXForwardRuntimeClient) performNodeXRequest(ctx context.Context, method, rawURL, token string, timeout time.Duration) ([]byte, PanelForwardRuntimeProbe) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, PanelForwardRuntimeProbe{
			Error: fmt.Sprintf("create NodeX request: %v", err),
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-API-Key", token)
	}

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, PanelForwardRuntimeProbe{
			Error: err.Error(),
		}
	}

	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	probe := PanelForwardRuntimeProbe{
		StatusCode: resp.StatusCode,
		OK:         resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices,
	}
	if readErr != nil {
		probe.OK = false
		probe.Error = fmt.Sprintf("read NodeX response: %v", readErr)
		return nil, probe
	}
	if closeErr != nil {
		probe.OK = false
		probe.Error = fmt.Sprintf("close NodeX response: %v", closeErr)
		return nil, probe
	}

	return body, probe
}

func buildNodeXOperatorCommands(baseURL string) PanelForwardRuntimeCommandHints {
	normalized := strings.TrimSpace(baseURL)
	if normalized == "" {
		normalized = "http://127.0.0.1:18081"
	}

	return PanelForwardRuntimeCommandHints{
		PowerShell: []string{
			fmt.Sprintf("Invoke-WebRequest '%s/health' | Select-Object -ExpandProperty Content", normalized),
			fmt.Sprintf("Invoke-WebRequest '%s/api/v2/internal/forward/runtime/status' -Headers @{ Authorization = 'Bearer <FORWARD_API_TOKEN>' } | Select-Object -ExpandProperty Content", normalized),
			"Invoke-WebRequest 'http://<RELAY_HOST>:<API_PORT>/api/config/services' -Headers @{ Authorization = 'Basic <BASE64(admin:RELAY_API_TOKEN)>' } | Select-Object -ExpandProperty Content",
		},
		Bash: []string{
			fmt.Sprintf("curl -fsSL '%s/health'", normalized),
			fmt.Sprintf("curl -fsSL -H 'Authorization: Bearer <FORWARD_API_TOKEN>' '%s/api/v2/internal/forward/runtime/status'", normalized),
			"curl -fsSL -u 'admin:<RELAY_API_TOKEN>' 'http://<RELAY_HOST>:<API_PORT>/api/config/services'",
		},
		Upgrade: []string{
			"git clone https://github.com/zdwtest/NodeX.git",
			"cd NodeX/control-plane && go run ./cmd/control-plane --version",
			"cd NodeX/control-plane && go run ./cmd/control-plane --config ../deploy/config/control-plane.yaml --addr :18081 --forward-api-token <FORWARD_API_TOKEN>",
		},
		References: []string{
			"Current repo: docs/reference/runtime.md",
			"Current repo: docs/guide/forward-relay-onboarding.md",
			"NodeX repo: https://github.com/zdwtest/NodeX",
			"NodeX doc: docs/forward-runtime-relay-onboarding.md",
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
