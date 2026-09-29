package plugin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AnixOps/anix-control/control-center/internal/core/plugin"
)

// MockPlugin is a mock implementation of Plugin for testing
type MockPlugin struct {
	info      plugin.PluginInfo
	initErr   error
	startErr  error
	stopErr   error
	healthErr error
}

func (m *MockPlugin) Info() plugin.PluginInfo {
	return m.info
}

func (m *MockPlugin) Init(ctx context.Context, config map[string]interface{}) error {
	return m.initErr
}

func (m *MockPlugin) Start(ctx context.Context) error {
	return m.startErr
}

func (m *MockPlugin) Stop(ctx context.Context) error {
	return m.stopErr
}

func (m *MockPlugin) HealthCheck(ctx context.Context) error {
	return m.healthErr
}

func (m *MockPlugin) Capabilities() []string {
	return []string{"test_action"}
}

// TestNewManager tests creating a new plugin manager
func TestNewManager(t *testing.T) {
	mgr := plugin.NewManager()
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

// TestRegister tests registering a plugin
func TestRegister(t *testing.T) {
	mgr := plugin.NewManager()

	p := &MockPlugin{
		info: plugin.PluginInfo{
			Name:    "test",
			Version: "1.0.0",
		},
	}

	err := mgr.Register("test", p)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Test duplicate registration
	err = mgr.Register("test", p)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

// TestGet tests getting a plugin
func TestGet(t *testing.T) {
	mgr := plugin.NewManager()

	p := &MockPlugin{
		info: plugin.PluginInfo{
			Name:    "test",
			Version: "1.0.0",
		},
	}
	mgr.Register("test", p)

	// Get existing plugin
	got, ok := mgr.Get("test")
	if !ok {
		t.Fatal("expected to find plugin")
	}
	if got.Info().Name != "test" {
		t.Errorf("expected plugin name 'test', got '%s'", got.Info().Name)
	}

	// Get non-existent plugin
	_, ok = mgr.Get("nonexistent")
	if ok {
		t.Fatal("expected not to find plugin")
	}
}

// TestUnregister tests unregistering a plugin
func TestUnregister(t *testing.T) {
	mgr := plugin.NewManager()

	p := &MockPlugin{
		info: plugin.PluginInfo{
			Name:    "test",
			Version: "1.0.0",
		},
	}
	mgr.Register("test", p)

	err := mgr.Unregister("test")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, ok := mgr.Get("test")
	if ok {
		t.Fatal("expected plugin to be unregistered")
	}
}

// TestStartPlugin tests starting a plugin
func TestStartPlugin(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mgr := plugin.NewManager()
		p := &MockPlugin{
			info: plugin.PluginInfo{Name: "test"},
		}
		mgr.Register("test", p)

		err := mgr.StartPlugin(ctx, "test")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		state := mgr.GetState("test")
		if state != plugin.StateRunning {
			t.Errorf("expected state %s, got %s", plugin.StateRunning, state)
		}
	})

	t.Run("init_error", func(t *testing.T) {
		mgr := plugin.NewManager()
		p := &MockPlugin{
			info:    plugin.PluginInfo{Name: "test"},
			initErr: errors.New("init failed"),
		}
		mgr.Register("test", p)

		err := mgr.StartPlugin(ctx, "test")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("start_error", func(t *testing.T) {
		mgr := plugin.NewManager()
		p := &MockPlugin{
			info:     plugin.PluginInfo{Name: "test"},
			startErr: errors.New("start failed"),
		}
		mgr.Register("test", p)

		err := mgr.StartPlugin(ctx, "test")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// TestStopPlugin tests stopping a plugin
func TestStopPlugin(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mgr := plugin.NewManager()
		p := &MockPlugin{
			info: plugin.PluginInfo{Name: "test"},
		}
		mgr.Register("test", p)
		mgr.StartPlugin(ctx, "test")

		err := mgr.StopPlugin(ctx, "test")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		state := mgr.GetState("test")
		if state != plugin.StateStopped {
			t.Errorf("expected state %s, got %s", plugin.StateStopped, state)
		}
	})
}

// TestList tests listing plugins
func TestList(t *testing.T) {
	mgr := plugin.NewManager()

	p1 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin1"}}
	p2 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin2"}}

	mgr.Register("plugin1", p1)
	mgr.Register("plugin2", p2)

	list := mgr.List()
	if len(list) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(list))
	}
}

// TestListByState tests listing plugins by state
func TestListByState(t *testing.T) {
	ctx := context.Background()
	mgr := plugin.NewManager()

	p1 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin1"}}
	p2 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin2"}}

	mgr.Register("plugin1", p1)
	mgr.Register("plugin2", p2)
	mgr.StartPlugin(ctx, "plugin1")

	running := mgr.ListByState(plugin.StateRunning)
	if len(running) != 1 {
		t.Errorf("expected 1 running plugin, got %d", len(running))
	}
}

// TestStartAll tests starting all plugins
func TestStartAll(t *testing.T) {
	ctx := context.Background()
	mgr := plugin.NewManager()

	p1 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin1"}}
	p2 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin2"}}

	mgr.Register("plugin1", p1)
	mgr.Register("plugin2", p2)

	err := mgr.StartAll(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	for _, name := range []string{"plugin1", "plugin2"} {
		state := mgr.GetState(name)
		if state != plugin.StateRunning {
			t.Errorf("expected plugin %s to be running, got %s", name, state)
		}
	}
}

// TestStopAll tests stopping all plugins
func TestStopAll(t *testing.T) {
	ctx := context.Background()
	mgr := plugin.NewManager()

	p1 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin1"}}
	p2 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin2"}}

	mgr.Register("plugin1", p1)
	mgr.Register("plugin2", p2)
	mgr.StartAll(ctx)

	err := mgr.StopAll(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestHealthCheck tests health checking all plugins
func TestHealthCheck(t *testing.T) {
	ctx := context.Background()
	mgr := plugin.NewManager()

	p1 := &MockPlugin{
		info:      plugin.PluginInfo{Name: "plugin1"},
		healthErr: nil,
	}
	p2 := &MockPlugin{
		info:      plugin.PluginInfo{Name: "plugin2"},
		healthErr: errors.New("unhealthy"),
	}

	mgr.Register("plugin1", p1)
	mgr.Register("plugin2", p2)

	results := mgr.HealthCheck(ctx)

	if results["plugin1"] != nil {
		t.Errorf("expected plugin1 to be healthy")
	}
	if results["plugin2"] == nil {
		t.Error("expected plugin2 to be unhealthy")
	}
}

// TestGetInfo tests getting plugin info
func TestGetInfo(t *testing.T) {
	mgr := plugin.NewManager()

	p1 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin1", Version: "1.0.0"}}
	p2 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin2", Version: "2.0.0"}}

	mgr.Register("plugin1", p1)
	mgr.Register("plugin2", p2)

	infos := mgr.GetInfo()

	if len(infos) != 2 {
		t.Errorf("expected 2 plugin infos, got %d", len(infos))
	}

	if infos["plugin1"].Version != "1.0.0" {
		t.Errorf("expected plugin1 version 1.0.0, got %s", infos["plugin1"].Version)
	}
}

// MockExecutablePlugin is a mock implementation of ExecutablePlugin
type MockExecutablePlugin struct {
	MockPlugin
	executeResult plugin.Result
	executeErr    error
}

func (m *MockExecutablePlugin) Execute(ctx context.Context, action string, params map[string]interface{}) (plugin.Result, error) {
	return m.executeResult, m.executeErr
}

// TestExecutablePlugin tests executing plugin actions
func TestExecutablePlugin(t *testing.T) {
	ctx := context.Background()
	mgr := plugin.NewManager()

	p := &MockExecutablePlugin{
		MockPlugin: MockPlugin{
			info: plugin.PluginInfo{Name: "test"},
		},
		executeResult: plugin.Result{
			Success: true,
			Data:    "test data",
		},
	}

	mgr.Register("test", p)
	mgr.StartPlugin(ctx, "test")

	// Get as ExecutablePlugin
	got, ok := mgr.Get("test")
	if !ok {
		t.Fatal("expected to find plugin")
	}

	execPlugin, ok := got.(plugin.ExecutablePlugin)
	if !ok {
		t.Fatal("expected plugin to be executable")
	}

	result, err := execPlugin.Execute(ctx, "test_action", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !result.Success {
		t.Error("expected success")
	}
}

// TestInitPlugin tests initializing a specific plugin
func TestInitPlugin(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mgr := plugin.NewManager()
		p := &MockPlugin{info: plugin.PluginInfo{Name: "test"}}
		mgr.Register("test", p)
		mgr.SetConfig("test", map[string]interface{}{"key": "value"})

		err := mgr.InitPlugin(ctx, "test")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		state := mgr.GetState("test")
		if state != plugin.StateInitialized {
			t.Errorf("expected state %s, got %s", plugin.StateInitialized, state)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		mgr := plugin.NewManager()

		err := mgr.InitPlugin(ctx, "nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent plugin")
		}
	})

	t.Run("init_error", func(t *testing.T) {
		mgr := plugin.NewManager()
		p := &MockPlugin{
			info:    plugin.PluginInfo{Name: "test"},
			initErr: errors.New("init failed"),
		}
		mgr.Register("test", p)

		err := mgr.InitPlugin(ctx, "test")
		if err == nil {
			t.Fatal("expected error for init failure")
		}

		state := mgr.GetState("test")
		if state != plugin.StateError {
			t.Errorf("expected state %s, got %s", plugin.StateError, state)
		}
	})
}

// TestUnregisterRunning tests unregistering a running plugin
func TestUnregisterRunning(t *testing.T) {
	ctx := context.Background()
	mgr := plugin.NewManager()

	p := &MockPlugin{
		info: plugin.PluginInfo{Name: "test"},
	}
	mgr.Register("test", p)
	mgr.StartPlugin(ctx, "test")

	// Unregister should stop the plugin first
	err := mgr.Unregister("test")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, ok := mgr.Get("test")
	if ok {
		t.Fatal("expected plugin to be unregistered")
	}
}

// TestStopPluginError tests stopping a plugin with error
func TestStopPluginError(t *testing.T) {
	ctx := context.Background()
	mgr := plugin.NewManager()

	p := &MockPlugin{
		info:    plugin.PluginInfo{Name: "test"},
		stopErr: errors.New("stop failed"),
	}
	mgr.Register("test", p)
	mgr.StartPlugin(ctx, "test")

	err := mgr.StopPlugin(ctx, "test")
	if err == nil {
		t.Fatal("expected error for stop failure")
	}

	state := mgr.GetState("test")
	if state != plugin.StateError {
		t.Errorf("expected state %s, got %s", plugin.StateError, state)
	}
}

// TestStopAllWithError tests stopping all plugins with one error
func TestStopAllWithError(t *testing.T) {
	ctx := context.Background()
	mgr := plugin.NewManager()

	p1 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin1"}}
	p2 := &MockPlugin{
		info:    plugin.PluginInfo{Name: "plugin2"},
		stopErr: errors.New("stop failed"),
	}

	mgr.Register("plugin1", p1)
	mgr.Register("plugin2", p2)
	mgr.StartAll(ctx)

	err := mgr.StopAll(ctx)
	if err == nil {
		t.Fatal("expected error for stop failure")
	}
}

// TestInitAllError tests initializing all plugins with error
func TestInitAllError(t *testing.T) {
	ctx := context.Background()
	mgr := plugin.NewManager()

	p1 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin1"}}
	p2 := &MockPlugin{
		info:    plugin.PluginInfo{Name: "plugin2"},
		initErr: errors.New("init failed"),
	}

	mgr.Register("plugin1", p1)
	mgr.Register("plugin2", p2)

	err := mgr.InitAll(ctx)
	if err == nil {
		t.Fatal("expected error for init failure")
	}
}

// TestStartAllError tests starting all plugins with error
func TestStartAllError(t *testing.T) {
	ctx := context.Background()
	mgr := plugin.NewManager()

	p1 := &MockPlugin{info: plugin.PluginInfo{Name: "plugin1"}}
	p2 := &MockPlugin{
		info:     plugin.PluginInfo{Name: "plugin2"},
		startErr: errors.New("start failed"),
	}

	mgr.Register("plugin1", p1)
	mgr.Register("plugin2", p2)

	err := mgr.StartAll(ctx)
	if err == nil {
		t.Fatal("expected error for start failure")
	}
}

// TestSetConfig tests setting config
func TestSetConfig(t *testing.T) {
	mgr := plugin.NewManager()
	config := map[string]interface{}{"key": "value"}

	mgr.SetConfig("test", config)

	retrieved := mgr.GetConfig("test")
	if retrieved == nil {
		t.Fatal("expected config to be set")
	}
	if retrieved["key"] != "value" {
		t.Errorf("expected key 'value', got '%v'", retrieved["key"])
	}
}

// TestStopPluginNotFound tests stopping a nonexistent plugin
func TestStopPluginNotFound(t *testing.T) {
	ctx := context.Background()
	mgr := plugin.NewManager()

	err := mgr.StopPlugin(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent plugin")
	}
}
