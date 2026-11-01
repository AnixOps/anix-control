package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TelegramBindingHandler binds members to Telegram accounts outside the bot:
// an administrator binds by hand, and a member opens a one-time bind link
// from the panel (service.TelegramBindLink) or unbinds.
type TelegramBindingHandler struct {
	db   func() *gorm.DB
	bots func(*gorm.DB) *service.TelegramBotService
	now  func() time.Time
}

// NewTelegramBindingHandler builds the handler over the process database.
func NewTelegramBindingHandler() *TelegramBindingHandler {
	return &TelegramBindingHandler{db: database.Get, bots: service.NewTelegramBotService, now: time.Now}
}

func (h *TelegramBindingHandler) service(c *gin.Context) (*service.TelegramBotService, *gorm.DB, bool) {
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return nil, nil, false
	}
	db = db.WithContext(c.Request.Context())
	return h.bots(db), db, true
}

// telegramBindingError answers a binding error with its status and code;
// the message is the service's, meant for people.
func telegramBindingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrTelegramBotNotConfigured):
		kernelError(c, http.StatusConflict, "bot_not_configured", err.Error())
	case errors.Is(err, service.ErrTelegramBindDisabled):
		kernelError(c, http.StatusForbidden, "bind_disabled", err.Error())
	case errors.Is(err, service.ErrTelegramUserNotFound):
		kernelError(c, http.StatusNotFound, "user_not_found", err.Error())
	case errors.Is(err, service.ErrTelegramUserAlreadyBound):
		kernelError(c, http.StatusConflict, "user_already_bound", err.Error())
	case errors.Is(err, service.ErrTelegramIDAlreadyBound):
		kernelError(c, http.StatusConflict, "telegram_already_bound", err.Error())
	case errors.Is(err, service.ErrTelegramNotBound):
		kernelError(c, http.StatusNotFound, "not_bound", err.Error())
	default:
		kernelDBError(c, err)
	}
}

// AdminBindRequest binds a member, named by user_id or email, to a Telegram
// user ID (a number, or its decimal string).
type AdminBindRequest struct {
	UserID     uint   `json:"user_id"`
	Email      string `json:"email"`
	TelegramID any    `json:"telegram_id"`
}

// AdminBind godoc
// @Summary 手动绑定 Telegram
// @Description 管理员把用户 (user_id 或 email) 绑定到一个 Telegram 用户 ID。不受机器人「允许绑定」开关限制。
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body AdminBindRequest true "绑定"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Router /api/v4/admin/telegram/bindings [post]
func (h *TelegramBindingHandler) AdminBind(c *gin.Context) {
	var req AdminBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "body must be JSON")
		return
	}
	telegramID, err := parseTelegramID(req.TelegramID)
	if err != nil || telegramID <= 0 {
		kernelError(c, http.StatusBadRequest, "invalid_telegram_id", "telegram_id must be a positive Telegram user ID")
		return
	}
	bots, db, ok := h.service(c)
	if !ok {
		return
	}
	userID := req.UserID
	if userID == 0 {
		email := strings.TrimSpace(req.Email)
		if email == "" {
			kernelError(c, http.StatusBadRequest, "invalid_request", "user_id or email is required")
			return
		}
		var user model.User
		if err := db.Select("id").Where("LOWER(email) = LOWER(?)", email).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				telegramBindingError(c, service.ErrTelegramUserNotFound)
				return
			}
			kernelDBError(c, err)
			return
		}
		userID = user.ID
	}
	binding, err := bots.AdminBind(userID, telegramID)
	if err != nil {
		telegramBindingError(c, err)
		return
	}
	var user model.User
	_ = db.Select("id", "email").First(&user, userID).Error
	kernelData(c, http.StatusCreated, gin.H{
		"id":          binding.ID,
		"user_id":     binding.UserID,
		"user_email":  user.Email,
		"telegram_id": binding.TelegramID,
		"created_at":  binding.CreatedAt,
	})
}

// AdminUnbind godoc
// @Summary 解除用户的 Telegram 绑定
// @Tags 管理端-通知
// @Produce json
// @Security BearerAuth
// @Param user_id path int true "用户 ID"
// @Success 200 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/v4/admin/telegram/bindings/{user_id} [delete]
func (h *TelegramBindingHandler) AdminUnbind(c *gin.Context) {
	userID, ok := parseKernelID(c, "user_id")
	if !ok {
		return
	}
	bots, _, ok := h.service(c)
	if !ok {
		return
	}
	if err := bots.UnbindUser(userID); err != nil {
		telegramBindingError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"user_id": userID, "bound": false})
}

// UserStatus godoc
// @Summary 我的 Telegram 绑定
// @Description 是否已绑定、绑定的 Telegram 账号，以及机器人是否可用于绑定。
// @Tags 用户-通知
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/user/telegram [get]
func (h *TelegramBindingHandler) UserStatus(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		kernelError(c, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	bots, db, ok := h.service(c)
	if !ok {
		return
	}
	status := gin.H{"bound": false, "bot_configured": false, "allow_bind": false}
	if bot, err := bots.GetBot(); err == nil && strings.TrimSpace(bot.Token) != "" {
		status["bot_configured"] = true
		status["allow_bind"] = bot.AllowBind
	}
	var binding model.TelegramUser
	err := db.Where("user_id = ?", userID).First(&binding).Error
	switch {
	case err == nil:
		status["bound"] = true
		status["telegram_id"] = binding.TelegramID
		status["username"] = binding.Username
		status["first_name"] = binding.FirstName
		status["bound_at"] = binding.CreatedAt
	case !errors.Is(err, gorm.ErrRecordNotFound):
		kernelDBError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, status)
}

// UserBindLink godoc
// @Summary 获取 Telegram 绑定链接
// @Description 一次性绑定链接 (t.me 深链接)，10 分钟内有效。打开后机器人收到 /start <绑定码> 并绑定发送者的 Telegram 账号。
// @Tags 用户-通知
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Router /api/v4/user/telegram/bind-link [post]
func (h *TelegramBindingHandler) UserBindLink(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		kernelError(c, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	bots, _, ok := h.service(c)
	if !ok {
		return
	}
	link, err := bots.BindLink(userID, h.now())
	if err != nil {
		telegramBindingError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, link)
}

// UserUnbind godoc
// @Summary 解除我的 Telegram 绑定
// @Tags 用户-通知
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/v4/user/telegram [delete]
func (h *TelegramBindingHandler) UserUnbind(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		kernelError(c, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	bots, _, ok := h.service(c)
	if !ok {
		return
	}
	if err := bots.UnbindUser(userID); err != nil {
		telegramBindingError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"bound": false})
}
