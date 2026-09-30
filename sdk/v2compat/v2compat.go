// Package v2compat holds the /api/v2 response conventions shared by the
// kernel's legacy handlers and package-native implementations, so both
// produce byte-identical output.
package v2compat

import (
	"bytes"
	"encoding/json"
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

// volatileKeys are top-level response keys that differ between two correct
// responses and are ignored by NormalizeForCompare.
var volatileKeys = []string{"ts"}

// NormalizeForCompare returns a canonical form of a response body for shadow
// comparison: JSON is re-encoded with sorted keys and without volatile
// top-level keys; other bodies are returned unchanged.
func NormalizeForCompare(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return body
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil || decoder.More() {
		return body
	}
	if object, ok := value.(map[string]any); ok {
		for _, key := range volatileKeys {
			delete(object, key)
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return encoded
}

// EqualForCompare reports whether two response bodies are equal after
// NormalizeForCompare.
func EqualForCompare(left, right []byte) bool {
	return bytes.Equal(NormalizeForCompare(left), NormalizeForCompare(right))
}
