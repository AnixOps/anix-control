package local

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Server 本地代理服务器
type Server struct {
	name       string
	binaryPath string
	configPath string
	port       int
	cmd        *exec.Cmd
	status     ServerStatus
	mu         sync.Mutex
}

// ServerStatus 服务器状态
type ServerStatus string

const (
	ServerStopped ServerStatus = "stopped"
	ServerRunning ServerStatus = "running"
	ServerError   ServerStatus = "error"
)

// ServerOption 服务器选项
type ServerOption func(*Server)

// WithServerPort 设置服务器端口
func WithServerPort(port int) ServerOption {
	return func(s *Server) {
		s.port = port
	}
}

// WithServerBinary 设置二进制路径
func WithServerBinary(path string) ServerOption {
	return func(s *Server) {
		s.binaryPath = path
	}
}

// NewServer 创建本地服务器
func NewServer(name string, opts ...ServerOption) *Server {
	s := &Server{
		name:   name,
		port:   0, // 自动分配
		status: ServerStopped,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// SetConfig 设置配置文件
func (s *Server) SetConfig(configPath string) {
	s.configPath = configPath
}

// Start 启动服务器
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.configPath == "" {
		return fmt.Errorf("config path not set")
	}

	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", s.configPath)
	}

	// 查找二进制文件
	binary := s.binaryPath
	if binary == "" {
		binary = findBinary(s.name)
	}

	// 构建命令
	var cmd *exec.Cmd
	switch s.name {
	case "xray":
		cmd = exec.CommandContext(ctx, binary, "run", "-c", s.configPath)
	case "mihomo":
		cmd = exec.CommandContext(ctx, binary, "-f", s.configPath)
	default:
		return fmt.Errorf("unknown server type: %s", s.name)
	}

	// 设置环境
	cmd.Env = append(os.Environ(),
		"XRAY_LOG_LEVEL=warning",
		"CLASH_LOG_LEVEL=warning",
	)

	// 捕获输出（可选）
	if runtime.GOOS != "windows" {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	// 启动进程
	if err := cmd.Start(); err != nil {
		s.status = ServerError
		return fmt.Errorf("failed to start %s: %w", s.name, err)
	}

	s.cmd = cmd
	s.status = ServerRunning

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	s.status = ServerStopped

	// 优雅关闭
	if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
		s.cmd.Process.Kill()
	}

	// 等待进程退出
	done := make(chan error, 1)
	go func() {
		done <- s.cmd.Wait()
	}()

	select {
	case <-done:
		s.cmd = nil
		return nil
	case <-time.After(5 * time.Second):
		s.cmd.Process.Kill()
		s.cmd = nil
		return nil
	}
}

// Status 获取状态
func (s *Server) Status() ServerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Port 获取端口
func (s *Server) Port() int {
	return s.port
}

// IsRunning 检查是否运行中
func (s *Server) IsRunning() bool {
	return s.status == ServerRunning && s.cmd != nil && s.cmd.Process != nil
}

// findBinary 查找二进制文件
func findBinary(name string) string {
	var binaryNames []string
	switch name {
	case "xray":
		binaryNames = []string{"xray", "xray.exe"}
	case "mihomo":
		binaryNames = []string{"mihomo", "mihomo.exe", "clash-meta", "clash-meta.exe"}
	}

	for _, bin := range binaryNames {
		if path, err := exec.LookPath(bin); err == nil {
			return path
		}
	}

	return name
}

// Environment 本地测试环境
type Environment struct {
	server     *Server
	serverPort int
	clientPort int
	configDir  string
	cleanup    []func()
	mu         sync.Mutex
}

// NewEnvironment 创建本地测试环境
func NewEnvironment(configDir string) *Environment {
	if configDir == "" {
		configDir = os.TempDir()
	}
	return &Environment{
		configDir: configDir,
		cleanup:   make([]func(), 0),
	}
}

// Setup 设置环境
func (e *Environment) Setup(ctx context.Context, protocol string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 获取空闲端口
	serverPort, err := getFreePort()
	if err != nil {
		return fmt.Errorf("failed to get server port: %w", err)
	}
	e.serverPort = serverPort

	clientPort, err := getFreePort()
	if err != nil {
		return fmt.Errorf("failed to get client port: %w", err)
	}
	e.clientPort = clientPort

	// 创建配置目录
	if err := os.MkdirAll(e.configDir, 0755); err != nil {
		return err
	}

	return nil
}

// ServerPort 获取服务器端口
func (e *Environment) ServerPort() int {
	return e.serverPort
}

// ClientPort 获取客户端端口
func (e *Environment) ClientPort() int {
	return e.clientPort
}

// Teardown 清理环境
func (e *Environment) Teardown() {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 按相反顺序执行清理
	for i := len(e.cleanup) - 1; i >= 0; i-- {
		e.cleanup[i]()
	}
	e.cleanup = nil
}

// addCleanup 添加清理函数
func (e *Environment) addCleanup(fn func()) {
	e.cleanup = append(e.cleanup, fn)
}

// getFreePort 获取空闲端口
func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
}

// WaitForPort 等待端口可用
func WaitForPort(port int, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 1*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for port %d", port)
}

// WaitForHTTP 等待 HTTP 服务可用
func WaitForHTTP(url string, timeout time.Duration) error {
	start := time.Now()
	client := &http.Client{Timeout: 1 * time.Second}

	for time.Since(start) < timeout {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for HTTP %s", url)
}

// ConfigBuilder 配置构建器
type ConfigBuilder struct {
	protocol   string
	serverPort int
	clientPort int
	password   string
	uuid       string
	method     string
	network    string
	tls        bool
}

// NewConfigBuilder 创建配置构建器
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		protocol:   "shadowsocks",
		network:    "tcp",
		method:     "aes-256-gcm",
		password:   "test-password",
		uuid:       "test-uuid-1234",
	}
}

// Protocol 设置协议
func (b *ConfigBuilder) Protocol(p string) *ConfigBuilder {
	b.protocol = p
	return b
}

// ServerPort 设置服务器端口
func (b *ConfigBuilder) ServerPort(port int) *ConfigBuilder {
	b.serverPort = port
	return b
}

// ClientPort 设置客户端端口
func (b *ConfigBuilder) ClientPort(port int) *ConfigBuilder {
	b.clientPort = port
	return b
}

// Password 设置密码
func (b *ConfigBuilder) Password(p string) *ConfigBuilder {
	b.password = p
	return b
}

// UUID 设置 UUID
func (b *ConfigBuilder) UUID(u string) *ConfigBuilder {
	b.uuid = u
	return b
}

// Method 设置加密方法
func (b *ConfigBuilder) Method(m string) *ConfigBuilder {
	b.method = m
	return b
}

// Network 设置传输层
func (b *ConfigBuilder) Network(n string) *ConfigBuilder {
	b.network = n
	return b
}

// TLS 设置 TLS
func (b *ConfigBuilder) TLS(enabled bool) *ConfigBuilder {
	b.tls = enabled
	return b
}

// BuildServerConfig 构建服务端配置
func (b *ConfigBuilder) BuildServerConfig() ([]byte, error) {
	switch b.protocol {
	case "shadowsocks":
		return b.buildShadowsocksServerConfig()
	case "vmess":
		return b.buildVMessServerConfig()
	case "vless":
		return b.buildVLESSServerConfig()
	case "trojan":
		return b.buildTrojanServerConfig()
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", b.protocol)
	}
}

// BuildClientConfig 构建客户端配置
func (b *ConfigBuilder) BuildClientConfig() ([]byte, error) {
	switch b.protocol {
	case "shadowsocks":
		return b.buildShadowsocksClientConfig()
	case "vmess":
		return b.buildVMessClientConfig()
	case "vless":
		return b.buildVLESSClientConfig()
	case "trojan":
		return b.buildTrojanClientConfig()
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", b.protocol)
	}
}

// buildShadowsocksServerConfig 构建 Shadowsocks 服务端配置
func (b *ConfigBuilder) buildShadowsocksServerConfig() ([]byte, error) {
	config := fmt.Sprintf(`{
	"inbounds": [{
		"port": %d,
		"protocol": "shadowsocks",
		"settings": {
			"method": "%s",
			"password": "%s",
			"network": "tcp,udp"
		}
	}],
	"outbounds": [{
		"protocol": "freedom"
	}]
}`, b.serverPort, b.method, b.password)

	return []byte(config), nil
}

// buildShadowsocksClientConfig 构建 Shadowsocks 客户端配置
func (b *ConfigBuilder) buildShadowsocksClientConfig() ([]byte, error) {
	config := fmt.Sprintf(`{
	"inbounds": [{
		"port": %d,
		"listen": "127.0.0.1",
		"protocol": "socks",
		"settings": {
			"udp": true
		}
	}],
	"outbounds": [{
		"protocol": "shadowsocks",
		"settings": {
			"servers": [{
				"address": "127.0.0.1",
				"port": %d,
				"method": "%s",
				"password": "%s"
			}]
		}
	}]
}`, b.clientPort, b.serverPort, b.method, b.password)

	return []byte(config), nil
}

// buildVMessServerConfig 构建 VMess 服务端配置
func (b *ConfigBuilder) buildVMessServerConfig() ([]byte, error) {
	config := fmt.Sprintf(`{
	"inbounds": [{
		"port": %d,
		"protocol": "vmess",
		"settings": {
			"clients": [{
				"id": "%s",
				"alterId": 0
			}]
		}
	}],
	"outbounds": [{
		"protocol": "freedom"
	}]
}`, b.serverPort, b.uuid)

	return []byte(config), nil
}

// buildVMessClientConfig 构建 VMess 客户端配置
func (b *ConfigBuilder) buildVMessClientConfig() ([]byte, error) {
	config := fmt.Sprintf(`{
	"inbounds": [{
		"port": %d,
		"listen": "127.0.0.1",
		"protocol": "socks",
		"settings": {
			"udp": true
		}
	}],
	"outbounds": [{
		"protocol": "vmess",
		"settings": {
			"vnext": [{
				"address": "127.0.0.1",
				"port": %d,
				"users": [{
					"id": "%s",
					"alterId": 0
				}]
			}]
		}
	}]
}`, b.clientPort, b.serverPort, b.uuid)

	return []byte(config), nil
}

// buildVLESSServerConfig 构建 VLESS 服务端配置
func (b *ConfigBuilder) buildVLESSServerConfig() ([]byte, error) {
	config := fmt.Sprintf(`{
	"inbounds": [{
		"port": %d,
		"protocol": "vless",
		"settings": {
			"clients": [{
				"id": "%s",
				"flow": ""
			}],
			"decryption": "none"
		}
	}],
	"outbounds": [{
		"protocol": "freedom"
	}]
}`, b.serverPort, b.uuid)

	return []byte(config), nil
}

// buildVLESSClientConfig 构建 VLESS 客户端配置
func (b *ConfigBuilder) buildVLESSClientConfig() ([]byte, error) {
	config := fmt.Sprintf(`{
	"inbounds": [{
		"port": %d,
		"listen": "127.0.0.1",
		"protocol": "socks",
		"settings": {
			"udp": true
		}
	}],
	"outbounds": [{
		"protocol": "vless",
		"settings": {
			"vnext": [{
				"address": "127.0.0.1",
				"port": %d,
				"users": [{
					"id": "%s",
					"encryption": "none"
				}]
			}]
		}
	}]
}`, b.clientPort, b.serverPort, b.uuid)

	return []byte(config), nil
}

// buildTrojanServerConfig 构建 Trojan 服务端配置
func (b *ConfigBuilder) buildTrojanServerConfig() ([]byte, error) {
	config := fmt.Sprintf(`{
	"inbounds": [{
		"port": %d,
		"protocol": "trojan",
		"settings": {
			"clients": [{
				"password": "%s"
			}]
		}
	}],
	"outbounds": [{
		"protocol": "freedom"
	}]
}`, b.serverPort, b.password)

	return []byte(config), nil
}

// buildTrojanClientConfig 构建 Trojan 客户端配置
func (b *ConfigBuilder) buildTrojanClientConfig() ([]byte, error) {
	config := fmt.Sprintf(`{
	"inbounds": [{
		"port": %d,
		"listen": "127.0.0.1",
		"protocol": "socks",
		"settings": {
			"udp": true
		}
	}],
	"outbounds": [{
		"protocol": "trojan",
		"settings": {
			"servers": [{
				"address": "127.0.0.1",
				"port": %d,
				"password": "%s"
			}]
		}
	}]
}`, b.clientPort, b.serverPort, b.password)

	return []byte(config), nil
}

// SaveConfig 保存配置到文件
func SaveConfig(dir, name string, data []byte) (string, error) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}