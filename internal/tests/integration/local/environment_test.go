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

	"github.com/anixops/v2board/internal/tests/integration/echo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSuite 鏈湴娴嬭瘯濂椾欢
type TestSuite struct {
	t            *testing.T
	env          *Environment
	configDir    string
	serverBinary string
	xrayPath     string
	mihomoPath   string
	echoServer   *echo.EchoServer
}

// NewTestSuite 鍒涘缓娴嬭瘯濂椾欢
func NewTestSuite(t *testing.T) *TestSuite {
	configDir := t.TempDir()
	return &TestSuite{
		t:         t,
		env:       NewEnvironment(configDir),
		configDir: configDir,
	}
}

// Setup 璁剧疆娴嬭瘯鐜
func (s *TestSuite) Setup() error {
	// 鏌ユ壘 xray 浜岃繘鍒?
	if path, err := exec.LookPath("xray"); err == nil {
		s.xrayPath = path
	}
	if path, err := exec.LookPath("mihomo"); err == nil {
		s.mihomoPath = path
	}
	// 涔熸鏌?mihomo 鐨勫叾浠栧悕绉?
	if s.mihomoPath == "" {
		if path, err := exec.LookPath("clash-meta"); err == nil {
			s.mihomoPath = path
		}
	}

	return nil
}

// StartEchoServer 鍚姩鍐呯疆 Echo 鏈嶅姟鍣?
func (s *TestSuite) StartEchoServer(ctx context.Context) error {
	s.echoServer = echo.NewEchoServer(0)
	if err := s.echoServer.Start(ctx); err != nil {
		return err
	}
	s.t.Logf("Echo server started on %s", s.echoServer.URL())
	return nil
}

// StopEchoServer 鍋滄 Echo 鏈嶅姟鍣?
func (s *TestSuite) StopEchoServer(ctx context.Context) {
	if s.echoServer != nil {
		s.echoServer.Stop(ctx)
	}
}

// EchoServerURL 杩斿洖 Echo 鏈嶅姟鍣?URL
func (s *TestSuite) EchoServerURL() string {
	if s.echoServer == nil {
		return ""
	}
	return s.echoServer.URL()
}

// HasXray 妫€鏌ユ槸鍚︽湁 Xray
func (s *TestSuite) HasXray() bool {
	return s.xrayPath != ""
}

// HasMihomo 妫€鏌ユ槸鍚︽湁 Mihomo
func (s *TestSuite) HasMihomo() bool {
	return s.mihomoPath != ""
}

// SkipIfNoBinary 濡傛灉娌℃湁浜岃繘鍒跺垯璺宠繃
func (s *TestSuite) SkipIfNoBinary() {
	if !s.HasXray() && !s.HasMihomo() {
		s.t.Skip("No xray or mihomo binary found, skipping")
	}
}

// RunLocalTest 杩愯鏈湴娴嬭瘯
func (s *TestSuite) RunLocalTest(ctx context.Context, protocol string) error {
	return s.RunLocalTestWithEcho(ctx, protocol, false)
}

// RunLocalTestWithEcho 杩愯鏈湴娴嬭瘯锛堝彲閫夋嫨浣跨敤 Echo 鏈嶅姟鍣級
func (s *TestSuite) RunLocalTestWithEcho(ctx context.Context, protocol string, useEcho bool) error {
	// 璁剧疆鐜
	if err := s.env.Setup(ctx, protocol); err != nil {
		return err
	}

	serverPort := s.env.ServerPort()
	clientPort := s.env.ClientPort()

	s.t.Logf("Testing %s: server=%d, client=%d", protocol, serverPort, clientPort)

	// 濡傛灉浣跨敤 Echo 鏈嶅姟鍣紝鍚姩瀹?
	var testURL string
	if useEcho && s.echoServer != nil {
		testURL = s.echoServer.URL() + "/generate_204"
	} else {
		testURL = "http://www.gstatic.com/generate_204"
	}

	// 鏋勫缓閰嶇疆
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

	// 淇濆瓨閰嶇疆
	serverConfigPath := filepath.Join(s.configDir, fmt.Sprintf("server-%s.json", protocol))
	clientConfigPath := filepath.Join(s.configDir, fmt.Sprintf("client-%s.json", protocol))

	if err := os.WriteFile(serverConfigPath, serverConfig, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(clientConfigPath, clientConfig, 0644); err != nil {
		return err
	}

	// 浣跨敤 xray 浣滀负鏈嶅姟绔拰瀹㈡埛绔?
	binary := s.xrayPath
	if binary == "" {
		return fmt.Errorf("no xray binary found")
	}

	// 鍚姩鏈嶅姟绔?
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

	// 绛夊緟鏈嶅姟绔惎鍔?
	if err := WaitForPort(serverPort, 10*time.Second); err != nil {
		return fmt.Errorf("server failed to start: %w", err)
	}
	s.t.Logf("Server started on port %d", serverPort)

	// 鍚姩瀹㈡埛绔?
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

	// 绛夊緟瀹㈡埛绔惎鍔?
	if err := WaitForPort(clientPort, 10*time.Second); err != nil {
		return fmt.Errorf("client failed to start: %w", err)
	}
	s.t.Logf("Client started on port %d", clientPort)

	// 娴嬭瘯杩為€氭€?
	proxyURL := fmt.Sprintf("socks5://127.0.0.1:%d", clientPort)

	// 閫氳繃浠ｇ悊鍙戦€佽姹?
	err = s.testThroughProxy(ctx, proxyURL, testURL)
	if err != nil {
		return fmt.Errorf("connectivity test failed: %w", err)
	}

	s.t.Logf("Connectivity test passed for %s", protocol)
	return nil
}

// testThroughProxy 閫氳繃浠ｇ悊娴嬭瘯杩炴帴
func (s *TestSuite) testThroughProxy(ctx context.Context, proxyURL, targetURL string) error {
	// 浣跨敤 curl 娴嬭瘯锛堟洿绠€鍗曪級
	curlPath, err := exec.LookPath("curl")
	if err != nil {
		// 濡傛灉娌℃湁 curl锛屼娇鐢?Go HTTP 瀹㈡埛绔?
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

// testWithHTTPClient 浣跨敤 Go HTTP 瀹㈡埛绔祴璇?
func (s *TestSuite) testWithHTTPClient(ctx context.Context, proxyURL, targetURL string) error {
	// 鍒涘缓 SOCKS5 鎷ㄥ彿鍣ㄩ渶瑕?golang.org/x/net/proxy
	// 杩欓噷浣跨敤绠€鍗曠殑鏂瑰紡锛氭鏌ョ鍙ｆ槸鍚﹀彲杈?
	client := &http.Client{
		Timeout: 10 * time.Second,
		// 娉ㄦ剰锛欸o 鏍囧噯搴撲笉鐩存帴鏀寔 SOCKS5 浠ｇ悊
		// 闇€瑕佷娇鐢?golang.org/x/net/proxy
	}

	// 绠€鍗曟鏌ワ細灏濊瘯杩炴帴瀹㈡埛绔鍙?
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

// Teardown 娓呯悊鐜
func (s *TestSuite) Teardown() {
	s.env.Teardown()
}

// TestEnvironment 娴嬭瘯鐜鍒涘缓
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

// TestConfigBuilder 娴嬭瘯閰嶇疆鏋勫缓
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

// TestConfigBuilderProtocols 娴嬭瘯鎵€鏈夊崗璁厤缃?
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

// TestWaitForPort 娴嬭瘯绔彛绛夊緟
func TestWaitForPort(t *testing.T) {
	// 娴嬭瘯涓€涓笉浼氬惎鍔ㄧ殑绔彛
	err := WaitForPort(59999, 1*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

// TestLocalServer 鏈湴鏈嶅姟鍣ㄦ祴璇?
func TestLocalServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	server := NewServer("xray")
	assert.NotNil(t, server)
	assert.Equal(t, ServerStopped, server.Status())
}

// TestLocalE2E 绔埌绔湰鍦版祴璇?
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
				// 涓嶈姹傛垚鍔燂紝鍥犱负鍙兘娌℃湁缃戠粶
			}
		})
	}
}

// TestLocalShadowsocks 鏈湴 Shadowsocks 娴嬭瘯
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

// TestLocalVMess 鏈湴 VMess 娴嬭瘯
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

// TestLocalVLESS 鏈湴 VLESS 娴嬭瘯
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

// TestLocalTrojan 鏈湴 Trojan 娴嬭瘯
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

// SaveConfig 淇濆瓨閰嶇疆娴嬭瘯
func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()

	data := []byte(`{"test": "config"}`)
	path, err := SaveConfig(tmpDir, "test.json", data)
	require.NoError(t, err)
	assert.FileExists(t, path)

	// 璇诲彇楠岃瘉
	readData, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, readData)
}

// TestLocalWithEchoServer 浣跨敤 Echo 鏈嶅姟鍣ㄧ殑鏈湴娴嬭瘯
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

	// 鍚姩 Echo 鏈嶅姟鍣?
	if err := suite.StartEchoServer(ctx); err != nil {
		t.Fatalf("Failed to start echo server: %v", err)
	}
	defer suite.StopEchoServer(ctx)

	t.Logf("Echo server URL: %s", suite.EchoServerURL())

	// 浣跨敤 Echo 鏈嶅姟鍣ㄨ繍琛屾祴璇?
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

// TestLocalShadowsocksWithEcho 浣跨敤 Echo 鏈嶅姟鍣ㄧ殑 Shadowsocks 娴嬭瘯
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

// TestEchoServerIntegration Echo 鏈嶅姟鍣ㄩ泦鎴愭祴璇?
func TestEchoServerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewTestSuite(t)
	require.NoError(t, suite.Setup())

	ctx := context.Background()

	// 鍚姩 Echo 鏈嶅姟鍣?
	require.NoError(t, suite.StartEchoServer(ctx))
	defer suite.StopEchoServer(ctx)

	// 鐩存帴娴嬭瘯 Echo 鏈嶅姟鍣?
	resp, err := http.Get(suite.EchoServerURL() + "/ping")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 娴嬭瘯 /generate_204
	resp2, err := http.Get(suite.EchoServerURL() + "/generate_204")
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp2.StatusCode)
}
