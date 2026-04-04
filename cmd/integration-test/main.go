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

	"github.com/anixops/v2board/internal/tests/integration/binary"
	"github.com/anixops/v2board/internal/tests/integration/clients"
	"github.com/anixops/v2board/internal/tests/integration/config"
	"github.com/anixops/v2board/internal/tests/integration/runner"
)

func main() {
	// 鍛戒护琛屽弬鏁?
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

	// 鍒濆鍖栦簩杩涘埗绠＄悊鍣紙鐢ㄤ簬鑷姩涓嬭浇锛?
	if *downloadBinaries {
		binMgr := binary.NewManager("")
		clients.SetBinaryManager(&binaryAdapter{mgr: binMgr})
	}

	// 楠岃瘉蹇呴渶鍙傛暟
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

	// 鍒涘缓鏈嶅姟绔厤缃?
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

	// 鍒涘缓鐢ㄦ埛閰嶇疆
	user := config.UserConfig{
		UUID:  *uuid,
		Email: *email,
		Flow:  *flow,
	}

	// 杩囨护娴嬭瘯鍦烘櫙
	var scenarios []config.TestScenario
	if *protocol != "" && *protocol != "all" {
		// 鍙繍琛屾寚瀹氬崗璁殑鍦烘櫙
		for _, s := range config.DefaultTestScenarios {
			if string(s.Protocol) == *protocol {
				scenarios = append(scenarios, s)
			}
		}
	} else {
		scenarios = config.DefaultTestScenarios
	}

	// 鍒涘缓杩愯鍣ㄩ€夐」
	opts := []runner.RunnerOption{
		runner.WithTimeout(*timeout),
		runner.WithConfigDir(*configDir),
		runner.WithScenarios(scenarios),
	}

	if *parallel > 0 {
		opts = append(opts, runner.WithParallel(*parallel))
	}

	// 鍒涘缓杩愯鍣?
	r := runner.NewRunner(opts...)

	// 濡傛灉鎸囧畾浜嗙壒瀹氬鎴风锛屼慨鏀圭敓鎴愬櫒
	if *client != "" && *client != "both" {
		// 鍙娇鐢ㄦ寚瀹氱殑瀹㈡埛绔?
		generators := map[string]config.Generator{}
		if *client == "xray" {
			generators["xray"] = config.NewXrayGenerator()
		} else if *client == "mihomo" {
			generators["mihomo"] = config.NewMihomoGenerator()
		}
		// 娉ㄦ剰: 闇€瑕佷慨鏀?Runner 鏉ユ敮鎸佽嚜瀹氫箟鐢熸垚鍣?
	}

	// 璁剧疆淇″彿澶勭悊
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt signal, stopping...")
		cancel()
	}()

	// 鎵撳嵃娴嬭瘯淇℃伅
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

	// 杩愯娴嬭瘯
	report := r.Run(ctx, server, user)

	// 鎵撳嵃鎶ュ憡
	r.PrintReport()

	// 淇濆瓨鎶ュ憡
	if *output != "" {
		if err := r.SaveReport(*output); err != nil {
			fmt.Printf("Error saving report: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Report saved to: %s\n", *output)
	}

	// 杈撳嚭閫€鍑虹爜
	if report.FailedTests > 0 {
		os.Exit(1)
	}
}

// printJSON 鎵撳嵃 JSON 鏍煎紡
func printJSON(v interface{}) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

// binaryAdapter 閫傞厤 binary.Manager 鍒?clients 鐨勬帴鍙?
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
