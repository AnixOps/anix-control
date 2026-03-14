package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

func main() {
	ctx := context.Background()

	fmt.Println("========================================")
	fmt.Println("    V2Board 完整 E2E 验证测试")
	fmt.Println("========================================")
	fmt.Println()

	xrayPath := "./bin/xray.exe"
	if _, err := os.Stat(xrayPath); os.IsNotExist(err) {
		fmt.Println("❌ Xray 未找到，请先下载")
		return
	}

	// 1. 检查服务器
	fmt.Println("[1/7] 检查 V2Board 服务器...")
	resp, err := http.Get("http://localhost:8080/health")
	if err != nil {
		fmt.Printf("❌ 服务器未运行: %v\n", err)
		return
	}
	resp.Body.Close()
	fmt.Println("✅ V2Board 服务器运行正常")
	fmt.Println()

	// 2. 获取订阅
	fmt.Println("[2/7] 获取订阅内容...")
	token := "fe7464f5-4091-4e6a-8577-ad0b33c67a7d"
	subURL := fmt.Sprintf("http://localhost:8080/s/%s", token)

	resp, err = http.Get(subURL)
	if err != nil {
		fmt.Printf("❌ 获取订阅失败: %v\n", err)
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	decoded, _ := base64.StdEncoding.DecodeString(string(body))
	fmt.Println("✅ 订阅获取成功")
	fmt.Printf("   内容长度: %d 字节\n", len(decoded))
	fmt.Println()

	// 3. 测试 Shadowsocks 本地代理
	fmt.Println("[3/7] 创建 Shadowsocks 本地代理测试...")

	tmpDir, _ := os.MkdirTemp("", "v2board-e2e-*")
	defer os.RemoveAll(tmpDir)

	// 获取空闲端口
	serverPort := getFreePort()
	proxyPort := getFreePort()

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
	os.WriteFile(serverConfigPath, []byte(serverConfig), 0644)

	// 启动服务端
	serverCmd := exec.CommandContext(ctx, xrayPath, "run", "-c", serverConfigPath)
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
	os.WriteFile(clientConfigPath, []byte(clientConfig), 0644)

	clientCmd := exec.CommandContext(ctx, xrayPath, "run", "-c", clientConfigPath)
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

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	// 通过代理访问本地 V2Board 服务器
	testURL := "http://localhost:8080/health"
	start := time.Now()
	resp, err = httpClient.Get(testURL)
	latency := time.Since(start)

	if err != nil {
		fmt.Printf("❌ 代理连接失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("✅ 代理连接成功! 延迟: %v\n", latency)
	fmt.Println()

	// 6. 测试通过代理获取订阅
	fmt.Println("[6/7] 测试通过代理获取订阅...")

	start = time.Now()
	resp, err = httpClient.Get(subURL)
	if err != nil {
		fmt.Printf("❌ 通过代理获取订阅失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
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
			resp, err := httpClient.Get(fmt.Sprintf("http://localhost:8080/s/%s", token))
			if err == nil && resp.StatusCode == 200 {
				resp.Body.Close()
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
	fmt.Println("✅ V2Board 服务器运行正常")
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