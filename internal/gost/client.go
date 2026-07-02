package gost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Client gost API 客户端
type Client struct {
	baseURL     string
	metricsHost string
	authUser    string
	authPass    string
	httpClient  *http.Client
}

// Config 客户端配置
type Config struct {
	Host        string // gost API 地址，如 "http://192.168.1.1:18080"
	MetricsHost string // gost Prometheus /metrics 地址，如 "http://192.168.1.1:9000"，为空表示未配置
	APIToken    string // API 认证令牌
	Timeout     time.Duration
}

// NewClient 创建客户端
func NewClient(cfg *Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	return &Client{
		baseURL:     cfg.Host,
		metricsHost: cfg.MetricsHost,
		authPass:    cfg.APIToken,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// HasMetricsEndpoint 是否已配置 Prometheus /metrics 地址
func (c *Client) HasMetricsEndpoint() bool {
	return c.metricsHost != ""
}

// ============== 通用方法 ==============

func (c *Client) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.authUser != "" || c.authPass != "" {
		req.SetBasicAuth(c.authUser, c.authPass)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
	}

	return respBody, nil
}

// ============== Service 管理 ==============

// ServiceConfig gost 服务配置
type ServiceConfig struct {
	Name      string            `json:"name"`
	Addr      string            `json:"addr"`
	Interface string            `json:"interface,omitempty"`
	Handler   *HandlerConfig    `json:"handler,omitempty"`
	Listener  *ListenerConfig   `json:"listener,omitempty"`
	Forwarder *ForwarderConfig  `json:"forwarder,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// HandlerConfig 处理器配置
type HandlerConfig struct {
	Type     string            `json:"type"`
	Auth     *AuthConfig       `json:"auth,omitempty"`
	Chain    string            `json:"chain,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ListenerConfig 监听器配置
type ListenerConfig struct {
	Type     string            `json:"type"`
	Auth     *AuthConfig       `json:"auth,omitempty"`
	Chain    string            `json:"chain,omitempty"`
	TLS      *TLSConfig        `json:"tls,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ForwarderConfig 转发器配置
type ForwarderConfig struct {
	Nodes    []ForwarderNode `json:"nodes,omitempty"`
	Selector *SelectorConfig `json:"selector,omitempty"`
}

// ForwarderNode 转发目标节点
type ForwarderNode struct {
	Name     string `json:"name"`
	Addr     string `json:"addr"`
	Protocol string `json:"protocol,omitempty"`
}

// SelectorConfig 选择器配置
type SelectorConfig struct {
	Strategy    string `json:"strategy"`              // round, rand, hash, failover
	MaxFails    int    `json:"maxFails,omitempty"`
	FailTimeout string `json:"failTimeout,omitempty"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// TLSConfig TLS 配置
type TLSConfig struct {
	CertFile   string `json:"certFile,omitempty"`
	KeyFile    string `json:"keyFile,omitempty"`
	CAFile     string `json:"caFile,omitempty"`
	ServerName string `json:"serverName,omitempty"`
	Secure     bool   `json:"secure,omitempty"`
}

// ServiceListResponse 服务列表响应
type ServiceListResponse struct {
	Count int               `json:"count"`
	List  []*ServiceConfig `json:"list"`
}

// GetServices 获取服务列表
func (c *Client) GetServices(ctx context.Context) (*ServiceListResponse, error) {
	respBody, err := c.doRequest(ctx, http.MethodGet, "/api/config/services", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data ServiceListResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp.Data, nil
}

// GetService 获取单个服务
func (c *Client) GetService(ctx context.Context, name string) (*ServiceConfig, error) {
	respBody, err := c.doRequest(ctx, http.MethodGet, "/api/config/services/"+name, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data *ServiceConfig `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return resp.Data, nil
}

// CreateService 创建服务
func (c *Client) CreateService(ctx context.Context, svc *ServiceConfig) error {
	req := struct {
		Data ServiceConfig `json:"data"`
	}{Data: *svc}

	_, err := c.doRequest(ctx, http.MethodPost, "/api/config/services", req)
	return err
}

// UpdateService 更新服务
func (c *Client) UpdateService(ctx context.Context, name string, svc *ServiceConfig) error {
	req := struct {
		Data ServiceConfig `json:"data"`
	}{Data: *svc}

	_, err := c.doRequest(ctx, http.MethodPut, "/api/config/services/"+name, req)
	return err
}

// DeleteService 删除服务
func (c *Client) DeleteService(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, "/api/config/services/"+name, nil)
	return err
}

// ============== Chain 管理 (多级转发) ==============

// ChainConfig 转发链配置
type ChainConfig struct {
	Name     string          `json:"name"`
	Selector *SelectorConfig `json:"selector,omitempty"`
	Hops     []string        `json:"hops,omitempty"`
}

// GetChains 获取转发链列表
func (c *Client) GetChains(ctx context.Context) ([]*ChainConfig, error) {
	respBody, err := c.doRequest(ctx, http.MethodGet, "/api/config/chains", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data struct {
			List []*ChainConfig `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return resp.Data.List, nil
}

// CreateChain 创建转发链
func (c *Client) CreateChain(ctx context.Context, chain *ChainConfig) error {
	req := struct {
		Data ChainConfig `json:"data"`
	}{Data: *chain}

	_, err := c.doRequest(ctx, http.MethodPost, "/api/config/chains", req)
	return err
}

// DeleteChain 删除转发链
func (c *Client) DeleteChain(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, "/api/config/chains/"+name, nil)
	return err
}

// ============== Hop 管理 (转发跳点) ==============

// HopConfig 跳点配置
type HopConfig struct {
	Name     string          `json:"name"`
	Selector *SelectorConfig `json:"selector,omitempty"`
	Nodes    []HopNode       `json:"nodes,omitempty"`
}

// HopNode 跳点节点
type HopNode struct {
	Name      string          `json:"name"`
	Addr      string          `json:"addr"`
	Interface string          `json:"interface,omitempty"`
	Connector *ConnectorConfig `json:"connector,omitempty"`
	Dialer    *DialerConfig    `json:"dialer,omitempty"`
}

// ConnectorConfig 连接器配置
type ConnectorConfig struct {
	Type     string            `json:"type"`
	Auth     *AuthConfig       `json:"auth,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// DialerConfig 拨号器配置
type DialerConfig struct {
	Type     string            `json:"type"`
	Auth     *AuthConfig       `json:"auth,omitempty"`
	TLS      *TLSConfig        `json:"tls,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// CreateHop 创建跳点
func (c *Client) CreateHop(ctx context.Context, hop *HopConfig) error {
	req := struct {
		Data HopConfig `json:"data"`
	}{Data: *hop}

	_, err := c.doRequest(ctx, http.MethodPost, "/api/config/hops", req)
	return err
}

// DeleteHop 删除跳点
func (c *Client) DeleteHop(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, "/api/config/hops/"+name, nil)
	return err
}

// ============== 健康检查 ==============

// HealthCheck 检查 gost API 是否可用
func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.doRequest(ctx, http.MethodGet, "/api/config/services", nil)
	return err
}

// ============== 统计信息 ==============

// StatsResponse 统计响应
type StatsResponse struct {
	Services []ServiceStats `json:"services"`
}

// ServiceStats 服务统计
type ServiceStats struct {
	Name      string          `json:"name"`
	Addr      string          `json:"addr"`
	Events    []ServiceEvent  `json:"events"`
	Current   CurrentStats    `json:"current"`
	Total     TotalStats      `json:"total"`
}

// ServiceEvent 服务事件
type ServiceEvent struct {
	Time   time.Time `json:"time"`
	Type   string    `json:"type"`
	Status string    `json:"status"`
}

// CurrentStats 当前统计
type CurrentStats struct {
	Connections int   `json:"connections"`
	Handled     int64 `json:"handled"`
	InBytes     int64 `json:"inBytes"`
	OutBytes    int64 `json:"outBytes"`
}

// TotalStats 总计统计
type TotalStats struct {
	Connections int64 `json:"connections"`
	Handled     int64 `json:"handled"`
	InBytes     int64 `json:"inBytes"`
	OutBytes    int64 `json:"outBytes"`
}

// GetStats 获取统计信息
func (c *Client) GetStats(ctx context.Context) (*StatsResponse, error) {
	respBody, err := c.doRequest(ctx, http.MethodGet, "/api/stats", nil)
	if err != nil {
		return nil, err
	}

	var resp StatsResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}

// GetServiceStats 获取单个服务统计
//
// Deprecated: gost v3.2.6 的管理 API (端口 18080) 不存在 /api/stats 系列接口，
// 调用这个方法会一直收到 404。字节计数只能通过 GetServiceTrafficTotals 从
// Prometheus /metrics 端口读取。保留此方法仅为兼容旧引用，不再作为采集手段。
func (c *Client) GetServiceStats(ctx context.Context, name string) (*ServiceStats, error) {
	respBody, err := c.doRequest(ctx, http.MethodGet, "/api/stats/services/"+name, nil)
	if err != nil {
		return nil, err
	}

	var resp ServiceStats
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}

// ============== Prometheus /metrics 流量统计 ==============

// ServiceTrafficTotals 单个 service 在所有 client 上的累计字节数
type ServiceTrafficTotals struct {
	InBytes  int64
	OutBytes int64
}

var gostMetricLineRe = regexp.MustCompile(`^(gost_service_transfer_(?:input|output)_bytes_total)\{([^}]*)\}\s+([0-9eE+\-.]+)\s*$`)

// GetServiceTrafficTotals 从 gost 的 Prometheus /metrics 端点读取指定 service 的累计流量。
// gost_service_transfer_input_bytes_total / _output_bytes_total 按 client IP 分行，
// 需要对同一个 service label 的所有行求和才是该服务的总流量。
func (c *Client) GetServiceTrafficTotals(ctx context.Context, serviceNames []string) (map[string]*ServiceTrafficTotals, error) {
	wanted := make(map[string]struct{}, len(serviceNames))
	for _, name := range serviceNames {
		wanted[name] = struct{}{}
	}

	all, err := c.fetchAllServiceTrafficTotals(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*ServiceTrafficTotals, len(serviceNames))
	for _, name := range serviceNames {
		result[name] = &ServiceTrafficTotals{}
	}
	for service, totals := range all {
		if _, ok := wanted[service]; ok {
			result[service] = totals
		}
	}
	return result, nil
}

// GetAllServiceTrafficTotals 从 gost 的 Prometheus /metrics 端点读取该节点所有 service 的累计流量，
// 用于节点级统计（不按具体 forward 过滤）。
func (c *Client) GetAllServiceTrafficTotals(ctx context.Context) (map[string]*ServiceTrafficTotals, error) {
	return c.fetchAllServiceTrafficTotals(ctx)
}

func (c *Client) fetchAllServiceTrafficTotals(ctx context.Context) (map[string]*ServiceTrafficTotals, error) {
	result := make(map[string]*ServiceTrafficTotals)
	if !c.HasMetricsEndpoint() {
		return result, fmt.Errorf("metrics endpoint not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.metricsHost+"/metrics", nil)
	if err != nil {
		return nil, fmt.Errorf("create metrics request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("metrics API error: %s - %s", resp.Status, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		matches := gostMetricLineRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		metricName := matches[1]
		labels := matches[2]
		valueStr := matches[3]

		serviceName := parsePrometheusLabel(labels, "service")
		if serviceName == "" {
			continue
		}

		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}

		totals, ok := result[serviceName]
		if !ok {
			totals = &ServiceTrafficTotals{}
			result[serviceName] = totals
		}
		switch metricName {
		case "gost_service_transfer_input_bytes_total":
			totals.InBytes += int64(value)
		case "gost_service_transfer_output_bytes_total":
			totals.OutBytes += int64(value)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read metrics response: %w", err)
	}

	return result, nil
}

// parsePrometheusLabel 从形如 `service="X",client="Y"` 的标签串里提取指定 key 的值
func parsePrometheusLabel(labels, key string) string {
	prefix := key + `="`
	idx := strings.Index(labels, prefix)
	if idx == -1 {
		return ""
	}
	rest := labels[idx+len(prefix):]
	end := strings.Index(rest, `"`)
	if end == -1 {
		return ""
	}
	return rest[:end]
}

// ============== 重载配置 ==============

// Reload 重载配置
func (c *Client) Reload(ctx context.Context) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/api/config/reload", nil)
	return err
}