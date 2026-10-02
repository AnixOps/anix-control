package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

const selfResetSecret = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"

func setupSubscriptionResetTest(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service.ResetLoginRateLimiterForTest()
	database.Reset()
	require.NoError(t, database.Init(&config.DatabaseConfig{Driver: "sqlite", Database: filepath.Join(t.TempDir(), "reset.db"), LogLevel: "silent"}))
	t.Cleanup(func() {
		_ = database.Close()
		database.Reset()
		service.ResetLoginRateLimiterForTest()
	})
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserMFA{}, &model.SubscriberRequest{}, &model.SubscriberChange{}, &model.IdentityRevocation{}))
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	require.NoError(t, err)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 2, Email: "member@example.test", Password: string(hash), UUID: "u2", Token: "t2"},
		{ID: 5, Email: "mfa@example.test", Password: string(hash), UUID: "u5", Token: "t5"},
	}).Error)
	require.NoError(t, db.Create(&model.UserMFA{UserID: 5, Enabled: true, TOTPSecret: selfResetSecret, BackupCodes: `["AAAA-BBBB"]`}).Error)
	config.Set(&config.Config{JWT: config.JWTConfig{Secret: "subscription-reset-test", Expire: 3600}})
	router := gin.New()
	router.POST("/api/v2/user/subscription/reset", func(c *gin.Context) {
		if id := c.GetHeader("X-Test-User"); id == "5" {
			c.Set("user_id", uint(5))
		} else {
			c.Set("user_id", uint(2))
		}
		NewUserHandler().ResetSubscription(c)
	})
	return router
}

func selfReset(t *testing.T, router *gin.Engine, user, body string) (map[string]any, http.Header) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/user/subscription/reset", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-User", user)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &decoded), recorder.Body.String())
	return decoded, recorder.Header()
}

func storedSubscriber(t *testing.T, id uint) model.User {
	t.Helper()
	var user model.User
	require.NoError(t, database.GetDB().Take(&user, id).Error)
	return user
}

// The legacy handler rotates exactly what the administrator's reset
// rotates (the token, not the uuid), after the user re-authenticates, and
// records it in the subscriber request ledger under the user's own id.
func TestResetSubscriptionRotatesTheTokenAfterThePassword(t *testing.T) {
	router := setupSubscriptionResetTest(t)
	answer, _ := selfReset(t, router, "2", `{"password":"wrong"}`)
	require.Equal(t, "invalid password", answer["msg"])
	require.Equal(t, "t2", storedSubscriber(t, 2).Token)
	answer, _ = selfReset(t, router, "2", `{}`)
	require.Equal(t, "password required", answer["msg"])

	answer, _ = selfReset(t, router, "2", `{"password":"correct-horse"}`)
	require.EqualValues(t, 0, answer["code"], "%v", answer)
	token := answer["data"].(map[string]any)["token"].(string)
	stored := storedSubscriber(t, 2)
	require.Equal(t, token, stored.Token)
	require.NotEqual(t, "t2", token)
	require.Equal(t, "u2", stored.UUID, "the proxy uuid is kept")
	var requests []model.SubscriberRequest
	require.NoError(t, database.GetDB().Find(&requests).Error)
	require.Len(t, requests, 1)
	require.Contains(t, requests[0].RequestID, "identity.user_reset_subscribe:2:")
	require.Equal(t, service.SubscriberReissueMethod, requests[0].Method)

	// A wrong and a right password so far (the missing one does not
	// count): the third attempt in the hour is the last.
	answer, _ = selfReset(t, router, "2", `{"password":"correct-horse"}`)
	require.EqualValues(t, 0, answer["code"], "%v", answer)
	answer, headers := selfReset(t, router, "2", `{"password":"correct-horse"}`)
	require.Equal(t, service.SubscriptionResetRateLimitedMessage, answer["msg"])
	require.NotEmpty(t, headers.Get("Retry-After"))
}

func TestResetSubscriptionWithASecondFactor(t *testing.T) {
	router := setupSubscriptionResetTest(t)
	answer, _ := selfReset(t, router, "5", `{"password":"correct-horse"}`)
	require.Equal(t, "mfa code required", answer["msg"])
	answer, _ = selfReset(t, router, "5", `{"code":"000000"}`)
	require.Equal(t, "invalid mfa code", answer["msg"])
	code, err := totp.GenerateCode(selfResetSecret, time.Now())
	require.NoError(t, err)
	answer, _ = selfReset(t, router, "5", `{"code":"`+code+`","method":"totp"}`)
	require.EqualValues(t, 0, answer["code"], "%v", answer)
	require.NotEqual(t, "t5", storedSubscriber(t, 5).Token)
}
