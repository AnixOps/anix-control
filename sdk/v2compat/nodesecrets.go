package v2compat

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// NodeSecretPlaceholder stands for a node secret in administrators'
// answers: a secret setting of a node protocol or of a node's raw
// configuration. A finalized node table keeps it at every secret position
// of its JSON columns (docs/architecture/node-ops-service.md section 4.4).
const NodeSecretPlaceholder = "********"

// IsNodeSecretKey reports whether a key of a node protocol's settings, or of
// a node's raw configuration, names a secret: Reality's and TLS's
// private_key (privateKey), WireGuard's server_private_key and
// preshared_key, Shadowsocks' server_key, psk and password, the Hysteria2
// obfs-password and auth, and any other *_key, password, passwd, pass,
// secret, token, credential or seed. Public keys, Reality's short_id and the
// paths of key files (key_file, wss_key_file) are not secrets.
//
// It is the kernel's rule (internal/nodesecrets.IsSecretKey calls it), so a
// package masks exactly what the kernel's answers mask.
func IsNodeSecretKey(key string) bool {
	compact := strings.ToLower(strings.TrimSpace(key))
	compact = strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(compact)
	if compact == "" {
		return false
	}
	if strings.Contains(compact, "public") || strings.Contains(compact, "publishable") ||
		compact == "shortid" || compact == "shortids" ||
		strings.HasSuffix(compact, "file") || strings.HasSuffix(compact, "path") {
		return false
	}
	switch compact {
	case "psk", "pass", "auth", "authstr", "seed":
		return true
	}
	if strings.HasSuffix(compact, "key") || strings.HasSuffix(compact, "keys") {
		return true
	}
	for _, marker := range []string{"secret", "password", "passwd", "token", "credential"} {
		if strings.Contains(compact, marker) {
			return true
		}
	}
	return false
}

// RedactNodeSecrets replaces the secret values of a node protocol setting
// or a raw configuration (JSON text) with NodeSecretPlaceholder: a non-empty
// string or array under a secret key is replaced whole, an object's own
// keys are checked, and so are the objects in arrays. A document without a
// secret is returned unchanged, and one that is not JSON is replaced whole,
// since it cannot be told apart. A blank document is returned as it is.
//
// It is the kernel's masked answer (internal/nodesecrets.Redact calls it)
// and the document a finalized node table keeps, and redacting that
// document again answers the same bytes.
func RedactNodeSecrets(document string) string {
	if strings.TrimSpace(document) == "" {
		return document
	}
	value, ok := decodeNodeSecretDocument(document)
	if !ok {
		return NodeSecretPlaceholder
	}
	if !redactNodeSecretValue(value) {
		return document
	}
	encoded := encodeNodeSecretDocument(value)
	if encoded == "" {
		return NodeSecretPlaceholder
	}
	return encoded
}

// redactNodeSecretValue replaces the secrets of value in place; it reports
// whether it replaced any.
func redactNodeSecretValue(value any) bool {
	changed := false
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if object, ok := item.(map[string]any); ok {
				changed = redactNodeSecretValue(object) || changed
				continue
			}
			if IsNodeSecretKey(key) && HasNodeSecretValue(item) {
				typed[key] = NodeSecretPlaceholder
				changed = true
				continue
			}
			changed = redactNodeSecretValue(item) || changed
		}
	case []any:
		for _, item := range typed {
			changed = redactNodeSecretValue(item) || changed
		}
	}
	return changed
}

// HasNodeSecretValue reports whether a decoded value under a secret key
// holds a secret: a non-empty string or array. Empty values, null, numbers
// and booleans are not secrets.
func HasNodeSecretValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	default:
		return false
	}
}

// decodeNodeSecretDocument decodes exactly one JSON value, keeping numbers
// as written.
func decodeNodeSecretDocument(document string) (any, bool) {
	decoder := json.NewDecoder(strings.NewReader(document))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return value, true
}

func encodeNodeSecretDocument(value any) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		// Decoded JSON always encodes again.
		return ""
	}
	return strings.TrimSuffix(buffer.String(), "\n")
}
