// Package kernelsettings serves the KernelSettings contract
// (sdk/api/kernelsettings/v1, docs/architecture/settings-service.md) to
// official packages: the system settings in v2_system_config and the
// backup configuration row, per namespace (service.SettingsNamespaces).
//
// Every call is authorized for the namespace's capability against the
// calling host's current generation. Writes run in one transaction with the
// audit entries the kernel's own handlers record for the same change and
// the request ledger, and bump the namespace's generation once committed,
// so every copy of it the kernel keeps in memory reloads before the call
// returns.
package kernelsettings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

const (
	maxRequestID = 128
	maxKeys      = 64
	// maxKeyBytes is v2_system_config.key's size.
	maxKeyBytes   = 100
	maxValueBytes = 256 << 10
	maxTypeBytes  = 20
	maxGroupBytes = 50
	// maxRemarkBytes is v2_system_config.remark's size.
	maxRemarkBytes = 255
	// maxUserAgent is v2_operation_log.user_agent's size, in characters; a
	// longer user agent is cut so the audit entry is always written.
	maxUserAgent = 255

	methodPut    = "put"
	methodDelete = "delete"
)

// RequestRetention is how long the request ledger keeps a request id.
const RequestRetention = 90 * 24 * time.Hour

// systemConfigTypes are the types the kernel's system configuration
// handler accepts.
var systemConfigTypes = map[string]bool{"": true, "string": true, "number": true, "boolean": true, "json": true, "bool": true, "int": true}

// Authorizer admits a calling host to a capability.
type Authorizer interface {
	AuthorizeCapability(ctx context.Context, host packagebridge.HostIdentity, capability string) error
}

// Server holds what every host's KernelSettings calls share.
type Server struct {
	DB         *gorm.DB
	Authorizer Authorizer
	// Now defaults to time.Now.
	Now func() time.Time
}

// For returns the KernelSettings server that host reaches; it has the shape
// of packagebridge.KernelSettingsProvider.
func (s *Server) For(host packagebridge.HostIdentity) kernelsettingsv1.KernelSettingsServer {
	return &hostServer{server: s, host: host}
}

type hostServer struct {
	kernelsettingsv1.UnimplementedKernelSettingsServer
	server *Server
	host   packagebridge.HostIdentity
}

func (h *hostServer) now() time.Time {
	if h.server.Now != nil {
		return h.server.Now()
	}
	return time.Now()
}

// authorize resolves the namespace and admits the host to access on it.
func (h *hostServer) authorize(ctx context.Context, name, access string) (service.SettingsNamespace, *gorm.DB, error) {
	if h.server == nil || h.server.DB == nil || h.server.Authorizer == nil {
		return service.SettingsNamespace{}, nil, status.Error(codes.Unavailable, "kernel settings are not configured")
	}
	namespace, ok := service.LookupSettingsNamespace(name)
	if !ok {
		return service.SettingsNamespace{}, nil, status.Errorf(codes.InvalidArgument, "unknown settings namespace %q", name)
	}
	if _, err := h.allowed(ctx, service.SettingsCapability(name, access), true); err != nil {
		return service.SettingsNamespace{}, nil, err
	}
	return namespace, h.server.DB.WithContext(ctx), nil
}

// allowed reports whether the host holds capability. A missing capability
// is PermissionDenied when required, else false.
func (h *hostServer) allowed(ctx context.Context, capability string, required bool) (bool, error) {
	err := h.server.Authorizer.AuthorizeCapability(ctx, h.host, capability)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, packagebridge.ErrHostFenced):
		return false, status.Error(codes.PermissionDenied, "package host generation is fenced")
	case errors.Is(err, service.ErrCapabilityNotAuthorized):
		if required {
			return false, status.Errorf(codes.PermissionDenied, "package is not authorized for %s", capability)
		}
		return false, nil
	default:
		return false, status.Error(codes.Unavailable, "kernel settings authorization failed")
	}
}

func requestID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxRequestID {
		return "", status.Errorf(codes.InvalidArgument, "request_id is required and at most %d bytes", maxRequestID)
	}
	return value, nil
}

// checkKeys validates keys of namespace: each in it, once, at most maxKeys.
func checkKeys(namespace service.SettingsNamespace, keys []string, required bool) error {
	if required && len(keys) == 0 {
		return status.Error(codes.InvalidArgument, "at least one key is required")
	}
	if len(keys) > maxKeys {
		return status.Errorf(codes.InvalidArgument, "at most %d keys per call", maxKeys)
	}
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if key == "" || len(key) > maxKeyBytes || strings.ContainsFunc(key, invalidKeyRune) {
			return status.Errorf(codes.InvalidArgument, "key %q is invalid", key)
		}
		if !namespace.Contains(key) {
			return status.Errorf(codes.InvalidArgument, "key %q is not in namespace %q", key, namespace.Name)
		}
		if namespace.Storage == service.SettingsInBackupConfig {
			if _, ok := backupFields[strings.TrimPrefix(key, service.BackupSettingsPrefix)]; !ok {
				return status.Errorf(codes.InvalidArgument, "key %q is not a backup setting", key)
			}
		}
		if seen[key] {
			return status.Errorf(codes.InvalidArgument, "key %q is repeated", key)
		}
		seen[key] = true
	}
	return nil
}

func invalidKeyRune(r rune) bool {
	return r <= ' ' || r == 0x7f
}

// failure maps an error of a call's transaction to its status.
func failure(operation string, err error) error {
	if status.Code(err) != codes.Unknown {
		return err
	}
	return status.Errorf(codes.Internal, "%s failed", operation)
}

// GetSettings reads keys of one namespace; every stored key with none.
func (h *hostServer) GetSettings(ctx context.Context, request *kernelsettingsv1.GetSettingsRequest) (*kernelsettingsv1.GetSettingsResponse, error) {
	namespace, db, err := h.authorize(ctx, request.GetNamespace(), service.SettingsAccessRead)
	if err != nil {
		return nil, err
	}
	if err := checkKeys(namespace, request.GetKeys(), false); err != nil {
		return nil, err
	}
	secrets, err := h.allowed(ctx, service.SettingsCapability(namespace.Name, service.SettingsAccessSecrets), false)
	if err != nil {
		return nil, err
	}
	var settings []*kernelsettingsv1.Setting
	if namespace.Storage == service.SettingsInBackupConfig {
		settings, err = getBackup(db, namespace, request.GetKeys(), secrets)
	} else {
		settings, err = getSystemConfig(db, namespace, request.GetKeys(), secrets)
	}
	if err != nil {
		return nil, failure("get settings", err)
	}
	return &kernelsettingsv1.GetSettingsResponse{Settings: settings}, nil
}

// shown is a key's setting as the caller may see it.
func shown(namespace service.SettingsNamespace, key, value string, stored, secrets bool) *kernelsettingsv1.Setting {
	setting := &kernelsettingsv1.Setting{
		Key: key, Value: value, Stored: stored, Secret: namespace.Secret(key), HasValue: strings.TrimSpace(value) != "",
	}
	if setting.Secret && !secrets {
		setting.Masked = true
		setting.Value = ""
		if setting.HasValue {
			setting.Value = service.SensitiveSystemConfigPlaceholder
		}
	}
	return setting
}

func getSystemConfig(db *gorm.DB, namespace service.SettingsNamespace, keys []string, secrets bool) ([]*kernelsettingsv1.Setting, error) {
	var rows []model.SystemConfig
	if len(keys) == 0 {
		var err error
		if rows, err = namespaceRows(db, namespace); err != nil {
			return nil, err
		}
		keys = make([]string, 0, len(rows))
		for _, row := range rows {
			keys = append(keys, row.Key)
		}
	} else if err := db.Where("key IN ?", keys).Find(&rows).Error; err != nil {
		return nil, err
	}
	byKey := make(map[string]model.SystemConfig, len(rows))
	for _, row := range rows {
		byKey[row.Key] = row
	}
	settings := make([]*kernelsettingsv1.Setting, 0, len(keys))
	for _, key := range keys {
		row, stored := byKey[key]
		setting := shown(namespace, key, row.Value, stored, secrets)
		setting.Type, setting.Group, setting.Remark = row.Type, row.Group, row.Remark
		settings = append(settings, setting)
	}
	return settings, nil
}

// namespaceRows reads every stored key of namespace, by key.
func namespaceRows(db *gorm.DB, namespace service.SettingsNamespace) ([]model.SystemConfig, error) {
	query := db.Where("1 = 0")
	if len(namespace.Keys) > 0 {
		query = query.Or("key IN ?", namespace.Keys)
	}
	for _, prefix := range namespace.Prefixes {
		query = query.Or(`key LIKE ? ESCAPE '\'`, likePrefix(prefix))
	}
	var candidates []model.SystemConfig
	if err := query.Order("key").Find(&candidates).Error; err != nil {
		return nil, err
	}
	rows := candidates[:0]
	for _, row := range candidates {
		if namespace.Contains(row.Key) {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func likePrefix(prefix string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix)
	return escaped + "%"
}

// PutSettings creates or changes keys of one namespace.
func (h *hostServer) PutSettings(ctx context.Context, request *kernelsettingsv1.PutSettingsRequest) (*kernelsettingsv1.PutSettingsResponse, error) {
	namespace, db, err := h.authorize(ctx, request.GetNamespace(), service.SettingsAccessWrite)
	if err != nil {
		return nil, err
	}
	id, err := requestID(request.GetRequestId())
	if err != nil {
		return nil, err
	}
	entries := request.GetEntries()
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.GetKey())
		if err := checkEntry(namespace, entry); err != nil {
			return nil, err
		}
	}
	// A backup write with no entries saves the row as it is and records its
	// audit entry, as the kernel's handler does for a request that changes
	// nothing.
	if err := checkKeys(namespace, keys, namespace.Storage != service.SettingsInBackupConfig); err != nil {
		return nil, err
	}
	actor, err := auditActor(db, request.GetActor())
	if err != nil {
		return nil, err
	}
	var result ledgerResult
	applied := false
	err = db.Transaction(func(tx *gorm.DB) error {
		replayed, err := h.replay(tx, id, namespace.Name, methodPut, &result)
		if err != nil || replayed {
			return err
		}
		if namespace.Storage == service.SettingsInBackupConfig {
			result.Changes, err = putBackup(tx, namespace, entries, actor)
		} else {
			result.Changes, err = putSystemConfig(tx, namespace, entries, actor)
		}
		if err != nil {
			return err
		}
		applied = true
		return h.record(tx, id, namespace.Name, methodPut, result)
	})
	if err != nil {
		return nil, failure("put settings", err)
	}
	if applied {
		service.BumpSettingsGeneration(namespace.Name)
	}
	return &kernelsettingsv1.PutSettingsResponse{Applied: applied, Changes: result.changes()}, nil
}

func checkEntry(namespace service.SettingsNamespace, entry *kernelsettingsv1.SettingEntry) error {
	if len(entry.GetValue()) > maxValueBytes || strings.ContainsRune(entry.GetValue(), 0) {
		return status.Errorf(codes.InvalidArgument, "the value of %q is too long or holds a NUL", entry.GetKey())
	}
	if namespace.Storage == service.SettingsInBackupConfig {
		if entry.GetType() != "" || entry.GetGroup() != "" || entry.GetRemark() != "" {
			return status.Errorf(codes.InvalidArgument, "backup setting %q takes no type, group or remark", entry.GetKey())
		}
		return nil
	}
	if !systemConfigTypes[entry.GetType()] {
		return status.Errorf(codes.InvalidArgument, "type of %q must be one of string, number, boolean, json, bool, int", entry.GetKey())
	}
	if len(entry.GetGroup()) > maxGroupBytes || len(entry.GetRemark()) > maxRemarkBytes || len(entry.GetType()) > maxTypeBytes {
		return status.Errorf(codes.InvalidArgument, "group or remark of %q is too long", entry.GetKey())
	}
	return nil
}

// kept reports whether entry leaves the stored value: keep, or a secret's
// placeholder.
func kept(namespace service.SettingsNamespace, entry *kernelsettingsv1.SettingEntry) bool {
	return entry.GetKeep() || (namespace.Secret(entry.GetKey()) && entry.GetValue() == service.SensitiveSystemConfigPlaceholder)
}

// putSystemConfig writes entries to v2_system_config as the kernel's system
// configuration handler does, with its audit entries when the namespace
// records them.
func putSystemConfig(tx *gorm.DB, namespace service.SettingsNamespace, entries []*kernelsettingsv1.SettingEntry, actor service.SettingsAuditActor) ([]ledgerChange, error) {
	configs := service.NewSystemConfigService(tx)
	operations := service.NewOperationLogService(tx)
	changes := make([]ledgerChange, 0, len(entries))
	for _, entry := range entries {
		key := entry.GetKey()
		existing, err := configs.GetEntry(key)
		if err != nil {
			return nil, err
		}
		keep := kept(namespace, entry)
		if keep && existing == nil {
			// Nothing to keep: the key is not created.
			changes = append(changes, ledgerChange{Key: key})
			continue
		}
		value := entry.GetValue()
		preserved := keep
		if keep {
			value = existing.Value
		} else {
			// A masked field sent as the placeholder (the SMTP password in
			// the e-mail configuration) keeps its stored secret, as the
			// kernel's handlers keep it.
			stored := ""
			if existing != nil {
				stored = existing.Value
			}
			value, preserved = service.KeepSystemConfigFields(key, value, stored)
		}
		if err := configs.Set(key, value, entry.GetType(), entry.GetGroup(), entry.GetRemark()); err != nil {
			return nil, err
		}
		changes = append(changes, ledgerChange{Key: key, Created: existing == nil, Kept: keep})
		if namespace.Audit != service.SettingsAuditSystemConfig {
			continue
		}
		saved, err := configs.GetEntry(key)
		if err != nil {
			return nil, err
		}
		action := "update"
		if existing == nil {
			action = "create"
		}
		if err := operations.Record(service.SystemConfigAuditInput(actor, action, saved, preserved)); err != nil {
			return nil, err
		}
	}
	return changes, nil
}

// putBackup writes entries to the backup configuration row as the kernel's
// backup configuration handler does: the first row, created with the
// defaults when there is none, and one audit entry naming the secrets kept.
func putBackup(tx *gorm.DB, namespace service.SettingsNamespace, entries []*kernelsettingsv1.SettingEntry, actor service.SettingsAuditActor) ([]ledgerChange, error) {
	cfg, err := service.LoadBackupConfig(tx)
	if err != nil {
		return nil, err
	}
	preserved := make([]string, 0, 2)
	changes := make([]ledgerChange, 0, len(entries))
	for _, entry := range entries {
		field := strings.TrimPrefix(entry.GetKey(), service.BackupSettingsPrefix)
		if kept(namespace, entry) {
			if namespace.Secret(entry.GetKey()) {
				preserved = append(preserved, field)
			}
			changes = append(changes, ledgerChange{Key: entry.GetKey(), Kept: true})
			continue
		}
		if err := setBackupField(cfg, field, entry.GetValue()); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "%s: %v", entry.GetKey(), err)
		}
		changes = append(changes, ledgerChange{Key: entry.GetKey()})
	}
	if err := tx.Save(cfg).Error; err != nil {
		return nil, err
	}
	if err := service.NewOperationLogService(tx).Record(service.BackupConfigAuditInput(actor, "update", cfg, preserved)); err != nil {
		return nil, err
	}
	return changes, nil
}

// DeleteSettings removes keys of one namespace.
func (h *hostServer) DeleteSettings(ctx context.Context, request *kernelsettingsv1.DeleteSettingsRequest) (*kernelsettingsv1.DeleteSettingsResponse, error) {
	namespace, db, err := h.authorize(ctx, request.GetNamespace(), service.SettingsAccessWrite)
	if err != nil {
		return nil, err
	}
	if namespace.Storage == service.SettingsInBackupConfig {
		return nil, status.Error(codes.FailedPrecondition, "backup settings cannot be deleted")
	}
	id, err := requestID(request.GetRequestId())
	if err != nil {
		return nil, err
	}
	if err := checkKeys(namespace, request.GetKeys(), true); err != nil {
		return nil, err
	}
	actor, err := auditActor(db, request.GetActor())
	if err != nil {
		return nil, err
	}
	var result ledgerResult
	applied := false
	err = db.Transaction(func(tx *gorm.DB) error {
		replayed, err := h.replay(tx, id, namespace.Name, methodDelete, &result)
		if err != nil || replayed {
			return err
		}
		configs := service.NewSystemConfigService(tx)
		operations := service.NewOperationLogService(tx)
		for _, key := range request.GetKeys() {
			existing, err := configs.GetEntry(key)
			if err != nil {
				return err
			}
			if existing == nil {
				result.Changes = append(result.Changes, ledgerChange{Key: key})
				continue
			}
			if err := configs.Delete(key); err != nil {
				return err
			}
			result.Changes = append(result.Changes, ledgerChange{Key: key, Deleted: true})
			if namespace.Audit == service.SettingsAuditSystemConfig {
				if err := operations.Record(service.SystemConfigAuditInput(actor, "delete", existing, false)); err != nil {
					return err
				}
			}
		}
		applied = true
		return h.record(tx, id, namespace.Name, methodDelete, result)
	})
	if err != nil {
		return nil, failure("delete settings", err)
	}
	if applied {
		service.BumpSettingsGeneration(namespace.Name)
	}
	return &kernelsettingsv1.DeleteSettingsResponse{Applied: applied, Changes: result.changes()}, nil
}

// auditActor is the caller's actor as the audit trail records it. The
// username is the user's e-mail, looked up by id before the write's
// transaction, as the kernel's legacy handlers look it up when the package
// bridge relays a request with the actor's id only.
func auditActor(db *gorm.DB, actor *kernelsettingsv1.Actor) (service.SettingsAuditActor, error) {
	var result service.SettingsAuditActor
	if actor == nil {
		return result, nil
	}
	if actor.UserId != nil {
		if actor.GetUserId() > uint64(^uint32(0)) {
			return result, status.Error(codes.InvalidArgument, "actor user_id is invalid")
		}
		id := uint(actor.GetUserId())
		result.UserID = &id
		result.Username = service.AuditUsername(db, result.UserID)
	}
	result.IP = strings.TrimSpace(actor.GetClientIp())
	if result.IP != "" && net.ParseIP(result.IP) == nil {
		return result, status.Error(codes.InvalidArgument, "actor client_ip is not an IP address")
	}
	userAgent := actor.GetUserAgent()
	if strings.ContainsAny(userAgent, "\r\n\x00") || !utf8.ValidString(userAgent) {
		return result, status.Error(codes.InvalidArgument, "actor user_agent is invalid")
	}
	if utf8.RuneCountInString(userAgent) > maxUserAgent {
		userAgent = string([]rune(userAgent)[:maxUserAgent])
	}
	result.UserAgent = userAgent
	return result, nil
}

// ledgerChange is a recorded SettingChange.
type ledgerChange struct {
	Key     string `json:"key"`
	Created bool   `json:"created,omitempty"`
	Kept    bool   `json:"kept,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

// ledgerResult is what the request ledger keeps of a write.
type ledgerResult struct {
	Changes []ledgerChange `json:"changes"`
}

func (r ledgerResult) changes() []*kernelsettingsv1.SettingChange {
	changes := make([]*kernelsettingsv1.SettingChange, 0, len(r.Changes))
	for _, change := range r.Changes {
		changes = append(changes, &kernelsettingsv1.SettingChange{Key: change.Key, Created: change.Created, Kept: change.Kept, Deleted: change.Deleted})
	}
	return changes
}

// replay loads the result of a request id applied before. A request id used
// for another namespace, method or package is refused.
func (h *hostServer) replay(tx *gorm.DB, id, namespace, method string, result *ledgerResult) (bool, error) {
	var row model.SettingsRequest
	err := tx.Where("request_id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.Namespace != namespace || row.Method != method || row.PackageID != h.host.PackageID {
		return true, status.Errorf(codes.FailedPrecondition, "request_id %q was used for another request", id)
	}
	if err := json.Unmarshal([]byte(row.Result), result); err != nil {
		return true, fmt.Errorf("settings request %s: stored result: %w", id, err)
	}
	return true, nil
}

func (h *hostServer) record(tx *gorm.DB, id, namespace, method string, result ledgerResult) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return tx.Create(&model.SettingsRequest{
		RequestID: id, Namespace: namespace, Method: method, PackageID: h.host.PackageID, Result: string(encoded), CreatedAt: h.now(),
	}).Error
}

// PruneRequests deletes request ledger rows older than RequestRetention.
func PruneRequests(db *gorm.DB, now time.Time) (int64, error) {
	result := db.Where("created_at < ?", now.Add(-RequestRetention)).Delete(&model.SettingsRequest{})
	return result.RowsAffected, result.Error
}

// backupFields are the backup namespace's keys (backup.<field>) and the
// configuration field each one is.
var backupFields = map[string]func(*model.BackupConfig) any{
	"enabled":         func(c *model.BackupConfig) any { return &c.Enabled },
	"auto_backup":     func(c *model.BackupConfig) any { return &c.AutoBackup },
	"schedule":        func(c *model.BackupConfig) any { return &c.Schedule },
	"retention_days":  func(c *model.BackupConfig) any { return &c.RetentionDays },
	"backup_database": func(c *model.BackupConfig) any { return &c.BackupDatabase },
	"backup_files":    func(c *model.BackupConfig) any { return &c.BackupFiles },
	"storage_type":    func(c *model.BackupConfig) any { return &c.StorageType },
	"storage_path":    func(c *model.BackupConfig) any { return &c.StoragePath },
	"s3_bucket":       func(c *model.BackupConfig) any { return &c.S3Bucket },
	"s3_region":       func(c *model.BackupConfig) any { return &c.S3Region },
	"s3_endpoint":     func(c *model.BackupConfig) any { return &c.S3Endpoint },
	"s3_access_key":   func(c *model.BackupConfig) any { return &c.S3AccessKey },
	"s3_secret_key":   func(c *model.BackupConfig) any { return &c.S3SecretKey },
}

// backupField is a field's value as a setting: a boolean as true or false,
// a number in decimal.
func backupField(cfg *model.BackupConfig, field string) string {
	switch value := backupFields[field](cfg).(type) {
	case *bool:
		return strconv.FormatBool(*value)
	case *int:
		return strconv.Itoa(*value)
	case *string:
		return *value
	}
	return ""
}

func setBackupField(cfg *model.BackupConfig, field, value string) error {
	switch target := backupFields[field](cfg).(type) {
	case *bool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New("not a boolean")
		}
		*target = parsed
	case *int:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return errors.New("not an integer")
		}
		*target = parsed
	case *string:
		*target = value
	}
	return nil
}

// getBackup reads backup fields: the first row's, or the defaults the
// kernel would create (stored false) when there is none. A read creates
// nothing.
func getBackup(db *gorm.DB, namespace service.SettingsNamespace, keys []string, secrets bool) ([]*kernelsettingsv1.Setting, error) {
	var cfg model.BackupConfig
	stored := true
	err := db.First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cfg, stored = service.DefaultBackupConfig(), false
	} else if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		for field := range backupFields {
			keys = append(keys, service.BackupSettingsPrefix+field)
		}
		sort.Strings(keys)
	}
	settings := make([]*kernelsettingsv1.Setting, 0, len(keys))
	for _, key := range keys {
		settings = append(settings, shown(namespace, key, backupField(&cfg, strings.TrimPrefix(key, service.BackupSettingsPrefix)), stored, secrets))
	}
	return settings, nil
}
