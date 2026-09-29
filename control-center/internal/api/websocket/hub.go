package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/control-center/internal/core/eventbus"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Message types
const (
	MessageTypeLog    = "log"
	MessageTypeEvent  = "event"
	MessageTypeStatus = "status"
	MessageTypeError  = "error"
	MessageTypePing   = "ping"
	MessageTypePong   = "pong"
)

// Message represents a WebSocket message
type Message struct {
	Type      string      `json:"type"`
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// Client represents a WebSocket client
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	topics map[string]bool
	userID string
	role   string
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	eventBus   *eventbus.EventBus
}

// NewHub creates a new Hub
func NewHub(eventBus *eventbus.EventBus) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		eventBus:   eventBus,
	}
}

// Run starts the hub
func (h *Hub) Run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("WebSocket client connected. Total: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("WebSocket client disconnected. Total: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()

		case <-ticker.C:
			// Send ping to all clients
			ping, _ := json.Marshal(Message{
				Type:      MessageTypePing,
				Timestamp: time.Now().Unix(),
			})
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- ping:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all connected clients
func (h *Hub) Broadcast(msgType string, data interface{}) {
	message, _ := json.Marshal(Message{
		Type:      msgType,
		Timestamp: time.Now().Unix(),
		Data:      data,
	})
	h.broadcast <- message
}

// BroadcastToTopic sends a message to clients subscribed to a topic
func (h *Hub) BroadcastToTopic(topic, msgType string, data interface{}) {
	message, _ := json.Marshal(Message{
		Type:      msgType,
		Timestamp: time.Now().Unix(),
		Data:      data,
	})

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client.topics[topic] {
			select {
			case client.send <- message:
			default:
				close(client.send)
				delete(h.clients, client)
			}
		}
	}
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ReadPump pumps messages from the websocket connection to the hub
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		// Parse message
		var msg struct {
			Action string   `json:"action"`
			Topics []string `json:"topics"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		// Handle actions
		switch msg.Action {
		case "subscribe":
			for _, topic := range msg.Topics {
				c.topics[topic] = true
			}
		case "unsubscribe":
			for _, topic := range msg.Topics {
				delete(c.topics, topic)
			}
		}
	}
}

// WritePump pumps messages from the hub to the websocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Batch queued messages
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Upgrader is the websocket upgrader
var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

// HandleWebSocket handles WebSocket connections
func HandleWebSocket(hub *Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := Upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}

		client := &Client{
			hub:    hub,
			conn:   conn,
			send:   make(chan []byte, 256),
			topics: make(map[string]bool),
			userID: c.GetString("userID"),
			role:   c.GetString("role"),
		}

		client.hub.register <- client

		// Send welcome message
		welcome, _ := json.Marshal(Message{
			Type:      "connected",
			Timestamp: time.Now().Unix(),
			Data: map[string]interface{}{
				"message": "Connected to AnixOps Control Center",
			},
		})
		client.send <- welcome

		// Start read/write pumps
		go client.WritePump()
		client.ReadPump()
	}
}

// LogStreamer streams logs via WebSocket
type LogStreamer struct {
	hub *Hub
}

// NewLogStreamer creates a new log streamer
func NewLogStreamer(hub *Hub) *LogStreamer {
	return &LogStreamer{hub: hub}
}

// Stream streams a log entry
func (s *LogStreamer) Stream(level, source, message string) {
	s.hub.BroadcastToTopic("logs", MessageTypeLog, map[string]interface{}{
		"level":   level,
		"source":  source,
		"message": message,
	})
}

// EventStreamer streams events via WebSocket
type EventStreamer struct {
	hub *Hub
}

// NewEventStreamer creates a new event streamer
func NewEventStreamer(hub *Hub) *EventStreamer {
	return &EventStreamer{hub: hub}
}

// Stream streams an event
func (s *EventStreamer) Stream(eventType string, data interface{}) {
	s.hub.BroadcastToTopic("events", MessageTypeEvent, map[string]interface{}{
		"event_type": eventType,
		"data":       data,
	})
}
