package handler

import (
	"errors"
	"net/http"

	"github.com/AnixOps/anix-control/v4/internal/identityimport"
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
	kernelData(c, http.StatusOK, gin.H{"state": state, "import": checkpoint, "import_available": runner != nil})
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
