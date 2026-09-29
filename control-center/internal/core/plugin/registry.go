package plugin

import (
	"fmt"
	"sync"
)

// Registry provides a global plugin registry
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// Factory creates plugin instances
type Factory func() Plugin

// Global registry instance
var globalRegistry = NewRegistry()

// NewRegistry creates a new plugin registry
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
	}
}

// RegisterFactory registers a plugin factory
func (r *Registry) RegisterFactory(name string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// GetFactory retrieves a plugin factory
func (r *Registry) GetFactory(name string) (Factory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[name]
	return f, ok
}

// ListFactories returns all registered factory names
func (r *Registry) ListFactories() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// Create creates a plugin instance using a registered factory
func (r *Registry) Create(name string) (Plugin, error) {
	factory, ok := r.GetFactory(name)
	if !ok {
		return nil, fmt.Errorf("plugin factory %s not found", name)
	}
	return factory(), nil
}

// Global registry functions

// RegisterFactory registers a plugin factory with the global registry
func RegisterFactory(name string, factory Factory) {
	globalRegistry.RegisterFactory(name, factory)
}

// GetFactory retrieves a plugin factory from the global registry
func GetFactory(name string) (Factory, bool) {
	return globalRegistry.GetFactory(name)
}

// ListFactories returns all registered factory names from the global registry
func ListFactories() []string {
	return globalRegistry.ListFactories()
}

// Create creates a plugin instance using the global registry
func Create(name string) (Plugin, error) {
	return globalRegistry.Create(name)
}
