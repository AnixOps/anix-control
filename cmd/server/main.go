package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/router"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

var (
	configPath string
	version    = "2.0.0"
	buildTime  = "unknown"
)

func init() {
	flag.StringVar(&configPath, "config", "config/config.yaml", "配置文件路径")
}

func main() {
	flag.Parse()

	// 打印版本信息
	fmt.Printf("V2Board Go Backend v%s (build: %s)\n", version, buildTime)

	// 加载配置
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 打印环境信息
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

	// 自动迁移数据库
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
	); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// 初始化管理员账号
	service.InitAdmin(cfg)

	// 初始化缓存 (默认使用内存缓存)
	cache.InitMemory()
	defer cache.CloseMemory()
	log.Println("Cache initialized: memory")

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
		frontendPath = "public"
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
            将您的前端文件放入 public/ 目录即可
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
