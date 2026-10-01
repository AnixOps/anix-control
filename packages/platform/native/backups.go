package native

import (
	"context"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"gorm.io/gorm"
)

// sensitivePlaceholder is the kernel's service.SensitiveSystemConfigPlaceholder,
// shown instead of a stored secret.
const sensitivePlaceholder = "********"

// BackupConfig is a v2_backup_config row, as the kernel model declares it.
type BackupConfig struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	Enabled        bool   `gorm:"default:false" json:"enabled"`
	AutoBackup     bool   `gorm:"default:false" json:"auto_backup"`
	Schedule       string `gorm:"size:50" json:"schedule"`
	RetentionDays  int    `gorm:"default:7" json:"retention_days"`
	BackupDatabase bool   `gorm:"default:true" json:"backup_database"`
	BackupFiles    bool   `gorm:"default:false" json:"backup_files"`
	StorageType    string `gorm:"size:20;default:local" json:"storage_type"`
	StoragePath    string `gorm:"size:255" json:"storage_path"`
	S3Bucket       string `gorm:"size:100" json:"s3_bucket"`
	S3Region       string `gorm:"size:50" json:"s3_region"`
	S3Endpoint     string `gorm:"size:255" json:"s3_endpoint"`
	S3AccessKey    string `gorm:"size:100" json:"s3_access_key"`
	S3SecretKey    string `gorm:"size:100" json:"s3_secret_key"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (BackupConfig) TableName() string { return "v2_backup_config" }

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
func backupConfigResponse(cfg *BackupConfig) map[string]any {
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

// loadBackupConfig is the kernel's BackupService.GetConfig: the first row,
// created with the defaults when there is none.
//
// The kernel's service also keeps the row it read or wrote in memory, and
// reloads it when the backup settings change (a legacy update or a
// KernelSettings write). The stored row holds the same values; after a
// legacy update, only the copy's timestamps can differ from the stored ones
// in precision and time zone (PostgreSQL keeps microseconds). The package
// only creates the defaults, as the kernel does on a first read; it changes
// the row through KernelSettings.
func loadBackupConfig(db *gorm.DB) (*BackupConfig, error) {
	var cfg BackupConfig
	err := db.First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		cfg = BackupConfig{
			Enabled:        false,
			AutoBackup:     false,
			RetentionDays:  7,
			BackupDatabase: true,
			StorageType:    "local",
			StoragePath:    "backups",
		}
		if createErr := db.Create(&cfg).Error; createErr != nil {
			return nil, createErr
		}
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// GetBackupConfig is GET /api/v2/admin/system/backup/config.
func (s *Service) GetBackupConfig(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	cfg, err := loadBackupConfig(db)
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
