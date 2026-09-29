package plugin

import (
	"context"
	"fmt"
	"sync"
)

// Errors
var (
	ErrPluginNotFound      = fmt.Errorf("plugin not found")
	ErrPluginNotExecutable = fmt.Errorf("plugin not executable")
)

// Manager manages plugin lifecycle
type Manager struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
	configs map[string]map[string]interface{}
	states  map[string]PluginState
}

// NewManager creates a new plugin manager
func NewManager() *Manager {
	return &Manager{
		plugins: make(map[string]Plugin),
		configs: make(map[string]map[string]interface{}),
		states:  make(map[string]PluginState),
	}
}

// Register registers a plugin with the manager
func (m *Manager) Register(name string, plugin Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	m.plugins[name] = plugin
	m.states[name] = StateUninitialized
	return nil
}

// Unregister removes a plugin from the manager
func (m *Manager) Unregister(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	// Stop plugin if running
	if m.states[name] == StateRunning {
		ctx := context.Background()
		if err := plugin.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop plugin %s: %w", name, err)
		}
	}

	delete(m.plugins, name)
	delete(m.configs, name)
	delete(m.states, name)
	return nil
}

// Get retrieves a plugin by name
func (m *Manager) Get(name string) (Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plugins[name]
	return p, ok
}

// GetState returns the current state of a plugin
func (m *Manager) GetState(name string) PluginState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states[name]
}

// SetConfig sets the configuration for a plugin
func (m *Manager) SetConfig(name string, config map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configs[name] = config
}

// GetConfig returns the configuration for a plugin
func (m *Manager) GetConfig(name string) map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configs[name]
}

// InitPlugin initializes a specific plugin
func (m *Manager) InitPlugin(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	config := m.configs[name]
	if err := plugin.Init(ctx, config); err != nil {
		m.states[name] = StateError
		return fmt.Errorf("failed to init plugin %s: %w", name, err)
	}

	m.states[name] = StateInitialized
	return nil
}

// StartPlugin starts a specific plugin
func (m *Manager) StartPlugin(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	if m.states[name] != StateInitialized {
		config := m.configs[name]
		if err := plugin.Init(ctx, config); err != nil {
			m.states[name] = StateError
			return fmt.Errorf("failed to init plugin %s: %w", name, err)
		}
		m.states[name] = StateInitialized
	}

	if err := plugin.Start(ctx); err != nil {
		m.states[name] = StateError
		return fmt.Errorf("failed to start plugin %s: %w", name, err)
	}

	m.states[name] = StateRunning
	return nil
}

// StopPlugin stops a specific plugin
func (m *Manager) StopPlugin(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	if err := plugin.Stop(ctx); err != nil {
		m.states[name] = StateError
		return fmt.Errorf("failed to stop plugin %s: %w", name, err)
	}

	m.states[name] = StateStopped
	return nil
}

// InitAll initializes all registered plugins
func (m *Manager) InitAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, plugin := range m.plugins {
		config := m.configs[name]
		if err := plugin.Init(ctx, config); err != nil {
			m.states[name] = StateError
			return fmt.Errorf("failed to init plugin %s: %w", name, err)
		}
		m.states[name] = StateInitialized
	}
	return nil
}

// StartAll starts all registered plugins
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, plugin := range m.plugins {
		// Initialize if needed
		if m.states[name] != StateInitialized {
			config := m.configs[name]
			if err := plugin.Init(ctx, config); err != nil {
				m.states[name] = StateError
				return fmt.Errorf("failed to init plugin %s: %w", name, err)
			}
			m.states[name] = StateInitialized
		}

		// Start
		if err := plugin.Start(ctx); err != nil {
			m.states[name] = StateError
			return fmt.Errorf("failed to start plugin %s: %w", name, err)
		}
		m.states[name] = StateRunning
	}
	return nil
}

// StopAll stops all running plugins
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var lastErr error
	for name, plugin := range m.plugins {
		if m.states[name] == StateRunning {
			if err := plugin.Stop(ctx); err != nil {
				m.states[name] = StateError
				lastErr = fmt.Errorf("failed to stop plugin %s: %w", name, err)
			} else {
				m.states[name] = StateStopped
			}
		}
	}
	return lastErr
}

// List returns all registered plugin names
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.plugins))
	for name := range m.plugins {
		names = append(names, name)
	}
	return names
}

// ListByState returns plugins filtered by state
func (m *Manager) ListByState(state PluginState) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0)
	for name, s := range m.states {
		if s == state {
			names = append(names, name)
		}
	}
	return names
}

// HealthCheck performs health check on all plugins
func (m *Manager) HealthCheck(ctx context.Context) map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]error)
	for name, plugin := range m.plugins {
		results[name] = plugin.HealthCheck(ctx)
	}
	return results
}

// GetInfo returns info for all plugins
func (m *Manager) GetInfo() map[string]PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make(map[string]PluginInfo)
	for name, plugin := range m.plugins {
		infos[name] = plugin.Info()
	}
	return infos
}
