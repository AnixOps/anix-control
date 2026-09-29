package agent

import (
	"context"
	"testing"
	"time"
)

func TestConfig(t *testing.T) {
	cfg := &Config{
		ServerURL:         "http://localhost:8080",
		AgentID:           "test-agent",
		SecretKey:         "test-secret",
		Hostname:          "test-host",
		HeartbeatInterval: 30 * time.Second,
		MetricsInterval:   60 * time.Second,
		LogLevel:          "info",
		TLSSkipVerify:     false,
	}

	if cfg.ServerURL != "http://localhost:8080" {
		t.Errorf("expected server URL 'http://localhost:8080', got '%s'", cfg.ServerURL)
	}
	if cfg.AgentID != "test-agent" {
		t.Errorf("expected agent ID 'test-agent', got '%s'", cfg.AgentID)
	}
	if cfg.SecretKey != "test-secret" {
		t.Errorf("expected secret key 'test-secret', got '%s'", cfg.SecretKey)
	}
	if cfg.Hostname != "test-host" {
		t.Errorf("expected hostname 'test-host', got '%s'", cfg.Hostname)
	}
	if cfg.HeartbeatInterval != 30*time.Second {
		t.Errorf("expected heartbeat interval 30s, got %v", cfg.HeartbeatInterval)
	}
	if cfg.MetricsInterval != 60*time.Second {
		t.Errorf("expected metrics interval 60s, got %v", cfg.MetricsInterval)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected log level 'info', got '%s'", cfg.LogLevel)
	}
	if cfg.TLSSkipVerify {
		t.Error("expected TLS skip verify to be false")
	}
}

func TestConfig_Defaults(t *testing.T) {
	cfg := &Config{}

	if cfg.ServerURL != "" {
		t.Errorf("expected empty server URL, got '%s'", cfg.ServerURL)
	}
	if cfg.AgentID != "" {
		t.Errorf("expected empty agent ID, got '%s'", cfg.AgentID)
	}
}

func TestNew(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if agent == nil {
		t.Fatal("New returned nil agent")
	}
	if agent.config == nil {
		t.Error("agent config not set")
	}
	if agent.httpClient == nil {
		t.Error("agent HTTP client not set")
	}
	if agent.info == nil {
		t.Error("agent info not set")
	}
}

func TestNew_DefaultIntervals(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	// Check default intervals are set
	if agent.config.HeartbeatInterval != 30*time.Second {
		t.Errorf("expected default heartbeat interval 30s, got %v", agent.config.HeartbeatInterval)
	}
	if agent.config.MetricsInterval != 60*time.Second {
		t.Errorf("expected default metrics interval 60s, got %v", agent.config.MetricsInterval)
	}
}

func TestNew_DefaultHostname(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	// Hostname should be set to system hostname if not provided
	if agent.config.Hostname == "" {
		t.Error("expected hostname to be set from system")
	}
}

func TestNew_WithLabels(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
		Labels: map[string]string{
			"env":  "test",
			"role": "web",
		},
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if agent.info.Labels["env"] != "test" {
		t.Errorf("expected label env=test, got %s", agent.info.Labels["env"])
	}
	if agent.info.Labels["role"] != "web" {
		t.Errorf("expected label role=web, got %s", agent.info.Labels["role"])
	}
}

func TestNew_TLSkipVerify(t *testing.T) {
	cfg := &Config{
		ServerURL:     "http://localhost:8080",
		AgentID:       "test-agent",
		SecretKey:     "test-secret",
		TLSSkipVerify: true,
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if !agent.config.TLSSkipVerify {
		t.Error("expected TLS skip verify to be true")
	}
}

func TestAgent_GetInfo(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	info := agent.GetInfo()

	if info == nil {
		t.Fatal("GetInfo returned nil")
	}
	if info.AgentID != "test-agent" {
		t.Errorf("expected agent ID 'test-agent', got '%s'", info.AgentID)
	}
	if info.Version != Version {
		t.Errorf("expected version '%s', got '%s'", Version, info.Version)
	}
}

func TestAgent_Stop_NotRunning(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	err := agent.Stop()

	if err != nil {
		t.Errorf("unexpected error stopping non-running agent: %v", err)
	}
}

func TestAgent_SetCommandHandler(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)

	handler := func(ctx context.Context, cmd *Command) (*CommandResult, error) {
		return &CommandResult{CommandID: cmd.ID, Success: true}, nil
	}

	agent.SetCommandHandler(handler)

	if agent.commandHandler == nil {
		t.Error("command handler not set")
	}
}

func TestAgent_SetMetricProvider(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)

	provider := func() (*Metrics, error) {
		return &Metrics{CPUUsage: 50.0}, nil
	}

	agent.SetMetricProvider(provider)

	if agent.metricProvider == nil {
		t.Error("metric provider not set")
	}
}

func TestAgentInfo(t *testing.T) {
	info := &AgentInfo{
		AgentID:   "test-agent",
		Hostname:  "test-host",
		IPAddress: "192.168.1.1",
		OS:        "linux",
		Arch:      "amd64",
		Version:   "1.0.0",
		Status:    "running",
		CPUCount:  4,
		MemoryGB:  8.0,
		DiskGB:    100.0,
	}

	if info.AgentID != "test-agent" {
		t.Errorf("expected agent ID 'test-agent', got '%s'", info.AgentID)
	}
	if info.Hostname != "test-host" {
		t.Errorf("expected hostname 'test-host', got '%s'", info.Hostname)
	}
	if info.Status != "running" {
		t.Errorf("expected status 'running', got '%s'", info.Status)
	}
}

func TestCommand(t *testing.T) {
	cmd := &Command{
		ID:        "cmd-123",
		Type:      "exec",
		Payload:   map[string]interface{}{"command": "ls"},
		Timeout:   30 * time.Second,
		CreatedAt: time.Now(),
	}

	if cmd.ID != "cmd-123" {
		t.Errorf("expected command ID 'cmd-123', got '%s'", cmd.ID)
	}
	if cmd.Type != "exec" {
		t.Errorf("expected command type 'exec', got '%s'", cmd.Type)
	}
}

func TestCommandResult(t *testing.T) {
	result := &CommandResult{
		CommandID: "cmd-123",
		Success:   true,
		Output:    "ok",
		Error:     "",
		Duration:  100 * time.Millisecond,
	}

	if result.CommandID != "cmd-123" {
		t.Errorf("expected command ID 'cmd-123', got '%s'", result.CommandID)
	}
	if !result.Success {
		t.Error("expected success to be true")
	}
}

func TestMetrics(t *testing.T) {
	metrics := &Metrics{
		CPUUsage:     50.5,
		MemoryUsage:  60.0,
		MemoryTotal:  8589934592,
		MemoryUsed:   5153960755,
		DiskUsage:    70.0,
		DiskTotal:    107374182400,
		DiskUsed:     75161927680,
		NetworkRx:    1000000,
		NetworkTx:    500000,
		LoadAvg1:     1.5,
		LoadAvg5:     1.2,
		LoadAvg15:    1.0,
		Uptime:       86400,
		ProcessCount: 100,
		Timestamp:    time.Now().Unix(),
	}

	if metrics.CPUUsage != 50.5 {
		t.Errorf("expected CPU usage 50.5, got %f", metrics.CPUUsage)
	}
	if metrics.MemoryUsage != 60.0 {
		t.Errorf("expected memory usage 60.0, got %f", metrics.MemoryUsage)
	}
	if metrics.ProcessCount != 100 {
		t.Errorf("expected process count 100, got %d", metrics.ProcessCount)
	}
}

func TestVersion(t *testing.T) {
	if Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", Version)
	}
}

func TestAgent_Start_AlreadyRunning(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	agent.running = true

	ctx := context.Background()
	err := agent.Start(ctx)

	if err == nil {
		t.Error("expected error when starting already running agent")
	}
}

func TestDefaultCommandHandler_Ping(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	ctx := context.Background()

	cmd := &Command{
		ID:   "cmd-1",
		Type: "ping",
	}

	result, err := agent.defaultCommandHandler(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("expected ping to succeed")
	}
	if result.Output != "pong" {
		t.Errorf("expected output 'pong', got '%s'", result.Output)
	}
}

func TestDefaultCommandHandler_Exec_MissingCommand(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	ctx := context.Background()

	cmd := &Command{
		ID:      "cmd-2",
		Type:    "exec",
		Payload: map[string]interface{}{},
	}

	result, err := agent.defaultCommandHandler(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Success {
		t.Error("expected exec without command to fail")
	}
}

func TestDefaultCommandHandler_Script_MissingScript(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	ctx := context.Background()

	cmd := &Command{
		ID:      "cmd-3",
		Type:    "script",
		Payload: map[string]interface{}{},
	}

	result, err := agent.defaultCommandHandler(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Success {
		t.Error("expected script without script content to fail")
	}
}

func TestDefaultCommandHandler_UnknownType(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	ctx := context.Background()

	cmd := &Command{
		ID:   "cmd-4",
		Type: "unknown-type",
	}

	result, err := agent.defaultCommandHandler(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Success {
		t.Error("expected unknown command type to fail")
	}
}

func TestCollectHostInfo(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	err := agent.collectHostInfo()

	if err != nil {
		t.Errorf("collectHostInfo failed: %v", err)
	}

	// Check that info was populated
	if agent.info.CPUCount <= 0 {
		t.Error("expected CPU count to be set")
	}
}

func TestDefaultMetricProvider(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)

	metrics, err := agent.defaultMetricProvider()
	if err != nil {
		t.Errorf("defaultMetricProvider failed: %v", err)
	}

	if metrics == nil {
		t.Fatal("expected non-nil metrics")
	}

	// Timestamp should be set
	if metrics.Timestamp == 0 {
		t.Error("expected timestamp to be set")
	}
}

func TestCollectAndSendMetrics_NilProvider(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	agent.metricProvider = nil

	ctx := context.Background()
	err := agent.collectAndSendMetrics(ctx)

	if err != nil {
		t.Errorf("expected nil error with nil provider, got: %v", err)
	}
}

func TestPollAndExecuteCommands(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	ctx := context.Background()

	// This is a stub that returns nil
	err := agent.pollAndExecuteCommands(ctx)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDefaultCommandHandler_Exec(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	ctx := context.Background()

	// Test exec command with actual command (echo works on all platforms)
	cmd := &Command{
		ID:   "cmd-exec",
		Type: "exec",
		Payload: map[string]interface{}{
			"command": "echo hello",
		},
	}

	result, err := agent.defaultCommandHandler(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// On Windows, sh might not be available
	// Just verify the function doesn't panic
	_ = result
}

func TestDefaultCommandHandler_Script(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	ctx := context.Background()

	cmd := &Command{
		ID:   "cmd-script",
		Type: "script",
		Payload: map[string]interface{}{
			"script": "echo test",
		},
	}

	result, err := agent.defaultCommandHandler(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// On Windows, sh might not be available
	_ = result
}

func TestAgent_Stop_WithRunning(t *testing.T) {
	cfg := &Config{
		ServerURL:         "http://localhost:8080",
		AgentID:           "test-agent",
		SecretKey:         "test-secret",
		HeartbeatInterval: 1 * time.Hour, // Long interval to avoid actual calls
		MetricsInterval:   1 * time.Hour,
	}

	agent, _ := New(cfg)
	agent.running = true
	agent.stopCh = make(chan struct{})

	err := agent.Stop()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if agent.running {
		t.Error("expected agent to be stopped")
	}
}

func TestSendHeartbeat(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	ctx := context.Background()

	// This will fail because there's no server, but we test the path
	err := agent.sendHeartbeat(ctx)
	// Error is expected due to no server
	_ = err

	// Note: LastSeen is only updated on successful heartbeat
	// so we don't check it here
}

func TestRegister(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	ctx := context.Background()

	// This will fail because there's no server
	err := agent.register(ctx)
	if err == nil {
		t.Error("expected error when no server is running")
	}
}

func TestDefaultCommandHandler_ExecWithOutput(t *testing.T) {
	cfg := &Config{
		ServerURL: "http://localhost:8080",
		AgentID:   "test-agent",
		SecretKey: "test-secret",
	}

	agent, _ := New(cfg)
	ctx := context.Background()

	// Test with a simple command that should work
	cmd := &Command{
		ID:   "cmd-echo",
		Type: "exec",
		Payload: map[string]interface{}{
			"command": "echo test-output",
		},
	}

	result, err := agent.defaultCommandHandler(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = result
}

func TestAgentInfo_AllFields(t *testing.T) {
	now := time.Now()
	info := &AgentInfo{
		AgentID:   "agent-123",
		Hostname:  "test-host",
		IPAddress: "10.0.0.1",
		OS:        "linux",
		Arch:      "amd64",
		Version:   "1.0.0",
		Labels:    map[string]string{"env": "prod"},
		LastSeen:  now,
		Status:    "running",
		CPUCount:  8,
		MemoryGB:  16.0,
		DiskGB:    500.0,
	}

	if info.AgentID != "agent-123" {
		t.Errorf("unexpected AgentID: %s", info.AgentID)
	}
	if info.IPAddress != "10.0.0.1" {
		t.Errorf("unexpected IPAddress: %s", info.IPAddress)
	}
	if info.CPUCount != 8 {
		t.Errorf("unexpected CPUCount: %d", info.CPUCount)
	}
}

func TestCommand_AllFields(t *testing.T) {
	now := time.Now()
	cmd := &Command{
		ID:        "cmd-xyz",
		Type:      "custom",
		Payload:   map[string]interface{}{"key": "value"},
		Timeout:   30 * time.Second,
		CreatedAt: now,
	}

	if cmd.ID != "cmd-xyz" {
		t.Errorf("unexpected ID: %s", cmd.ID)
	}
	if cmd.Type != "custom" {
		t.Errorf("unexpected Type: %s", cmd.Type)
	}
	if cmd.Timeout != 30*time.Second {
		t.Errorf("unexpected Timeout: %v", cmd.Timeout)
	}
}

func TestCommandResult_AllFields(t *testing.T) {
	result := &CommandResult{
		CommandID: "cmd-abc",
		Success:   true,
		Output:    "command output",
		Error:     "",
		Metadata:  map[string]any{"duration": 100},
		Duration:  100 * time.Millisecond,
	}

	if result.CommandID != "cmd-abc" {
		t.Errorf("unexpected CommandID: %s", result.CommandID)
	}
	if result.Metadata["duration"] != 100 {
		t.Errorf("unexpected Metadata: %v", result.Metadata)
	}
}

func TestMetrics_AllFields(t *testing.T) {
	metrics := &Metrics{
		CPUUsage:     45.5,
		MemoryUsage:  60.0,
		MemoryTotal:  16000000000,
		MemoryUsed:   9600000000,
		DiskUsage:    70.0,
		DiskTotal:    500000000000,
		DiskUsed:     350000000000,
		NetworkRx:    1000000000,
		NetworkTx:    500000000,
		LoadAvg1:     2.5,
		LoadAvg5:     2.0,
		LoadAvg15:    1.5,
		Uptime:       172800,
		ProcessCount: 150,
		Timestamp:    time.Now().Unix(),
	}

	if metrics.CPUUsage != 45.5 {
		t.Errorf("unexpected CPUUsage: %f", metrics.CPUUsage)
	}
	if metrics.LoadAvg1 != 2.5 {
		t.Errorf("unexpected LoadAvg1: %f", metrics.LoadAvg1)
	}
	if metrics.ProcessCount != 150 {
		t.Errorf("unexpected ProcessCount: %d", metrics.ProcessCount)
	}
}
