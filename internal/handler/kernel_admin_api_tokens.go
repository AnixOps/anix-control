package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/adminapitoken"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// stepUpFreshness is how recent the sign-in that creates a token must be
// (see AdminAPITokensHandler.stepUp).
const stepUpFreshness = 10 * time.Minute

// adminAPITokenStepUpStaleCode is the refusal code when the caller's sign-in
// is older than stepUpFreshness.
const adminAPITokenStepUpStaleCode = "step_up_sign_in_stale" // #nosec G101 -- a response error code, not a credential

// AdminAPITokensHandler manages administrators' personal access tokens
// (docs/reference/admin-api-tokens.md). Its routes need a signed-in session:
// the authentication middleware refuses every API token on them, so a stolen
// token cannot mint, list or end tokens.
type AdminAPITokensHandler struct {
	db  func() *gorm.DB
	now func() time.Time
}

// NewAdminAPITokensHandler returns the handler over the process database.
func NewAdminAPITokensHandler() *AdminAPITokensHandler {
	return &AdminAPITokensHandler{db: database.Get, now: time.Now}
}

type createAdminAPITokenRequest struct {
	// Name says what the token is for; 1 to 100 printable characters.
	Name string `json:"name" binding:"required"`
	// Scope is "read" or "admin".
	Scope string `json:"scope" binding:"required"`
	// ExpiresInDays ends the token after that many days (at most 730); 0 or
	// absent never.
	ExpiresInDays int `json:"expires_in_days" binding:"omitempty,min=1,max=730"`
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
// @Description Issues a personal access token for automation: `anixadm_` and 43 characters, shown once in this answer; only its SHA-256 is stored. Scope `read` allows GET and HEAD on the administrator APIs except the reads that answer a secret in clear; `admin` allows what the owner may do except managing tokens. Needs a signed-in session (never an API token) at most 10 minutes old, or it answers 403 step_up_sign_in_stale. Up to 25 active tokens per administrator; the owner's rights are read on every use. Audited. Send the token as `Authorization: Bearer <token>` only, never in a URL.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body createAdminAPITokenRequest true "name, scope (read or admin), optional expires_in_days"
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
	if !h.stepUp(c, tokens, actorID) {
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

// stepUp requires a sign-in at most stepUpFreshness old before a token is
// created: identity's login (with its MFA policy) has just checked the
// administrator's credentials, which the kernel does not hold. A refusal is
// audited.
func (h *AdminAPITokensHandler) stepUp(c *gin.Context, tokens *adminapitoken.Service, userID uint) bool {
	issued, _ := c.Get("token_issued_at")
	if at, isTime := issued.(time.Time); isTime && h.now().Sub(at) <= stepUpFreshness {
		return true
	}
	tokens.RecordCreateDenied(c.Request.Context(), adminapitoken.Event{
		UserID: userID, Actor: c.GetString("email"), IP: c.ClientIP(), Method: c.Request.Method, Path: c.Request.URL.Path,
		Reason: adminAPITokenStepUpStaleCode,
	})
	kernelError(c, http.StatusForbidden, adminAPITokenStepUpStaleCode, "sign in again and retry within 10 minutes")
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
