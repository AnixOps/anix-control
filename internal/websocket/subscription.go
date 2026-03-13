package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/anixops/v2board/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

// WebSocket 升级器
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 生产环境应该验证 Origin
		return true
	},
}

// MessageType 消息类型
type MessageType string

const (
	MessageTypeSubscribe    MessageType = "subscribe"
	MessageTypeUnsubscribe  MessageType = "unsubscribe"
	MessageTypeNodeUpdate   MessageType = "node_update"
	MessageTypeUserUpdate   MessageType = "user_update"
	MessageTypeConfigUpdate MessageType = "config_update"
	MessageTypeHeartbeat    MessageType = "heartbeat"
	MessageTypeError        MessageType = "error"
)

// Message WebSocket 消息
type Message struct {
	Type      MessageType            `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// Client WebSocket 客户端
type Client struct {
	conn         *websocket.Conn
	userID       uint
	isAdmin      bool
	subscription *SubscriptionManager
	send         chan []byte
	mu           sync.Mutex
}

// SubscriptionManager 订阅管理器
type SubscriptionManager struct {
	clients    map[uint]*Client // 用户ID -> 客户端
	admins     map[uint]*Client // 管理员ID -> 客户端
	register   chan *Client
	unregister chan *Client
	broadcast  chan *BroadcastMessage
	mu         sync.RWMutex
}

// BroadcastMessage 广播消息
type BroadcastMessage struct {
	UserIDs []uint      // 目标用户ID，nil 表示广播给所有
	Message *Message    // 消息内容
	Exclude uint        // 排除的用户ID
}

// NewSubscriptionManager 创建订阅管理器
func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{
		clients:    make(map[uint]*Client),
		admins:     make(map[uint]*Client),
		register:   make(chan *Client, 256),
		unregister: make(chan *Client, 256),
		broadcast:  make(chan *BroadcastMessage, 1024),
	}
}

// Run 运行订阅管理器
func (sm *SubscriptionManager) Run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-sm.register:
			sm.mu.Lock()
			if client.isAdmin {
				sm.admins[client.userID] = client
			} else {
				sm.clients[client.userID] = client
			}
			sm.mu.Unlock()
			log.Printf("WebSocket client connected: user_id=%d, is_admin=%v", client.userID, client.isAdmin)

		case client := <-sm.unregister:
			sm.mu.Lock()
			if client.isAdmin {
				delete(sm.admins, client.userID)
			} else {
				delete(sm.clients, client.userID)
			}
			sm.mu.Unlock()
			close(client.send)
			log.Printf("WebSocket client disconnected: user_id=%d", client.userID)

		case msg := <-sm.broadcast:
			sm.broadcastMessage(msg)

		case <-ticker.C:
			// 发送心跳
			sm.sendHeartbeat()
		}
	}
}

// broadcastMessage 广播消息
func (sm *SubscriptionManager) broadcastMessage(msg *BroadcastMessage) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	data, err := json.Marshal(msg.Message)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}

	// 如果指定了目标用户
	if msg.UserIDs != nil {
		for _, userID := range msg.UserIDs {
			if userID == msg.Exclude {
				continue
			}
			if client, ok := sm.clients[userID]; ok {
				select {
				case client.send <- data:
				default:
					// 缓冲区满，跳过
				}
			}
		}
		return
	}

	// 广播给所有用户
	for _, client := range sm.clients {
		if client.userID == msg.Exclude {
			continue
		}
		select {
		case client.send <- data:
		default:
		}
	}

	// 也发送给管理员
	for _, client := range sm.admins {
		if client.userID == msg.Exclude {
			continue
		}
		select {
		case client.send <- data:
		default:
		}
	}
}

// sendHeartbeat 发送心跳
func (sm *SubscriptionManager) sendHeartbeat() {
	msg := &Message{
		Type:      MessageTypeHeartbeat,
		Timestamp: time.Now().Unix(),
	}

	sm.broadcast <- &BroadcastMessage{Message: msg}
}

// NotifyNodeUpdate 通知节点更新
func (sm *SubscriptionManager) NotifyNodeUpdate(nodeID uint, changeType string) {
	msg := &Message{
		Type: MessageTypeNodeUpdate,
		Data: map[string]interface{}{
			"node_id":     nodeID,
			"change_type": changeType,
		},
		Timestamp: time.Now().Unix(),
	}

	sm.broadcast <- &BroadcastMessage{Message: msg}
}

// NotifyUserUpdate 通知用户更新
func (sm *SubscriptionManager) NotifyUserUpdate(userIDs []uint, changeType string) {
	msg := &Message{
		Type: MessageTypeUserUpdate,
		Data: map[string]interface{}{
			"change_type": changeType,
		},
		Timestamp: time.Now().Unix(),
	}

	sm.broadcast <- &BroadcastMessage{
		UserIDs: userIDs,
		Message: msg,
	}
}

// NotifyConfigUpdate 通知配置更新
func (sm *SubscriptionManager) NotifyConfigUpdate(userIDs []uint, configType string) {
	msg := &Message{
		Type: MessageTypeConfigUpdate,
		Data: map[string]interface{}{
			"config_type": configType,
		},
		Timestamp: time.Now().Unix(),
	}

	sm.broadcast <- &BroadcastMessage{
		UserIDs: userIDs,
		Message: msg,
	}
}

// readPump 读取客户端消息
func (c *Client) readPump() {
	defer func() {
		c.subscription.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512 * 1024) // 512KB
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

		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			c.sendError("invalid message format")
			continue
		}

		c.handleMessage(&msg)
	}
}

// writePump 发送消息到客户端
func (c *Client) writePump() {
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

			// 批量发送
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

// handleMessage 处理客户端消息
func (c *Client) handleMessage(msg *Message) {
	switch msg.Type {
	case MessageTypeHeartbeat:
		c.sendPong()

	case MessageTypeSubscribe:
		// 订阅特定频道
		c.sendAck(msg.Type)

	case MessageTypeUnsubscribe:
		// 取消订阅
		c.sendAck(msg.Type)

	default:
		c.sendError("unknown message type")
	}
}

// sendPong 发送心跳响应
func (c *Client) sendPong() {
	msg := &Message{
		Type:      MessageTypeHeartbeat,
		Timestamp: time.Now().Unix(),
	}
	data, _ := json.Marshal(msg)
	c.send <- data
}

// sendAck 发送确认
func (c *Client) sendAck(msgType MessageType) {
	msg := &Message{
		Type:      msgType,
		Timestamp: time.Now().Unix(),
		Data:      map[string]interface{}{"status": "ok"},
	}
	data, _ := json.Marshal(msg)
	c.send <- data
}

// sendError 发送错误
func (c *Client) sendError(errMsg string) {
	msg := &Message{
		Type:      MessageTypeError,
		Timestamp: time.Now().Unix(),
		Error:     errMsg,
	}
	data, _ := json.Marshal(msg)
	c.send <- data
}

// WebSocketHandler WebSocket 处理器
type WebSocketHandler struct {
	sm          *SubscriptionManager
	jwtSecret   string
	userService *service.UserService
}

// NewWebSocketHandler 创建 WebSocket 处理器
func NewWebSocketHandler(sm *SubscriptionManager, jwtSecret string) *WebSocketHandler {
	return &WebSocketHandler{
		sm:          sm,
		jwtSecret:   jwtSecret,
		userService: service.NewUserService(),
	}
}

// HandleWebSocket 处理 WebSocket 连接
func (h *WebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 从查询参数或 Header 获取 token
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}
	}

	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 验证 JWT
	userID, isAdmin, err := h.validateToken(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// 升级为 WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// 创建客户端
	client := &Client{
		conn:         conn,
		userID:       userID,
		isAdmin:      isAdmin,
		subscription: h.sm,
		send:         make(chan []byte, 256),
	}

	// 注册客户端
	h.sm.register <- client

	// 启动读写协程
	go client.writePump()
	go client.readPump()
}

// validateToken 验证 JWT Token
func (h *WebSocketHandler) validateToken(tokenString string) (uint, bool, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(h.jwtSecret), nil
	})

	if err != nil {
		return 0, false, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, false, jwt.ErrSignatureInvalid
	}

	userID := uint(claims["user_id"].(float64))
	isAdmin := claims["is_admin"].(bool)

	return userID, isAdmin, nil
}

// GetSubscriptionManager 获取订阅管理器
func (h *WebSocketHandler) GetSubscriptionManager() *SubscriptionManager {
	return h.sm
}