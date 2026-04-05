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
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/anixops/v2board/docs" // swagger docs
	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/router"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// @title V2Board AnixOps API
// @version 2.0.0
// @description V2Board 高性能代理面板管理系统 API 文档
// @description 支持用户管理、节点管理、订阅系统、支付网关、流量转发等功能
// @termsOfService https://github.com/anixops/v2board

// @contact.name API Support
// @contact.url https://github.com/anixops/v2board/issues
// @contact.email support@example.com

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

var (
	configPath string
	version    = "2.0.0"
	buildTime  = "unknown"
)

func init() {
	flag.StringVar(&configPath, "config", "config/config.yaml", "配置文件路径")
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

func main() {
	flag.Parse()

	resolvedConfigPath, resolveErr := resolveConfigPath(configPath)
	log.Printf("Loading config file: %s", resolvedConfigPath)
	if resolveErr != nil {
		log.Fatalf("Failed to locate config: %v", resolveErr)
	}

	// 打印版本信息
	fmt.Printf("V2Board Go Backend v%s (build: %s)\n", version, buildTime)

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
	defer database.Close()

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
			&model.NodeGroup{},
			&model.AuthorizedKey{},
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
			&model.ForwardRuntimeJob{},
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
		); err != nil {
			log.Fatalf("Failed to migrate database: %v", err)
		}
	} else {
		log.Println("Production mode: skipping AutoMigrate. Use explicit migrations in production.")
	}

	// 初始化管理员账号
	service.InitAdmin(cfg)

	// 初始化默认订阅分组
	service.InitSubscriptionDefaults()

	// 初始化默认套餐
	service.InitDefaultPlan()

	// 初始化缓存 (默认使用内存缓存)
	cache.InitMemory()
	defer cache.CloseMemory()
	log.Println("Cache initialized: memory")
	go service.NewPanelForwardRuntimeJobExecutor(database.Get()).Start(context.Background())
	log.Println("Forward runtime executor started")

	// 设置Gin模式
	gin.SetMode(cfg.Server.Mode)

	var wg sync.WaitGroup

	// 启动前端服务器（如果启用）
	if cfg.Frontend.Enable {
		wg.Add(1)
		go func() {
			defer wg.Done()
			startFrontendServer(cfg)
		}()
	}

	// 启动API服务器
	wg.Add(1)
	go func() {
		defer wg.Done()
		startAPIServer(cfg)
	}()

	wg.Wait()
}

// startAPIServer 启动API服务器
func startAPIServer(cfg *config.Config) {
	r := gin.New()
	router.Setup(r, cfg)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
	}

	log.Printf("API Server starting on %s:%d", cfg.Server.Host, cfg.Server.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start API server: %v", err)
	}
}

// startFrontendServer 启动前端静态文件服务器
func startFrontendServer(cfg *config.Config) {
	frontendPath := cfg.Frontend.Path
	if frontendPath == "" {
		frontendPath = "web/public"
	}

	// 检查前端目录是否存在
	if _, err := os.Stat(frontendPath); os.IsNotExist(err) {
		log.Printf("Frontend directory '%s' not found, creating...", frontendPath)
		os.MkdirAll(frontendPath, 0755)
		// 创建默认的 index.html
		createDefaultIndex(frontendPath)
	}

	r := gin.New()
	r.Use(gin.Recovery())

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

	// 静态文件服务
	r.Static("/assets", frontendPath+"/assets")
	r.StaticFile("/favicon.ico", frontendPath+"/favicon.ico")

	// SPA 路由支持 - 所有未匹配的路由返回 index.html
	r.NoRoute(func(c *gin.Context) {
		c.File(frontendPath + "/index.html")
	})

	port := cfg.Frontend.Port
	if port == 0 {
		port = 3000
	}

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, port),
		Handler:      r,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
	}

	// 根据 TLS 配置决定启动方式
	if cfg.TLS.Enable && cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		log.Printf("Frontend Server starting on https://%s:%d (TLS enabled, serving: %s)", cfg.Server.Host, port, frontendPath)
		if err := server.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start frontend server with TLS: %v", err)
		}
	} else {
		log.Printf("Frontend Server starting on http://%s:%d (serving: %s)", cfg.Server.Host, port, frontendPath)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start frontend server: %v", err)
		}
	}
}

// createDefaultIndex 创建默认的index.html
func createDefaultIndex(path string) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>V2Board</title>
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
        <h1>🚀 V2Board</h1>
        <p>Go Backend v2.0.0</p>
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
	os.WriteFile(path+"/index.html", []byte(html), 0644)
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
		io.WriteString(w, `{"error": "API server unavailable"}`)
	}

	// 修改请求
	c.Request.URL.Host = targetURL.Host
	c.Request.URL.Scheme = targetURL.Scheme
	c.Request.Header.Set("X-Forwarded-Host", c.Request.Header.Get("Host"))
	c.Request.Host = targetURL.Host

	proxy.ServeHTTP(c.Writer, c.Request)
}
