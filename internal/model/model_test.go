package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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

func TestSubscriptionTemplateJSONHelpersRejectInvalidJSON(t *testing.T) {
	invalid := "{invalid"
	template := SubscriptionTemplate{
		TransportSettings: &invalid,
		ProtocolSettings:  &invalid,
		Tags:              &invalid,
	}

	assert.Nil(t, template.GetTransportSettings())
	assert.Nil(t, template.GetProtocolSettings())
	assert.Nil(t, template.GetTags())
}

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
		{
			name:     "invalid group json",
			server:   BaseServer{GroupID: "{invalid"},
			expected: nil,
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
		{
			name:     "invalid route json",
			server:   BaseServer{RouteID: "{invalid"},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.server.GetRouteIDs()
			assert.Equal(t, tt.expected, result)
		})
	}
}

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
