package identitycompat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
)

func legacyAnswer(t *testing.T, serve gin.HandlerFunc, body string) map[string]any {
	t.Helper()
	engine := gin.New()
	engine.POST("/", serve)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	var answer map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
	return answer
}

// Before finalize, switching the routes back to legacy is the rollback: what
// identity changed in the meantime must work through the legacy handlers.
func TestLegacyRoutesWorkAfterNativeChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database.Reset()
	require.NoError(t, database.Init(&config.DatabaseConfig{Driver: "sqlite", Database: filepath.Join(t.TempDir(), "kernel.db")}))
	t.Cleanup(func() { _ = database.Close(); database.Reset() })
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(Models...))
	seedUsers(nil, nil)(t, db)
	ctx := context.Background()

	handlers := nativeService(db).Handlers()
	register := handlers["identity.auth.register"]
	response, err := register(ctx, pluginhostsdk.NativeRequest{Body: []byte(`{"email":"native@example.test","password":"native-secret"}`)})
	require.NoError(t, err)
	require.Contains(t, string(response.Body), `"token"`)

	login := func(email, secret string) map[string]any {
		body, _ := json.Marshal(map[string]string{"email": email, "password": secret})
		return legacyAnswer(t, func(c *gin.Context) { handler.NewAuthHandler(config.Get()).Login(c) }, string(body))
	}
	answer := login("native@example.test", "native-secret")
	require.EqualValues(t, 0, answer["code"], "a native registration logs in through the legacy route: %v", answer)

	setup := handlers["identity.user.mfa.totp.setup.post"]
	response, err = setup(ctx, pluginhostsdk.NativeRequest{Principal: pluginhostsdk.Principal{ActorID: 2}})
	require.NoError(t, err)
	var setupAnswer struct {
		Data struct {
			Secret string `json:"secret"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body, &setupAnswer))
	code, err := totp.GenerateCode(setupAnswer.Data.Secret, time.Now())
	require.NoError(t, err)
	enable := handlers["identity.user.mfa.totp.enable.post"]
	response, err = enable(ctx, pluginhostsdk.NativeRequest{Principal: pluginhostsdk.Principal{ActorID: 2}, Body: []byte(`{"code":"` + code + `"}`)})
	require.NoError(t, err)
	require.Contains(t, string(response.Body), "MFA enabled successfully")

	var legacyMFA model.UserMFA
	require.NoError(t, db.Take(&legacyMFA, "user_id = ?", 2).Error)
	require.True(t, legacyMFA.Enabled, "MFA enabled natively is enabled for the legacy route")
	require.Equal(t, setupAnswer.Data.Secret, legacyMFA.TOTPSecret)
	answer = login("member@example.test", password)
	require.Equal(t, true, answer["data"].(map[string]any)["mfa_required"], "the legacy login asks for the second factor: %v", answer)
}
