package native

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// Settings is the part of the kernel's KernelSettings contract the native
// routes call.
type Settings interface {
	GetSettings(ctx context.Context, in *kernelsettingsv1.GetSettingsRequest, opts ...grpc.CallOption) (*kernelsettingsv1.GetSettingsResponse, error)
	PutSettings(ctx context.Context, in *kernelsettingsv1.PutSettingsRequest, opts ...grpc.CallOption) (*kernelsettingsv1.PutSettingsResponse, error)
}

// backupNamespace is the KernelSettings namespace of the backup
// configuration row; its keys are backup.<field>.
const backupNamespace = "backup"

// backupConfig is the backup configuration as KernelSettings answers it to
// the package (kernel.settings.backup.read.v1, without secrets): the S3
// keys are the placeholder when set, else "".
type backupConfig struct {
	ID             uint
	Enabled        bool
	AutoBackup     bool
	Schedule       string
	RetentionDays  int
	BackupDatabase bool
	BackupFiles    bool
	StorageType    string
	StoragePath    string
	S3Bucket       string
	S3Region       string
	S3Endpoint     string
	S3AccessKey    string
	S3SecretKey    string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// loadBackupConfig reads the backup configuration through KernelSettings,
// as the kernel's BackupService.GetConfig reads it: the first row, which
// the kernel creates with the defaults when there is none.
//
// The kernel's service also keeps the row it read or wrote in memory, and
// reloads it when the backup settings change (a legacy update or a
// KernelSettings write). The stored row holds the same values; after a
// legacy update, only the copy's timestamps can differ from the stored ones
// in precision and time zone (PostgreSQL keeps microseconds).
func (s *Service) loadBackupConfig(ctx context.Context) (*backupConfig, error) {
	response, err := s.Settings.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: backupNamespace})
	if err != nil {
		return nil, errors.New(status.Convert(err).Message())
	}
	var cfg backupConfig
	booleans := map[string]*bool{
		"enabled": &cfg.Enabled, "auto_backup": &cfg.AutoBackup, "backup_database": &cfg.BackupDatabase, "backup_files": &cfg.BackupFiles,
	}
	texts := map[string]*string{
		"schedule": &cfg.Schedule, "storage_type": &cfg.StorageType, "storage_path": &cfg.StoragePath, "s3_bucket": &cfg.S3Bucket,
		"s3_region": &cfg.S3Region, "s3_endpoint": &cfg.S3Endpoint, "s3_access_key": &cfg.S3AccessKey, "s3_secret_key": &cfg.S3SecretKey,
	}
	times := map[string]*time.Time{"created_at": &cfg.CreatedAt, "updated_at": &cfg.UpdatedAt}
	for _, setting := range response.GetSettings() {
		field := strings.TrimPrefix(setting.GetKey(), backupNamespace+".")
		value := setting.GetValue()
		var parseErr error
		switch {
		case booleans[field] != nil:
			*booleans[field], parseErr = strconv.ParseBool(value)
		case texts[field] != nil:
			*texts[field] = value
		case times[field] != nil:
			*times[field], parseErr = time.Parse(time.RFC3339Nano, value)
		case field == "retention_days":
			cfg.RetentionDays, parseErr = strconv.Atoi(value)
		case field == "id":
			var id uint64
			id, parseErr = strconv.ParseUint(value, 10, 0)
			cfg.ID = uint(id)
		}
		if parseErr != nil {
			return nil, fmt.Errorf("backup setting %s: %w", setting.GetKey(), parseErr)
		}
	}
	return &cfg, nil
}

// BackupConfigRequestID names a backup configuration update in the
// kernel's settings request ledger, so a retried request is applied once.
// token identifies the HTTP request: its Idempotency-Key, else its request
// id, else a fresh value.
func BackupConfigRequestID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("platform.backup_config:%x", sum[:12])
}

// requestToken is what identifies the request for BackupConfigRequestID.
func (s *Service) requestToken(request pluginhostsdk.NativeRequest) string {
	for _, name := range []string{"Idempotency-Key", "X-Request-Id"} {
		for key, values := range request.Metadata.Headers {
			if strings.EqualFold(key, name) && len(values) > 0 {
				if value := strings.TrimSpace(values[0]); value != "" {
					return value
				}
			}
		}
	}
	if s.NewToken != nil {
		return s.NewToken()
	}
	return uuid.NewString()
}

// actor is who asks for the change, as the kernel's audit trail records
// it: the administrator the kernel authenticated, with the request's
// address and user agent.
func actor(request pluginhostsdk.NativeRequest) *kernelsettingsv1.Actor {
	userID := uint64(request.Principal.ActorID)
	return &kernelsettingsv1.Actor{UserId: &userID, ClientIp: request.Metadata.ClientIP, UserAgent: request.Metadata.UserAgent}
}

// backupConfigEntries reads an update request as the kernel's
// UpdateBackupConfig does, field by field, into the KernelSettings entries
// of the fields it changes: a field of another JSON type is ignored, a
// float number is truncated, keep_count wins over retention_days and a
// positive interval over schedule. An S3 key sent as the placeholder, or as
// null or blank with preserve_existing_sensitive, keeps the stored key.
func backupConfigEntries(req map[string]any) []*kernelsettingsv1.SettingEntry {
	var entries []*kernelsettingsv1.SettingEntry
	set := func(field, value string) {
		entries = append(entries, &kernelsettingsv1.SettingEntry{Key: backupNamespace + "." + field, Value: value})
	}
	boolean := func(field string) {
		if value, ok := req[field].(bool); ok {
			set(field, strconv.FormatBool(value))
		}
	}
	text := func(field string) {
		if value, ok := req[field].(string); ok {
			set(field, value)
		}
	}
	preserve, _ := req["preserve_existing_sensitive"].(bool)
	secret := func(field string) {
		raw, exists := req[field]
		if !exists {
			return
		}
		if raw == nil {
			if preserve {
				entries = append(entries, &kernelsettingsv1.SettingEntry{Key: backupNamespace + "." + field, Keep: true})
			} else {
				set(field, "")
			}
			return
		}
		value, ok := raw.(string)
		if !ok {
			return
		}
		if value == sensitivePlaceholder || (preserve && strings.TrimSpace(value) == "") {
			entries = append(entries, &kernelsettingsv1.SettingEntry{Key: backupNamespace + "." + field, Keep: true})
			return
		}
		set(field, value)
	}

	boolean("enabled")
	boolean("auto_backup")
	schedule, scheduleSet := req["schedule"].(string)
	if raw, ok := req["interval"].(float64); ok && int(raw) > 0 {
		schedule, scheduleSet = "interval:"+strconv.Itoa(int(raw)), true
	}
	if scheduleSet {
		set("schedule", schedule)
	}
	retention, retentionSet := 0, false
	if raw, ok := req["retention_days"].(float64); ok {
		retention, retentionSet = int(raw), true
	}
	if raw, ok := req["keep_count"].(float64); ok {
		retention, retentionSet = int(raw), true
	}
	if retentionSet {
		set("retention_days", strconv.Itoa(retention))
	}
	boolean("backup_database")
	boolean("backup_files")
	text("storage_type")
	text("storage_path")
	text("s3_bucket")
	text("s3_region")
	text("s3_endpoint")
	secret("s3_access_key")
	secret("s3_secret_key")
	return entries
}

// UpdateBackupConfig is PUT /api/v2/admin/system/backup/config. The kernel
// writes the configuration through KernelSettings (namespace backup): it
// saves the row, records the audit entry its own handler records and makes
// its backup service reload the configuration it keeps in memory, from
// which backup creation takes its storage path. The answer is the stored
// configuration, read back through KernelSettings with the S3 keys masked.
func (s *Service) UpdateBackupConfig(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	// The kernel's handler reads the configuration before it binds the
	// request, creating the defaults when there is none.
	if _, err := s.loadBackupConfig(ctx); err != nil {
		return s.panelError(err.Error())
	}
	var req map[string]any
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}
	if _, err := s.Settings.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{
		Namespace: backupNamespace, RequestId: BackupConfigRequestID(s.requestToken(request)),
		Entries: backupConfigEntries(req), Actor: actor(request),
	}); err != nil {
		return s.panelError(status.Convert(err).Message())
	}
	cfg, err := s.loadBackupConfig(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(backupConfigResponse(cfg))
}
