package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/service"
)

// auditRedacted replaces a credential in an audited request body.
const auditRedacted = "[REDACTED]"

// auditCaptureLimit bounds the request body captured for redaction. A longer
// body is not valid JSON once cut, so it is recorded only by its size.
const auditCaptureLimit = 64 << 10

// auditSecretMarkers are the field-name fragments that mark a credential:
// passwords, tokens, API and private keys, gateway secrets, subscription
// UUIDs. Matching is by substring, so a new field such as "stripe_secret_key"
// is covered without being listed.
var auditSecretMarkers = []string{
	"password", "passwd", "secret", "token", "private", "credential",
	"key", "uuid", "cookie", "authorization",
}

// auditPublicFields are key-like names that hold no secret.
var auditPublicFields = map[string]bool{
	"public_key":         true,
	"reality_public_key": true,
	"publishable_key":    true,
	"key_id":             true,
	"issuer_key_id":      true,
	"trust_root_key_id":  true,
	"idempotency_key":    true,
	"key_path":           true,
}

func auditSecretField(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if auditPublicFields[normalized] {
		return false
	}
	for _, marker := range auditSecretMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

// redactAuditBody returns the request body as the audit trail may keep it:
// JSON with every credential replaced, or only its size when it is not JSON.
// The last path segment names the system setting of a
// PUT /admin/system/configs/:key, whose "value" is then a secret when the
// setting is sensitive.
func redactAuditBody(body []byte, path string) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil || decoder.More() {
		return fmt.Sprintf("[body omitted: %d bytes, not JSON]", len(body))
	}
	if object, ok := value.(map[string]any); ok {
		segment := path[strings.LastIndex(path, "/")+1:]
		if _, has := object["value"]; has && service.IsSensitiveSystemConfigKey(segment) {
			object["value"] = auditRedacted
		}
	}
	encoded, err := json.Marshal(redactAuditValue(value))
	if err != nil {
		return fmt.Sprintf("[body omitted: %d bytes]", len(body))
	}
	return string(encoded)
}

func redactAuditValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		// A {"key": ..., "value": ...} pair is a named setting: its name stays
		// readable and its value is a secret when the name is sensitive.
		name, isSetting := typed["key"].(string)
		_, hasValue := typed["value"]
		isSetting = isSetting && hasValue
		for field, item := range typed {
			switch {
			case isSetting && field == "key":
			case isSetting && field == "value":
				if service.IsSensitiveSystemConfigKey(name) {
					typed[field] = auditRedacted
				} else {
					typed[field] = redactAuditValue(item)
				}
			case auditSecretField(field):
				typed[field] = auditRedacted
			default:
				typed[field] = redactAuditValue(item)
			}
		}
		return typed
	case []any:
		for index, item := range typed {
			typed[index] = redactAuditValue(item)
		}
		return typed
	case string:
		// Payment gateways send their settings as a JSON string.
		inner := strings.TrimSpace(typed)
		if !strings.HasPrefix(inner, "{") && !strings.HasPrefix(inner, "[") {
			return typed
		}
		decoder := json.NewDecoder(strings.NewReader(inner))
		decoder.UseNumber()
		var nested any
		if err := decoder.Decode(&nested); err != nil || decoder.More() {
			return typed
		}
		encoded, err := json.Marshal(redactAuditValue(nested))
		if err != nil {
			return auditRedacted
		}
		return string(encoded)
	default:
		return typed
	}
}
