package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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