package shadowsample

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/sdk/v2compat"
)

// Limits of one sample.
const (
	// MaxDiffBytes bounds the encoded diff of a sample.
	MaxDiffBytes = 8 << 10
	// MaxDiffEntries bounds the number of differing paths in a sample.
	MaxDiffEntries = 64
	// maxValueBytes bounds the encoded value of one side of a diff entry;
	// a larger object or array is described by its size.
	maxValueBytes = 512
	maxPathBytes  = 512
)

// Kinds of a diff entry.
const (
	DiffChanged    = "changed"
	DiffType       = "type"
	DiffLegacyOnly = "legacy_only"
	DiffNativeOnly = "native_only"
	DiffBody       = "body"
)

// DiffEntry is one JSON path whose value differs between the legacy and the
// native answer, with both sides sanitized.
type DiffEntry struct {
	Path   string          `json:"path"`
	Kind   string          `json:"kind"`
	Legacy json.RawMessage `json:"legacy,omitempty"`
	Native json.RawMessage `json:"native,omitempty"`
}

// Sample is one shadow mismatch. Package hosts report recent samples in the
// shadow_samples list of their Health details document.
type Sample struct {
	// ID is 32 lowercase hex characters, unique per sample; the kernel
	// stores each sample once.
	ID      string `json:"id"`
	RouteID string `json:"route_id"`
	Method  string `json:"method"`
	// Path is the sanitized request path (SanitizePath): query values are
	// always masked.
	Path          string      `json:"path,omitempty"`
	LegacyStatus  uint32      `json:"legacy_status"`
	NativeStatus  uint32      `json:"native_status"`
	Diff          []DiffEntry `json:"diff"`
	DiffTruncated bool        `json:"diff_truncated,omitempty"`
	RequestID     string      `json:"request_id,omitempty"`
	// ObservedAt is the Unix time of the comparison.
	ObservedAt int64 `json:"observed_at_unix"`
}

var (
	sampleIDPattern  = regexp.MustCompile(`^[0-9a-f]{32}$`)
	routeIDPattern   = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,191}$`)
	methodPattern    = regexp.MustCompile(`^[A-Z]{1,16}$`)
	requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
)

// ValidID reports whether id is a sample id.
func ValidID(id string) bool { return sampleIDPattern.MatchString(id) }

// ValidRouteID reports whether id may be a package route id.
func ValidRouteID(id string) bool { return routeIDPattern.MatchString(id) }

// Sanitize returns the sample with every field sanitized and the diff within
// its limits. It is idempotent: the kernel applies it again to what a host
// reports.
func Sanitize(sample Sample) Sample {
	out := Sample{
		ID: sample.ID, RouteID: sample.RouteID, LegacyStatus: sample.LegacyStatus, NativeStatus: sample.NativeStatus,
		ObservedAt: sample.ObservedAt, DiffTruncated: sample.DiffTruncated,
	}
	if method := strings.ToUpper(strings.TrimSpace(sample.Method)); methodPattern.MatchString(method) {
		out.Method = method
	}
	if sample.Path != "" {
		path, _, _ := strings.Cut(sample.Path, "?")
		segments := strings.Split(path, "/")
		for index, segment := range segments {
			if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
				segments[index] = "{" + SanitizeKey(strings.Trim(segment, "{}")) + "}"
				continue
			}
			segments[index] = SanitizeString(segment)
		}
		out.Path = truncate(strings.Join(segments, "/")+MaskedQuery(QueryKeys(sample.Path)), maxPathBytes)
	}
	out.RequestID = SanitizeRequestID(sample.RequestID)
	out.Diff = []DiffEntry{}
	budget := MaxDiffBytes
	for _, entry := range sample.Diff {
		if len(out.Diff) >= MaxDiffEntries {
			out.DiffTruncated = true
			break
		}
		clean, ok := sanitizeEntry(entry)
		if !ok {
			continue
		}
		size := entrySize(clean)
		if size > budget {
			out.DiffTruncated = true
			break
		}
		budget -= size
		out.Diff = append(out.Diff, clean)
	}
	return out
}

// SanitizeRequestID keeps a request id made of safe characters, masking one
// that looks like a token. Request ids are kept to correlate a sample with
// the kernel's logs.
func SanitizeRequestID(id string) string {
	id = strings.TrimSpace(id)
	if id == Mask {
		return Mask
	}
	if id == "" || !requestIDPattern.MatchString(id) {
		return ""
	}
	if isWhole(uuidPattern, id) {
		// A UUID request id is the kernel's own correlation id.
		return strings.ToLower(id)
	}
	if SanitizeString(id) != id {
		return Mask
	}
	return id
}

func validKind(kind string) bool {
	switch kind {
	case DiffChanged, DiffType, DiffLegacyOnly, DiffNativeOnly, DiffBody:
		return true
	}
	return false
}

func sanitizeEntry(entry DiffEntry) (DiffEntry, bool) {
	if !validKind(entry.Kind) {
		return DiffEntry{}, false
	}
	path := truncate(SanitizeString(strings.TrimSpace(entry.Path)), maxPathBytes)
	if path == "" {
		path = "$"
	}
	key := lastKey(entry.Path)
	out := DiffEntry{Path: path, Kind: entry.Kind}
	out.Legacy = sanitizeRaw(key, entry.Legacy)
	out.Native = sanitizeRaw(key, entry.Native)
	return out, true
}

// lastKey is the object key that ends a diff path ("" for an array index or
// the root). A sensitive key anywhere in the path masks the value.
func lastKey(path string) string {
	for _, part := range splitPath(path) {
		if IsSensitiveKey(part) {
			return part
		}
	}
	return ""
}

func splitPath(path string) []string {
	return strings.FieldsFunc(path, func(character rune) bool {
		return character == '.' || character == '[' || character == ']' || character == '"' || character == '$'
	})
}

func sanitizeRaw(key string, raw json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	value, ok := decode(raw)
	if !ok {
		var scalar any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&scalar) != nil || decoder.More() {
			return mustMarshal(Mask)
		}
		value = scalar
	}
	return renderValue(SanitizeValue(key, value))
}

func renderValue(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return mustMarshal(Mask)
	}
	if len(encoded) <= maxValueBytes {
		return encoded
	}
	switch typed := value.(type) {
	case map[string]any:
		return mustMarshal("<object, " + strconv.Itoa(len(typed)) + " keys>")
	case []any:
		return mustMarshal("<array, " + strconv.Itoa(len(typed)) + " items>")
	default:
		return mustMarshal("<value, " + strconv.Itoa(len(encoded)) + " bytes>")
	}
}

func mustMarshal(value string) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func entrySize(entry DiffEntry) int {
	encoded, err := json.Marshal(entry)
	if err != nil {
		return MaxDiffBytes + 1
	}
	return len(encoded) + 1
}

// decode decodes one JSON object or array, keeping number spellings.
func decode(raw []byte) (any, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil || decoder.More() {
		return nil, false
	}
	return value, true
}

// volatileKeys are the top-level keys the shadow comparison ignores.
var volatileKeys = []string{"ts"}

// Diff compares a legacy and a native response body structurally and returns
// the differing JSON paths with both sides sanitized, within MaxDiffEntries
// and MaxDiffBytes; truncated reports that differences were left out. Sealed
// handles compare equal to any string, as in the shadow comparison. A body
// that is not JSON is never included: the entry describes both sides by
// their size.
func Diff(legacy, native []byte) (entries []DiffEntry, truncated bool) {
	legacyValue, legacyOK := decode(legacy)
	nativeValue, nativeOK := decode(native)
	builder := &diffBuilder{}
	if !legacyOK || !nativeOK {
		if !bytes.Equal(bytes.TrimSpace(legacy), bytes.TrimSpace(native)) {
			builder.add("$", DiffBody, "", describeBody(legacy, legacyValue, legacyOK), describeBody(native, nativeValue, nativeOK), true, true)
		}
		return builder.result()
	}
	for _, value := range []any{legacyValue, nativeValue} {
		if object, ok := value.(map[string]any); ok {
			for _, key := range volatileKeys {
				delete(object, key)
			}
		}
	}
	builder.walk("$", "", legacyValue, nativeValue)
	return builder.result()
}

func describeBody(raw []byte, value any, ok bool) any {
	if !ok {
		return nonJSONSummary(raw)
	}
	switch typed := value.(type) {
	case map[string]any:
		return "<object, " + strconv.Itoa(len(typed)) + " keys>"
	case []any:
		return "<array, " + strconv.Itoa(len(typed)) + " items>"
	}
	return "<JSON body>"
}

type diffBuilder struct {
	entries   []DiffEntry
	budget    int
	truncated bool
	started   bool
}

func (b *diffBuilder) result() ([]DiffEntry, bool) {
	if b.entries == nil {
		b.entries = []DiffEntry{}
	}
	return b.entries, b.truncated
}

func (b *diffBuilder) full() bool {
	return b.truncated
}

func (b *diffBuilder) add(path, kind, key string, legacy, native any, hasLegacy, hasNative bool) {
	if b.truncated {
		return
	}
	if !b.started {
		b.started, b.budget = true, MaxDiffBytes
	}
	if len(b.entries) >= MaxDiffEntries {
		b.truncated = true
		return
	}
	entry := DiffEntry{Path: truncate(path, maxPathBytes), Kind: kind}
	if hasLegacy {
		entry.Legacy = renderValue(SanitizeValue(key, legacy))
	}
	if hasNative {
		entry.Native = renderValue(SanitizeValue(key, native))
	}
	size := entrySize(entry)
	if size > b.budget {
		b.truncated = true
		return
	}
	b.budget -= size
	b.entries = append(b.entries, entry)
}

func (b *diffBuilder) walk(path, key string, legacy, native any) {
	if b.full() {
		return
	}
	if key != "" && IsSensitiveKey(key) {
		if !equalValues(legacy, native) {
			b.add(path, DiffChanged, key, legacy, native, true, true)
		}
		return
	}
	switch legacyTyped := legacy.(type) {
	case map[string]any:
		nativeTyped, ok := native.(map[string]any)
		if !ok {
			b.add(path, DiffType, key, legacy, native, true, true)
			return
		}
		keys := make([]string, 0, len(legacyTyped)+len(nativeTyped))
		for name := range legacyTyped {
			keys = append(keys, name)
		}
		for name := range nativeTyped {
			if _, seen := legacyTyped[name]; !seen {
				keys = append(keys, name)
			}
		}
		sort.Strings(keys)
		for _, name := range keys {
			child := childPath(path, name)
			legacyItem, inLegacy := legacyTyped[name]
			nativeItem, inNative := nativeTyped[name]
			switch {
			case inLegacy && !inNative:
				b.add(child, DiffLegacyOnly, name, legacyItem, nil, true, false)
			case inNative && !inLegacy:
				b.add(child, DiffNativeOnly, name, nil, nativeItem, false, true)
			default:
				b.walk(child, name, legacyItem, nativeItem)
			}
			if b.full() {
				return
			}
		}
	case []any:
		nativeTyped, ok := native.([]any)
		if !ok {
			b.add(path, DiffType, key, legacy, native, true, true)
			return
		}
		length := max(len(legacyTyped), len(nativeTyped))
		for index := 0; index < length; index++ {
			child := path + "[" + strconv.Itoa(index) + "]"
			switch {
			case index >= len(nativeTyped):
				b.add(child, DiffLegacyOnly, "", legacyTyped[index], nil, true, false)
			case index >= len(legacyTyped):
				b.add(child, DiffNativeOnly, "", nil, nativeTyped[index], false, true)
			default:
				b.walk(child, "", legacyTyped[index], nativeTyped[index])
			}
			if b.full() {
				return
			}
		}
	default:
		if equalValues(legacy, native) {
			return
		}
		kind := DiffChanged
		if jsonKind(legacy) != jsonKind(native) {
			kind = DiffType
		}
		b.add(path, kind, key, legacy, native, true, true)
	}
}

// childPath appends an object key to a diff path; the key is sanitized, and
// quoted when it is not a plain identifier.
func childPath(path, key string) string {
	safe := SanitizeKey(key)
	if plainKey(safe) {
		return path + "." + safe
	}
	quoted, _ := json.Marshal(safe)
	return path + "[" + string(quoted) + "]"
}

func plainKey(key string) bool {
	if key == "" || !utf8.ValidString(key) {
		return false
	}
	for _, character := range key {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' && character != '-' && character != '*' && character != '~' {
			return false
		}
	}
	return true
}

func jsonKind(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case json.Number:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	}
	return "other"
}

func equalValues(legacy, native any) bool {
	legacyString, legacyIsString := legacy.(string)
	nativeString, nativeIsString := native.(string)
	if legacyIsString && nativeIsString && (v2compat.IsSealedHandle(legacyString) || v2compat.IsSealedHandle(nativeString)) {
		return true
	}
	legacyEncoded, legacyErr := json.Marshal(legacy)
	nativeEncoded, nativeErr := json.Marshal(native)
	return legacyErr == nil && nativeErr == nil && bytes.Equal(legacyEncoded, nativeEncoded)
}
