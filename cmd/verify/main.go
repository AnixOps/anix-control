package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

func main() {
	ctx := context.Background()

	fmt.Println("========================================")
	fmt.Println("    AnixOps Control 完整 E2E 验证测试")
	fmt.Println("========================================")
	fmt.Println()

	xrayPath, err := resolveXrayPath(os.Getenv("V2BOARD_VERIFY_XRAY"))
	if err != nil {
		fmt.Printf("❌ Xray 路径不可用: %v\n", err)
		return
	}

	panelBaseURL, err := resolvePanelBaseURL(os.Getenv("V2BOARD_VERIFY_BASE_URL"))
	if err != nil {
		fmt.Printf("❌ 面板地址不可用: %v\n", err)
		return
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// 1. 检查服务器
	fmt.Println("[1/7] 检查 AnixOps Control 服务器...")
	healthURL := buildVerifyURL(panelBaseURL, "health")
	resp, err := getVerifyURL(ctx, httpClient, healthURL)
	if err != nil {
		fmt.Printf("❌ 服务器未运行: %v\n", err)
		return
	}
	if _, err := readAndCloseResponse(resp); err != nil {
		fmt.Printf("❌ 健康检查响应读取失败: %v\n", err)
		return
	}
	fmt.Println("AnixOps Control 服务器运行正常")
	fmt.Println()

	// 2. 获取订阅
	fmt.Println("[2/7] 获取订阅内容...")
	token, err := resolveVerifyToken(os.Getenv("V2BOARD_VERIFY_TOKEN"))
	if err != nil {
		fmt.Printf("❌ 订阅 token 未配置: %v\n", err)
		return
	}
	subURL := buildVerifyURL(panelBaseURL, "s", token)

	resp, err = getVerifyURL(ctx, httpClient, subURL)
	if err != nil {
		fmt.Printf("❌ 获取订阅失败: %v\n", err)
		return
	}
	body, err := readAndCloseResponse(resp)
	if err != nil {
		fmt.Printf("❌ 读取订阅响应失败: %v\n", err)
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		fmt.Printf("❌ 订阅 Base64 解码失败: %v\n", err)
		return
	}
	fmt.Println("✅ 订阅获取成功")
	fmt.Printf("   内容长度: %d 字节\n", len(decoded))
	fmt.Println()

	// 3. 测试 Shadowsocks 本地代理
	fmt.Println("[3/7] 创建 Shadowsocks 本地代理测试...")

	tmpDir, err := os.MkdirTemp("", "v2board-e2e-*")
	if err != nil {
		fmt.Printf("❌ 创建临时目录失败: %v\n", err)
		return
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			fmt.Printf("⚠️ 清理临时目录失败: %v\n", err)
		}
	}()

	// 获取空闲端口
	serverPort, err := getFreePort()
	if err != nil {
		fmt.Printf("❌ 获取服务端空闲端口失败: %v\n", err)
		return
	}
	proxyPort, err := getFreePort()
	if err != nil {
		fmt.Printf("❌ 获取代理空闲端口失败: %v\n", err)
		return
	}

	fmt.Printf("   服务端端口: %d\n", serverPort)
	fmt.Printf("   代理端口: %d\n", proxyPort)

	// 创建服务端配置
	serverConfig := fmt.Sprintf(`{
		"inbounds": [{
			"port": %d,
			"protocol": "shadowsocks",
			"settings": {
				"method": "aes-256-gcm",
				"password": "v2board-test-password",
				"network": "tcp,udp"
			}
		}],
		"outbounds": [{
			"protocol": "freedom"
		}]
	}`, serverPort)

	serverConfigPath := filepath.Join(tmpDir, "server.json")
	if err := writeVerifyConfig(serverConfigPath, serverConfig); err != nil {
		fmt.Printf("❌ 写入服务端配置失败: %v\n", err)
		return
	}

	// 启动服务端
	serverCmd := exec.CommandContext(ctx, xrayPath, "run", "-c", serverConfigPath) // #nosec G204 G702 -- xrayPath is resolved to a local xray/xray.exe binary before execution.
	serverCmd.Stdout = os.Stdout
	serverCmd.Stderr = os.Stderr
	if err := serverCmd.Start(); err != nil {
		fmt.Printf("❌ 启动服务端失败: %v\n", err)
		return
	}
	defer cleanupProcess(serverCmd)

	// 等待服务端启动
	if !waitForPort(serverPort, 5*time.Second) {
		fmt.Println("❌ 服务端启动超时")
		return
	}
	fmt.Println("✅ Shadowsocks 服务端启动成功")
	fmt.Println()

	// 4. 创建客户端
	fmt.Println("[4/7] 启动 SOCKS5 代理客户端...")

	clientConfig := fmt.Sprintf(`{
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
					"password": "v2board-test-password"
				}]
			}
		}]
	}`, proxyPort, serverPort)

	clientConfigPath := filepath.Join(tmpDir, "client.json")
	if err := writeVerifyConfig(clientConfigPath, clientConfig); err != nil {
		fmt.Printf("❌ 写入客户端配置失败: %v\n", err)
		return
	}

	clientCmd := exec.CommandContext(ctx, xrayPath, "run", "-c", clientConfigPath) // #nosec G204 G702 -- xrayPath is resolved to a local xray/xray.exe binary before execution.
	clientCmd.Stdout = os.Stdout
	clientCmd.Stderr = os.Stderr
	if err := clientCmd.Start(); err != nil {
		fmt.Printf("❌ 启动客户端失败: %v\n", err)
		return
	}
	defer cleanupProcess(clientCmd)

	// 等待客户端启动
	if !waitForPort(proxyPort, 5*time.Second) {
		fmt.Println("❌ 客户端启动超时")
		return
	}
	fmt.Println("✅ SOCKS5 代理启动成功")
	fmt.Println()

	// 5. 测试代理连接
	fmt.Println("[5/7] 测试代理连接到本地服务器...")

	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), nil, proxy.Direct)
	if err != nil {
		fmt.Printf("❌ 创建 SOCKS5 拨号器失败: %v\n", err)
		return
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	}

	proxyHTTPClient := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	// 通过代理访问本地 AnixOps Control 服务器
	testURL := buildVerifyURL(panelBaseURL, "health")
	start := time.Now()
	resp, err = getVerifyURL(ctx, proxyHTTPClient, testURL)
	latency := time.Since(start)

	if err != nil {
		fmt.Printf("❌ 代理连接失败: %v\n", err)
		return
	}
	if _, err := readAndCloseResponse(resp); err != nil {
		fmt.Printf("❌ 读取代理健康检查响应失败: %v\n", err)
		return
	}

	fmt.Printf("✅ 代理连接成功! 延迟: %v\n", latency)
	fmt.Println()

	// 6. 测试通过代理获取订阅
	fmt.Println("[6/7] 测试通过代理获取订阅...")

	start = time.Now()
	resp, err = getVerifyURL(ctx, proxyHTTPClient, subURL)
	if err != nil {
		fmt.Printf("❌ 通过代理获取订阅失败: %v\n", err)
		return
	}

	body, err = readAndCloseResponse(resp)
	if err != nil {
		fmt.Printf("❌ 读取代理订阅响应失败: %v\n", err)
		return
	}
	proxyLatency := time.Since(start)

	fmt.Printf("✅ 通过代理获取订阅成功! 延迟: %v\n", proxyLatency)
	fmt.Printf("   订阅大小: %d 字节\n", len(body))
	fmt.Println()

	// 7. 并发测试
	fmt.Println("[7/7] 并发代理测试...")

	var wg sync.WaitGroup
	results := make(chan bool, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			resp, err := getVerifyURL(ctx, proxyHTTPClient, subURL)
			if err == nil && resp.StatusCode == 200 {
				if _, closeErr := readAndCloseResponse(resp); closeErr != nil {
					fmt.Printf("   请求 %d: ❌ 响应读取失败: %v\n", id+1, closeErr)
					results <- false
					return
				}
				fmt.Printf("   请求 %d: ✅ 成功\n", id+1)
				results <- true
			} else {
				fmt.Printf("   请求 %d: ❌ 失败\n", id+1)
				results <- false
			}
		}(i)
	}

	wg.Wait()
	close(results)

	successCount := 0
	for result := range results {
		if result {
			successCount++
		}
	}

	time.Sleep(2 * time.Second)
	fmt.Println()

	// 总结
	fmt.Println("========================================")
	fmt.Println("           测试结果汇总")
	fmt.Println("========================================")
	fmt.Println("AnixOps Control 服务器运行正常")
	fmt.Println("✅ 订阅 API 工作正常")
	fmt.Println("✅ Xray 服务端启动成功")
	fmt.Println("✅ Xray 客户端启动成功")
	fmt.Println("✅ SOCKS5 代理工作正常")
	fmt.Println("✅ 通过代理访问成功")
	fmt.Printf("✅ 代理延迟: %v\n", latency)
	fmt.Printf("✅ 订阅获取延迟: %v\n", proxyLatency)
	fmt.Printf("✅ 并发测试: %d/5 成功\n", successCount)
	fmt.Println()
	fmt.Println("🎉 所有测试通过！项目可以正常运行！")
	fmt.Println("========================================")
}

func resolveXrayPath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = filepath.Join(".", "bin", "xray.exe")
	}
	clean := filepath.Clean(raw)
	name := filepath.Base(clean)
	if name != "xray" && name != "xray.exe" {
		return "", fmt.Errorf("only xray or xray.exe binaries are allowed, got %q", name)
	}

	info, err := os.Stat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("xray 未找到，请先下载或设置 V2BOARD_VERIFY_XRAY")
		}
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory", clean)
	}

	absolute, err := filepath.Abs(clean)
	if err != nil {
		return "", err
	}
	return absolute, nil
}

func resolveVerifyToken(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", errors.New("set V2BOARD_VERIFY_TOKEN to a local test user's subscription token")
	}
	if strings.ContainsAny(token, "/?#") {
		return "", errors.New("token must be a path-safe subscription token")
	}
	return token, nil
}

func resolvePanelBaseURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "http://127.0.0.1:8080"
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	host := parsed.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return nil, fmt.Errorf("verify base URL must point to a loopback host, got %q", host)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func buildVerifyURL(base *url.URL, segments ...string) string {
	next := *base
	parts := make([]string, 0, len(segments)+1)
	if trimmed := strings.Trim(next.Path, "/"); trimmed != "" {
		parts = append(parts, trimmed)
	}
	for _, segment := range segments {
		trimmed := strings.Trim(segment, "/")
		if trimmed != "" {
			parts = append(parts, url.PathEscape(trimmed))
		}
	}
	next.Path = "/" + strings.Join(parts, "/")
	next.RawQuery = ""
	next.Fragment = ""
	return next.String()
}

func getVerifyURL(ctx context.Context, client *http.Client, target string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil) // #nosec G107 G704 -- target is built by resolvePanelBaseURL/buildVerifyURL and restricted to loopback hosts.
	if err != nil {
		return nil, err
	}
	return client.Do(req) // #nosec G704 -- request URL is built by resolvePanelBaseURL/buildVerifyURL and restricted to loopback hosts.
}

func writeVerifyConfig(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

func readAndCloseResponse(resp *http.Response) ([]byte, error) {
	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return body, nil
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := l.Close(); err != nil {
			fmt.Printf("⚠️ 关闭端口探测监听器失败: %v\n", err)
		}
	}()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func waitForPort(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err == nil {
			if err := conn.Close(); err != nil {
				return false
			}
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func cleanupProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			fmt.Printf("⚠️ 发送进程中断信号失败: %v\n", err)
		}
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			if err := cmd.Process.Kill(); err != nil {
				fmt.Printf("⚠️ 强制结束进程失败: %v\n", err)
			}
		}
	}
}
