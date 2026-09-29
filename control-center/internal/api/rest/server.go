package rest

import (
	"net/http"

	"github.com/AnixOps/anix-control/control-center/internal/api/rest/handlers"
	"github.com/AnixOps/anix-control/control-center/internal/api/rest/middleware"
	"github.com/AnixOps/anix-control/control-center/internal/core/plugin"
	"github.com/AnixOps/anix-control/control-center/internal/security/auth"
	"github.com/gin-gonic/gin"
)

// Server represents the REST API server
type Server struct {
	router      *gin.Engine
	pluginMgr   *plugin.Manager
	jwtManager  *auth.JWTManager
	rbacManager *auth.RBACManager
	handlers    *handlers.Handlers
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Mode        string // debug, release, test
	PluginMgr   *plugin.Manager
	JWTManager  *auth.JWTManager
	RBACManager *auth.RBACManager
}

// NewServer creates a new REST server
func NewServer(cfg *ServerConfig) *Server {
	// Set gin mode
	gin.SetMode(cfg.Mode)

	s := &Server{
		router:      gin.New(),
		pluginMgr:   cfg.PluginMgr,
		jwtManager:  cfg.JWTManager,
		rbacManager: cfg.RBACManager,
	}

	s.handlers = handlers.NewHandlers(s.pluginMgr)

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// setupMiddleware sets up middleware
func (s *Server) setupMiddleware() {
	s.router.Use(gin.Recovery())
	s.router.Use(middleware.CORSMiddleware())
	s.router.Use(middleware.RequestLogger())
	s.router.Use(middleware.RequestID())
}

// setupRoutes sets up routes
func (s *Server) setupRoutes() {
	// Health check (no auth required)
	s.router.GET("/health", s.handlers.HealthCheck)
	s.router.GET("/ready", s.handlers.ReadinessCheck)

	// API v1
	api := s.router.Group("/api/v1")
	{
		// Auth routes
		authGroup := api.Group("/auth")
		{
			authGroup.POST("/login", s.handlers.Login)
			authGroup.POST("/logout", s.handlers.Logout)
			authGroup.POST("/refresh", s.handlers.RefreshToken)
			authGroup.GET("/me", s.AuthRequired(), s.handlers.GetCurrentUser)
		}

		// Protected routes
		protected := api.Group("")
		protected.Use(s.AuthRequired())
		{
			// Plugins
			protected.GET("/plugins", s.handlers.ListPlugins)
			protected.GET("/plugins/:name", s.handlers.GetPlugin)
			protected.POST("/plugins/:name/execute", s.handlers.ExecutePlugin)
			protected.GET("/plugins/:name/status", s.handlers.GetPluginStatus)
			protected.GET("/plugins/:name/capabilities", s.handlers.GetPluginCapabilities)

			// Nodes (via v2board plugin)
			protected.GET("/nodes", s.handlers.ListNodes)
			protected.POST("/nodes", s.handlers.CreateNode)
			protected.GET("/nodes/:id", s.handlers.GetNode)
			protected.PUT("/nodes/:id", s.handlers.UpdateNode)
			protected.DELETE("/nodes/:id", s.handlers.DeleteNode)
			protected.GET("/nodes/:id/stats", s.handlers.GetNodeStats)

			// Agents
			protected.GET("/agents", s.handlers.ListAgents)
			protected.POST("/agents/connect", s.handlers.ConnectAgent)
			protected.POST("/agents/disconnect", s.handlers.DisconnectAgent)
			protected.POST("/agents/exec", s.handlers.ExecAgent)
			protected.GET("/agents/:id/info", s.handlers.GetAgentInfo)

			// Playbooks (via ansible plugin)
			protected.GET("/playbooks", s.handlers.ListPlaybooks)
			protected.GET("/playbooks/:name", s.handlers.GetPlaybook)
			protected.POST("/playbooks/run", s.handlers.RunPlaybook)
			protected.POST("/playbooks/validate", s.handlers.ValidatePlaybook)

			// Inventory
			protected.GET("/inventory", s.handlers.GetInventory)
			protected.GET("/inventory/hosts", s.handlers.ListHosts)

			// Users
			protected.GET("/users", s.handlers.ListUsers)
			protected.POST("/users", s.handlers.CreateUser)
			protected.GET("/users/:id", s.handlers.GetUser)
			protected.PUT("/users/:id", s.handlers.UpdateUser)
			protected.DELETE("/users/:id", s.handlers.DeleteUser)
			protected.POST("/users/:id/ban", s.handlers.BanUser)
			protected.POST("/users/:id/unban", s.handlers.UnbanUser)
			protected.GET("/users/:id/subscriptions", s.handlers.GetUserSubscriptions)

			// Orders
			protected.GET("/orders", s.handlers.ListOrders)
			protected.GET("/orders/:id", s.handlers.GetOrder)

			// Plans
			protected.GET("/plans", s.handlers.ListPlans)
			protected.GET("/plans/:id", s.handlers.GetPlan)

			// Dashboard
			protected.GET("/dashboard", s.handlers.GetDashboard)
			protected.GET("/dashboard/stats", s.handlers.GetDashboardStats)

			// Logs
			protected.GET("/logs", s.handlers.GetLogs)
			protected.GET("/logs/stream", s.handlers.StreamLogs)

			// Settings
			protected.GET("/settings", s.handlers.GetSettings)
			protected.PUT("/settings", s.handlers.UpdateSettings)

			// Events (SSE for real-time updates)
			protected.GET("/events", s.handlers.StreamEvents)
		}

		// Admin routes
		admin := api.Group("/admin")
		admin.Use(s.AuthRequired())
		admin.Use(s.RequireRole("admin"))
		{
			admin.GET("/stats", s.handlers.GetAdminStats)
			admin.POST("/plugins/:name/start", s.handlers.StartPlugin)
			admin.POST("/plugins/:name/stop", s.handlers.StopPlugin)
			admin.POST("/plugins/:name/restart", s.handlers.RestartPlugin)
			admin.GET("/audit-logs", s.handlers.GetAuditLogs)
			admin.GET("/system/info", s.handlers.GetSystemInfo)
			admin.POST("/system/backup", s.handlers.CreateBackup)
			admin.GET("/system/backups", s.handlers.ListBackups)
		}
	}
}

// AuthRequired returns middleware that requires authentication
func (s *Server) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := s.extractToken(c)
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			c.Abort()
			return
		}

		claims, err := s.jwtManager.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		// Store claims in context
		c.Set("userID", claims.Subject)
		c.Set("role", claims.Role)
		c.Set("scopes", claims.Scopes)
		c.Set("authProvider", claims.AuthProvider)

		c.Next()
	}
}

// RequireRole returns middleware that requires a specific role
func (s *Server) RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "role not found"})
			c.Abort()
			return
		}

		if userRole.(string) != role && userRole.(string) != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequirePermission returns middleware that requires a specific permission
func (s *Server) RequirePermission(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")

		if !s.rbacManager.HasPermission(role.(string), resource, action) {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// extractToken extracts JWT token from request
func (s *Server) extractToken(c *gin.Context) string {
	// Check Authorization header
	auth := c.GetHeader("Authorization")
	if auth != "" {
		if len(auth) > 7 && auth[:7] == "Bearer " {
			return auth[7:]
		}
	}

	// Check query parameter
	if token := c.Query("token"); token != "" {
		return token
	}

	// Check cookie
	if cookie, err := c.Cookie("token"); err == nil {
		return cookie
	}

	return ""
}

// Run starts the server
func (s *Server) Run(addr string) error {
	return s.router.Run(addr)
}

// Router returns the underlying gin router
func (s *Server) Router() *gin.Engine {
	return s.router
}

// Stop gracefully stops the server
func (s *Server) Stop() error {
	// TODO: Implement graceful shutdown
	return nil
}
