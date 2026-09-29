package plugin

import (
	"context"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.factories == nil {
		t.Error("factories map not initialized")
	}
}

func TestRegistry_RegisterFactory(t *testing.T) {
	r := NewRegistry()
	factory := func() Plugin {
		return &mockPlugin{name: "test"}
	}

	r.RegisterFactory("test", factory)

	if _, ok := r.GetFactory("test"); !ok {
		t.Error("factory not registered")
	}
}

func TestRegistry_GetFactory_NotFound(t *testing.T) {
	r := NewRegistry()

	_, ok := r.GetFactory("nonexistent")
	if ok {
		t.Error("expected false for nonexistent factory")
	}
}

func TestRegistry_ListFactories(t *testing.T) {
	r := NewRegistry()
	factory := func() Plugin { return &mockPlugin{} }

	r.RegisterFactory("plugin1", factory)
	r.RegisterFactory("plugin2", factory)

	list := r.ListFactories()
	if len(list) != 2 {
		t.Errorf("expected 2 factories, got %d", len(list))
	}
}

func TestRegistry_ListFactories_Empty(t *testing.T) {
	r := NewRegistry()

	list := r.ListFactories()
	if len(list) != 0 {
		t.Errorf("expected 0 factories, got %d", len(list))
	}
}

func TestRegistry_Create(t *testing.T) {
	r := NewRegistry()
	factory := func() Plugin { return &mockPlugin{name: "test"} }
	r.RegisterFactory("test", factory)

	p, err := r.Create("test")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if p == nil {
		t.Error("expected plugin instance")
	}
}

func TestRegistry_Create_NotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.Create("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent factory")
	}
}

func TestGlobalRegisterFactory(t *testing.T) {
	factory := func() Plugin { return &mockPlugin{name: "global-test"} }
	RegisterFactory("global-test", factory)

	f, ok := GetFactory("global-test")
	if !ok {
		t.Error("factory not registered globally")
	}
	if f == nil {
		t.Error("expected non-nil factory")
	}
}

func TestGlobalListFactories(t *testing.T) {
	// The global registry should have at least our test factory
	list := ListFactories()
	if list == nil {
		t.Error("expected non-nil list")
	}
}

func TestGlobalCreate(t *testing.T) {
	// Register a factory for the test
	RegisterFactory("create-test", func() Plugin { return &mockPlugin{name: "create-test"} })

	p, err := Create("create-test")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if p == nil {
		t.Error("expected plugin instance")
	}
}

func TestGlobalCreate_NotFound(t *testing.T) {
	_, err := Create("nonexistent-global")
	if err == nil {
		t.Error("expected error for nonexistent factory")
	}
}

// mockPlugin for testing
type mockPlugin struct {
	name string
}

func (m *mockPlugin) Info() PluginInfo {
	return PluginInfo{Name: m.name}
}

func (m *mockPlugin) Init(ctx context.Context, config map[string]interface{}) error {
	return nil
}

func (m *mockPlugin) Start(ctx context.Context) error {
	return nil
}

func (m *mockPlugin) Stop(ctx context.Context) error {
	return nil
}

func (m *mockPlugin) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockPlugin) Capabilities() []string {
	return []string{}
}
