package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"
)

// XrayClient Xray 客户端
type XrayClient struct {
	*BaseClient
}

// NewXrayClient 创建 Xray 客户端
func NewXrayClient(opts ...ClientOption) *XrayClient {
	return &XrayClient{
		BaseClient: NewBaseClient(ClientXray, opts...),
	}
}

// Start 启动 Xray 客户端
func (c *XrayClient) Start(ctx context.Context) error {
	if c.configPath == "" {
		return fmt.Errorf("config path not set")
	}

	if _, err := os.Stat(c.configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", c.configPath)
	}

	c.status = StatusStarting

	// 构建命令
	args := []string{"run", "-c", c.configPath}
	c.cmd = exec.CommandContext(ctx, c.binaryPath, args...)

	// 设置环境变量
	c.cmd.Env = append(os.Environ(),
		"XRAY_LOG_LEVEL=warning",
	)

	// 捕获输出
	c.cmd.Stdout = &c.logs
	c.cmd.Stderr = &c.logs

	// 启动进程
	if err := c.cmd.Start(); err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to start xray: %w", err)
	}

	// 等待启动完成
	if err := c.waitForStart(ctx); err != nil {
		c.status = StatusError
		return err
	}

	c.status = StatusRunning
	return nil
}

// Restart 重启客户端
func (c *XrayClient) Restart(ctx context.Context) error {
	if err := c.Stop(); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	return c.Start(ctx)
}

// IsHealthy 检查客户端是否健康
func (c *XrayClient) IsHealthy(ctx context.Context) bool {
	if c.status != StatusRunning {
		return false
	}

	// 通过代理发送测试请求
	proxyURL, err := url.Parse(c.HTTPProxyAddr())
	if err != nil {
		return false
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "http://www.gstatic.com/generate_204", nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 204 || resp.StatusCode == 200
}

// waitForStart 等待启动完成
func (c *XrayClient) waitForStart(ctx context.Context) error {
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	proxyURL, err := url.Parse(c.HTTPProxyAddr())
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for xray to start")
		case <-ticker.C:
			// 检查进程是否还在运行
			if c.cmd == nil || c.cmd.Process == nil {
				return fmt.Errorf("xray process exited unexpectedly")
			}

			// 尝试连接代理
			client := &http.Client{
				Timeout: 1 * time.Second,
				Transport: &http.Transport{
					Proxy: http.ProxyURL(proxyURL),
				},
			}

			req, _ := http.NewRequest("GET", "http://www.gstatic.com/generate_204", nil)
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				return nil
			}
		}
	}
}

// MihomoClient Mihomo (Clash.Meta) 客户端
type MihomoClient struct {
	*BaseClient
	apiURL string
}

// NewMihomoClient 创建 Mihomo 客户端
func NewMihomoClient(opts ...ClientOption) *MihomoClient {
	base := NewBaseClient(ClientMihomo, opts...)
	// Mihomo 默认使用 mixed port
	base.httpPort = 0 // Mihomo 不单独提供 HTTP 端口
	return &MihomoClient{
		BaseClient: base,
		apiURL:     fmt.Sprintf("http://127.0.0.1:%d", base.apiPort),
	}
}

// Start 启动 Mihomo 客户端
func (c *MihomoClient) Start(ctx context.Context) error {
	if c.configPath == "" {
		return fmt.Errorf("config path not set")
	}

	if _, err := os.Stat(c.configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", c.configPath)
	}

	c.status = StatusStarting

	// 构建命令
	args := []string{"-f", c.configPath}
	c.cmd = exec.CommandContext(ctx, c.binaryPath, args...)

	// 捕获输出
	stdoutPipe, err := c.cmd.StdoutPipe()
	if err != nil {
		c.status = StatusError
		return err
	}
	stderrPipe, err := c.cmd.StderrPipe()
	if err != nil {
		c.status = StatusError
		return err
	}

	go io.Copy(&c.logs, stdoutPipe)
	go io.Copy(&c.logs, stderrPipe)

	// 启动进程
	if err := c.cmd.Start(); err != nil {
		c.status = StatusError
		return fmt.Errorf("failed to start mihomo: %w", err)
	}

	// 等待启动完成
	if err := c.waitForStart(ctx); err != nil {
		c.status = StatusError
		return err
	}

	c.status = StatusRunning
	return nil
}

// Restart 重启客户端
func (c *MihomoClient) Restart(ctx context.Context) error {
	if err := c.Stop(); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	return c.Start(ctx)
}

// IsHealthy 检查客户端是否健康
func (c *MihomoClient) IsHealthy(ctx context.Context) bool {
	if c.status != StatusRunning {
		return false
	}

	// 通过 API 检查健康状态
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", c.apiURL, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}

// waitForStart 等待启动完成
func (c *MihomoClient) waitForStart(ctx context.Context) error {
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for mihomo to start")
		case <-ticker.C:
			// 检查进程是否还在运行
			if c.cmd == nil || c.cmd.Process == nil {
				return fmt.Errorf("mihomo process exited unexpectedly")
			}

			// 尝试连接 API
			client := &http.Client{Timeout: 1 * time.Second}
			req, _ := http.NewRequest("GET", c.apiURL, nil)
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				return nil
			}
		}
	}
}

// GetProxies 获取代理列表 (Mihomo 特有)
func (c *MihomoClient) GetProxies(ctx context.Context) (map[string]interface{}, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", c.apiURL+"/proxies", nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// SelectProxy 选择代理 (Mihomo 特有)
func (c *MihomoClient) SelectProxy(ctx context.Context, group, proxy string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	apiURL := fmt.Sprintf("%s/proxies/%s", c.apiURL, group)

	req, err := http.NewRequestWithContext(ctx, "PUT", apiURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("failed to select proxy: status %d", resp.StatusCode)
	}

	return nil
}

// DelayTest 延迟测试 (Mihomo 特有)
func (c *MihomoClient) DelayTest(ctx context.Context, proxyName string) (int, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	apiURL := fmt.Sprintf("%s/proxies/%s/delay?timeout=5000&url=http://www.gstatic.com/generate_204",
		c.apiURL, proxyName)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return 0, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	return result.Delay, nil
}