package gost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client gost API 客户端
type Client struct {
	baseURL    string
	authUser   string
	authPass   string
	httpClient *http.Client
}

// Config 客户端配置
type Config struct {
	Host     string // gost API 地址，如 "http://192.168.1.1:18080"
	APIToken string // API 认证令牌
	Timeout  time.Duration
}

// NewClient 创建客户端
func NewClient(cfg *Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	return &Client{
		baseURL:  cfg.Host,
		authPass: cfg.APIToken,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// ============== 通用方法 ==============

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
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

// ============== 重载配置 ==============

// Reload 重载配置
func (c *Client) Reload(ctx context.Context) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/api/config/reload", nil)
	return err
}