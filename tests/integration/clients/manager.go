package clients

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Manager 客户端管理器
type Manager struct {
	mu      sync.RWMutex
	clients map[string]Client
}

// NewManager 创建客户端管理器
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]Client),
	}
}

// Add 添加客户端
func (m *Manager) Add(name string, client Client) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clients[name]; exists {
		return fmt.Errorf("client %s already exists", name)
	}

	m.clients[name] = client
	return nil
}

// Get 获取客户端
func (m *Manager) Get(name string) (Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, exists := m.clients[name]
	if !exists {
		return nil, fmt.Errorf("client %s not found", name)
	}

	return client, nil
}

// Remove 移除客户端
func (m *Manager) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, exists := m.clients[name]
	if !exists {
		return fmt.Errorf("client %s not found", name)
	}

	// 停止客户端
	_ = client.Stop()
	delete(m.clients, name)

	return nil
}

// StartAll 启动所有客户端
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errs []error
	var wg sync.WaitGroup

	for name, client := range m.clients {
		wg.Add(1)
		go func(n string, c Client) {
			defer wg.Done()
			if err := c.Start(ctx); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", n, err))
			}
		}(name, client)
	}

	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("errors starting clients: %v", errs)
	}

	return nil
}

// StopAll 停止所有客户端
func (m *Manager) StopAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errs []error
	var wg sync.WaitGroup

	for name, client := range m.clients {
		wg.Add(1)
		go func(n string, c Client) {
			defer wg.Done()
			if err := c.Stop(); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", n, err))
			}
		}(name, client)
	}

	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("errors stopping clients: %v", errs)
	}

	return nil
}

// List 列出所有客户端
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}

	return names
}

// Status 获取所有客户端状态
func (m *Manager) Status() map[string]ClientStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]ClientStatus)
	for name, client := range m.clients {
		status[name] = client.Status()
	}

	return status
}

// HealthyClients 返回健康的客户端列表
func (m *Manager) HealthyClients(ctx context.Context) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var healthy []string
	for name, client := range m.clients {
		if client.IsHealthy(ctx) {
			healthy = append(healthy, name)
		}
	}

	return healthy
}

// UnhealthyClients 返回不健康的客户端列表
func (m *Manager) UnhealthyClients(ctx context.Context) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var unhealthy []string
	for name, client := range m.clients {
		if !client.IsHealthy(ctx) {
			unhealthy = append(unhealthy, name)
		}
	}

	return unhealthy
}

// HealthCheck 对所有客户端进行健康检查
func (m *Manager) HealthCheck(ctx context.Context) map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]bool)
	for name, client := range m.clients {
		results[name] = client.IsHealthy(ctx)
	}

	return results
}

// RestartUnhealthy 重启不健康的客户端
func (m *Manager) RestartUnhealthy(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errs []error

	for name, client := range m.clients {
		if !client.IsHealthy(ctx) {
			if err := client.Restart(ctx); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors restarting clients: %v", errs)
	}

	return nil
}

// Watch 监控客户端状态
func (m *Manager) Watch(ctx context.Context, interval time.Duration, callback func(name string, healthy bool)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.RLock()
			for name, client := range m.clients {
				healthy := client.IsHealthy(ctx)
				callback(name, healthy)
			}
			m.mu.RUnlock()
		}
	}
}