package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// Settings is the part of the kernel's KernelSettings contract the native
// routes call.
type Settings interface {
	GetSettings(ctx context.Context, in *kernelsettingsv1.GetSettingsRequest, opts ...grpc.CallOption) (*kernelsettingsv1.GetSettingsResponse, error)
}

// Route ids of the NodeX runtime status and diagnosis.
const (
	NodeXStatusRouteID = "gost.admin.forward.nodex.status.get"
	NodeXDoctorRouteID = "gost.admin.forward.nodex.doctor.get"
)

// The NodeX control plane settings, in the KernelSettings namespace nodex.
const (
	nodeXNamespace         = "nodex"
	nodeXBaseURLKey        = "forward.runtime.nodex.base_url"
	nodeXTokenKey          = "forward.runtime.nodex.token" // #nosec G101 -- a configuration key, not a credential.
	nodeXTimeoutSecondsKey = "forward.runtime.nodex.timeout_seconds"
)

const (
	nodeXHealthPath     = "/health"
	nodeXStatusPath     = "/api/v2/internal/forward/runtime/status"
	defaultNodeXTimeout = 15 * time.Second
	// gostBackend is the kernel's model.ForwardRuntimeBackendGost.
	gostBackend = "gost"

	nodeXAttachmentModel        = "nodex_gost_stateful"
	nodeXHealthSuccessReason    = "NodeX /health responded with ok from the panel host"
	nodeXRuntimeReadyReason     = "NodeX runtime status responded and advertises gost support"
	nodeXRuntimeNoBackendReason = "NodeX runtime status responded, but gost support is not advertised yet"
)

// The answer types are the kernel's (internal/service/forward_panel_runtime_diagnostics.go):
// same fields, same JSON names and omissions. The NodeX status types keep
// the kernel's names as well: a status that does not decode answers
// encoding/json's error, which names them (decodeError).

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

type runtimeProbe struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"statusCode,omitempty"`
	Body       string `json:"body,omitempty"`
	Error      string `json:"error,omitempty"`
}

type runtimeStatusSnapshot struct {
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

type runtimeCommandHints struct {
	PowerShell []string `json:"powerShell"`
	Bash       []string `json:"bash"`
	Upgrade    []string `json:"upgrade"`
	References []string `json:"references"`
}

type runtimeReadiness struct {
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`
}

type runtimeConfigState struct {
	Backend           string `json:"backend"`
	NodeXMode         bool   `json:"nodeXMode"`
	BaseURL           string `json:"baseUrl,omitempty"`
	BaseURLConfigured bool   `json:"baseUrlConfigured"`
	TokenConfigured   bool   `json:"tokenConfigured"`
	TimeoutSeconds    int64  `json:"timeoutSeconds"`
}

type runtimeAttachmentState struct {
	Model       string `json:"model"`
	Description string `json:"description"`
}

type runtimeStatusSummary struct {
	BaseURL       string                            `json:"baseUrl,omitempty"`
	CheckedAt     string                            `json:"checkedAt"`
	Config        runtimeConfigState                `json:"config"`
	Attachment    runtimeAttachmentState            `json:"attachment"`
	Reachability  runtimeReadiness                  `json:"reachability"`
	RuntimeReady  runtimeReadiness                  `json:"runtimeReady"`
	Summary       string                            `json:"summary,omitempty"`
	Warnings      []string                          `json:"warnings,omitempty"`
	Health        runtimeProbe                      `json:"health"`
	RuntimeStatus runtimeStatusSnapshot             `json:"runtimeStatus"`
	LocalAnsible  *nodeXForwardRuntimeAnsibleStatus `json:"localAnsible,omitempty"`
}

type runtimeDoctorSummary struct {
	runtimeStatusSummary
	Commands runtimeCommandHints `json:"commands"`
}

// nodeXSettings are the NodeX control plane's address, shared token and
// request timeout.
type nodeXSettings struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

// loadNodeXSettings is the kernel's nodeXForwardRuntimeClient.loadSettings
// on the values KernelSettings answers; the token in clear
// (kernel.settings.nodex.secrets.v1).
func (s *Service) loadNodeXSettings(ctx context.Context) (*nodeXSettings, error) {
	response, err := s.Settings.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{
		Namespace: nodeXNamespace, Keys: []string{nodeXBaseURLKey, nodeXTokenKey, nodeXTimeoutSecondsKey},
	})
	if err != nil {
		return nil, errors.New(status.Convert(err).Message())
	}
	values := map[string]string{}
	for _, setting := range response.GetSettings() {
		if setting.GetMasked() {
			return nil, fmt.Errorf("%s is masked: the package needs kernel.settings.nodex.secrets.v1", setting.GetKey())
		}
		values[setting.GetKey()] = setting.GetValue()
	}
	settings := &nodeXSettings{
		BaseURL: strings.TrimRight(strings.TrimSpace(values[nodeXBaseURLKey]), "/"),
		Token:   strings.TrimSpace(values[nodeXTokenKey]),
		Timeout: defaultNodeXTimeout,
	}
	if timeout := strings.TrimSpace(values[nodeXTimeoutSecondsKey]); timeout != "" {
		seconds, err := strconv.Atoi(timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid %s value: %w", nodeXTimeoutSecondsKey, err)
		}
		if seconds <= 0 {
			return nil, fmt.Errorf("%s must be greater than zero", nodeXTimeoutSecondsKey)
		}
		settings.Timeout = time.Duration(seconds) * time.Second
	}
	return settings, nil
}

// nodeXDoctor is the kernel's nodeXForwardRuntimeClient.Doctor: the
// configuration, a /health probe and the runtime status, from the package
// host.
func (s *Service) nodeXDoctor(ctx context.Context) (*runtimeDoctorSummary, error) {
	settings, err := s.loadNodeXSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings.BaseURL == "" {
		return nil, fmt.Errorf("%s is required for NodeX forward runtime", nodeXBaseURLKey)
	}
	if settings.Token == "" {
		return nil, fmt.Errorf("%s is required for NodeX forward runtime", nodeXTokenKey)
	}
	client := &http.Client{Timeout: settings.Timeout}
	summary := &runtimeDoctorSummary{
		runtimeStatusSummary: runtimeStatusSummary{
			BaseURL:    settings.BaseURL,
			CheckedAt:  s.now().Format(time.RFC3339),
			Config:     runtimeConfig(settings),
			Attachment: runtimeAttachmentState{Model: nodeXAttachmentModel, Description: "Stateful NodeX/gost path. The panel talks to the NodeX control plane, and actual relay attachment only exists after the gost runtime job succeeds."},
		},
		Commands: nodeXOperatorCommands(settings.BaseURL),
	}
	summary.Health = fetchHealth(ctx, client, settings.BaseURL)
	summary.Reachability = nodeXReachability(summary.Health)
	summary.RuntimeStatus = fetchRuntimeStatus(ctx, client, settings.BaseURL, settings.Token)
	summary.RuntimeReady = gostRuntimeReady(summary.RuntimeStatus)
	summary.Warnings = uniqueNonEmptyStrings(summary.RuntimeStatus.Issues, []string{summary.RuntimeStatus.Error})
	summary.Summary = runtimeSummary(&summary.runtimeStatusSummary)
	return summary, nil
}

// NodeXStatus is GET /api/v2/admin/forward/nodex/status: the diagnosis
// without its command hints.
func (s *Service) NodeXStatus(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	doctor, err := s.nodeXDoctor(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(doctor.runtimeStatusSummary)
}

// NodeXDoctor is GET /api/v2/admin/forward/nodex/doctor.
func (s *Service) NodeXDoctor(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	doctor, err := s.nodeXDoctor(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(doctor)
}

func runtimeConfig(settings *nodeXSettings) runtimeConfigState {
	timeoutSeconds := int64(defaultNodeXTimeout / time.Second)
	if settings.Timeout > 0 {
		timeoutSeconds = int64(settings.Timeout / time.Second)
	}
	baseURL := strings.TrimSpace(settings.BaseURL)
	return runtimeConfigState{
		Backend: gostBackend, NodeXMode: true, BaseURL: baseURL, BaseURLConfigured: baseURL != "",
		TokenConfigured: strings.TrimSpace(settings.Token) != "", TimeoutSeconds: timeoutSeconds,
	}
}

func nodeXReachability(probe runtimeProbe) runtimeReadiness {
	if probe.OK {
		return runtimeReadiness{Ready: true, Reason: nodeXHealthSuccessReason}
	}
	if strings.TrimSpace(probe.Error) != "" {
		return runtimeReadiness{Ready: false, Reason: probe.Error}
	}
	if probe.StatusCode > 0 {
		return runtimeReadiness{Ready: false, Reason: fmt.Sprintf("NodeX /health returned HTTP %d", probe.StatusCode)}
	}
	return runtimeReadiness{Ready: false, Reason: "NodeX health probe did not succeed"}
}

func gostRuntimeReady(snapshot runtimeStatusSnapshot) runtimeReadiness {
	if !snapshot.OK {
		if strings.TrimSpace(snapshot.Error) != "" {
			return runtimeReadiness{Ready: false, Reason: snapshot.Error}
		}
		if snapshot.StatusCode > 0 {
			return runtimeReadiness{Ready: false, Reason: fmt.Sprintf("NodeX runtime status returned HTTP %d", snapshot.StatusCode)}
		}
		return runtimeReadiness{Ready: false, Reason: "NodeX runtime status is not available yet"}
	}
	supportsGost := snapshot.Supports != nil && containsString(snapshot.Supports.Backends, gostBackend)
	if snapshot.Modes != nil && snapshot.Modes.Gost.Supported {
		supportsGost = true
	}
	if !supportsGost {
		return runtimeReadiness{Ready: false, Reason: nodeXRuntimeNoBackendReason}
	}
	return runtimeReadiness{Ready: true, Reason: nodeXRuntimeReadyReason}
}

// runtimeSummary is the kernel's buildPanelForwardRuntimeSummary for the
// NodeX backend.
func runtimeSummary(summary *runtimeStatusSummary) string {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func fetchHealth(ctx context.Context, client *http.Client, baseURL string) runtimeProbe {
	body, probe := nodeXRequest(ctx, client, baseURL+nodeXHealthPath, "")
	probe.Body = strings.TrimSpace(string(body))
	probe.OK = probe.OK && strings.EqualFold(probe.Body, "ok")
	if !probe.OK && probe.Error == "" && probe.StatusCode >= http.StatusBadRequest && probe.Body != "" {
		probe.Error = probe.Body
	}
	return probe
}

func fetchRuntimeStatus(ctx context.Context, client *http.Client, baseURL, token string) runtimeStatusSnapshot {
	body, probe := nodeXRequest(ctx, client, baseURL+nodeXStatusPath, token)
	snapshot := runtimeStatusSnapshot{OK: false, StatusCode: probe.StatusCode, Error: probe.Error}
	if len(bytes.TrimSpace(body)) == 0 {
		if snapshot.Error == "" {
			snapshot.Error = "empty runtime status response"
		}
		return snapshot
	}
	var envelope nodeXForwardRuntimeStatusEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		snapshot.Error = fmt.Sprintf("decode NodeX runtime status response: %v", decodeError(err))
		return snapshot
	}
	if !probe.OK {
		snapshot.Error = firstNonEmpty(snapshot.Error, strings.TrimSpace(envelope.Error), strings.TrimSpace(envelope.Msg), strings.TrimSpace(envelope.Message), strings.TrimSpace(string(body)))
		return snapshot
	}
	if envelope.Data == nil {
		snapshot.Error = firstNonEmpty(strings.TrimSpace(envelope.Error), strings.TrimSpace(envelope.Msg), strings.TrimSpace(envelope.Message), "runtime status data is empty")
		return snapshot
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
	return snapshot
}

// decodeError is a decoding error as the kernel words it: encoding/json
// names the Go types it decodes into with their package, the kernel's
// service and the package's native.
func decodeError(err error) string {
	return strings.ReplaceAll(err.Error(), "native.nodeXForwardRuntime", "service.nodeXForwardRuntime")
}

// nodeXRequest is the kernel's performNodeXRequest: a GET with the token as
// bearer and API key, and the answer's status and body.
func nodeXRequest(ctx context.Context, client *http.Client, rawURL, token string) ([]byte, runtimeProbe) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, runtimeProbe{Error: fmt.Sprintf("create NodeX request: %v", err)}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-API-Key", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, runtimeProbe{Error: err.Error()}
	}
	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	probe := runtimeProbe{StatusCode: resp.StatusCode, OK: resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices}
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

func nodeXOperatorCommands(baseURL string) runtimeCommandHints {
	normalized := strings.TrimSpace(baseURL)
	if normalized == "" {
		normalized = "http://127.0.0.1:18081"
	}
	return runtimeCommandHints{
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
