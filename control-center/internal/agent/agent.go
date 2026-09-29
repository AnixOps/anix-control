// Package agent provides the AnixOps Agent client for server management.
//
// The agent runs on target servers and communicates with the AnixOps Control Center
// to receive commands, report metrics, and execute playbooks.
package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// Version is the current agent version
const Version = "1.0.0"

// Config holds the agent configuration
type Config struct {
	// ServerURL is the AnixOps Control Center API URL
	ServerURL string `json:"server_url"`

	// AgentID is the unique identifier for this agent
	AgentID string `json:"agent_id"`

	// SecretKey is the authentication secret
	SecretKey string `json:"secret_key"`

	// Hostname is the server hostname
	Hostname string `json:"hostname"`

	// Labels are custom labels for the agent
	Labels map[string]string `json:"labels"`

	// HeartbeatInterval is the interval between heartbeats
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`

	// MetricsInterval is the interval between metrics collection
	MetricsInterval time.Duration `json:"metrics_interval"`

	// LogLevel is the logging level
	LogLevel string `json:"log_level"`

	// TLSSkipVerify skips TLS certificate verification
	TLSSkipVerify bool `json:"tls_skip_verify"`
}

// Agent represents the AnixOps agent instance
type Agent struct {
	config     *Config
	httpClient *http.Client
	info       *AgentInfo
	running    bool
	stopCh     chan struct{}
	wg         sync.WaitGroup

	// Handlers
	commandHandler CommandHandler
	metricProvider MetricProvider
}

// AgentInfo contains information about the agent and its host
type AgentInfo struct {
	AgentID   string            `json:"agent_id"`
	Hostname  string            `json:"hostname"`
	IPAddress string            `json:"ip_address"`
	OS        string            `json:"os"`
	Arch      string            `json:"arch"`
	Version   string            `json:"version"`
	Labels    map[string]string `json:"labels"`
	LastSeen  time.Time         `json:"last_seen"`
	Status    string            `json:"status"`
	CPUCount  int               `json:"cpu_count"`
	MemoryGB  float64           `json:"memory_gb"`
	DiskGB    float64           `json:"disk_gb"`
}

// CommandHandler handles incoming commands from the control center
type CommandHandler func(ctx context.Context, cmd *Command) (*CommandResult, error)

// MetricProvider provides system metrics
type MetricProvider func() (*Metrics, error)

// Command represents a command from the control center
type Command struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Payload   map[string]interface{} `json:"payload"`
	Timeout   time.Duration          `json:"timeout"`
	CreatedAt time.Time              `json:"created_at"`
}

// CommandResult is the result of command execution
type CommandResult struct {
	CommandID string         `json:"command_id"`
	Success   bool           `json:"success"`
	Output    string         `json:"output"`
	Error     string         `json:"error"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Duration  time.Duration  `json:"duration"`
}

// Metrics contains system metrics
type Metrics struct {
	CPUUsage     float64 `json:"cpu_usage"`
	MemoryUsage  float64 `json:"memory_usage"`
	MemoryTotal  uint64  `json:"memory_total"`
	MemoryUsed   uint64  `json:"memory_used"`
	DiskUsage    float64 `json:"disk_usage"`
	DiskTotal    uint64  `json:"disk_total"`
	DiskUsed     uint64  `json:"disk_used"`
	NetworkRx    uint64  `json:"network_rx"`
	NetworkTx    uint64  `json:"network_tx"`
	LoadAvg1     float64 `json:"load_avg_1"`
	LoadAvg5     float64 `json:"load_avg_5"`
	LoadAvg15    float64 `json:"load_avg_15"`
	Uptime       uint64  `json:"uptime"`
	ProcessCount int     `json:"process_count"`
	Timestamp    int64   `json:"timestamp"`
}

// New creates a new Agent instance
func New(cfg *Config) (*Agent, error) {
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 30 * time.Second
	}
	if cfg.MetricsInterval == 0 {
		cfg.MetricsInterval = 60 * time.Second
	}

	// Get hostname if not set
	if cfg.Hostname == "" {
		hostname, _ := os.Hostname()
		cfg.Hostname = hostname
	}

	// Create HTTP client
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cfg.TLSSkipVerify,
			},
		},
	}

	agent := &Agent{
		config:     cfg,
		httpClient: httpClient,
		stopCh:     make(chan struct{}),
		info: &AgentInfo{
			AgentID:  cfg.AgentID,
			Hostname: cfg.Hostname,
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
			Version:  Version,
			Labels:   cfg.Labels,
			CPUCount: runtime.NumCPU(),
			Status:   "initialized",
		},
	}

	// Set default handlers
	agent.commandHandler = agent.defaultCommandHandler
	agent.metricProvider = agent.defaultMetricProvider

	return agent, nil
}

// Start starts the agent
func (a *Agent) Start(ctx context.Context) error {
	if a.running {
		return fmt.Errorf("agent already running")
	}

	a.running = true
	a.info.Status = "running"

	// Collect initial info
	if err := a.collectHostInfo(); err != nil {
		return fmt.Errorf("failed to collect host info: %w", err)
	}

	// Register with control center
	if err := a.register(ctx); err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}

	// Start heartbeat goroutine
	a.wg.Add(1)
	go a.heartbeatLoop(ctx)

	// Start metrics collection goroutine
	a.wg.Add(1)
	go a.metricsLoop(ctx)

	// Start command polling goroutine
	a.wg.Add(1)
	go a.commandLoop(ctx)

	return nil
}

// Stop stops the agent
func (a *Agent) Stop() error {
	if !a.running {
		return nil
	}

	a.running = false
	a.info.Status = "stopped"
	close(a.stopCh)
	a.wg.Wait()

	return nil
}

// SetCommandHandler sets a custom command handler
func (a *Agent) SetCommandHandler(handler CommandHandler) {
	a.commandHandler = handler
}

// SetMetricProvider sets a custom metric provider
func (a *Agent) SetMetricProvider(provider MetricProvider) {
	a.metricProvider = provider
}

// GetInfo returns the current agent info
func (a *Agent) GetInfo() *AgentInfo {
	return a.info
}

// heartbeatLoop sends periodic heartbeats to the control center
func (a *Agent) heartbeatLoop(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-ticker.C:
			if err := a.sendHeartbeat(ctx); err != nil {
				// Log error but continue
				fmt.Fprintf(os.Stderr, "heartbeat error: %v\n", err)
			}
		}
	}
}

// metricsLoop collects and sends metrics periodically
func (a *Agent) metricsLoop(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-ticker.C:
			if err := a.collectAndSendMetrics(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "metrics error: %v\n", err)
			}
		}
	}
}

// commandLoop polls for commands from the control center
func (a *Agent) commandLoop(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-ticker.C:
			if err := a.pollAndExecuteCommands(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "command poll error: %v\n", err)
			}
		}
	}
}

// register registers the agent with the control center
func (a *Agent) register(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1/agents/register", a.config.ServerURL)

	payload := map[string]interface{}{
		"agent_id":  a.config.AgentID,
		"secret":    a.config.SecretKey,
		"hostname":  a.info.Hostname,
		"os":        a.info.OS,
		"arch":      a.info.Arch,
		"version":   a.info.Version,
		"labels":    a.info.Labels,
		"cpu_count": a.info.CPUCount,
		"memory_gb": a.info.MemoryGB,
		"disk_gb":   a.info.DiskGB,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-ID", a.config.AgentID)
	req.Header.Set("X-Agent-Secret", a.config.SecretKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("registration failed: %d", resp.StatusCode)
	}

	return nil
}

// sendHeartbeat sends a heartbeat to the control center
func (a *Agent) sendHeartbeat(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1/agents/heartbeat", a.config.ServerURL)

	payload := map[string]interface{}{
		"agent_id":  a.config.AgentID,
		"status":    a.info.Status,
		"timestamp": time.Now().Unix(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-ID", a.config.AgentID)
	req.Header.Set("X-Agent-Secret", a.config.SecretKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	a.info.LastSeen = time.Now()
	return nil
}

// defaultCommandHandler handles basic commands
func (a *Agent) defaultCommandHandler(ctx context.Context, cmd *Command) (*CommandResult, error) {
	result := &CommandResult{
		CommandID: cmd.ID,
		Success:   false,
	}

	switch cmd.Type {
	case "ping":
		result.Success = true
		result.Output = "pong"

	case "exec":
		// Execute shell command
		command, _ := cmd.Payload["command"].(string)
		if command == "" {
			result.Error = "command is required"
			return result, nil
		}

		execCmd := exec.CommandContext(ctx, "sh", "-c", command)
		output, err := execCmd.CombinedOutput()
		if err != nil {
			result.Error = err.Error()
			result.Output = string(output)
			return result, nil
		}

		result.Success = true
		result.Output = string(output)

	case "script":
		// Execute script from control center
		script, _ := cmd.Payload["script"].(string)
		if script == "" {
			result.Error = "script is required"
			return result, nil
		}

		// Write script to temp file and execute
		tmpFile, err := os.CreateTemp("", "anixops-script-*.sh")
		if err != nil {
			result.Error = err.Error()
			return result, nil
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		if _, err := tmpFile.WriteString(script); err != nil {
			result.Error = err.Error()
			return result, nil
		}

		execCmd := exec.CommandContext(ctx, "sh", tmpFile.Name())
		output, err := execCmd.CombinedOutput()
		if err != nil {
			result.Error = err.Error()
			result.Output = string(output)
			return result, nil
		}

		result.Success = true
		result.Output = string(output)

	default:
		result.Error = fmt.Sprintf("unknown command type: %s", cmd.Type)
	}

	return result, nil
}
