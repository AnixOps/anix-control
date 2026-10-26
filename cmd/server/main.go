package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"
	_ "time/tzdata" // database.timezone and TZ resolve without system zoneinfo

	_ "github.com/AnixOps/anix-control/v5/docs" // swagger docs
	"github.com/AnixOps/anix-control/v5/internal/agenttransport"
	"github.com/AnixOps/anix-control/v5/internal/branding"
	"github.com/AnixOps/anix-control/v5/internal/cache"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	grpcserver "github.com/AnixOps/anix-control/v5/internal/grpc"
	"github.com/AnixOps/anix-control/v5/internal/handler"
	"github.com/AnixOps/anix-control/v5/internal/health"
	"github.com/AnixOps/anix-control/v5/internal/identitybridge"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/lease"
	"github.com/AnixOps/anix-control/v5/internal/logging"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	_ "github.com/AnixOps/anix-control/v5/internal/payment/gateways" // register payment gateway plugins
	"github.com/AnixOps/anix-control/v5/internal/plugincontrol"
	"github.com/AnixOps/anix-control/v5/internal/pluginhost"
	"github.com/AnixOps/anix-control/v5/internal/requestorigin"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// @title AnixOps Control API
// @version 5.0.0-rc.7
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
// @description Type "Bearer" followed by a space and a JWT, or, on the administrator APIs, an admin API token (anixadm_..., see docs/reference/admin-api-tokens.md).

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
	// debugMode runs gin in debug mode and logs at debug level: the
	// development instance (make run). server.mode and env are built in.
	debugMode bool
	version   = branding.DefaultVersion
	buildTime = "unknown"
	buildCode = ""
	commit    = "unknown"
	// agentRelease is the Agent release tag this build was released with
	// (config/AGENT_RELEASE, set with -X main.agentRelease). Empty in a
	// development build.
	agentRelease = ""
)

func init() {
	flag.StringVar(&configPath, "config", defaultConfigPath, "配置文件路径 (or "+config.ConfigPathEnv+")")
	flag.BoolVar(&printEnv, "print-env", false, "print every ANIX_CONTROL_* configuration variable and exit")
	flag.BoolVar(&debugMode, "debug", false, "development instance: gin debug mode and debug logging")
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
	printf("The variables of deprecated keys still work or warn: docs/reference/configuration.md lists them.\n")
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
	handler.ReleaseVersion = version
	handler.AgentRelease = agentRelease
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

// applyTrustedProxies installs server.trusted_proxies: on the gin engine,
// so Context.ClientIP honours X-Forwarded-For/X-Real-IP only from those
// proxies, and as the process-wide requestorigin policy, which decides
// whether X-Forwarded-Proto/Host are read. A nil list (the key unset) is the
// loopback default; an empty list trusts no proxy.
func applyTrustedProxies(r *gin.Engine, proxies []string) error {
	policy, err := requestorigin.NewPolicy(proxies)
	if err != nil {
		return err
	}
	if err := policy.ApplyTo(r); err != nil {
		return err
	}
	requestorigin.SetDefault(policy)
	return nil
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
	// A package host started by the kernel runs before anything else: it
	// loads no configuration and prints no banner.
	if code, handled := runPackageHostCommand(os.Args[1:]); handled {
		os.Exit(code)
	}
	if code := run(); code != 0 {
		os.Exit(code)
	}
}

// run starts Control and blocks until shutdown. It returns the process exit
// code so that deferred cleanup (cache, database) runs before os.Exit.
func run() int {
	if reason, removed := removedCommand(os.Args[1:]); removed {
		_, _ = fmt.Fprintln(os.Stderr, reason)
		return 2
	}
	// `config rewrite` and `config check` read configuration files only:
	// they open no database and need no root key.
	if code, handled := runConfigCommand(os.Args[1:], os.Stdout, os.Stderr); handled {
		return code
	}
	// `backup ...` opens the database itself, without migrating it.
	if code, handled := runBackupCommand(os.Args[1:], os.Stdout, os.Stderr); handled {
		return code
	}
	migrateOnly := takeMigrateCommand()
	adminArguments := takeAdminCommand()
	nodeSecretsArguments := takeNodeSecretsCommand()
	// Module, agent and node-secrets commands print JSON on stdout, and
	// admin claim-link its link: their logs go to stderr.
	jsonCommand := adminArguments != nil || nodeSecretsArguments != nil
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
	// Module and agent commands print JSON on stdout, so the banner goes to stderr.
	banner := os.Stdout
	if jsonCommand {
		banner = os.Stderr
	}
	_, _ = fmt.Fprintf(banner, "%s v%s (build: %s)\n", branding.ControlName, version, buildTime)

	// 加载配置
	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if debugMode {
		cfg.Server.Mode = gin.DebugMode
		cfg.Log.Level = "debug"
	}
	// The root key, before anything reads a key: the server creates
	// root.key (or adopts module_runtime.ca_kek into it); a command only
	// reads it, so that a command run as another user never creates it.
	serverStart := !migrateOnly && !jsonCommand && adminArguments == nil
	deferRootKeyCheck = adminArguments != nil && adminArguments[0] == "doctor"
	activeRootKey, err = resolveRootKey(cfg, serverStart, log.Printf)
	if err != nil {
		log.Fatalf("Root key: %v", err)
	}
	if !migrateOnly && !jsonCommand {
		if err := cfg.ValidateForServer(); err != nil {
			log.Fatalf("Invalid config: %v", err)
		}
	}
	if jsonCommand {
		// Module, agent and node-secrets commands print their result on stdout; logs go to stderr.
		logging.SetupTo(cfg.Log, os.Stderr)
		database.SetSQLLogOutput(os.Stderr)
	} else {
		logging.Setup(cfg.Log)
	}
	// The route modes of the v2 Control packages: the rehearsed list.
	if summary, err := service.PackageRouteDefaultsSummary(); err != nil {
		log.Fatalf("Invalid package route defaults: %v", err)
	} else if adminArguments == nil {
		log.Print(summary)
	}
	if adminArguments == nil && !migrateOnly && !jsonCommand {
		logSecretStrength(log.Printf, cfg)
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

	forwardLegacyDataDir = resolveDataDir(cfg, resolvedConfigPath)

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

	releaseBackups := setupBackups(cfg, resolvedConfigPath, serverStart || migrateOnly)
	defer releaseBackups()

	// 初始化数据库
	if err := database.Init(&cfg.Database); err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	health.Default.SetDatabasePinger(database.PingContext)
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Database close error: %v", err)
		}
	}()

	if err := bootstrapDatabase(context.Background(), cfg); err != nil {
		log.Fatalf("Failed to prepare database: %v", err)
	}
	if migrateOnly {
		log.Println("Database schema and seed data are up to date; migrate finished.")
		return 0
	}
	if adminArguments != nil {
		if err := runAdminCommand(context.Background(), cfg, database.Get(), adminArguments, os.Stdout); err != nil {
			log.Printf("%s: %v", adminArguments[0], err)
			return adminCommandExitCode(err)
		}
		return 0
	}
	if nodeSecretsArguments != nil {
		if err := runNodeSecretsCommand(context.Background(), database.Get(), nodeSecretsArguments, os.Stdout); err != nil {
			log.Printf("node-secrets: %v", err)
			if errors.Is(err, errNodeSecretsMismatch) || errors.Is(err, errNodeSecretsInvalid) {
				return 3
			}
			return 2
		}
		return 0
	}
	// A panel nobody has claimed yet says how to claim it, once per start.
	noteMissingAdministrator(context.Background(), database.Get(), log.Printf)
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

	// The panel's certificates (internal/panelcert): what is stored, the
	// files and the self-signed fallback are loaded now, so that every
	// listener serves the right certificate from its first handshake; the
	// ACME requests start with the workers.
	panel := cfg.Panel()
	certs, err := newPanelCertificates(rootCtx, cfg, panel, database.Get())
	if err != nil {
		log.Fatalf("Failed to prepare the panel certificates: %v", err)
	}
	panelcert.SetDefault(certs)
	defer panelcert.SetDefault(nil)

	// Bind the required node-facing listener before starting workers or HTTP
	// servers. A port conflict must fail startup without exposing a partially
	// initialized Control instance.
	grpcSrv, grpcAddr, err := startGRPCServer(cfg, panel, certs)
	if err != nil {
		log.Fatalf("Failed to start gRPC server: %v", err)
	}
	if grpcSrv != nil {
		log.Printf("gRPC server listening on %s (%s)", grpcAddr, describeGRPCTLS(panel))
	}
	agenttransport.SetMode(cfg.AgentControl.MTLSOrDefault())
	log.Print(agentTransportPolicyLog(cfg, grpcSrv))
	// Under required, warn about the enabled nodes it refuses (legacy
	// Agents, nodes that never enrolled). It never stops the start.
	logRequiredReadiness(rootCtx, cfg, database.Get(), log.Printf)
	// KernelNodeOps dispatches node operations on the Agent Control streams
	// of both node kinds (node-ops-service.md section 3.8) and serves the
	// node configuration and agent kinds (section 3.11, NO-6).
	kernelnodeops.UseAgentStreams(grpcserver.GetAgentStreams())
	if err := kernelnodeops.RegisterNodeOperationExecutors(kernelnodeops.DefaultExecutors, nil); err != nil {
		log.Fatalf("Failed to register kernel node operations: %v", err)
	}

	// 从这里开始已有组件在运行：之后的错误不再 log.Fatal，而是走同一套有序关闭，
	// 清理完成后以非零状态码退出。
	drainDelay, _ := cfg.Server.DrainDelay() // validated by config.Load
	rt := &serverRuntime{
		grpcSrv:    grpcSrv,
		certs:      certs,
		panel:      panel,
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

// pluginPollIntervals holds the validated poll intervals of the plugin
// control loops. The Agent plugin dispatcher and the topology executor are
// off until M4 (service.AgentPluginDispatchEnabled,
// service.TopologyExecutionEnabled) and have no setting.
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
	intervals := pluginPollIntervals{dispatch: service.AgentPluginWorkerPollInterval, topology: service.AgentPluginWorkerPollInterval}
	var err error
	if intervals.control, err = parsePluginPollInterval("plugins.control_poll_interval", cfg.Plugins.ControlPollInterval); err != nil {
		return intervals, err
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

	// certs serves and renews the panel's certificates; panel is the
	// listener layout; panelServers are its bound listeners.
	certs        *panelcert.Manager
	panel        config.PanelSettings
	panelServers []*panelServer
	servers      sync.WaitGroup
	drainDelay   time.Duration
}

// start launches plugin hosts, background workers and HTTP servers. On error
// it returns immediately; the caller still runs shutdown for whatever started.
func (rt *serverRuntime) start(cfg *config.Config, intervals pluginPollIntervals) error {
	if err := rt.startTokenRevocations(cfg); err != nil {
		return err
	}
	rt.startSubscriberChangePruner()
	rt.startOrderPaymentReconciler()
	rt.startAgentPKIMaintenance()
	if err := rt.startKernelCAMaintenance(cfg, database.Get()); err != nil {
		return err
	}
	pluginhost.SetDefaultManager(nil)
	// The built-in packages always run: their hosts start with Control.
	{
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

	// The panel's one handler on the listeners of tls.mode, all bound
	// before any serves: a port conflict stops the start.
	handler, err := newPanelHandler(cfg)
	if err != nil {
		return err
	}
	servers, err := bindPanelServers(cfg, rt.panel, rt.certs, handler)
	if err != nil {
		return err
	}
	rt.panelServers = servers
	log.Print(describePanel(rt.panel, servers))
	rt.workers.Go("panel certificates", rt.certs.Run)

	// Serve in goroutines. A listener failure triggers shutdown.
	for _, server := range rt.panelServers {
		rt.servers.Add(1)
		go func() {
			defer rt.servers.Done()
			if err := server.serve(); err != nil {
				rt.fatal.Report(fmt.Errorf("panel %s listener: %w", server.purpose, err))
			}
		}()
	}

	// Off until M4 (service.AgentPluginDispatchEnabled): the code stays.
	if service.AgentPluginDispatchEnabled && rt.grpcSrv != nil {
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
	// Off until M4 (service.TopologyExecutionEnabled): the code stays.
	if service.TopologyExecutionEnabled && service.AgentPluginDispatchEnabled {
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
	for _, server := range rt.panelServers {
		httpWg.Add(1)
		go func() {
			defer httpWg.Done()
			if err := server.server.Shutdown(httpCtx); err != nil {
				log.Printf("Panel %s listener shutdown error: %v", server.purpose, err)
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
	// Agent transport sightings not yet written (at most a minute old).
	flushCtx, cancelFlush := context.WithTimeout(context.Background(), 5*time.Second)
	if err := agenttransport.Default().Flush(flushCtx); err != nil {
		log.Printf("Agent transport inventory flush failed: %v", err)
	}
	cancelFlush()

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

// startGRPCServer binds the node gRPC listener (always with domain, else
// with grpc.enabled). Its certificate comes from certs per handshake
// (renewals and reloads reach new connections); without a gRPC
// certificate (no name and no files) it is plain.
func startGRPCServer(cfg *config.Config, panel config.PanelSettings, certs *panelcert.Manager) (*grpcserver.Server, string, error) {
	if cfg == nil || !panel.GRPCEnabled {
		return nil, "", nil
	}

	grpcCfg := grpcserver.DefaultServerConfig()
	if cfg.GRPC.Host != "" {
		grpcCfg.Host = cfg.GRPC.Host
	}
	grpcCfg.Port = panel.GRPCPort
	grpcCfg.APIToken = cfg.GRPC.APIToken
	// grpc.edge_proxies: a reverse proxy terminates the nodes' TLS (no gRPC
	// certificate here) and the listener admits its address only.
	grpcCfg.EdgeProxies = panel.GRPCEdgeProxies
	if certs != nil && certs.HasRole(panelcert.RoleGRPC) {
		grpcCfg.GetCertificate = certs.GetCertificate(panelcert.RoleGRPC)
	}
	grpcCfg.AgentMTLS = cfg.AgentControl.MTLSOrDefault()
	grpcCfg.AgentLegacySunset, _ = cfg.AgentControl.LegacySunsetTime() // validated by ValidateForServer
	agentPKI, err := agentPKIForGRPC(cfg, database.Get())
	if err != nil {
		return nil, "", err
	}
	grpcCfg.AgentPKI = agentPKI
	grpcCfg.OnJoin = adminJoinNotifier{db: database.Get(), cfg: cfg}.Notify

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
	// Built-in package hosts execute this binary again: record its identity
	// before the first one starts (docs/architecture/plugin-kernel-contract.md,
	// "Built-in Packages"). The digest is the binary inside the release
	// archive that SHA256SUMS.txt (signed by SHA256SUMS.txt.sig) covers.
	if self, err := pluginhost.Self(); err != nil {
		log.Printf("WARNING: the Control executable cannot be recorded, so no built-in package host can start: %v", err)
	} else {
		log.Printf("Control executable %s (SHA-256 %s): built-in package hosts run this file", self.Path, self.SHA256)
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
