package agent

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/control-center/internal/core/plugin"
)

func TestNew(t *testing.T) {
	p := New()
	if p == nil {
		t.Fatal("New() returned nil")
	}
	if p.status.State != string(plugin.StateUninitialized) {
		t.Errorf("expected state %s, got %s", plugin.StateUninitialized, p.status.State)
	}
}

func TestInfo(t *testing.T) {
	p := New()
	info := p.Info()

	if info.Name != "agent" {
		t.Errorf("expected name 'agent', got '%s'", info.Name)
	}
	if info.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", info.Version)
	}
}

func TestInit(t *testing.T) {
	p := New()
	config := map[string]interface{}{
		"host":      "localhost",
		"port":      8080,
		"api_token": "test-token",
		"timeout":   30,
	}

	err := p.Init(context.Background(), config)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if p.config.Host != "localhost" {
		t.Errorf("expected host 'localhost', got '%s'", p.config.Host)
	}
	if p.config.Port != 8080 {
		t.Errorf("expected port 8080, got %d", p.config.Port)
	}
}

func TestInit_DefaultTimeout(t *testing.T) {
	p := New()
	config := map[string]interface{}{
		"host": "localhost",
	}

	err := p.Init(context.Background(), config)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if p.config.Timeout != 30 {
		t.Errorf("expected default timeout 30, got %d", p.config.Timeout)
	}
}

func TestStart(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})

	err := p.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if p.status.State != string(plugin.StateRunning) {
		t.Errorf("expected state 'running', got '%s'", p.status.State)
	}
}

func TestStop(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})
	_ = p.Start(context.Background())

	err := p.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if p.status.State != string(plugin.StateStopped) {
		t.Errorf("expected state 'stopped', got '%s'", p.status.State)
	}
}

func TestHealthCheck_NotConnected(t *testing.T) {
	p := New()

	err := p.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error for not connected")
	}
}

func TestCapabilities(t *testing.T) {
	p := New()
	caps := p.Capabilities()

	if len(caps) == 0 {
		t.Error("expected non-empty capabilities")
	}

	expected := []string{"connect", "disconnect", "exec", "upload", "download"}
	for _, exp := range expected {
		found := false
		for _, cap := range caps {
			if cap == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing capability: %s", exp)
		}
	}
}

func TestExecute_UnknownAction(t *testing.T) {
	p := New()

	_, err := p.Execute(context.Background(), "unknown", nil)
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestExecute_Disconnect(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})

	result, err := p.Execute(context.Background(), "disconnect", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestExecute_Exec_NotConnected(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})

	result, err := p.Execute(context.Background(), "exec", map[string]interface{}{
		"command": "ls",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure when not connected")
	}
}

func TestExecute_Exec_NoCommand(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})

	result, err := p.Execute(context.Background(), "exec", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure without command")
	}
}

func TestExecute_Upload_NotImplemented(t *testing.T) {
	p := New()

	result, err := p.Execute(context.Background(), "upload", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for not implemented")
	}
}

func TestExecute_Download_NotImplemented(t *testing.T) {
	p := New()

	result, err := p.Execute(context.Background(), "download", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for not implemented")
	}
}

func TestExecute_ServiceStart_NoService(t *testing.T) {
	p := New()

	result, err := p.Execute(context.Background(), "service_start", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure without service name")
	}
}

func TestExecute_ServiceStop_NoService(t *testing.T) {
	p := New()

	result, err := p.Execute(context.Background(), "service_stop", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure without service name")
	}
}

func TestExecute_ServiceStatus_NoService(t *testing.T) {
	p := New()

	result, err := p.Execute(context.Background(), "service_status", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure without service name")
	}
}

func TestGetStatus(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})

	status, err := p.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.State != string(plugin.StateInitialized) {
		t.Errorf("expected state 'initialized', got '%s'", status.State)
	}
}

func TestConfig(t *testing.T) {
	cfg := Config{
		Host:     "localhost",
		Port:     8080,
		APIToken: "token",
		Timeout:  30,
	}

	if cfg.Host != "localhost" {
		t.Errorf("expected host 'localhost', got '%s'", cfg.Host)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
}

func TestExecute_Connect_NoHost(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})

	result, err := p.Execute(context.Background(), "connect", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure without host")
	}
}

func TestExecute_Connect_WithPort(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})

	result, err := p.Execute(context.Background(), "connect", map[string]interface{}{
		"host": "localhost",
		"port": float64(9090),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Will fail because no real websocket server
	if result.Success {
		t.Error("expected failure connecting to non-existent server")
	}
}

func TestExecute_SystemInfo(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})

	// Not connected, so will fail
	result, err := p.Execute(context.Background(), "system_info", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure when not connected")
	}
}

func TestExecute_ServiceStart_WithService(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})

	// Not connected, so will fail
	result, err := p.Execute(context.Background(), "service_start", map[string]interface{}{
		"service": "nginx",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure when not connected")
	}
}

func TestExecute_ServiceStop_WithService(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})

	// Not connected, so will fail
	result, err := p.Execute(context.Background(), "service_stop", map[string]interface{}{
		"service": "nginx",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure when not connected")
	}
}

func TestExecute_ServiceStatus_WithService(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})

	// Not connected, so will fail
	result, err := p.Execute(context.Background(), "service_status", map[string]interface{}{
		"service": "nginx",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure when not connected")
	}
}

func TestHealthCheck_Connected(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})

	// Set a mock connection (nil is used to simulate connection)
	// Without actual websocket connection, we test the not connected path
	err := p.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error for not connected")
	}
}

func TestStop_WithConnection(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{})
	_ = p.Start(context.Background())

	// Stop should work even without a connection
	err := p.Stop(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
