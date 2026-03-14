package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anixops/v2board/tests/integration/binary"
	"github.com/anixops/v2board/tests/integration/clients"
	"github.com/anixops/v2board/tests/integration/config"
	"github.com/anixops/v2board/tests/integration/runner"
)

func main() {
	// 命令行参数
	host := flag.String("host", "", "Server host (required)")
	port := flag.Int("port", 443, "Server port")
	protocol := flag.String("protocol", "vless", "Protocol (vmess, vless, trojan, shadowsocks, hysteria2, tuic)")
	transport := flag.String("transport", "tcp", "Transport type (tcp, ws, grpc, h2, quic)")
	tlsType := flag.String("tls", "reality", "TLS type (none, tls, reality)")
	uuid := flag.String("uuid", "", "User UUID (required)")
	email := flag.String("email", "", "User email")
	sni := flag.String("sni", "", "SNI for TLS")
	publicKey := flag.String("public-key", "", "Reality public key")
	shortID := flag.String("short-id", "", "Reality short ID")
	flow := flag.String("flow", "xtls-rprx-vision", "VLESS flow")
	password := flag.String("password", "", "Password (for Trojan/SS)")
	method := flag.String("method", "aes-256-gcm", "SS encryption method")

	timeout := flag.Duration("timeout", 30*time.Second, "Test timeout")
	parallel := flag.Int("parallel", 0, "Run tests in parallel (0 = sequential)")
	configDir := flag.String("config-dir", "./test_configs", "Config output directory")
	output := flag.String("output", "", "Output report file (JSON)")
	client := flag.String("client", "", "Client type (xray, mihomo, or both)")
	downloadBinaries := flag.Bool("download", false, "Auto-download binaries if not found")

	flag.Parse()

	// 初始化二进制管理器（用于自动下载）
	if *downloadBinaries {
		binMgr := binary.NewManager("")
		clients.SetBinaryManager(&binaryAdapter{mgr: binMgr})
	}

	// 验证必需参数
	if *host == "" {
		fmt.Println("Error: -host is required")
		flag.Usage()
		os.Exit(1)
	}
	if *uuid == "" {
		fmt.Println("Error: -uuid is required")
		flag.Usage()
		os.Exit(1)
	}

	// 创建服务端配置
	server := config.ServerConfig{
		Host:      *host,
		Port:      *port,
		Protocol:  config.Protocol(*protocol),
		Transport: config.TransportType(*transport),
		TLSType:   config.TLSType(*tlsType),
		SNI:       *sni,
		PublicKey: *publicKey,
		ShortID:   *shortID,
		Flow:      *flow,
		Password:  *password,
		Method:    *method,
	}

	// 创建用户配置
	user := config.UserConfig{
		UUID:  *uuid,
		Email: *email,
		Flow:  *flow,
	}

	// 过滤测试场景
	var scenarios []config.TestScenario
	if *protocol != "" && *protocol != "all" {
		// 只运行指定协议的场景
		for _, s := range config.DefaultTestScenarios {
			if string(s.Protocol) == *protocol {
				scenarios = append(scenarios, s)
			}
		}
	} else {
		scenarios = config.DefaultTestScenarios
	}

	// 创建运行器选项
	opts := []runner.RunnerOption{
		runner.WithTimeout(*timeout),
		runner.WithConfigDir(*configDir),
		runner.WithScenarios(scenarios),
	}

	if *parallel > 0 {
		opts = append(opts, runner.WithParallel(*parallel))
	}

	// 创建运行器
	r := runner.NewRunner(opts...)

	// 如果指定了特定客户端，修改生成器
	if *client != "" && *client != "both" {
		// 只使用指定的客户端
		generators := map[string]config.Generator{}
		if *client == "xray" {
			generators["xray"] = config.NewXrayGenerator()
		} else if *client == "mihomo" {
			generators["mihomo"] = config.NewMihomoGenerator()
		}
		// 注意: 需要修改 Runner 来支持自定义生成器
	}

	// 设置信号处理
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt signal, stopping...")
		cancel()
	}()

	// 打印测试信息
	fmt.Println("========== Integration Test ==========")
	fmt.Printf("Server: %s:%d\n", server.Host, server.Port)
	fmt.Printf("Protocol: %s\n", server.Protocol)
	fmt.Printf("Transport: %s\n", server.Transport)
	fmt.Printf("TLS: %s\n", server.TLSType)
	fmt.Printf("UUID: %s\n", user.UUID)
	fmt.Printf("Scenarios: %d\n", len(scenarios))
	fmt.Printf("Timeout: %v\n", *timeout)
	fmt.Println("=======================================")
	fmt.Println()

	// 运行测试
	report := r.Run(ctx, server, user)

	// 打印报告
	r.PrintReport()

	// 保存报告
	if *output != "" {
		if err := r.SaveReport(*output); err != nil {
			fmt.Printf("Error saving report: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Report saved to: %s\n", *output)
	}

	// 输出退出码
	if report.FailedTests > 0 {
		os.Exit(1)
	}
}

// printJSON 打印 JSON 格式
func printJSON(v interface{}) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

// binaryAdapter 适配 binary.Manager 到 clients 的接口
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