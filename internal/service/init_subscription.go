package service

import (
	"log"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
)

// InitSubscriptionDefaults 初始化默认订阅分组和模板
func InitSubscriptionDefaults() {
	db := database.Get()

	// 检查是否已有分组
	var count int64
	db.Model(&model.SubscriptionGroup{}).Count(&count)
	if count > 0 {
		return // 已有数据，不重复初始化
	}

	log.Println("Initializing default subscription groups...")

	// 创建默认分组
	defaultGroup := &model.SubscriptionGroup{
		Name:        "default",
		Description: strPtr("默认订阅分组，所有用户可见"),
		Priority:    0,
		Enable:      1,
	}

	if err := db.Create(defaultGroup).Error; err != nil {
		log.Printf("Failed to create default subscription group: %v", err)
		return
	}

	// 创建 VIP 分组示例
	vipGroup := &model.SubscriptionGroup{
		Name:        "vip",
		Description: strPtr("VIP 订阅分组，需要购买特定套餐或手动分配"),
		Priority:    10,
		Enable:      1,
	}

	if err := db.Create(vipGroup).Error; err != nil {
		log.Printf("Failed to create VIP subscription group: %v", err)
	}

	// 创建示例模板 - VLESS Reality
	// 符合 V2bX 协议配置规范
	vlessTemplate := &model.SubscriptionTemplate{
		GroupID:          defaultGroup.ID,
		Name:             "示例节点 - VLESS Reality",
		Type:             "vless",
		Enable:           0, // 默认禁用，需要管理员配置后启用
		Sort:             0,
		Server:           "your-server.example.com",
		Port:             443,
		ServerName:       strPtr("www.microsoft.com"),
		TLS:              2, // Reality
		TLSFingerprint:   strPtr("chrome"),
		Transport:        "tcp",
		Flow:             strPtr("xtls-rprx-vision"), // 使用新字段
		RealityPublicKey: strPtr("your-reality-public-key"),
		RealityShortID:   strPtr(""),
		TemplateJSON:     "", // 使用默认配置
	}

	if err := db.Create(vlessTemplate).Error; err != nil {
		log.Printf("Failed to create VLESS template: %v", err)
	}

	// 初始化默认套餐
	InitDefaultPlan()

	// 创建示例模板 - VMess WS
	vmessTemplate := &model.SubscriptionTemplate{
		GroupID:           defaultGroup.ID,
		Name:              "示例节点 - VMess WebSocket",
		Type:              "vmess",
		Enable:            0,
		Sort:              1,
		Server:            "your-server.example.com",
		Port:              443,
		ServerName:        strPtr("your-server.example.com"),
		TLS:               1,
		Transport:         "ws",
		TransportSettings: strPtr(`{"path":"/ws","host":"your-server.example.com"}`),
		TemplateJSON:      "",
	}

	if err := db.Create(vmessTemplate).Error; err != nil {
		log.Printf("Failed to create VMess template: %v", err)
	}

	// 创建示例模板 - Trojan
	trojanTemplate := &model.SubscriptionTemplate{
		GroupID:    defaultGroup.ID,
		Name:       "示例节点 - Trojan",
		Type:       "trojan",
		Enable:     0,
		Sort:       2,
		Server:     "your-server.example.com",
		Port:       443,
		ServerName: strPtr("your-server.example.com"),
		TLS:        1,
		Transport:  "tcp",
	}

	if err := db.Create(trojanTemplate).Error; err != nil {
		log.Printf("Failed to create Trojan template: %v", err)
	}

	// 创建 Hysteria2 示例模板 (VIP 分组)
	hy2Template := &model.SubscriptionTemplate{
		GroupID:          vipGroup.ID,
		Name:             "VIP 节点 - Hysteria2",
		Type:             "hysteria2",
		Enable:           0,
		Sort:             0,
		Server:           "hy2-server.example.com",
		Port:             443,
		ServerName:       strPtr("hy2-server.example.com"),
		TLS:              1,
		ProtocolSettings: strPtr(`{"obfs":"salamander","obfs-password":"your-obfs-password"}`),
	}

	if err := db.Create(hy2Template).Error; err != nil {
		log.Printf("Failed to create Hysteria2 template: %v", err)
	}

	// 创建测试分组 - 包含本地测试节点
	// 符合 V2bX/Xray 协议配置规范
	testGroup := &model.SubscriptionGroup{
		Name:        "test",
		Description: strPtr("测试分组 - 包含本地测试节点，仅管理员可见"),
		Priority:    100, // 最高优先级，排在最前面
		Enable:      1,
	}

	if err := db.Create(testGroup).Error; err != nil {
		log.Printf("Failed to create test subscription group: %v", err)
	} else {
		// 创建本地测试节点 - VLESS TCP (无 TLS)
		// 对应 V2bX: tls=0, network=tcp
		localTest := &model.SubscriptionTemplate{
			GroupID:   testGroup.ID,
			Name:      "本地test",
			Type:      "vless",
			Enable:    1, // 测试节点默认启用
			Sort:      0,
			Server:    "127.0.0.1",
			Port:      19999,
			TLS:       0, // 无 TLS (security=none)
			Transport: "tcp",
		}
		db.Create(localTest)

		// 美国节点 - VLESS Reality
		// 对应 V2bX: tls=2, network=tcp, flow=xtls-rprx-vision
		usReality := &model.SubscriptionTemplate{
			GroupID:          testGroup.ID,
			Name:             "🇺🇸 美国节点-Reality",
			Type:             "vless",
			Enable:           1,
			Sort:             1,
			Server:           "us.example.com",
			Port:             443,
			ServerName:       strPtr("www.microsoft.com"),
			TLS:              2, // Reality
			TLSFingerprint:   strPtr("chrome"),
			Transport:        "tcp",
			Flow:             strPtr("xtls-rprx-vision"),
			RealityPublicKey: strPtr("your-public-key-here"),
			RealityShortID:   strPtr("abcd1234"),
		}
		db.Create(usReality)

		// 日本节点 - VLESS WebSocket + TLS
		// 对应 V2bX: tls=1, network=ws
		jpWs := &model.SubscriptionTemplate{
			GroupID:           testGroup.ID,
			Name:              "🇯🇵 日本节点-WS",
			Type:              "vless",
			Enable:            1,
			Sort:              2,
			Server:            "jp.example.com",
			Port:              443,
			ServerName:        strPtr("jp.example.com"),
			TLS:               1, // TLS
			TLSFingerprint:    strPtr("chrome"),
			Transport:         "ws",
			TransportSettings: strPtr(`{"path":"/ws","headers":{"Host":"jp.example.com"}}`),
		}
		db.Create(jpWs)

		// VIP 香港节点 - Hysteria2
		// Hysteria2 强制 TLS
		hkHy2 := &model.SubscriptionTemplate{
			GroupID:    testGroup.ID,
			Name:       "🇭🇰 VIP香港-Hysteria2",
			Type:       "hysteria2",
			Enable:     1,
			Sort:       3,
			Server:     "hk.example.com",
			Port:       443,
			ServerName: strPtr("hk.example.com"),
			TLS:        1, // Hysteria2 强制 TLS
		}
		db.Create(hkHy2)

		// Shadowsocks 2022 测试节点
		ssSingapore := &model.SubscriptionTemplate{
			GroupID:     testGroup.ID,
			Name:        "🇸🇬 新加坡-SS2022",
			Type:        "shadowsocks",
			Enable:      1,
			Sort:        4,
			Server:      "sg.example.com",
			Port:        8388,
			TLS:         0,
			SSCipher:    strPtr("2022-blake3-aes-256-gcm"),
			SSServerKey: strPtr("your-ss2022-server-key-base64"),
		}
		db.Create(ssSingapore)

		// Trojan 测试节点
		trojanTw := &model.SubscriptionTemplate{
			GroupID:        testGroup.ID,
			Name:           "🇹🇼 台湾-Trojan",
			Type:           "trojan",
			Enable:         1,
			Sort:           5,
			Server:         "tw.example.com",
			Port:           443,
			ServerName:     strPtr("tw.example.com"),
			TLS:            1, // Trojan 强制 TLS
			TLSFingerprint: strPtr("chrome"),
			Transport:      "tcp",
		}
		db.Create(trojanTw)

		log.Printf("Created test subscription group: %s (ID:%d) with 6 test nodes", testGroup.Name, testGroup.ID)
	}

	log.Printf("Created default subscription groups: %s (ID:%d), %s (ID:%d)",
		defaultGroup.Name, defaultGroup.ID, vipGroup.Name, vipGroup.ID)
	log.Println("Created sample subscription templates (disabled by default)")
	log.Println("Please configure your actual servers and enable templates in admin panel")
}

func strPtr(s string) *string {
	return &s
}

// InitDefaultPlan 确保数据库中有一个默认套餐（如“基础套餐”）
func InitDefaultPlan() {
	db := database.Get()
	var count int64
	db.Model(&model.Plan{}).Count(&count)
	if count > 0 {
		return // 已有套餐，不重复初始化
	}

	log.Println("Initializing default plan...")

	// 查找 default 分组
	var group model.SubscriptionGroup
	if err := db.Where("name = ?", "default").First(&group).Error; err != nil {
		log.Printf("Failed to find default subscription group for plan: %v", err)
		return
	}

	plan := &model.Plan{
		GroupID:        group.ID,
		Name:           "基础套餐",
		TransferEnable: 100 * 1024 * 1024 * 1024, // 100GB
		MonthPrice:     ptrInt64(1200),           // 单位：分（12元）
		Show:           1,
		Renew:          1,
		Sort:           ptrInt(0),
	}
	if err := db.Create(plan).Error; err != nil {
		log.Printf("Failed to create default plan: %v", err)
	}
}

func ptrInt64(v int64) *int64 { return &v }
func ptrInt(v int) *int       { return &v }
