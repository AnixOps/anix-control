package gost

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
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
		require.NoError(t, database.Close())
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
