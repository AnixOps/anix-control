package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	_ "github.com/AnixOps/anix-control/v4/docs" // swagger docs
	"github.com/AnixOps/anix-control/v4/internal/branding"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	grpcserver "github.com/AnixOps/anix-control/v4/internal/grpc"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	_ "github.com/AnixOps/anix-control/v4/internal/payment/gateways" // register payment gateway plugins
	"github.com/AnixOps/anix-control/v4/internal/plugincontrol"
	"github.com/AnixOps/anix-control/v4/internal/router"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// @title AnixOps Control API
// @version 4.0.0-alpha.2
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
	version    = branding.DefaultVersion
	buildTime  = "unknown"
	buildCode  = ""
	commit     = "unknown"
)

func init() {
	flag.StringVar(&configPath, "config", "config/config.yaml", "配置文件路径")
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

// draining is set to 1 during graceful shutdown so /health returns 503.
var draining atomic.Int64

func healthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if draining.Load() == 1 {
			c.Header("Retry-After", "30")
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "draining"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
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
	flag.Parse()

	resolvedConfigPath, resolveErr := resolveConfigPath(configPath)
	log.Printf("Loading config file: %s", resolvedConfigPath)
	if resolveErr != nil {
		log.Fatalf("Failed to locate config: %v", resolveErr)
	}

	// 打印版本信息
	syncBuildInfo()
	fmt.Printf("%s v%s (build: %s)\n", branding.ControlName, version, buildTime)

	// 加载配置
	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
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
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Database close error: %v", err)
		}
	}()

	// 自动迁移数据库：仅在 development 或 test 环境下运行，避免在生产环境自动修改数据库结构
	if env == "development" || env == "test" {
		if err := database.AutoMigrate(
			&model.User{},
			&model.Plan{},
			&model.Order{},
			&model.Payment{},
			&model.PaymentLog{},
			&model.ServerVMess{},
			&model.ServerVLESS{},
			&model.ServerTrojan{},
			&model.ServerShadowsocks{},
			// 新版节点管理
			&model.Node{},
			&model.NodeProtocol{},
			&model.WireGuardPeer{},
			&model.NodeGroup{},
			&model.AuthorizedKey{},
			// 流量与统计日志
			&model.TrafficLog{},
			&model.OnlineLog{},
			&model.StatUser{},
			&model.StatServer{},
			&model.NodeLog{},
			// 订阅分组和模板
			&model.SubscriptionGroup{},
			&model.SubscriptionTemplate{},
			&model.UserSubscriptionGroup{},
			&model.PlanSubscriptionGroup{},
			// 事件与新模型
			&model.Event{},
			// 工单系统
			&model.Ticket{},
			&model.TicketMessage{},
			// 优惠券系统
			&model.Coupon{},
			&model.CouponUsage{},
			// 知识库
			&model.Knowledge{},
			// 流量转发系统
			&model.ForwardNode{},
			&model.ForwardRule{},
			&model.ForwardRoute{},
			&model.ForwardLog{},
			&model.ForwardStats{},
			&model.ForwardTunnel{},
			&model.ForwardUserTunnel{},
			&model.Forward{},
			&model.ForwardPortBinding{},
			&model.SpeedLimit{},
			&model.ForwardRuntimeJob{},
			&model.ForwardTrafficCursor{},
			&model.ForwardCleanAgent{},
			&model.ForwardAgentBridgeTask{},
			&model.ForwardLatencyBucket{},
			// 支付网关
			&model.PaymentGateway{},
			&model.PaymentRecord{},
			// Telegram Bot
			&model.TelegramBot{},
			&model.TelegramUser{},
			&model.TelegramChat{},
			&model.TelegramCommand{},
			&model.TelegramNotification{},
			// 通知系统
			&model.NotificationTemplate{},
			&model.NotificationLog{},
			// MFA多因素认证
			&model.UserMFA{},
			&model.MFALoginAttempt{},
			// 邀请返利系统
			&model.UserLevel{},
			&model.InviteCode{},
			&model.CommissionRecord{},
			&model.CommissionWithdraw{},
			&model.InviteConfig{},
			// 系统管理
			&model.LoadBalancer{},
			&model.SystemConfig{},
			&model.BackupRecord{},
			&model.BackupConfig{},
			&model.OperationLog{},
			&model.AuditLog{},
		); err != nil {
			log.Fatalf("Failed to migrate database: %v", err)
		}
	} else {
		log.Println("Production mode: skipping AutoMigrate. Use explicit migrations in production.")
	}
	if err := service.EnsureWireGuardPeerSchema(database.Get()); err != nil {
		log.Fatalf("Failed to ensure WireGuard peer schema: %v", err)
	}
	if err := service.EnsureNodeRuntimeHealthSchema(database.Get()); err != nil {
		log.Fatalf("Failed to ensure node runtime health schema: %v", err)
	}
	if err := service.EnsureKernelSchema(database.Get()); err != nil {
		log.Fatalf("Failed to ensure control kernel schema: %v", err)
	}

	// 初始化管理员账号
	service.InitAdmin(cfg)

	// 初始化默认订阅分组
	service.InitSubscriptionDefaults()

	// 初始化默认套餐
	service.InitDefaultPlan()

	// 从环境变量初始化默认授权密钥
	service.InitDefaultAuthKeyFromEnv()

	// 初始化缓存 (默认使用内存缓存)
	if err := service.InitForwardRuntimeSystemConfig(database.Get()); err != nil {
		log.Fatalf("Failed to initialize forward runtime config: %v", err)
	}
	if err := service.EnsureObservabilitySchema(database.Get()); err != nil {
		log.Fatalf("Failed to ensure observability schema: %v", err)
	}
	if err := service.EnsureStatsSchema(database.Get()); err != nil {
		log.Fatalf("Failed to ensure stats schema: %v", err)
	}
	if err := service.EnsureForwardBridgeSchema(database.Get()); err != nil {
		log.Fatalf("Failed to ensure forward bridge schema: %v", err)
	}
	if err := service.EnsureForwardRuntimeJobSchema(database.Get()); err != nil {
		log.Fatalf("Failed to ensure forward runtime job schema: %v", err)
	}
	if err := service.EnsureForwardPortBindingSchema(database.Get()); err != nil {
		log.Fatalf("Failed to ensure forward port binding schema: %v", err)
	}
	if err := service.EnsureForwardNodeMetricsPortColumn(database.Get()); err != nil {
		log.Fatalf("Failed to ensure forward node metrics_port column: %v", err)
	}
	if err := service.EnsureAgentDiagnosticTaskSchema(database.Get()); err != nil {
		log.Fatalf("Failed to ensure agent diagnostic task schema: %v", err)
	}
	cache.InitMemory()
	defer cache.CloseMemory()
	log.Println("Cache initialized: memory")

	var controlPluginCancel context.CancelFunc
	if cfg.Plugins.ControlExecutionEnabled {
		registry, err := plugincontrol.DefaultRegistry(database.Get())
		if err != nil {
			log.Fatalf("Failed to initialize Control plugin executors: %v", err)
		}
		worker, err := plugincontrol.NewOperationWorker(database.Get(), registry)
		if err != nil {
			log.Fatalf("Failed to initialize Control plugin lifecycle worker: %v", err)
		}
		interval := 5 * time.Second
		if raw := strings.TrimSpace(cfg.Plugins.ControlPollInterval); raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil || parsed <= 0 {
				log.Fatalf("Invalid plugins.control_poll_interval %q", raw)
			}
			interval = parsed
		}
		queued, err := worker.QueueReconciliation(uuid.NewString())
		if err != nil {
			log.Printf("Control plugin restart reconciliation queued with errors: %v", err)
		}
		workerCtx, cancelWorker := context.WithCancel(context.Background())
		controlPluginCancel = cancelWorker
		worker.Start(workerCtx, interval, func(err error) {
			log.Printf("Control plugin lifecycle worker error: %v", err)
		})
		log.Printf("Control plugin execution enabled (poll interval %s, reconciliation operations %d)", interval, queued)
	}

	// 设置Gin模式
	go func() {
		executor := service.NewPanelForwardRuntimeJobExecutor(database.Get())
		executor.Start(context.Background())
	}()

	if shouldStartForwardAgentBridgeWorker(cfg) {
		go func() {
			worker := service.NewForwardAgentBridgeWorker(database.Get())
			worker.Start(context.Background())
		}()
	} else {
		log.Println("Forward clean_agent legacy bridge worker disabled")
	}

	go func() {
		worker := service.NewForwardFlowResetWorker(database.Get())
		if err := worker.RunOnce(time.Now()); err != nil {
			log.Printf("Initial forward flow reset run failed: %v", err)
		}
		worker.Start(context.Background())
	}()

	go func() {
		worker := service.NewNodeMonthlyResetWorker(database.Get())
		if err := worker.RunOnce(time.Now()); err != nil {
			log.Printf("Initial node monthly reset run failed: %v", err)
		}
		worker.Start(context.Background())
	}()

	go func() {
		worker := service.NewForwardGostStatsWorker(database.Get())
		worker.Start(context.Background())
	}()

	go func() {
		worker := service.NewForwardAnsibleStatsWorker(database.Get())
		worker.Start(context.Background())
	}()

	go func() {
		prober := service.NewForwardLatencyProber(database.Get())
		prober.Start(context.Background())
	}()

	gin.SetMode(cfg.Server.Mode)

	// Create HTTP servers with proper timeouts before starting goroutines.
	apiSrv := newAPIServer(cfg)

	var frontendSrv *http.Server
	if cfg.Frontend.Enable {
		frontendSrv = newFrontendServer(cfg)
	}

	// Start the servers in goroutines.
	var wg sync.WaitGroup

	if cfg.Frontend.Enable && frontendSrv != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runFrontendServer(frontendSrv, cfg); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start frontend server: %v", err)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := runAPIServer(apiSrv); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start API server: %v", err)
		}
	}()

	// Start the node-facing gRPC server (AnixOps Agent nodes connect here) when enabled.
	var grpcSrv *grpcserver.Server
	var kernelDispatchCancel context.CancelFunc
	var topologyExecutionCancel context.CancelFunc
	if cfg.GRPC.Enable {
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
		grpcSrv = grpcserver.NewServer(grpcCfg)
		if err := grpcSrv.Start(); err != nil {
			log.Fatalf("Failed to start gRPC server: %v", err)
		}
		log.Printf("gRPC server listening on %s:%d", grpcCfg.Host, grpcCfg.Port)
	}
	if cfg.Plugins.DispatchEnabled {
		if grpcSrv == nil {
			log.Fatal("Plugin operation dispatch requires grpc.enabled=true")
		}
		interval := 5 * time.Second
		if raw := strings.TrimSpace(cfg.Plugins.DispatchPollInterval); raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil || parsed <= 0 {
				log.Fatalf("Invalid plugins.dispatch_poll_interval %q", raw)
			}
			interval = parsed
		}
		bridge, err := grpcserver.NewKernelOperationBridge(database.Get(), grpcSrv.GetAgentControlManager())
		if err != nil {
			log.Fatalf("Failed to initialize plugin operation dispatcher: %v", err)
		}
		dispatchCtx, cancelDispatch := context.WithCancel(context.Background())
		kernelDispatchCancel = cancelDispatch
		bridge.Start(dispatchCtx, interval, func(err error) {
			log.Printf("Plugin operation dispatcher error: %v", err)
		})
		log.Printf("Plugin operation dispatcher enabled (poll interval %s)", interval)
	}
	if cfg.Plugins.TopologyExecutionEnabled {
		if !cfg.Plugins.DispatchEnabled {
			log.Fatal("Topology execution requires plugins.dispatch_enabled=true")
		}
		interval := 5 * time.Second
		if raw := strings.TrimSpace(cfg.Plugins.TopologyPollInterval); raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil || parsed <= 0 {
				log.Fatalf("Invalid plugins.topology_poll_interval %q", raw)
			}
			interval = parsed
		}
		executor, err := service.NewTopologyDeploymentExecutor(database.Get())
		if err != nil {
			log.Fatalf("Failed to initialize topology deployment executor: %v", err)
		}
		topologyCtx, cancelTopology := context.WithCancel(context.Background())
		topologyExecutionCancel = cancelTopology
		executor.Start(topologyCtx, interval, func(err error) {
			log.Printf("Topology deployment executor error: %v", err)
		})
		log.Printf("Topology deployment execution enabled (poll interval %s)", interval)
	}

	// Wait for shutdown signal, then gracefully stop all servers.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutdown signal received, draining in-flight requests...")

	// Mark servers as draining so /health returns 503 to load balancers.
	draining.Store(1)

	// Register a second-signal handler for immediate force-exit.
	forceChan := make(chan os.Signal, 1)
	signal.Notify(forceChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-forceChan
		log.Println("Second signal received, forcing immediate exit")
		os.Exit(1)
	}()

	// Create a timeout context for graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Shutdown HTTP servers (drains in-flight requests).
	var httpWg sync.WaitGroup
	if frontendSrv != nil {
		httpWg.Add(1)
		go func() {
			defer httpWg.Done()
			if err := frontendSrv.Shutdown(ctx); err != nil {
				log.Printf("Frontend server shutdown error: %v", err)
			}
		}()
	}

	httpWg.Add(1)
	go func() {
		defer httpWg.Done()
		if err := apiSrv.Shutdown(ctx); err != nil {
			log.Printf("API server shutdown error: %v", err)
		}
	}()

	httpWg.Wait()
	log.Println("HTTP servers shut down gracefully")

	// Stop the gRPC server after HTTP shutdown: it stops accepting new RPCs
	// and waits for existing ones to finish.
	if grpcSrv != nil {
		grpcSrv.Stop()
	}
	if kernelDispatchCancel != nil {
		kernelDispatchCancel()
	}
	if topologyExecutionCancel != nil {
		topologyExecutionCancel()
	}
	if controlPluginCancel != nil {
		controlPluginCancel()
	}

	// Give goroutines time to finish returning from ListenAndServe.
	wg.Wait()
	log.Println("All servers stopped")
}

// newAPIServer creates the API server with proper timeouts.
func newAPIServer(cfg *config.Config) *http.Server {
	r := gin.New()
	if err := applyTrustedProxies(r, cfg.Server.TrustedProxies); err != nil {
		log.Fatalf("Failed to configure trusted proxies for API server: %v", err)
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
	}
}

// runAPIServer starts the API server (blocking).
func runAPIServer(srv *http.Server) error {
	log.Printf("API Server starting on %s", srv.Addr)
	return srv.ListenAndServe()
}

// newFrontendServer creates the frontend server with proper timeouts.
func newFrontendServer(cfg *config.Config) *http.Server {
	frontendPath := cfg.Frontend.Path
	if frontendPath == "" {
		frontendPath = "web/public"
	}

	// 检查前端目录是否存在
	if _, err := os.Stat(frontendPath); os.IsNotExist(err) {
		log.Printf("Frontend directory '%s' not found, creating...", frontendPath)
		if err := os.MkdirAll(frontendPath, 0o750); err != nil {
			log.Fatalf("Failed to create frontend directory %q: %v", frontendPath, err)
		}
		// 创建默认的 index.html
		if err := createDefaultIndex(frontendPath); err != nil {
			log.Fatalf("Failed to create default frontend index in %q: %v", frontendPath, err)
		}
	}

	r := gin.New()
	if err := applyTrustedProxies(r, cfg.Server.TrustedProxies); err != nil {
		log.Fatalf("Failed to configure trusted proxies for frontend server: %v", err)
	}
	r.Use(gin.Recovery())

	// Health check with draining awareness.
	r.GET("/health", healthHandler())

	// API 代理 - 将 /api 请求转发到 API 服务器
	apiTarget := fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
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
	}
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
