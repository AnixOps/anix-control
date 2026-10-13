package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/adminclaim"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claimLogs captures everything the process logs while a test runs: the
// standard logger, slog and gin's access log.
type claimLogs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *claimLogs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *claimLogs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func captureClaimLogs(t *testing.T) *claimLogs {
	t.Helper()
	logs := &claimLogs{}
	writer, previousSlog, previousGin := log.Writer(), slog.Default(), gin.DefaultWriter
	log.SetOutput(logs)
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	gin.DefaultWriter = logs
	t.Cleanup(func() {
		log.SetOutput(writer)
		slog.SetDefault(previousSlog)
		gin.DefaultWriter = previousGin
	})
	return logs
}

func claimRouter(t *testing.T) *gin.Engine {
	t.Helper()
	r, _ := setupTestRouter(t)
	t.Cleanup(func() { teardownTestRouter(t) })
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&migrations.MetaEntry{}, &model.AuditLog{}))
	return r
}

func postClaim(r *gin.Engine, address string, body any) *httptest.ResponseRecorder {
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, "/api/v5/claim", bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = address + ":40000"
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, request)
	return recorder
}

func claimStatus(t *testing.T, r *gin.Engine) bool {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v5/claim/status", nil)
	request.RemoteAddr = "192.0.2.200:40000"
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	var answer struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
	require.Len(t, answer.Data, 1, "the status says claimable and nothing else")
	claimable, ok := answer.Data["claimable"].(bool)
	require.True(t, ok)
	return claimable
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var answer struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer), recorder.Body.String())
	return answer.Error.Code
}

// The claim end to end through the router: public, the answers of every
// refusal, one administrator, and the token and password in no log, no
// answer and no audit row.
func TestClaimRoutes(t *testing.T) {
	logs := captureClaimLogs(t)
	r := claimRouter(t)
	db := database.GetDB()
	assert.True(t, claimStatus(t, r), "no administrator yet")

	issued, err := adminclaim.Issue(context.Background(), db, adminclaim.DefaultTTL, time.Now())
	require.NoError(t, err)
	password := "first-owner-Pa55"
	ip := 1
	next := func() string { ip++; return fmt.Sprintf("192.0.2.%d", ip) }

	refusals := []struct {
		body   any
		status int
		code   string
	}{
		{"not an object", http.StatusBadRequest, "invalid_request"},
		{map[string]string{"token": "anixclaim_wrong", "email": "owner@example.com", "password": password}, http.StatusForbidden, "claim_invalid"},
		{map[string]string{"token": issued.Token, "email": "owner", "password": password}, http.StatusBadRequest, "invalid_email"},
		{map[string]string{"token": issued.Token, "email": "owner@example.com", "password": "12345"}, http.StatusUnprocessableEntity, "weak_password"},
		{map[string]string{"token": issued.Token, "email": "owner@example.com", "password": strings.Repeat("p", 73)}, http.StatusUnprocessableEntity, "password_too_long"},
	}
	for _, refusal := range refusals {
		response := postClaim(r, next(), refusal.body)
		assert.Equal(t, refusal.status, response.Code, response.Body.String())
		assert.Equal(t, refusal.code, errorCode(t, response))
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		assert.NotContains(t, response.Body.String(), issued.Token)
	}
	tooBig := postClaim(r, next(), map[string]string{"token": issued.Token, "email": "owner@example.com", "password": strings.Repeat("p", 5000)})
	assert.Equal(t, http.StatusBadRequest, tooBig.Code)

	response := postClaim(r, "198.51.100.9", map[string]string{"token": issued.Token, "email": "owner@example.com", "password": password})
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var created struct {
		Data struct {
			UserID uint   `json:"user_id"`
			Email  string `json:"email"`
			Next   string `json:"next"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
	assert.Equal(t, "owner@example.com", created.Data.Email)
	assert.Equal(t, "/login", created.Data.Next)
	assert.NotZero(t, created.Data.UserID)
	assert.NotContains(t, response.Body.String(), issued.Token)
	assert.NotContains(t, response.Body.String(), password)

	// Claimed: nothing more to claim, whatever the token.
	assert.False(t, claimStatus(t, r))
	again := postClaim(r, next(), map[string]string{"token": issued.Token, "email": "other@example.com", "password": password})
	assert.Equal(t, http.StatusNotFound, again.Code)
	assert.Equal(t, "not_found", errorCode(t, again))
	other := postClaim(r, next(), map[string]string{"token": "anything", "email": "other@example.com", "password": password})
	assert.Equal(t, http.StatusNotFound, other.Code)

	var users []model.User
	require.NoError(t, db.Where("is_admin = ?", 1).Find(&users).Error)
	require.Len(t, users, 1)
	assert.Equal(t, created.Data.UserID, users[0].ID)

	// One audit row: the action, the email and the source address.
	var audits []model.AuditLog
	require.NoError(t, db.Find(&audits).Error)
	require.Len(t, audits, 1)
	assert.Equal(t, "admin_claimed", audits[0].Action)
	assert.Equal(t, "owner@example.com", audits[0].Email)
	assert.Equal(t, "198.51.100.9", audits[0].IP)
	assert.Empty(t, audits[0].RequestBody)
	encoded, err := json.Marshal(audits[0])
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), issued.Token)
	assert.NotContains(t, string(encoded), password)

	// The access log saw the requests, never their bodies.
	captured := logs.String()
	assert.Contains(t, captured, "/api/v5/claim", "the access log records the requests")
	assert.NotContains(t, captured, issued.Token, "the token is in a log")
	assert.NotContains(t, captured, strings.TrimPrefix(issued.Token, adminclaim.TokenPrefix))
	assert.NotContains(t, captured, password, "the password is in a log")
}

// The claim and its status take the public rate: a burst of 10, then 429
// for that address only.
func TestClaimRoutesAreRateLimited(t *testing.T) {
	r := claimRouter(t)
	statuses := map[int]int{}
	for range 15 {
		statuses[postClaim(r, "203.0.113.50", map[string]string{"token": "x", "email": "a@example.com", "password": "secret-pass-12"}).Code]++
	}
	assert.Equal(t, 10, statuses[http.StatusForbidden], "the burst is answered")
	assert.Equal(t, 5, statuses[http.StatusTooManyRequests])
	assert.Equal(t, http.StatusForbidden, postClaim(r, "203.0.113.51", map[string]string{"token": "x"}).Code, "another address has its own bucket")
}

// Concurrent POSTs with one token through the router, on a database file
// as Control opens it: exactly one creates the administrator; the others
// answer 404 (an administrator exists) or 403 (the token is used up).
func TestClaimRouteConcurrentPostsCreateOneAdministrator(t *testing.T) {
	require.NoError(t, database.Init(&config.DatabaseConfig{Driver: "sqlite", Database: filepath.Join(t.TempDir(), "control.db"), LogLevel: "silent"}))
	t.Cleanup(func() { teardownTestRouter(t) })
	db := database.GetDB()
	_, err := migrations.Migrate(context.Background(), db, migrations.Options{ProgramVersion: "4.2.1"})
	require.NoError(t, err)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-jwt-secret", Expire: 86400}, App: config.AppConfig{SubscribePath: "s"}}
	config.Set(cfg)
	r := gin.New()
	Setup(r, cfg)

	issued, err := adminclaim.Issue(context.Background(), db, adminclaim.DefaultTTL, time.Now())
	require.NoError(t, err)
	const racers = 10
	var (
		wg       sync.WaitGroup
		start    = make(chan struct{})
		statuses = make([]int, racers)
	)
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			statuses[i] = postClaim(r, fmt.Sprintf("198.51.100.%d", 10+i), map[string]string{
				"token": issued.Token, "email": fmt.Sprintf("owner%d@example.com", i), "password": "first-owner-Pa55",
			}).Code
		}()
	}
	close(start)
	wg.Wait()
	counts := map[int]int{}
	for _, status := range statuses {
		counts[status]++
	}
	assert.Equal(t, 1, counts[http.StatusCreated], "statuses %v", statuses)
	assert.Equal(t, racers-1, counts[http.StatusNotFound]+counts[http.StatusForbidden], "statuses %v", statuses)
	var admins int64
	require.NoError(t, db.Model(&model.User{}).Where("is_admin = ?", 1).Count(&admins).Error)
	assert.Equal(t, int64(1), admins)
}
