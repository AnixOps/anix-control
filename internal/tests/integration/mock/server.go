package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server Mock 服务器
type Server struct {
	port       int
	server     *http.Server
	users      map[string]*MockUser
	stats      map[string]*MockStats
	mu         sync.RWMutex
	startTime  time.Time
	nodeOnline bool
}

// MockUser 模拟用户
type MockUser struct {
	ID            uint32 `json:"id"`
	UUID          string `json:"uuid"`
	Email         string `json:"email"`
	SpeedLimit    int64  `json:"speed_limit"`
	DeviceLimit   int    `json:"device_limit"`
	TransferEnable int64  `json:"transfer_enable"`
	UsedUpload    int64  `json:"used_upload"`
	UsedDownload  int64  `json:"used_download"`
}

// MockStats 模拟流量统计
type MockStats struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

// NodeConfig 节点配置响应
type NodeConfig struct {
	Host       string `json:"host"`
	ServerPort int    `json:"server_port"`
	Type       string `json:"type"`
	Network    string `json:"network"`
	TLS        int    `json:"tls"`
}

// NewServer 创建 Mock 服务器
func NewServer(port int) *Server {
	return &Server{
		port:   port,
		users:  make(map[string]*MockUser),
		stats:  make(map[string]*MockStats),
	}
}

// Start 启动服务器
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// 健康检查
	mux.HandleFunc("/health", s.handleHealth)

	// 节点 API
	mux.HandleFunc("/api/v2/server/UniProxy/config", s.handleNodeConfig)
	mux.HandleFunc("/api/v2/server/UniProxy/user", s.handleGetUsers)
	mux.HandleFunc("/api/v2/server/UniProxy/push", s.handlePushTraffic)
	mux.HandleFunc("/api/v2/server/UniProxy/alive", s.handleReportOnline)

	// 节点注册
	mux.HandleFunc("/api/v2/node/register", s.handleNodeRegister)
	mux.HandleFunc("/api/v2/node/heartbeat", s.handleNodeHeartbeat)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	s.startTime = time.Now()
	s.nodeOnline = true

	// 启动服务器
	errCh := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// 等待服务器启动
	select {
	case err := <-errCh:
		return err
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// Stop 停止服务器
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// AddUser 添加用户
func (s *Server) AddUser(user *MockUser) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[user.UUID] = user
	s.stats[user.UUID] = &MockStats{}
}

// RemoveUser 移除用户
func (s *Server) RemoveUser(uuid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, uuid)
	delete(s.stats, uuid)
}

// GetStats 获取流量统计
func (s *Server) GetStats(uuid string) *MockStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats[uuid]
}

// URL 返回服务器 URL
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

// 处理函数
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"uptime":    time.Since(s.startTime).Seconds(),
	})
}

func (s *Server) handleNodeConfig(w http.ResponseWriter, r *http.Request) {
	// 检查 token
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 返回默认配置
	config := NodeConfig{
		Host:       "127.0.0.1",
		ServerPort: 8388,
		Type:       "shadowsocks",
		Network:    "tcp",
		TLS:        0,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

func (s *Server) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]interface{}, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, map[string]interface{}{
			"id":              user.ID,
			"uuid":            user.UUID,
			"speed_limit":     user.SpeedLimit,
			"device_limit":    user.DeviceLimit,
			"transfer_enable": user.TransferEnable,
			"u":               user.UsedUpload,
			"d":               user.UsedDownload,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"users": users,
		"total": len(users),
	})
}

func (s *Server) handlePushTraffic(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Data []struct {
			UUID     string `json:"uuid"`
			Upload   int64  `json:"u"`
			Download int64  `json:"d"`
		} `json:"data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range data.Data {
		if stats, ok := s.stats[item.UUID]; ok {
			stats.Upload += item.Upload
			stats.Download += item.Download
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleReportOnline(w http.ResponseWriter, r *http.Request) {
	var data map[string][]string
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleNodeRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AuthKey string `json:"auth_key"`
		Name    string `json:"name"`
		Host    string `json:"host"`
		Port    int    `json:"port"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"node_id": 1,
		"api_key": "test-api-key",
		"message": "registered successfully",
	})
}

func (s *Server) handleNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.nodeOnline = true
	s.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// GetFreePort 获取空闲端口
func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
}