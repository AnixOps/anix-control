package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/adminapitoken"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// stepUpFreshness is how recent a sign-in must be when the kernel cannot
// check a credential itself (see AdminAPITokensHandler.stepUp).
const stepUpFreshness = 10 * time.Minute

// adminAPITokenStepUpStaleCode is the refusal code when identity holds the
// credentials and the caller's sign-in is older than stepUpFreshness. It used
// to share step_up_required with "a password is required", which clients could
// tell apart only by the message text.
const adminAPITokenStepUpStaleCode = "step_up_sign_in_stale"

// adminAPITokenStepUpLimit bounds the re-authentication attempts of one
// administrator: the fifth failure in fifteen minutes locks creations out for
// fifteen minutes.
var adminAPITokenStepUpLimit = service.LoginRateLimitOptions{
	Enabled:      true,
	MaxAttempts:  5,
	Window:       15 * time.Minute,
	Lockout:      15 * time.Minute,
	CleanupAfter: 40 * time.Minute,
}

// AdminAPITokensHandler manages administrators' personal access tokens
// (docs/reference/admin-api-tokens.md). Its routes need a signed-in session:
// the authentication middleware refuses every API token on them, so a stolen
// token cannot mint, list or end tokens.
type AdminAPITokensHandler struct {
	db      func() *gorm.DB
	limiter *service.LoginRateLimiter
	now     func() time.Time
}

// NewAdminAPITokensHandler returns the handler over the process database.
func NewAdminAPITokensHandler() *AdminAPITokensHandler {
	return &AdminAPITokensHandler{db: database.Get, limiter: service.NewLoginRateLimiter(), now: time.Now}
}

type createAdminAPITokenRequest struct {
	// Name says what the token is for; 1 to 100 printable characters.
	Name string `json:"name" binding:"required"`
	// Scope is "read" or "admin".
	Scope string `json:"scope" binding:"required"`
	// ExpiresInDays ends the token after that many days (at most 730); 0 or
	// absent never.
	ExpiresInDays int `json:"expires_in_days" binding:"omitempty,min=1,max=730"`
	// Password re-authenticates the administrator; with a second factor on,
	// Code and Method (totp or backup) do instead.
	Password string `json:"password"`
	Code     string `json:"code"`
	Method   string `json:"method"`
}

type createdAdminAPIToken struct {
	// Token is returned once; the kernel stores only its SHA-256.
	Token    string              `json:"token"`
	APIToken model.AdminAPIToken `json:"api_token"`
}

func (h *AdminAPITokensHandler) requireSession(c *gin.Context) bool {
	if c.GetString(adminapitoken.ContextKeyAuthMethod) == adminapitoken.AuthMethodAPIToken {
		kernelError(c, http.StatusForbidden, "session_required", adminapitoken.ErrInteractiveOnly.Error())
		return false
	}
	return true
}

func (h *AdminAPITokensHandler) actor(c *gin.Context) (*gorm.DB, uint, bool) {
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "the database is unavailable")
		return nil, 0, false
	}
	id := kernelActorID(c)
	if id == 0 {
		kernelError(c, http.StatusUnauthorized, "not_authenticated", "a signed-in administrator is required")
		return nil, 0, false
	}
	return db, id, true
}

// Create godoc
// @Summary Create an admin API token
// @Description Issues a personal access token for automation: `anixadm_` and 43 characters, shown once in this answer; only its SHA-256 is stored. Scope `read` allows GET and HEAD on the administrator APIs except the reads that answer a secret in clear; `admin` allows what the owner may do except managing tokens. Needs a signed-in session (never an API token) and a re-authentication: the current password, or a TOTP or recovery code when the account has a second factor (when the kernel cannot check a credential because identity holds it, the sign-in must be at most 10 minutes old, or it answers 403 step_up_sign_in_stale). Up to 25 active tokens per administrator; the owner's rights are read on every use. Audited. Send the token as `Authorization: Bearer <token>` only, never in a URL.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body createAdminAPITokenRequest true "name, scope (read or admin), optional expires_in_days, and password or code (+ method)"
// @Success 201 {object} map[string]any "data.token is shown once; data.api_token is the stored record"
// @Router /api/v4/kernel/api-tokens [post]
func (h *AdminAPITokensHandler) Create(c *gin.Context) {
	if !h.requireSession(c) {
		return
	}
	db, actorID, ok := h.actor(c)
	if !ok {
		return
	}
	var request createAdminAPITokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "name and scope are required; expires_in_days is 1 to 730")
		return
	}
	tokens := adminapitoken.New(db)
	if !h.stepUp(c, db, tokens, actorID, request) {
		return
	}
	input := adminapitoken.CreateInput{
		UserID: actorID, Name: request.Name, Scope: request.Scope, Actor: c.GetString("email"), IP: c.ClientIP(),
	}
	if request.ExpiresInDays > 0 {
		expires := h.now().Add(time.Duration(request.ExpiresInDays) * 24 * time.Hour)
		input.ExpiresAt = &expires
	}
	secret, row, err := tokens.Create(c.Request.Context(), input)
	switch {
	case errors.Is(err, adminapitoken.ErrInvalidRequest):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	case errors.Is(err, adminapitoken.ErrTooMany):
		kernelError(c, http.StatusConflict, "too_many_tokens",
			fmt.Sprintf("an administrator may hold at most %d active API tokens: revoke one first", adminapitoken.MaxActivePerUser))
		return
	case errors.Is(err, adminapitoken.ErrOwnerNotAdmin):
		kernelError(c, http.StatusForbidden, "not_an_administrator", err.Error())
		return
	case err != nil:
		kernelDBError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusCreated, createdAdminAPIToken{Token: secret, APIToken: row})
}

// stepUp re-authenticates the administrator before a token is created, as
// the user's own subscription reset does (UserService.VerifyStepUp): with a
// second factor on, a TOTP or recovery code; otherwise the current password.
// When identity holds the credentials (the account's legacy password is the
// unusable marker), the kernel cannot check one: it then requires a sign-in
// at most stepUpFreshness old, which identity's login (with its MFA policy)
// has just checked. Attempts are rate limited per administrator and a
// refusal is audited.
func (h *AdminAPITokensHandler) stepUp(c *gin.Context, db *gorm.DB, tokens *adminapitoken.Service, userID uint, request createAdminAPITokenRequest) bool {
	key := "admin_api_token_step_up|" + strconv.FormatUint(uint64(userID), 10)
	event := adminapitoken.Event{
		UserID: userID, Actor: c.GetString("email"), IP: c.ClientIP(), Method: c.Request.Method, Path: c.Request.URL.Path,
	}
	refuse := func(status int, code, message string) bool {
		event.Reason = code
		tokens.RecordCreateDenied(c.Request.Context(), event)
		kernelError(c, status, code, message)
		return false
	}
	if blocked, wait := h.limiter.Check(key, adminAPITokenStepUpLimit); blocked {
		c.Header("Retry-After", strconv.Itoa(max(int(wait.Seconds()), 1)))
		return refuse(http.StatusTooManyRequests, "step_up_rate_limited", "too many failed re-authentications, try again later")
	}
	var owner model.User
	if err := db.Select("id", "password").Where("id = ?", userID).Take(&owner).Error; err != nil {
		return refuse(http.StatusForbidden, "not_an_administrator", adminapitoken.ErrOwnerNotAdmin.Error())
	}
	if owner.Password == model.UnusableLegacyPassword {
		issued, _ := c.Get("token_issued_at")
		at, isTime := issued.(time.Time)
		if !isTime || h.now().Sub(at) > stepUpFreshness {
			return refuse(http.StatusForbidden, adminAPITokenStepUpStaleCode, "sign in again and retry within 10 minutes")
		}
		return true
	}
	checked, err := service.NewUserServiceFor(db).VerifyStepUp(userID, request.Password, request.Code, request.Method)
	if checked && err != nil {
		h.limiter.RecordFailure(key, adminAPITokenStepUpLimit)
	}
	switch {
	case err == nil:
		h.limiter.RecordSuccess(key)
		return true
	case errors.Is(err, service.ErrStepUpPasswordRequired):
		return refuse(http.StatusForbidden, "step_up_required", "password is required")
	case errors.Is(err, service.ErrStepUpMFARequired):
		return refuse(http.StatusForbidden, "step_up_required", "an MFA code is required")
	case errors.Is(err, service.ErrStepUpInvalidPassword), errors.Is(err, service.ErrStepUpInvalidMFACode):
		return refuse(http.StatusForbidden, "step_up_failed", "the password or code is not valid")
	}
	kernelError(c, http.StatusInternalServerError, "step_up_unavailable", "the re-authentication could not be checked")
	return false
}

// List godoc
// @Summary List admin API tokens
// @Description Lists the caller's active API tokens (add include_inactive=true for revoked and expired ones): id, name, scope, last four characters, expiry, last use. Never a secret. A super administrator may list another administrator's tokens with user_id, or everyone's with all=true. Needs a signed-in session.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param user_id query int false "another administrator (super administrators only)"
// @Param all query bool false "every administrator's tokens (super administrators only)"
// @Param include_inactive query bool false "also revoked and expired tokens"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/api-tokens [get]
func (h *AdminAPITokensHandler) List(c *gin.Context) {
	if !h.requireSession(c) {
		return
	}
	db, actorID, ok := h.actor(c)
	if !ok {
		return
	}
	filter := adminapitoken.ListFilter{UserID: &actorID, IncludeInactive: c.Query("include_inactive") == "true"}
	foreign := false
	if raw := c.Query("user_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 {
			kernelError(c, http.StatusBadRequest, "invalid_request", "user_id must be a positive integer")
			return
		}
		if uint(id) != actorID {
			owner := uint(id)
			filter.UserID, foreign = &owner, true
		}
	}
	if c.Query("all") == "true" {
		filter.UserID, foreign = nil, true
	}
	if foreign {
		super, err := service.IsSuperAdmin(db, actorID)
		if err != nil {
			kernelDBError(c, err)
			return
		}
		if !super {
			kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may list other administrators' API tokens")
			return
		}
	}
	rows, err := adminapitoken.New(db).List(c.Request.Context(), filter)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, rows)
}

// Revoke godoc
// @Summary Revoke an admin API token
// @Description Ends a token at once. An administrator revokes their own tokens; a super administrator revokes anyone's (for example a departed administrator's). Revoking a revoked token is not an error. Needs a signed-in session. Audited.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "token id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/api-tokens/{id} [delete]
func (h *AdminAPITokensHandler) Revoke(c *gin.Context) {
	if !h.requireSession(c) {
		return
	}
	db, actorID, ok := h.actor(c)
	if !ok {
		return
	}
	super, err := service.IsSuperAdmin(db, actorID)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	row, changed, err := adminapitoken.New(db).Revoke(c.Request.Context(), c.Param("id"), adminapitoken.Actor{
		UserID: actorID, Name: c.GetString("email"), IP: c.ClientIP(), Super: super,
	})
	switch {
	case errors.Is(err, adminapitoken.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", "the API token does not exist or is not yours")
		return
	case err != nil:
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"api_token": row, "changed": changed})
}
