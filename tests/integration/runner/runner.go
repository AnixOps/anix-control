package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anixops/v2board/tests/integration/clients"
	"github.com/anixops/v2board/tests/integration/config"
)

// TestResult 测试结果
type TestResult struct {
	ScenarioName string        `json:"scenario_name"`
	ClientType   string        `json:"client_type"`
	Protocol     string        `json:"protocol"`
	TLS          string        `json:"tls"`
	Success      bool          `json:"success"`
	Duration     time.Duration `json:"duration"`
	Error        string        `json:"error,omitempty"`
	Latency      time.Duration `json:"latency,omitempty"`
	Logs         string        `json:"logs,omitempty"`
}

// TestReport 测试报告
type TestReport struct {
	Timestamp   time.Time    `json:"timestamp"`
	TotalTests  int          `json:"total_tests"`
	PassedTests int          `json:"passed_tests"`
	FailedTests int          `json:"failed_tests"`
	Results     []TestResult `json:"results"`
}

// Runner 测试运行器
type Runner struct {
	configDir    string
	timeout      time.Duration
	parallel     bool
	maxParallel  int
	report       *TestReport
	mu           sync.Mutex
	scenarios    []config.TestScenario
	generators   map[string]config.Generator
}

// RunnerOption 运行器选项
type RunnerOption func(*Runner)

// WithTimeout 设置超时
func WithTimeout(timeout time.Duration) RunnerOption {
	return func(r *Runner) {
		r.timeout = timeout
	}
}

// WithParallel 设置并行测试
func WithParallel(maxParallel int) RunnerOption {
	return func(r *Runner) {
		r.parallel = true
		r.maxParallel = maxParallel
	}
}

// WithConfigDir 设置配置目录
func WithConfigDir(dir string) RunnerOption {
	return func(r *Runner) {
		r.configDir = dir
	}
}

// WithScenarios 设置测试场景
func WithScenarios(scenarios []config.TestScenario) RunnerOption {
	return func(r *Runner) {
		r.scenarios = scenarios
	}
}

// NewRunner 创建测试运行器
func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{
		configDir:   "./test_configs",
		timeout:     30 * time.Second,
		parallel:    false,
		maxParallel: 4,
		report: &TestReport{
			Results: []TestResult{},
		},
		scenarios: config.DefaultTestScenarios,
		generators: map[string]config.Generator{
			"xray":   config.NewXrayGenerator(),
			"mihomo": config.NewMihomoGenerator(),
		},
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Run 运行所有测试场景
func (r *Runner) Run(ctx context.Context, server config.ServerConfig, user config.UserConfig) *TestReport {
	r.report.Timestamp = time.Now()
	r.report.TotalTests = len(r.scenarios) * len(r.generators)
	r.report.PassedTests = 0
	r.report.FailedTests = 0
	r.report.Results = []TestResult{}

	// 确保配置目录存在
	os.MkdirAll(r.configDir, 0755)

	if r.parallel {
		r.runParallel(ctx, server, user)
	} else {
		r.runSequential(ctx, server, user)
	}

	return r.report
}

// runSequential 顺序执行测试
func (r *Runner) runSequential(ctx context.Context, server config.ServerConfig, user config.UserConfig) {
	for _, scenario := range r.scenarios {
		for genName, generator := range r.generators {
			result := r.runTest(ctx, scenario, server, user, genName, generator)
			r.addResult(result)
		}
	}
}

// runParallel 并行执行测试
func (r *Runner) runParallel(ctx context.Context, server config.ServerConfig, user config.UserConfig) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, r.maxParallel)

	for _, scenario := range r.scenarios {
		for genName, generator := range r.generators {
			wg.Add(1)
			go func(s config.TestScenario, gn string, g config.Generator) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				result := r.runTest(ctx, s, server, user, gn, g)
				r.addResult(result)
			}(scenario, genName, generator)
		}
	}

	wg.Wait()
}

// runTest 执行单个测试
func (r *Runner) runTest(ctx context.Context, scenario config.TestScenario, server config.ServerConfig, user config.UserConfig, genName string, generator config.Generator) TestResult {
	result := TestResult{
		ScenarioName: scenario.Name,
		ClientType:   genName,
		Protocol:     string(scenario.Protocol),
		TLS:          string(scenario.TLS),
	}

	startTime := time.Now()
	defer func() {
		result.Duration = time.Since(startTime)
	}()

	// 检查协议是否支持
	if !generator.IsProtocolSupported(scenario.Protocol) {
		result.Success = false
		result.Error = fmt.Sprintf("protocol %s not supported by %s", scenario.Protocol, genName)
		return result
	}

	// 创建客户端配置
	configContent, err := generator.GenerateFromScenario(scenario, server, user)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to generate config: %v", err)
		return result
	}

	// 保存配置文件
	configPath := filepath.Join(r.configDir, fmt.Sprintf("%s_%s.json", scenario.Name, genName))
	if genName == "mihomo" {
		configPath = filepath.Join(r.configDir, fmt.Sprintf("%s_%s.yaml", scenario.Name, genName))
	}
	if err := os.WriteFile(configPath, configContent, 0644); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to write config: %v", err)
		return result
	}
	defer os.Remove(configPath)

	// 创建客户端
	var client clients.Client
	switch genName {
	case "xray":
		client = clients.NewXrayClient(
			clients.WithConfig(configPath),
			clients.WithPorts(getPorts(scenario.Name, "xray")),
		)
	case "mihomo":
		client = clients.NewMihomoClient(
			clients.WithConfig(configPath),
			clients.WithPorts(getPorts(scenario.Name, "mihomo")),
		)
	}

	// 启动客户端
	testCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	if err := client.Start(testCtx); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to start client: %v", err)
		result.Logs = client.Logs()
		return result
	}
	defer client.Stop()

	// 等待客户端稳定
	time.Sleep(500 * time.Millisecond)

	// 执行连通性测试
	latency, err := r.testConnectivity(testCtx, client)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("connectivity test failed: %v", err)
		result.Logs = client.Logs()
		return result
	}

	result.Success = true
	result.Latency = latency
	return result
}

// testConnectivity 测试连通性
func (r *Runner) testConnectivity(ctx context.Context, client clients.Client) (time.Duration, error) {
	start := time.Now()

	// 通过代理发送请求
	healthy := client.IsHealthy(ctx)
	if !healthy {
		return 0, fmt.Errorf("health check failed")
	}

	return time.Since(start), nil
}

// addResult 添加测试结果
func (r *Runner) addResult(result TestResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.report.Results = append(r.report.Results, result)
	if result.Success {
		r.report.PassedTests++
	} else {
		r.report.FailedTests++
	}
}

// GetReport 获取测试报告
func (r *Runner) GetReport() *TestReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.report
}

// SaveReport 保存测试报告
func (r *Runner) SaveReport(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.MarshalIndent(r.report, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// PrintReport 打印测试报告
func (r *Runner) PrintReport() {
	r.mu.Lock()
	defer r.mu.Unlock()

	fmt.Println("\n========== Test Report ==========")
	fmt.Printf("Time: %s\n", r.report.Timestamp.Format(time.RFC3339))
	fmt.Printf("Total: %d, Passed: %d, Failed: %d\n",
		r.report.TotalTests, r.report.PassedTests, r.report.FailedTests)
	fmt.Printf("Pass Rate: %.1f%%\n",
		float64(r.report.PassedTests)/float64(r.report.TotalTests)*100)
	fmt.Println("\nDetails:")

	for _, result := range r.report.Results {
		status := "✅ PASS"
		if !result.Success {
			status = "❌ FAIL"
		}
		fmt.Printf("  %s [%s/%s] %s (%v) - %v\n",
			status, result.ClientType, result.Protocol, result.ScenarioName,
			result.Duration.Round(time.Millisecond), result.Latency)
		if result.Error != "" {
			fmt.Printf("       Error: %s\n", result.Error)
		}
	}
	fmt.Println("=================================")
}

// getPorts 获取端口配置 (避免端口冲突)
func getPorts(scenarioName, clientType string) (http, socks, mixed, api int) {
	// 基础端口
	baseHTTP := 20000
	baseSocks := 20100
	baseMixed := 20200
	baseAPI := 20300

	// 根据场景名生成唯一偏移
	offset := 0
	for i, c := range scenarioName {
		offset += int(c) + i
	}

	http = baseHTTP + offset%100
	socks = baseSocks + offset%100
	mixed = baseMixed + offset%100
	api = baseAPI + offset%100

	return
}