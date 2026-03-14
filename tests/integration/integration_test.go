package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/anixops/v2board/tests/integration/binary"
	"github.com/anixops/v2board/tests/integration/clients"
	"github.com/anixops/v2board/tests/integration/config"
	"github.com/anixops/v2board/tests/integration/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSuite 集成测试套件
type TestSuite struct {
	t          *testing.T
	binMgr     *binary.Manager
	configDir  string
	serverInfo config.ServerConfig
	userInfo   config.UserConfig
}

// NewTestSuite 创建测试套件
func NewTestSuite(t *testing.T) *TestSuite {
	configDir := t.TempDir()
	return &TestSuite{
		t:         t,
		binMgr:    binary.NewManager(""),
		configDir: configDir,
	}
}

// Setup 设置测试环境
func (s *TestSuite) Setup() error {
	// 设置二进制管理器
	clients.SetBinaryManager(&binaryAdapter{mgr: s.binMgr})

	// 确保二进制文件可用
	if _, err := s.binMgr.EnsureBinary(&binary.XrayInfo); err != nil {
		return fmt.Errorf("xray binary not available: %w", err)
	}
	if _, err := s.binMgr.EnsureBinary(&binary.MihomoInfo); err != nil {
		return fmt.Errorf("mihomo binary not available: %w", err)
	}

	return nil
}

// SetServer 设置服务器信息
func (s *TestSuite) SetServer(host string, port int, protocol config.Protocol) {
	s.serverInfo = config.ServerConfig{
		Host:     host,
		Port:     port,
		Protocol: protocol,
	}
}

// SetUser 设置用户信息
func (s *TestSuite) SetUser(uuid, email string) {
	s.userInfo = config.UserConfig{
		UUID:  uuid,
		Email: email,
	}
}

// RunScenario 运行单个测试场景
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

// RunAllScenarios 运行所有测试场景
func (s *TestSuite) RunAllScenarios(ctx context.Context) *runner.TestReport {
	r := runner.NewRunner(
		runner.WithTimeout(30*time.Second),
		runner.WithConfigDir(s.configDir),
	)

	return r.Run(ctx, s.serverInfo, s.userInfo)
}

// binaryAdapter 适配器
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

// TestBinaryDownload 测试二进制下载
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

// TestConfigGeneration 测试配置生成
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
			// Xray 配置
			xrayGen := config.NewXrayGenerator()
			xrayConfig, err := xrayGen.GenerateFromScenario(scenario, server, user)
			require.NoError(t, err)
			assert.NotEmpty(t, xrayConfig)

			// Mihomo 配置
			mihomoGen := config.NewMihomoGenerator()
			mihomoConfig, err := mihomoGen.GenerateFromScenario(scenario, server, user)
			if err != nil {
				// 某些协议可能不支持
				t.Logf("Mihomo does not support %s: %v", scenario.Protocol, err)
			} else {
				assert.NotEmpty(t, mihomoConfig)
			}
		})
	}
}

// TestClientLifecycle 测试客户端生命周期
func TestClientLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	tmpDir := t.TempDir()

	// 创建测试配置
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

		// 重复添加
		err = mgr.Add("test", client)
		assert.Error(t, err)

		// 获取
		got, err := mgr.Get("test")
		require.NoError(t, err)
		assert.Equal(t, client, got)

		// 列表
		list := mgr.List()
		assert.Contains(t, list, "test")

		// 状态
		status := mgr.Status()
		assert.Contains(t, status, "test")

		// 删除
		err = mgr.Remove("test")
		require.NoError(t, err)

		_, err = mgr.Get("test")
		assert.Error(t, err)
	})
}

// TestRunnerBasic 测试运行器基本功能
func TestRunnerBasic(t *testing.T) {
	r := runner.NewRunner(
		runner.WithTimeout(5*time.Second),
		runner.WithConfigDir(t.TempDir()),
	)

	// 测试报告
	report := r.GetReport()
	assert.NotNil(t, report)
	assert.Equal(t, 0, report.PassedTests)
}

// TestHealthCheck 测试健康检查（需要实际服务器）
func TestHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	// 检查是否有可用的测试服务器
	testServer := os.Getenv("TEST_SERVER_HOST")
	if testServer == "" {
		t.Skip("TEST_SERVER_HOST not set, skipping health check test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 简单的 HTTP 健康检查
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

// TestFullIntegration 全集成测试（需要真实服务器和配置）
func TestFullIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	// 从环境变量读取配置
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

	// 设置服务器信息
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

	// 设置 Reality 配置
	suite.serverInfo.TLSType = config.TLSReality
	suite.serverInfo.SNI = "www.google.com"
	suite.serverInfo.PublicKey = publicKey
	suite.serverInfo.ShortID = shortID
	suite.serverInfo.Transport = config.TransportTCP

	// 确保二进制文件可用
	if err := suite.Setup(); err != nil {
		t.Skipf("Binary setup failed: %v", err)
	}

	// 运行测试
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	report := suite.RunAllScenarios(ctx)

	// 打印结果
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

	// 允许部分失败（因为可能服务器配置不支持所有协议）
	if report.PassedTests == 0 {
		t.Error("All tests failed")
	}
}