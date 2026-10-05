package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// telegramTestToken is shaped like a bot token and recognisable: no response,
// audit row or log line may contain it.
const telegramTestToken = "123456789:SECRETtokenValueForTests-0123456789abc"

const telegramPostgresDSN = "ANIX_TEST_POSTGRES_DSN"

var telegramTestGormConfig = &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true}

// forEachTelegramDatabase runs body on SQLite and, when
// ANIX_TEST_POSTGRES_DSN is set, on PostgreSQL (a throwaway schema each).
func forEachTelegramDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	migrate := func(t *testing.T, db *gorm.DB) {
		require.NoError(t, db.AutoMigrate(&model.TelegramBot{}, &model.TelegramUser{}, &model.OperationLog{}))
	}
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "telegram.db")), telegramTestGormConfig)
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		migrate(t, db)
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		base := strings.TrimSpace(os.Getenv(telegramPostgresDSN))
		if base == "" {
			t.Skip(telegramPostgresDSN + " is not set")
		}
		admin, err := gorm.Open(postgres.Open(base), telegramTestGormConfig)
		require.NoError(t, err)
		adminDB, err := admin.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = adminDB.Close() })
		var databaseName string
		require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
		if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
			t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
		}
		suffix := make([]byte, 4)
		_, err = rand.Read(suffix)
		require.NoError(t, err)
		schema := "telegram_test_" + hex.EncodeToString(suffix)
		require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
		db, err := gorm.Open(postgres.Open(base+" search_path="+schema), telegramTestGormConfig)
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		migrate(t, db)
		body(t, db)
	})
}

// fakeTelegram is a Bot API stand-in: it answers every sendMessage with the
// configured status and body and records what it was asked.
type fakeTelegram struct {
	server *httptest.Server
	mu     sync.Mutex
	status int
	answer string
	paths  []string
	chats  []int64
	texts  []string
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	t.Helper()
	fake := &fakeTelegram{status: http.StatusOK, answer: `{"ok":true,"result":{"message_id":1}}`}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChatID int64  `json:"chat_id"`
			Text   string `json:"text"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		fake.mu.Lock()
		fake.paths = append(fake.paths, r.URL.Path)
		fake.chats = append(fake.chats, body.ChatID)
		fake.texts = append(fake.texts, body.Text)
		status, answer := fake.status, fake.answer
		fake.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeTelegram) reply(status int, answer string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.answer = status, answer
}

func (f *fakeTelegram) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.paths)
}

// telegramLogs captures slog output for the duration of a test.
func telegramLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buffer
}

// resetTelegramTables empties what a test case creates.
func resetTelegramTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []string{"v2_telegram_bot", "v2_telegram_user", "v2_operation_log"} {
		require.NoError(t, db.Exec("DELETE FROM "+table).Error)
	}
}

type telegramEnv struct {
	db      *gorm.DB
	fake    *fakeTelegram
	handler *KernelNotificationsHandler
	router  *gin.Engine
	logs    *bytes.Buffer
	clock   *telegramClock
}

type telegramClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *telegramClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *telegramClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTelegramEnv(t *testing.T, db *gorm.DB) *telegramEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	resetTelegramTables(t, db)
	env := &telegramEnv{db: db, fake: newFakeTelegram(t), logs: telegramLogs(t), clock: &telegramClock{now: time.Unix(1_800_000_000, 0)}}
	env.handler = &KernelNotificationsHandler{
		db: func() *gorm.DB { return db },
		bots: func(db *gorm.DB) *service.TelegramBotService {
			bots := service.NewTelegramBotService(db)
			bots.SetAPIBase(env.fake.server.URL)
			return bots
		},
		limiter: newTelegramTestLimiter(env.clock.Now),
	}
	env.router = gin.New()
	env.router.Use(func(c *gin.Context) {
		user := uint(7)
		if header := c.GetHeader("X-Test-User"); header != "" {
			switch header {
			case "8":
				user = 8
			case "9":
				user = 9
			}
		}
		c.Set("user_id", user)
		c.Set("email", "admin@example.com")
		c.Set("is_admin", true)
	})
	env.router.POST("/telegram/test", env.handler.TelegramTest)
	return env
}

func (e *telegramEnv) bot(t *testing.T, token string, adminIDs string) {
	t.Helper()
	require.NoError(t, e.db.Create(&model.TelegramBot{Name: "bot", Token: token, AdminIDs: adminIDs}).Error)
}

func (e *telegramEnv) bind(t *testing.T, userID uint, telegramID int64) {
	t.Helper()
	require.NoError(t, e.db.Create(&model.TelegramUser{UserID: userID, TelegramID: telegramID}).Error)
}

type telegramTestResponse struct {
	Data struct {
		Class     string `json:"class"`
		OK        bool   `json:"ok"`
		Message   string `json:"message"`
		Target    string `json:"target"`
		ErrorCode int    `json:"error_code"`
		Reason    string `json:"reason"`
	} `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (e *telegramEnv) post(t *testing.T, body string) (*httptest.ResponseRecorder, telegramTestResponse) {
	t.Helper()
	return e.postAs(t, "", body, context.Background())
}

func (e *telegramEnv) postAs(t *testing.T, user, body string, ctx context.Context) (*httptest.ResponseRecorder, telegramTestResponse) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/telegram/test", bytes.NewBufferString(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	if user != "" {
		request.Header.Set("X-Test-User", user)
	}
	e.router.ServeHTTP(recorder, request)
	var parsed telegramTestResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &parsed)
	return recorder, parsed
}

// assertNoToken fails when the bot token appears anywhere an administrator,
// an auditor or an operator could read it.
func (e *telegramEnv) assertNoToken(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	secret := strings.Split(telegramTestToken, ":")[1]
	assert.NotContains(t, recorder.Body.String(), secret)
	assert.NotContains(t, recorder.Body.String(), telegramTestToken)
	assert.NotContains(t, e.logs.String(), secret)
	var rows []model.OperationLog
	require.NoError(t, e.db.Find(&rows).Error)
	for _, row := range rows {
		assert.NotContains(t, row.Content, secret)
		assert.NotContains(t, row.Username, secret)
	}
}

func (e *telegramEnv) auditRows(t *testing.T) []model.OperationLog {
	t.Helper()
	var rows []model.OperationLog
	require.NoError(t, e.db.Where("action = ?", "telegram_test").Order("id").Find(&rows).Error)
	return rows
}

func TestTelegramTestResultClasses(t *testing.T) {
	// Every description repeats the token: nothing of it may be answered.
	leaking := func(description string) string { return description + " " + telegramTestToken }
	cases := []struct {
		name      string
		status    int
		answer    string
		class     string
		ok        bool
		errorCode int
	}{
		{"ok", 200, `{"ok":true,"result":{"message_id":1}}`, "ok", true, 0},
		{"unauthorized", 401, `{"ok":false,"error_code":401,"description":"` + leaking("Unauthorized") + `"}`, "invalid_token", false, 401},
		{"not found path", 404, `{"ok":false,"error_code":404,"description":"` + leaking("Not Found") + `"}`, "invalid_token", false, 404},
		{"chat not found", 400, `{"ok":false,"error_code":400,"description":"` + leaking("Bad Request: chat not found") + `"}`, "chat_not_found", false, 400},
		{"user not found", 400, `{"ok":false,"error_code":400,"description":"Bad Request: user not found"}`, "chat_not_found", false, 400},
		{"blocked", 403, `{"ok":false,"error_code":403,"description":"` + leaking("Forbidden: bot was blocked by the user") + `"}`, "bot_blocked", false, 403},
		{"cannot initiate", 403, `{"ok":false,"error_code":403,"description":"Forbidden: bot can't initiate conversation with a user"}`, "bot_blocked", false, 403},
		{"rate limited", 429, `{"ok":false,"error_code":429,"description":"` + leaking("Too Many Requests: retry after 5") + `","parameters":{"retry_after":5}}`, "rate_limited", false, 429},
		{"other 400", 400, `{"ok":false,"error_code":400,"description":"` + leaking("Bad Request: message is too long") + `"}`, "unknown", false, 400},
		{"server error", 502, `{"ok":false,"error_code":502,"description":"Bad Gateway"}`, "unknown", false, 502},
		{"html answer", 200, `<html>captive portal</html>`, "unknown", false, 200},
		{"ok false on 200", 200, `{"ok":false,"description":"` + leaking("odd") + `"}`, "unknown", false, 200},
		{"status without json", 401, `nope`, "invalid_token", false, 401},
	}
	forEachTelegramDatabase(t, func(t *testing.T, db *gorm.DB) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				env := newTelegramEnv(t, db)
				env.bot(t, telegramTestToken, "")
				env.bind(t, 7, 4242)
				env.fake.reply(tc.status, tc.answer)

				recorder, response := env.post(t, `{}`)
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
				assert.Equal(t, tc.class, response.Data.Class)
				assert.Equal(t, tc.ok, response.Data.OK)
				assert.Equal(t, "self", response.Data.Target)
				assert.NotEmpty(t, response.Data.Message)
				assert.Equal(t, service.TelegramProbeResult{Class: service.TelegramProbeClass(tc.class)}.Message(), response.Data.Message,
					"the message is the fixed sentence of the class, not Telegram's text")
				assert.Equal(t, tc.errorCode, response.Data.ErrorCode)
				assert.NotContains(t, recorder.Body.String(), "description")
				assert.NotContains(t, recorder.Body.String(), "captive portal")
				// Telegram got the path with the token, the bound chat and the fixed text.
				require.Equal(t, 1, env.fake.calls())
				assert.Equal(t, "/bot"+telegramTestToken+"/sendMessage", env.fake.paths[0])
				assert.EqualValues(t, 4242, env.fake.chats[0])
				assert.Contains(t, env.fake.texts[0], "test message")
				env.assertNoToken(t, recorder)

				rows := env.auditRows(t)
				require.Len(t, rows, 1)
				assert.Equal(t, "notification", rows[0].Module)
				assert.Equal(t, "admin@example.com", rows[0].Username)
				require.NotNil(t, rows[0].UserID)
				assert.EqualValues(t, 7, *rows[0].UserID)
				wantStatus := 2
				if tc.ok {
					wantStatus = 1
				}
				assert.Equal(t, wantStatus, rows[0].Status)
				var content map[string]any
				require.NoError(t, json.Unmarshal([]byte(rows[0].Content), &content))
				assert.Equal(t, map[string]any{"class": tc.class, "target": "self"}, content)
				assert.NotContains(t, rows[0].Content, "4242")
			})
		}
	})
}

func TestTelegramTestNotConfigured(t *testing.T) {
	forEachTelegramDatabase(t, func(t *testing.T, db *gorm.DB) {
		t.Run("no bot row", func(t *testing.T) {
			env := newTelegramEnv(t, db)
			recorder, response := env.post(t, ``)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			assert.Equal(t, "not_configured", response.Data.Class)
			assert.False(t, response.Data.OK)
			assert.Zero(t, env.fake.calls())
			rows := env.auditRows(t)
			require.Len(t, rows, 1)
			assert.JSONEq(t, `{"class":"not_configured","target":"none"}`, rows[0].Content)
		})
		t.Run("empty token", func(t *testing.T) {
			env := newTelegramEnv(t, db)
			env.bot(t, "  ", "")
			recorder, response := env.post(t, `{}`)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "not_configured", response.Data.Class)
			assert.Zero(t, env.fake.calls())
		})
		t.Run("disabled flag does not stop delivery, so it does not stop the test", func(t *testing.T) {
			// Bot.Enabled is display-only: no delivery path reads it and no
			// route sets it, so the test follows what delivery does.
			env := newTelegramEnv(t, db)
			require.NoError(t, db.Create(&model.TelegramBot{Name: "bot", Token: telegramTestToken, Enabled: false}).Error)
			env.bind(t, 7, 4242)
			recorder, response := env.post(t, `{}`)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "ok", response.Data.Class)
		})
	})
}

func TestTelegramTestMalformedTokenIsNeverSent(t *testing.T) {
	forEachTelegramDatabase(t, func(t *testing.T, db *gorm.DB) {
		for _, token := range []string{"not-a-token", "123456789:abc", "123456789:ok/../../deleteWebhook-0123456789", "123:SECRET value with space 0123456789", "123456789:SECRET?x=1&y=2-0123456789"} {
			env := newTelegramEnv(t, db)
			env.bot(t, token, "")
			env.bind(t, 7, 4242)
			recorder, response := env.post(t, `{}`)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "invalid_token", response.Data.Class, token)
			assert.Zero(t, env.fake.calls(), "a malformed token must never reach a URL: %q", token)
			assert.NotContains(t, recorder.Body.String(), "SECRET")
			assert.NotContains(t, env.logs.String(), "SECRET")
		}
	})
}

func TestTelegramTestNetworkErrorsAreScrubbed(t *testing.T) {
	secret := strings.Split(telegramTestToken, ":")[1]
	forEachTelegramDatabase(t, func(t *testing.T, db *gorm.DB) {
		t.Run("connection refused", func(t *testing.T) {
			env := newTelegramEnv(t, db)
			env.bot(t, telegramTestToken, "")
			env.bind(t, 7, 4242)
			// The server is gone: the error of net/http would quote the URL.
			env.fake.server.Close()
			recorder, response := env.post(t, `{}`)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			assert.Equal(t, "network_error", response.Data.Class)
			assert.False(t, response.Data.OK)
			assert.Equal(t, "connect", response.Data.Reason)
			assert.NotContains(t, recorder.Body.String(), "127.0.0.1")
			assert.NotContains(t, recorder.Body.String(), secret)
			env.assertNoToken(t, recorder)
			rows := env.auditRows(t)
			require.Len(t, rows, 1)
			assert.JSONEq(t, `{"class":"network_error","target":"self"}`, rows[0].Content)
		})
		t.Run("timeout", func(t *testing.T) {
			env := newTelegramEnv(t, db)
			env.bot(t, telegramTestToken, "")
			env.bind(t, 7, 4242)
			slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
				case <-time.After(5 * time.Second):
				}
			}))
			t.Cleanup(slow.Close)
			env.handler.bots = func(db *gorm.DB) *service.TelegramBotService {
				bots := service.NewTelegramBotService(db)
				bots.SetAPIBase(slow.URL)
				return bots
			}
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			recorder, response := env.postAs(t, "", `{}`, ctx)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			assert.Equal(t, "network_error", response.Data.Class)
			assert.Equal(t, "timeout", response.Data.Reason)
			env.assertNoToken(t, recorder)
		})
		t.Run("a redirect is not followed", func(t *testing.T) {
			env := newTelegramEnv(t, db)
			env.bot(t, telegramTestToken, "")
			env.bind(t, 7, 4242)
			target := newFakeTelegram(t)
			redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.server.URL+r.URL.Path, http.StatusTemporaryRedirect)
			}))
			t.Cleanup(redirector.Close)
			env.handler.bots = func(db *gorm.DB) *service.TelegramBotService {
				bots := service.NewTelegramBotService(db)
				bots.SetAPIBase(redirector.URL)
				return bots
			}
			recorder, response := env.post(t, `{}`)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "unknown", response.Data.Class)
			assert.Zero(t, target.calls(), "the token must not follow a redirect")
		})
	})
}

func TestTelegramTestChatRestriction(t *testing.T) {
	forEachTelegramDatabase(t, func(t *testing.T, db *gorm.DB) {
		t.Run("own chat by default and by id", func(t *testing.T) {
			env := newTelegramEnv(t, db)
			env.bot(t, telegramTestToken, "")
			env.bind(t, 7, 4242)
			_, response := env.post(t, `{}`)
			assert.Equal(t, "self", response.Data.Target)
			_, response = env.post(t, `{"chat_id":4242}`)
			assert.Equal(t, "ok", response.Data.Class)
			assert.Equal(t, "self", response.Data.Target)
			assert.Equal(t, []int64{4242, 4242}, env.fake.chats)
		})
		t.Run("a bot admin id, JSON and comma separated", func(t *testing.T) {
			for _, ids := range []string{`[11,22]`, `11, 22`} {
				env := newTelegramEnv(t, db)
				env.bot(t, telegramTestToken, ids)
				env.bind(t, 7, 4242)
				recorder, response := env.post(t, `{"chat_id":22}`)
				require.Equal(t, http.StatusOK, recorder.Code, ids)
				assert.Equal(t, "ok", response.Data.Class)
				assert.Equal(t, "bot_admin", response.Data.Target)
				assert.Equal(t, []int64{22}, env.fake.chats)
				rows := env.auditRows(t)
				require.Len(t, rows, 1)
				assert.JSONEq(t, `{"class":"ok","target":"bot_admin"}`, rows[0].Content)
				assert.NotContains(t, rows[0].Content, "22\"")
			}
		})
		t.Run("any other chat is refused and nothing is sent", func(t *testing.T) {
			env := newTelegramEnv(t, db)
			env.bot(t, telegramTestToken, `[11]`)
			env.bind(t, 7, 4242)
			env.bind(t, 8, 5555) // another administrator's chat
			for _, body := range []string{`{"chat_id":5555}`, `{"chat_id":-100123}`, `{"chat_id":99}`} {
				recorder, response := env.post(t, body)
				assert.Equal(t, http.StatusForbidden, recorder.Code, body)
				assert.Equal(t, "chat_not_allowed", response.Error.Code, body)
			}
			assert.Zero(t, env.fake.calls())
			rows := env.auditRows(t)
			require.Len(t, rows, 3)
			for _, row := range rows {
				assert.Equal(t, 2, row.Status)
				assert.JSONEq(t, `{"class":"chat_not_allowed","target":"none"}`, row.Content)
			}
		})
		t.Run("no bound account and no chat id", func(t *testing.T) {
			env := newTelegramEnv(t, db)
			env.bot(t, telegramTestToken, `[11]`)
			recorder, response := env.post(t, `{}`)
			assert.Equal(t, http.StatusConflict, recorder.Code)
			assert.Equal(t, "telegram_not_bound", response.Error.Code)
			assert.Zero(t, env.fake.calls())
			// An explicit bot admin chat still works without a binding.
			_, response = env.post(t, `{"chat_id":11}`)
			assert.Equal(t, "bot_admin", response.Data.Target)
			assert.Equal(t, 1, env.fake.calls())
		})
		t.Run("a banned binding is not a target", func(t *testing.T) {
			env := newTelegramEnv(t, db)
			env.bot(t, telegramTestToken, "")
			require.NoError(t, db.Create(&model.TelegramUser{UserID: 7, TelegramID: 4242, IsBanned: true}).Error)
			recorder, response := env.post(t, `{}`)
			assert.Equal(t, http.StatusConflict, recorder.Code)
			assert.Equal(t, "telegram_not_bound", response.Error.Code)
			assert.Zero(t, env.fake.calls())
		})
		t.Run("bad bodies", func(t *testing.T) {
			env := newTelegramEnv(t, db)
			env.bot(t, telegramTestToken, "")
			env.bind(t, 7, 4242)
			for _, body := range []string{`{"chat_id":0}`, `{"chat_id":"abc"}`, `{"chat_id":1.5}`, `[1]`, `not json`} {
				recorder, response := env.post(t, body)
				assert.Equal(t, http.StatusBadRequest, recorder.Code, body)
				assert.Equal(t, "invalid_request", response.Error.Code, body)
			}
			assert.Zero(t, env.fake.calls())
		})
	})
}

func TestTelegramTestRateLimit(t *testing.T) {
	forEachTelegramDatabase(t, func(t *testing.T, db *gorm.DB) {
		env := newTelegramEnv(t, db)
		env.bot(t, telegramTestToken, `[11]`)
		for user, chat := range map[uint]int64{7: 4242, 8: 4343, 9: 4444} {
			env.bind(t, user, chat)
		}

		// Five a minute for one administrator, the sixth is refused with Retry-After.
		for i := 0; i < 5; i++ {
			recorder, _ := env.postAs(t, "", `{}`, context.Background())
			require.Equal(t, http.StatusOK, recorder.Code)
		}
		recorder, response := env.postAs(t, "", `{}`, context.Background())
		require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		assert.Equal(t, "rate_limited", response.Error.Code)
		assert.Equal(t, "60", recorder.Header().Get("Retry-After"))
		assert.Equal(t, 5, env.fake.calls(), "a refused test does not reach Telegram")
		assert.Len(t, env.auditRows(t), 5, "only attempts that reach the Bot API are audited; a refusal is logged")
		assert.Contains(t, env.logs.String(), "rate limit")

		// Another administrator has their own allowance.
		recorder, _ = env.postAs(t, "8", `{}`, context.Background())
		assert.Equal(t, http.StatusOK, recorder.Code)

		// The allowance comes back as the window slides.
		env.clock.advance(30 * time.Second)
		recorder, _ = env.postAs(t, "", `{}`, context.Background())
		assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
		assert.Equal(t, "30", recorder.Header().Get("Retry-After"))
		env.clock.advance(31 * time.Second)
		recorder, _ = env.postAs(t, "", `{}`, context.Background())
		assert.Equal(t, http.StatusOK, recorder.Code)
	})
}

func TestTelegramTestGlobalRateLimit(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limiter := newTelegramTestLimiter(func() time.Time { return now })
	// Twenty administrators one test each in ten minutes: the 21st is refused.
	for user := uint(1); user <= 20; user++ {
		allowed, _ := limiter.allow(user)
		require.True(t, allowed, user)
		now = now.Add(time.Second)
	}
	allowed, wait := limiter.allow(21)
	assert.False(t, allowed)
	assert.Equal(t, 10*time.Minute-20*time.Second, wait)
	now = now.Add(wait)
	allowed, _ = limiter.allow(21)
	assert.True(t, allowed)
}

func TestTelegramTestNeedsAnAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &KernelNotificationsHandler{limiter: newTelegramTestLimiter(time.Now)}
	router := gin.New()
	router.POST("/telegram/test", handler.TelegramTest)
	recorder := serveModuleRequest(router, http.MethodPost, "/telegram/test", `{}`)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
}
