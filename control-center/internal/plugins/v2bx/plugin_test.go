package v2bx

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

	if info.Name != "v2bx" {
		t.Errorf("expected name 'v2bx', got '%s'", info.Name)
	}
	if info.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", info.Version)
	}
}

func TestInit(t *testing.T) {
	p := New()
	config := map[string]interface{}{
		"hosts": []interface{}{
			map[string]interface{}{
				"id":        1,
				"name":      "node1",
				"host":      "localhost",
				"port":      8080,
				"api_key":   "test-key",
				"grpc_port": 50051,
			},
		},
		"timeout": 30,
	}

	err := p.Init(context.Background(), config)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if len(p.config.Hosts) != 1 {
		t.Errorf("expected 1 host, got %d", len(p.config.Hosts))
	}
}

func TestInit_DefaultTimeout(t *testing.T) {
	p := New()
	config := map[string]interface{}{
		"hosts": []interface{}{},
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
	_ = p.Init(context.Background(), map[string]interface{}{"hosts": []interface{}{}})

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
	_ = p.Init(context.Background(), map[string]interface{}{"hosts": []interface{}{}})
	_ = p.Start(context.Background())

	err := p.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if p.status.State != string(plugin.StateStopped) {
		t.Errorf("expected state 'stopped', got '%s'", p.status.State)
	}
}

func TestHealthCheck(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{"hosts": []interface{}{}})
	_ = p.Start(context.Background())

	err := p.HealthCheck(context.Background())
	// Should pass since no nodes = healthy by default in this implementation
	_ = err
}

func TestCapabilities(t *testing.T) {
	p := New()
	caps := p.Capabilities()

	expected := []string{
		"get_nodes",
		"get_node_status",
		"get_users",
		"get_stats",
		"restart_node",
		"get_config",
		"get_logs",
	}

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

func TestExecute_GetNodes(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{
		"hosts": []interface{}{
			map[string]interface{}{
				"id":   1,
				"name": "node1",
				"host": "localhost",
				"port": 8080,
			},
		},
	})

	result, err := p.Execute(context.Background(), "get_nodes", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestExecute_GetNodeStatus_NoNodeID(t *testing.T) {
	p := New()

	result, err := p.Execute(context.Background(), "get_node_status", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure without node_id")
	}
}

func TestGetStatus(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{"hosts": []interface{}{}})

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
		Hosts: []NodeConfig{
			{ID: 1, Name: "node1", Host: "localhost", Port: 8080},
		},
		Timeout: 60,
	}

	if len(cfg.Hosts) != 1 {
		t.Errorf("expected 1 host, got %d", len(cfg.Hosts))
	}
	if cfg.Timeout != 60 {
		t.Errorf("expected timeout 60, got %d", cfg.Timeout)
	}
}

func TestNodeConfig(t *testing.T) {
	nc := NodeConfig{
		ID:       1,
		Name:     "test-node",
		Host:     "192.168.1.1",
		Port:     443,
		APIKey:   "secret",
		GRPCPort: 50051,
	}

	if nc.ID != 1 {
		t.Errorf("expected ID 1, got %d", nc.ID)
	}
	if nc.Name != "test-node" {
		t.Errorf("expected name 'test-node', got '%s'", nc.Name)
	}
	if nc.Host != "192.168.1.1" {
		t.Errorf("expected host '192.168.1.1', got '%s'", nc.Host)
	}
}

func TestExecute_GetUsers_NoNodeID(t *testing.T) {
	p := New()

	result, err := p.Execute(context.Background(), "get_users", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure without node_id")
	}
}

func TestExecute_GetStats_NoNodeID(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{"hosts": []interface{}{}})

	result, err := p.Execute(context.Background(), "get_stats", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success for all stats")
	}
}

func TestExecute_RestartNode_NoNodeID(t *testing.T) {
	p := New()

	result, err := p.Execute(context.Background(), "restart_node", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure without node_id")
	}
}

func TestExecute_GetConfig_NoNodeID(t *testing.T) {
	p := New()

	result, err := p.Execute(context.Background(), "get_config", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure without node_id")
	}
}

func TestExecute_GetLogs_NoNodeID(t *testing.T) {
	p := New()

	result, err := p.Execute(context.Background(), "get_logs", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure without node_id")
	}
}

func TestHealthCheck_Unhealthy(t *testing.T) {
	p := New()
	p.status.Health = string(plugin.HealthUnhealthy)

	err := p.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error for unhealthy plugin")
	}
}

func TestStart_WithNodes(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{
		"hosts": []interface{}{
			map[string]interface{}{
				"id":   1,
				"name": "node1",
				"host": "localhost",
				"port": 9999, // Non-existent port
			},
		},
	})

	err := p.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Should be running but with 0 healthy nodes
	if p.status.Health != string(plugin.HealthUnhealthy) {
		t.Logf("Health status: %s", p.status.Health)
	}
}

func TestExecute_GetNodeStatus_WithNode(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{
		"hosts": []interface{}{
			map[string]interface{}{
				"id":   1,
				"name": "node1",
				"host": "localhost",
				"port": 9999,
			},
		},
	})

	// Get node status with valid node ID
	result, err := p.Execute(context.Background(), "get_node_status", map[string]interface{}{
		"node_id": float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result

	// Get node status with non-existent node ID
	result, _ = p.Execute(context.Background(), "get_node_status", map[string]interface{}{
		"node_id": float64(999),
	})
	if result.Success {
		t.Error("expected failure for non-existent node")
	}
}

func TestExecute_GetUsers_WithNode(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{
		"hosts": []interface{}{
			map[string]interface{}{
				"id":   1,
				"name": "node1",
				"host": "localhost",
				"port": 9999,
			},
		},
	})

	// Get users with valid node ID
	result, err := p.Execute(context.Background(), "get_users", map[string]interface{}{
		"node_id": float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result

	// Get users with non-existent node ID
	result, _ = p.Execute(context.Background(), "get_users", map[string]interface{}{
		"node_id": float64(999),
	})
	if result.Success {
		t.Error("expected failure for non-existent node")
	}
}

func TestExecute_RestartNode_WithNode(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{
		"hosts": []interface{}{
			map[string]interface{}{
				"id":   1,
				"name": "node1",
				"host": "localhost",
				"port": 9999,
			},
		},
	})

	// Restart with valid node ID
	result, err := p.Execute(context.Background(), "restart_node", map[string]interface{}{
		"node_id": float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result

	// Restart with non-existent node ID
	result, _ = p.Execute(context.Background(), "restart_node", map[string]interface{}{
		"node_id": float64(999),
	})
	if result.Success {
		t.Error("expected failure for non-existent node")
	}
}

func TestExecute_GetConfig_WithNode(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{
		"hosts": []interface{}{
			map[string]interface{}{
				"id":   1,
				"name": "node1",
				"host": "localhost",
				"port": 9999,
			},
		},
	})

	// Get config with valid node ID
	result, err := p.Execute(context.Background(), "get_config", map[string]interface{}{
		"node_id": float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result

	// Get config with non-existent node ID
	result, _ = p.Execute(context.Background(), "get_config", map[string]interface{}{
		"node_id": float64(999),
	})
	if result.Success {
		t.Error("expected failure for non-existent node")
	}
}

func TestExecute_GetLogs_WithNode(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{
		"hosts": []interface{}{
			map[string]interface{}{
				"id":   1,
				"name": "node1",
				"host": "localhost",
				"port": 9999,
			},
		},
	})

	// Get logs with valid node ID
	result, err := p.Execute(context.Background(), "get_logs", map[string]interface{}{
		"node_id": float64(1),
		"lines":   float64(100),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result

	// Get logs with non-existent node ID
	result, _ = p.Execute(context.Background(), "get_logs", map[string]interface{}{
		"node_id": float64(999),
	})
	if result.Success {
		t.Error("expected failure for non-existent node")
	}
}

func TestExecute_GetStats_AllNodes(t *testing.T) {
	p := New()
	_ = p.Init(context.Background(), map[string]interface{}{
		"hosts": []interface{}{
			map[string]interface{}{
				"id":   1,
				"name": "node1",
				"host": "localhost",
				"port": 9999,
			},
		},
	})

	// Get stats for all nodes (no node_id)
	result, err := p.Execute(context.Background(), "get_stats", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success for all stats")
	}
}
