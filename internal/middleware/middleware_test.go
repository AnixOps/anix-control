package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/authn"
	"github.com/AnixOps/anix-control/v5/internal/cache"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestDB(t *testing.T) func() {
	cache.InitMemory()
	err := database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().AutoMigrate(&model.Node{}))
	return func() {
		require.NoError(t, database.Close())
	}
}

func TestNodeAPIKeyHeaderAuth_MissingAPIKey(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	router := gin.New()
	router.Use(NodeAPIKeyHeaderAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestNodeAPIKeyHeaderAuth_ValidKey(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 创建测试节点
	apiKey := "test-api-key-123"
	keyHash := keyHash(apiKey)
	node := &model.Node{
		Name:       "Test Node",
		Host:       "127.0.0.1",
		Port:       443,
		APIKeyHash: keyHash,
		Status:     model.NodeStatusOnline,
	}
	database.GetDB().Create(node)

	router := gin.New()
	router.Use(NodeAPIKeyHeaderAuth())
	router.GET("/test", func(c *gin.Context) {
		nodeID, _ := c.Get("node_id")
		c.JSON(http.StatusOK, gin.H{"node_id": nodeID})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestNodeAPIKeyHeaderAuth_DisabledNode(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	apiKey := "test-api-key-disabled"
	keyHash := keyHash(apiKey)
	node := &model.Node{
		Name:       "Disabled Node",
		Host:       "127.0.0.1",
		Port:       443,
		APIKeyHash: keyHash,
		Status:     model.NodeStatusDisabled,
	}
	database.GetDB().Create(node)

	router := gin.New()
	router.Use(NodeAPIKeyHeaderAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestNodeAPIKeyHeaderAuth_IgnoresTheQuery(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	apiKey := "test-api-key-query"
	keyHash := keyHash(apiKey)
	node := &model.Node{
		Name:       "Test Node",
		Host:       "127.0.0.1",
		Port:       443,
		APIKeyHash: keyHash,
		Status:     model.NodeStatusOnline,
	}
	database.GetDB().Create(node)

	router := gin.New()
	router.Use(NodeAPIKeyHeaderAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	for _, query := range []string{"?api_key=", "?token="} {
		req := httptest.NewRequest("GET", "/test"+query+apiKey, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "a key in the URL is not read")
	}
}

func TestNodeAPIKeyHeaderAuth_PlainAPIKeyFallback(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	apiKey := "legacy-api-key-without-hash"
	node := &model.Node{
		Name:   "Legacy Node",
		Host:   "127.0.0.1",
		Port:   443,
		APIKey: apiKey,
		Status: model.NodeStatusOnline,
	}
	require.NoError(t, database.GetDB().Create(node).Error)

	router := gin.New()
	router.Use(NodeAPIKeyHeaderAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestJWTAuth_MissingHeader(t *testing.T) {
	router := gin.New()
	router.Use(JWTAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	router := gin.New()
	router.Use(JWTAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAuth_ValidToken(t *testing.T) {
	// 生成有效的 token
	secret := "test-secret-key"
	token, err := utils.GenerateToken(1, "test@example.com", false, secret, 3600)
	assert.NoError(t, err)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: secret,
		},
	}
	config.Set(cfg)

	router := gin.New()
	router.Use(JWTAuth())
	router.GET("/test", func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		email, _ := c.Get("email")
		c.JSON(http.StatusOK, gin.H{
			"user_id": userID,
			"email":   email,
		})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// A JWT in the query string is accepted only on a WebSocket handshake, which
// cannot carry an Authorization header; on any other request it is refused,
// the way V2Board-style panels leaked or forged credentials through the URL.
func TestJWTAuth_QueryTokenOnlyOnWebSocketUpgrade(t *testing.T) {
	secret := "test-secret-key"
	token, err := utils.GenerateToken(1, "test@example.com", false, secret, 3600)
	assert.NoError(t, err)
	config.Set(&config.Config{JWT: config.JWTConfig{Secret: secret}})

	router := gin.New()
	router.Use(JWTAuth())
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	plain := httptest.NewRequest("GET", "/test?token="+token, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, plain)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "plain request with ?token=")

	upgrade := httptest.NewRequest("GET", "/test?token="+token, nil)
	upgrade.Header.Set("Upgrade", "websocket")
	upgrade.Header.Set("Connection", "keep-alive, Upgrade")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, upgrade)
	assert.Equal(t, http.StatusOK, w.Code, "WebSocket handshake with ?token=")
}

func TestTextAccessLogOmitsQuery(t *testing.T) {
	line := textAccessLogFormatter(gin.LogFormatterParams{
		Path: "/api/v1/server/UniProxy/config?node_id=1&token=node-secret", Method: "GET", StatusCode: 200,
	})
	assert.Contains(t, line, "/api/v1/server/UniProxy/config")
	assert.NotContains(t, line, "node-secret")
	assert.NotContains(t, line, "?")
}

func TestJWTAuth_BearerPrefix(t *testing.T) {
	secret := "test-secret-key"
	token, err := utils.GenerateToken(1, "test@example.com", true, secret, 3600)
	assert.NoError(t, err)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: secret,
		},
	}
	config.Set(cfg)

	router := gin.New()
	router.Use(JWTAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// 测试 Bearer 前缀
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 测试直接 token
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAdminAuth_NotAdmin(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("is_admin", false)
		c.Next()
	})
	router.Use(AdminAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAdminAuth_IsAdmin(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("is_admin", true)
		c.Next()
	})
	router.Use(AdminAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAdminAuth_MissingContext(t *testing.T) {
	router := gin.New()
	router.Use(AdminAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCORS(t *testing.T) {
	router := gin.New()
	router.Use(CORS())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// Request without Origin header — no CORS headers should be set
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, "", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusOK, w.Code)

	// Request with Origin header — origin should be echoed back (default: allow all)
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("Origin", "https://example.com")
	w2 := httptest.NewRecorder()

	router.ServeHTTP(w2, req2)

	assert.Equal(t, "https://example.com", w2.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w2.Header().Get("Access-Control-Allow-Methods"), "GET")
	assert.Contains(t, w2.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Equal(t, "Origin", w2.Header().Get("Vary"))
}

func TestCORS_OptionsRequest(t *testing.T) {
	router := gin.New()
	router.Use(CORS())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// Preflight without Origin — should be rejected
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	// Preflight with Origin — should succeed with echoed origin
	req2 := httptest.NewRequest("OPTIONS", "/test", nil)
	req2.Header.Set("Origin", "https://example.com")
	w2 := httptest.NewRecorder()

	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusNoContent, w2.Code)
	assert.Equal(t, "https://example.com", w2.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, w2.Header().Get("Access-Control-Max-Age"))
}

func TestCORS_WithAllowedOrigins(t *testing.T) {
	// Configure allowed origins
	config.Set(&config.Config{
		Server: config.ServerConfig{
			CORS: config.CORSConfig{
				AllowedOrigins:   []string{"https://app.example.com", "https://admin.example.com"},
				AllowCredentials: true,
			},
		},
	})
	defer config.Set(nil)

	router := gin.New()
	router.Use(CORS())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// Allowed origin
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, "https://app.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))

	// Disallowed origin
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("Origin", "https://evil.com")
	w2 := httptest.NewRecorder()

	router.ServeHTTP(w2, req2)

	assert.Equal(t, "", w2.Header().Get("Access-Control-Allow-Origin"))

	// Preflight with allowed origin
	req3 := httptest.NewRequest("OPTIONS", "/test", nil)
	req3.Header.Set("Origin", "https://app.example.com")
	w3 := httptest.NewRecorder()

	router.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusNoContent, w3.Code)
	assert.Equal(t, "https://app.example.com", w3.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w3.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORS_WildcardOriginNoCredentials(t *testing.T) {
	// Wildcard origin with credentials should be denied (credentials stripped)
	config.Set(&config.Config{
		Server: config.ServerConfig{
			CORS: config.CORSConfig{
				AllowedOrigins:   []string{"*"},
				AllowCredentials: true,
			},
		},
	})
	defer config.Set(nil)

	router := gin.New()
	router.Use(CORS())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	// Credentials must NOT be set with wildcard origin
	assert.Equal(t, "", w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestNodeSecureLogger(t *testing.T) {
	router := gin.New()
	router.Use(NodeSecureLogger())
	router.GET("/test", func(c *gin.Context) {
		c.Set("node_id", uint(1))
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestLogger(t *testing.T) {
	router := gin.New()
	router.Use(Logger())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRecovery(t *testing.T) {
	router := gin.New()
	router.Use(Recovery())
	router.GET("/test", func(c *gin.Context) {
		panic("test panic")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Recovery should prevent crash and return 500
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestJWTAuth_RejectsRevokedToken(t *testing.T) {
	secret := "test-secret-key"
	config.Set(&config.Config{JWT: config.JWTConfig{Secret: secret}})
	token, err := utils.GenerateToken(41, "revoked@example.com", false, secret, 3600)
	assert.NoError(t, err)

	store := authn.NewStore(nil, time.Hour)
	store.Remember(authn.Revocation{UserID: 41, NotBefore: time.Now(), Reason: "ban"})
	authn.SetDefaultStore(store)
	t.Cleanup(func() { authn.SetDefaultStore(nil) })

	router := gin.New()
	router.Use(JWTAuth())
	router.GET("/test", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "ok"}) })
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func keyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
