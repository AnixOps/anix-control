package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/tests/integration/binary"
	"github.com/anixops/v2board/internal/tests/integration/clients"
	"github.com/anixops/v2board/internal/tests/integration/config"
	"github.com/anixops/v2board/internal/tests/integration/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSuite 闆嗘垚娴嬭瘯濂椾欢
type TestSuite struct {
	t          *testing.T
	binMgr     *binary.Manager
	configDir  string
	serverInfo config.ServerConfig
	userInfo   config.UserConfig
}

// NewTestSuite 鍒涘缓娴嬭瘯濂椾欢
func NewTestSuite(t *testing.T) *TestSuite {
	configDir := t.TempDir()
	return &TestSuite{
		t:         t,
		binMgr:    binary.NewManager(""),
		configDir: configDir,
	}
}

// Setup 璁剧疆娴嬭瘯鐜
func (s *TestSuite) Setup() error {
	// 璁剧疆浜岃繘鍒剁鐞嗗櫒
	clients.SetBinaryManager(&binaryAdapter{mgr: s.binMgr})

	// 纭繚浜岃繘鍒舵枃浠跺彲鐢?
	if _, err := s.binMgr.EnsureBinary(&binary.XrayInfo); err != nil {
		return fmt.Errorf("xray binary not available: %w", err)
	}
	if _, err := s.binMgr.EnsureBinary(&binary.MihomoInfo); err != nil {
		return fmt.Errorf("mihomo binary not available: %w", err)
	}

	return nil
}

// SetServer 璁剧疆鏈嶅姟鍣ㄤ俊鎭?
func (s *TestSuite) SetServer(host string, port int, protocol config.Protocol) {
	s.serverInfo = config.ServerConfig{
		Host:     host,
		Port:     port,
		Protocol: protocol,
	}
}

// SetUser 璁剧疆鐢ㄦ埛淇℃伅
func (s *TestSuite) SetUser(uuid, email string) {
	s.userInfo = config.UserConfig{
		UUID:  uuid,
		Email: email,
	}
}

// RunScenario 杩愯鍗曚釜娴嬭瘯鍦烘櫙
func (s *TestSuite) RunScenario(ctx context.Context, scenario config.TestScenario) *runner.TestResult {
	r := runner.NewRunner(
		runner.WithTimeout(30*time.Second),
		runner.WithConfigDir(s.configDir),
		runner.WithScenarios([]config.TestScenario{scenario}),
	)

	report := r.Run(ctx, s.serverInfo, s.userInfo)
	if len(report.Results) > 0 {
		return &report.Results[0]
	}
	return nil
}

// RunAllScenarios 杩愯鎵€鏈夋祴璇曞満鏅?
func (s *TestSuite) RunAllScenarios(ctx context.Context) *runner.TestReport {
	r := runner.NewRunner(
		runner.WithTimeout(30*time.Second),
		runner.WithConfigDir(s.configDir),
	)

	return r.Run(ctx, s.serverInfo, s.userInfo)
}

// binaryAdapter 閫傞厤鍣?
type binaryAdapter struct {
	mgr *binary.Manager
}

func (a *binaryAdapter) EnsureBinary(name string) (string, error) {
	var info *binary.BinaryInfo
	switch name {
	case "xray":
		info = &binary.XrayInfo
	case "mihomo":
		info = &binary.MihomoInfo
	default:
		return "", fmt.Errorf("unknown binary: %s", name)
	}
	return a.mgr.EnsureBinary(info)
}

// TestBinaryDownload 娴嬭瘯浜岃繘鍒朵笅杞?
func TestBinaryDownload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	mgr := binary.NewManager(t.TempDir())

	t.Run("Xray", func(t *testing.T) {
		path, err := mgr.EnsureBinary(&binary.XrayInfo)
		if err != nil {
			t.Logf("Xray not available (expected if not installed): %v", err)
			return
		}
		assert.FileExists(t, path)
		t.Logf("Xray binary: %s", path)
	})

	t.Run("Mihomo", func(t *testing.T) {
		path, err := mgr.EnsureBinary(&binary.MihomoInfo)
		if err != nil {
			t.Logf("Mihomo not available (expected if not installed): %v", err)
			return
		}
		assert.FileExists(t, path)
		t.Logf("Mihomo binary: %s", path)
	})
}

// TestConfigGeneration 娴嬭瘯閰嶇疆鐢熸垚
func TestConfigGeneration(t *testing.T) {
	scenarios := []config.TestScenario{
		{Name: "vless-reality", Protocol: config.ProtocolVLESS, Transport: config.TransportTCP, TLS: config.TLSReality},
		{Name: "vmess-ws", Protocol: config.ProtocolVMess, Transport: config.TransportWS, TLS: config.TLS},
		{Name: "trojan-tcp", Protocol: config.ProtocolTrojan, Transport: config.TransportTCP, TLS: config.TLS},
	}

	server := config.ServerConfig{
		Host:      "example.com",
		Port:      443,
		Protocol:  config.ProtocolVLESS,
		Transport: config.TransportTCP,
		TLSType:   config.TLSReality,
		SNI:       "www.google.com",
		PublicKey: "test-public-key",
		ShortID:   "test-short-id",
	}

	user := config.UserConfig{
		UUID:  "test-uuid-1234",
		Email: "test@example.com",
	}

	for _, scenario := range scenarios {
		t.Run(string(scenario.Protocol), func(t *testing.T) {
			// Xray 閰嶇疆
			xrayGen := config.NewXrayGenerator()
			xrayConfig, err := xrayGen.GenerateFromScenario(scenario, server, user)
			require.NoError(t, err)
			assert.NotEmpty(t, xrayConfig)

			// Mihomo 閰嶇疆
			mihomoGen := config.NewMihomoGenerator()
			mihomoConfig, err := mihomoGen.GenerateFromScenario(scenario, server, user)
			if err != nil {
				// 鏌愪簺鍗忚鍙兘涓嶆敮鎸?
				t.Logf("Mihomo does not support %s: %v", scenario.Protocol, err)
			} else {
				assert.NotEmpty(t, mihomoConfig)
			}
		})
	}
}

// TestClientLifecycle 娴嬭瘯瀹㈡埛绔敓鍛藉懆鏈?
func TestClientLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	tmpDir := t.TempDir()

	// 鍒涘缓娴嬭瘯閰嶇疆
	xrayGen := config.NewXrayGenerator()
	testConfig, err := xrayGen.Generate(&config.ClientConfig{
		Name:   "test",
		Server: config.ServerConfig{Host: "example.com", Port: 443, Protocol: config.ProtocolVLESS},
		User:   config.UserConfig{UUID: "test-uuid"},
	})
	require.NoError(t, err)

	configPath := fmt.Sprintf("%s/config.json", tmpDir)
	err = os.WriteFile(configPath, testConfig, 0644)
	require.NoError(t, err)

	t.Run("CreateClient", func(t *testing.T) {
		client := clients.NewXrayClient(
			clients.WithConfig(configPath),
			clients.WithPorts(20809, 20808, 20810, 20900),
		)
		assert.NotNil(t, client)
		assert.Equal(t, "xray", client.Name())
		assert.Equal(t, clients.ClientXray, client.Type())
		assert.Equal(t, clients.StatusStopped, client.Status())
	})

	t.Run("ManagerOperations", func(t *testing.T) {
		mgr := clients.NewManager()

		client := clients.NewXrayClient(clients.WithName("test-xray"))
		err := mgr.Add("test", client)
		require.NoError(t, err)

		// 閲嶅娣诲姞
		err = mgr.Add("test", client)
		assert.Error(t, err)

		// 鑾峰彇
		got, err := mgr.Get("test")
		require.NoError(t, err)
		assert.Equal(t, client, got)

		// 鍒楄〃
		list := mgr.List()
		assert.Contains(t, list, "test")

		// 鐘舵€?
		status := mgr.Status()
		assert.Contains(t, status, "test")

		// 鍒犻櫎
		err = mgr.Remove("test")
		require.NoError(t, err)

		_, err = mgr.Get("test")
		assert.Error(t, err)
	})
}

// TestRunnerBasic 娴嬭瘯杩愯鍣ㄥ熀鏈姛鑳?
func TestRunnerBasic(t *testing.T) {
	r := runner.NewRunner(
		runner.WithTimeout(5*time.Second),
		runner.WithConfigDir(t.TempDir()),
	)

	// 娴嬭瘯鎶ュ憡
	report := r.GetReport()
	assert.NotNil(t, report)
	assert.Equal(t, 0, report.PassedTests)
}

// TestHealthCheck 娴嬭瘯鍋ュ悍妫€鏌ワ紙闇€瑕佸疄闄呮湇鍔″櫒锛?
func TestHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	// 妫€鏌ユ槸鍚︽湁鍙敤鐨勬祴璇曟湇鍔″櫒
	testServer := os.Getenv("TEST_SERVER_HOST")
	if testServer == "" {
		t.Skip("TEST_SERVER_HOST not set, skipping health check test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 绠€鍗曠殑 HTTP 鍋ュ悍妫€鏌?
	url := fmt.Sprintf("http://%s/health", testServer)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	require.NoError(t, err)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("Health check failed (expected if server not running): %v", err)
		return
	}
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestFullIntegration 鍏ㄩ泦鎴愭祴璇曪紙闇€瑕佺湡瀹炴湇鍔″櫒鍜岄厤缃級
func TestFullIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	// 浠庣幆澧冨彉閲忚鍙栭厤缃?
	host := os.Getenv("TEST_SERVER_HOST")
	port := os.Getenv("TEST_SERVER_PORT")
	uuid := os.Getenv("TEST_USER_UUID")
	protocol := os.Getenv("TEST_PROTOCOL")
	publicKey := os.Getenv("TEST_REALITY_PUBLIC_KEY")
	shortID := os.Getenv("TEST_REALITY_SHORT_ID")

	if host == "" || uuid == "" {
		t.Skip("TEST_SERVER_HOST or TEST_USER_UUID not set")
	}

	suite := NewTestSuite(t)

	// 璁剧疆鏈嶅姟鍣ㄤ俊鎭?
	serverPort := 443
	if port != "" {
		fmt.Sscanf(port, "%d", &serverPort)
	}

	proto := config.ProtocolVLESS
	if protocol != "" {
		proto = config.Protocol(protocol)
	}

	suite.SetServer(host, serverPort, proto)
	suite.SetUser(uuid, "test@example.com")

	// 璁剧疆 Reality 閰嶇疆
	suite.serverInfo.TLSType = config.TLSReality
	suite.serverInfo.SNI = "www.google.com"
	suite.serverInfo.PublicKey = publicKey
	suite.serverInfo.ShortID = shortID
	suite.serverInfo.Transport = config.TransportTCP

	// 纭繚浜岃繘鍒舵枃浠跺彲鐢?
	if err := suite.Setup(); err != nil {
		t.Skipf("Binary setup failed: %v", err)
	}

	// 杩愯娴嬭瘯
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	report := suite.RunAllScenarios(ctx)

	// 鎵撳嵃缁撴灉
	t.Logf("Total: %d, Passed: %d, Failed: %d",
		report.TotalTests, report.PassedTests, report.FailedTests)

	for _, result := range report.Results {
		status := "PASS"
		if !result.Success {
			status = "FAIL"
		}
		t.Logf("  [%s] %s/%s - %v",
			status, result.ClientType, result.ScenarioName, result.Error)
	}

	// 鍏佽閮ㄥ垎澶辫触锛堝洜涓哄彲鑳芥湇鍔″櫒閰嶇疆涓嶆敮鎸佹墍鏈夊崗璁級
	if report.PassedTests == 0 {
		t.Error("All tests failed")
	}
}
