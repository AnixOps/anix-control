package native

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// AuditLog is a row of kapi_system_audit_log_v1, the kernel's read-only view
// of the v2_operation_log rows of module "system". Its fields and tags are
// the kernel model's.
type AuditLog struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	UserID     *uint  `gorm:"index" json:"user_id"`
	Username   string `gorm:"size:100" json:"username"`
	Action     string `gorm:"size:50;index" json:"action"`
	Module     string `gorm:"size:50;index" json:"module"`
	TargetType string `gorm:"size:50" json:"target_type"`
	TargetID   *uint  `json:"target_id"`
	Content    string `gorm:"type:text" json:"content"`
	IP         string `gorm:"size:45" json:"ip"`
	UserAgent  string `gorm:"size:255" json:"user_agent"`
	Status     int    `gorm:"default:1" json:"status"`

	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// TableName is the kernel view.
func (AuditLog) TableName() string { return "kapi_system_audit_log_v1" }

// Maximum page size of the audit log list, which does not use the kernel's
// usual pagination limits.
const maxAuditLogPageSize = 200

var auditKeyValueSecretPattern = regexp.MustCompile(`(?i)((?:password|passwd|secret|token|api[_-]?key|private[_-]?key|access[_-]?key)\s*[=:]\s*)([^,\s;]+)`)

// sanitizeAuditLogContent is the kernel's: JSON content with its sensitive
// fields redacted, or else key=value secrets replaced.
func sanitizeAuditLogContent(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}

	redactedJSON := redactJSON(trimmed)
	if redactedJSON != trimmed {
		return redactedJSON
	}

	return auditKeyValueSecretPattern.ReplaceAllString(trimmed, `${1}[REDACTED]`)
}

// sensitiveKeys is the kernel's utils.SensitiveKeys.
var sensitiveKeys = []string{
	"password",
	"secret",
	"api_key",
	"apikey",
	"api-key",
	"token",
	"access_token",
	"refresh_token",
	"auth_key",
	"private_key",
	"credential",
}

// redact is the kernel's utils.Redact.
func redact(s string) string {
	if s == "" {
		return ""
	}
	length := len(s)
	if length <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[length-4:]
}

// redactMap is the kernel's utils.RedactMap.
func redactMap(data map[string]any) map[string]any {
	result := make(map[string]any)
	for key, value := range data {
		lowerKey := strings.ToLower(key)
		isSensitive := false
		for _, sk := range sensitiveKeys {
			if strings.Contains(lowerKey, sk) {
				isSensitive = true
				break
			}
		}
		if isSensitive {
			if str, ok := value.(string); ok {
				result[key] = redact(str)
			} else {
				result[key] = "[REDACTED]"
			}
		} else {
			if nested, ok := value.(map[string]any); ok {
				result[key] = redactMap(nested)
			} else {
				result[key] = value
			}
		}
	}
	return result
}

// redactJSON is the kernel's utils.RedactJSON.
func redactJSON(jsonStr string) string {
	if jsonStr == "" {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return redactJSONRegex(jsonStr)
	}
	redacted := redactMap(data)
	result, _ := json.Marshal(redacted)
	return string(result)
}

// redactJSONRegex is the kernel's fallback for content that is not a JSON
// object.
func redactJSONRegex(jsonStr string) string {
	for _, key := range sensitiveKeys {
		pattern := `("` + key + `"\s*:\s*")([^"]+)(")`
		re := regexp.MustCompile("(?i)" + pattern)
		jsonStr = re.ReplaceAllString(jsonStr, `$1[REDACTED]$3`)
	}
	return jsonStr
}

func auditLogResponse(entry *AuditLog) map[string]any {
	return map[string]any{
		"id":          entry.ID,
		"user_id":     entry.UserID,
		"username":    entry.Username,
		"action":      entry.Action,
		"module":      entry.Module,
		"target_type": entry.TargetType,
		"target_id":   entry.TargetID,
		"content":     sanitizeAuditLogContent(entry.Content),
		"ip":          entry.IP,
		"user_agent":  entry.UserAgent,
		"status":      entry.Status,
		"created_at":  entry.CreatedAt,
	}
}

// GetAuditLogs is GET /api/v2/admin/system/audit-logs: the system audit
// trail, newest first, by target type and action.
func (s *Service) GetAuditLogs(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	page, _ := strconv.Atoi(defaultQuery(request, "page", "1"))
	pageSize, _ := strconv.Atoi(defaultQuery(request, "page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > maxAuditLogPageSize {
		pageSize = maxAuditLogPageSize
	}

	targetType := strings.TrimSpace(query(request, "target_type"))
	action := strings.TrimSpace(query(request, "action"))

	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	// The view holds only module "system", which the kernel filters on.
	logs := db.Model(&AuditLog{})
	if targetType != "" {
		logs = logs.Where("target_type = ?", targetType)
	}
	if action != "" {
		logs = logs.Where("action = ?", action)
	}

	var total int64
	if err := logs.Count(&total).Error; err != nil {
		return s.panelError(err.Error())
	}

	var records []AuditLog
	offset := (page - 1) * pageSize
	if err := logs.Order("id DESC").Limit(pageSize).Offset(offset).Find(&records).Error; err != nil {
		return s.panelError(err.Error())
	}

	list := make([]map[string]any, 0, len(records))
	for i := range records {
		list = append(list, auditLogResponse(&records[i]))
	}

	return s.panel(map[string]any{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
