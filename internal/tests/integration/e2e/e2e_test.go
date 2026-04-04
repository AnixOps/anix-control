package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/tests/integration/echo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/proxy"
)

// E2ETestSuite 绔埌绔祴璇曞浠?
type E2ETestSuite struct {
	t          *testing.T
	configDir  string
	xrayPath   string
	echoServer *echo.EchoServer
	processes  []*exec.Cmd
	mu         sync.Mutex
}

// NewE2ETestSuite 鍒涘缓绔埌绔祴璇曞浠?
func NewE2ETestSuite(t *testing.T) *E2ETestSuite {
	configDir := t.TempDir()
	return &E2ETestSuite{
		t:         t,
		configDir: configDir,
		processes: make([]*exec.Cmd, 0),
	}
}

// Setup 璁剧疆娴嬭瘯鐜
func (s *E2ETestSuite) Setup() error {
	// 鏌ユ壘 xray 浜岃繘鍒?
	if path, err := exec.LookPath("xray"); err == nil {
		s.xrayPath = path
		s.t.Logf("Found xray at: %s", path)
	} else {
		return fmt.Errorf("xray binary not found in PATH")
	}

	return nil
}

// StartEchoServer 鍚姩 Echo 鏈嶅姟鍣?
func (s *E2ETestSuite) StartEchoServer(ctx context.Context) (int, error) {
	s.echoServer = echo.NewEchoServer(0)
	if err := s.echoServer.Start(ctx); err != nil {
		return 0, err
	}
	port := s.echoServer.Port()
	s.t.Logf("Echo server started on port %d", port)
	return port, nil
}

// StopEchoServer 鍋滄 Echo 鏈嶅姟鍣?
func (s *E2ETestSuite) StopEchoServer(ctx context.Context) {
	if s.echoServer != nil {
		s.echoServer.Stop(ctx)
	}
}

// GetFreePort 鑾峰彇绌洪棽绔彛
func (s *E2ETestSuite) GetFreePort() (int, error) {
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

// WaitForPort 绛夊緟绔彛鍙敤
func (s *E2ETestSuite) WaitForPort(port int, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for port %d", port)
}

// RunFullTest 杩愯瀹屾暣娴嬭瘯
// proxyPort: 瀹㈡埛绔唬鐞嗙鍙?
// echoPort: Echo 鏈嶅姟鍣ㄧ鍙?
// protocol: 娴嬭瘯鍗忚
func (s *E2ETestSuite) RunFullTest(ctx context.Context, proxyPort, echoPort int, protocol string) (*TestResult, error) {
	result := &TestResult{
		Protocol:  protocol,
		ProxyPort: proxyPort,
		EchoPort:  echoPort,
		StartTime: time.Now(),
	}

	// 1. 鐢熸垚鏈嶅姟绔厤缃?
	serverPort, err := s.GetFreePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get server port: %w", err)
	}
	result.ServerPort = serverPort

	serverConfig := s.generateServerConfig(protocol, serverPort)
	serverConfigPath := filepath.Join(s.configDir, fmt.Sprintf("server-%s.json", protocol))
	if err := os.WriteFile(serverConfigPath, []byte(serverConfig), 0644); err != nil {
		return nil, err
	}

	// 2. 鐢熸垚瀹㈡埛绔厤缃?
	clientConfig := s.generateClientConfig(protocol, serverPort, proxyPort)
	clientConfigPath := filepath.Join(s.configDir, fmt.Sprintf("client-%s.json", protocol))
	if err := os.WriteFile(clientConfigPath, []byte(clientConfig), 0644); err != nil {
		return nil, err
	}

	// 3. 鍚姩鏈嶅姟绔?
	serverCmd := exec.CommandContext(ctx, s.xrayPath, "run", "-c", serverConfigPath)
	if runtime.GOOS != "windows" {
		serverCmd.Stdout = os.Stdout
		serverCmd.Stderr = os.Stderr
	}
	if err := serverCmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start server: %w", err)
	}
	s.addProcess(serverCmd)

	// 绛夊緟鏈嶅姟绔惎鍔?
	if err := s.WaitForPort(serverPort, 10*time.Second); err != nil {
		return nil, fmt.Errorf("server failed to start: %w", err)
	}
	s.t.Logf("[%s] Server started on port %d", protocol, serverPort)

	// 4. 鍚姩瀹㈡埛绔?
	clientCmd := exec.CommandContext(ctx, s.xrayPath, "run", "-c", clientConfigPath)
	if runtime.GOOS != "windows" {
		clientCmd.Stdout = os.Stdout
		clientCmd.Stderr = os.Stderr
	}
	if err := clientCmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start client: %w", err)
	}
	s.addProcess(clientCmd)

	// 绛夊緟瀹㈡埛绔惎鍔?
	if err := s.WaitForPort(proxyPort, 10*time.Second); err != nil {
		return nil, fmt.Errorf("client failed to start: %w", err)
	}
	s.t.Logf("[%s] Client started on port %d", protocol, proxyPort)

	// 5. 娴嬭瘯杩為€氭€?
	testStart := time.Now()
	err = s.testConnectivity(ctx, proxyPort, echoPort)
	result.Latency = time.Since(testStart)

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}

// testConnectivity 娴嬭瘯杩為€氭€?
func (s *E2ETestSuite) testConnectivity(ctx context.Context, proxyPort, echoPort int) error {
	// 鍒涘缓 SOCKS5 鎷ㄥ彿鍣?
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), nil, proxy.Direct)
	if err != nil {
		return fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	// 鍒涘缓 HTTP 瀹㈡埛绔?
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	// 閫氳繃浠ｇ悊璁块棶 Echo 鏈嶅姟鍣?
	targetURL := fmt.Sprintf("http://127.0.0.1:%d/ping", echoPort)

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if string(body) != "pong" {
		return fmt.Errorf("unexpected response: %s", string(body))
	}

	s.t.Logf("鉁?Successfully connected through proxy to Echo server")
	return nil
}

// TestResult 娴嬭瘯缁撴灉
type TestResult struct {
	Protocol   string
	ServerPort int
	ProxyPort  int
	EchoPort   int
	Success    bool
	Error      string
	Latency    time.Duration
	Duration   time.Duration
	StartTime  time.Time
	EndTime    time.Time
}

// generateServerConfig 鐢熸垚鏈嶅姟绔厤缃?
func (s *E2ETestSuite) generateServerConfig(protocol string, port int) string {
	switch protocol {
	case "shadowsocks":
		return fmt.Sprintf(`{
	"inbounds": [{
		"port": %d,
		"protocol": "shadowsocks",
		"settings": {
			"method": "aes-256-gcm",
			"password": "test-password-123",
			"network": "tcp,udp"
		}
	}],
	"outbounds": [{
		"protocol": "freedom"
	}]
}`, port)

	case "vmess":
		return fmt.Sprintf(`{
	"inbounds": [{
		"port": %d,
		"protocol": "vmess",
		"settings": {
			"clients": [{
				"id": "12345678-1234-1234-1234-123456789abc",
				"alterId": 0
			}]
		}
	}],
	"outbounds": [{
		"protocol": "freedom"
	}]
}`, port)

	case "vless":
		return fmt.Sprintf(`{
	"inbounds": [{
		"port": %d,
		"protocol": "vless",
		"settings": {
			"clients": [{
				"id": "12345678-1234-1234-1234-123456789abc",
				"flow": ""
			}],
			"decryption": "none"
		}
	}],
	"outbounds": [{
		"protocol": "freedom"
	}]
}`, port)

	case "trojan":
		return fmt.Sprintf(`{
	"inbounds": [{
		"port": %d,
		"protocol": "trojan",
		"settings": {
			"clients": [{
				"password": "test-password-123"
			}]
		}
	}],
	"outbounds": [{
		"protocol": "freedom"
	}]
}`, port)

	default:
		return ""
	}
}

// generateClientConfig 鐢熸垚瀹㈡埛绔厤缃?
func (s *E2ETestSuite) generateClientConfig(protocol string, serverPort, proxyPort int) string {
	switch protocol {
	case "shadowsocks":
		return fmt.Sprintf(`{
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
				"method": "aes-256-gcm",
				"password": "test-password-123"
			}]
		}
	}]
}`, proxyPort, serverPort)

	case "vmess":
		return fmt.Sprintf(`{
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
					"id": "12345678-1234-1234-1234-123456789abc",
					"alterId": 0
				}]
			}]
		}
	}]
}`, proxyPort, serverPort)

	case "vless":
		return fmt.Sprintf(`{
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
					"id": "12345678-1234-1234-1234-123456789abc",
					"encryption": "none"
				}]
			}]
		}
	}]
}`, proxyPort, serverPort)

	case "trojan":
		return fmt.Sprintf(`{
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
				"password": "test-password-123"
			}]
		}
	}]
}`, proxyPort, serverPort)

	default:
		return ""
	}
}

// addProcess 娣诲姞杩涚▼
func (s *E2ETestSuite) addProcess(cmd *exec.Cmd) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processes = append(s.processes, cmd)
}

// StopAll 鍋滄鎵€鏈夎繘绋?
func (s *E2ETestSuite) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := len(s.processes) - 1; i >= 0; i-- {
		cmd := s.processes[i]
		if cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt)
			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				cmd.Process.Kill()
			}
		}
	}
	s.processes = nil
}

// Teardown 娓呯悊鐜
func (s *E2ETestSuite) Teardown() {
	s.StopAll()
	s.StopEchoServer(context.Background())
}

// TestResultReport 娴嬭瘯鎶ュ憡
type TestResultReport struct {
	Timestamp time.Time
	Total     int
	Passed    int
	Failed    int
	Results   []TestResult
}

// Print 鎵撳嵃鎶ュ憡
func (r *TestResultReport) Print() {
	fmt.Println("\n========== E2E Test Report ==========")
	fmt.Printf("Time: %s\n", r.Timestamp.Format(time.RFC3339))
	fmt.Printf("Total: %d, Passed: %d, Failed: %d\n", r.Total, r.Passed, r.Failed)
	fmt.Printf("Pass Rate: %.1f%%\n", float64(r.Passed)/float64(r.Total)*100)
	fmt.Println("\nDetails:")

	for _, result := range r.Results {
		status := "鉁?PASS"
		if !result.Success {
			status = "鉂?FAIL"
		}
		fmt.Printf("  %s [%s] Server:%d 鈫?Proxy:%d 鈫?Echo:%d (latency: %v)\n",
			status, result.Protocol, result.ServerPort, result.ProxyPort, result.EchoPort, result.Latency)
		if result.Error != "" {
			fmt.Printf("       Error: %s\n", result.Error)
		}
	}
	fmt.Println("======================================")
}

// ========== 娴嬭瘯鍑芥暟 ==========

func TestE2ESetup(t *testing.T) {
	suite := NewE2ETestSuite(t)
	err := suite.Setup()
	if err != nil {
		t.Skipf("Setup failed (expected if xray not installed): %v", err)
	}
	assert.NotEmpty(t, suite.xrayPath)
}

func TestE2EGetFreePort(t *testing.T) {
	suite := NewE2ETestSuite(t)

	port1, err := suite.GetFreePort()
	require.NoError(t, err)
	assert.Greater(t, port1, 0)

	port2, err := suite.GetFreePort()
	require.NoError(t, err)
	assert.Greater(t, port2, 0)
}

func TestE2EStartEchoServer(t *testing.T) {
	suite := NewE2ETestSuite(t)
	ctx := context.Background()

	port, err := suite.StartEchoServer(ctx)
	require.NoError(t, err)
	assert.Greater(t, port, 0)

	// 娴嬭瘯 Echo 鏈嶅姟鍣?
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", port))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	suite.StopEchoServer(ctx)
}

func TestE2EShadowsocksFull(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewE2ETestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	ctx := context.Background()

	// 鍚姩 Echo 鏈嶅姟鍣?
	echoPort, err := suite.StartEchoServer(ctx)
	require.NoError(t, err)

	// 鑾峰彇浠ｇ悊绔彛
	proxyPort, err := suite.GetFreePort()
	require.NoError(t, err)

	// 杩愯瀹屾暣娴嬭瘯
	result, err := suite.RunFullTest(ctx, proxyPort, echoPort, "shadowsocks")
	require.NoError(t, err)

	t.Logf("Result: Success=%v, Latency=%v, Error=%s", result.Success, result.Latency, result.Error)

	if !result.Success {
		t.Errorf("Shadowsocks test failed: %s", result.Error)
	}
}

func TestE2EVMessFull(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewE2ETestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	ctx := context.Background()

	echoPort, err := suite.StartEchoServer(ctx)
	require.NoError(t, err)

	proxyPort, err := suite.GetFreePort()
	require.NoError(t, err)

	result, err := suite.RunFullTest(ctx, proxyPort, echoPort, "vmess")
	require.NoError(t, err)

	t.Logf("Result: Success=%v, Latency=%v", result.Success, result.Latency)

	if !result.Success {
		t.Errorf("VMess test failed: %s", result.Error)
	}
}

func TestE2EVLESSFull(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewE2ETestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	ctx := context.Background()

	echoPort, err := suite.StartEchoServer(ctx)
	require.NoError(t, err)

	proxyPort, err := suite.GetFreePort()
	require.NoError(t, err)

	result, err := suite.RunFullTest(ctx, proxyPort, echoPort, "vless")
	require.NoError(t, err)

	t.Logf("Result: Success=%v, Latency=%v", result.Success, result.Latency)

	if !result.Success {
		t.Errorf("VLESS test failed: %s", result.Error)
	}
}

func TestE2ETrojanFull(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewE2ETestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	ctx := context.Background()

	echoPort, err := suite.StartEchoServer(ctx)
	require.NoError(t, err)

	proxyPort, err := suite.GetFreePort()
	require.NoError(t, err)

	result, err := suite.RunFullTest(ctx, proxyPort, echoPort, "trojan")
	require.NoError(t, err)

	t.Logf("Result: Success=%v, Latency=%v", result.Success, result.Latency)

	if !result.Success {
		t.Errorf("Trojan test failed: %s", result.Error)
	}
}

func TestE2EAllProtocols(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewE2ETestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	ctx := context.Background()

	echoPort, err := suite.StartEchoServer(ctx)
	require.NoError(t, err)

	protocols := []string{"shadowsocks", "vmess", "vless", "trojan"}

	report := &TestResultReport{
		Timestamp: time.Now(),
		Results:   make([]TestResult, 0),
	}

	for _, protocol := range protocols {
		t.Run(protocol, func(t *testing.T) {
			proxyPort, err := suite.GetFreePort()
			require.NoError(t, err)

			result, err := suite.RunFullTest(ctx, proxyPort, echoPort, protocol)
			if err != nil {
				t.Logf("Test setup failed: %v", err)
				result = &TestResult{
					Protocol: protocol,
					Success:  false,
					Error:    err.Error(),
				}
			}

			report.Results = append(report.Results, *result)
			report.Total++
			if result.Success {
				report.Passed++
				t.Logf("鉁?%s passed (latency: %v)", protocol, result.Latency)
			} else {
				report.Failed++
				t.Logf("鉂?%s failed: %s", protocol, result.Error)
			}

			// 娓呯悊杩涚▼锛屼负涓嬩竴涓崗璁祴璇曞仛鍑嗗
			suite.StopAll()
			time.Sleep(500 * time.Millisecond)
		})
	}

	report.Print()

	// 鑷冲皯瑕佹湁涓€浜涙祴璇曢€氳繃
	if report.Passed == 0 {
		t.Error("All protocol tests failed")
	}
}

func TestE2EConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewE2ETestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	ctx := context.Background()

	echoPort, err := suite.StartEchoServer(ctx)
	require.NoError(t, err)

	proxyPort, err := suite.GetFreePort()
	require.NoError(t, err)

	result, err := suite.RunFullTest(ctx, proxyPort, echoPort, "shadowsocks")
	require.NoError(t, err)
	require.True(t, result.Success, "Initial test should pass")

	// 骞跺彂娴嬭瘯
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 鍒涘缓 SOCKS5 鎷ㄥ彿鍣?
			dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), nil, proxy.Direct)
			if err != nil {
				errors <- err
				return
			}

			transport := &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				},
			}

			client := &http.Client{
				Transport: transport,
				Timeout:   10 * time.Second,
			}

			targetURL := fmt.Sprintf("http://127.0.0.1:%d/echo?request=%d", echoPort, id)
			resp, err := client.Get(targetURL)
			if err != nil {
				errors <- err
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errors <- fmt.Errorf("unexpected status: %d", resp.StatusCode)
				return
			}

			t.Logf("Concurrent request %d completed", id)
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent request error: %v", err)
	}
}

func TestE2ELargeData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	suite := NewE2ETestSuite(t)
	if err := suite.Setup(); err != nil {
		t.Skipf("Setup failed: %v", err)
	}
	defer suite.Teardown()

	ctx := context.Background()

	echoPort, err := suite.StartEchoServer(ctx)
	require.NoError(t, err)

	proxyPort, err := suite.GetFreePort()
	require.NoError(t, err)

	result, err := suite.RunFullTest(ctx, proxyPort, echoPort, "shadowsocks")
	require.NoError(t, err)
	require.True(t, result.Success)

	// 鍙戦€佸ぇ鏁版嵁
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), nil, proxy.Direct)
	require.NoError(t, err)

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	// 鍙戦€?1MB 鏁版嵁
	largeData := bytes.Repeat([]byte("X"), 1024*1024)
	targetURL := fmt.Sprintf("http://127.0.0.1:%d/echo", echoPort)

	resp, err := client.Post(targetURL, "application/octet-stream", bytes.NewReader(largeData))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// 楠岃瘉鍝嶅簲鍖呭惈鏁版嵁淇℃伅
	assert.Contains(t, string(body), "Content-Length")
	t.Logf("Large data test completed, response length: %d", len(body))
}

func TestE2EReportJSON(t *testing.T) {
	report := &TestResultReport{
		Timestamp: time.Now(),
		Total:     4,
		Passed:    3,
		Failed:    1,
		Results: []TestResult{
			{Protocol: "shadowsocks", Success: true, Latency: 10 * time.Millisecond},
			{Protocol: "vmess", Success: true, Latency: 15 * time.Millisecond},
			{Protocol: "vless", Success: true, Latency: 12 * time.Millisecond},
			{Protocol: "trojan", Success: false, Error: "connection timeout"},
		},
	}

	data, err := json.MarshalIndent(report, "", "  ")
	require.NoError(t, err)

	assert.Contains(t, string(data), "shadowsocks")
	assert.Contains(t, string(data), "vmess")
	assert.Contains(t, string(data), "\"Passed\": 3")
}
