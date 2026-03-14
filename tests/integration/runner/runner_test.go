package runner

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/anixops/v2board/tests/integration/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRunner(t *testing.T) {
	r := NewRunner()
	assert.NotNil(t, r)
	assert.Equal(t, 30*time.Second, r.timeout)
	assert.False(t, r.parallel)
	assert.NotNil(t, r.generators)
	assert.Contains(t, r.generators, "xray")
	assert.Contains(t, r.generators, "mihomo")
}

func TestRunnerWithOptions(t *testing.T) {
	r := NewRunner(
		WithTimeout(60*time.Second),
		WithParallel(8),
		WithConfigDir("/tmp/test_configs"),
	)

	assert.Equal(t, 60*time.Second, r.timeout)
	assert.True(t, r.parallel)
	assert.Equal(t, 8, r.maxParallel)
	assert.Equal(t, "/tmp/test_configs", r.configDir)
}

func TestRunnerWithCustomScenarios(t *testing.T) {
	customScenarios := []config.TestScenario{
		{
			Name:        "test-vless-reality",
			Description: "Test VLESS with Reality",
			Protocol:    config.ProtocolVLESS,
			Transport:   config.TransportTCP,
			TLS:         config.TLSReality,
		},
	}

	r := NewRunner(WithScenarios(customScenarios))
	assert.Len(t, r.scenarios, 1)
	assert.Equal(t, "test-vless-reality", r.scenarios[0].Name)
}

func TestGetReport(t *testing.T) {
	r := NewRunner()
	report := r.GetReport()
	assert.NotNil(t, report)
	assert.Equal(t, 0, report.TotalTests)
}

func TestAddResult(t *testing.T) {
	r := NewRunner()

	r.addResult(TestResult{
		ScenarioName: "test1",
		ClientType:   "xray",
		Success:      true,
	})

	r.addResult(TestResult{
		ScenarioName: "test2",
		ClientType:   "mihomo",
		Success:      false,
		Error:        "connection failed",
	})

	report := r.GetReport()
	assert.Len(t, report.Results, 2)
	assert.Equal(t, 1, report.PassedTests)
	assert.Equal(t, 1, report.FailedTests)
}

func TestGetPorts(t *testing.T) {
	http, socks, mixed, api := getPorts("test-scenario", "xray")

	// 端口应该在合理范围内
	assert.Greater(t, http, 20000)
	assert.Less(t, http, 20100)
	assert.Greater(t, socks, 20100)
	assert.Less(t, socks, 20200)
	assert.Greater(t, mixed, 20200)
	assert.Less(t, mixed, 20300)
	assert.Greater(t, api, 20300)
	assert.Less(t, api, 20400)
}

func TestGetPortsUnique(t *testing.T) {
	http1, socks1, mixed1, api1 := getPorts("scenario-a", "xray")
	http2, socks2, mixed2, api2 := getPorts("scenario-b", "xray")

	// 不同场景应该有不同的端口
	assert.NotEqual(t, []int{http1, socks1, mixed1, api1},
		[]int{http2, socks2, mixed2, api2})
}

func TestRunProtocolNotSupported(t *testing.T) {
	// 创建一个只支持特定协议的场景
	scenarios := []config.TestScenario{
		{
			Name:        "test-hysteria2",
			Description: "Test Hysteria2",
			Protocol:    config.ProtocolHysteria2,
			Transport:   config.TransportQUIC,
			TLS:         config.TLS,
		},
	}

	r := NewRunner(
		WithScenarios(scenarios),
		WithTimeout(5*time.Second),
	)

	server := config.ServerConfig{
		Host:       "example.com",
		Port:       443,
		Protocol:   config.ProtocolHysteria2,
		TLSType:    config.TLS,
		Transport:  config.TransportQUIC,
	}

	user := config.UserConfig{
		UUID:  "test-uuid",
		Email: "test@example.com",
	}

	// Hysteria2 不被 Xray/Mihomo 标准配置生成器支持
	report := r.Run(context.Background(), server, user)

	// 应该有结果，但可能会失败（因为协议不支持）
	assert.NotNil(t, report)
}

func TestRunnerSaveReport(t *testing.T) {
	r := NewRunner()

	r.addResult(TestResult{
		ScenarioName: "test1",
		ClientType:   "xray",
		Protocol:     "vless",
		TLS:          "reality",
		Success:      true,
		Latency:      100 * time.Millisecond,
	})

	// 创建临时文件
	tmpDir := t.TempDir()
	reportPath := tmpDir + "/report.json"

	err := r.SaveReport(reportPath)
	require.NoError(t, err)

	// 读取并验证文件
	data, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "test1")
	assert.Contains(t, string(data), "vless")
}

func TestRunnerPrintReport(t *testing.T) {
	r := NewRunner()

	r.addResult(TestResult{
		ScenarioName: "test-pass",
		ClientType:   "xray",
		Protocol:     "vless",
		TLS:          "reality",
		Success:      true,
		Latency:      50 * time.Millisecond,
	})

	r.addResult(TestResult{
		ScenarioName: "test-fail",
		ClientType:   "mihomo",
		Protocol:     "vmess",
		TLS:          "tls",
		Success:      false,
		Error:        "connection timeout",
	})

	// PrintReport 不应该 panic
	r.PrintReport()
}

func TestTestResultJSON(t *testing.T) {
	result := TestResult{
		ScenarioName: "vless-reality",
		ClientType:   "xray",
		Protocol:     "vless",
		TLS:          "reality",
		Success:      true,
		Duration:     5 * time.Second,
		Latency:      100 * time.Millisecond,
	}

	// 序列化
	data, err := json.Marshal(result)
	require.NoError(t, err)

	// 反序列化
	var decoded TestResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, result.ScenarioName, decoded.ScenarioName)
	assert.Equal(t, result.ClientType, decoded.ClientType)
	assert.Equal(t, result.Success, decoded.Success)
}

func TestTestReportJSON(t *testing.T) {
	report := TestReport{
		Timestamp:   time.Now(),
		TotalTests:  10,
		PassedTests: 8,
		FailedTests: 2,
		Results: []TestResult{
			{ScenarioName: "test1", Success: true},
			{ScenarioName: "test2", Success: false, Error: "timeout"},
		},
	}

	data, err := json.Marshal(report)
	require.NoError(t, err)

	var decoded TestReport
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, report.TotalTests, decoded.TotalTests)
	assert.Equal(t, report.PassedTests, decoded.PassedTests)
	assert.Len(t, decoded.Results, 2)
}