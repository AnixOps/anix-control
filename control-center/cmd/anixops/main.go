package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/AnixOps/anix-control/control-center/internal/core/config"
	"github.com/AnixOps/anix-control/control-center/internal/core/plugin"
	"github.com/AnixOps/anix-control/control-center/internal/plugins/agent"
	"github.com/AnixOps/anix-control/control-center/internal/plugins/ansible"
	"github.com/AnixOps/anix-control/control-center/internal/plugins/v2board"
	"github.com/AnixOps/anix-control/control-center/internal/plugins/v2bx"
	"github.com/AnixOps/anix-control/control-center/internal/security/auth"
)

// Build information (set via ldflags)
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

var (
	cfgFile string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "anixops",
	Short: "AnixOps Control Center - Unified infrastructure management",
	Long: `AnixOps Control Center is a unified control plane for managing
all AnixOps products including v2board, V2bX, and AnixOps-agent.

It provides a TUI interface, REST API, and CLI for comprehensive
infrastructure management.`,
	Version: version,
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the API server",
	Long:  `Start the REST API server for AnixOps Control Center.`,
	Run:   runServer,
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Start the TUI interface",
	Long:  `Start the terminal user interface for AnixOps Control Center.`,
	Run:   runTUI,
}

var ansibleCmd = &cobra.Command{
	Use:   "ansible",
	Short: "Ansible operations",
	Long:  `Execute Ansible playbooks and manage infrastructure.`,
}

var playbookCmd = &cobra.Command{
	Use:   "run <playbook>",
	Short: "Run an Ansible playbook",
	Args:  cobra.ExactArgs(1),
	Run:   runPlaybook,
}

var nodesCmd = &cobra.Command{
	Use:   "nodes",
	Short: "Node management",
	Long:  `Manage proxy nodes through v2board.`,
}

var listNodesCmd = &cobra.Command{
	Use:   "list",
	Short: "List all nodes",
	Run:   listNodes,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("AnixOps Control Center\n")
		fmt.Printf("  Version: %s\n", version)
		fmt.Printf("  Commit:  %s\n", commit)
		fmt.Printf("  Built:   %s\n", date)
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is ./config.yaml)")

	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(versionCmd)

	// Ansible commands
	ansibleCmd.AddCommand(playbookCmd)
	playbookCmd.Flags().StringP("inventory", "i", "", "Inventory file")
	playbookCmd.Flags().StringP("tags", "t", "", "Run specific tags")
	playbookCmd.Flags().StringP("limit", "l", "", "Limit to specific hosts")
	playbookCmd.Flags().Bool("check", false, "Dry run mode")
	playbookCmd.Flags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.AddCommand(ansibleCmd)

	// Node commands
	nodesCmd.AddCommand(listNodesCmd)
	rootCmd.AddCommand(nodesCmd)
}

func initConfig() {
	// Load config if specified
	if cfgFile != "" {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
		_ = cfg // Use config
	}
}

func runServer(cmd *cobra.Command, args []string) {
	fmt.Println("Starting AnixOps Control Center server...")
	fmt.Printf("Version: %s (%s)\n", version, commit)

	// Load config
	cfg := loadConfigOrDie()

	// Initialize plugin manager
	pluginMgr := plugin.NewManager()

	// Register plugins
	registerPlugins(pluginMgr, cfg)

	// Initialize security
	_ = auth.NewJWTManager(
		cfg.Auth.JWT.Secret,
		cfg.Auth.JWT.Expire,
		cfg.Auth.JWT.RefreshExpire,
		cfg.Auth.JWT.Issuer,
	)
	_ = auth.NewRBACManager()

	// Start plugins
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := pluginMgr.StartAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start plugins: %v\n", err)
		os.Exit(1)
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("Server running on %s:%d\n", cfg.Server.Host, cfg.Server.Port)

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\nShutting down...")

	// Stop plugins
	pluginMgr.StopAll(ctx)
}

func runTUI(cmd *cobra.Command, args []string) {
	// Run TUI
	// tui.Run()
	fmt.Println("TUI mode - use 'go run cmd/anixops-tui/main.go' for TUI")
}

func runPlaybook(cmd *cobra.Command, args []string) {
	playbook := args[0]

	cfg := loadConfigOrDie()
	pluginMgr := plugin.NewManager()
	registerPlugins(pluginMgr, cfg)

	ctx := context.Background()
	if err := pluginMgr.StartAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start plugins: %v\n", err)
		os.Exit(1)
	}
	defer pluginMgr.StopAll(ctx)

	// Get ansible plugin
	p, ok := pluginMgr.Get("ansible")
	if !ok {
		fmt.Fprintln(os.Stderr, "Ansible plugin not found")
		os.Exit(1)
	}

	execPlugin, ok := p.(plugin.ExecutablePlugin)
	if !ok {
		fmt.Fprintln(os.Stderr, "Ansible plugin not executable")
		os.Exit(1)
	}

	// Build params
	params := map[string]interface{}{
		"playbook": playbook,
	}

	if inv, _ := cmd.Flags().GetString("inventory"); inv != "" {
		params["inventory"] = inv
	}
	if tags, _ := cmd.Flags().GetString("tags"); tags != "" {
		params["tags"] = tags
	}
	if limit, _ := cmd.Flags().GetString("limit"); limit != "" {
		params["limit"] = limit
	}
	if check, _ := cmd.Flags().GetBool("check"); check {
		params["check"] = true
	}
	if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
		params["verbose"] = true
	}

	// Execute
	result, err := execPlugin.Execute(ctx, "run_playbook", params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if result.Success {
		fmt.Println("Playbook completed successfully")
	} else {
		fmt.Printf("Playbook failed: %s\n", result.Error)
		os.Exit(1)
	}

	fmt.Println(result.Data)
}

func listNodes(cmd *cobra.Command, args []string) {
	cfg := loadConfigOrDie()
	pluginMgr := plugin.NewManager()
	registerPlugins(pluginMgr, cfg)

	ctx := context.Background()
	if err := pluginMgr.StartAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start plugins: %v\n", err)
		os.Exit(1)
	}
	defer pluginMgr.StopAll(ctx)

	p, ok := pluginMgr.Get("v2board")
	if !ok {
		fmt.Fprintln(os.Stderr, "v2board plugin not found")
		os.Exit(1)
	}

	execPlugin, ok := p.(plugin.ExecutablePlugin)
	if !ok {
		fmt.Fprintln(os.Stderr, "v2board plugin not executable")
		os.Exit(1)
	}

	result, err := execPlugin.Execute(ctx, "get_nodes", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Nodes: %v\n", result.Data)
}

func loadConfigOrDie() *config.Config {
	if cfgFile == "" {
		return config.DefaultConfig()
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	return cfg
}

func registerPlugins(mgr *plugin.Manager, cfg *config.Config) {
	// Register Ansible plugin
	ansiblePlugin := ansible.New()
	mgr.Register("ansible", ansiblePlugin)
	mgr.SetConfig("ansible", map[string]interface{}{
		"playbook_dir":   cfg.Plugins.Ansible["playbook_dir"],
		"inventory_file": cfg.Plugins.Ansible["inventory_file"],
	})

	// Register v2board plugin
	v2boardPlugin := v2board.New()
	mgr.Register("v2board", v2boardPlugin)
	mgr.SetConfig("v2board", map[string]interface{}{
		"host":    cfg.Plugins.V2board["host"],
		"api_key": cfg.Plugins.V2board["api_key"],
	})

	// Register V2bX plugin
	v2bxPlugin := v2bx.New()
	mgr.Register("v2bx", v2bxPlugin)
	mgr.SetConfig("v2bx", map[string]interface{}{
		"hosts": cfg.Plugins.V2bX["hosts"],
	})

	// Register agent plugin
	agentPlugin := agent.New()
	mgr.Register("agent", agentPlugin)
	mgr.SetConfig("agent", map[string]interface{}{
		"host": cfg.Plugins.Agent["host"],
		"port": cfg.Plugins.Agent["port"],
	})
}
