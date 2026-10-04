package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

// forwardLegacyArchivePath is the download of the v4.1 flux forwarding
// archive (forward-sdk.md section 10, F5c). The kernel answers it, not the
// forward package: the archive is a file on Control's host. Only the
// archive is served here; the drop has no HTTP route on purpose (it is
// irreversible and runs only from the command line).
const forwardLegacyArchivePath = ForwardV4Prefix + "/legacy/archive"

// ForwardLegacyArchive serves the newest recorded archive to a super
// administrator, and writes every attempt to the audit log.
func (h *KernelHandler) ForwardLegacyArchive(c *gin.Context) {
	started := time.Now()
	if c.Request.Method != http.MethodGet {
		kernelError(c, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	allowed, err := service.IsSuperAdmin(h.db, kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if !allowed {
		h.auditLegacyArchive(c, started, http.StatusForbidden, "super administrator required")
		kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may download the forwarding archive")
		return
	}
	record, err := forwardlegacy.LatestArchive(h.db)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if record == nil {
		h.auditLegacyArchive(c, started, http.StatusNotFound, "no archive")
		kernelError(c, http.StatusNotFound, "legacy_archive_not_found", "no forwarding archive was written")
		return
	}
	if _, err := forwardlegacy.VerifyArchive(record); err != nil {
		h.auditLegacyArchive(c, started, http.StatusConflict, err.Error())
		kernelError(c, http.StatusConflict, "legacy_archive_unreadable", "the recorded forwarding archive is missing or changed: write a new one with anix-control forward legacy archive")
		return
	}
	content, err := os.ReadFile(record.Path) // #nosec G304 -- the path is the recorded archive's.
	if err != nil {
		h.auditLegacyArchive(c, started, http.StatusConflict, err.Error())
		kernelError(c, http.StatusConflict, "legacy_archive_unreadable", "the recorded forwarding archive is not readable")
		return
	}
	h.auditLegacyArchive(c, started, http.StatusOK, "")
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(record.Path)))
	c.Header("X-Archive-SHA256", record.SHA256)
	c.Data(http.StatusOK, "application/json", content)
}

// auditLegacyArchive records a download attempt. The audit middleware
// records writes only; this read hands out every flux forward, so it is
// recorded like one.
func (h *KernelHandler) auditLegacyArchive(c *gin.Context, started time.Time, status int, message string) {
	entry := model.AuditLog{
		Method: c.Request.Method, Path: c.Request.URL.Path, Module: "forward", Action: "legacy_archive_download",
		IP: c.ClientIP(), UserAgent: c.GetHeader("User-Agent"), RequestID: c.GetHeader("X-Request-ID"),
		StatusCode: status, DurationMS: time.Since(started).Milliseconds(), ErrorMessage: message,
	}
	if id := kernelActorID(c); id != 0 {
		entry.UserID = &id
	}
	if email, ok := c.Get("email"); ok {
		entry.Email, _ = email.(string)
	}
	_ = h.db.Create(&entry).Error
}
