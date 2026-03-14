package config

// Protocol 协议类型
type Protocol string

const (
	ProtocolVMess       Protocol = "vmess"
	ProtocolVLESS       Protocol = "vless"
	ProtocolTrojan      Protocol = "trojan"
	ProtocolShadowsocks Protocol = "shadowsocks"
	ProtocolHysteria2   Protocol = "hysteria2"
	ProtocolTUIC        Protocol = "tuic"
)

// TransportType 传输层类型
type TransportType string

const (
	TransportTCP       TransportType = "tcp"
	TransportWS        TransportType = "ws"
	TransportGRPC      TransportType = "grpc"
	TransportHTTP2     TransportType = "h2"
	TransportQUIC      TransportType = "quic"
)

// TLSType TLS 类型
type TLSType string

const (
	TLSNone    TLSType = "none"
	TLS        TLSType = "tls"
	TLSReality TLSType = "reality"
)

// ServerConfig 服务端配置
type ServerConfig struct {
	Host     string // 服务器地址
	Port     int    // 服务器端口
	Protocol Protocol

	// TLS 配置
	TLSType   TLSType
	SNI       string // Server Name Indication
	Insecure  bool   // 跳过证书验证

	// Reality 配置
	PublicKey  string
	ShortID    string
	SpiderX    string

	// 传输层配置
	Transport   TransportType
	TransportWS *TransportWSConfig
	TransportGRPC *TransportGRPCConfig

	// 协议特定配置
	UUID       string // VMess/VLESS UUID
	Password   string // Trojan/SS 密码
	Method     string // SS 加密方法
	Flow       string // VLESS flow (xtls-rprx-vision)
}

// TransportWSConfig WebSocket 传输配置
type TransportWSConfig struct {
	Path    string
	Headers map[string]string
}

// TransportGRPCConfig gRPC 传输配置
type TransportGRPCConfig struct {
	ServiceName string
}

// UserConfig 用户配置
type UserConfig struct {
	UUID     string
	Email    string
	AlterID  int // VMess only
	Flow     string // VLESS only
}

// ClientConfig 客户端配置（用于测试）
type ClientConfig struct {
	Name     string
	Server   ServerConfig
	User     UserConfig

	// 测试相关
	TestURLs []string // 测试用的 URL 列表
	Timeout  int      // 超时时间（秒）
}

// TestScenario 测试场景
type TestScenario struct {
	Name        string
	Description string
	Protocol    Protocol
	Transport   TransportType
	TLS         TLSType
	TestURLs    []string
}

// DefaultTestScenarios 默认测试场景
var DefaultTestScenarios = []TestScenario{
	{
		Name:        "vmess-tcp",
		Description: "VMess over TCP",
		Protocol:    ProtocolVMess,
		Transport:   TransportTCP,
		TLS:         TLSNone,
		TestURLs:    []string{"http://www.gstatic.com/generate_204", "https://www.google.com/generate_204"},
	},
	{
		Name:        "vmess-ws-tls",
		Description: "VMess over WebSocket with TLS",
		Protocol:    ProtocolVMess,
		Transport:   TransportWS,
		TLS:         TLS,
		TestURLs:    []string{"http://www.gstatic.com/generate_204", "https://www.google.com/generate_204"},
	},
	{
		Name:        "vless-reality",
		Description: "VLESS with Reality",
		Protocol:    ProtocolVLESS,
		Transport:   TransportTCP,
		TLS:         TLSReality,
		TestURLs:    []string{"http://www.gstatic.com/generate_204", "https://www.google.com/generate_204"},
	},
	{
		Name:        "trojan-tls",
		Description: "Trojan with TLS",
		Protocol:    ProtocolTrojan,
		Transport:   TransportTCP,
		TLS:         TLS,
		TestURLs:    []string{"http://www.gstatic.com/generate_204", "https://www.google.com/generate_204"},
	},
	{
		Name:        "ss-tcp",
		Description: "Shadowsocks over TCP",
		Protocol:    ProtocolShadowsocks,
		Transport:   TransportTCP,
		TLS:         TLSNone,
		TestURLs:    []string{"http://www.gstatic.com/generate_204"},
	},
	{
		Name:        "hysteria2",
		Description: "Hysteria2 over QUIC",
		Protocol:    ProtocolHysteria2,
		Transport:   TransportQUIC,
		TLS:         TLS,
		TestURLs:    []string{"http://www.gstatic.com/generate_204", "https://www.google.com/generate_204"},
	},
}