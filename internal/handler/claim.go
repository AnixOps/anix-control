package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/adminclaim"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/identityimport"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// claimBodyLimit bounds the claim's body: a token, an email and a password.
const claimBodyLimit = 4 << 10

// ClaimHandler serves the first administrator's claim link (adminclaim):
// the claim page's status and the claim itself, both public. Neither ever
// answers or logs the token.
type ClaimHandler struct {
	db       func() *gorm.DB
	identity func() adminclaim.IdentityAccounts
}

// NewClaimHandler claims through the process's database and, once identity
// holds the credentials, its identity importer.
func NewClaimHandler() *ClaimHandler {
	return &ClaimHandler{
		db: database.Get,
		identity: func() adminclaim.IdentityAccounts {
			runner, _ := identityimport.Default()
			return adminclaim.IdentityFromImporter(runner)
		},
	}
}

// claimRequest is the body of POST /api/v5/claim.
type claimRequest struct {
	// Token is the claim link's fragment, anixclaim_ and 43 characters.
	Token    string `json:"token"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// claimAnswer is the 201 of POST /api/v5/claim.
type claimAnswer struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	// Next is where to sign in: the claim creates the account and starts no
	// session, the page signs in with the same email and password.
	Next string `json:"next"`
}

// Status godoc
// @Summary Whether the panel can be claimed
// @Description Public. data.claimable is true while the panel has no administrator: the claim page then asks for the email and password, and the sign-in page says no administrator exists yet. It says nothing else (not whether a link was issued, nor its expiry).
// @Tags Kernel
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 429 {object} map[string]any
// @Router /api/v5/claim/status [get]
func (h *ClaimHandler) Status(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "claim_unavailable", "the database is not available")
		return
	}
	claimable, err := adminclaim.Claimable(c.Request.Context(), db)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"claimable": claimable})
}

// Claim godoc
// @Summary Create the first administrator with the claim link
// @Description Public, rate limited. The body is {token, email, password}: the token is the fragment of the link `anix-control admin claim-link` printed (https://<domain>/claim#anixclaim_...). 201 with data {user_id, email, next: "/login"}: the administrator exists, the token is used up and the page signs in with the same email and password. 404 not_found once the panel has an administrator (whatever the token); 403 claim_invalid when the token is not the pending link's (a newer link replaces an older one); 410 claim_expired; 400 invalid_request or invalid_email; 422 weak_password (fewer than 6 characters) or password_too_long (more than 72 bytes); 409 email_taken; 503 identity_unavailable when identity holds the credentials and cannot be reached (nothing changed). The body is never logged; the audit row holds admin_claimed, the email and the source address.
// @Tags Kernel
// @Accept json
// @Produce json
// @Param request body claimRequest true "token, email and password"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 410 {object} map[string]any
// @Failure 422 {object} map[string]any
// @Failure 429 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v5/claim [post]
func (h *ClaimHandler) Claim(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "claim_unavailable", "the database is not available")
		return
	}
	var request claimRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, claimBodyLimit))
	if err := decoder.Decode(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "send {\"token\", \"email\", \"password\"} as JSON")
		return
	}
	result, err := (&adminclaim.Claimer{DB: db, Identity: h.identity()}).Claim(c.Request.Context(), adminclaim.Request{
		Token: request.Token, Email: request.Email, Password: request.Password, SourceIP: c.ClientIP(),
	})
	switch {
	case err == nil:
		slog.Info("first administrator claimed", slog.String("email", result.Email), slog.String("ip", c.ClientIP()),
			slog.Bool("identity", result.Identity))
		kernelData(c, http.StatusCreated, claimAnswer{UserID: result.UserID, Email: result.Email, Next: "/login"})
	case errors.Is(err, adminclaim.ErrAdministratorExists):
		kernelError(c, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, adminclaim.ErrInvalidToken):
		kernelError(c, http.StatusForbidden, "claim_invalid", "this claim link is not valid: run anix-control admin claim-link for a new one")
	case errors.Is(err, adminclaim.ErrExpired):
		kernelError(c, http.StatusGone, "claim_expired", "this claim link has expired: run anix-control admin claim-link for a new one")
	case errors.Is(err, adminclaim.ErrInvalidEmail):
		kernelError(c, http.StatusBadRequest, "invalid_email", "enter an email address such as name@example.com")
	case errors.Is(err, adminclaim.ErrWeakPassword):
		kernelError(c, http.StatusUnprocessableEntity, "weak_password", err.Error())
	case errors.Is(err, adminclaim.ErrPasswordTooLong):
		kernelError(c, http.StatusUnprocessableEntity, "password_too_long", err.Error())
	case errors.Is(err, adminclaim.ErrEmailTaken):
		kernelError(c, http.StatusConflict, "email_taken", err.Error())
	case errors.Is(err, adminclaim.ErrIdentityUnavailable):
		slog.Warn("first administrator claim: identity is not reachable", slog.String("error", err.Error()))
		kernelError(c, http.StatusServiceUnavailable, "identity_unavailable", "the identity module is not reachable: try again shortly")
	default:
		slog.Error("first administrator claim failed", slog.String("error", err.Error()))
		kernelError(c, http.StatusInternalServerError, "claim_failed", "the claim failed")
	}
}
