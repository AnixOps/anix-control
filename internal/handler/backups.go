package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/backup"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/kernelalerts"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// BackupsHandler answers the console's backup page and the dashboard
// (docs/reference/backups.md): the bundles with their verification, a
// manual backup, and the download of a bundle. There is no restore here:
// a restore runs on the server with Control stopped
// (`anix-control backup restore`).
type BackupsHandler struct {
	manager func() *backup.Manager
	db      func() *gorm.DB
	// background runs the verification that follows a manual backup.
	background func(func())
	now        func() time.Time
}

// NewBackupsHandler reads the process's backup manager (backup.Default).
func NewBackupsHandler() *BackupsHandler {
	return &BackupsHandler{manager: backup.Default, db: database.Get, background: func(run func()) { go run() }, now: time.Now}
}

// backupView is a bundle as the API answers it.
type backupView struct {
	ID             uint       `json:"id"`
	File           string     `json:"file"`
	Type           string     `json:"type"`
	Size           int64      `json:"size"`
	SHA256         string     `json:"sha256"`
	CreatedAt      time.Time  `json:"created_at"`
	VerifiedAt     *time.Time `json:"verified_at"`
	VerifyStatus   string     `json:"verify_status"`
	VerifyError    string     `json:"verify_error"`
	ProgramVersion string     `json:"program_version"`
	SchemaVersion  int        `json:"schema_version"`
}

func viewOf(row model.Backup) backupView {
	return backupView{
		ID: row.ID, File: row.Path, Type: row.Type, Size: row.Size, SHA256: row.SHA256, CreatedAt: row.CreatedAt,
		VerifiedAt: row.VerifiedAt, VerifyStatus: row.VerifyStatus, VerifyError: row.VerifyError,
		ProgramVersion: row.ProgramVersion, SchemaVersion: row.SchemaVersion,
	}
}

// backupSummary is the state the dashboard shows.
type backupSummary struct {
	// LastVerified is the newest bundle that passed its verification.
	LastVerified *backupView `json:"last_verified"`
	// Stale is true when no verified bundle is younger than
	// StaleAfterHours and the database is older than that (the
	// backup_stale alert's rule).
	Stale           bool       `json:"stale"`
	StaleAfterHours int64      `json:"stale_after_hours"`
	NextDailyAt     *time.Time `json:"next_daily_at"`
	Directory       string     `json:"directory"`
}

type backupRetention struct {
	Daily            int `json:"daily"`
	Weekly           int `json:"weekly"`
	PreMigrationDays int `json:"pre_migration_days"`
	Manual           int `json:"manual"`
	Verify           int `json:"verify"`
}

// backupsAnswer is GET /api/v5/backups.
type backupsAnswer struct {
	// Supported is false on PostgreSQL (Reason says why).
	Supported bool            `json:"supported"`
	Reason    string          `json:"reason,omitempty"`
	Backups   []backupView    `json:"backups"`
	Summary   backupSummary   `json:"summary"`
	Retention backupRetention `json:"retention"`
}

var retentionView = backupRetention{
	Daily: backup.KeepDaily, Weekly: backup.KeepWeekly, PreMigrationDays: int(backup.KeepPreMigration / (24 * time.Hour)),
	Manual: backup.KeepManual, Verify: backup.KeepVerify,
}

// List godoc
// @Summary The backup bundles and the last verified backup
// @Description The bundles in data_dir/backups, newest first, in data.backups: id, file, type (daily, pre-migration, manual, verify), size, sha256, created_at, verified_at, verify_status (pending, passed, failed), verify_error, program_version, schema_version. data.summary has last_verified (the newest bundle that passed its verification, or null), stale (no verified bundle younger than stale_after_hours, 36, on a database older than that), next_daily_at and directory; data.retention the built-in retention. On PostgreSQL data.supported is false and data.reason says why: backups need SQLite. Any administrator; no secret.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v5/backups [get]
func (h *BackupsHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	manager := h.manager()
	if manager == nil {
		kernelError(c, http.StatusServiceUnavailable, "backups_unavailable", "this process runs no backups")
		return
	}
	answer := backupsAnswer{Backups: []backupView{}, Retention: retentionView, Summary: backupSummary{StaleAfterHours: int64(kernelalerts.BackupStaleAfter / time.Hour)}}
	if err := manager.Supported(); err != nil {
		answer.Reason = err.Error()
		kernelData(c, http.StatusOK, answer)
		return
	}
	rows, err := manager.List(c.Request.Context())
	if err != nil {
		kernelError(c, http.StatusInternalServerError, "backups_unreadable", err.Error())
		return
	}
	answer.Supported = true
	for _, row := range rows {
		answer.Backups = append(answer.Backups, viewOf(row))
		if row.VerifyStatus == model.BackupVerifyPassed && answer.Summary.LastVerified == nil {
			view := viewOf(row)
			answer.Summary.LastVerified = &view
		}
	}
	now := h.now()
	next := backup.NextDaily(dailySeed(manager), now.Local())
	answer.Summary.NextDailyAt, answer.Summary.Directory = &next, manager.Dir()
	answer.Summary.Stale = h.stale(c.Request.Context(), answer.Summary.LastVerified, now)
	kernelData(c, http.StatusOK, answer)
}

// stale is the backup_stale alert's rule: no verified bundle younger than
// BackupStaleAfter, on a database older than that.
func (h *BackupsHandler) stale(ctx context.Context, last *backupView, now time.Time) bool {
	if last != nil {
		return now.Sub(last.CreatedAt) > kernelalerts.BackupStaleAfter
	}
	db := h.db()
	if db == nil {
		return true
	}
	meta, err := migrations.Meta(ctx, db)
	if err != nil {
		return true
	}
	created, err := time.Parse(time.RFC3339, meta[migrations.MetaCreatedAt])
	return err != nil || now.Sub(created) > kernelalerts.BackupStaleAfter
}

// dailySeed is what picks the daily time: the domain, else the host name
// (cmd/server backupWorker).
func dailySeed(manager *backup.Manager) string {
	if manager.Domain != "" {
		return manager.Domain
	}
	name, _ := os.Hostname()
	return name
}

// Create godoc
// @Summary Back up now
// @Description Writes a manual bundle of the database, root.key and the configuration file into data_dir/backups and answers it (201, data.backup, verify_status pending); its verification follows in the background and the list shows the result. 409 backups_need_sqlite on PostgreSQL, 409 backup_in_progress while another backup or verification runs. Any administrator.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 201 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /api/v5/backups [post]
func (h *BackupsHandler) Create(c *gin.Context) {
	manager := h.manager()
	if manager == nil {
		kernelError(c, http.StatusServiceUnavailable, "backups_unavailable", "this process runs no backups")
		return
	}
	_, row, err := manager.Create(c.Request.Context(), backup.TypeManual)
	switch {
	case errors.Is(err, backup.ErrNeedsSQLite):
		kernelError(c, http.StatusConflict, "backups_need_sqlite", err.Error())
		return
	case errors.Is(err, backup.ErrBusy):
		kernelError(c, http.StatusConflict, "backup_in_progress", err.Error())
		return
	case err != nil:
		_ = c.Error(err)
		kernelError(c, http.StatusInternalServerError, "backup_failed", err.Error())
		return
	}
	h.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		_, _, _ = manager.VerifyNewestUnverified(ctx)
	})
	kernelData(c, http.StatusCreated, gin.H{"backup": viewOf(*row)})
}

// Download godoc
// @Summary Download a backup bundle
// @Description Streams the bundle (application/x-tar, attachment). A bundle holds the root key, which opens every key Control keeps, so only a super administrator may download one, and every download is in the audit log (action download). 403 super_admin_required, 404 backup_not_found.
// @Tags Kernel
// @Produce application/x-tar
// @Security BearerAuth
// @Param id path int true "backup id"
// @Success 200 {file} file
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/v5/backups/{id}/download [get]
func (h *BackupsHandler) Download(c *gin.Context) {
	super, err := service.IsSuperAdmin(h.db(), kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if !super {
		kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may download a backup: it holds the root key")
		return
	}
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	manager := h.manager()
	if manager == nil {
		kernelError(c, http.StatusServiceUnavailable, "backups_unavailable", "this process runs no backups")
		return
	}
	row, err := manager.Find(c.Request.Context(), id)
	if errors.Is(err, backup.ErrNeedsSQLite) {
		kernelError(c, http.StatusConflict, "backups_need_sqlite", err.Error())
		return
	}
	if err != nil {
		kernelError(c, http.StatusNotFound, "backup_not_found", "no backup with this id")
		return
	}
	file, err := manager.Open(row)
	if err != nil {
		kernelError(c, http.StatusNotFound, "backup_not_found", "the bundle is no longer there")
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		kernelError(c, http.StatusInternalServerError, "backup_unreadable", err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	c.DataFromReader(http.StatusOK, info.Size(), "application/x-tar", file, map[string]string{
		"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, row.Path),
		"X-Content-SHA256":    row.SHA256,
	})
}
