package service

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuildNodeProtocolConfig_WireGuardDefaults(t *testing.T) {
	settings := `{"cidr":"10.88.0.0/24","server_address":"10.88.0.1/24","server_private_key":"server-private","server_public_key":"server-public"}`
	host := "entry.example.com"
	node := &model.Node{
		ID:        1,
		Name:      "Entry",
		Host:      "node.example.com",
		CreatedAt: time.Now(),
	}
	protocol := &model.NodeProtocol{
		ID:        10,
		NodeID:    1,
		Name:      "WG",
		Type:      model.ProtocolWireGuard,
		Port:      51820,
		Host:      &host,
		Settings:  &settings,
		Transport: stringPtr("udp"),
	}

	config := BuildNodeProtocolConfig(node, protocol)

	require.Equal(t, "wireguard", config["type"])
	require.Equal(t, "wireguard", config["node_type"])
	require.Equal(t, 51820, config["server_port"])
	require.Equal(t, "entry.example.com", config["host"])
	require.Equal(t, "10.88.0.0/24", config["cidr"])
	require.Equal(t, "10.88.0.1/24", config["server_address"])
	require.Equal(t, "server-private", config["server_private_key"])
	require.Equal(t, "server-public", config["server_public_key"])
	require.Equal(t, 1280, config["mtu"])
	require.Equal(t, "quic", config["tunnel_type"])
	require.Equal(t, "udp", config["network"])
	relay, ok := config["relay"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "gost", relay["backend"])
	require.Equal(t, "entry", relay["role"])
	require.Equal(t, 8421, relay["tun_port"])
	require.Equal(t, "172.31.66.2/24", relay["entry_tun_address"])
	require.Equal(t, "172.31.66.1/24", relay["exit_tun_address"])
	require.Equal(t, "/ws", relay["wss_path"])
	require.Equal(t, true, relay["wss_secure"])
}

func TestProtocolWireGuardTemplate(t *testing.T) {
	templates := model.GetProtocolTemplates()
	for _, template := range templates {
		if template.Type == model.ProtocolWireGuard {
			require.Equal(t, 51820, template.DefaultPort)
			require.Contains(t, template.Settings, `"tunnel_type":"quic"`)
			return
		}
	}
	require.Fail(t, "wireguard protocol template missing")
}

func TestEnsureWireGuardPeerSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, EnsureWireGuardPeerSchema(db))
	require.True(t, db.Migrator().HasTable(&model.WireGuardPeer{}))
	require.NoError(t, EnsureNodeRuntimeHealthSchema(db))
	require.True(t, db.Migrator().HasColumn(&model.Node{}, "runtime_healthy"))
	require.True(t, db.Migrator().HasColumn(&model.Node{}, "runtime_error"))
	require.True(t, db.Migrator().HasColumn(&model.Node{}, "runtime_checked_at"))
}

func TestUserDeleteRemovesWireGuardPeers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.WireGuardPeer{}))
	require.NoError(t, db.Create(&model.User{ID: 901, Email: "wireguard-delete@example.com"}).Error)
	require.NoError(t, db.Create(&model.WireGuardPeer{
		NodeProtocolID: 10,
		UserID:         901,
		PeerIP:         "10.99.0.2",
		PrivateKey:     "private",
		PublicKey:      "public",
		PresharedKey:   "psk",
	}).Error)

	svc := &UserService{db: db}
	require.NoError(t, svc.Delete(901))
	var count int64
	require.NoError(t, db.Model(&model.WireGuardPeer{}).Where("user_id = ?", 901).Count(&count).Error)
	require.Zero(t, count)
}

func TestRecordNodeTrafficReportIsConsistent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.User{}, &model.TrafficLog{}, &model.StatServer{}))
	require.NoError(t, db.Create(&model.Node{ID: 902, Name: "wg-entry", Host: "entry.example.com"}).Error)
	require.NoError(t, db.Create(&model.User{ID: 903, Email: "wireguard-traffic@example.com"}).Error)

	svc := &ServerService{db: db}
	require.NoError(t, svc.RecordNodeTrafficReport(model.ServerType("wireguard"), 902, map[uint][2]int64{
		903: {100, 200},
	}, 1.5))

	var user model.User
	require.NoError(t, db.First(&user, 903).Error)
	require.Equal(t, int64(150), user.U)
	require.Equal(t, int64(300), user.D)
	var node model.Node
	require.NoError(t, db.First(&node, 902).Error)
	require.Equal(t, int64(150), node.TotalUpload)
	require.Equal(t, int64(300), node.TotalDownload)
	var logEntry model.TrafficLog
	require.NoError(t, db.Where("user_id = ?", 903).First(&logEntry).Error)
	require.Equal(t, int64(100), logEntry.U)
	require.Equal(t, int64(200), logEntry.D)
	require.InDelta(t, 1.5, logEntry.Rate, 0.0001)
	var stats int64
	require.NoError(t, db.Model(&model.StatServer{}).Where("server_id = ? AND server_type = ?", 902, "wireguard").Count(&stats).Error)
	require.Equal(t, int64(2), stats)
}

func TestWireGuardPeerAllocationAndKeyGeneration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WireGuardPeer{}))

	svc := &SubscriptionService{db: db}
	first, err := svc.GetOrCreateWireGuardPeer(10, 100, "10.99.0.0/29")
	require.NoError(t, err)
	require.Equal(t, "10.99.0.2", first.PeerIP)

	again, err := svc.GetOrCreateWireGuardPeer(10, 100, "10.99.0.0/29")
	require.NoError(t, err)
	require.Equal(t, first.ID, again.ID)
	require.Equal(t, first.PrivateKey, again.PrivateKey)
	require.Equal(t, first.PeerIP, again.PeerIP)

	second, err := svc.GetOrCreateWireGuardPeer(10, 101, "10.99.0.0/29")
	require.NoError(t, err)
	require.Equal(t, "10.99.0.3", second.PeerIP)
	require.NotEqual(t, first.PublicKey, second.PublicKey)

	requireBase64KeyLen(t, first.PrivateKey, 32)
	requireBase64KeyLen(t, first.PublicKey, 32)
	requireBase64KeyLen(t, first.PresharedKey, 32)
}

func TestBuildWireGuardRuntimeUserExtras(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WireGuardPeer{}))

	settings := `{"cidr":"10.77.0.0/29"}`
	protocol := &model.NodeProtocol{
		ID:       12,
		Type:     model.ProtocolWireGuard,
		Settings: &settings,
	}
	users := []*model.User{
		{ID: 201, UUID: "user-a"},
		{ID: 202, UUID: "user-b"},
	}
	svc := &SubscriptionService{db: db}

	extras, err := svc.BuildWireGuardRuntimeUserExtras(protocol, users)
	require.NoError(t, err)
	require.Equal(t, "10.77.0.2", extras[201]["wireguard_peer_ip"])
	require.Equal(t, "10.77.0.3", extras[202]["wireguard_peer_ip"])
	require.NotEmpty(t, extras[201]["wireguard_public_key"])
	require.NotEmpty(t, extras[201]["wireguard_preshared_key"])

	again, err := svc.BuildWireGuardRuntimeUserExtras(protocol, users[:1])
	require.NoError(t, err)
	require.Equal(t, extras[201]["wireguard_peer_ip"], again[201]["wireguard_peer_ip"])
	require.Equal(t, extras[201]["wireguard_public_key"], again[201]["wireguard_public_key"])
}

func TestWireGuardExitDoesNotAllocatePeerCredentials(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WireGuardPeer{}))
	settings := `{"cidr":"10.77.0.0/24","relay":{"role":"exit"}}`
	protocol := &model.NodeProtocol{ID: 55, Type: model.ProtocolWireGuard, Settings: &settings}
	svc := &SubscriptionService{db: db}

	extras, err := svc.BuildWireGuardRuntimeUserExtras(protocol, []*model.User{{ID: 301}})
	require.NoError(t, err)
	require.Empty(t, extras)
	require.True(t, IsWireGuardExitProtocol(protocol))
	entrySettings := `{"cidr":"10.77.0.0/24","relay":{"role":"entry"}}`
	require.False(t, IsWireGuardExitProtocol(&model.NodeProtocol{Type: model.ProtocolWireGuard, Settings: &entrySettings}))
	require.False(t, IsWireGuardExitProtocol(nil))

	var peers int64
	require.NoError(t, db.Model(&model.WireGuardPeer{}).Count(&peers).Error)
	require.Zero(t, peers)
}

func TestPreferNativeWireGuardFormatForLinkOnlyClients(t *testing.T) {
	onlyWireGuard := []*model.ParsedNode{{Type: string(model.ProtocolWireGuard)}}
	for _, format := range []model.SubscriptionFormat{model.FormatV2Ray, model.FormatShadowrocket, model.FormatLoon} {
		require.Equal(t, model.FormatWireGuard, preferNativeWireGuardFormat(format, onlyWireGuard))
	}

	mixed := []*model.ParsedNode{{Type: string(model.ProtocolWireGuard)}, {Type: string(model.ProtocolVLESS)}}
	require.Equal(t, model.FormatShadowrocket, preferNativeWireGuardFormat(model.FormatShadowrocket, mixed))
	require.Equal(t, model.FormatSingBox, preferNativeWireGuardFormat(model.FormatSingBox, onlyWireGuard))
}

func TestValidateWireGuardProtocolRejectsUnsafeRuntimeSettings(t *testing.T) {
	privateKey, publicKey, err := generateWireGuardKeypair()
	require.NoError(t, err)
	settings := fmt.Sprintf(`{"cidr":"10.77.0.0/24","server_address":"10.77.0.1/24","server_private_key":%q,"server_public_key":%q,"relay":{"backend":"gost","role":"entry","server":"exit.example.com","server_port":8443,"entry_tun_address":"172.31.77.2/24","exit_tun_address":"172.31.77.1/24"}}`, privateKey, publicKey)
	protocol := &model.NodeProtocol{Type: model.ProtocolWireGuard, Port: 51820, Enable: 1, Settings: &settings}
	require.NoError(t, ValidateNodeProtocol(protocol))

	bad := `{"cidr":"10.77.0.0/33","server_address":"10.77.0.1/24"}`
	protocol.Settings = &bad
	require.ErrorIs(t, ValidateNodeProtocol(protocol), ErrInvalidNodeProtocol)
}

func TestValidateWireGuardExitCanOmitEntryKeyMaterial(t *testing.T) {
	settings := `{"cidr":"10.77.0.0/24","mtu":1280,"relay":{"backend":"gost","role":"exit","server_port":8443,"entry_tun_address":"172.31.77.2/24","exit_tun_address":"172.31.77.1/24"}}`
	protocol := &model.NodeProtocol{Type: model.ProtocolWireGuard, Enable: 1, Settings: &settings}
	require.NoError(t, ValidateNodeProtocol(protocol))

	leakedEntryPrivateKey := strings.Replace(settings, `"mtu":1280,`, `"server_private_key":"entry-private-key","mtu":1280,`, 1)
	protocol.Settings = &leakedEntryPrivateKey
	require.ErrorIs(t, ValidateNodeProtocol(protocol), ErrInvalidNodeProtocol)
	protocol.Settings = &settings

	config := BuildNodeProtocolConfig(&model.Node{ID: 1, Host: "exit.example.com"}, protocol)
	require.Empty(t, config["server_private_key"])
	require.Empty(t, config["server_public_key"])
	require.Empty(t, config["server_address"])
}

func TestValidateWireGuardWSSCertificateContract(t *testing.T) {
	privateKey, publicKey, err := generateWireGuardKeypair()
	require.NoError(t, err)
	entrySettings := fmt.Sprintf(`{"cidr":"10.77.0.0/24","server_address":"10.77.0.1/24","server_private_key":%q,"server_public_key":%q,"tunnel_type":"wss","relay":{"backend":"gost","mode":"relay+wss","role":"entry","wss_compat":true,"wss_path":"/wireguard","wss_secure":true,"wss_server_name":"exit.example.com","wss_ca_file":"/etc/v2bx/relay-ca.pem","server":"exit.example.com","server_port":8443,"entry_tun_address":"172.31.77.2/24","exit_tun_address":"172.31.77.1/24"}}`, privateKey, publicKey)
	entry := &model.NodeProtocol{Type: model.ProtocolWireGuard, Port: 51820, Enable: 1, Settings: &entrySettings}
	require.NoError(t, ValidateNodeProtocol(entry))

	insecureEntry := strings.Replace(entrySettings, `"wss_secure":true`, `"wss_secure":false`, 1)
	entry.Settings = &insecureEntry
	require.ErrorIs(t, ValidateNodeProtocol(entry), ErrInvalidNodeProtocol)

	leakedEntry := strings.Replace(entrySettings, `"wss_ca_file":`, `"wss_cert_file":"/etc/v2bx/relay-cert.pem","wss_ca_file":`, 1)
	entry.Settings = &leakedEntry
	require.ErrorIs(t, ValidateNodeProtocol(entry), ErrInvalidNodeProtocol)

	exitSettings := `{"cidr":"10.77.0.0/24","tunnel_type":"wss","relay":{"backend":"gost","mode":"relay+wss","role":"exit","wss_compat":true,"wss_path":"/wireguard","wss_cert_file":"/etc/v2bx/relay-cert.pem","wss_key_file":"/etc/v2bx/relay-key.pem","server_port":8443,"entry_tun_address":"172.31.77.2/24","exit_tun_address":"172.31.77.1/24"}}`
	exit := &model.NodeProtocol{Type: model.ProtocolWireGuard, Enable: 1, Settings: &exitSettings}
	require.NoError(t, ValidateNodeProtocol(exit))

	leakedExit := strings.Replace(exitSettings, `"wss_cert_file":`, `"wss_server_name":"exit.example.com","wss_cert_file":`, 1)
	exit.Settings = &leakedExit
	require.ErrorIs(t, ValidateNodeProtocol(exit), ErrInvalidNodeProtocol)

	missingKey := strings.Replace(exitSettings, `"wss_key_file":"/etc/v2bx/relay-key.pem",`, "", 1)
	exit.Settings = &missingKey
	require.ErrorIs(t, ValidateNodeProtocol(exit), ErrInvalidNodeProtocol)
}

func TestBuildNodeProtocolConfigSeparatesWireGuardWSSRoleSecrets(t *testing.T) {
	entrySettings := `{"tunnel_type":"wss","relay":{"role":"entry","wss_cert_file":"/etc/v2bx/exit-cert.pem","wss_key_file":"/etc/v2bx/exit-key.pem"}}`
	entry := BuildNodeProtocolConfig(&model.Node{ID: 1, Host: "entry.example.com"}, &model.NodeProtocol{
		Type:     model.ProtocolWireGuard,
		Port:     51820,
		Settings: &entrySettings,
	})
	entryRelay := entry["relay"].(map[string]any)
	require.Empty(t, entryRelay["wss_cert_file"])
	require.Empty(t, entryRelay["wss_key_file"])

	exitSettings := `{"tunnel_type":"wss","relay":{"role":"exit","wss_secure":true,"wss_server_name":"exit.example.com","wss_ca_file":"/etc/v2bx/relay-ca.pem","wss_cert_file":"/etc/v2bx/exit-cert.pem","wss_key_file":"/etc/v2bx/exit-key.pem"}}`
	exit := BuildNodeProtocolConfig(&model.Node{ID: 2, Host: "exit.example.com"}, &model.NodeProtocol{
		Type:     model.ProtocolWireGuard,
		Settings: &exitSettings,
	})
	exitRelay := exit["relay"].(map[string]any)
	require.Equal(t, false, exitRelay["wss_secure"])
	require.Empty(t, exitRelay["wss_server_name"])
	require.Empty(t, exitRelay["wss_ca_file"])
	require.Equal(t, "/etc/v2bx/exit-cert.pem", exitRelay["wss_cert_file"])
	require.Equal(t, "/etc/v2bx/exit-key.pem", exitRelay["wss_key_file"])
}

func TestBuildNodeProtocolConfigIgnoresLegacyWireGuardCustomRuntimeOverride(t *testing.T) {
	settings := `{"cidr":"10.77.0.0/24","relay":{"role":"exit","server_port":8443,"entry_tun_address":"172.31.77.2/24","exit_tun_address":"172.31.77.1/24"}}`
	custom := `{"type":"vless","node_type":"vless","server_port":51820,"server_private_key":"leaked-entry-key","relay":{"role":"entry"},"legacy_note":"preserved"}`
	config := BuildNodeProtocolConfig(&model.Node{ID: 3, Host: "exit.example.com"}, &model.NodeProtocol{
		Type:         model.ProtocolWireGuard,
		Settings:     &settings,
		CustomConfig: &custom,
	})

	require.Equal(t, "wireguard", config["type"])
	require.Equal(t, "wireguard", config["node_type"])
	require.Zero(t, config["server_port"])
	require.Empty(t, config["server_private_key"])
	require.Equal(t, "exit", config["relay"].(map[string]any)["role"])
	require.Equal(t, "preserved", config["legacy_note"])
}

func TestValidateWireGuardRuntimeConfig(t *testing.T) {
	privateKey, publicKey, err := generateWireGuardKeypair()
	require.NoError(t, err)
	config := map[string]any{
		"type":               "wireguard",
		"node_type":          "wireguard",
		"server_port":        51820,
		"cidr":               "10.77.0.0/24",
		"server_address":     "10.77.0.1/24",
		"server_private_key": privateKey,
		"server_public_key":  publicKey,
		"allowed_ips":        []any{"0.0.0.0/0"},
		"relay": map[string]any{
			"backend":           "gost",
			"role":              "entry",
			"server":            "exit.example.com",
			"server_port":       8443,
			"entry_tun_address": "172.31.77.2/24",
			"exit_tun_address":  "172.31.77.1/24",
		},
	}
	require.NoError(t, ValidateWireGuardRuntimeConfig(config))

	exitConfig := map[string]any{
		"type":               "wireguard",
		"node_type":          "wireguard",
		"server_port":        0,
		"cidr":               "10.77.0.0/24",
		"mtu":                1280,
		"tunnel_type":        "quic",
		"server_address":     "",
		"server_private_key": "",
		"server_public_key":  "",
		"relay": map[string]any{
			"backend":           "gost",
			"role":              "exit",
			"server_port":       8443,
			"tun_port":          8421,
			"entry_tun_address": "172.31.77.2/24",
			"exit_tun_address":  "172.31.77.1/24",
		},
	}
	require.NoError(t, ValidateWireGuardRuntimeConfig(exitConfig))
	exitConfig["server_private_key"] = privateKey
	require.Error(t, ValidateWireGuardRuntimeConfig(exitConfig))

	config["allowed_ips"] = []any{"::/0"}
	require.Error(t, ValidateWireGuardRuntimeConfig(config))
	config["allowed_ips"] = []any{"0.0.0.0/0"}
	config["node_type"] = "wireguard"
	config["type"] = "vless"
	require.Error(t, ValidateWireGuardRuntimeConfig(config))
}

func TestValidateWireGuardProtocolRejectsCustomRuntimeOverride(t *testing.T) {
	settings := `{"cidr":"10.77.0.0/24","server_address":"10.77.0.1/24","server_private_key":"invalid","relay":{"backend":"gost","role":"entry","server":"exit.example.com","server_port":8443,"entry_tun_address":"172.31.77.2/24","exit_tun_address":"172.31.77.1/24"}}`
	custom := `{"relay":{"role":"exit"}}`
	protocol := &model.NodeProtocol{Type: model.ProtocolWireGuard, Port: 51820, Enable: 1, Settings: &settings, CustomConfig: &custom}
	require.Error(t, ValidateNodeProtocol(protocol))
}

func TestWireGuardPeerAllocationSupportsIPv6AndCIDRMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WireGuardPeer{}))

	svc := &SubscriptionService{db: db}
	peer, err := svc.GetOrCreateWireGuardPeer(20, 300, "fd00::/120")
	require.NoError(t, err)
	require.Equal(t, "fd00::2", peer.PeerIP)

	migrated, err := svc.GetOrCreateWireGuardPeer(20, 300, "fd01::/120")
	require.NoError(t, err)
	require.Equal(t, peer.ID, migrated.ID)
	require.Equal(t, "fd01::2", migrated.PeerIP)
}

func TestBuildNodeProtocolConfigDerivesWireGuardServerPublicKey(t *testing.T) {
	privateKey, publicKey, err := generateWireGuardKeypair()
	require.NoError(t, err)
	settings := fmt.Sprintf(`{"server_private_key":%q}`, privateKey)
	config := BuildNodeProtocolConfig(&model.Node{ID: 1, Host: "entry.example.com"}, &model.NodeProtocol{
		Type:     model.ProtocolWireGuard,
		Port:     51820,
		Settings: &settings,
	})
	require.Equal(t, publicKey, config["server_public_key"])
}

func TestWireGuardSubscriptionDerivesServerPublicKeyFromPrivateKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WireGuardPeer{}))
	svc := &SubscriptionService{db: db}
	privateKey, publicKey, err := generateWireGuardKeypair()
	require.NoError(t, err)
	settings := fmt.Sprintf(`{"cidr":"10.66.0.0/24","server_private_key":%q}`, privateKey)
	protocol := &model.NodeProtocol{ID: 44, Type: model.ProtocolWireGuard, Port: 51820, Settings: &settings}

	parsed := &model.ParsedNode{Type: "wireguard", Settings: map[string]any{
		"server_private_key": privateKey,
	}}
	ctx := &model.TemplateRenderContext{UserID: 901}
	svc.applyWireGuardPeer(parsed, protocol, ctx)
	require.Equal(t, publicKey, parsed.PublicKey)
}

func requireBase64KeyLen(t *testing.T, key string, want int) {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(key)
	require.NoError(t, err)
	require.Len(t, decoded, want)
}

func stringPtr(v string) *string {
	return &v
}
