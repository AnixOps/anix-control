package native

import (
	"context"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// sensitivePlaceholder is the kernel's service.SensitiveSystemConfigPlaceholder,
// shown instead of a stored secret.
const sensitivePlaceholder = "********"

// BackupRecord is a v2_backup_record row, as the kernel model declares it.
type BackupRecord struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	Name      string `gorm:"size:100" json:"name"`
	Type      string `gorm:"size:20" json:"type"`
	Size      int64  `json:"size"`
	Path      string `gorm:"size:255" json:"path"`
	Status    int    `gorm:"default:0" json:"status"`
	Error     string `gorm:"type:text" json:"error"`
	Auto      bool   `gorm:"default:false" json:"auto"`
	CreatedBy *uint  `json:"created_by"`

	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (BackupRecord) TableName() string { return "v2_backup_record" }

func backupStatusToText(status int) string {
	switch status {
	case 1:
		return "completed"
	case 2:
		return "failed"
	default:
		return "pending"
	}
}

func backupIntervalFromSchedule(schedule string) int {
	schedule = strings.TrimSpace(schedule)
	if strings.HasPrefix(schedule, "interval:") {
		raw := strings.TrimPrefix(schedule, "interval:")
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 24
}

func maskBackupSensitiveValue(value string) (displayValue string, sensitive bool, hasValue bool) {
	hasValue = strings.TrimSpace(value) != ""
	if !hasValue {
		return "", true, false
	}

	return sensitivePlaceholder, true, true
}

// backupConfigResponse is the kernel's answer for the backup configuration;
// the S3 keys are never returned.
func backupConfigResponse(cfg *backupConfig) map[string]any {
	s3AccessKeyDisplayValue, s3AccessKeySensitive, s3AccessKeyHasValue := maskBackupSensitiveValue(cfg.S3AccessKey)
	s3SecretKeyDisplayValue, s3SecretKeySensitive, s3SecretKeyHasValue := maskBackupSensitiveValue(cfg.S3SecretKey)

	return map[string]any{
		"id":                          cfg.ID,
		"enabled":                     cfg.Enabled,
		"auto_backup":                 cfg.AutoBackup,
		"schedule":                    cfg.Schedule,
		"retention_days":              cfg.RetentionDays,
		"backup_database":             cfg.BackupDatabase,
		"backup_files":                cfg.BackupFiles,
		"storage_type":                cfg.StorageType,
		"storage_path":                cfg.StoragePath,
		"s3_bucket":                   cfg.S3Bucket,
		"s3_region":                   cfg.S3Region,
		"s3_endpoint":                 cfg.S3Endpoint,
		"s3_access_key":               s3AccessKeyDisplayValue,
		"s3_access_key_display_value": s3AccessKeyDisplayValue,
		"s3_access_key_sensitive":     s3AccessKeySensitive,
		"s3_access_key_has_value":     s3AccessKeyHasValue,
		"s3_secret_key":               s3SecretKeyDisplayValue,
		"s3_secret_key_display_value": s3SecretKeyDisplayValue,
		"s3_secret_key_sensitive":     s3SecretKeySensitive,
		"s3_secret_key_has_value":     s3SecretKeyHasValue,
		"created_at":                  cfg.CreatedAt,
		"updated_at":                  cfg.UpdatedAt,
		// Frontend aliases used by System.vue.
		"interval":   backupIntervalFromSchedule(cfg.Schedule),
		"keep_count": cfg.RetentionDays,
	}
}

// GetBackupConfig is GET /api/v2/admin/system/backup/config: the
// configuration read through KernelSettings, with the S3 keys masked.
func (s *Service) GetBackupConfig(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	cfg, err := s.loadBackupConfig(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}

	return s.panel(backupConfigResponse(cfg))
}

// ListBackups is GET /api/v2/admin/system/backups: the backup records, newest
// first. Like the kernel's service it ignores a failed count.
func (s *Service) ListBackups(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	page, pageSize := clampPagination(pagination(request))

	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	var records []BackupRecord
	var total int64

	db.Model(&BackupRecord{}).Count(&total)
	offset := (page - 1) * pageSize
	if err := db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&records).Error; err != nil {
		return s.panelError(err.Error())
	}

	list := make([]map[string]any, 0, len(records))
	for _, record := range records {
		filename := record.Name
		if record.Path != "" {
			filename = filepath.Base(record.Path)
		}
		list = append(list, map[string]any{
			"id":           record.ID,
			"name":         record.Name,
			"filename":     filename,
			"type":         record.Type,
			"size":         record.Size,
			"path":         record.Path,
			"status":       backupStatusToText(record.Status),
			"status_code":  record.Status,
			"error":        record.Error,
			"auto":         record.Auto,
			"created_by":   record.CreatedBy,
			"completed_at": record.CompletedAt,
			"created_at":   record.CreatedAt,
			"updated_at":   record.UpdatedAt,
		})
	}

	return s.panel(map[string]any{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetBackupStats is GET /api/v2/admin/system/backup/stats: count, total size
// and time of the successful backups. Like the kernel's service it ignores
// failed queries, which count as nothing.
func (s *Service) GetBackupStats(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var totalBackups int64
	var totalSize int64
	var lastBackup time.Time

	if db, err := s.Open(ctx); err != nil {
		log.Printf("platform backup stats: open storage: %v", err)
	} else {
		db.Model(&BackupRecord{}).Where("status = 1").Count(&totalBackups)
		db.Model(&BackupRecord{}).Where("status = 1").
			Select("COALESCE(SUM(size), 0)").Scan(&totalSize)

		var record BackupRecord
		if err := db.Where("status = 1").Order("created_at DESC").First(&record).Error; err == nil {
			lastBackup = record.CreatedAt
		}
	}

	stats := map[string]any{
		"total_backups": totalBackups,
		"total_size":    totalSize,
	}
	if !lastBackup.IsZero() {
		stats["last_backup"] = lastBackup
	}
	stats["total_count"] = stats["total_backups"]

	return s.panel(stats)
}
