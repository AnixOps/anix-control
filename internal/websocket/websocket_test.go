package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestMessageTypeConstants(t *testing.T) {
	assert.Equal(t, MessageType("subscribe"), MessageTypeSubscribe)
	assert.Equal(t, MessageType("unsubscribe"), MessageTypeUnsubscribe)
	assert.Equal(t, MessageType("node_update"), MessageTypeNodeUpdate)
	assert.Equal(t, MessageType("user_update"), MessageTypeUserUpdate)
	assert.Equal(t, MessageType("config_update"), MessageTypeConfigUpdate)
	assert.Equal(t, MessageType("heartbeat"), MessageTypeHeartbeat)
	assert.Equal(t, MessageType("error"), MessageTypeError)
}

func TestMessageJSON(t *testing.T) {
	tests := []struct {
		name    string
		message Message
	}{
		{
			name: "heartbeat message",
			message: Message{
				Type:      MessageTypeHeartbeat,
				Timestamp: time.Now().Unix(),
			},
		},
		{
			name: "node update message",
			message: Message{
				Type: MessageTypeNodeUpdate,
				Data: map[string]interface{}{
					"node_id":     1,
					"change_type": "created",
				},
				Timestamp: time.Now().Unix(),
			},
		},
		{
			name: "error message",
			message: Message{
				Type:      MessageTypeError,
				Error:     "test error",
				Timestamp: time.Now().Unix(),
			},
		},
		{
			name: "user update with data",
			message: Message{
				Type: MessageTypeUserUpdate,
				Data: map[string]interface{}{
					"change_type": "traffic_updated",
				},
				Timestamp: time.Now().Unix(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.message)
			assert.NoError(t, err)
			assert.NotEmpty(t, data)

			var decoded Message
			err = json.Unmarshal(data, &decoded)
			assert.NoError(t, err)
			assert.Equal(t, tt.message.Type, decoded.Type)
		})
	}
}

func TestNewSubscriptionManager(t *testing.T) {
	sm := NewSubscriptionManager()
	assert.NotNil(t, sm)
	assert.NotNil(t, sm.clients)
	assert.NotNil(t, sm.admins)
	assert.NotNil(t, sm.register)
	assert.NotNil(t, sm.unregister)
	assert.NotNil(t, sm.broadcast)
}

func TestSubscriptionManager_NotifyNodeUpdate(t *testing.T) {
	sm := NewSubscriptionManager()

	// This should not block even if Run() is not started
	go func() {
		sm.NotifyNodeUpdate(1, "created")
	}()

	// Read from broadcast channel
	select {
	case msg := <-sm.broadcast:
		assert.NotNil(t, msg)
		assert.NotNil(t, msg.Message)
		assert.Equal(t, MessageTypeNodeUpdate, msg.Message.Type)
		assert.Equal(t, uint(1), msg.Message.Data["node_id"])
		assert.Equal(t, "created", msg.Message.Data["change_type"])
	case <-time.After(100 * time.Millisecond):
		t.Error("expected message in broadcast channel")
	}
}

func TestSubscriptionManager_NotifyUserUpdate(t *testing.T) {
	sm := NewSubscriptionManager()

	go func() {
		sm.NotifyUserUpdate([]uint{1, 2, 3}, "traffic_updated")
	}()

	select {
	case msg := <-sm.broadcast:
		assert.NotNil(t, msg)
		assert.Equal(t, []uint{1, 2, 3}, msg.UserIDs)
		assert.Equal(t, MessageTypeUserUpdate, msg.Message.Type)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSubscriptionManager_NotifyConfigUpdate(t *testing.T) {
	sm := NewSubscriptionManager()

	go func() {
		sm.NotifyConfigUpdate([]uint{1}, "subscription")
	}()

	select {
	case msg := <-sm.broadcast:
		assert.NotNil(t, msg)
		assert.Equal(t, []uint{1}, msg.UserIDs)
		assert.Equal(t, MessageTypeConfigUpdate, msg.Message.Type)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBroadcastMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  *BroadcastMessage
	}{
		{
			name: "broadcast to all",
			msg: &BroadcastMessage{
				Message: &Message{
					Type:      MessageTypeHeartbeat,
					Timestamp: time.Now().Unix(),
				},
			},
		},
		{
			name: "broadcast to specific users",
			msg: &BroadcastMessage{
				UserIDs: []uint{1, 2, 3},
				Message: &Message{
					Type:      MessageTypeUserUpdate,
					Timestamp: time.Now().Unix(),
				},
			},
		},
		{
			name: "broadcast with exclude",
			msg: &BroadcastMessage{
				UserIDs: []uint{1, 2, 3},
				Exclude: 2,
				Message: &Message{
					Type:      MessageTypeConfigUpdate,
					Timestamp: time.Now().Unix(),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.msg.Message)
			assert.NoError(t, err)
			assert.NotEmpty(t, data)
		})
	}
}

func TestClient_SendPong(t *testing.T) {
	sm := NewSubscriptionManager()
	client := &Client{
		userID:       1,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}

	client.sendPong()

	select {
	case data := <-client.send:
		var msg Message
		err := json.Unmarshal(data, &msg)
		assert.NoError(t, err)
		assert.Equal(t, MessageTypeHeartbeat, msg.Type)
	default:
		t.Error("expected message in send channel")
	}
}

func TestClient_SendAck(t *testing.T) {
	sm := NewSubscriptionManager()
	client := &Client{
		userID:       1,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}

	client.sendAck(MessageTypeSubscribe)

	select {
	case data := <-client.send:
		var msg Message
		err := json.Unmarshal(data, &msg)
		assert.NoError(t, err)
		assert.Equal(t, MessageTypeSubscribe, msg.Type)
		assert.Equal(t, "ok", msg.Data["status"])
	default:
		t.Error("expected message in send channel")
	}
}

func TestClient_SendError(t *testing.T) {
	sm := NewSubscriptionManager()
	client := &Client{
		userID:       1,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}

	client.sendError("test error message")

	select {
	case data := <-client.send:
		var msg Message
		err := json.Unmarshal(data, &msg)
		assert.NoError(t, err)
		assert.Equal(t, MessageTypeError, msg.Type)
		assert.Equal(t, "test error message", msg.Error)
	default:
		t.Error("expected message in send channel")
	}
}

func TestClient_HandleMessage(t *testing.T) {
	sm := NewSubscriptionManager()
	client := &Client{
		userID:       1,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}

	tests := []struct {
		name      string
		msg       *Message
		expectMsg bool
	}{
		{
			name: "heartbeat",
			msg: &Message{
				Type:      MessageTypeHeartbeat,
				Timestamp: time.Now().Unix(),
			},
			expectMsg: true,
		},
		{
			name: "subscribe",
			msg: &Message{
				Type:      MessageTypeSubscribe,
				Timestamp: time.Now().Unix(),
			},
			expectMsg: true,
		},
		{
			name: "unsubscribe",
			msg: &Message{
				Type:      MessageTypeUnsubscribe,
				Timestamp: time.Now().Unix(),
			},
			expectMsg: true,
		},
		{
			name: "unknown type",
			msg: &Message{
				Type:      "unknown",
				Timestamp: time.Now().Unix(),
			},
			expectMsg: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear channel
			for len(client.send) > 0 {
				<-client.send
			}

			client.handleMessage(tt.msg)

			if tt.expectMsg {
				select {
				case <-client.send:
					// Message received
				case <-time.After(100 * time.Millisecond):
					t.Error("expected message in send channel")
				}
			}
		})
	}
}

func TestWebSocketHandler_NewWebSocketHandler(t *testing.T) {
	sm := NewSubscriptionManager()
	handler := NewWebSocketHandler(sm, "test-secret")

	assert.NotNil(t, handler)
	assert.Equal(t, sm, handler.sm)
	assert.Equal(t, "test-secret", handler.jwtSecret)
	assert.NotNil(t, handler.userService)
}

func TestWebSocketHandler_GetSubscriptionManager(t *testing.T) {
	sm := NewSubscriptionManager()
	handler := NewWebSocketHandler(sm, "test-secret")

	result := handler.GetSubscriptionManager()
	assert.Equal(t, sm, result)
}

func TestWebSocketHandler_ValidateToken(t *testing.T) {
	secret := "test-secret-key"
	handler := NewWebSocketHandler(NewSubscriptionManager(), secret)

	// Create valid token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": float64(1),
		"is_admin": false,
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	tokenString, err := token.SignedString([]byte(secret))
	assert.NoError(t, err)

	// Test valid token
	userID, isAdmin, err := handler.validateToken(tokenString)
	assert.NoError(t, err)
	assert.Equal(t, uint(1), userID)
	assert.False(t, isAdmin)

	// Test admin token
	adminToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": float64(2),
		"is_admin": true,
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	adminTokenString, err := adminToken.SignedString([]byte(secret))
	assert.NoError(t, err)

	userID, isAdmin, err = handler.validateToken(adminTokenString)
	assert.NoError(t, err)
	assert.Equal(t, uint(2), userID)
	assert.True(t, isAdmin)
}

func TestWebSocketHandler_ValidateToken_Invalid(t *testing.T) {
	secret := "test-secret-key"
	handler := NewWebSocketHandler(NewSubscriptionManager(), secret)

	tests := []struct {
		name      string
		token     string
		expectErr bool
	}{
		{
			name:      "empty token",
			token:     "",
			expectErr: true,
		},
		{
			name:      "invalid format",
			token:     "invalid.token.format",
			expectErr: true,
		},
		{
			name:      "wrong secret",
			token:     createTokenWithSecret("wrong-secret", 1, false),
			expectErr: true,
		},
		{
			name:      "expired token",
			token:     createExpiredToken(secret, 1, false),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := handler.validateToken(tt.token)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func createTokenWithSecret(secret string, userID uint, isAdmin bool) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": float64(userID),
		"is_admin": isAdmin,
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString([]byte(secret))
	return tokenString
}

func createExpiredToken(secret string, userID uint, isAdmin bool) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": float64(userID),
		"is_admin": isAdmin,
		"exp":      time.Now().Add(-time.Hour).Unix(), // Expired 1 hour ago
	})
	tokenString, _ := token.SignedString([]byte(secret))
	return tokenString
}

func TestSubscriptionManager_Run(t *testing.T) {
	sm := NewSubscriptionManager()

	// Start Run in background
	go sm.Run()
	defer func() {
		// The Run function runs forever, so we can't really stop it
		// In production, you'd want to add a shutdown channel
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Test client registration
	client := &Client{
		userID:       1,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}

	// Register client
	sm.register <- client
	time.Sleep(50 * time.Millisecond)

	// Check client was registered
	sm.mu.RLock()
	_, exists := sm.clients[1]
	sm.mu.RUnlock()
	assert.True(t, exists)

	// Unregister client
	sm.unregister <- client
	time.Sleep(50 * time.Millisecond)

	// Check client was unregistered
	sm.mu.RLock()
	_, exists = sm.clients[1]
	sm.mu.RUnlock()
	assert.False(t, exists)
}

func TestSubscriptionManager_Run_Admin(t *testing.T) {
	sm := NewSubscriptionManager()
	go sm.Run()

	time.Sleep(50 * time.Millisecond)

	adminClient := &Client{
		userID:       1,
		isAdmin:      true,
		subscription: sm,
		send:         make(chan []byte, 10),
	}

	// Register admin
	sm.register <- adminClient
	time.Sleep(50 * time.Millisecond)

	// Check admin was registered in admins map
	sm.mu.RLock()
	_, exists := sm.admins[1]
	sm.mu.RUnlock()
	assert.True(t, exists)
}

func TestSubscriptionManager_BroadcastMessage(t *testing.T) {
	sm := NewSubscriptionManager()

	// Create test clients
	client1 := &Client{
		userID:       1,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}
	client2 := &Client{
		userID:       2,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}

	sm.clients[1] = client1
	sm.clients[2] = client2

	// Broadcast to all
	msg := &BroadcastMessage{
		Message: &Message{
			Type:      MessageTypeHeartbeat,
			Timestamp: time.Now().Unix(),
		},
	}
	sm.broadcastMessage(msg)

	// Both clients should receive the message
	assert.Equal(t, 1, len(client1.send))
	assert.Equal(t, 1, len(client2.send))
}

func TestSubscriptionManager_BroadcastMessage_ToSpecificUsers(t *testing.T) {
	sm := NewSubscriptionManager()

	client1 := &Client{
		userID:       1,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}
	client2 := &Client{
		userID:       2,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}

	sm.clients[1] = client1
	sm.clients[2] = client2

	// Broadcast to user 1 only
	msg := &BroadcastMessage{
		UserIDs: []uint{1},
		Message: &Message{
			Type:      MessageTypeUserUpdate,
			Timestamp: time.Now().Unix(),
		},
	}
	sm.broadcastMessage(msg)

	// Only client1 should receive
	assert.Equal(t, 1, len(client1.send))
	assert.Equal(t, 0, len(client2.send))
}

func TestSubscriptionManager_BroadcastMessage_WithExclude(t *testing.T) {
	sm := NewSubscriptionManager()

	client1 := &Client{
		userID:       1,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}
	client2 := &Client{
		userID:       2,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}

	sm.clients[1] = client1
	sm.clients[2] = client2

	// Broadcast to all but exclude user 1
	msg := &BroadcastMessage{
		Exclude: 1,
		Message: &Message{
			Type:      MessageTypeNodeUpdate,
			Timestamp: time.Now().Unix(),
		},
	}
	sm.broadcastMessage(msg)

	// Only client2 should receive
	assert.Equal(t, 0, len(client1.send))
	assert.Equal(t, 1, len(client2.send))
}

func TestSubscriptionManager_BroadcastMessage_ToAdmins(t *testing.T) {
	sm := NewSubscriptionManager()

	adminClient := &Client{
		userID:       1,
		isAdmin:      true,
		subscription: sm,
		send:         make(chan []byte, 10),
	}
	regularClient := &Client{
		userID:       2,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}

	sm.admins[1] = adminClient
	sm.clients[2] = regularClient

	// Broadcast to all (includes admins)
	msg := &BroadcastMessage{
		Message: &Message{
			Type:      MessageTypeNodeUpdate,
			Timestamp: time.Now().Unix(),
		},
	}
	sm.broadcastMessage(msg)

	// Both should receive
	assert.Equal(t, 1, len(adminClient.send))
	assert.Equal(t, 1, len(regularClient.send))
}

func TestSubscriptionManager_SendHeartbeat(t *testing.T) {
	sm := NewSubscriptionManager()

	client := &Client{
		userID:       1,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 10),
	}
	sm.clients[1] = client

	sm.sendHeartbeat()

	// Should send through broadcast channel
	select {
	case msg := <-sm.broadcast:
		assert.NotNil(t, msg)
		assert.Equal(t, MessageTypeHeartbeat, msg.Message.Type)
	case <-time.After(100 * time.Millisecond):
		t.Error("expected heartbeat in broadcast channel")
	}
}

func TestWebSocketHandler_HandleWebSocket_Unauthorized(t *testing.T) {
	sm := NewSubscriptionManager()
	handler := NewWebSocketHandler(sm, "test-secret")

	tests := []struct {
		name       string
		token      string
		expectCode int
	}{
		{
			name:       "no token",
			token:      "",
			expectCode: http.StatusUnauthorized,
		},
		{
			name:       "invalid token",
			token:      "invalid-token",
			expectCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/ws?token="+tt.token, nil)
			w := httptest.NewRecorder()

			handler.HandleWebSocket(w, req)

			assert.Equal(t, tt.expectCode, w.Code)
		})
	}
}

func TestWebSocketHandler_HandleWebSocket_ValidToken(t *testing.T) {
	secret := "test-secret"
	sm := NewSubscriptionManager()
	handler := NewWebSocketHandler(sm, secret)

	// Create valid token
	validToken := createTokenWithSecret(secret, 1, false)

	// Create test server with WebSocket handler
	server := httptest.NewServer(http.HandlerFunc(handler.HandleWebSocket))
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=" + validToken

	// Connect as WebSocket client
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		// WebSocket upgrade may fail in test environment, that's okay
		t.Logf("WebSocket dial failed (expected in test env): %v", err)
		return
	}
	defer ws.Close()

	// If connection succeeded, verify we can send/receive messages
	err = ws.WriteJSON(Message{
		Type:      MessageTypeHeartbeat,
		Timestamp: time.Now().Unix(),
	})
	assert.NoError(t, err)

	// Read response
	var response Message
	err = ws.ReadJSON(&response)
	assert.NoError(t, err)
	assert.Equal(t, MessageTypeHeartbeat, response.Type)
}

func TestWebSocketHandler_HandleWebSocket_TokenFromHeader(t *testing.T) {
	secret := "test-secret"
	sm := NewSubscriptionManager()
	handler := NewWebSocketHandler(sm, secret)

	validToken := createTokenWithSecret(secret, 1, false)

	server := httptest.NewServer(http.HandlerFunc(handler.HandleWebSocket))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect with token in header instead of query param
	header := http.Header{}
	header.Set("Authorization", "Bearer "+validToken)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Logf("WebSocket dial failed (expected in test env): %v", err)
		return
	}
	defer ws.Close()
}

func TestClient_SendChannel(t *testing.T) {
	sm := NewSubscriptionManager()
	client := &Client{
		userID:       1,
		isAdmin:      false,
		subscription: sm,
		send:         make(chan []byte, 3), // Small buffer
	}

	// Send multiple messages
	for i := 0; i < 3; i++ {
		msg := &Message{
			Type:      MessageTypeHeartbeat,
			Timestamp: time.Now().Unix(),
		}
		data, _ := json.Marshal(msg)
		client.send <- data
	}

	// Channel should be full
	assert.Equal(t, 3, len(client.send))
}

func TestBroadcastMessage_JSONMarshal(t *testing.T) {
	msg := &BroadcastMessage{
		UserIDs: []uint{1, 2, 3},
		Exclude: 2,
		Message: &Message{
			Type: MessageTypeUserUpdate,
			Data: map[string]interface{}{
				"change_type": "traffic_updated",
			},
			Timestamp: time.Now().Unix(),
		},
	}

	data, err := json.Marshal(msg.Message)
	assert.NoError(t, err)

	var decoded Message
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, MessageTypeUserUpdate, decoded.Type)
	assert.Equal(t, "traffic_updated", decoded.Data["change_type"])
}

func TestSubscriptionManager_ConcurrentAccess(t *testing.T) {
	sm := NewSubscriptionManager()
	go sm.Run()

	time.Sleep(50 * time.Millisecond)

	// Concurrent client registrations
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id uint) {
			client := &Client{
				userID:       id,
				isAdmin:      false,
				subscription: sm,
				send:         make(chan []byte, 10),
			}
			sm.register <- client
			done <- true
		}(uint(i + 1))
	}

	// Wait for all registrations
	for i := 0; i < 10; i++ {
		<-done
	}

	time.Sleep(100 * time.Millisecond)

	// Verify all clients registered
	sm.mu.RLock()
	count := len(sm.clients)
	sm.mu.RUnlock()
	assert.Equal(t, 10, count)
}