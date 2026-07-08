package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func setupAuthRateLimitTest(t *testing.T, cfg *config.Config) (*gin.Engine, func()) {
	gin.SetMode(gin.TestMode)
	cache.InitMemory()
	service.ResetLoginRateLimiterForTest()

	err := database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().AutoMigrate(&model.User{}))

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	require.NoError(t, err)

	user := &model.User{
		Email:    "ratelimit@example.com",
		Password: string(hashedPassword),
		Token:    uuid.NewString(),
		UUID:     uuid.NewString(),
	}
	require.NoError(t, database.GetDB().Create(user).Error)

	config.Set(cfg)
	router := gin.New()
	authHandler := NewAuthHandler(cfg)
	router.POST("/api/v2/login", authHandler.Login)

	cleanup := func() {
		require.NoError(t, database.Close())
		service.ResetLoginRateLimiterForTest()
	}
	return router, cleanup
}

func performLogin(t *testing.T, router *gin.Engine, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestAuthLoginRateLimit_BlocksAfterThreshold(t *testing.T) {
	enabled := true
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret",
			Expire: 3600,
		},
		Auth: config.AuthConfig{
			LoginRateLimit: config.LoginRateLimitConfig{
				Enabled:        &enabled,
				MaxAttempts:    2,
				WindowSeconds:  600,
				LockoutSeconds: 120,
			},
		},
	}

	router, cleanup := setupAuthRateLimitTest(t, cfg)
	defer cleanup()

	resp1 := performLogin(t, router, "ratelimit@example.com", "wrong-password")
	assert.Equal(t, http.StatusUnauthorized, resp1.Code)

	resp2 := performLogin(t, router, "ratelimit@example.com", "wrong-password")
	assert.Equal(t, http.StatusUnauthorized, resp2.Code)

	resp3 := performLogin(t, router, "ratelimit@example.com", "wrong-password")
	assert.Equal(t, http.StatusTooManyRequests, resp3.Code)
	assert.NotEmpty(t, resp3.Header().Get("Retry-After"))
}

func TestAuthLoginRateLimit_SuccessClearsFailureCounter(t *testing.T) {
	enabled := true
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret",
			Expire: 3600,
		},
		Auth: config.AuthConfig{
			LoginRateLimit: config.LoginRateLimitConfig{
				Enabled:        &enabled,
				MaxAttempts:    2,
				WindowSeconds:  600,
				LockoutSeconds: 120,
			},
		},
	}

	router, cleanup := setupAuthRateLimitTest(t, cfg)
	defer cleanup()

	resp1 := performLogin(t, router, "ratelimit@example.com", "wrong-password")
	assert.Equal(t, http.StatusUnauthorized, resp1.Code)

	resp2 := performLogin(t, router, "ratelimit@example.com", "correct-password")
	assert.Equal(t, http.StatusOK, resp2.Code)

	resp3 := performLogin(t, router, "ratelimit@example.com", "wrong-password")
	assert.Equal(t, http.StatusUnauthorized, resp3.Code)
}

func TestAuthLoginRateLimit_Disabled(t *testing.T) {
	enabled := false
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret",
			Expire: 3600,
		},
		Auth: config.AuthConfig{
			LoginRateLimit: config.LoginRateLimitConfig{
				Enabled:        &enabled,
				MaxAttempts:    1,
				WindowSeconds:  600,
				LockoutSeconds: 120,
			},
		},
	}

	router, cleanup := setupAuthRateLimitTest(t, cfg)
	defer cleanup()

	resp1 := performLogin(t, router, "ratelimit@example.com", "wrong-password")
	assert.Equal(t, http.StatusUnauthorized, resp1.Code)

	resp2 := performLogin(t, router, "ratelimit@example.com", "wrong-password")
	assert.Equal(t, http.StatusUnauthorized, resp2.Code)
	assert.Empty(t, resp2.Header().Get("Retry-After"))
}
