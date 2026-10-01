// Package v2compat holds the /api/v2 response conventions shared by the
// kernel's legacy handlers and package-native implementations, so both
// produce byte-identical output.
package v2compat

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

// PanelSuccessMessage is the msg of a successful panel envelope.
const PanelSuccessMessage = "操作成功"

// Panel is the Flux-compatible panel envelope {code, data, msg, ts}. Its field
// order is the alphabetical key order the kernel's map-based envelope had, so
// marshalling it produces the same bytes.
type Panel struct {
	Code int    `json:"code"`
	Data any    `json:"data"`
	Msg  string `json:"msg"`
	// TS is the server time in Unix milliseconds.
	TS int64 `json:"ts"`
}

// PanelSuccess wraps data in a successful envelope.
func PanelSuccess(data any, now time.Time) Panel {
	return Panel{Code: 0, Data: data, Msg: PanelSuccessMessage, TS: now.UnixMilli()}
}

// PanelError is a failed envelope; the HTTP status stays 200, as in the
// legacy handlers.
func PanelError(msg string, now time.Time) Panel {
	return Panel{Code: -1, Data: nil, Msg: msg, TS: now.UnixMilli()}
}

// SealedHandlePrefix starts a sealed secret handle: an opaque stand-in the
// kernel's gateway puts where a node secret was in an administrator's
// request, and that a package puts where a secret the kernel generated is
// shown once in its answer (docs/architecture/node-ops-service.md section
// 3.7). A handle is SealedHandlePrefix and 43 characters of unpadded
// base64url; it resolves only in the kernel, for the request it was sealed
// for.
const SealedHandlePrefix = "anix-sealed:v1:"

// SealedHandleLength is the length of every sealed handle.
const SealedHandleLength = len(SealedHandlePrefix) + 43

// SealedHandleMask is what a sealed handle compares as in
// NormalizeForCompare.
const SealedHandleMask = SealedHandlePrefix + "*"

// IsSealedHandle reports whether value is a sealed handle.
func IsSealedHandle(value string) bool {
	if len(value) != SealedHandleLength || !strings.HasPrefix(value, SealedHandlePrefix) {
		return false
	}
	for _, character := range value[len(SealedHandlePrefix):] {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

// volatileKeys are top-level response keys that differ between two correct
// responses and are ignored by NormalizeForCompare.
var volatileKeys = []string{"ts"}

// NormalizeForCompare returns a canonical form of a response body for shadow
// comparison: JSON is re-encoded with sorted keys, without volatile
// top-level keys and with every sealed handle replaced by SealedHandleMask;
// other bodies are returned unchanged.
func NormalizeForCompare(body []byte) []byte {
	value, ok := decodeForCompare(body)
	if !ok {
		return body
	}
	encoded, err := json.Marshal(maskHandles(value))
	if err != nil {
		return body
	}
	return encoded
}

// EqualForCompare reports whether two response bodies are equal after
// NormalizeForCompare, where a sealed handle on one side also matches any
// string on the other: a native answer holds a handle where the legacy
// answer shows the secret it stands for, so answers that differ only in
// handles compare equal.
func EqualForCompare(left, right []byte) bool {
	leftValue, leftOK := decodeForCompare(left)
	rightValue, rightOK := decodeForCompare(right)
	if !leftOK || !rightOK {
		return bytes.Equal(NormalizeForCompare(left), NormalizeForCompare(right))
	}
	leftValue, rightValue = maskHandlePairs(leftValue, rightValue)
	leftEncoded, leftErr := json.Marshal(maskHandles(leftValue))
	rightEncoded, rightErr := json.Marshal(maskHandles(rightValue))
	if leftErr != nil || rightErr != nil {
		return bytes.Equal(NormalizeForCompare(left), NormalizeForCompare(right))
	}
	return bytes.Equal(leftEncoded, rightEncoded)
}

// decodeForCompare decodes one JSON object or array, without the volatile
// top-level keys. Numbers keep their spelling.
func decodeForCompare(body []byte) (any, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil || decoder.More() {
		return nil, false
	}
	if object, ok := value.(map[string]any); ok {
		for _, key := range volatileKeys {
			delete(object, key)
		}
	}
	return value, true
}

// maskHandles replaces every sealed handle in value with SealedHandleMask.
func maskHandles(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			typed[key] = maskHandles(item)
		}
	case []any:
		for index, item := range typed {
			typed[index] = maskHandles(item)
		}
	case string:
		if IsSealedHandle(typed) {
			return SealedHandleMask
		}
	}
	return value
}

// maskHandlePairs walks two documents together and masks both sides of
// every position where one side is a sealed handle and the other a string.
func maskHandlePairs(left, right any) (any, any) {
	switch leftTyped := left.(type) {
	case map[string]any:
		if rightTyped, ok := right.(map[string]any); ok {
			for key, leftItem := range leftTyped {
				if rightItem, present := rightTyped[key]; present {
					leftTyped[key], rightTyped[key] = maskHandlePairs(leftItem, rightItem)
				}
			}
		}
	case []any:
		if rightTyped, ok := right.([]any); ok {
			for index := 0; index < len(leftTyped) && index < len(rightTyped); index++ {
				leftTyped[index], rightTyped[index] = maskHandlePairs(leftTyped[index], rightTyped[index])
			}
		}
	case string:
		if rightTyped, ok := right.(string); ok && (IsSealedHandle(leftTyped) || IsSealedHandle(rightTyped)) {
			return SealedHandleMask, SealedHandleMask
		}
	}
	return left, right
}
