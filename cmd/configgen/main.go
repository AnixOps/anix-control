package main

import (
	"fmt"
	"log"
	"os"

	"github.com/anixops/v2board/tests/integration/config"
)

func main() {
	// 创建服务端配置
	server := config.ServerConfig{
		Host:      "example.com",
		Port:      443,
		Protocol:  config.ProtocolVLESS,
		TLSType:   config.TLSReality,
		SNI:       "www.google.com",
		PublicKey: "your-public-key",
		ShortID:   "your-short-id",
		Flow:      "xtls-rprx-vision",
	}

	// 创建用户配置
	user := config.UserConfig{
		UUID:  "your-uuid-here",
		Email: "test@example.com",
	}

	// 创建客户端配置
	client := &config.ClientConfig{
		Name:     "test-vless-reality",
		Server:   server,
		User:     user,
		TestURLs: []string{"http://www.gstatic.com/generate_204"},
		Timeout:  10,
	}

	// 生成 Xray 配置
	xrayGen := config.NewXrayGenerator()
	xrayConfig, err := xrayGen.Generate(client)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("=== Xray Config (JSON) ===")
	fmt.Println(string(xrayConfig))

	// 生成 Mihomo 配置
	mihomoGen := config.NewMihomoGenerator()
	mihomoConfig, err := mihomoGen.Generate(client)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\n=== Mihomo Config (YAML) ===")
	fmt.Println(string(mihomoConfig))

	// 显示测试场景
	fmt.Println("\n=== Available Test Scenarios ===")
	for _, scenario := range config.DefaultTestScenarios {
		fmt.Printf("- %s: %s (Protocol: %s, TLS: %s)\n",
			scenario.Name, scenario.Description, scenario.Protocol, scenario.TLS)
	}

	// 保存到文件
	os.MkdirAll("output", 0755)
	os.WriteFile("output/xray-config.json", xrayConfig, 0644)
	os.WriteFile("output/mihomo-config.yaml", mihomoConfig, 0644)
	fmt.Println("\n✅ 配置已保存到 output/ 目录")
}