package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/utils"
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
	database.GetDB().AutoMigrate(&model.Node{})
	return func() {
		database.Close()
	}
}

func TestNodeAuth_MissingToken(t *testing.T) {
	router := gin.New()
	router.Use(NodeAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestNodeAuth_ValidToken(t *testing.T) {
	// 设置配置
	cfg := &config.Config{
		App: config.AppConfig{
			APIToken: "test-token",
		},
	}
	config.Set(cfg)

	router := gin.New()
	router.Use(NodeAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test?token=test-token", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestNodeAuth_InvalidToken(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			APIToken: "correct-token",
		},
	}
	config.Set(cfg)

	router := gin.New()
	router.Use(NodeAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test?token=wrong-token", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestNodeAuth_NoConfig(t *testing.T) {
	// 清除配置
	config.Set(nil)

	router := gin.New()
	router.Use(NodeAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// 没有配置时，任何 token 都应该通过
	req := httptest.NewRequest("GET", "/test?token=any-token", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestNodeAPIKeyAuth_MissingAPIKey(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	router := gin.New()
	router.Use(NodeAPIKeyAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestNodeAPIKeyAuth_ValidKey(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 创建测试节点
	apiKey := "test-api-key-123"
	keyHash := sha256Hash(apiKey)
	node := &model.Node{
		Name:       "Test Node",
		Host:       "127.0.0.1",
		Port:       443,
		APIKeyHash: keyHash,
		Status:     model.NodeStatusOnline,
	}
	database.GetDB().Create(node)

	router := gin.New()
	router.Use(NodeAPIKeyAuth())
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

func TestNodeAPIKeyAuth_DisabledNode(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	apiKey := "test-api-key-disabled"
	keyHash := sha256Hash(apiKey)
	node := &model.Node{
		Name:       "Disabled Node",
		Host:       "127.0.0.1",
		Port:       443,
		APIKeyHash: keyHash,
		Status:     model.NodeStatusDisabled,
	}
	database.GetDB().Create(node)

	router := gin.New()
	router.Use(NodeAPIKeyAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestNodeAPIKeyAuth_QueryParam(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	apiKey := "test-api-key-query"
	keyHash := sha256Hash(apiKey)
	node := &model.Node{
		Name:       "Test Node",
		Host:       "127.0.0.1",
		Port:       443,
		APIKeyHash: keyHash,
		Status:     model.NodeStatusOnline,
	}
	database.GetDB().Create(node)

	router := gin.New()
	router.Use(NodeAPIKeyAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test?api_key="+apiKey, nil)
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

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
}

func TestCORS_OptionsRequest(t *testing.T) {
	router := gin.New()
	router.Use(CORS())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestSha256Hash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test", "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"},
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"hello world", "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sha256Hash(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSignatureAuth_NoSignature(t *testing.T) {
	router := gin.New()
	router.Use(SignatureAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// 没有签名头应该跳过验证（向后兼容）
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSignatureAuth_MissingTimestamp(t *testing.T) {
	router := gin.New()
	router.Use(SignatureAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(SignatureHeader, "some-signature")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSignatureAuth_InvalidTimestamp(t *testing.T) {
	router := gin.New()
	router.Use(SignatureAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(SignatureHeader, "some-signature")
	req.Header.Set(TimestampHeader, "invalid-timestamp")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSignatureAuth_ExpiredRequest(t *testing.T) {
	router := gin.New()
	router.Use(SignatureAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// 使用过期的时间戳 (6分钟前，超过 MaxTimeDiff)
	expiredTimestamp := time.Now().Add(-6 * time.Minute).Unix()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(SignatureHeader, "some-signature")
	req.Header.Set(TimestampHeader, strconv.FormatInt(expiredTimestamp, 10))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSignatureAuth_MissingNode(t *testing.T) {
	router := gin.New()
	router.Use(SignatureAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(SignatureHeader, "some-signature")
	req.Header.Set(TimestampHeader, strconv.FormatInt(time.Now().Unix(), 10))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSignatureAuth_ValidSignature(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	secret := "test-secret"
	apiKey := "test-api-key"
	keyHash := sha256Hash(apiKey)
	node := &model.Node{
		Name:       "Test Node",
		Host:       "127.0.0.1",
		Port:       443,
		APIKeyHash: keyHash,
		Secret:     secret,
		Status:     model.NodeStatusOnline,
	}
	database.GetDB().Create(node)

	router := gin.New()
	router.Use(NodeAPIKeyAuth())
	router.Use(SignatureAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	method := "GET"
	path := "/test"
	body := ""
	signData := timestamp + method + path + body

	// 计算 HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signData))
	signature := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set(SignatureHeader, signature)
	req.Header.Set(TimestampHeader, timestamp)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestStrictSignatureAuth_MissingSignature(t *testing.T) {
	router := gin.New()
	router.Use(StrictSignatureAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCalculateHMAC(t *testing.T) {
	secret := "test-secret"
	data := "test-data"

	result := calculateHMAC(data, secret)
	assert.NotEmpty(t, result)
	assert.Len(t, result, 64) // SHA256 produces 64 hex characters

	// Verify the result is deterministic
	result2 := calculateHMAC(data, secret)
	assert.Equal(t, result, result2)
}

func TestNonceCache(t *testing.T) {
	// Clear the nonce cache
	nonceCache = make(map[string]int64)

	nonce := "test-nonce-123"

	// Initially not used
	assert.False(t, isNonceUsed(nonce))

	// Mark as used
	markNonceUsed(nonce)
	assert.True(t, isNonceUsed(nonce))

	// Clear cache
	nonceCache = make(map[string]int64)
	assert.False(t, isNonceUsed(nonce))
}

func TestNonceExpiration(t *testing.T) {
	nonceCache = make(map[string]int64)

	nonce := "expiring-nonce"
	// Set expired nonce
	nonceCache[nonce] = time.Now().Add(-10 * time.Second).Unix()

	// Expired nonce should be considered not used
	assert.False(t, isNonceUsed(nonce))
	_, exists := nonceCache[nonce]
	assert.False(t, exists) // Should be deleted
}

func TestSecureLogger(t *testing.T) {
	router := gin.New()
	router.Use(SecureLogger())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSecureLogger_WithUserContext(t *testing.T) {
	router := gin.New()
	router.Use(SecureLogger())
	router.GET("/test", func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
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

func TestNodeSecureLogger_WithSignature(t *testing.T) {
	router := gin.New()
	router.Use(NodeSecureLogger())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(SignatureHeader, "test-signature")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuditLog(t *testing.T) {
	entry := &AuditLog{
		Timestamp:  time.Now(),
		Action:     "test_action",
		UserID:     1,
		NodeID:     2,
		IP:         "192.168.1.1",
		UserAgent:  "test-agent",
		Path:       "/test",
		Method:     "GET",
		StatusCode: 200,
		Latency:    time.Millisecond * 100,
		Extra:      map[string]interface{}{"key": "value"},
	}

	// WriteAuditLog should not panic
	WriteAuditLog(entry)

	// Check IP is redacted
	assert.NotEqual(t, "192.168.1.1", entry.IP)
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