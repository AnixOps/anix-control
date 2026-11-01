package grpc

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/cache"
	"github.com/stretchr/testify/assert"
)

// TestServerStart 测试服务器启动
func TestServerStart(t *testing.T) {
	cache.InitMemory()
	requireInMemoryDatabase(t)
	defer requireDatabaseClosed(t)

	cfg := &ServerConfig{
		Host:             "127.0.0.1",
		Port:             0,
		KeepaliveTime:    30 * time.Second,
		KeepaliveTimeout: 10 * time.Second,
	}

	server := NewServer(cfg)
	err := server.Start()
	assert.NoError(t, err)

	// 验证服务器启动
	time.Sleep(200 * time.Millisecond)
	assert.NotNil(t, server.grpcServer)
	assert.NotNil(t, server.listener)

	// 停止服务器
	server.Stop()
}

// TestServerStop 测试服务器停止
func TestServerStop(t *testing.T) {
	cache.InitMemory()
	requireInMemoryDatabase(t)
	defer requireDatabaseClosed(t)

	server := NewServer(&ServerConfig{
		Host: "127.0.0.1",
		Port: 0,
	})

	err := server.Start()
	assert.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// 停止服务器
	server.Stop()

	// 服务器应该已停止
	assert.NotNil(t, server.grpcServer)
}
