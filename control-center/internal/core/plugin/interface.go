package plugin

import "context"

// Plugin is the base interface all plugins must implement
type Plugin interface {
	// Metadata
	Info() PluginInfo

	// Lifecycle
	Init(ctx context.Context, config map[string]interface{}) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	// Health
	HealthCheck(ctx context.Context) error

	// Capabilities
	Capabilities() []string
}

// PluginInfo contains plugin metadata
type PluginInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
}

// ExecutablePlugin can execute actions
type ExecutablePlugin interface {
	Plugin
	Execute(ctx context.Context, action string, params map[string]interface{}) (Result, error)
}

// Result represents execution result
type Result struct {
	Success bool                   `json:"success"`
	Data    interface{}            `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Metrics map[string]interface{} `json:"metrics,omitempty"`
}

// ObservablePlugin can report status
type ObservablePlugin interface {
	Plugin
	GetStatus(ctx context.Context) (Status, error)
	Watch(ctx context.Context) (<-chan Event, error)
}

// Status represents plugin status
type Status struct {
	State       string                 `json:"state"`   // running, stopped, error
	Health      string                 `json:"health"`  // healthy, degraded, unhealthy
	Metrics     map[string]interface{} `json:"metrics"` // plugin-specific metrics
	LastUpdated int64                  `json:"last_updated"`
}

// Event represents a plugin event
type Event struct {
	Type      string      `json:"type"`
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// PluginState represents the current state of a plugin
type PluginState string

const (
	StateUninitialized PluginState = "uninitialized"
	StateInitialized   PluginState = "initialized"
	StateRunning       PluginState = "running"
	StateStopped       PluginState = "stopped"
	StateError         PluginState = "error"
)

// HealthStatus represents the health of a plugin
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
)
