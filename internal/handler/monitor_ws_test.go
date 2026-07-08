package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestMonitorClientEnqueueMessageRejectsMarshalError(t *testing.T) {
	client := &monitorClient{
		userID: 1,
		send:   make(chan []byte, 1),
	}

	client.enqueueMessage(MonitorWSMessage{
		Type: "bad",
		Data: func() {},
	})

	assert.Equal(t, 0, len(client.send))
}

func TestMonitorClientEnqueueMessageDropsWhenQueueFull(t *testing.T) {
	client := &monitorClient{
		userID: 1,
		send:   make(chan []byte, 1),
	}
	client.send <- []byte(`{"type":"initial"}`)

	done := make(chan struct{})
	go func() {
		client.enqueueMessage(MonitorWSMessage{Type: "delta", Timestamp: time.Now().Unix()})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("enqueueMessage blocked on a full send queue")
	}

	assert.Equal(t, 1, len(client.send))
}

func TestMonitorWSUpgraderOriginPolicy(t *testing.T) {
	config.Set(nil)
	defer config.Set(nil)

	req := httptest.NewRequest("GET", "http://panel.example.com/admin/ws/monitor", nil)
	req.Header.Set("Origin", "https://evil.example.net")
	assert.False(t, monitorWSUpgrader.CheckOrigin(req))

	req = httptest.NewRequest("GET", "http://panel.example.com/admin/ws/monitor", nil)
	req.Header.Set("Origin", "https://panel.example.com")
	assert.True(t, monitorWSUpgrader.CheckOrigin(req))

	config.Set(&config.Config{
		Server: config.ServerConfig{
			CORS: config.CORSConfig{
				AllowedOrigins: []string{"https://ops.example.com"},
			},
		},
	})
	req = httptest.NewRequest("GET", "http://panel.example.com/admin/ws/monitor", nil)
	req.Header.Set("Origin", "https://ops.example.com")
	assert.True(t, monitorWSUpgrader.CheckOrigin(req))
}
