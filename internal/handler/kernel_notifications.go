package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/branding"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// The administrator's Telegram test: one fixed message through the
// configured bot, answered as a result class (service.TelegramProbeClass).
// The route sits behind admin authentication; the audit middleware does not
// cover /api/v4/kernel, so every attempt writes its own operation log row.
const (
	auditModuleNotification   = "notification"
	auditActionTelegramTest   = "telegram_test"
	auditTargetTelegramBot    = "telegram_bot"
	telegramTestTargetSelf    = "self"
	telegramTestTargetBotAdm  = "bot_admin"
	telegramTestTargetNone    = "none"
	telegramTestPerAdminLimit = 5
	telegramTestPerAdminSpan  = time.Minute
	telegramTestGlobalLimit   = 20
	telegramTestGlobalSpan    = 10 * time.Minute
)

// KernelNotificationsHandler serves the kernel's notification checks.
type KernelNotificationsHandler struct {
	db      func() *gorm.DB
	bots    func(*gorm.DB) *service.TelegramBotService
	limiter *telegramTestLimiter
}

// NewKernelNotificationsHandler builds the handler over the process database.
func NewKernelNotificationsHandler() *KernelNotificationsHandler {
	return &KernelNotificationsHandler{
		db:      database.Get,
		bots:    service.NewTelegramBotService,
		limiter: newTelegramTestLimiter(time.Now),
	}
}

// telegramTestLimiter allows each administrator a few tests a minute and
// all administrators a few more in ten minutes. Every attempt that reaches
// the Bot API counts, whatever Telegram answered.
type telegramTestLimiter struct {
	mu      sync.Mutex
	now     func() time.Time
	perUser map[uint][]time.Time
	global  []time.Time
}

func newTelegramTestLimiter(now func() time.Time) *telegramTestLimiter {
	return &telegramTestLimiter{now: now, perUser: map[uint][]time.Time{}}
}

// allow records an attempt of user, or reports how long to wait.
func (l *telegramTestLimiter) allow(user uint) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.global = pruneAttempts(l.global, now, telegramTestGlobalSpan)
	for id, attempts := range l.perUser {
		if pruned := pruneAttempts(attempts, now, telegramTestPerAdminSpan); len(pruned) == 0 {
			delete(l.perUser, id)
		} else {
			l.perUser[id] = pruned
		}
	}
	perUser := l.perUser[user]
	if len(perUser) >= telegramTestPerAdminLimit {
		return false, perUser[0].Add(telegramTestPerAdminSpan).Sub(now)
	}
	if len(l.global) >= telegramTestGlobalLimit {
		return false, l.global[0].Add(telegramTestGlobalSpan).Sub(now)
	}
	l.perUser[user] = append(perUser, now)
	l.global = append(l.global, now)
	return true, 0
}

// pruneAttempts drops the attempts older than span.
func pruneAttempts(attempts []time.Time, now time.Time, span time.Duration) []time.Time {
	cut := 0
	for cut < len(attempts) && !attempts[cut].Add(span).After(now) {
		cut++
	}
	return attempts[cut:]
}

type telegramTestRequest struct {
	// ChatID is the chat to send to: the administrator's own bound chat
	// (the default) or one of the bot's admin_ids. Anything else is refused.
	ChatID *int64 `json:"chat_id"`
}

type telegramTestAnswer struct {
	service.TelegramProbeResult
	// OK is true when Telegram accepted the message.
	OK bool `json:"ok"`
	// Message is a fixed sentence for the class.
	Message string `json:"message"`
	// Target is "self" (the administrator's bound chat) or "bot_admin".
	Target string `json:"target,omitempty"`
}

// TelegramTest godoc
// @Summary Send a test message through the Telegram bot
// @Description Sends one fixed test message through the bot configured in the Telegram bot settings (the v2_telegram_bot row) and answers the result class: ok, invalid_token, chat_not_found, bot_blocked, rate_limited, not_configured, network_error (with a reason: timeout, dns, tls, connect, canceled, other) or unknown. The answer never carries the bot token, a URL or Telegram's own text. The message goes to the calling administrator's own bound Telegram chat; an explicit chat_id is accepted only when it is that chat or one of the bot's admin_ids. An administrator without a bound Telegram account gets 409. At most 5 tests a minute per administrator and 20 in ten minutes for all of them (429 with Retry-After). Every attempt is recorded in the operation log (module notification, action telegram_test) with its class only.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body telegramTestRequest false "chat_id (optional)"
// @Success 200 {object} map[string]any "data.class, data.ok, data.message, data.target, data.error_code, data.reason"
// @Failure 400 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 429 {object} map[string]any
// @Router /api/v4/kernel/notifications/telegram/test [post]
func (h *KernelNotificationsHandler) TelegramTest(c *gin.Context) {
	actor := kernelActorID(c)
	if actor == 0 {
		kernelError(c, http.StatusForbidden, "admin_required", "an administrator is required")
		return
	}
	c.Header("Cache-Control", "no-store")
	var request telegramTestRequest
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		kernelError(c, http.StatusBadRequest, "invalid_request", "the body must be a JSON object with an optional integer chat_id")
		return
	}
	if request.ChatID != nil && *request.ChatID == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", "chat_id must not be 0")
		return
	}
	if allowed, wait := h.limiter.allow(actor); !allowed {
		seconds := max(int(math.Ceil(wait.Seconds())), 1)
		c.Header("Retry-After", strconv.Itoa(seconds))
		slog.Warn("telegram test refused: rate limit", slog.Uint64("user_id", uint64(actor)), slog.Int("retry_after_seconds", seconds))
		kernelError(c, http.StatusTooManyRequests, "rate_limited", "too many Telegram tests, try again later")
		return
	}
	db := h.db()
	bots := h.bots(db)
	bot, err := bots.GetBot()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		kernelDBError(c, err)
		return
	}
	if err != nil {
		bot = nil
	}
	if bot == nil || strings.TrimSpace(bot.Token) == "" {
		// Nothing to send with: say so before asking who to send to.
		result := service.TelegramProbeResult{Class: service.TelegramProbeNotConfigured}
		h.audit(c, db, string(result.Class), telegramTestTargetNone)
		kernelData(c, http.StatusOK, newTelegramTestAnswer(result, ""))
		return
	}
	chatID, target, refusal := h.resolveTarget(db, bot, actor, request.ChatID)
	if refusal != nil {
		h.audit(c, db, refusal.audit, telegramTestTargetNone)
		kernelError(c, refusal.status, refusal.code, refusal.message)
		return
	}
	result := bots.ProbeSend(c.Request.Context(), bot, chatID, branding.ControlName+" test message: if you can read this, the Telegram bot token and your chat are working.")
	attrs := []any{slog.Uint64("user_id", uint64(actor)), slog.String("class", string(result.Class)), slog.String("target", target)}
	if result.Reason != "" {
		attrs = append(attrs, slog.String("reason", result.Reason))
	}
	slog.Info("telegram test", attrs...)
	h.audit(c, db, string(result.Class), target)
	kernelData(c, http.StatusOK, newTelegramTestAnswer(result, target))
}

func newTelegramTestAnswer(result service.TelegramProbeResult, target string) telegramTestAnswer {
	return telegramTestAnswer{TelegramProbeResult: result, OK: result.OK(), Message: result.Message(), Target: target}
}

// telegramTestRefusal is a request that names no chat the test may use.
type telegramTestRefusal struct {
	status  int
	code    string
	message string
	// audit is the audit class of the refusal.
	audit string
}

// resolveTarget picks the chat the test goes to. With no chat_id it is the
// administrator's own bound chat; an explicit chat_id must be that chat or
// one of the bot's admin_ids, so the bot cannot be made to message anybody.
func (h *KernelNotificationsHandler) resolveTarget(db *gorm.DB, bot *model.TelegramBot, actor uint, requested *int64) (int64, string, *telegramTestRefusal) {
	var own int64
	if binding, err := service.NewTelegramUserService(db).GetByUserID(actor); err == nil && !binding.IsBanned {
		own = binding.TelegramID
	}
	if requested == nil {
		if own == 0 {
			return 0, telegramTestTargetNone, &telegramTestRefusal{
				status: http.StatusConflict, code: "telegram_not_bound", audit: "no_target",
				message: "this administrator has no bound Telegram account: bind it first, or pass a chat_id from the bot's admin_ids",
			}
		}
		return own, telegramTestTargetSelf, nil
	}
	if own != 0 && *requested == own {
		return own, telegramTestTargetSelf, nil
	}
	if bot != nil {
		for _, id := range parseTelegramBotAdminIDs(bot.AdminIDs) {
			if id == *requested {
				return id, telegramTestTargetBotAdm, nil
			}
		}
	}
	return 0, telegramTestTargetNone, &telegramTestRefusal{
		status: http.StatusForbidden, code: "chat_not_allowed", audit: "chat_not_allowed",
		message: "chat_id must be this administrator's bound Telegram chat or one of the bot's admin_ids",
	}
}

// audit records a test attempt in the operation log: its class and the kind
// of chat, never the token or a chat id.
func (h *KernelNotificationsHandler) audit(c *gin.Context, db *gorm.DB, class, target string) {
	actor := kernelActorID(c)
	content, err := json.Marshal(map[string]any{"class": class, "target": target})
	if err != nil {
		return
	}
	username := strings.TrimSpace(c.GetString("email"))
	if username == "" {
		username = "user:" + strconv.FormatUint(uint64(actor), 10)
	}
	status := 2
	if class == string(service.TelegramProbeOK) {
		status = 1
	}
	userID := actor
	entry := model.OperationLog{
		UserID: &userID, Username: truncateAudit(username, 100), Action: auditActionTelegramTest, Module: auditModuleNotification,
		TargetType: auditTargetTelegramBot, Content: string(content), IP: truncateAudit(c.ClientIP(), 45), Status: status,
	}
	if err := db.Create(&entry).Error; err != nil {
		slog.Error("telegram test audit failed", slog.Uint64("user_id", uint64(actor)), slog.String("class", class))
	}
}

func truncateAudit(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
