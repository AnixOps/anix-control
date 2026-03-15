package gost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) func() {
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}

	err := database.Init(cfg)
	require.NoError(t, err)

	// Auto migrate
	err = database.AutoMigrate(&model.ForwardNode{}, &model.ForwardRule{})
	require.NoError(t, err)

	return func() {
		database.Close()
	}
}

func TestNewManager(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	assert.NotNil(t, manager)
	assert.Equal(t, database.GetDB(), manager.db)
}

func TestManager_GetClient_NotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	_, err := manager.GetClient(999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node not found")
}

func TestManager_GetClient_Success(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	// Create test node
	node := &model.ForwardNode{
		Name:     "test-node",
		Host:     "192.168.1.1",
		Port:     8080,
		APIPort:  18080,
		APIToken: "test-token",
	}
	database.GetDB().Create(node)

	// We can't actually connect to the gost API, but we can test client creation
	client, err := manager.GetClient(node.ID)
	assert.NoError(t, err)
	assert.NotNil(t, client)

	// Second call should return cached client
	client2, err := manager.GetClient(node.ID)
	assert.NoError(t, err)
	assert.Equal(t, client, client2) // Same instance
}

func TestManager_GetClient_NoAPIPort(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	// Create node without API port
	node := &model.ForwardNode{
		Name: "test-node",
		Host: "192.168.1.1",
		Port: 8080,
	}
	database.GetDB().Create(node)

	_, err := manager.GetClient(node.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API port not configured")
}

func TestManager_RegisterNode(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	node := &model.ForwardNode{
		ID:       1,
		Name:     "test-node",
		Host:     "192.168.1.1",
		Port:     8080,
		APIPort:  18080,
		APIToken: "test-token",
	}

	err := manager.RegisterNode(node)
	assert.NoError(t, err)

	// Should be able to get client after registration
	client, err := manager.GetClient(1)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestManager_UnregisterNode(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	node := &model.ForwardNode{
		ID:       1,
		Name:     "test-node",
		Host:     "192.168.1.1",
		Port:     8080,
		APIPort:  18080,
		APIToken: "test-token",
	}

	// Register then unregister
	manager.RegisterNode(node)
	manager.UnregisterNode(1)

	// Create node in DB so GetClient doesn't fail on DB lookup
	database.GetDB().Create(node)

	// Client should still be creatable from DB (not cached)
	client, err := manager.GetClient(1)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestManager_CreateForwardRule(t *testing.T) {
	// Create mock gost server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/config/services" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	// Create relay node
	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	// Create exit node
	exitNode := &model.ForwardNode{
		Name: "exit-node",
		Host: "192.168.2.1",
		Port: 8081,
	}
	database.GetDB().Create(exitNode)

	// Create rule
	rule := &model.ForwardRule{
		RelayNodeID: relayNode.ID,
		ExitNodeID:  exitNode.ID,
		ListenPort:  9000,
		TargetHost:  "10.0.0.1",
		TargetPort:  80,
		Protocol:    "tcp",
		Enabled:     true,
	}
	database.GetDB().Create(rule)

	// Manually create client for test server URL
	manager.clients.Store(relayNode.ID, NewClient(&Config{
		Host: server.URL,
	}))

	err := manager.CreateForwardRule(context.Background(), rule)
	assert.NoError(t, err)
}

func TestManager_CreateForwardRule_UDP(t *testing.T) {
	serviceCreated := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/config/services" && r.Method == http.MethodPost {
			serviceCreated++
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	exitNode := &model.ForwardNode{
		Name: "exit-node",
		Host: "192.168.2.1",
		Port: 8081,
	}
	database.GetDB().Create(exitNode)

	rule := &model.ForwardRule{
		RelayNodeID: relayNode.ID,
		ExitNodeID:  exitNode.ID,
		ListenPort:  9000,
		TargetHost:  "10.0.0.1",
		TargetPort:  80,
		Protocol:    "udp",
		Enabled:     true,
	}
	database.GetDB().Create(rule)

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	err := manager.CreateForwardRule(context.Background(), rule)
	assert.NoError(t, err)
	assert.Equal(t, 1, serviceCreated)
}

func TestManager_CreateForwardRule_Both(t *testing.T) {
	serviceCreated := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/config/services" && r.Method == http.MethodPost {
			serviceCreated++
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	exitNode := &model.ForwardNode{
		Name: "exit-node",
		Host: "192.168.2.1",
		Port: 8081,
	}
	database.GetDB().Create(exitNode)

	rule := &model.ForwardRule{
		RelayNodeID: relayNode.ID,
		ExitNodeID:  exitNode.ID,
		ListenPort:  9000,
		TargetHost:  "10.0.0.1",
		TargetPort:  80,
		Protocol:    "both",
		Enabled:     true,
	}
	database.GetDB().Create(rule)

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	err := manager.CreateForwardRule(context.Background(), rule)
	assert.NoError(t, err)
	assert.Equal(t, 2, serviceCreated) // TCP + UDP
}

func TestManager_DeleteForwardRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	rule := &model.ForwardRule{
		ID:          1,
		RelayNodeID: relayNode.ID,
		ListenPort:  9000,
		Protocol:    "tcp",
	}

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	err := manager.DeleteForwardRule(context.Background(), rule)
	assert.NoError(t, err)
}

func TestManager_UpdateForwardRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	exitNode := &model.ForwardNode{
		Name: "exit-node",
		Host: "192.168.2.1",
		Port: 8081,
	}
	database.GetDB().Create(exitNode)

	rule := &model.ForwardRule{
		ID:          1,
		RelayNodeID: relayNode.ID,
		ExitNodeID:  exitNode.ID,
		ListenPort:  9000,
		TargetHost:  "10.0.0.1",
		TargetPort:  80,
		Protocol:    "tcp",
	}

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	err := manager.UpdateForwardRule(context.Background(), rule)
	assert.NoError(t, err)
}

func TestManager_SyncRuleToGost_Enable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	exitNode := &model.ForwardNode{
		Name: "exit-node",
		Host: "192.168.2.1",
		Port: 8081,
	}
	database.GetDB().Create(exitNode)

	rule := &model.ForwardRule{
		RelayNodeID: relayNode.ID,
		ExitNodeID:  exitNode.ID,
		ListenPort:  9000,
		TargetHost:  "10.0.0.1",
		TargetPort:  80,
		Protocol:    "tcp",
		Enabled:     true,
	}

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	err := manager.SyncRuleToGost(context.Background(), rule)
	assert.NoError(t, err)
}

func TestManager_SyncRuleToGost_Disable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	rule := &model.ForwardRule{
		ID:          1,
		RelayNodeID: relayNode.ID,
		ListenPort:  9000,
		Protocol:    "tcp",
		Enabled:     false,
	}

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	err := manager.SyncRuleToGost(context.Background(), rule)
	assert.NoError(t, err)
}

func TestManager_GetNodeStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stats" {
			stats := StatsResponse{
				Services: []ServiceStats{
					{Name: "service-1", Addr: ":8080"},
				},
			}
			json.NewEncoder(w).Encode(stats)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	stats, err := manager.GetNodeStats(context.Background(), relayNode.ID)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Len(t, stats.Services, 1)
}

func TestManager_GetRuleStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stats/services/forward-rule-1" {
			stats := ServiceStats{
				Name: "forward-rule-1",
				Addr: ":9000",
			}
			json.NewEncoder(w).Encode(stats)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	rule := &model.ForwardRule{
		ID:          1,
		RelayNodeID: relayNode.ID,
	}

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	stats, err := manager.GetRuleStats(context.Background(), rule)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, "forward-rule-1", stats.Name)
}

func TestManager_CheckNodeHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	err := manager.CheckNodeHealth(context.Background(), relayNode.ID)
	assert.NoError(t, err)
}

func TestManager_GetServiceName(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	rule := &model.ForwardRule{
		ID: 123,
	}

	name := manager.getServiceName(rule)
	assert.Equal(t, "forward-rule-123", name)
}

func TestManager_SyncAllRules(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	exitNode := &model.ForwardNode{
		Name: "exit-node",
		Host: "192.168.2.1",
		Port: 8081,
	}
	database.GetDB().Create(exitNode)

	// Create enabled rule
	rule := &model.ForwardRule{
		RelayNodeID: relayNode.ID,
		ExitNodeID:  exitNode.ID,
		ListenPort:  9000,
		TargetHost:  "10.0.0.1",
		TargetPort:  80,
		Protocol:    "tcp",
		Enabled:     true,
	}
	database.GetDB().Create(rule)

	// Create disabled rule (should not be synced)
	disabledRule := &model.ForwardRule{
		RelayNodeID: relayNode.ID,
		ExitNodeID:  exitNode.ID,
		ListenPort:  9001,
		TargetHost:  "10.0.0.2",
		TargetPort:  80,
		Protocol:    "tcp",
		Enabled:     false,
	}
	database.GetDB().Create(disabledRule)

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	err := manager.SyncAllRules(context.Background())
	assert.NoError(t, err)
}

func TestManager_CreateForwardRule_RelayNodeNotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	exitNode := &model.ForwardNode{
		Name: "exit-node",
		Host: "192.168.2.1",
		Port: 8081,
	}
	database.GetDB().Create(exitNode)

	rule := &model.ForwardRule{
		RelayNodeID: 999, // Non-existent
		ExitNodeID:  exitNode.ID,
		ListenPort:  9000,
		TargetHost:  "10.0.0.1",
		TargetPort:  80,
		Protocol:    "tcp",
		Enabled:     true,
	}

	err := manager.CreateForwardRule(context.Background(), rule)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get relay node client")
}

func TestManager_CreateForwardRule_ExitNodeNotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: "http://localhost:8080"}))

	rule := &model.ForwardRule{
		RelayNodeID: relayNode.ID,
		ExitNodeID:  999, // Non-existent
		ListenPort:  9000,
		TargetHost:  "10.0.0.1",
		TargetPort:  80,
		Protocol:    "tcp",
		Enabled:     true,
	}

	err := manager.CreateForwardRule(context.Background(), rule)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exit node not found")
}

func TestManager_CreateChain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/config/hops" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			return
		}
		if r.URL.Path == "/api/config/chains" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	// Create relay node
	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	// Create exit node
	exitNode := &model.ForwardNode{
		Name:     "exit-node",
		Host:     "192.168.2.1",
		Port:     9090,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(exitNode)

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	err := manager.CreateChain(context.Background(), relayNode.ID, exitNode.ID, "test-chain")
	assert.NoError(t, err)
}

func TestManager_CreateChain_RelayNodeNotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	exitNode := &model.ForwardNode{
		Name: "exit-node",
		Host: "192.168.2.1",
		Port: 9090,
	}
	database.GetDB().Create(exitNode)

	err := manager.CreateChain(context.Background(), 999, exitNode.ID, "test-chain")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "relay node not found")
}

func TestManager_CreateChain_ExitNodeNotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	err := manager.CreateChain(context.Background(), relayNode.ID, 999, "test-chain")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exit node not found")
}

func TestManager_CreateChain_GetClientError(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	// Create nodes without registering client
	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  18080,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	exitNode := &model.ForwardNode{
		Name: "exit-node",
		Host: "192.168.2.1",
		Port: 9090,
	}
	database.GetDB().Create(exitNode)

	// No client registered, will fail
	err := manager.CreateChain(context.Background(), relayNode.ID, exitNode.ID, "test-chain")
	assert.Error(t, err)
}

func TestManager_CreateChain_CreateHopError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/config/hops" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	exitNode := &model.ForwardNode{
		Name: "exit-node",
		Host: "192.168.2.1",
		Port: 9090,
	}
	database.GetDB().Create(exitNode)

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	err := manager.CreateChain(context.Background(), relayNode.ID, exitNode.ID, "test-chain")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "create hop")
}

func TestManager_DeleteForwardRule_NoClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	// Create node without API port - GetClient will fail
	relayNode := &model.ForwardNode{
		Name: "relay-node",
		Host: "127.0.0.1",
		Port: 8080,
		// No APIPort
	}
	database.GetDB().Create(relayNode)

	rule := &model.ForwardRule{
		ID:          1,
		RelayNodeID: relayNode.ID,
		ListenPort:  9000,
		Protocol:    "tcp",
	}

	// GetClient will fail because node has no APIPort
	err := manager.DeleteForwardRule(context.Background(), rule)
	assert.Error(t, err)
}

func TestManager_CheckNodeHealth_NodeNotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	err := manager.CheckNodeHealth(context.Background(), 999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node not found")
}

func TestManager_GetNodeStats_NodeNotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	_, err := manager.GetNodeStats(context.Background(), 999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node not found")
}

func TestManager_GetRuleStats_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	relayNode := &model.ForwardNode{
		Name:     "relay-node",
		Host:     "127.0.0.1",
		Port:     8080,
		APIPort:  80,
		APIToken: "test-token",
	}
	database.GetDB().Create(relayNode)

	rule := &model.ForwardRule{
		ID:          1,
		RelayNodeID: relayNode.ID,
	}

	manager.clients.Store(relayNode.ID, NewClient(&Config{Host: server.URL}))

	stats, err := manager.GetRuleStats(context.Background(), rule)
	assert.Error(t, err)
	assert.Nil(t, stats)
}

func TestManager_SyncAllRules_Empty(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database.GetDB())

	err := manager.SyncAllRules(context.Background())
	assert.NoError(t, err)
}