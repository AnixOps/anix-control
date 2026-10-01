package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
)

const SensitiveSystemConfigPlaceholder = "********"

var sensitiveSystemConfigMarkers = []string{
	"token",
	"secret",
	"password",
	"passwd",
	"private_key",
	"privatekey",
	"api_key",
	"apikey",
	"access_key",
	"accesskey",
	"client_secret",
}

func IsSensitiveSystemConfigKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}

	for _, marker := range sensitiveSystemConfigMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}

	return false
}

// MaskSystemConfigValue is a value as an administrator's answer shows it:
// the placeholder for a sensitive key with a value, "" for one without, and
// any other value with its masked fields masked (MaskSystemConfigFields).
func MaskSystemConfigValue(key, value string) (displayValue string, sensitive bool, hasValue bool) {
	sensitive = IsSensitiveSystemConfigKey(key)
	hasValue = strings.TrimSpace(value) != ""
	if !sensitive {
		return MaskSystemConfigFields(key, value), false, hasValue
	}
	if !hasValue {
		return "", true, false
	}
	return SensitiveSystemConfigPlaceholder, true, true
}

// systemConfigMaskedFields are the fields of system configuration values
// (JSON objects) that hold a secret the key's name does not reveal: the
// SMTP password in the e-mail configuration. Such a key is not sensitive
// as a whole: administrators see and edit its value, with these fields as
// SensitiveSystemConfigPlaceholder. The kernel's own readers (the test
// e-mail) and packages holding the namespace's secrets capability read the
// stored value.
var systemConfigMaskedFields = map[string][]string{
	"notification.email.config": {"password"},
}

// SystemConfigMaskedFields returns the masked fields of key's value, none
// for most keys.
func SystemConfigMaskedFields(key string) []string {
	return append([]string(nil), systemConfigMaskedFields[key]...)
}

// MaskSystemConfigFields returns value, the JSON object stored under key,
// with each of its masked fields that has a value (anything but null and a
// blank string) replaced by SensitiveSystemConfigPlaceholder. The rest of
// the text is kept as it is. A key without masked fields, a blank value
// and a JSON value that is not an object are returned unchanged; a value
// that is not JSON is replaced whole, since its secret cannot be told
// apart.
func MaskSystemConfigFields(key, value string) string {
	fields := systemConfigMaskedFields[key]
	if len(fields) == 0 || strings.TrimSpace(value) == "" {
		return value
	}
	if !json.Valid([]byte(value)) {
		return SensitiveSystemConfigPlaceholder
	}
	members, ok := jsonObjectMembers(value)
	if !ok {
		return value
	}
	placeholder := strconv.Quote(SensitiveSystemConfigPlaceholder)
	replacements := map[int]string{}
	for index, member := range members {
		if slices.Contains(fields, member.name) && maskedFieldHasValue(member.raw) {
			replacements[index] = placeholder
		}
	}
	return spliceJSONMembers(value, members, replacements)
}

// KeepSystemConfigFields returns incoming, a value about to be stored under
// key, in which every masked field sent as SensitiveSystemConfigPlaceholder
// has the value stored before ("" when nothing is stored), so an
// administrator can save what a masked answer showed without retyping its
// secrets. The placeholder sent for the whole value keeps the whole stored
// value. kept reports whether a stored secret was kept. The rest of the
// text is kept as it is.
func KeepSystemConfigFields(key, incoming, stored string) (value string, kept bool) {
	fields := systemConfigMaskedFields[key]
	if len(fields) == 0 {
		return incoming, false
	}
	if incoming == SensitiveSystemConfigPlaceholder {
		return stored, strings.TrimSpace(stored) != ""
	}
	if !strings.Contains(incoming, SensitiveSystemConfigPlaceholder) {
		return incoming, false
	}
	members, ok := jsonObjectMembers(incoming)
	if !ok {
		return incoming, false
	}
	var previous map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stored), &previous); err != nil {
		previous = nil
	}
	replacements := map[int]string{}
	for index, member := range members {
		if !slices.Contains(fields, member.name) || !isPlaceholder(member.raw) {
			continue
		}
		replacement, ok := storedField(previous[member.name])
		if !ok {
			replacements[index] = `""`
			continue
		}
		replacements[index] = replacement
		kept = kept || maskedFieldHasValue(previous[member.name])
	}
	return spliceJSONMembers(incoming, members, replacements), kept
}

// storedField re-encodes a stored field as encoding/json writes it, so a
// kept secret is stored exactly as a handler that decoded and re-encoded
// the configuration stores it.
func storedField(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func isPlaceholder(raw json.RawMessage) bool {
	var text string
	return json.Unmarshal(raw, &text) == nil && text == SensitiveSystemConfigPlaceholder
}

// maskedFieldHasValue reports whether a masked field holds something to
// hide: anything but null and a blank string.
func maskedFieldHasValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	var text string
	if json.Unmarshal(trimmed, &text) == nil {
		return strings.TrimSpace(text) != ""
	}
	return true
}

// jsonMember is one member of a JSON object: its name and where its value
// is in the text.
type jsonMember struct {
	name       string
	start, end int
	raw        json.RawMessage
}

// jsonObjectMembers lists the members of text, a JSON object, in order,
// with the position of each value. ok is false when text is not one JSON
// object.
func jsonObjectMembers(text string) (members []jsonMember, ok bool) {
	decoder := json.NewDecoder(strings.NewReader(text))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, false
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		name, isName := token.(string)
		if !isName {
			return nil, false
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, false
		}
		end := int(decoder.InputOffset())
		members = append(members, jsonMember{name: name, start: end - len(raw), end: end, raw: raw})
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return members, true
}

// spliceJSONMembers replaces the values of the members at the given
// indexes, keeping the rest of text.
func spliceJSONMembers(text string, members []jsonMember, replacements map[int]string) string {
	if len(replacements) == 0 {
		return text
	}
	var builder strings.Builder
	last := 0
	for index, member := range members {
		replacement, ok := replacements[index]
		if !ok {
			continue
		}
		builder.WriteString(text[last:member.start])
		builder.WriteString(replacement)
		last = member.end
	}
	builder.WriteString(text[last:])
	return builder.String()
}
