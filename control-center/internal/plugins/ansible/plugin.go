package ansible

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/control-center/internal/core/eventbus"
	"github.com/AnixOps/anix-control/control-center/internal/core/plugin"
)

// Config holds ansible plugin configuration
type Config struct {
	PlaybookDir   string            `yaml:"playbook_dir"`
	InventoryFile string            `yaml:"inventory_file"`
	VaultPassword string            `yaml:"vault_password"`
	EnvVars       map[string]string `yaml:"env_vars"`
	Timeout       int               `yaml:"timeout"` // seconds
}

// AnsiblePlugin implements the plugin interface for Ansible
type AnsiblePlugin struct {
	config   Config
	executor *Executor
	eventBus *eventbus.EventBus
	status   plugin.Status
	mu       sync.RWMutex
}

// New creates a new Ansible plugin
func New() *AnsiblePlugin {
	return &AnsiblePlugin{
		status: plugin.Status{
			State:   string(plugin.StateUninitialized),
			Health:  string(plugin.HealthHealthy),
			Metrics: make(map[string]interface{}),
		},
	}
}

// Info returns plugin information
func (p *AnsiblePlugin) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		Name:        "ansible",
		Version:     "1.0.0",
		Description: "Ansible automation engine for infrastructure management",
		Author:      "AnixOps",
	}
}

// Init initializes the plugin
func (p *AnsiblePlugin) Init(ctx context.Context, config map[string]interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Parse config
	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := json.Unmarshal(configBytes, &p.config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if p.config.Timeout == 0 {
		p.config.Timeout = 3600 // 1 hour default
	}

	// Verify ansible is installed
	if _, err := exec.LookPath("ansible"); err != nil {
		return fmt.Errorf("ansible not found in PATH")
	}
	if _, err := exec.LookPath("ansible-playbook"); err != nil {
		return fmt.Errorf("ansible-playbook not found in PATH")
	}

	// Create executor
	p.executor = NewExecutor(p.config)

	p.status.State = string(plugin.StateInitialized)
	return nil
}

// Start starts the plugin
func (p *AnsiblePlugin) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status.State = string(plugin.StateRunning)
	p.status.LastUpdated = time.Now().Unix()
	return nil
}

// Stop stops the plugin
func (p *AnsiblePlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status.State = string(plugin.StateStopped)
	p.status.LastUpdated = time.Now().Unix()
	return nil
}

// HealthCheck performs a health check
func (p *AnsiblePlugin) HealthCheck(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Check if ansible is still available
	if _, err := exec.LookPath("ansible"); err != nil {
		return fmt.Errorf("ansible not available")
	}

	return nil
}

// Capabilities returns plugin capabilities
func (p *AnsiblePlugin) Capabilities() []string {
	return []string{
		"run_playbook",
		"run_task",
		"list_hosts",
		"list_playbooks",
		"validate_inventory",
		"get_inventory",
	}
}

// Execute executes an action
func (p *AnsiblePlugin) Execute(ctx context.Context, action string, params map[string]interface{}) (plugin.Result, error) {
	switch action {
	case "run_playbook":
		return p.runPlaybook(ctx, params)
	case "run_task":
		return p.runTask(ctx, params)
	case "list_hosts":
		return p.listHosts(ctx, params)
	case "list_playbooks":
		return p.listPlaybooks(ctx)
	case "validate_inventory":
		return p.validateInventory(ctx, params)
	case "get_inventory":
		return p.getInventory(ctx, params)
	default:
		return plugin.Result{}, fmt.Errorf("unknown action: %s", action)
	}
}

// GetStatus returns current status
func (p *AnsiblePlugin) GetStatus(ctx context.Context) (plugin.Status, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status, nil
}

// runPlaybook runs an ansible playbook
func (p *AnsiblePlugin) runPlaybook(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	playbook, ok := params["playbook"].(string)
	if !ok {
		return plugin.Result{}, fmt.Errorf("playbook parameter required")
	}

	// Build args
	args := []string{playbook}

	// Add inventory
	if inv, ok := params["inventory"].(string); ok {
		args = append(args, "-i", inv)
	} else if p.config.InventoryFile != "" {
		args = append(args, "-i", p.config.InventoryFile)
	}

	// Add extra vars
	if extraVars, ok := params["extra_vars"].(map[string]interface{}); ok {
		varsJSON, _ := json.Marshal(extraVars)
		args = append(args, "--extra-vars", string(varsJSON))
	}

	// Add tags
	if tags, ok := params["tags"].(string); ok {
		args = append(args, "--tags", tags)
	}

	// Add limit
	if limit, ok := params["limit"].(string); ok {
		args = append(args, "--limit", limit)
	}

	// Add verbose flag
	if verbose, ok := params["verbose"].(bool); ok && verbose {
		args = append(args, "-v")
	}

	// Add check mode
	if check, ok := params["check"].(bool); ok && check {
		args = append(args, "--check")
	}

	// Add diff
	if diff, ok := params["diff"].(bool); ok && diff {
		args = append(args, "--diff")
	}

	// Execute
	output, err := p.executor.Execute(ctx, "ansible-playbook", args...)

	result := plugin.Result{
		Success: err == nil,
		Data:    string(output),
		Metrics: map[string]interface{}{
			"playbook": playbook,
			"duration": time.Now().Unix(),
		},
	}

	if err != nil {
		result.Error = err.Error()
	}

	return result, nil
}

// runTask runs a single ansible task
func (p *AnsiblePlugin) runTask(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	module, ok := params["module"].(string)
	if !ok {
		return plugin.Result{}, fmt.Errorf("module parameter required")
	}

	pattern, ok := params["pattern"].(string)
	if !ok {
		pattern = "all"
	}

	args := []string{pattern, "-m", module}

	// Add module args
	if moduleArgs, ok := params["args"].(string); ok {
		args = append(args, "-a", moduleArgs)
	}

	// Add inventory
	if inv, ok := params["inventory"].(string); ok {
		args = append(args, "-i", inv)
	} else if p.config.InventoryFile != "" {
		args = append(args, "-i", p.config.InventoryFile)
	}

	output, err := p.executor.Execute(ctx, "ansible", args...)

	result := plugin.Result{
		Success: err == nil,
		Data:    string(output),
	}

	if err != nil {
		result.Error = err.Error()
	}

	return result, nil
}

// listHosts lists all hosts in the inventory
func (p *AnsiblePlugin) listHosts(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	args := []string{"--list-hosts", "all"}

	if inv, ok := params["inventory"].(string); ok {
		args = append(args, "-i", inv)
	} else if p.config.InventoryFile != "" {
		args = append(args, "-i", p.config.InventoryFile)
	}

	output, err := p.executor.Execute(ctx, "ansible", args...)
	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	// Parse hosts
	hosts := parseHosts(output)

	return plugin.Result{
		Success: true,
		Data:    hosts,
	}, nil
}

// listPlaybooks lists available playbooks
func (p *AnsiblePlugin) listPlaybooks(ctx context.Context) (plugin.Result, error) {
	if p.config.PlaybookDir == "" {
		return plugin.Result{Success: false, Error: "playbook_dir not configured"}, nil
	}

	playbooks := []string{}
	err := filepath.Walk(p.config.PlaybookDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") {
			rel, _ := filepath.Rel(p.config.PlaybookDir, path)
			playbooks = append(playbooks, rel)
		}
		return nil
	})

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	return plugin.Result{
		Success: true,
		Data:    playbooks,
	}, nil
}

// validateInventory validates the inventory file
func (p *AnsiblePlugin) validateInventory(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	inv, ok := params["inventory"].(string)
	if !ok {
		inv = p.config.InventoryFile
	}
	if inv == "" {
		return plugin.Result{Success: false, Error: "no inventory specified"}, nil
	}

	args := []string{"-i", inv, "--list-hosts", "all"}
	output, err := p.executor.Execute(ctx, "ansible", args...)

	return plugin.Result{
		Success: err == nil,
		Data:    string(output),
		Error:   errToString(err),
	}, nil
}

// getInventory returns the inventory as JSON
func (p *AnsiblePlugin) getInventory(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	inv, ok := params["inventory"].(string)
	if !ok {
		inv = p.config.InventoryFile
	}
	if inv == "" {
		return plugin.Result{Success: false, Error: "no inventory specified"}, nil
	}

	args := []string{"-i", inv, "--list"}
	output, err := p.executor.Execute(ctx, "ansible-inventory", args...)
	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	var inventory map[string]interface{}
	if err := json.Unmarshal(output, &inventory); err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	return plugin.Result{
		Success: true,
		Data:    inventory,
	}, nil
}

// parseHosts parses ansible host list output
func parseHosts(output []byte) []string {
	hosts := []string{}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "  ") {
			host := strings.TrimSpace(line)
			if host != "" {
				hosts = append(hosts, host)
			}
		}
	}
	return hosts
}

func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
