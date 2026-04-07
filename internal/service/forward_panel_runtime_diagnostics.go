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

	"github.com/anixops/v2board/internal/model"
)

const (
	defaultForwardRuntimeNodeXHealthPath = "/health"
	defaultForwardRuntimeNodeXStatusPath = "/api/v2/internal/forward/runtime/status"

	panelForwardRuntimeAttachmentModelNodeXGost        = "nodex_gost_stateful"
	panelForwardRuntimeAttachmentModelLocalAnsible     = "local_iptables_ansible_stateless"
	panelForwardRuntimeIgnoredNodeXConfigWarning       = "NodeX base_url/token are configured but ignored while runtime backend is iptables_ansible"
	panelForwardRuntimeMissingNodeXBaseURLReason       = "NodeX mode requires forward.runtime.nodex.base_url before the panel can probe the control plane"
	panelForwardRuntimeMissingNodeXTokenReason         = "NodeX mode requires forward.runtime.nodex.token before runtime readiness can be confirmed"
	panelForwardRuntimeNodeXHealthSuccessReason        = "NodeX /health responded with ok from the panel host"
	panelForwardRuntimeLocalExecutorReachableReason    = "ansible-playbook is available on the panel host"
	panelForwardRuntimeLocalExecutorReadyReason        = "Local ansible executor resolved inventory/playbooks and is ready to queue jobs"
	panelForwardRuntimeNodeXRuntimeReadyReason         = "NodeX runtime status responded and advertises gost support"
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
	BaseURL       string                            `json:"baseUrl,omitempty"`
	CheckedAt     string                            `json:"checkedAt"`
	Config        PanelForwardRuntimeConfigState    `json:"config"`
	Attachment    PanelForwardRuntimeAttachmentState `json:"attachment"`
	Reachability  PanelForwardRuntimeReadiness      `json:"reachability"`
	RuntimeReady  PanelForwardRuntimeReadiness      `json:"runtimeReady"`
	Summary       string                            `json:"summary,omitempty"`
	Warnings      []string                          `json:"warnings,omitempty"`
	Health        PanelForwardRuntimeProbe          `json:"health"`
	RuntimeStatus PanelForwardRuntimeStatusSnapshot `json:"runtimeStatus"`
	LocalAnsible  *nodeXForwardRuntimeAnsibleStatus `json:"localAnsible,omitempty"`
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

func (s *PanelForwardService) DiagnoseRuntime(ctx context.Context) (*PanelForwardRuntimeDoctorSummary, error) {
	if s.runtimeService == nil {
		return nil, fmt.Errorf("forward runtime service is not configured")
	}
	return s.runtimeService.DiagnosePanelRuntime(ctx)
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

	if backend == model.ForwardRuntimeBackendIptablesAnsible {
		summary.LocalAnsible = s.inspectLocalAnsibleRuntime()
		summary.Reachability = buildLocalAnsibleReachability(summary.LocalAnsible)
		summary.RuntimeReady = buildLocalAnsibleRuntimeReady(summary.LocalAnsible)
		if summary.Config.BaseURLConfigured || summary.Config.TokenConfigured {
			summary.Warnings = append(summary.Warnings, panelForwardRuntimeIgnoredNodeXConfigWarning)
		}
	} else {
		client.populateNodeXRuntimeSummary(ctx, settings, summary)
	}

	summary.Warnings = uniqueNonEmptyStrings(summary.Warnings, summary.RuntimeStatus.Issues)
	summary.Summary = buildPanelForwardRuntimeSummary(summary)
	return summary, nil
}

func (s *PanelForwardRuntimeService) inspectLocalAnsibleRuntime() *nodeXForwardRuntimeAnsibleStatus {
	status := &nodeXForwardRuntimeAnsibleStatus{
		Supported: true,
	}

	cfg, err := s.loadPanelForwardAnsibleConfigForDiagnostics()
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
	if backend == model.ForwardRuntimeBackendIptablesAnsible {
		return PanelForwardRuntimeAttachmentState{
			Model:       panelForwardRuntimeAttachmentModelLocalAnsible,
			Description: "Stateless iptables/Ansible path. Only the execution node identity is stored on the tunnel; SSH access still comes from inventory or environment variables.",
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

	supportsGost := false
	if snapshot.Supports != nil && containsString(snapshot.Supports.Backends, model.ForwardRuntimeBackendGost) {
		supportsGost = true
	}
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
	if summary.Config.Backend == model.ForwardRuntimeBackendIptablesAnsible {
		if summary.RuntimeReady.Ready {
			return "Local iptables/Ansible executor is ready. This path stays stateless and still requires queued jobs to finish successfully before a forward actually exists."
		}
		if summary.Reachability.Ready {
			return "ansible-playbook is reachable on the panel host, but inventory/playbooks or related runtime files are not ready yet."
		}
		return "Local iptables/Ansible executor is not ready on the panel host yet."
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
	if backend == model.ForwardRuntimeBackendIptablesAnsible {
		return PanelForwardRuntimeCommandHints{
			PowerShell: []string{
				"Get-Command ansible-playbook",
				"ansible-playbook --version",
				"Get-Content .\\config\\deploy\\ansible\\inventory.ini",
			},
			Bash: []string{
				"command -v ansible-playbook",
				"ansible-playbook --version",
				"cat ./config/deploy/ansible/inventory.ini",
			},
			Upgrade: []string{
				"git pull --ff-only",
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
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	probe := PanelForwardRuntimeProbe{
		StatusCode: resp.StatusCode,
		OK:         resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices,
	}
	if readErr != nil {
		probe.OK = false
		probe.Error = fmt.Sprintf("read NodeX response: %v", readErr)
		return nil, probe
	}

	return body, probe
}

func buildNodeXOperatorCommands(baseURL string) PanelForwardRuntimeCommandHints {
	normalized := strings.TrimSpace(baseURL)
	if normalized == "" {
		normalized = "http://127.0.0.1:8080"
	}

	return PanelForwardRuntimeCommandHints{
		PowerShell: []string{
			"powershell -File .\\tools\\nodex.ps1 check-version",
			fmt.Sprintf("powershell -File .\\tools\\nodex.ps1 doctor -BaseUrl %s -ForwardApiToken <FORWARD_API_TOKEN>", normalized),
			fmt.Sprintf("powershell -File .\\tools\\nodex.ps1 runtime-status -BaseUrl %s -ForwardApiToken <FORWARD_API_TOKEN>", normalized),
		},
		Bash: []string{
			"bash ./tools/nodex.sh check-version",
			fmt.Sprintf("BASE_URL=%s FORWARD_API_TOKEN=<FORWARD_API_TOKEN> bash ./tools/nodex.sh doctor", normalized),
			fmt.Sprintf("BASE_URL=%s FORWARD_API_TOKEN=<FORWARD_API_TOKEN> bash ./tools/nodex.sh runtime-status", normalized),
		},
		Upgrade: []string{
			"git pull --ff-only",
			"powershell -File .\\tools\\nodex.ps1 version",
			"powershell -File .\\tools\\nodex.ps1 sync-config",
		},
		References: []string{
			"docs/reference/check-version.md",
			"docs/reference/upgrade.md",
			"docs/reference/connect-model.md",
			"docs/reference/nodeclient-faq.md",
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
