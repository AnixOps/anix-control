package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"
	_ "time/tzdata" // database.timezone and TZ resolve without system zoneinfo

	_ "github.com/AnixOps/anix-control/v4/docs" // swagger docs
	"github.com/AnixOps/anix-control/v4/internal/branding"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	grpcserver "github.com/AnixOps/anix-control/v4/internal/grpc"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/health"
	"github.com/AnixOps/anix-control/v4/internal/identitybridge"
	"github.com/AnixOps/anix-control/v4/internal/lease"
	"github.com/AnixOps/anix-control/v4/internal/logging"
	_ "github.com/AnixOps/anix-control/v4/internal/payment/gateways" // register payment gateway plugins
	"github.com/AnixOps/anix-control/v4/internal/plugincontrol"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/router"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// @title AnixOps Control API
// @version 4.0.0
// @description AnixOps Control 统一控制面 API 文档
// @description 支持用户管理、节点管理、订阅系统、支付网关、流量转发等功能
// @termsOfService https://github.com/AnixOps/anix-control

// @contact.name API Support
// @contact.url https://github.com/AnixOps/anix-control/issues
// @contact.email support@anixops.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /api/v2
// @schemes http https

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

// @tag.name 认证
// @tag.description 用户登录、注册等认证相关接口

// @tag.name 用户端
// @tag.description 用户个人信息、订阅、工单、订单等接口

// @tag.name 管理端
// @tag.description 管理员用户管理、节点管理、配置等接口

// @tag.name 节点通信
// @tag.description 节点注册、心跳、配置同步等接口

// shutdownTimeout defines how long to wait for in-flight requests to drain.
const shutdownTimeout = 30 * time.Second

var (
	configPath string
	printEnv   bool
	version    = branding.DefaultVersion
	buildTime  = "unknown"
	buildCode  = ""
	commit     = "unknown"
)

func init() {
	flag.StringVar(&configPath, "config", defaultConfigPath, "配置文件路径 (or "+config.ConfigPathEnv+")")
	flag.BoolVar(&printEnv, "print-env", false, "print every ANIX_CONTROL_* configuration variable and exit")
}

const defaultConfigPath = "config/config.yaml"

// pluginHostHealthPollInterval is how often package host health details are
// read for metrics.
const pluginHostHealthPollInterval = 30 * time.Second

// takeMigrateCommand removes a leading "migrate" argument and reports whether
// it was present. `anix-control migrate [flags]` prepares the database schema
// and seed data, then exits: the one-shot step for a Compose service or a
// Kubernetes Job before the server starts.
func takeMigrateCommand() bool {
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		os.Args = append(os.Args[:1], os.Args[2:]...)
		return true
	}
	return false
}

// selectConfigPath decides which config file to load. An explicit -config or
// ANIX_CONTROL_CONFIG must exist. Without either, the default
// config/config.yaml is used when present; otherwise Control starts from its
// built-in defaults plus ANIX_CONTROL_* variables and the returned path is "".
func selectConfigPath(flagValue string, flagSet bool, envValue string) (string, error) {
	switch {
	case flagSet:
		return resolveConfigPath(flagValue)
	case envValue != "":
		return resolveConfigPath(envValue)
	}
	resolved, err := resolveConfigPath(defaultConfigPath)
	if err != nil {
		return "", nil
	}
	return resolved, nil
}

func configFlagSet() bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			set = true
		}
	})
	return set
}

// writeEnvTable prints the configuration variables with their built-in
// defaults; secret values are never printed.
func writeEnvTable(w io.Writer) error {
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	var writeErr error
	printf := func(format string, args ...any) {
		if writeErr == nil {
			_, writeErr = fmt.Fprintf(table, format, args...)
		}
	}
	printf("VARIABLE\tTYPE\tDEFAULT\tKEY\n")
	for _, variable := range config.EnvVars(config.Defaults()) {
		value := variable.Default
		if variable.Secret {
			value = "(secret)"
		}
		printf("%s\t%s\t%s\t%s\n", variable.Name, variable.Type, value, variable.Path)
	}
	printf("\nEvery variable also accepts NAME%s=<path> to read the value from a file.\n", config.EnvFileSuffix)
	if writeErr != nil {
		return writeErr
	}
	return table.Flush()
}

func formatDisplayVersion(baseVersion, code string) string {
	if code == "" {
		return baseVersion
	}
	return baseVersion + " #" + code
}

func syncBuildInfo() {
	handler.BuildVersion = formatDisplayVersion(version, buildCode)
	handler.BuildTime = buildTime
	handler.BuildCode = buildCode
	handler.BuildCommit = commit
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func resolveConfigPath(rawPath string) (string, error) {
	if rawPath == "" {
		rawPath = "config/config.yaml"
	}

	if filepath.IsAbs(rawPath) {
		if fileExists(rawPath) {
			return rawPath, nil
		}
		return rawPath, fmt.Errorf("config file not found: %s", rawPath)
	}

	checked := make([]string, 0, 3)

	if cwdPath, err := filepath.Abs(rawPath); err == nil {
		checked = append(checked, cwdPath)
		if fileExists(cwdPath) {
			return cwdPath, nil
		}
	}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)

		exeCandidate := filepath.Clean(filepath.Join(exeDir, rawPath))
		checked = append(checked, exeCandidate)
		if fileExists(exeCandidate) {
			return exeCandidate, nil
		}

		// Support binaries placed under ./build while config stays at project ./config.
		exeParentCandidate := filepath.Clean(filepath.Join(exeDir, "..", rawPath))
		checked = append(checked, exeParentCandidate)
		if fileExists(exeParentCandidate) {
			return exeParentCandidate, nil
		}
	}

	if len(checked) > 0 {
		return checked[0], fmt.Errorf("config file not found (checked: %s)", strings.Join(checked, ", "))
	}

	return rawPath, fmt.Errorf("config file not found: %s", rawPath)
}

func resolveRuntimePath(rawPath, resolvedConfigPath string) string {
	if rawPath == "" {
		return rawPath
	}
	if filepath.IsAbs(rawPath) {
		return rawPath
	}

	candidates := make([]string, 0, 4)
	configDir := filepath.Dir(resolvedConfigPath)
	if configDir != "" {
		// If config file lives in ./config/, prefer resolving relative paths from project root.
		if strings.EqualFold(filepath.Base(configDir), "config") {
			candidates = append(candidates, filepath.Join(filepath.Dir(configDir), rawPath))
		}
		candidates = append(candidates, filepath.Join(configDir, rawPath))
	}
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), rawPath))
	}
	if cwdPath, err := filepath.Abs(rawPath); err == nil {
		candidates = append(candidates, cwdPath)
	}

	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return rawPath
}

func resolveSQLitePath(rawDBPath, resolvedConfigPath string) string {
	if rawDBPath == "" {
		rawDBPath = "config/data/v2board.db"
	}
	return resolveRuntimePath(rawDBPath, resolvedConfigPath)
}

func resolveForwardRuntimePaths(cfg *config.Config, resolvedConfigPath string) {
	if cfg == nil {
		return
	}

	ansibleCfg := &cfg.ForwardRuntime.NftablesAnsible
	ansibleCfg.Inventory = resolveRuntimePath(ansibleCfg.Inventory, resolvedConfigPath)
	ansibleCfg.ApplyPlaybook = resolveRuntimePath(ansibleCfg.ApplyPlaybook, resolvedConfigPath)
	ansibleCfg.RemovePlaybook = resolveRuntimePath(ansibleCfg.RemovePlaybook, resolvedConfigPath)
	ansibleCfg.WorkingDir = resolveRuntimePath(ansibleCfg.WorkingDir, resolvedConfigPath)

	if len(ansibleCfg.Environment) == 0 {
		return
	}
	if value := strings.TrimSpace(ansibleCfg.Environment["ANSIBLE_CONFIG"]); value != "" {
		ansibleCfg.Environment["ANSIBLE_CONFIG"] = resolveRuntimePath(value, resolvedConfigPath)
	}
}

func applyTrustedProxies(r *gin.Engine, proxies []string) error {
	normalized := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		value := strings.TrimSpace(proxy)
		if value != "" {
			normalized = append(normalized, value)
		}
	}

	if len(normalized) == 0 {
		return r.SetTrustedProxies(nil)
	}

	return r.SetTrustedProxies(normalized)
}

func shouldStartForwardAgentBridgeWorker(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return cfg.ForwardRuntime.CleanAgent.LegacyBridgeEnabled
}

func setHTMLNoCacheHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Surrogate-Control", "no-store")
}

func serveAssetFiles(frontendPath string) gin.HandlerFunc {
	fileServer := http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(frontendPath, "assets"))))
	return func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

func main() {
	if code := run(); code != 0 {
		os.Exit(code)
	}
}

// run starts Control and blocks until shutdown. It returns the process exit
// code so that deferred cleanup (cache, database) runs before os.Exit.
func run() int {
	migrateOnly := takeMigrateCommand()
	moduleArguments := takeModuleCommand()
	flag.Parse()
	if printEnv {
		if err := writeEnvTable(os.Stdout); err != nil {
			log.Printf("print-env: %v", err)
			return 1
		}
		return 0
	}

	resolvedConfigPath, resolveErr := selectConfigPath(configPath, configFlagSet(), config.PathFromEnv())
	if resolveErr != nil {
		log.Fatalf("Failed to locate config: %v", resolveErr)
	}
	if resolvedConfigPath == "" {
		log.Printf("No config file; using built-in defaults and %s* environment variables", config.EnvPrefix)
	} else {
		log.Printf("Loading config file: %s", resolvedConfigPath)
	}

	// 打印版本信息
	syncBuildInfo()
	// Module commands print JSON on stdout, so the banner goes to stderr.
	banner := os.Stdout
	if moduleArguments != nil {
		banner = os.Stderr
	}
	_, _ = fmt.Fprintf(banner, "%s v%s (build: %s)\n", branding.ControlName, version, buildTime)

	// 加载配置
	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if !migrateOnly && moduleArguments == nil {
		if err := cfg.ValidateForServer(); err != nil {
			log.Fatalf("Invalid config: %v", err)
		}
	}
	if moduleArguments != nil {
		// Module commands print their result on stdout; logs go to stderr.
		logging.SetupTo(cfg.Log, os.Stderr)
	} else {
		logging.Setup(cfg.Log)
	}

	// 打印环境信息
	driver := strings.ToLower(cfg.Database.Driver)
	if driver == "" || driver == "sqlite" || driver == "sqlite3" {
		cfg.Database.Database = resolveSQLitePath(cfg.Database.Database, resolvedConfigPath)
		log.Printf("SQLite DB path: %s", cfg.Database.Database)
	} else {
		log.Printf("PostgreSQL DSN target: host=%s port=%d db=%s user=%s",
			cfg.Database.Host, cfg.Database.Port, cfg.Database.Database, cfg.Database.Username)
	}

	frontendPath := cfg.Frontend.Path
	if frontendPath == "" {
		frontendPath = "web/public"
	}
	cfg.Frontend.Path = resolveRuntimePath(frontendPath, resolvedConfigPath)
	log.Printf("Frontend static path: %s", cfg.Frontend.Path)
	resolveForwardRuntimePaths(cfg, resolvedConfigPath)

	env := cfg.Env
	if env == "" {
		env = "development"
	}
	log.Printf("Environment: %s", env)
	if cfg.TLS.Enable {
		log.Printf("TLS: enabled (domain: %s)", cfg.TLS.Domain)
	} else {
		log.Printf("TLS: disabled")
	}

	// 初始化数据库
	if err := database.Init(&cfg.Database); err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	if sqlDB, err := database.Get().DB(); err == nil {
		health.Default.SetDatabasePinger(sqlDB.PingContext)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Database close error: %v", err)
		}
	}()

	if err := bootstrapDatabase(context.Background(), cfg, env); err != nil {
		log.Fatalf("Failed to prepare database: %v", err)
	}
	if migrateOnly {
		log.Println("Database schema and seed data are up to date; migrate finished.")
		return 0
	}
	if moduleArguments != nil {
		if err := runModuleCommand(context.Background(), cfg, database.Get(), moduleArguments, os.Stdout); err != nil {
			log.Printf("module: %v", err)
			return 2
		}
		return 0
	}
	cache.InitMemory()
	defer cache.CloseMemory()
	log.Println("Cache initialized: memory")

	// 插件轮询间隔和依赖关系属于纯配置校验，在启动任何组件之前完成，
	// 配置错误时直接退出，不会留下已启动的监听器或后台任务。
	pollIntervals, err := parsePluginPollIntervals(cfg)
	if err != nil {
		log.Fatalf("Invalid plugin configuration: %v", err)
	}

	// 根 context：SIGINT/SIGTERM 会取消它，触发下面的有序关闭流程。
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	// Bind the required node-facing listener before starting workers or HTTP
	// servers. A port conflict must fail startup without exposing a partially
	// initialized Control instance.
	grpcSrv, grpcAddr, err := startGRPCServer(cfg)
	if err != nil {
		log.Fatalf("Failed to start gRPC server: %v", err)
	}
	if grpcSrv != nil {
		log.Printf("gRPC server listening on %s", grpcAddr)
	}

	// 从这里开始已有组件在运行：之后的错误不再 log.Fatal，而是走同一套有序关闭，
	// 清理完成后以非零状态码退出。
	drainDelay, _ := cfg.Server.DrainDelay() // validated by config.Load
	rt := &serverRuntime{
		grpcSrv:    grpcSrv,
		workers:    newBackgroundWorkers(rootCtx),
		fatal:      newFatalErrors(),
		drainDelay: drainDelay,
	}
	exitCode := 0
	if err := rt.start(cfg, pollIntervals); err != nil {
		log.Printf("Startup failed, shutting down: %v", err)
		exitCode = 1
	} else {
		health.Default.MarkStarted()
		select {
		case <-rootCtx.Done():
			log.Println("Shutdown signal received, draining in-flight requests...")
		case err := <-rt.fatal.ch:
			log.Printf("Fatal runtime error, shutting down: %v", err)
			exitCode = 1
		}
	}
	rt.shutdown(stopSignals)
	return exitCode
}

// pluginPollIntervals holds the validated poll intervals of the optional
// plugin control loops.
type pluginPollIntervals struct {
	control  time.Duration
	dispatch time.Duration
	topology time.Duration
}

func parsePluginPollInterval(key, raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 5 * time.Second, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid %s %q", key, raw)
	}
	return parsed, nil
}

func parsePluginPollIntervals(cfg *config.Config) (pluginPollIntervals, error) {
	var intervals pluginPollIntervals
	var err error
	if cfg.Plugins.ControlExecutionEnabled {
		if intervals.control, err = parsePluginPollInterval("plugins.control_poll_interval", cfg.Plugins.ControlPollInterval); err != nil {
			return intervals, err
		}
	}
	if cfg.Plugins.DispatchEnabled {
		if !cfg.GRPC.Enable {
			return intervals, errors.New("plugin operation dispatch requires grpc.enabled=true")
		}
		if intervals.dispatch, err = parsePluginPollInterval("plugins.dispatch_poll_interval", cfg.Plugins.DispatchPollInterval); err != nil {
			return intervals, err
		}
	}
	if cfg.Plugins.TopologyExecutionEnabled {
		if !cfg.Plugins.DispatchEnabled {
			return intervals, errors.New("topology execution requires plugins.dispatch_enabled=true")
		}
		if intervals.topology, err = parsePluginPollInterval("plugins.topology_poll_interval", cfg.Plugins.TopologyPollInterval); err != nil {
			return intervals, err
		}
	}
	return intervals, nil
}

// serverRuntime owns everything started after the gRPC listener so a single
// shutdown path can stop it, whether shutdown is triggered by a signal, a
// startup error, or a fatal runtime error.
type serverRuntime struct {
	grpcSrv *grpcserver.Server
	workers *backgroundWorkers
	fatal   *fatalErrors

	controlPluginHosts           *pluginhost.Supervisor
	controlPluginArtifactCleanup func()

	apiSrv      *http.Server
	frontendSrv *http.Server
	servers     sync.WaitGroup
	drainDelay  time.Duration
}

// start launches plugin hosts, background workers and HTTP servers. On error
// it returns immediately; the caller still runs shutdown for whatever started.
func (rt *serverRuntime) start(cfg *config.Config, intervals pluginPollIntervals) error {
	if err := rt.startTokenRevocations(cfg); err != nil {
		return err
	}
	pluginhost.SetDefaultManager(nil)
	if cfg.Plugins.ControlExecutionEnabled {
		hosts, err := newControlPluginHostManager(cfg)
		if err != nil {
			return fmt.Errorf("initialize Control plugin hosts: %w", err)
		}
		rt.controlPluginHosts = hosts
		pluginhost.SetDefaultManager(hosts)
		// Remote packages must be known before reconciliation starts hosts.
		if err := rt.startModuleRuntime(cfg, hosts); err != nil {
			return err
		}
		// Read route modes and shadow counters from package hosts for metrics.
		rt.workers.Go("plugin host health poller", func(ctx context.Context) {
			hosts.PollHealth(ctx, pluginHostHealthPollInterval)
		})
		artifacts, cleanupArtifacts, err := newControlPluginArtifactResolver(cfg)
		if err != nil {
			return fmt.Errorf("initialize Control plugin artifact resolver: %w", err)
		}
		rt.controlPluginArtifactCleanup = cleanupArtifacts
		worker, err := plugincontrol.NewOperationWorker(
			database.Get(),
			plugincontrol.NewHostLifecycleDispatcher(hosts, artifacts).
				WithMigrator(service.PackageHostMigrator{DB: database.Get(), Hosts: hosts}),
		)
		if err != nil {
			return fmt.Errorf("initialize Control plugin lifecycle worker: %w", err)
		}
		queued, err := worker.QueueReconciliation(uuid.NewString())
		if err != nil {
			log.Printf("Control plugin restart reconciliation queued with errors: %v", err)
		}
		rt.workers.Go("control plugin lifecycle worker", func(ctx context.Context) {
			worker.Run(ctx, intervals.control, func(err error) {
				log.Printf("Control plugin lifecycle worker error: %v", err)
			})
		})
		log.Printf("Control plugin host supervision enabled (poll interval %s, reconciliation operations %d)", intervals.control, queued)
	}

	if err := rt.startIdentityKeys(); err != nil {
		return err
	}

	if !cfg.Plugins.ControlExecutionEnabled {
		// Without package execution the listener still serves ModulePKI.
		if err := rt.startModuleRuntime(cfg, nil); err != nil {
			return err
		}
	}

	// Periodic resets, stats collection, latency probing and the forward job
	// executors must run in exactly one Control process per database. They
	// run only while this process holds the singleton-worker lease; another
	// process (a rolling update, or a second replica) takes over when this
	// one stops renewing it.
	bridgeEnabled := shouldStartForwardAgentBridgeWorker(cfg)
	if !bridgeEnabled {
		log.Println("Forward clean_agent legacy bridge worker disabled")
	}
	elector := &lease.Elector{DB: database.Get(), Name: singletonWorkerLease, Holder: lease.InstanceID(), TTL: singletonWorkerLeaseTTL}
	rt.workers.Go("singleton worker lease", func(ctx context.Context) {
		elector.Run(ctx, func(ctx context.Context) {
			runSingletonWorkers(ctx, bridgeEnabled)
		})
	})

	// 设置Gin模式
	gin.SetMode(cfg.Server.Mode)

	// Create HTTP servers with proper timeouts before starting goroutines.
	apiSrv, err := newAPIServer(cfg)
	if err != nil {
		return err
	}
	rt.apiSrv = apiSrv
	if cfg.Frontend.Enable {
		frontendSrv, err := newFrontendServer(cfg)
		if err != nil {
			return err
		}
		rt.frontendSrv = frontendSrv
	}

	// Start the servers in goroutines. A listener failure triggers shutdown.
	if rt.frontendSrv != nil {
		rt.servers.Add(1)
		go func() {
			defer rt.servers.Done()
			if err := runFrontendServer(rt.frontendSrv, cfg); err != nil && !errors.Is(err, http.ErrServerClosed) {
				rt.fatal.Report(fmt.Errorf("frontend server: %w", err))
			}
		}()
	}

	rt.servers.Add(1)
	go func() {
		defer rt.servers.Done()
		if err := runAPIServer(rt.apiSrv); err != nil && !errors.Is(err, http.ErrServerClosed) {
			rt.fatal.Report(fmt.Errorf("API server: %w", err))
		}
	}()

	if cfg.Plugins.DispatchEnabled {
		bridge, err := grpcserver.NewKernelOperationBridge(database.Get(), rt.grpcSrv.GetAgentControlManager())
		if err != nil {
			return fmt.Errorf("initialize plugin operation dispatcher: %w", err)
		}
		rt.workers.Go("plugin operation dispatcher", func(ctx context.Context) {
			bridge.Run(ctx, intervals.dispatch, func(err error) {
				log.Printf("Plugin operation dispatcher error: %v", err)
			})
		})
		log.Printf("Plugin operation dispatcher enabled (poll interval %s)", intervals.dispatch)
	}
	if cfg.Plugins.TopologyExecutionEnabled {
		executor, err := service.NewTopologyDeploymentExecutor(database.Get())
		if err != nil {
			return fmt.Errorf("initialize topology deployment executor: %w", err)
		}
		rt.workers.Go("topology deployment executor", func(ctx context.Context) {
			executor.Run(ctx, intervals.topology, func(err error) {
				log.Printf("Topology deployment executor error: %v", err)
			})
		})
		log.Printf("Topology deployment execution enabled (poll interval %s)", intervals.topology)
	}
	return nil
}

// shutdown stops everything start launched, in dependency order: stop taking
// traffic, stop background work, stop the node-facing gRPC server, then stop
// plugin hosts. Each blocking step has its own bound. The database and cache
// are closed afterwards by run's deferred calls.
func (rt *serverRuntime) shutdown(stopSignals context.CancelFunc) {
	// Mark servers as draining so /readyz and /health return 503 to load
	// balancers; /livez keeps succeeding.
	health.Default.MarkDraining()

	// A second signal forces an immediate exit. Register it before releasing
	// the root signal context so no signal falls through to the default
	// handler; stopSignals also cancels the root context.
	forceChan := make(chan os.Signal, 1)
	signal.Notify(forceChan, syscall.SIGINT, syscall.SIGTERM)
	stopSignals()
	go func() {
		<-forceChan
		log.Println("Second signal received, forcing immediate exit")
		os.Exit(1)
	}()

	// Keep accepting requests while load balancers notice the failing
	// readiness probe, then stop the listeners.
	if rt.drainDelay > 0 {
		log.Printf("Draining: waiting %s before closing listeners", rt.drainDelay)
		time.Sleep(rt.drainDelay)
	}

	// Shutdown HTTP servers (drains in-flight requests).
	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), shutdownTimeout)
	var httpWg sync.WaitGroup
	for _, srv := range []struct {
		name string
		srv  *http.Server
	}{{"Frontend", rt.frontendSrv}, {"API", rt.apiSrv}} {
		if srv.srv == nil {
			continue
		}
		httpWg.Add(1)
		go func() {
			defer httpWg.Done()
			if err := srv.srv.Shutdown(httpCtx); err != nil {
				log.Printf("%s server shutdown error: %v", srv.name, err)
			}
		}()
	}
	httpWg.Wait()
	cancelHTTP()
	log.Println("HTTP servers shut down gracefully")

	// Background workers observe the cancelled root context; wait for them
	// before tearing down the gRPC server and plugin hosts they depend on.
	if stopped, running := rt.workers.Stop(workerStopTimeout); stopped {
		log.Println("Background workers stopped")
	} else {
		log.Printf("Background workers still running after %s: %s", workerStopTimeout, strings.Join(running, ", "))
	}

	// Stop the gRPC server after HTTP shutdown: it stops accepting new RPCs
	// and waits for existing ones to finish (bounded: agent streams are
	// long-lived).
	if rt.grpcSrv != nil {
		if !waitTimeout(rt.grpcSrv.Stop, grpcStopTimeout) {
			log.Printf("gRPC server did not stop within %s, continuing shutdown", grpcStopTimeout)
		}
	}

	if rt.controlPluginHosts != nil {
		hostCtx, cancelHosts := context.WithTimeout(context.Background(), pluginHostStopTimeout)
		if err := rt.controlPluginHosts.Shutdown(hostCtx); err != nil {
			log.Printf("Control plugin host shutdown error: %v", err)
		}
		cancelHosts()
	}
	pluginhost.SetDefaultManager(nil)
	if rt.controlPluginArtifactCleanup != nil {
		rt.controlPluginArtifactCleanup()
	}

	// Give goroutines time to finish returning from ListenAndServe.
	rt.servers.Wait()
	log.Println("All servers stopped")
}

func ensureConfiguredPluginTrustRoot(db *gorm.DB, encodedPublicKey string) error {
	publicKey, err := service.ParseOfficialPluginPublicKey(encodedPublicKey)
	if err != nil {
		return err
	}
	_, err = service.EnsurePluginTrustRoot(db, publicKey)
	return err
}

func startGRPCServer(cfg *config.Config) (*grpcserver.Server, string, error) {
	if cfg == nil || !cfg.GRPC.Enable {
		return nil, "", nil
	}

	grpcCfg := grpcserver.DefaultServerConfig()
	if cfg.GRPC.Host != "" {
		grpcCfg.Host = cfg.GRPC.Host
	}
	if cfg.GRPC.Port > 0 {
		grpcCfg.Port = cfg.GRPC.Port
	}
	grpcCfg.APIToken = cfg.GRPC.APIToken
	grpcCfg.TLSCertFile = cfg.GRPC.TLSCertFile
	grpcCfg.TLSKeyFile = cfg.GRPC.TLSKeyFile

	server := grpcserver.NewServer(grpcCfg)
	if err := server.Start(); err != nil {
		return nil, "", err
	}
	return server, fmt.Sprintf("%s:%d", grpcCfg.Host, grpcCfg.Port), nil
}

func newControlPluginHostManager(cfg *config.Config) (*pluginhost.Supervisor, error) {
	if cfg == nil {
		return nil, errors.New("plugin host configuration is required")
	}
	startupTimeout := 5 * time.Second
	if raw := strings.TrimSpace(cfg.Plugins.ControlHostStartupTimeout); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("invalid plugins.control_host_startup_timeout %q", raw)
		}
		startupTimeout = parsed
	}
	if raw := strings.TrimSpace(cfg.Plugins.ControlHostRequestTimeout); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("invalid plugins.control_host_request_timeout %q", raw)
		}
	}
	if raw := strings.TrimSpace(cfg.Plugins.ControlHostWebSocketSessionTimeout); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("invalid plugins.control_host_websocket_session_timeout %q", raw)
		}
	}
	bridgeFactory, err := identitybridge.NewFactory(cfg)
	if err != nil {
		return nil, fmt.Errorf("configure package bridge: %w", err)
	}
	return pluginhost.NewManager(pluginhost.ManagerConfig{
		RuntimeDir: cfg.Plugins.ControlHostRuntimeDir, StartupTimeout: startupTimeout,
		BridgeFactory: bridgeFactory, MaxResponseBytes: cfg.Plugins.ControlHostResponseBodyLimit(),
		PackageEnvironment: identityHostEnvironment(cfg),
	})
}

// identityHostEnvironment gives the local identity-platform host its signing
// key KEK; no other package receives it.
func identityHostEnvironment(cfg *config.Config) map[string][]string {
	kek := strings.TrimSpace(cfg.Identity.KEK)
	if kek == "" {
		return nil
	}
	return map[string][]string{"identity-platform": {"ANIX_IDENTITY_KEK=" + kek}}
}

func newControlPluginArtifactResolver(cfg *config.Config) (plugincontrol.ArtifactRefResolver, func(), error) {
	if cfg == nil {
		return nil, nil, errors.New("plugin artifact configuration is required")
	}
	publicKey, err := service.ParseOfficialPluginPublicKey(cfg.Plugins.OfficialPublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("parse plugins.official_public_key: %w", err)
	}
	root := strings.TrimSpace(cfg.Plugins.ControlHostArtifactDir)
	cleanup := func() {}
	if root == "" {
		root, err = os.MkdirTemp("", "anixops-plugin-artifacts-")
		if err != nil {
			return nil, nil, fmt.Errorf("create private plugin artifact directory: %w", err)
		}
		cleanup = func() { _ = os.RemoveAll(root) }
	} else {
		root, err = secureControlArtifactRoot(root)
		if err != nil {
			return nil, nil, err
		}
		removeStaleArtifactCopies(root)
	}
	databaseHandle := database.Get()
	resolver := func(ctx context.Context, packageID, version string) (pluginhost.ArtifactRef, error) {
		if err := ctx.Err(); err != nil {
			return pluginhost.ArtifactRef{}, err
		}
		return service.MaterializePluginControlArtifact(databaseHandle, publicKey, packageID, version, root)
	}
	return resolver, cleanup, nil
}

func secureControlArtifactRoot(configuredRoot string) (string, error) {
	if !filepath.IsAbs(configuredRoot) {
		return "", errors.New("plugins.control_host_artifact_dir must be absolute")
	}
	root := filepath.Clean(configuredRoot)
	if root == string(filepath.Separator) {
		return "", errors.New("plugins.control_host_artifact_dir must not be the filesystem root")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create plugin artifact directory: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", fmt.Errorf("inspect plugin artifact directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("plugins.control_host_artifact_dir must be a directory, not a symlink")
	}
	// #nosec G302 -- this is a directory; 0700 is the required private directory mode.
	if err := os.Chmod(root, 0o700); err != nil {
		return "", fmt.Errorf("secure plugin artifact directory: %w", err)
	}
	return root, nil
}

// removeStaleArtifactCopies deletes package copies a previous process left in
// a configured artifact directory. Artifacts are materialized again from the
// database on demand, and no host runs yet when this is called, so the
// directory must not be shared by concurrently running processes.
func removeStaleArtifactCopies(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		log.Printf("Warning: list plugin artifact directory: %v", err)
		return
	}
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), service.PluginArtifactCopyPrefix) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			log.Printf("Warning: remove stale plugin artifact copy %s: %v", entry.Name(), err)
			continue
		}
		removed++
	}
	if removed > 0 {
		log.Printf("Removed %d stale plugin artifact copies from %s", removed, root)
	}
}

// newAPIServer creates the API server with proper timeouts.
func newAPIServer(cfg *config.Config) (*http.Server, error) {
	r := gin.New()
	if err := applyTrustedProxies(r, cfg.Server.TrustedProxies); err != nil {
		return nil, fmt.Errorf("configure trusted proxies for API server: %w", err)
	}
	router.Setup(r, cfg)

	readTimeout := time.Duration(cfg.Server.ReadTimeout) * time.Second
	if readTimeout == 0 {
		readTimeout = 30 * time.Second
	}
	writeTimeout := time.Duration(cfg.Server.WriteTimeout) * time.Second
	if writeTimeout == 0 {
		writeTimeout = 60 * time.Second
	}

	return &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  120 * time.Second,
	}, nil
}

// runAPIServer starts the API server (blocking).
func runAPIServer(srv *http.Server) error {
	log.Printf("API Server starting on %s", srv.Addr)
	return srv.ListenAndServe()
}

// newFrontendServer creates the frontend server with proper timeouts.
func newFrontendServer(cfg *config.Config) (*http.Server, error) {
	frontendPath := cfg.Frontend.Path
	if frontendPath == "" {
		frontendPath = "web/public"
	}

	// 检查前端目录是否存在
	if _, err := os.Stat(frontendPath); os.IsNotExist(err) {
		log.Printf("Frontend directory '%s' not found, creating a placeholder...", frontendPath)
		// A read-only root filesystem (containers) cannot hold the placeholder;
		// the API keeps working and the UI answers 404 until assets exist.
		if err := os.MkdirAll(frontendPath, 0o750); err != nil {
			log.Printf("Warning: cannot create frontend directory %q: %v", frontendPath, err)
		} else if err := createDefaultIndex(frontendPath); err != nil {
			log.Printf("Warning: cannot create default frontend index in %q: %v", frontendPath, err)
		}
	}

	r := gin.New()
	if err := applyTrustedProxies(r, cfg.Server.TrustedProxies); err != nil {
		return nil, fmt.Errorf("configure trusted proxies for frontend server: %w", err)
	}
	r.Use(gin.Recovery())

	// Probes with draining awareness (/livez, /readyz, /health).
	health.Default.Register(r)

	// API 代理 - 将 /api 请求转发到 API 服务器
	apiTarget := frontendAPIProxyTarget(cfg)
	r.Any("/api/*path", func(c *gin.Context) {
		proxyAPI(c, apiTarget)
	})

	// 订阅代理 - 将 /s (或自定义路径) 转发到 API 服务器
	subPath := cfg.App.SubscribePath
	if subPath == "" {
		subPath = "s"
	}
	r.Any("/"+subPath+"/*path", func(c *gin.Context) {
		proxyAPI(c, apiTarget)
	})

	// 静态文件服务。Hash assets can be cached aggressively; the SPA HTML
	// entry below must stay uncached so deploys do not serve stale bundles.
	assetHandler := serveAssetFiles(frontendPath)
	r.GET("/assets/*filepath", assetHandler)
	r.HEAD("/assets/*filepath", assetHandler)
	r.StaticFile("/favicon.ico", frontendPath+"/favicon.ico")

	// SPA 路由支持 - 所有未匹配的路由返回 index.html
	r.NoRoute(func(c *gin.Context) {
		setHTMLNoCacheHeaders(c)
		c.File(filepath.Join(frontendPath, "index.html"))
	})

	port := cfg.Frontend.Port
	if port == 0 {
		port = 3000
	}

	readTimeout := time.Duration(cfg.Server.ReadTimeout) * time.Second
	if readTimeout == 0 {
		readTimeout = 30 * time.Second
	}
	writeTimeout := time.Duration(cfg.Server.WriteTimeout) * time.Second
	if writeTimeout == 0 {
		writeTimeout = 60 * time.Second
	}

	return &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, port),
		Handler:      r,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  120 * time.Second,
	}, nil
}

func frontendAPIProxyTarget(cfg *config.Config) string {
	host := strings.TrimSpace(cfg.Server.Host)
	if host == "" {
		host = "127.0.0.1"
	} else if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, strconv.Itoa(cfg.Server.Port)),
	}).String()
}

// runFrontendServer starts the frontend server (blocking).
func runFrontendServer(srv *http.Server, cfg *config.Config) error {
	if cfg.TLS.Enable && cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		log.Printf("Frontend Server starting on https://%s (serving: %s)", srv.Addr, cfg.Frontend.Path)
		return srv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	}
	log.Printf("Frontend Server starting on http://%s (serving: %s)", srv.Addr, cfg.Frontend.Path)
	return srv.ListenAndServe()
}

// createDefaultIndex 创建默认的index.html
func createDefaultIndex(path string) error {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AnixOps Control</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .container {
            text-align: center;
            color: white;
            padding: 40px;
        }
        h1 { font-size: 3rem; margin-bottom: 1rem; }
        p { font-size: 1.2rem; opacity: 0.9; margin-bottom: 2rem; }
        .status {
            background: rgba(255,255,255,0.2);
            padding: 20px 40px;
            border-radius: 10px;
            backdrop-filter: blur(10px);
        }
        .dot {
            display: inline-block;
            width: 12px;
            height: 12px;
            background: #4ade80;
            border-radius: 50%;
            margin-right: 8px;
            animation: pulse 2s infinite;
        }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>AnixOps Control</h1>
        <p>Control plane service</p>
        <div class="status">
            <span class="dot"></span>
            服务运行中
        </div>
        <p style="margin-top: 2rem; font-size: 0.9rem; opacity: 0.7;">
            将您的前端文件放入 web/public/ 目录即可
        </p>
    </div>
</body>
</html>`
	return os.WriteFile(filepath.Join(path, "index.html"), []byte(html), 0o600)
}

// proxyAPI 将 API 请求代理到后端服务器
func proxyAPI(c *gin.Context, target string) {
	targetURL, err := url.Parse(target)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "proxy error"})
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		if _, writeErr := io.WriteString(w, `{"error": "API server unavailable"}`); writeErr != nil {
			log.Printf("Proxy error response write failed: %v", writeErr)
		}
	}

	// 修改请求
	c.Request.URL.Host = targetURL.Host
	c.Request.URL.Scheme = targetURL.Scheme
	c.Request.Header.Set("X-Forwarded-Host", c.Request.Header.Get("Host"))
	c.Request.Host = targetURL.Host

	proxy.ServeHTTP(c.Writer, c.Request)
}
