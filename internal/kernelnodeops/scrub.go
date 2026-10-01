package kernelnodeops

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// minScrubbedSecret is the shortest credential value scrubbed by value; a
// shorter one would mask unrelated text, and no generated credential is
// that short.
const minScrubbedSecret = 4

var (
	// keyBeforeValue is a key and its separator in free text: "key": as
	// JSON writes it, key= and key: as query strings, headers, logs and
	// YAML write them. The value is read after it.
	keyBeforeValue = regexp.MustCompile(`(?:"([^"\\]{1,64})"|([A-Za-z0-9_.\-]{1,64}))\s*[:=]\s*`)
	// authorizationValue is an Authorization header's credentials.
	authorizationValue = regexp.MustCompile(`(?i)\b(bearer|basic)(\s+)([A-Za-z0-9._~+/=\-]{4,})`)
	// urlPassword is the password of a URL's user information.
	urlPassword = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://[^/\s:@]*:)([^@\s/]+)(@)`)
)

// scrubText replaces every credential an operation used, and every value
// that IsNodeSecretKey marks, with the placeholder: free text from a node,
// NodeX or Ansible can echo a token (section 3.7). A JSON document is
// scrubbed by key and in every string it holds.
func scrubText(text string, secrets []string) string {
	if text == "" {
		return text
	}
	text = maskKnown(text, secrets)
	if trimmed := strings.TrimSpace(text); strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if value, err := decodeJSON([]byte(trimmed)); err == nil {
			scrubbed, changed := scrubJSON(value, false)
			if !changed {
				return text
			}
			if encoded, err := encodeJSON(scrubbed); err == nil {
				return encoded
			}
			return service.NodeSecretPlaceholder
		}
	}
	return scrubFree(text)
}

// scrubFree masks the values of secret keys, authorization values and URL
// passwords in free text.
func scrubFree(text string) string {
	var out strings.Builder
	last := 0
	for _, match := range keyBeforeValue.FindAllStringSubmatchIndex(text, -1) {
		if match[0] < last {
			continue
		}
		key := ""
		if match[2] >= 0 {
			key = text[match[2]:match[3]]
		} else {
			key = text[match[4]:match[5]]
		}
		if !service.IsNodeSecretKey(key) {
			continue
		}
		valueEnd := valueSpan(text, match[1])
		value := text[match[1]:valueEnd]
		unquoted := strings.Trim(value, `"'`)
		if unquoted == "" || unquoted == service.NodeSecretPlaceholder {
			continue
		}
		masked := service.NodeSecretPlaceholder
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') {
			masked = string(value[0]) + masked + string(value[0])
		}
		out.WriteString(text[last:match[1]])
		out.WriteString(masked)
		last = valueEnd
	}
	out.WriteString(text[last:])
	text = authorizationValue.ReplaceAllString(out.String(), "${1}${2}"+service.NodeSecretPlaceholder)
	return urlPassword.ReplaceAllString(text, "${1}"+service.NodeSecretPlaceholder+"${3}")
}

// valueSpan returns where the value starting at start ends: after its
// closing quote, or at the first delimiter of a bare value.
func valueSpan(text string, start int) int {
	if start >= len(text) {
		return start
	}
	switch quote := text[start]; quote {
	case '"', '\'':
		for i := start + 1; i < len(text); i++ {
			switch text[i] {
			case '\\':
				if quote == '"' {
					i++
				}
			case quote:
				return i + 1
			}
		}
		return len(text)
	}
	end := start
	for end < len(text) && !strings.ContainsRune(" \t\r\n,;&}]\"'", rune(text[end])) {
		end++
	}
	return end
}

// scrubJSON masks a decoded JSON value: under a secret key a string or a
// non-empty array is replaced whole, as RedactNodeSecretsJSON does, and
// every other string is scrubbed as free text (it may itself be JSON).
func scrubJSON(value any, secretKey bool) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		for key, item := range typed {
			scrubbed, itemChanged := scrubJSON(item, service.IsNodeSecretKey(key))
			if itemChanged {
				typed[key], changed = scrubbed, true
			}
		}
		return typed, changed
	case []any:
		if secretKey && len(typed) > 0 {
			return service.NodeSecretPlaceholder, true
		}
		changed := false
		for index, item := range typed {
			scrubbed, itemChanged := scrubJSON(item, false)
			if itemChanged {
				typed[index], changed = scrubbed, true
			}
		}
		return typed, changed
	case string:
		if secretKey {
			if strings.TrimSpace(typed) == "" || typed == service.NodeSecretPlaceholder {
				return typed, false
			}
			return service.NodeSecretPlaceholder, true
		}
		scrubbed := scrubText(typed, nil)
		return scrubbed, scrubbed != typed
	}
	return value, false
}

func encodeJSON(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buffer.String(), "\n"), nil
}

// maskKnown replaces each known credential, as written and in its JSON and
// URL encodings.
func maskKnown(text string, secrets []string) string {
	for _, secret := range secrets {
		if len(secret) < minScrubbedSecret {
			continue
		}
		forms := []string{secret, url.QueryEscape(secret), url.PathEscape(secret)}
		if encoded, err := json.Marshal(secret); err == nil {
			forms = append(forms, strings.Trim(string(encoded), `"`))
		}
		for _, form := range forms {
			if len(form) >= minScrubbedSecret {
				text = strings.ReplaceAll(text, form, service.NodeSecretPlaceholder)
			}
		}
	}
	return text
}

// scrubBytes scrubs a bytes field: a JSON document is redacted by key, then
// any known credential is masked.
func scrubBytes(raw []byte, secrets []string) []byte {
	if len(raw) == 0 {
		return raw
	}
	return []byte(scrubText(string(raw), secrets))
}

// scrubMessage scrubs every string and bytes field of message, in place,
// recursively. SecretHandle messages are left alone: a handle is not a
// secret, and the gateway expands it only in its own request's answer.
func scrubMessage(message protoreflect.Message, secrets []string) {
	if message.Descriptor().FullName() == (&kernelnodeopsv1.SecretHandle{}).ProtoReflect().Descriptor().FullName() {
		return
	}
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		switch {
		case field.IsMap():
		case field.IsList():
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				switch field.Kind() {
				case protoreflect.StringKind:
					list.Set(i, protoreflect.ValueOfString(scrubText(list.Get(i).String(), secrets)))
				case protoreflect.BytesKind:
					list.Set(i, protoreflect.ValueOfBytes(scrubBytes(list.Get(i).Bytes(), secrets)))
				case protoreflect.MessageKind, protoreflect.GroupKind:
					scrubMessage(list.Get(i).Message(), secrets)
				}
			}
		case field.Kind() == protoreflect.StringKind:
			message.Set(field, protoreflect.ValueOfString(scrubText(value.String(), secrets)))
		case field.Kind() == protoreflect.BytesKind:
			message.Set(field, protoreflect.ValueOfBytes(scrubBytes(value.Bytes(), secrets)))
		case field.Kind() == protoreflect.MessageKind || field.Kind() == protoreflect.GroupKind:
			scrubMessage(value.Message(), secrets)
		}
		return true
	})
}

// scrubResult returns a scrubbed copy of result.
func scrubResult(result *kernelnodeopsv1.OperationResult, secrets []string) *kernelnodeopsv1.OperationResult {
	if result == nil {
		return nil
	}
	copied := proto.Clone(result).(*kernelnodeopsv1.OperationResult)
	scrubMessage(copied.ProtoReflect(), secrets)
	return copied
}
