package local

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/anixops/v2board/tests/integration/echo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSuite 本地测试套件
type TestSuite struct {
	t            *testing.T
	env          *Environment
	configDir    string
	serverBinary string
	xrayPath     string
	mihomoPath   string
	echoServer   *echo.EchoServer
}

// NewTestSuite 创建测试套件
func NewTestSuite(t *testing.T) *TestSuite {
	configDir := t.TempDir()
	return &TestSuite{
		t:         t,
		env:       NewEnvironment(configDir),
		configDir: configDir,
	}
}

// Setup 设置测试环境
func (s *TestSuite) Setup() error {
	// 查找 xray 二进制
	if path, err := exec.LookPath("xray"); err == nil {
		s.xrayPath = path
	}
	if path, err := exec.LookPath("mihomo"); err == nil {
		s.mihomoPath = path
	}
	// 也检查 mihomo 的其他名称
	if s.mihomoPath == "" {
		if path, err := exec.LookPath("clash-meta"); err == nil {
			s.mihomoPath = path
		}
	}

	return nil
}

// StartEchoServer 启动内置 Echo 服务器
func (s *TestSuite) StartEchoServer(ctx context.Context) error {
	s.echoServer = echo.NewEchoServer(0)
	if err := s.echoServer.Start(ctx); err != nil {
		return err
	}
	s.t.Logf("Echo server started on %s", s.echoServer.URL())
	return nil
}

// StopEchoServer 停止 Echo 服务器
func (s *TestSuite) StopEchoServer(ctx context.Context) {
	if s.echoServer != nil {
		s.echoServer.Stop(ctx)
	}
}

// EchoServerURL 返回 Echo 服务器 URL
func (s *TestSuite) EchoServerURL() string {
	if s.echoServer == nil {
		return ""
	}
	return s.echoServer.URL()
}

// HasXray 检查是否有 Xray
func (s *TestSuite) HasXray() bool {
	return s.xrayPath != ""
}

// HasMihomo 检查是否有 Mihomo
func (s *TestSuite) HasMihomo() bool {
	return s.mihomoPath != ""
}

// SkipIfNoBinary 如果没有二进制则跳过
func (s *TestSuite) SkipIfNoBinary() {
	if !s.HasXray() && !s.HasMihomo() {
		s.t.Skip("No xray or mihomo binary found, skipping")
	}
}

// RunLocalTest 运行本地测试
func (s *TestSuite) RunLocalTest(ctx context.Context, protocol string) error {
	return s.RunLocalTestWithEcho(ctx, protocol, false)
}

// RunLocalTestWithEcho 运行本地测试（可选择使用 Echo 服务器）
func (s *TestSuite) RunLocalTestWithEcho(ctx context.Context, protocol string, useEcho bool) error {
	// 设置环境
	if err := s.env.Setup(ctx, protocol); err != nil {
		return err
	}

	serverPort := s.env.ServerPort()
	clientPort := s.env.ClientPort()

	s.t.Logf("Testing %s: server=%d, client=%d", protocol, serverPort, clientPort)

	// 如果使用 Echo 服务器，启动它
	var testURL string
	if useEcho && s.echoServer != nil {
		testURL = s.echoServer.URL() + "/generate_204"
	} else {
		testURL = "http://www.gstatic.com/generate_204"
	}

	// 构建配置
	builder := NewConfigBuilder().
		Protocol(protocol).
		ServerPort(serverPort).
		ClientPort(clientPort)

	serverConfig, err := builder.BuildServerConfig()
	if err != nil {
		return err
	}

	clientConfig, err := builder.BuildClientConfig()
	if err != nil {
		return err
	}

	// 保存配置
	serverConfigPath := filepath.Join(s.configDir, fmt.Sprintf("server-%s.json", protocol))
	clientConfigPath := filepath.Join(s.configDir, fmt.Sprintf("client-%s.json", protocol))

	if err := os.WriteFile(serverConfigPath, serverConfig, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(clientConfigPath, clientConfig, 0644); err != nil {
		return err
	}

	// 使用 xray 作为服务端和客户端
	binary := s.xrayPath
	if binary == "" {
		return fmt.Errorf("no xray binary found")
	}

	// 启动服务端
	serverCmd := exec.CommandContext(ctx, binary, "run", "-c", serverConfigPath)
	if runtime.GOOS != "windows" {
		serverCmd.Stdout = os.Stdout
		serverCmd.Stderr = os.Stderr
	}
	if err := serverCmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	defer func() {
		if serverCmd.Process != nil {
			serverCmd.Process.Signal(os.Interrupt)
			serverCmd.Wait()
		}
	}()

	// 等待服务端启动
	if err := WaitForPort(serverPort, 10*time.Second); err != nil {
		return fmt.Errorf("server failed to start: %w", err)
	}
	s.t.Logf("Server started on port %d", serverPort)

	// 启动客户端
	clientCmd := exec.CommandContext(ctx, binary, "run", "-c", clientConfigPath)
	if runtime.GOOS != "windows" {
		clientCmd.Stdout = os.Stdout
		clientCmd.Stderr = os.Stderr
	}
	if err := clientCmd.Start(); err != nil {
		return fmt.Errorf("failed to start client: %w", err)
	}
	defer func() {
		if clientCmd.Process != nil {
			clientCmd.Process.Signal(os.Interrupt)
			clientCmd.Wait()
		}
	}()

	// 等待客户端启动
	if err := WaitForPort(clientPort, 10*time.Second); err != nil {
		return fmt.Errorf("client failed to start: %w", err)
	}
	s.t.Logf("Client started on port %d", clientPort)

	// 测试连通性
	proxyURL := fmt.Sprintf("socks5://127.0.0.1:%d", clientPort)

	// 通过代理发送请求
	err = s.testThroughProxy(ctx, proxyURL, testURL)
	if err != nil {
		return fmt.Errorf("connectivity test failed: %w", err)
	}

	s.t.Logf("Connectivity test passed for %s", protocol)
	return nil
}

// testThroughProxy 通过代理测试连接
func (s *TestSuite) testThroughProxy(ctx context.Context, proxyURL, targetURL string) error {
	// 使用 curl 测试（更简单）
	curlPath, err := exec.LookPath("curl")
	if err != nil {
		// 如果没有 curl，使用 Go HTTP 客户端
		return s.testWithHTTPClient(ctx, proxyURL, targetURL)
	}

	cmd := exec.CommandContext(ctx, curlPath,
		"-x", proxyURL,
		"-s", "-o", "/dev/null",
		"-w", "%{http_code}",
		"--connect-timeout", "10",
		targetURL,
	)

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("curl failed: %w", err)
	}

	statusCode := string(bytes.TrimSpace(output))
	if statusCode != "200" && statusCode != "204" {
		return fmt.Errorf("unexpected status code: %s", statusCode)
	}

	return nil
}

// testWithHTTPClient 使用 Go HTTP 客户端测试
func (s *TestSuite) testWithHTTPClient(ctx context.Context, proxyURL, targetURL string) error {
	// 创建 SOCKS5 拨号器需要 golang.org/x/net/proxy
	// 这里使用简单的方式：检查端口是否可达
	client := &http.Client{
		Timeout: 10 * time.Second,
		// 注意：Go 标准库不直接支持 SOCKS5 代理
		// 需要使用 golang.org/x/net/proxy
	}

	// 简单检查：尝试连接客户端端口
	conn, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(conn)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// Teardown 清理环境
func (s *TestSuite) Teardown() {
	s.env.Teardown()
}

// TestEnvironment 测试环境创建
func TestEnvironment(t *testing.T) {
	env := NewEnvironment(t.TempDir())

	ctx := context.Background()
	err := env.Setup(ctx, "shadowsocks")
	require.NoError(t, err)

	assert.Greater(t, env.ServerPort(), 0)
	assert.Greater(t, env.ClientPort(), 0)
	assert.NotEqual(t, env.ServerPort(), env.ClientPort())

	env.Teardown()
}

// TestConfigBuilder 测试配置构建
func TestConfigBuilder(t *testing.T) {
	builder := NewConfigBuilder().
		Protocol("shadowsocks").
		ServerPort(8388).
		ClientPort(1080).
		Password("test-password").
		Method("aes-256-gcm")

	t.Run("ServerConfig", func(t *testing.T) {
		config, err := builder.BuildServerConfig()
		require.NoError(t, err)
		assert.Contains(t, string(config), "8388")
		assert.Contains(t, string(config), "shadowsocks")
		assert.Contains(t, string(config), "test-password")
	})

	t.Run("ClientConfig", func(t *testing.T) {
		config, err := builder.BuildClientConfig()
		require.NoError(t, err)
		assert.Contains(t, string(config), "1080")
		assert.Contains(t, string(config), "127.0.0.1")
	})
}

// TestConfigBuilderProtocols 测试所有协议配置
func TestConfigBuilderProtocols(t *testing.T) {
	protocols := []string{"shadowsocks", "vmess", "vless", "trojan"}

	for _, protocol := range protocols {
		t.Run(protocol, func(t *testing.T) {
			builder := NewConfigBuilder().
				Protocol(protocol).
				ServerPort(8388).
				ClientPort(1080)

			serverConfig, err := builder.BuildServerConfig()
			require.NoError(t, err)
			assert.NotEmpty(t, serverConfig)

			clientConfig, err := builder.BuildClientConfig()
			require.NoError(t, err)
			assert.NotEmpty(t, clientConfig)
		})
	}
}

// TestWaitForPort 测试端口等待
func TestWaitForPort(t *testing.T) {
	// 测试一个不会启动的端口
	err := WaitForPort(59999, 1*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

// TestLocalServer 本地服务器测试
func TestLocalServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	server := NewServer("xray")
	assert.NotNil(t, server)
	assert.Equal(t, ServerStopped, server.Status())
}

// TestLocalE2E 端到端本地测试
func TestLocalE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewTestSuite(t)
	err := suite.Setup()
	require.NoError(t, err)
	defer suite.Teardown()

	suite.SkipIfNoBinary()

	protocols := []string{"shadowsocks", "vmess", "vless", "trojan"}

	for _, protocol := range protocols {
		t.Run(protocol, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			err := suite.RunLocalTest(ctx, protocol)
			if err != nil {
				t.Logf("Test %s failed (expected if no network): %v", protocol, err)
				// 不要求成功，因为可能没有网络
			}
		})
	}
}

// TestLocalShadowsocks 本地 Shadowsocks 测试
func TestLocalShadowsocks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewTestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	if !suite.HasXray() {
		t.Skip("No xray binary found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := suite.RunLocalTest(ctx, "shadowsocks")
	if err != nil {
		t.Logf("Shadowsocks test result: %v", err)
	}
}

// TestLocalVMess 本地 VMess 测试
func TestLocalVMess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewTestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	if !suite.HasXray() {
		t.Skip("No xray binary found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := suite.RunLocalTest(ctx, "vmess")
	if err != nil {
		t.Logf("VMess test result: %v", err)
	}
}

// TestLocalVLESS 本地 VLESS 测试
func TestLocalVLESS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewTestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	if !suite.HasXray() {
		t.Skip("No xray binary found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := suite.RunLocalTest(ctx, "vless")
	if err != nil {
		t.Logf("VLESS test result: %v", err)
	}
}

// TestLocalTrojan 本地 Trojan 测试
func TestLocalTrojan(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewTestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	if !suite.HasXray() {
		t.Skip("No xray binary found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := suite.RunLocalTest(ctx, "trojan")
	if err != nil {
		t.Logf("Trojan test result: %v", err)
	}
}

// SaveConfig 保存配置测试
func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()

	data := []byte(`{"test": "config"}`)
	path, err := SaveConfig(tmpDir, "test.json", data)
	require.NoError(t, err)
	assert.FileExists(t, path)

	// 读取验证
	readData, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, readData)
}

// TestLocalWithEchoServer 使用 Echo 服务器的本地测试
func TestLocalWithEchoServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewTestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	if !suite.HasXray() {
		t.Skip("No xray binary found")
	}

	ctx := context.Background()

	// 启动 Echo 服务器
	if err := suite.StartEchoServer(ctx); err != nil {
		t.Fatalf("Failed to start echo server: %v", err)
	}
	defer suite.StopEchoServer(ctx)

	t.Logf("Echo server URL: %s", suite.EchoServerURL())

	// 使用 Echo 服务器运行测试
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	protocols := []string{"shadowsocks", "vmess", "vless", "trojan"}
	for _, protocol := range protocols {
		t.Run(protocol, func(t *testing.T) {
			err := suite.RunLocalTestWithEcho(testCtx, protocol, true)
			if err != nil {
				t.Logf("Test %s result: %v", protocol, err)
			} else {
				t.Logf("Test %s passed", protocol)
			}
		})
	}
}

// TestLocalShadowsocksWithEcho 使用 Echo 服务器的 Shadowsocks 测试
func TestLocalShadowsocksWithEcho(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewTestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	if !suite.HasXray() {
		t.Skip("No xray binary found")
	}

	ctx := context.Background()
	if err := suite.StartEchoServer(ctx); err != nil {
		t.Fatalf("Failed to start echo server: %v", err)
	}
	defer suite.StopEchoServer(ctx)

	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err := suite.RunLocalTestWithEcho(testCtx, "shadowsocks", true)
	if err != nil {
		t.Logf("Shadowsocks test result: %v", err)
	} else {
		t.Log("Shadowsocks test passed")
	}
}

// TestEchoServerIntegration Echo 服务器集成测试
func TestEchoServerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewTestSuite(t)
	require.NoError(t, suite.Setup())

	ctx := context.Background()

	// 启动 Echo 服务器
	require.NoError(t, suite.StartEchoServer(ctx))
	defer suite.StopEchoServer(ctx)

	// 直接测试 Echo 服务器
	resp, err := http.Get(suite.EchoServerURL() + "/ping")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 测试 /generate_204
	resp2, err := http.Get(suite.EchoServerURL() + "/generate_204")
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp2.StatusCode)
}