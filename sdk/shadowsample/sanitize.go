// Package shadowsample builds and sanitizes the samples a package host keeps
// of shadow-mode mismatches: what differed between a route's legacy and
// native answer, with every secret and personal value masked.
//
// The package host sanitizes a sample before it leaves the process (in the
// Health details document), and the kernel sanitizes it again before storing
// it, so a host built with an older or modified SDK cannot store an
// unmasked value.
//
// Masking rules:
//
//   - Every value under a key whose name matches SensitiveKey (tokens,
//     secrets, passwords, hashes, keys, UUIDs, signatures, cookies,
//     authorization, auth data, subscription URLs, ...) becomes Mask, whatever
//     its type.
//   - Strings that look like a secret become Mask whatever their key: JWTs,
//     UUIDs, password hashes, PEM private keys, and long hex or base64 runs.
//     Inside longer text only the matching run is replaced.
//   - URLs keep their scheme and host only (https://example.com/***); a URL
//     with user information or another scheme than http(s), for example a
//     vless:// or ss:// share link, keeps only its scheme.
//   - E-mail addresses keep their first character and their domain
//     (a***@example.com).
//   - IPv4 addresses keep their first two octets (10.2.*.*), IPv6 addresses
//     their first two hextets (2001:db8:*:*:*:*:*:*).
//   - Strings are cut to MaxStringBytes.
package shadowsample

import (
	"encoding/json"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Mask replaces a masked value.
const Mask = "***"

// MaxStringBytes bounds a sanitized string.
const MaxStringBytes = 256

// SensitiveKey matches the names of fields whose values are always masked.
var SensitiveKey = regexp.MustCompile(`(?i)token|secret|passw|pwd|key|uuid|sign|cookie|authori[sz]|auth|hash|salt|credential|private|session|subscri(be|ption)_?(url|link)|sub_?url|otp|nonce`)

var (
	jwtPattern        = regexp.MustCompile(`[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)
	uuidPattern       = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	hexPattern        = regexp.MustCompile(`(?i)[0-9a-f]{32,}`)
	urlSafePattern    = regexp.MustCompile(`[A-Za-z0-9_-]{32,}={0,2}`)
	base64Pattern     = regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)
	emailPattern      = regexp.MustCompile(`([A-Za-z0-9._%+-])[A-Za-z0-9._%+-]*@([A-Za-z0-9.-]+\.[A-Za-z]{2,})`)
	ipv4Pattern       = regexp.MustCompile(`\b(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\b`)
	ipv6Pattern       = regexp.MustCompile(`[0-9A-Fa-f:]*:[0-9A-Fa-f]*:[0-9A-Fa-f:.]*`)
	urlPattern        = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]{1,15}://[^\s"'<>]+`)
	bearerPattern     = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[^\s"',;]+`)
	passwordHash      = regexp.MustCompile(`^\$(2[abxy]?|argon2(id|i|d)|[156]|scrypt|pbkdf2[a-z0-9-]*)\$`)
	privateKeyPattern = regexp.MustCompile(`(?i)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)
	maskedURLPattern  = regexp.MustCompile(`^https?://(\[[0-9a-f:*]+\]|\d{1,3}\.\d{1,3}\.\*\.\*)(:\d+)?(/\*\*\*)?$`)
)

// IsSensitiveKey reports whether values under name are always masked.
func IsSensitiveKey(name string) bool {
	return SensitiveKey.MatchString(name)
}

// SanitizeValue masks a decoded JSON value found under key ("" for none).
// Objects and arrays are sanitized recursively; numbers, booleans and null
// are kept unless key is sensitive.
func SanitizeValue(key string, value any) any {
	if key != "" && IsSensitiveKey(key) {
		return Mask
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		keys := make([]string, 0, len(typed))
		for name := range typed {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			safe := uniqueKey(out, SanitizeKey(name))
			out[safe] = SanitizeValue(name, typed[name])
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = SanitizeValue("", item)
		}
		return out
	case string:
		return SanitizeString(typed)
	default:
		return value
	}
}

func uniqueKey(object map[string]any, key string) string {
	if _, taken := object[key]; !taken {
		return key
	}
	for index := 2; ; index++ {
		candidate := key + "~" + strconv.Itoa(index)
		if _, taken := object[candidate]; !taken {
			return candidate
		}
	}
}

// SanitizeKey masks an object key that itself looks like a secret or a
// personal value (a map keyed by UUID, token or e-mail address). Field names
// are kept.
func SanitizeKey(key string) string {
	return SanitizeString(key)
}

// SanitizeString masks secrets and personal values in a string.
func SanitizeString(value string) string {
	if value == "" {
		return value
	}
	if privateKeyPattern.MatchString(value) || passwordHash.MatchString(value) {
		return Mask
	}
	trimmed := strings.TrimSpace(value)
	if isWhole(uuidPattern, trimmed) || isWhole(jwtPattern, trimmed) {
		return Mask
	}
	value = urlPattern.ReplaceAllStringFunc(value, sanitizeURL)
	value = bearerPattern.ReplaceAllStringFunc(value, func(match string) string {
		fields := strings.Fields(match)
		return fields[0] + " " + Mask
	})
	value = jwtPattern.ReplaceAllString(value, Mask)
	value = uuidPattern.ReplaceAllString(value, Mask)
	value = emailPattern.ReplaceAllString(value, "$1***@$2")
	value = ipv6Pattern.ReplaceAllStringFunc(value, maskIPv6)
	value = ipv4Pattern.ReplaceAllStringFunc(value, maskIPv4)
	value = hexPattern.ReplaceAllString(value, Mask)
	value = urlSafePattern.ReplaceAllStringFunc(value, func(match string) string {
		if hasDigitAndLetter(match) {
			return Mask
		}
		return match
	})
	value = base64Pattern.ReplaceAllStringFunc(value, func(match string) string {
		if hasDigitAndLetter(match) && hasUpperAndLower(match) {
			return Mask
		}
		return match
	})
	return truncate(value, MaxStringBytes)
}

func isWhole(pattern *regexp.Regexp, value string) bool {
	location := pattern.FindStringIndex(value)
	return location != nil && location[0] == 0 && location[1] == len(value)
}

func hasDigitAndLetter(value string) bool {
	digit, letter := false, false
	for _, character := range value {
		switch {
		case character >= '0' && character <= '9':
			digit = true
		case (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z'):
			letter = true
		}
	}
	return digit && letter
}

func hasUpperAndLower(value string) bool {
	upper, lower := false, false
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
			lower = true
		case character >= 'A' && character <= 'Z':
			upper = true
		}
	}
	return upper && lower
}

// sanitizeURL keeps the scheme and host of an http(s) URL, and only the
// scheme of any other URL or of one with user information.
func sanitizeURL(raw string) string {
	if maskedURLPattern.MatchString(raw) {
		// Already sanitized: an IPv6 host with masked hextets does not parse.
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return Mask
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.User != nil || parsed.Host == "" {
		return scheme + "://" + Mask
	}
	host := parsed.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		host = maskIP(ip)
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
	} else {
		host = emailPattern.ReplaceAllString(host, "$1***@$2")
	}
	out := scheme + "://" + host
	if port := parsed.Port(); port != "" {
		out += ":" + port
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		out += "/" + Mask
	}
	return out
}

func maskIPv4(match string) string {
	ip := net.ParseIP(match)
	if ip == nil || ip.To4() == nil {
		return match
	}
	return maskIP(ip)
}

func maskIPv6(match string) string {
	candidate := strings.Trim(match, ":.")
	if strings.HasPrefix(match, "::") {
		candidate = "::" + strings.TrimLeft(candidate, ":")
	}
	if strings.HasSuffix(match, "::") {
		candidate = strings.TrimRight(candidate, ":") + "::"
	}
	ip := net.ParseIP(candidate)
	if ip == nil || ip.To4() != nil && !strings.Contains(candidate, ":") {
		return match
	}
	return strings.Replace(match, candidate, maskIP(ip), 1)
}

// maskIP keeps the first two octets of an IPv4 address and the first two
// hextets of an IPv6 address.
func maskIP(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return strconv.Itoa(int(v4[0])) + "." + strconv.Itoa(int(v4[1])) + ".*.*"
	}
	v6 := ip.To16()
	if v6 == nil {
		return Mask
	}
	first := uint(v6[0])<<8 | uint(v6[1])
	second := uint(v6[2])<<8 | uint(v6[3])
	return strconv.FormatUint(uint64(first), 16) + ":" + strconv.FormatUint(uint64(second), 16) + ":*:*:*:*:*:*"
}

// SanitizeIP masks one address (IPv4 keeps two octets, IPv6 two hextets);
// anything that is not an address is sanitized as a string.
func SanitizeIP(value string) string {
	if ip := net.ParseIP(strings.TrimSpace(value)); ip != nil {
		return maskIP(ip)
	}
	return SanitizeString(value)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + "…"
}

// SanitizeJSON sanitizes a JSON document. A body that is not JSON is never
// kept: it is described by its size only.
func SanitizeJSON(raw []byte) json.RawMessage {
	value, ok := decode(raw)
	if !ok {
		encoded, _ := json.Marshal(nonJSONSummary(raw))
		return encoded
	}
	encoded, err := json.Marshal(SanitizeValue("", value))
	if err != nil {
		return json.RawMessage(`"` + Mask + `"`)
	}
	return encoded
}

func nonJSONSummary(raw []byte) string {
	return "<non-JSON body, " + strconv.Itoa(len(raw)) + " bytes>"
}

// SanitizePath returns the request path with the values of path parameters
// replaced by their {name}, every other segment sanitized, and every query
// value masked (keys are kept, sorted).
func SanitizePath(path string, params map[string]string, query map[string][]string) string {
	rawPath, rawQuery, hasQuery := strings.Cut(path, "?")
	byValue := make(map[string]string, len(params))
	for name, value := range params {
		if value != "" {
			byValue[value] = name
		}
	}
	segments := strings.Split(rawPath, "/")
	for index, segment := range segments {
		if segment == "" {
			continue
		}
		if decoded, err := url.PathUnescape(segment); err == nil {
			segment = decoded
		}
		if name, ok := byValue[segment]; ok {
			segments[index] = "{" + SanitizeKey(name) + "}"
			continue
		}
		segments[index] = SanitizeString(segment)
	}
	out := strings.Join(segments, "/")
	keys := map[string]struct{}{}
	for name := range query {
		keys[name] = struct{}{}
	}
	if hasQuery {
		if values, err := url.ParseQuery(rawQuery); err == nil {
			for name := range values {
				keys[name] = struct{}{}
			}
		}
	}
	return truncate(out+MaskedQuery(keys), 512)
}

// MaskedQuery renders query keys with masked values: "?a=***&b=***".
func MaskedQuery(keys map[string]struct{}) string {
	if len(keys) == 0 {
		return ""
	}
	names := make([]string, 0, len(keys))
	for name := range keys {
		names = append(names, SanitizeKey(name))
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+Mask)
	}
	return "?" + strings.Join(parts, "&")
}

// QueryKeys returns the query keys of a sanitized path.
func QueryKeys(path string) map[string]struct{} {
	_, rawQuery, ok := strings.Cut(path, "?")
	if !ok {
		return nil
	}
	keys := map[string]struct{}{}
	for _, part := range strings.Split(rawQuery, "&") {
		name, _, _ := strings.Cut(part, "=")
		if name = strings.TrimSpace(name); name != "" {
			keys[name] = struct{}{}
		}
	}
	return keys
}
