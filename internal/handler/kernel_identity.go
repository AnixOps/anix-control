package handler

import (
	"errors"
	"net/http"

	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/AnixOps/anix-control/v4/internal/identitycutover"
	"github.com/AnixOps/anix-control/v4/internal/identityimport"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
)

// IdentityStatus reports the identity authority state and the account
// import progress.
func IdentityStatus(c *gin.Context) {
	runner, db := identityimport.Default()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "identity_unavailable", "the identity importer is not configured")
		return
	}
	state, checkpoint, err := identityimport.Status(c.Request.Context(), db)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	var events []model.IdentityCutoverEvent
	if err := db.WithContext(c.Request.Context()).Order("id DESC").Limit(10).Find(&events).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	cutover := identitycutover.Default()
	var latest *identitycutover.Operation
	if cutover != nil {
		latest = cutover.Latest()
	}
	kernelData(c, http.StatusOK, gin.H{
		"state": state, "import": checkpoint, "import_available": runner != nil,
		"cutover_available": cutover != nil, "authority_change": latest, "events": events,
		"legacy_tokens_refused": authn.LegacyTokensRefused(),
	})
}

func identityCutoverError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identitycutover.ErrBusy):
		kernelError(c, http.StatusConflict, "identity_change_running", err.Error())
	case errors.Is(err, identitycutover.ErrNotReady):
		kernelError(c, http.StatusConflict, "identity_not_ready", err.Error())
	case errors.Is(err, identitycutover.ErrTooEarly):
		kernelError(c, http.StatusConflict, "identity_finalize_too_early", err.Error())
	case errors.Is(err, identitycutover.ErrFinal):
		kernelError(c, http.StatusConflict, "identity_finalized", err.Error())
	default:
		kernelError(c, http.StatusInternalServerError, "identity_change_failed", err.Error())
	}
}

func startIdentityChange(c *gin.Context, action string) {
	cutover := identitycutover.Default()
	if cutover == nil {
		kernelError(c, http.StatusServiceUnavailable, "identity_unavailable", "identity cutover needs Control package hosts (plugins.control_execution_enabled)")
		return
	}
	if err := cutover.Start(c.Request.Context(), action, kernelActorID(c)); err != nil {
		identityCutoverError(c, err)
		return
	}
	kernelData(c, http.StatusAccepted, gin.H{"started": true, "action": action})
}

// StartIdentityCutover hands group A to identity in the background;
// IdentityStatus follows it.
func StartIdentityCutover(c *gin.Context) {
	startIdentityChange(c, model.IdentityCutoverActionCutover)
}

// StartIdentityRollback hands group A back to the legacy handlers (before
// finalize) in the background; IdentityStatus follows it.
func StartIdentityRollback(c *gin.Context) {
	startIdentityChange(c, model.IdentityCutoverActionRollback)
}

type identityFinalizeRequest struct {
	Force bool `json:"force"`
}

// FinalizeIdentity removes the legacy credentials. It cannot be undone.
func FinalizeIdentity(c *gin.Context) {
	var request identityFinalizeRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	cutover := identitycutover.Default()
	if cutover == nil {
		kernelError(c, http.StatusServiceUnavailable, "identity_unavailable", "identity finalize needs Control package hosts (plugins.control_execution_enabled)")
		return
	}
	if err := cutover.Finalize(c.Request.Context(), kernelActorID(c), request.Force); err != nil {
		identityCutoverError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"state": model.IdentityAuthorityFinalized})
}

type identityImportRequest struct {
	Delta bool `json:"delta"`
}

// StartIdentityImport starts a full or delta account import in the
// background; IdentityStatus follows it.
func StartIdentityImport(c *gin.Context) {
	var request identityImportRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	runner, _ := identityimport.Default()
	switch err := runner.Start(request.Delta); {
	case errors.Is(err, identityimport.ErrRunning):
		kernelError(c, http.StatusConflict, "identity_import_running", err.Error())
	case errors.Is(err, identityimport.ErrNotInitialized):
		kernelError(c, http.StatusServiceUnavailable, "identity_unavailable", "account import needs Control package hosts (plugins.control_execution_enabled)")
	case err != nil:
		kernelError(c, http.StatusInternalServerError, "identity_import_failed", err.Error())
	default:
		kernelData(c, http.StatusAccepted, gin.H{"started": true, "delta": request.Delta})
	}
}
