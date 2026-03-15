package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ========== Server GetPort/GetServerPort Tests ==========

func TestServerVMess_GetPort(t *testing.T) {
	server := &ServerVMess{
		BaseServer: BaseServer{
			Port:       443,
			ServerPort: 8443,
		},
	}
	assert.Equal(t, 443, server.GetPort())
	assert.Equal(t, 8443, server.GetServerPort())
}

func TestServerVLESS_GetPort(t *testing.T) {
	server := &ServerVLESS{
		BaseServer: BaseServer{
			Port:       443,
			ServerPort: 8443,
		},
	}
	assert.Equal(t, 443, server.GetPort())
	assert.Equal(t, 8443, server.GetServerPort())
}

func TestServerTrojan_GetPort(t *testing.T) {
	server := &ServerTrojan{
		BaseServer: BaseServer{
			Port:       443,
			ServerPort: 8443,
		},
	}
	assert.Equal(t, 443, server.GetPort())
	assert.Equal(t, 8443, server.GetServerPort())
}

func TestServerShadowsocks_GetPort(t *testing.T) {
	server := &ServerShadowsocks{
		BaseServer: BaseServer{
			Port:       8388,
			ServerPort: 9388,
		},
	}
	assert.Equal(t, 8388, server.GetPort())
	assert.Equal(t, 9388, server.GetServerPort())
}

func TestServerHysteria_GetPort(t *testing.T) {
	server := &ServerHysteria{
		BaseServer: BaseServer{
			Port:       443,
			ServerPort: 8443,
		},
	}
	assert.Equal(t, 443, server.GetPort())
	assert.Equal(t, 8443, server.GetServerPort())
}

func TestServerTUIC_GetPort(t *testing.T) {
	server := &ServerTUIC{
		BaseServer: BaseServer{
			Port:       443,
			ServerPort: 8443,
		},
	}
	assert.Equal(t, 443, server.GetPort())
	assert.Equal(t, 8443, server.GetServerPort())
}

func TestServerAnyTLS_GetPort(t *testing.T) {
	server := &ServerAnyTLS{
		BaseServer: BaseServer{
			Port:       443,
			ServerPort: 8443,
		},
	}
	assert.Equal(t, 443, server.GetPort())
	assert.Equal(t, 8443, server.GetServerPort())
}

// ========== TableName Tests ==========

func TestForwardNode_TableName(t *testing.T) {
	assert.Equal(t, "v2_forward_node", ForwardNode{}.TableName())
}

func TestForwardRule_TableName(t *testing.T) {
	assert.Equal(t, "v2_forward_rule", ForwardRule{}.TableName())
}

func TestInviteCode_TableName(t *testing.T) {
	assert.Equal(t, "v2_invite_code", InviteCode{}.TableName())
}

func TestCommissionRecord_TableName(t *testing.T) {
	assert.Equal(t, "v2_commission_record", CommissionRecord{}.TableName())
}

func TestCommissionWithdraw_TableName(t *testing.T) {
	assert.Equal(t, "v2_commission_withdraw", CommissionWithdraw{}.TableName())
}

func TestInviteConfig_TableName(t *testing.T) {
	assert.Equal(t, "v2_invite_config", InviteConfig{}.TableName())
}

func TestTrafficLog_TableName(t *testing.T) {
	assert.Equal(t, "v2_server_log", TrafficLog{}.TableName())
}

func TestOnlineLog_TableName(t *testing.T) {
	assert.Equal(t, "v2_online_log", OnlineLog{}.TableName())
}

func TestStatServer_TableName(t *testing.T) {
	assert.Equal(t, "v2_stat_server", StatServer{}.TableName())
}

func TestStatUser_TableName(t *testing.T) {
	assert.Equal(t, "v2_stat_user", StatUser{}.TableName())
}

// ========== Notification Tests ==========

func TestNotificationTemplate_TableName(t *testing.T) {
	assert.Equal(t, "v2_notification_template", NotificationTemplate{}.TableName())
}

func TestNotificationLog_TableName(t *testing.T) {
	assert.Equal(t, "v2_notification_log", NotificationLog{}.TableName())
}

// ========== Telegram Tests ==========

func TestTelegramBot_TableName(t *testing.T) {
	assert.Equal(t, "v2_telegram_bot", TelegramBot{}.TableName())
}

func TestTelegramUser_TableName(t *testing.T) {
	assert.Equal(t, "v2_telegram_user", TelegramUser{}.TableName())
}

func TestTelegramChat_TableName(t *testing.T) {
	assert.Equal(t, "v2_telegram_chat", TelegramChat{}.TableName())
}