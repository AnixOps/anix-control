package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/anixops/v2board/internal/tests/integration/config"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// 鍒涘缓鏈嶅姟绔厤缃?
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

	// 鍒涘缓鐢ㄦ埛閰嶇疆
	user := config.UserConfig{
		UUID:  "your-uuid-here",
		Email: "test@example.com",
	}

	// 鍒涘缓瀹㈡埛绔厤缃?
	client := &config.ClientConfig{
		Name:     "test-vless-reality",
		Server:   server,
		User:     user,
		TestURLs: []string{"http://www.gstatic.com/generate_204"},
		Timeout:  10,
	}

	// 鐢熸垚 Xray 閰嶇疆
	xrayGen := config.NewXrayGenerator()
	xrayConfig, err := xrayGen.Generate(client)
	if err != nil {
		return err
	}
	fmt.Println("=== Xray Config (JSON) ===")
	fmt.Println(string(xrayConfig))

	// 鐢熸垚 Mihomo 閰嶇疆
	mihomoGen := config.NewMihomoGenerator()
	mihomoConfig, err := mihomoGen.Generate(client)
	if err != nil {
		return err
	}
	fmt.Println("\n=== Mihomo Config (YAML) ===")
	fmt.Println(string(mihomoConfig))

	// 鏄剧ず娴嬭瘯鍦烘櫙
	fmt.Println("\n=== Available Test Scenarios ===")
	for _, scenario := range config.DefaultTestScenarios {
		fmt.Printf("- %s: %s (Protocol: %s, TLS: %s)\n",
			scenario.Name, scenario.Description, scenario.Protocol, scenario.TLS)
	}

	// 淇濆瓨鍒版枃浠?
	if err := saveGeneratedConfigs("output", xrayConfig, mihomoConfig); err != nil {
		return err
	}
	fmt.Println("\n鉁?閰嶇疆宸蹭繚瀛樺埌 output/ 鐩綍")
	return nil
}

func saveGeneratedConfigs(outputDir string, xrayConfig, mihomoConfig []byte) error {
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "xray-config.json"), xrayConfig, 0o600); err != nil {
		return fmt.Errorf("write xray config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "mihomo-config.yaml"), mihomoConfig, 0o600); err != nil {
		return fmt.Errorf("write mihomo config: %w", err)
	}
	return nil
}
