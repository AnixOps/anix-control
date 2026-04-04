package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	serverConnCh := make(chan *websocket.Conn, 1)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		serverConnCh <- conn
	}))

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for websocket server connection")
	}

	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		httpServer.Close()
	}

	return serverConn, clientConn, cleanup
}

func TestHandleWebSocketMessage_RequireAckSendsAck(t *testing.T) {
	initTestDB()

	h := NewAgentHandler()
	serverConn, clientConn, cleanup := newTestWebSocketPair(t)
	defer cleanup()

	nodeID := uint(1001)
	agentConn := &AgentConnection{
		NodeID:   nodeID,
		WsConn:   serverConn,
		LastSeen: time.Now(),
	}
	h.connections.Store(nodeID, agentConn)

	inbound := wsInboundEnvelope{
		ID:         "msg-require-ack-1",
		Type:       "heartbeat",
		NodeID:     nodeID,
		Timestamp:  time.Now().Unix(),
		Payload:    json.RawMessage(`{"ping":"ok"}`),
		RequireAck: true,
	}
	raw, err := json.Marshal(inbound)
	require.NoError(t, err)

	h.handleWebSocketMessage(agentConn, raw)

	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var outbound struct {
		Type    string       `json:"type"`
		NodeID  uint         `json:"node_id"`
		Payload wsAckPayload `json:"payload"`
	}
	err = clientConn.ReadJSON(&outbound)
	require.NoError(t, err)

	assert.Equal(t, "ack", outbound.Type)
	assert.Equal(t, nodeID, outbound.NodeID)
	assert.Equal(t, inbound.ID, outbound.Payload.MessageID)
	assert.True(t, outbound.Payload.Success)
}

func TestHandleWebSocketMessage_AckResolvesPending(t *testing.T) {
	initTestDB()

	h := NewAgentHandler()
	nodeID := uint(1002)
	agentConn := &AgentConnection{NodeID: nodeID}

	waiter := &wsPendingAck{
		NodeID: nodeID,
		Chan:   make(chan *wsAckPayload, 1),
	}
	h.pendingAcks.Store("msg-pending-1", waiter)

	payload, err := json.Marshal(wsAckPayload{
		MessageID: "msg-pending-1",
		Success:   true,
		Timestamp: time.Now().Unix(),
	})
	require.NoError(t, err)

	rawAck, err := json.Marshal(wsInboundEnvelope{
		ID:        "ack-msg-1",
		Type:      "ack",
		NodeID:    nodeID,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	})
	require.NoError(t, err)

	h.handleWebSocketMessage(agentConn, rawAck)

	select {
	case ack := <-waiter.Chan:
		require.NotNil(t, ack)
		assert.Equal(t, "msg-pending-1", ack.MessageID)
		assert.True(t, ack.Success)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for resolved ack")
	}

	_, exists := h.pendingAcks.Load("msg-pending-1")
	assert.False(t, exists)
}

func TestDispatchWithAckRetry_SucceedsWhenAckArrives(t *testing.T) {
	initTestDB()

	h := NewAgentHandler()
	h.ackTimeout = 500 * time.Millisecond
	h.maxRetries = 0

	serverConn, clientConn, cleanup := newTestWebSocketPair(t)
	defer cleanup()

	nodeID := uint(1003)
	agentConn := &AgentConnection{
		NodeID:   nodeID,
		WsConn:   serverConn,
		LastSeen: time.Now(),
	}
	h.connections.Store(nodeID, agentConn)

	readAndAckErr := make(chan error, 1)
	go func() {
		clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var outbound wsOutboundEnvelope
		if err := clientConn.ReadJSON(&outbound); err != nil {
			readAndAckErr <- err
			return
		}
		if !outbound.RequireAck {
			readAndAckErr <- fmt.Errorf("outbound message does not require ack")
			return
		}

		ackPayload, err := json.Marshal(wsAckPayload{
			MessageID: outbound.ID,
			Success:   true,
			Timestamp: time.Now().Unix(),
		})
		if err != nil {
			readAndAckErr <- err
			return
		}

		rawAck, err := json.Marshal(wsInboundEnvelope{
			ID:        "ack-for-" + outbound.ID,
			Type:      "ack",
			NodeID:    nodeID,
			Timestamp: time.Now().Unix(),
			Payload:   ackPayload,
		})
		if err != nil {
			readAndAckErr <- err
			return
		}

		h.handleWebSocketMessage(agentConn, rawAck)
		readAndAckErr <- nil
	}()

	messageID, ack, err := h.dispatchWithAckRetry(agentConn, "task.assign", map[string]interface{}{"task": "demo"}, true)
	require.NoError(t, err)
	require.NotNil(t, ack)
	assert.Equal(t, messageID, ack.MessageID)
	assert.True(t, ack.Success)

	require.NoError(t, <-readAndAckErr)
}

func TestDispatchWithAckRetry_FailsOnTimeout(t *testing.T) {
	initTestDB()

	h := NewAgentHandler()
	h.ackTimeout = 50 * time.Millisecond
	h.maxRetries = 0

	serverConn, clientConn, cleanup := newTestWebSocketPair(t)
	defer cleanup()

	nodeID := uint(1004)
	agentConn := &AgentConnection{
		NodeID:   nodeID,
		WsConn:   serverConn,
		LastSeen: time.Now(),
	}
	h.connections.Store(nodeID, agentConn)

	readDone := make(chan error, 1)
	go func() {
		clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var outbound wsOutboundEnvelope
		readDone <- clientConn.ReadJSON(&outbound)
	}()

	messageID, ack, err := h.dispatchWithAckRetry(agentConn, "task.assign", map[string]interface{}{"task": "demo"}, true)
	require.Error(t, err)
	assert.Nil(t, ack)
	assert.NotEmpty(t, messageID)
	assert.Contains(t, err.Error(), "ack timeout")
	require.NoError(t, <-readDone)

	_, exists := h.pendingAcks.Load(messageID)
	assert.False(t, exists)
}
