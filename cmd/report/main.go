package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/net/proxy"
)

// TestReport 测试报告
type TestReport struct {
	GeneratedAt   time.Time     `json:"generated_at"`
	ServerVersion string        `json:"server_version"`
	GoVersion     string        `json:"go_version"`
	Platform      string        `json:"platform"`
	Summary       TestSummary   `json:"summary"`
	Results       []TestResult  `json:"results"`
	E2EResults    []E2EResult   `json:"e2e_results"`
	APIResults    []APITest     `json:"api_tests"`
}

// TestSummary 测试汇总
type TestSummary struct {
	TotalTests     int     `json:"total_tests"`
	PassedTests    int     `json:"passed_tests"`
	FailedTests    int     `json:"failed_tests"`
	SkippedTests   int     `json:"skipped_tests"`
	PassRate       float64 `json:"pass_rate"`
	TotalDuration  string  `json:"total_duration"`
}

// TestResult 单元测试结果
type TestResult struct {
	Package    string        `json:"package"`
	TestName   string        `json:"test_name"`
	Status     string        `json:"status"` // PASS, FAIL, SKIP
	Duration   time.Duration `json:"duration"`
	Error      string        `json:"error,omitempty"`
}

// E2EResult E2E测试结果
type E2EResult struct {
	Protocol    string        `json:"protocol"`
	ServerPort  int           `json:"server_port"`
	ProxyPort   int           `json:"proxy_port"`
	EchoPort    int           `json:"echo_port"`
	Success     bool          `json:"success"`
	Latency     time.Duration `json:"latency"`
	Error       string        `json:"error,omitempty"`
	Duration    time.Duration `json:"duration"`
}

// APITest API测试结果
type APITest struct {
	Endpoint    string        `json:"endpoint"`
	Method      string        `json:"method"`
	Status      int           `json:"status"`
	Latency     time.Duration `json:"latency"`
	Success     bool          `json:"success"`
	Response    string        `json:"response,omitempty"`
	Error       string        `json:"error,omitempty"`
}

var report TestReport

func main() {
	report = TestReport{
		GeneratedAt:   time.Now(),
		ServerVersion: "v2.0.0",
		GoVersion:     runtime.Version(),
		Platform:      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Results:       make([]TestResult, 0),
		E2EResults:    make([]E2EResult, 0),
		APIResults:    make([]APITest, 0),
	}

	fmt.Println("========================================")
	fmt.Println("   V2Board 完整测试报告生成器")
	fmt.Println("========================================")
	fmt.Println()

	// 1. API 测试
	fmt.Println("[1/4] 执行 API 测试...")
	runAPITests()

	// 2. E2E 代理测试
	fmt.Println("[2/4] 执行 E2E 代理测试...")
	runE2ETests()

	// 3. 单元测试
	fmt.Println("[3/4] 执行单元测试...")
	runUnitTests()

	// 4. 生成报告
	fmt.Println("[4/4] 生成报告文件...")
	generateSummary()
	generateCSVReport()
	generateHTMLReport()

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("           测试报告生成完成")
	fmt.Println("========================================")
	fmt.Printf("总测试数: %d\n", report.Summary.TotalTests)
	fmt.Printf("通过: %d\n", report.Summary.PassedTests)
	fmt.Printf("失败: %d\n", report.Summary.FailedTests)
	fmt.Printf("跳过: %d\n", report.Summary.SkippedTests)
	fmt.Printf("通过率: %.1f%%\n", report.Summary.PassRate)
	fmt.Println()
	fmt.Println("报告文件:")
	fmt.Println("  - test-reports/report.html")
	fmt.Println("  - test-reports/report.csv")
	fmt.Println("  - test-reports/report.json")
}

func runAPITests() {
	baseURL := "http://localhost:8080"

	tests := []struct {
		name     string
		endpoint string
		method   string
	}{
		{"健康检查", "/health", "GET"},
		{"订阅接口", "/s/fe7464f5-4091-4e6a-8577-ad0b33c67a7d", "GET"},
	}

	for _, tt := range tests {
		start := time.Now()
		var req *http.Request
		var err error

		url := baseURL + tt.endpoint
		if tt.method == "GET" {
			req, err = http.NewRequest("GET", url, nil)
		}

		var result APITest
		if err != nil {
			result = APITest{
				Endpoint: tt.endpoint,
				Method:   tt.method,
				Success:  false,
				Error:    err.Error(),
			}
		} else {
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			result = APITest{
				Endpoint: tt.endpoint,
				Method:   tt.method,
				Latency:  time.Since(start),
				Success:  err == nil && resp.StatusCode >= 200 && resp.StatusCode < 400,
			}
			if err != nil {
				result.Error = err.Error()
			} else {
				result.Status = resp.StatusCode
				body, _ := io.ReadAll(resp.Body)
				if len(body) > 200 {
					result.Response = string(body[:200]) + "..."
				} else {
					result.Response = string(body)
				}
				resp.Body.Close()
			}
		}

		report.APIResults = append(report.APIResults, result)
		status := "✅"
		if !result.Success {
			status = "❌"
		}
		fmt.Printf("  %s %s (%v)\n", status, tt.name, result.Latency)
	}
}

func runE2ETests() {
	xrayPath := "./bin/xray.exe"
	if _, err := os.Stat(xrayPath); os.IsNotExist(err) {
		// 尝试从 PATH 查找
		if path, err := exec.LookPath("xray"); err == nil {
			xrayPath = path
		} else {
			fmt.Println("  ⚠️ Xray 未找到，跳过 E2E 测试")
			return
		}
	}

	protocols := []string{"shadowsocks", "vmess", "vless", "trojan"}

	for _, protocol := range protocols {
		result := runE2EProtocolTest(xrayPath, protocol)
		report.E2EResults = append(report.E2EResults, result)

		status := "✅"
		if !result.Success {
			status = "❌"
		}
		fmt.Printf("  %s %s (延迟: %v)\n", status, protocol, result.Latency)
	}
}

func runE2EProtocolTest(xrayPath, protocol string) E2EResult {
	result := E2EResult{
		Protocol: protocol,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "v2board-test-*")
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer os.RemoveAll(tmpDir)

	// 获取端口
	echoPort := getFreePort()
	serverPort := getFreePort()
	proxyPort := getFreePort()

	result.EchoPort = echoPort
	result.ServerPort = serverPort
	result.ProxyPort = proxyPort

	// 启动 Echo 服务器
	echoServer := NewEchoServer(echoPort)
	if err := echoServer.Start(ctx); err != nil {
		result.Error = fmt.Sprintf("Echo server failed: %v", err)
		return result
	}
	defer echoServer.Stop()

	// 生成并启动服务端
	serverConfig := generateServerConfig(protocol, serverPort)
	serverConfigPath := filepath.Join(tmpDir, "server.json")
	os.WriteFile(serverConfigPath, []byte(serverConfig), 0644)

	serverCmd := exec.CommandContext(ctx, xrayPath, "run", "-c", serverConfigPath)
	if runtime.GOOS != "windows" {
		serverCmd.Stdout = os.Stdout
		serverCmd.Stderr = os.Stderr
	}
	if err := serverCmd.Start(); err != nil {
		result.Error = fmt.Sprintf("Server start failed: %v", err)
		return result
	}
	defer cleanupProcess(serverCmd)

	if !waitForPort(serverPort, 5*time.Second) {
		result.Error = "Server startup timeout"
		return result
	}

	// 生成并启动客户端
	clientConfig := generateClientConfig(protocol, serverPort, proxyPort)
	clientConfigPath := filepath.Join(tmpDir, "client.json")
	os.WriteFile(clientConfigPath, []byte(clientConfig), 0644)

	clientCmd := exec.CommandContext(ctx, xrayPath, "run", "-c", clientConfigPath)
	if runtime.GOOS != "windows" {
		clientCmd.Stdout = os.Stdout
		clientCmd.Stderr = os.Stderr
	}
	if err := clientCmd.Start(); err != nil {
		result.Error = fmt.Sprintf("Client start failed: %v", err)
		return result
	}
	defer cleanupProcess(clientCmd)

	if !waitForPort(proxyPort, 5*time.Second) {
		result.Error = "Client startup timeout"
		return result
	}

	// 测试连通性
	start := time.Now()
	err = testThroughProxy(proxyPort, echoPort)
	result.Latency = time.Since(start)
	result.Duration = result.Latency

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
	}

	return result
}

func runUnitTests() {
	// 运行测试命令并解析输出
	cmd := exec.Command("go", "test", "-v", "-short", "-json", "./...")
	cmd.Dir = "."

	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("  ⚠️ 单元测试执行警告: %v\n", err)
	}

	// 解析 JSON 输出
	lines := splitLines(string(output))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if action, ok := event["Action"].(string); ok {
			if action == "pass" || action == "fail" || action == "skip" {
				pkg, _ := event["Package"].(string)
				testName, _ := event["Test"].(string)
				if testName == "" {
					continue
				}

				result := TestResult{
					Package:  pkg,
					TestName: testName,
					Status:   string(action),
				}

				if elapsed, ok := event["Elapsed"].(float64); ok {
					result.Duration = time.Duration(elapsed * float64(time.Second))
				}

				report.Results = append(report.Results, result)
			}
		}
	}
}

func generateSummary() {
	total := len(report.Results) + len(report.E2EResults) + len(report.APIResults)
	passed := 0
	failed := 0
	skipped := 0

	for _, r := range report.Results {
		switch r.Status {
		case "pass":
			passed++
		case "fail":
			failed++
		case "skip":
			skipped++
		}
	}

	for _, r := range report.E2EResults {
		if r.Success {
			passed++
		} else {
			failed++
		}
	}

	for _, r := range report.APIResults {
		if r.Success {
			passed++
		} else {
			failed++
		}
	}

	var passRate float64
	if total > 0 {
		passRate = float64(passed) / float64(total) * 100
	}

	report.Summary = TestSummary{
		TotalTests:    total,
		PassedTests:   passed,
		FailedTests:   failed,
		SkippedTests:  skipped,
		PassRate:      passRate,
		TotalDuration: time.Since(report.GeneratedAt).String(),
	}
}

func generateCSVReport() {
	// 创建 CSV 文件
	file, err := os.Create("test-reports/report.csv")
	if err != nil {
		fmt.Printf("创建 CSV 失败: %v\n", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入汇总
	writer.Write([]string{"测试报告汇总"})
	writer.Write([]string{"生成时间", report.GeneratedAt.Format("2006-01-02 15:04:05")})
	writer.Write([]string{"服务器版本", report.ServerVersion})
	writer.Write([]string{"Go 版本", report.GoVersion})
	writer.Write([]string{"平台", report.Platform})
	writer.Write([]string{})
	writer.Write([]string{"总测试数", fmt.Sprintf("%d", report.Summary.TotalTests)})
	writer.Write([]string{"通过数", fmt.Sprintf("%d", report.Summary.PassedTests)})
	writer.Write([]string{"失败数", fmt.Sprintf("%d", report.Summary.FailedTests)})
	writer.Write([]string{"跳过数", fmt.Sprintf("%d", report.Summary.SkippedTests)})
	writer.Write([]string{"通过率", fmt.Sprintf("%.1f%%", report.Summary.PassRate)})
	writer.Write([]string{})

	// API 测试结果
	writer.Write([]string{"API 测试结果"})
	writer.Write([]string{"端点", "方法", "状态码", "延迟", "结果", "错误"})
	for _, r := range report.APIResults {
		status := "成功"
		if !r.Success {
			status = "失败"
		}
		writer.Write([]string{
			r.Endpoint,
			r.Method,
			fmt.Sprintf("%d", r.Status),
			r.Latency.String(),
			status,
			r.Error,
		})
	}
	writer.Write([]string{})

	// E2E 测试结果
	writer.Write([]string{"E2E 代理测试结果"})
	writer.Write([]string{"协议", "服务端端口", "代理端口", "Echo端口", "延迟", "结果", "错误"})
	for _, r := range report.E2EResults {
		status := "成功"
		if !r.Success {
			status = "失败"
		}
		writer.Write([]string{
			r.Protocol,
			fmt.Sprintf("%d", r.ServerPort),
			fmt.Sprintf("%d", r.ProxyPort),
			fmt.Sprintf("%d", r.EchoPort),
			r.Latency.String(),
			status,
			r.Error,
		})
	}
	writer.Write([]string{})

	// 单元测试结果
	writer.Write([]string{"单元测试结果"})
	writer.Write([]string{"包名", "测试名", "状态", "耗时"})
	for _, r := range report.Results {
		writer.Write([]string{
			r.Package,
			r.TestName,
			r.Status,
			r.Duration.String(),
		})
	}

	fmt.Println("  ✅ CSV 报告已生成: test-reports/report.csv")
}

func generateHTMLReport() {
	tmpl := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>V2Board 测试报告</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f7fa; color: #333; }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; border-radius: 10px; margin-bottom: 20px; }
        .header h1 { font-size: 28px; margin-bottom: 10px; }
        .header .meta { opacity: 0.9; font-size: 14px; }
        .summary { display: grid; grid-template-columns: repeat(5, 1fr); gap: 15px; margin-bottom: 20px; }
        .summary-card { background: white; padding: 20px; border-radius: 10px; text-align: center; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .summary-card .value { font-size: 32px; font-weight: bold; margin-bottom: 5px; }
        .summary-card .label { color: #666; font-size: 14px; }
        .summary-card.success .value { color: #10b981; }
        .summary-card.fail .value { color: #ef4444; }
        .summary-card.skip .value { color: #f59e0b; }
        .summary-card.rate .value { color: #3b82f6; }
        .section { background: white; border-radius: 10px; padding: 20px; margin-bottom: 20px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .section h2 { font-size: 18px; margin-bottom: 15px; padding-bottom: 10px; border-bottom: 2px solid #eee; }
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #eee; }
        th { background: #f8fafc; font-weight: 600; }
        tr:hover { background: #f8fafc; }
        .badge { padding: 4px 12px; border-radius: 20px; font-size: 12px; font-weight: 500; }
        .badge-success { background: #d1fae5; color: #065f46; }
        .badge-fail { background: #fee2e2; color: #991b1b; }
        .badge-skip { background: #fef3c7; color: #92400e; }
        .progress-bar { width: 100%; height: 20px; background: #e5e7eb; border-radius: 10px; overflow: hidden; }
        .progress-bar .fill { height: 100%; background: linear-gradient(90deg, #10b981 0%, #34d399 100%); transition: width 0.5s; }
        .latency { font-family: 'SF Mono', Monaco, monospace; color: #6b7280; }
        .footer { text-align: center; padding: 20px; color: #666; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🧪 V2Board 测试报告</h1>
            <div class="meta">
                <p>生成时间: {{.GeneratedAt.Format "2006-01-02 15:04:05"}}</p>
                <p>服务器版本: {{.ServerVersion}} | Go 版本: {{.GoVersion}} | 平台: {{.Platform}}</p>
            </div>
        </div>

        <div class="summary">
            <div class="summary-card">
                <div class="value">{{.Summary.TotalTests}}</div>
                <div class="label">总测试数</div>
            </div>
            <div class="summary-card success">
                <div class="value">{{.Summary.PassedTests}}</div>
                <div class="label">通过</div>
            </div>
            <div class="summary-card fail">
                <div class="value">{{.Summary.FailedTests}}</div>
                <div class="label">失败</div>
            </div>
            <div class="summary-card skip">
                <div class="value">{{.Summary.SkippedTests}}</div>
                <div class="label">跳过</div>
            </div>
            <div class="summary-card rate">
                <div class="value">{{printf "%.1f" .Summary.PassRate}}%</div>
                <div class="label">通过率</div>
            </div>
        </div>

        <div class="section">
            <h2>📊 通过率</h2>
            <div class="progress-bar">
                <div class="fill" style="width: {{printf "%.1f" .Summary.PassRate}}%"></div>
            </div>
        </div>

        <div class="section">
            <h2>🌐 API 测试结果</h2>
            <table>
                <thead>
                    <tr>
                        <th>端点</th>
                        <th>方法</th>
                        <th>状态码</th>
                        <th>延迟</th>
                        <th>结果</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .APIResults}}
                    <tr>
                        <td><code>{{.Endpoint}}</code></td>
                        <td>{{.Method}}</td>
                        <td>{{if .Status}}{{.Status}}{{else}}-{{end}}</td>
                        <td class="latency">{{.Latency}}</td>
                        <td>
                            {{if .Success}}
                            <span class="badge badge-success">✅ 成功</span>
                            {{else}}
                            <span class="badge badge-fail">❌ 失败</span>
                            {{end}}
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>

        <div class="section">
            <h2>🚀 E2E 代理测试结果</h2>
            <table>
                <thead>
                    <tr>
                        <th>协议</th>
                        <th>服务端端口</th>
                        <th>代理端口</th>
                        <th>延迟</th>
                        <th>结果</th>
                        <th>错误</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .E2EResults}}
                    <tr>
                        <td><strong>{{.Protocol}}</strong></td>
                        <td>{{.ServerPort}}</td>
                        <td>{{.ProxyPort}}</td>
                        <td class="latency">{{.Latency}}</td>
                        <td>
                            {{if .Success}}
                            <span class="badge badge-success">✅ 通过</span>
                            {{else}}
                            <span class="badge badge-fail">❌ 失败</span>
                            {{end}}
                        </td>
                        <td>{{if .Error}}{{.Error}}{{else}}-{{end}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>

        <div class="section">
            <h2>🧪 单元测试结果 (前50个)</h2>
            <table>
                <thead>
                    <tr>
                        <th>测试名</th>
                        <th>包</th>
                        <th>状态</th>
                        <th>耗时</th>
                    </tr>
                </thead>
                <tbody>
                    {{range $i, $r := .Results}}
                    {{if lt $i 50}}
                    <tr>
                        <td><code>{{$r.TestName}}</code></td>
                        <td>{{$r.Package}}</td>
                        <td>
                            {{if eq $r.Status "pass"}}
                            <span class="badge badge-success">✅ 通过</span>
                            {{else if eq $r.Status "fail"}}
                            <span class="badge badge-fail">❌ 失败</span>
                            {{else}}
                            <span class="badge badge-skip">⏭️ 跳过</span>
                            {{end}}
                        </td>
                        <td class="latency">{{$r.Duration}}</td>
                    </tr>
                    {{end}}
                    {{end}}
                </tbody>
            </table>
            {{if gt (len .Results) 50}}
            <p style="margin-top: 10px; color: #666;">... 共 {{len .Results}} 个测试</p>
            {{end}}
        </div>

        <div class="footer">
            <p>V2Board AnixOps 测试报告 | 生成于 {{.GeneratedAt.Format "2006-01-02 15:04:05"}}</p>
        </div>
    </div>
</body>
</html>`

	t, err := template.New("report").Parse(tmpl)
	if err != nil {
		fmt.Printf("模板解析失败: %v\n", err)
		return
	}

	file, err := os.Create("test-reports/report.html")
	if err != nil {
		fmt.Printf("创建 HTML 失败: %v\n", err)
		return
	}
	defer file.Close()

	t.Execute(file, report)
	fmt.Println("  ✅ HTML 报告已生成: test-reports/report.html")

	// 同时生成 JSON
	jsonData, _ := json.MarshalIndent(report, "", "  ")
	os.WriteFile("test-reports/report.json", jsonData, 0644)
	fmt.Println("  ✅ JSON 报告已生成: test-reports/report.json")
}

// 辅助函数
func getFreePort() int {
	addr, _ := net.ResolveTCPAddr("tcp", "localhost:0")
	l, _ := net.ListenTCP("tcp", addr)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForPort(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func cleanupProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cmd.Process.Kill()
		}
	}
}

func testThroughProxy(proxyPort, echoPort int) error {
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), nil, proxy.Direct)
	if err != nil {
		return err
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	}

	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", echoPort))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		return fmt.Errorf("unexpected response: %s", string(body))
	}
	return nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// Echo Server
type EchoServer struct {
	port   int
	server *http.Server
}

func NewEchoServer(port int) *EchoServer {
	return &EchoServer{port: port}
}

func (s *EchoServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("echo"))
	})

	s.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler: mux,
	}

	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}
	s.port = ln.Addr().(*net.TCPAddr).Port

	go s.server.Serve(ln)
	return nil
}

func (s *EchoServer) Stop() {
	if s.server != nil {
		s.server.Shutdown(context.Background())
	}
}

func generateServerConfig(protocol string, port int) string {
	switch protocol {
	case "shadowsocks":
		return fmt.Sprintf(`{"inbounds":[{"port":%d,"protocol":"shadowsocks","settings":{"method":"aes-256-gcm","password":"test-password-123","network":"tcp,udp"}}],"outbounds":[{"protocol":"freedom"}]}`, port)
	case "vmess":
		return fmt.Sprintf(`{"inbounds":[{"port":%d,"protocol":"vmess","settings":{"clients":[{"id":"12345678-1234-1234-1234-123456789abc","alterId":0}]}}],"outbounds":[{"protocol":"freedom"}]}`, port)
	case "vless":
		return fmt.Sprintf(`{"inbounds":[{"port":%d,"protocol":"vless","settings":{"clients":[{"id":"12345678-1234-1234-1234-123456789abc","flow":""}],"decryption":"none"}}],"outbounds":[{"protocol":"freedom"}]}`, port)
	case "trojan":
		return fmt.Sprintf(`{"inbounds":[{"port":%d,"protocol":"trojan","settings":{"clients":[{"password":"test-password-123"}]}}],"outbounds":[{"protocol":"freedom"}]}`, port)
	}
	return ""
}

func generateClientConfig(protocol string, serverPort, proxyPort int) string {
	switch protocol {
	case "shadowsocks":
		return fmt.Sprintf(`{"inbounds":[{"port":%d,"listen":"127.0.0.1","protocol":"socks","settings":{"udp":true}}],"outbounds":[{"protocol":"shadowsocks","settings":{"servers":[{"address":"127.0.0.1","port":%d,"method":"aes-256-gcm","password":"test-password-123"}]}}]}`, proxyPort, serverPort)
	case "vmess":
		return fmt.Sprintf(`{"inbounds":[{"port":%d,"listen":"127.0.0.1","protocol":"socks","settings":{"udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"127.0.0.1","port":%d,"users":[{"id":"12345678-1234-1234-1234-123456789abc","alterId":0}]}]}}]}`, proxyPort, serverPort)
	case "vless":
		return fmt.Sprintf(`{"inbounds":[{"port":%d,"listen":"127.0.0.1","protocol":"socks","settings":{"udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"127.0.0.1","port":%d,"users":[{"id":"12345678-1234-1234-1234-123456789abc","encryption":"none"}]}]}}]}`, proxyPort, serverPort)
	case "trojan":
		return fmt.Sprintf(`{"inbounds":[{"port":%d,"listen":"127.0.0.1","protocol":"socks","settings":{"udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"127.0.0.1","port":%d,"password":"test-password-123"}]}}]}`, proxyPort, serverPort)
	}
	return ""
}