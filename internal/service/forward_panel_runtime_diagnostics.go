package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultForwardRuntimeNodeXHealthPath = "/health"
	defaultForwardRuntimeNodeXStatusPath = "/api/v2/internal/forward/runtime/status"
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

type PanelForwardRuntimeDoctorSummary struct {
	BaseURL       string                            `json:"baseUrl"`
	CheckedAt     string                            `json:"checkedAt"`
	Health        PanelForwardRuntimeProbe          `json:"health"`
	RuntimeStatus PanelForwardRuntimeStatusSnapshot `json:"runtimeStatus"`
	Commands      PanelForwardRuntimeCommandHints   `json:"commands"`
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

func (s *PanelForwardRuntimeService) DiagnoseNodeXRuntime(ctx context.Context) (*PanelForwardRuntimeDoctorSummary, error) {
	return s.nodeXDiagnosticsClient().Doctor(ctx)
}

func (s *PanelForwardService) GetRuntimeStatus(ctx context.Context) (*nodeXForwardRuntimeStatus, error) {
	if s.runtimeService == nil {
		return nil, fmt.Errorf("forward runtime service is not configured")
	}
	return s.runtimeService.GetNodeXRuntimeStatus(ctx)
}

func (s *PanelForwardService) DiagnoseRuntime(ctx context.Context) (*PanelForwardRuntimeDoctorSummary, error) {
	if s.runtimeService == nil {
		return nil, fmt.Errorf("forward runtime service is not configured")
	}
	return s.runtimeService.DiagnoseNodeXRuntime(ctx)
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
		BaseURL:   baseURL,
		CheckedAt: time.Now().Format(time.RFC3339),
		Commands:  buildNodeXOperatorCommands(baseURL),
	}

	summary.Health = c.fetchHealth(ctx, baseURL, settings.Timeout)
	_, summary.RuntimeStatus = c.fetchRuntimeStatus(ctx, baseURL, token, settings.Timeout)

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
