package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ========== Helper Functions ==========
// Note: ptrInt64 and ptrInt are defined in user_test.go

// ========== User Tests ==========
// Note: User tests are in user_test.go

func TestUserTableName(t *testing.T) {
	assert.Equal(t, "v2_user", User{}.TableName())
}

// ========== Node Tests ==========

func TestNodeTableName(t *testing.T) {
	assert.Equal(t, "v2_node", Node{}.TableName())
}

func TestNodeIsOnline(t *testing.T) {
	tests := []struct {
		name     string
		node     Node
		expected bool
	}{
		{
			name:     "no last check",
			node:     Node{},
			expected: false,
		},
		{
			name: "recent heartbeat",
			node: Node{
				LastCheckAt: ptrInt64(time.Now().Unix() - 60), // 1 minute ago
			},
			expected: true,
		},
		{
			name: "old heartbeat",
			node: Node{
				LastCheckAt: ptrInt64(time.Now().Unix() - 600), // 10 minutes ago
			},
			expected: false,
		},
		{
			name: "just now",
			node: Node{
				LastCheckAt: ptrInt64(time.Now().Unix()),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.node.IsOnline()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNodeProtocolTableName(t *testing.T) {
	assert.Equal(t, "v2_node_protocol", NodeProtocol{}.TableName())
}

func TestNodeGroupTableName(t *testing.T) {
	assert.Equal(t, "v2_node_group", NodeGroup{}.TableName())
}

func TestAuthorizedKeyTableName(t *testing.T) {
	assert.Equal(t, "v2_authorized_key", AuthorizedKey{}.TableName())
}

func TestGetProtocolTemplates(t *testing.T) {
	templates := GetProtocolTemplates()
	assert.NotEmpty(t, templates)

	// Check that all expected protocol types are present
	protocolTypes := make(map[ProtocolType]bool)
	for _, tmpl := range templates {
		protocolTypes[tmpl.Type] = true
	}

	assert.True(t, protocolTypes[ProtocolVLESS])
	assert.True(t, protocolTypes[ProtocolVMess])
	assert.True(t, protocolTypes[ProtocolTrojan])
	assert.True(t, protocolTypes[ProtocolShadowsocks])
	assert.True(t, protocolTypes[ProtocolHysteria2])
	assert.True(t, protocolTypes[ProtocolTUIC])
}

func TestProtocolTypeConstants(t *testing.T) {
	assert.Equal(t, ProtocolType("vmess"), ProtocolVMess)
	assert.Equal(t, ProtocolType("vless"), ProtocolVLESS)
	assert.Equal(t, ProtocolType("trojan"), ProtocolTrojan)
	assert.Equal(t, ProtocolType("shadowsocks"), ProtocolShadowsocks)
	assert.Equal(t, ProtocolType("hysteria2"), ProtocolHysteria2)
	assert.Equal(t, ProtocolType("tuic"), ProtocolTUIC)
	assert.Equal(t, ProtocolType("anytls"), ProtocolAnyTLS)
}

func TestNodeStatusConstants(t *testing.T) {
	assert.Equal(t, NodeStatus(0), NodeStatusPending)
	assert.Equal(t, NodeStatus(1), NodeStatusOnline)
	assert.Equal(t, NodeStatus(2), NodeStatusOffline)
	assert.Equal(t, NodeStatus(3), NodeStatusDisabled)
}

// ========== Plan Tests ==========

func TestPlanTableName(t *testing.T) {
	assert.Equal(t, "v2_plan", Plan{}.TableName())
}

// ========== Order Tests ==========

func TestOrderTableName(t *testing.T) {
	assert.Equal(t, "v2_order", Order{}.TableName())
}

// ========== Event Tests ==========

func TestEventTableName(t *testing.T) {
	assert.Equal(t, "v2_event", Event{}.TableName())
}

// ========== Subscription Tests ==========

func TestSubscriptionGroupTableName(t *testing.T) {
	assert.Equal(t, "v2_subscription_group", SubscriptionGroup{}.TableName())
}

func TestSubscriptionTemplateTableName(t *testing.T) {
	assert.Equal(t, "v2_subscription_template", SubscriptionTemplate{}.TableName())
}

func TestUserSubscriptionGroupTableName(t *testing.T) {
	assert.Equal(t, "v2_user_subscription_group", UserSubscriptionGroup{}.TableName())
}

func TestPlanSubscriptionGroupTableName(t *testing.T) {
	assert.Equal(t, "v2_plan_subscription_group", PlanSubscriptionGroup{}.TableName())
}

// ========== Payment Tests ==========

func TestPaymentTableName(t *testing.T) {
	assert.Equal(t, "v2_payment", Payment{}.TableName())
}

func TestPaymentLogTableName(t *testing.T) {
	assert.Equal(t, "v2_payment_log", PaymentLog{}.TableName())
}

// ========== Ticket Tests ==========

func TestTicketTableName(t *testing.T) {
	assert.Equal(t, "v2_ticket", Ticket{}.TableName())
}

func TestTicketMessageTableName(t *testing.T) {
	assert.Equal(t, "v2_ticket_message", TicketMessage{}.TableName())
}

// ========== Coupon Tests ==========

func TestCouponTableName(t *testing.T) {
	assert.Equal(t, "v2_coupon", Coupon{}.TableName())
}

func TestCouponUsageTableName(t *testing.T) {
	assert.Equal(t, "v2_coupon_usage", CouponUsage{}.TableName())
}

// ========== Knowledge Tests ==========

func TestKnowledgeTableName(t *testing.T) {
	assert.Equal(t, "v2_knowledge", Knowledge{}.TableName())
}

// ========== ParsedNode Tests ==========

func TestParsedNodeDefaults(t *testing.T) {
	node := &ParsedNode{
		Name:   "Test Node",
		Type:   "vmess",
		Server: "example.com",
		Port:   443,
	}

	assert.Equal(t, "Test Node", node.Name)
	assert.Equal(t, "vmess", node.Type)
	assert.Equal(t, "example.com", node.Server)
	assert.Equal(t, 443, node.Port)
}

// ========== Forward Model Tests ==========

func TestForwardNodeTableName(t *testing.T) {
	assert.Equal(t, "v2_forward_node", ForwardNode{}.TableName())
}

func TestForwardRuleTableName(t *testing.T) {
	assert.Equal(t, "v2_forward_rule", ForwardRule{}.TableName())
}

func TestForwardRouteTableName(t *testing.T) {
	assert.Equal(t, "v2_forward_route", ForwardRoute{}.TableName())
}

// ========== Payment Gateway Tests ==========

func TestPaymentGatewayTableName(t *testing.T) {
	assert.Equal(t, "v2_payment_gateway", PaymentGateway{}.TableName())
}

// ========== MFA Tests ==========

func TestUserMFATableName(t *testing.T) {
	assert.Equal(t, "v2_user_mfa", UserMFA{}.TableName())
}

func TestMFALoginAttemptTableName(t *testing.T) {
	assert.Equal(t, "v2_mfa_login_attempt", MFALoginAttempt{}.TableName())
}

func TestMFAMethodConstants(t *testing.T) {
	assert.Equal(t, "totp", MFAMethodTOTP)
	assert.Equal(t, "sms", MFAMethodSMS)
	assert.Equal(t, "email", MFAMethodEmail)
	assert.Equal(t, "backup", MFAMethodBackup)
}

// ========== Notification Tests ==========

func TestNotificationTemplateTableName(t *testing.T) {
	assert.Equal(t, "v2_notification_template", NotificationTemplate{}.TableName())
}

func TestNotificationLogTableName(t *testing.T) {
	assert.Equal(t, "v2_notification_log", NotificationLog{}.TableName())
}

// ========== Invite Tests ==========

func TestInviteCodeTableName(t *testing.T) {
	assert.Equal(t, "v2_invite_code", InviteCode{}.TableName())
}

// ========== System Config Tests ==========

func TestSystemConfigTableName(t *testing.T) {
	assert.Equal(t, "v2_system_config", SystemConfig{}.TableName())
}

// ========== Format Constants Tests ==========

func TestSubscriptionFormatConstants(t *testing.T) {
	assert.Equal(t, SubscriptionFormat("v2ray"), FormatV2Ray)
	assert.Equal(t, SubscriptionFormat("clash"), FormatClash)
	assert.Equal(t, SubscriptionFormat("stash"), FormatStash)
	assert.Equal(t, SubscriptionFormat("egern"), FormatEgern)
	assert.Equal(t, SubscriptionFormat("surge"), FormatSurge)
	assert.Equal(t, SubscriptionFormat("loon"), FormatLoon)
	assert.Equal(t, SubscriptionFormat("json"), FormatJSON)
	assert.Equal(t, SubscriptionFormat("base64json"), FormatBase64JSON)
	assert.Equal(t, SubscriptionFormat("shadowrocket"), FormatShadowrocket)
	assert.Equal(t, SubscriptionFormat("quantumultx"), FormatQuantumultX)
	assert.Equal(t, SubscriptionFormat("sing-box"), FormatSingBox)
}

// ========== User Method Tests ==========

func TestUser_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		user     User
		expected bool
	}{
		{
			name:     "valid user",
			user:     User{Banned: 0},
			expected: true,
		},
		{
			name:     "banned user",
			user:     User{Banned: 1},
			expected: false,
		},
		{
			name:     "expired user",
			user:     User{ExpiredAt: ptrInt64(time.Now().Unix() - 3600)},
			expected: false,
		},
		{
			name:     "valid with future expiry",
			user:     User{ExpiredAt: ptrInt64(time.Now().Unix() + 3600)},
			expected: true,
		},
		{
			name:     "no expiry and not banned",
			user:     User{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.user.IsValid()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUser_HasTraffic(t *testing.T) {
	tests := []struct {
		name     string
		user     User
		expected bool
	}{
		{
			name:     "has remaining traffic",
			user:     User{U: 100, D: 100, TransferEnable: 1000},
			expected: true,
		},
		{
			name:     "exactly used all traffic",
			user:     User{U: 500, D: 500, TransferEnable: 1000},
			expected: false,
		},
		{
			name:     "exceeded traffic",
			user:     User{U: 600, D: 600, TransferEnable: 1000},
			expected: false,
		},
		{
			name:     "no traffic used",
			user:     User{U: 0, D: 0, TransferEnable: 1000},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.user.HasTraffic()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUser_GetSpeedLimit(t *testing.T) {
	tests := []struct {
		name     string
		user     User
		expected int64
	}{
		{
			name:     "with speed limit",
			user:     User{SpeedLimit: ptrInt64(1024000)},
			expected: 1024000,
		},
		{
			name:     "nil speed limit",
			user:     User{SpeedLimit: nil},
			expected: 0,
		},
		{
			name:     "zero speed limit",
			user:     User{SpeedLimit: ptrInt64(0)},
			expected: 0,
		},
		{
			name:     "negative speed limit",
			user:     User{SpeedLimit: ptrInt64(-100)},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.user.GetSpeedLimit()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUser_GetDeviceLimit(t *testing.T) {
	tests := []struct {
		name     string
		user     User
		expected int
	}{
		{
			name:     "with device limit",
			user:     User{DeviceLimit: ptrInt(5)},
			expected: 5,
		},
		{
			name:     "nil device limit",
			user:     User{DeviceLimit: nil},
			expected: 0,
		},
		{
			name:     "zero device limit",
			user:     User{DeviceLimit: ptrInt(0)},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.user.GetDeviceLimit()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ========== Server Model Tests ==========

func TestServerTypeConstants(t *testing.T) {
	assert.Equal(t, ServerType("vmess"), ServerTypeVMess)
	assert.Equal(t, ServerType("vless"), ServerTypeVLESS)
	assert.Equal(t, ServerType("trojan"), ServerTypeTrojan)
	assert.Equal(t, ServerType("shadowsocks"), ServerTypeShadowsocks)
	assert.Equal(t, ServerType("hysteria"), ServerTypeHysteria)
	assert.Equal(t, ServerType("hysteria2"), ServerTypeHysteria2)
	assert.Equal(t, ServerType("tuic"), ServerTypeTUIC)
	assert.Equal(t, ServerType("anytls"), ServerTypeAnyTLS)
}

func TestServerVMess_TableName(t *testing.T) {
	assert.Equal(t, "v2_server_vmess", ServerVMess{}.TableName())
}

func TestServerVLESS_TableName(t *testing.T) {
	assert.Equal(t, "v2_server_vless", ServerVLESS{}.TableName())
}

func TestServerTrojan_TableName(t *testing.T) {
	assert.Equal(t, "v2_server_trojan", ServerTrojan{}.TableName())
}

func TestServerShadowsocks_TableName(t *testing.T) {
	assert.Equal(t, "v2_server_shadowsocks", ServerShadowsocks{}.TableName())
}

func TestServerHysteria_TableName(t *testing.T) {
	assert.Equal(t, "v2_server_hysteria", ServerHysteria{}.TableName())
}

func TestServerTUIC_TableName(t *testing.T) {
	assert.Equal(t, "v2_server_tuic", ServerTUIC{}.TableName())
}

func TestServerAnyTLS_TableName(t *testing.T) {
	assert.Equal(t, "v2_server_anytls", ServerAnyTLS{}.TableName())
}

func TestServerGroup_TableName(t *testing.T) {
	assert.Equal(t, "v2_server_group", ServerGroup{}.TableName())
}

func TestServerRoute_TableName(t *testing.T) {
	assert.Equal(t, "v2_server_route", ServerRoute{}.TableName())
}

func TestBaseServer_GetGroupIDs(t *testing.T) {
	tests := []struct {
		name     string
		server   BaseServer
		expected []uint
	}{
		{
			name:     "empty group id",
			server:   BaseServer{GroupID: ""},
			expected: nil,
		},
		{
			name:     "single group id",
			server:   BaseServer{GroupID: "[1]"},
			expected: []uint{1},
		},
		{
			name:     "multiple group ids",
			server:   BaseServer{GroupID: "[1,2,3]"},
			expected: []uint{1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.server.GetGroupIDs()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBaseServer_GetRouteIDs(t *testing.T) {
	tests := []struct {
		name     string
		server   BaseServer
		expected []uint
	}{
		{
			name:     "empty route id",
			server:   BaseServer{RouteID: ""},
			expected: nil,
		},
		{
			name:     "single route id",
			server:   BaseServer{RouteID: "[1]"},
			expected: []uint{1},
		},
		{
			name:     "multiple route ids",
			server:   BaseServer{RouteID: "[1,2,3]"},
			expected: []uint{1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.server.GetRouteIDs()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestServerVMess_Interface(t *testing.T) {
	server := &ServerVMess{
		BaseServer: BaseServer{
			ID:         1,
			Host:       "example.com",
			Port:       443,
			ServerPort: 8443,
		},
		Network: "ws",
	}

	assert.Equal(t, uint(1), server.GetID())
	assert.Equal(t, ServerTypeVMess, server.GetType())
	assert.Equal(t, "example.com", server.GetHost())
	assert.Equal(t, 443, server.GetPort())
	assert.Equal(t, 8443, server.GetServerPort())
}

func TestServerVLESS_Interface(t *testing.T) {
	server := &ServerVLESS{
		BaseServer: BaseServer{
			ID:         2,
			Host:       "vless.example.com",
			Port:       443,
			ServerPort: 8443,
		},
		Network: "tcp",
		Flow:    "xtls-rprx-vision",
	}

	assert.Equal(t, uint(2), server.GetID())
	assert.Equal(t, ServerTypeVLESS, server.GetType())
	assert.Equal(t, "vless.example.com", server.GetHost())
}

func TestServerTrojan_Interface(t *testing.T) {
	server := &ServerTrojan{
		BaseServer: BaseServer{
			ID:         3,
			Host:       "trojan.example.com",
			Port:       443,
			ServerPort: 8443,
		},
		Network: "grpc",
	}

	assert.Equal(t, uint(3), server.GetID())
	assert.Equal(t, ServerTypeTrojan, server.GetType())
	assert.Equal(t, "trojan.example.com", server.GetHost())
}

func TestServerShadowsocks_Interface(t *testing.T) {
	server := &ServerShadowsocks{
		BaseServer: BaseServer{
			ID:         4,
			Host:       "ss.example.com",
			Port:       8388,
			ServerPort: 9388,
		},
		Cipher: "aes-256-gcm",
	}

	assert.Equal(t, uint(4), server.GetID())
	assert.Equal(t, ServerTypeShadowsocks, server.GetType())
	assert.Equal(t, "ss.example.com", server.GetHost())
}

func TestServerHysteria_Interface(t *testing.T) {
	server := &ServerHysteria{
		BaseServer: BaseServer{
			ID:         5,
			Host:       "hy.example.com",
			Port:       443,
			ServerPort: 8443,
		},
		Version: 2,
		UpMbps:  100,
	}

	assert.Equal(t, uint(5), server.GetID())
	assert.Equal(t, ServerTypeHysteria, server.GetType())
	assert.Equal(t, "hy.example.com", server.GetHost())
}

func TestServerTUIC_Interface(t *testing.T) {
	server := &ServerTUIC{
		BaseServer: BaseServer{
			ID:         6,
			Host:       "tuic.example.com",
			Port:       443,
			ServerPort: 8443,
		},
		CongestionControl: "bbr",
	}

	assert.Equal(t, uint(6), server.GetID())
	assert.Equal(t, ServerTypeTUIC, server.GetType())
	assert.Equal(t, "tuic.example.com", server.GetHost())
}

func TestServerAnyTLS_Interface(t *testing.T) {
	server := &ServerAnyTLS{
		BaseServer: BaseServer{
			ID:         7,
			Host:       "anytls.example.com",
			Port:       443,
			ServerPort: 8443,
		},
	}

	assert.Equal(t, uint(7), server.GetID())
	assert.Equal(t, ServerTypeAnyTLS, server.GetType())
	assert.Equal(t, "anytls.example.com", server.GetHost())
}

// ========== Plan Model Tests ==========

func TestPlan_GetSpeedLimit(t *testing.T) {
	speedLimit := int64(1024000)
	tests := []struct {
		name     string
		plan     Plan
		expected int64
	}{
		{
			name:     "with speed limit",
			plan:     Plan{SpeedLimit: &speedLimit},
			expected: 1024000,
		},
		{
			name:     "nil speed limit",
			plan:     Plan{SpeedLimit: nil},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.plan.GetSpeedLimit()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPlan_GetDeviceLimit(t *testing.T) {
	deviceLimit := 5
	tests := []struct {
		name     string
		plan     Plan
		expected int
	}{
		{
			name:     "with device limit",
			plan:     Plan{DeviceLimit: &deviceLimit},
			expected: 5,
		},
		{
			name:     "nil device limit",
			plan:     Plan{DeviceLimit: nil},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.plan.GetDeviceLimit()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ========== Forward Node Type Tests ==========

func TestForwardNodeTypeConstants(t *testing.T) {
	assert.Equal(t, "relay", ForwardNodeTypeRelay)
	assert.Equal(t, "exit", ForwardNodeTypeExit)
}

func TestForwardNodeStatusConstants(t *testing.T) {
	assert.Equal(t, 0, ForwardNodeStatusOffline)
	assert.Equal(t, 1, ForwardNodeStatusOnline)
}

// ========== Payment Gateway Tests ==========

func TestPaymentGatewayTypeConstants(t *testing.T) {
	assert.Equal(t, "alipay", PaymentGatewayAlipay)
	assert.Equal(t, "wechat", PaymentGatewayWechat)
	assert.Equal(t, "epay", PaymentGatewayEPay)
	assert.Equal(t, "usdt", PaymentGatewayUSDT)
}

func TestPaymentRecord_TableName(t *testing.T) {
	assert.Equal(t, "v2_payment_record", PaymentRecord{}.TableName())
}
