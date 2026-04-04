package clients

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ClientType 客户端类型
type ClientType string

const (
	ClientXray  ClientType = "xray"
	ClientMihomo ClientType = "mihomo"
)

// ClientStatus 客户端状态
type ClientStatus string

const (
	StatusStopped  ClientStatus = "stopped"
	StatusStarting ClientStatus = "starting"
	StatusRunning  ClientStatus = "running"
	StatusError    ClientStatus = "error"
)

// Client 客户端接口
type Client interface {
	// Name 返回客户端名称
	Name() string

	// Type 返回客户端类型
	Type() ClientType

	// Start 启动客户端
	Start(ctx context.Context) error

	// Stop 停止客户端
	Stop() error

	// Restart 重启客户端
	Restart(ctx context.Context) error

	// Status 获取客户端状态
	Status() ClientStatus

	// IsHealthy 检查客户端是否健康
	IsHealthy(ctx context.Context) bool

	// ProxyAddr 返回代理地址
	ProxyAddr() string

	// HTTPProxyAddr 返回 HTTP 代理地址
	HTTPProxyAddr() string

	// SocksProxyAddr 返回 SOCKS 代理地址
	SocksProxyAddr() string

	// Logs 返回日志
	Logs() string

	// SetConfig 设置配置文件路径
	SetConfig(configPath string)
}

// BaseClient 基础客户端实现
type BaseClient struct {
	name         string
	clientType   ClientType
	configPath   string
	binaryPath   string
	cmd          *exec.Cmd
	status       ClientStatus
	logs         strings.Builder
	httpPort     int
	socksPort    int
	mixedPort    int
	apiPort      int
}

// ClientOption 客户端选项
type ClientOption func(*BaseClient)

// WithName 设置名称
func WithName(name string) ClientOption {
	return func(c *BaseClient) {
		c.name = name
	}
}

// WithConfig 设置配置文件
func WithConfig(configPath string) ClientOption {
	return func(c *BaseClient) {
		c.configPath = configPath
	}
}

// WithBinary 设置二进制文件路径
func WithBinary(binaryPath string) ClientOption {
	return func(c *BaseClient) {
		c.binaryPath = binaryPath
	}
}

// WithPorts 设置端口
func WithPorts(http, socks, mixed, api int) ClientOption {
	return func(c *BaseClient) {
		c.httpPort = http
		c.socksPort = socks
		c.mixedPort = mixed
		c.apiPort = api
	}
}

// NewBaseClient 创建基础客户端
func NewBaseClient(clientType ClientType, opts ...ClientOption) *BaseClient {
	c := &BaseClient{
		name:       string(clientType),
		clientType: clientType,
		status:     StatusStopped,
		httpPort:   10809,
		socksPort:  10808,
		mixedPort:  10810,
		apiPort:    9090,
	}

	for _, opt := range opts {
		opt(c)
	}

	// 自动查找二进制文件
	if c.binaryPath == "" {
		c.binaryPath = findBinary(string(clientType))
	}

	return c
}

// Name 返回客户端名称
func (c *BaseClient) Name() string {
	return c.name
}

// Type 返回客户端类型
func (c *BaseClient) Type() ClientType {
	return c.clientType
}

// Status 获取客户端状态
func (c *BaseClient) Status() ClientStatus {
	return c.status
}

// ProxyAddr 返回代理地址（优先使用 mixed port）
func (c *BaseClient) ProxyAddr() string {
	if c.mixedPort > 0 {
		return fmt.Sprintf("127.0.0.1:%d", c.mixedPort)
	}
	return fmt.Sprintf("socks5://127.0.0.1:%d", c.socksPort)
}

// HTTPProxyAddr 返回 HTTP 代理地址
func (c *BaseClient) HTTPProxyAddr() string {
	return fmt.Sprintf("http://127.0.0.1:%d", c.httpPort)
}

// SocksProxyAddr 返回 SOCKS 代理地址
func (c *BaseClient) SocksProxyAddr() string {
	return fmt.Sprintf("socks5://127.0.0.1:%d", c.socksPort)
}

// SetConfig 设置配置文件路径
func (c *BaseClient) SetConfig(configPath string) {
	c.configPath = configPath
}

// Logs 返回日志
func (c *BaseClient) Logs() string {
	return c.logs.String()
}

// Stop 停止客户端
func (c *BaseClient) Stop() error {
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}

	c.status = StatusStopped

	// 尝试优雅关闭
	if err := c.cmd.Process.Signal(os.Interrupt); err != nil {
		// 强制杀死
		c.cmd.Process.Kill()
	}

	// 等待进程退出
	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()

	select {
	case <-done:
		c.cmd = nil
		return nil
	case <-time.After(5 * time.Second):
		c.cmd.Process.Kill()
		c.cmd = nil
		return nil
	}
}

// binaryManager 全局二进制管理器（延迟初始化）
var binaryManager interface {
	EnsureBinary(name string) (string, error)
}

// SetBinaryManager 设置二进制管理器
func SetBinaryManager(m interface {
	EnsureBinary(name string) (string, error)
}) {
	binaryManager = m
}

// findBinary 查找客户端二进制文件
func findBinary(name string) string {
	// 常见的可执行文件名
	var binaryNames []string
	switch name {
	case "xray":
		binaryNames = []string{"xray", "xray.exe"}
	case "mihomo":
		binaryNames = []string{"mihomo", "mihomo.exe", "clash-meta", "clash-meta.exe"}
	}

	// 在 PATH 中查找
	for _, bin := range binaryNames {
		if path, err := exec.LookPath(bin); err == nil {
			return path
		}
	}

	// 在当前目录和子目录中查找
	searchDirs := []string{
		".",
		"./bin",
		"./clients",
		"/usr/local/bin",
		"/usr/bin",
	}

	if runtime.GOOS == "windows" {
		// Windows 常见位置
		searchDirs = append(searchDirs,
			filepath.Join(os.Getenv("ProgramFiles"), name),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), name),
		)
	}

	for _, dir := range searchDirs {
		for _, bin := range binaryNames {
			path := filepath.Join(dir, bin)
			if _, err := os.Stat(path); err == nil {
				absPath, _ := filepath.Abs(path)
				return absPath
			}
		}
	}

	// 如果设置了二进制管理器，尝试自动下载
	if binaryManager != nil {
		if path, err := binaryManager.EnsureBinary(name); err == nil {
			return path
		}
	}

	return name // 返回名称，让系统在 PATH 中查找
}