package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/control-center/internal/core/plugin"
	"github.com/gorilla/websocket"
)

// Config holds agent plugin configuration
type Config struct {
	Host     string `yaml:"host"` // WebSocket host
	Port     int    `yaml:"port"` // WebSocket port
	APIToken string `yaml:"api_token"`
	Timeout  int    `yaml:"timeout"`
}

// AgentPlugin implements the plugin interface for AnixOps-agent
type AgentPlugin struct {
	config Config
	conn   *websocket.Conn
	status plugin.Status
	mu     sync.RWMutex
}

// New creates a new agent plugin
func New() *AgentPlugin {
	return &AgentPlugin{
		status: plugin.Status{
			State:   string(plugin.StateUninitialized),
			Health:  string(plugin.HealthHealthy),
			Metrics: make(map[string]interface{}),
		},
	}
}

// Info returns plugin information
func (p *AgentPlugin) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		Name:        "agent",
		Version:     "1.0.0",
		Description: "AnixOps-agent remote control plugin",
		Author:      "AnixOps",
	}
}

// Init initializes the plugin
func (p *AgentPlugin) Init(ctx context.Context, config map[string]interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := json.Unmarshal(configBytes, &p.config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if p.config.Timeout == 0 {
		p.config.Timeout = 30
	}

	p.status.State = string(plugin.StateInitialized)
	return nil
}

// Start starts the plugin
func (p *AgentPlugin) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.status.State = string(plugin.StateRunning)
	p.status.LastUpdated = time.Now().Unix()
	return nil
}

// Stop stops the plugin
func (p *AgentPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}

	p.status.State = string(plugin.StateStopped)
	p.status.LastUpdated = time.Now().Unix()
	return nil
}

// HealthCheck performs a health check
func (p *AgentPlugin) HealthCheck(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.conn == nil {
		return fmt.Errorf("not connected to any agent")
	}
	return nil
}

// Capabilities returns plugin capabilities
func (p *AgentPlugin) Capabilities() []string {
	return []string{
		"connect",
		"disconnect",
		"exec",
		"upload",
		"download",
		"service_start",
		"service_stop",
		"service_status",
		"system_info",
		"list_agents",
	}
}

// Execute executes an action
func (p *AgentPlugin) Execute(ctx context.Context, action string, params map[string]interface{}) (plugin.Result, error) {
	switch action {
	case "connect":
		return p.connect(ctx, params)
	case "disconnect":
		return p.disconnect()
	case "exec":
		return p.executeCommand(ctx, params)
	case "upload":
		return p.uploadFile(ctx, params)
	case "download":
		return p.downloadFile(ctx, params)
	case "service_start":
		return p.serviceStart(ctx, params)
	case "service_stop":
		return p.serviceStop(ctx, params)
	case "service_status":
		return p.serviceStatus(ctx, params)
	case "system_info":
		return p.systemInfo(ctx, params)
	default:
		return plugin.Result{}, fmt.Errorf("unknown action: %s", action)
	}
}

// GetStatus returns current status
func (p *AgentPlugin) GetStatus(ctx context.Context) (plugin.Status, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status, nil
}

// Agent actions

func (p *AgentPlugin) connect(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	host, ok := params["host"].(string)
	if !ok {
		return plugin.Result{Success: false, Error: "host required"}, nil
	}

	port := 8080
	if v, ok := params["port"].(float64); ok {
		port = int(v)
	}

	url := fmt.Sprintf("ws://%s:%d/ws", host, port)

	dialer := websocket.Dialer{
		HandshakeTimeout: time.Duration(p.config.Timeout) * time.Second,
	}

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	p.mu.Lock()
	p.conn = conn
	p.status.Metrics["connected_host"] = host
	p.status.Metrics["connected_port"] = port
	p.mu.Unlock()

	return plugin.Result{Success: true, Data: map[string]interface{}{
		"host": host,
		"port": port,
	}}, nil
}

func (p *AgentPlugin) disconnect() (plugin.Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
		delete(p.status.Metrics, "connected_host")
		delete(p.status.Metrics, "connected_port")
	}

	return plugin.Result{Success: true}, nil
}

func (p *AgentPlugin) executeCommand(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	cmd, ok := params["command"].(string)
	if !ok {
		return plugin.Result{Success: false, Error: "command required"}, nil
	}

	p.mu.RLock()
	conn := p.conn
	p.mu.RUnlock()

	if conn == nil {
		return plugin.Result{Success: false, Error: "not connected"}, nil
	}

	// Send command
	msg := map[string]interface{}{
		"type":    "exec",
		"command": cmd,
	}
	if err := conn.WriteJSON(msg); err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	// Read response
	var resp map[string]interface{}
	if err := conn.ReadJSON(&resp); err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	return plugin.Result{
		Success: true,
		Data:    resp,
	}, nil
}

func (p *AgentPlugin) uploadFile(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	// TODO: Implement file upload
	return plugin.Result{Success: false, Error: "not implemented"}, nil
}

func (p *AgentPlugin) downloadFile(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	// TODO: Implement file download
	return plugin.Result{Success: false, Error: "not implemented"}, nil
}

func (p *AgentPlugin) serviceStart(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	service, ok := params["service"].(string)
	if !ok {
		return plugin.Result{Success: false, Error: "service name required"}, nil
	}

	return p.executeCommand(ctx, map[string]interface{}{
		"command": fmt.Sprintf("systemctl start %s", service),
	})
}

func (p *AgentPlugin) serviceStop(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	service, ok := params["service"].(string)
	if !ok {
		return plugin.Result{Success: false, Error: "service name required"}, nil
	}

	return p.executeCommand(ctx, map[string]interface{}{
		"command": fmt.Sprintf("systemctl stop %s", service),
	})
}

func (p *AgentPlugin) serviceStatus(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	service, ok := params["service"].(string)
	if !ok {
		return plugin.Result{Success: false, Error: "service name required"}, nil
	}

	return p.executeCommand(ctx, map[string]interface{}{
		"command": fmt.Sprintf("systemctl status %s", service),
	})
}

func (p *AgentPlugin) systemInfo(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	return p.executeCommand(ctx, map[string]interface{}{
		"command": "uname -a",
	})
}
