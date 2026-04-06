package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

const (
	forwardRuntimeNodeXBaseURLConfigKey        = "forward.runtime.nodex.base_url"
	forwardRuntimeNodeXTokenConfigKey          = "forward.runtime.nodex.token"
	forwardRuntimeNodeXTimeoutSecondsConfigKey = "forward.runtime.nodex.timeout_seconds"

	defaultForwardRuntimeNodeXExecutePath = "/api/v2/internal/forward/runtime/execute"
	defaultForwardRuntimeNodeXTimeout     = 15 * time.Second

	nodeXForwardResourceTypePanelForward = "panel_forward"
	nodeXForwardResourceTypeLegacyRule   = "legacy_rule"

	// Legacy test compatibility constants kept while the request contract is shared.
	nodeXForwardRuntimeApplyPath = defaultForwardRuntimeNodeXExecutePath
	nodeXForwardRuntimeSource    = nodeXForwardResourceTypeLegacyRule
)

type forwardRuntimeNodeXExecutor interface {
	Execute(ctx context.Context, req nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error)
}

type nodeXForwardHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type nodeXForwardRuntimeClient struct {
	configService *SystemConfigService
	httpClient    nodeXForwardHTTPDoer
}

type nodeXForwardRuntimeSettings struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

type nodeXForwardExecuteRequest struct {
	ResourceType   string                             `json:"resourceType"`
	Backend        string                             `json:"backend"`
	Action         string                             `json:"action"`
	PanelForward   *nodeXPanelForwardRequest          `json:"panelForward,omitempty"`
	AnsibleRuntime *panelForwardAnsibleRuntimePayload `json:"ansibleRuntime,omitempty"`
	LegacyRule     *nodeXLegacyForwardRuleInput       `json:"legacyRule,omitempty"`
}

type nodeXForwardExecuteResponse struct {
	Data    *nodeXForwardExecuteResult `json:"data,omitempty"`
	Error   string                     `json:"error,omitempty"`
	Msg     string                     `json:"msg,omitempty"`
	Message string                     `json:"message,omitempty"`
	Backend string                     `json:"backend,omitempty"`
	Status  int                        `json:"status,omitempty"`
	Result  string                     `json:"result,omitempty"`
	Async   bool                       `json:"async,omitempty"`
}

type nodeXForwardExecuteResult struct {
	Backend string `json:"backend"`
	Status  int    `json:"status"`
	Message string `json:"message"`
	Result  string `json:"result,omitempty"`
	Async   bool   `json:"async"`
}

type nodeXPanelForwardRequest struct {
	Forward     nodeXPanelForwardPayload    `json:"forward"`
	Tunnel      nodeXPanelTunnelPayload     `json:"tunnel"`
	IngressNode nodeXForwardNodePayload     `json:"ingressNode"`
	Limiter     *panelForwardLimiterPayload `json:"limiter,omitempty"`
}

type nodeXPanelForwardPayload struct {
	ID            uint   `json:"id"`
	UserID        uint   `json:"userId"`
	Name          string `json:"name"`
	InPort        int    `json:"inPort"`
	RemoteAddr    string `json:"remoteAddr"`
	InterfaceName string `json:"interfaceName"`
	Strategy      string `json:"strategy"`
	Status        int    `json:"status"`
}

type panelForwardLimiterPayload struct {
	SpeedID uint   `json:"speedId"`
	Name    string `json:"name,omitempty"`
	Speed   int64  `json:"speed"`
}

type nodeXPanelTunnelPayload struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	InNodeID      uint   `json:"inNodeId"`
	Protocol      string `json:"protocol"`
	TCPListenAddr string `json:"tcpListenAddr"`
	UDPListenAddr string `json:"udpListenAddr"`
	InterfaceName string `json:"interfaceName"`
}

type nodeXLegacyForwardRuleInput struct {
	Rule      nodeXLegacyForwardRulePayload `json:"rule"`
	RelayNode nodeXForwardNodePayload       `json:"relayNode"`
	ExitNode  nodeXForwardNodePayload       `json:"exitNode"`
}

type nodeXLegacyForwardRulePayload struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	ListenPort int    `json:"listenPort"`
	Protocol   string `json:"protocol"`
	TargetHost string `json:"targetHost"`
	TargetPort int    `json:"targetPort"`
	Enabled    bool   `json:"enabled"`
}

type nodeXForwardNodePayload struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	APIPort  int    `json:"apiPort"`
	APIToken string `json:"apiToken"`
}

// nodeXForwardRuntimeRequest is a compatibility decoder kept for existing legacy
// provider tests that asserted the older flat request body shape.
type nodeXForwardRuntimeRequest struct {
	Source    string                        `json:"source"`
	Action    string                        `json:"action"`
	Rule      nodeXLegacyForwardRulePayload `json:"rule"`
	RelayNode nodeXForwardNodePayload       `json:"relayNode"`
	ExitNode  nodeXForwardNodePayload       `json:"exitNode"`
}

func (r *nodeXForwardRuntimeRequest) UnmarshalJSON(data []byte) error {
	type legacyAlias nodeXForwardRuntimeRequest
	var legacy legacyAlias
	if err := json.Unmarshal(data, &legacy); err == nil {
		if legacy.Source != "" || legacy.Rule.ID != 0 || legacy.Action != "" {
			*r = nodeXForwardRuntimeRequest(legacy)
			return nil
		}
	}

	var req nodeXForwardExecuteRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}

	r.Source = req.ResourceType
	r.Action = req.Action
	if req.LegacyRule != nil {
		r.Rule = req.LegacyRule.Rule
		r.RelayNode = req.LegacyRule.RelayNode
		r.ExitNode = req.LegacyRule.ExitNode
	}
	return nil
}

func newNodeXForwardRuntimeClient(configService *SystemConfigService) *nodeXForwardRuntimeClient {
	return &nodeXForwardRuntimeClient{configService: configService}
}

func NewNodeXForwardRuntimeProviderWithHTTPClient(db *gorm.DB, httpClient nodeXForwardHTTPDoer) *nodeXForwardRuntimeProvider {
	configService := NewSystemConfigService(db)
	return &nodeXForwardRuntimeProvider{
		db: db,
		client: &nodeXForwardRuntimeClient{
			configService: configService,
			httpClient:    httpClient,
		},
	}
}

func (c *nodeXForwardRuntimeClient) Execute(ctx context.Context, req nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error) {
	settings, err := c.loadSettings()
	if err != nil {
		return nil, err
	}

	targetNode := resolveNodeXTargetNode(req)
	baseURL, err := resolveNodeXBaseURL(targetNode, settings, requiresConfiguredNodeXControlPlane(req))
	if err != nil {
		return nil, err
	}

	token := resolveNodeXToken(targetNode, settings)

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal NodeX forward runtime request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		baseURL+defaultForwardRuntimeNodeXExecutePath,
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, fmt.Errorf("create NodeX forward runtime request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
		httpReq.Header.Set("X-API-Key", token)
	}
	if targetNode != nil && targetNode.ID != 0 {
		httpReq.Header.Set("X-Forward-Node-ID", strconv.FormatUint(uint64(targetNode.ID), 10))
	}

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: settings.Timeout}
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute NodeX forward runtime request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read NodeX forward runtime response: %w", err)
	}

	var apiResp nodeXForwardExecuteResponse
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &apiResp); err != nil {
			return nil, fmt.Errorf("decode NodeX forward runtime response: %w", err)
		}
	}

	result := apiResp.toResult(resp.StatusCode, req.Backend)
	if resp.StatusCode >= http.StatusBadRequest {
		message := strings.TrimSpace(apiResp.errorMessage())
		if message == "" {
			message = strings.TrimSpace(string(body))
		}
		if message == "" {
			message = fmt.Sprintf("NodeX forward runtime returned %s", resp.Status)
		}
		if result != nil {
			return result, fmt.Errorf("%s", message)
		}
		return nil, fmt.Errorf("%s", message)
	}

	if result == nil {
		return &nodeXForwardExecuteResult{
			Backend: req.Backend,
			Status:  model.ForwardRuntimeJobStatusSuccess,
		}, nil
	}
	return result, nil
}

func (c *nodeXForwardRuntimeClient) loadSettings() (*nodeXForwardRuntimeSettings, error) {
	settings := &nodeXForwardRuntimeSettings{
		Timeout: defaultForwardRuntimeNodeXTimeout,
	}
	if c.configService == nil {
		return settings, nil
	}

	baseURL, err := c.configService.Get(forwardRuntimeNodeXBaseURLConfigKey)
	if err != nil {
		return nil, err
	}
	settings.BaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")

	token, err := c.configService.Get(forwardRuntimeNodeXTokenConfigKey)
	if err != nil {
		return nil, err
	}
	settings.Token = strings.TrimSpace(token)

	timeoutValue, err := c.configService.Get(forwardRuntimeNodeXTimeoutSecondsConfigKey)
	if err != nil {
		return nil, err
	}
	timeoutValue = strings.TrimSpace(timeoutValue)
	if timeoutValue != "" {
		seconds, err := strconv.Atoi(timeoutValue)
		if err != nil {
			return nil, fmt.Errorf("invalid %s value: %w", forwardRuntimeNodeXTimeoutSecondsConfigKey, err)
		}
		if seconds <= 0 {
			return nil, fmt.Errorf("%s must be greater than zero", forwardRuntimeNodeXTimeoutSecondsConfigKey)
		}
		settings.Timeout = time.Duration(seconds) * time.Second
	}

	return settings, nil
}

func resolveNodeXTargetNode(req nodeXForwardExecuteRequest) *nodeXForwardNodePayload {
	if req.PanelForward != nil && hasNodeXForwardNode(req.PanelForward.IngressNode) {
		node := req.PanelForward.IngressNode
		return &node
	}
	if req.LegacyRule != nil && hasNodeXForwardNode(req.LegacyRule.RelayNode) {
		node := req.LegacyRule.RelayNode
		return &node
	}
	return nil
}

func requiresConfiguredNodeXControlPlane(req nodeXForwardExecuteRequest) bool {
	switch req.ResourceType {
	case nodeXForwardResourceTypePanelForward, nodeXForwardResourceTypeLegacyRule:
		return true
	default:
		return req.AnsibleRuntime != nil
	}
}

func hasNodeXForwardNode(node nodeXForwardNodePayload) bool {
	return node.ID != 0 ||
		strings.TrimSpace(node.Host) != "" ||
		node.APIPort > 0 ||
		strings.TrimSpace(node.APIToken) != ""
}

func resolveNodeXBaseURL(node *nodeXForwardNodePayload, settings *nodeXForwardRuntimeSettings, requireConfigured bool) (string, error) {
	if settings != nil && strings.TrimSpace(settings.BaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/"), nil
	}
	if requireConfigured {
		return "", fmt.Errorf("%s is required for NodeX forward runtime", forwardRuntimeNodeXBaseURLConfigKey)
	}
	if node == nil {
		return "", fmt.Errorf("forward runtime target node is required")
	}

	host := strings.TrimSpace(node.Host)
	if host == "" {
		return "", fmt.Errorf("forward runtime target node host is required")
	}

	if strings.Contains(host, "://") {
		parsed, err := url.Parse(host)
		if err != nil {
			return "", fmt.Errorf("invalid forward runtime target node host %q: %w", host, err)
		}
		if parsed.Host == "" && parsed.Path != "" {
			parsed.Host = parsed.Path
			parsed.Path = ""
		}
		if parsed.Scheme == "" {
			parsed.Scheme = "http"
		}
		if parsed.Host == "" {
			return "", fmt.Errorf("forward runtime target node host is required")
		}
		if parsed.Port() == "" {
			if node.APIPort <= 0 {
				return "", fmt.Errorf("forward runtime target node api port is required")
			}
			parsed.Host = net.JoinHostPort(parsed.Hostname(), strconv.Itoa(node.APIPort))
		}
		return strings.TrimRight(parsed.String(), "/"), nil
	}

	if node.APIPort <= 0 {
		return "", fmt.Errorf("forward runtime target node api port is required")
	}

	return "http://" + net.JoinHostPort(host, strconv.Itoa(node.APIPort)), nil
}

func resolveNodeXToken(node *nodeXForwardNodePayload, settings *nodeXForwardRuntimeSettings) string {
	if settings != nil && strings.TrimSpace(settings.Token) != "" {
		return strings.TrimSpace(settings.Token)
	}
	if node != nil && strings.TrimSpace(node.APIToken) != "" {
		return strings.TrimSpace(node.APIToken)
	}
	if settings != nil {
		return strings.TrimSpace(settings.Token)
	}
	return ""
}

func (r *nodeXForwardExecuteResponse) toResult(statusCode int, fallbackBackend string) *nodeXForwardExecuteResult {
	if r == nil {
		return defaultNodeXForwardExecuteResult(statusCode, fallbackBackend, "")
	}
	if r.Data != nil {
		return r.Data
	}

	message := strings.TrimSpace(r.Message)
	if message == "" {
		message = strings.TrimSpace(r.Msg)
	}

	if strings.TrimSpace(r.Backend) != "" || r.Status != 0 || strings.TrimSpace(r.Result) != "" || r.Async || message != "" {
		return &nodeXForwardExecuteResult{
			Backend: r.Backend,
			Status:  r.Status,
			Message: message,
			Result:  r.Result,
			Async:   r.Async,
		}
	}

	return defaultNodeXForwardExecuteResult(statusCode, fallbackBackend, message)
}

func (r *nodeXForwardExecuteResponse) errorMessage() string {
	if r == nil {
		return ""
	}
	if strings.TrimSpace(r.Error) != "" {
		return r.Error
	}
	if strings.TrimSpace(r.Message) != "" {
		return r.Message
	}
	return r.Msg
}

func defaultNodeXForwardExecuteResult(statusCode int, backend, message string) *nodeXForwardExecuteResult {
	switch {
	case statusCode == http.StatusAccepted:
		if message == "" {
			message = http.StatusText(statusCode)
		}
		return &nodeXForwardExecuteResult{
			Backend: backend,
			Status:  model.ForwardRuntimeJobStatusPending,
			Message: message,
			Async:   true,
		}
	case statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices:
		return &nodeXForwardExecuteResult{
			Backend: backend,
			Status:  model.ForwardRuntimeJobStatusSuccess,
			Message: message,
		}
	default:
		return nil
	}
}
