// Package forwardddns is the DNS provider layer of forwarding entry high
// availability (docs/architecture/forward-sdk.md section 7.4, L2): Control
// keeps a route's entry name on the healthy entry nodes' addresses by
// writing A and AAAA records through a DNS provider's API. The providers
// are those of H21: Cloudflare (API token), Alibaba Cloud DNS (AccessKey,
// signature V3 ACS3-HMAC-SHA256), DNSPod (Tencent Cloud API 3.0,
// TC3-HMAC-SHA256), Huawei Cloud DNS (AK/SK, SDK-HMAC-SHA256) and a generic
// webhook (an HMAC-SHA256-signed JSON POST).
//
// Every client is the standard library's HTTP client with hand-written
// request signing; no vendor SDK is linked. A Provider sets a whole record
// set (one name and type): it adds the new values before it deletes the
// old ones, so the name never resolves to nothing while it changes.
// Credentials never appear in an error, a log line or a metric label.
package forwardddns

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Provider kinds (H21).
const (
	KindCloudflare  = "cloudflare"
	KindAliDNS      = "alidns"
	KindDNSPod      = "dnspod"
	KindHuaweiCloud = "huaweicloud"
	KindWebhook     = "webhook"
)

// Record types.
const (
	TypeA     = "A"
	TypeAAAA  = "AAAA"
	TypeCNAME = "CNAME"
)

// ConfigEndpoint overrides a provider API's host; ConfigURL is a webhook's
// URL.
const (
	ConfigEndpoint = "endpoint"
	ConfigURL      = "url"
)

// Credential names.
const (
	CredentialAPIToken        = "api_token"
	CredentialAccessKeyID     = "access_key_id"
	CredentialAccessKeySecret = "access_key_secret"
	CredentialSecretID        = "secret_id"
	CredentialSecretKey       = "secret_key"
	CredentialAccessKey       = "access_key"
	CredentialSecret          = "secret"
)

// DefaultTimeout bounds one provider HTTP call.
const DefaultTimeout = 15 * time.Second

// maxResponse bounds a provider's answer.
const maxResponse = 1 << 20

// RecordSet is the records of one name and type. Name is fully qualified,
// inside Zone, without the trailing dot.
type RecordSet struct {
	Zone   string
	Name   string
	Type   string
	Values []string
	TTL    uint32
}

// Provider writes records through one DNS provider account.
type Provider interface {
	// Kind is the provider's kind.
	Kind() string
	// SetRecords makes the records of set.Type at set.Name exactly
	// set.Values with set.TTL. New values are added before removed ones
	// are deleted. Values must not be empty: use DeleteRecords.
	SetRecords(ctx context.Context, set RecordSet) error
	// DeleteRecords removes every record of rtype at name.
	DeleteRecords(ctx context.Context, zone, name, rtype string) error
}

// Spec is what a kind of provider takes.
type Spec struct {
	Kind string
	// Config are the settings it accepts; Required those it needs.
	Config   []string
	Required []string
	// Credentials are the secrets it needs.
	Credentials []string
}

var specs = map[string]Spec{
	KindCloudflare:  {Kind: KindCloudflare, Config: []string{ConfigEndpoint}, Credentials: []string{CredentialAPIToken}},
	KindAliDNS:      {Kind: KindAliDNS, Config: []string{ConfigEndpoint}, Credentials: []string{CredentialAccessKeyID, CredentialAccessKeySecret}},
	KindDNSPod:      {Kind: KindDNSPod, Config: []string{ConfigEndpoint}, Credentials: []string{CredentialSecretID, CredentialSecretKey}},
	KindHuaweiCloud: {Kind: KindHuaweiCloud, Config: []string{ConfigEndpoint}, Credentials: []string{CredentialAccessKey, CredentialSecretKey}},
	KindWebhook:     {Kind: KindWebhook, Config: []string{ConfigURL}, Required: []string{ConfigURL}, Credentials: []string{CredentialSecret}},
}

// SpecFor returns the spec of kind.
func SpecFor(kind string) (Spec, bool) {
	spec, ok := specs[kind]
	return spec, ok
}

// Kinds lists the provider kinds.
func Kinds() []string {
	out := make([]string, 0, len(specs))
	for kind := range specs {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}

// FieldError is one reason a provider's settings are refused.
type FieldError struct {
	Field   string
	Code    string
	Message string
}

// Field error codes.
const (
	CodeRequired      = "required"
	CodeInvalidFormat = "invalid_format"
	CodeUnknownField  = "unknown_field"
)

// Validate checks a provider's kind, configuration and credentials.
func Validate(kind string, config, credentials map[string]string) []FieldError {
	spec, ok := specs[kind]
	if !ok {
		return []FieldError{{Field: "kind", Code: CodeInvalidFormat, Message: "unknown provider kind"}}
	}
	var out []FieldError
	allowed := map[string]bool{}
	for _, name := range spec.Config {
		allowed[name] = true
	}
	for _, name := range sortedKeys(config) {
		if !allowed[name] {
			out = append(out, FieldError{Field: "config." + name, Code: CodeUnknownField, Message: "not a setting of this provider"})
		}
	}
	for _, name := range spec.Required {
		if strings.TrimSpace(config[name]) == "" {
			out = append(out, FieldError{Field: "config." + name, Code: CodeRequired, Message: "is required"})
		}
	}
	if endpoint := strings.TrimSpace(config[ConfigEndpoint]); endpoint != "" && !validHost(endpoint) {
		out = append(out, FieldError{Field: "config." + ConfigEndpoint, Code: CodeInvalidFormat, Message: "must be a host name, optionally with a port"})
	}
	if raw := strings.TrimSpace(config[ConfigURL]); raw != "" {
		if _, err := webhookURL(raw); err != nil {
			out = append(out, FieldError{Field: "config." + ConfigURL, Code: CodeInvalidFormat, Message: err.Error()})
		}
	}
	creds := map[string]bool{}
	for _, name := range spec.Credentials {
		creds[name] = true
		if strings.TrimSpace(credentials[name]) == "" {
			out = append(out, FieldError{Field: "credentials." + name, Code: CodeRequired, Message: "is required"})
		}
	}
	for _, name := range sortedKeys(credentials) {
		if !creds[name] {
			out = append(out, FieldError{Field: "credentials." + name, Code: CodeUnknownField, Message: "not a credential of this provider"})
		}
	}
	return out
}

func validHost(raw string) bool {
	host := raw
	if h, port, err := net.SplitHostPort(raw); err == nil {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return false
		}
		host = h
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	return ValidName(host)
}

// Options are a provider's runtime dependencies.
type Options struct {
	// Client is the HTTP client; one with DefaultTimeout by default.
	Client *http.Client
	// Now defaults to time.Now; Nonce to 16 random bytes in hex.
	Now   func() time.Time
	Nonce func() string
}

func (o Options) client() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	return &http.Client{Timeout: DefaultTimeout}
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now().UTC()
	}
	return time.Now().UTC()
}

func (o Options) nonce() string {
	if o.Nonce != nil {
		return o.Nonce()
	}
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}

// New returns the provider of kind with its configuration and
// credentials, which Validate accepts.
func New(kind string, config, credentials map[string]string, opts Options) (Provider, error) {
	if problems := Validate(kind, config, credentials); len(problems) > 0 {
		return nil, fmt.Errorf("dns provider %s: %s %s", kind, problems[0].Field, problems[0].Message)
	}
	endpoint := strings.TrimSpace(config[ConfigEndpoint])
	switch kind {
	case KindCloudflare:
		return newCloudflare(endpoint, credentials[CredentialAPIToken], opts), nil
	case KindAliDNS:
		return newAliDNS(endpoint, credentials[CredentialAccessKeyID], credentials[CredentialAccessKeySecret], opts), nil
	case KindDNSPod:
		return newDNSPod(endpoint, credentials[CredentialSecretID], credentials[CredentialSecretKey], opts), nil
	case KindHuaweiCloud:
		return newHuawei(endpoint, credentials[CredentialAccessKey], credentials[CredentialSecretKey], opts), nil
	case KindWebhook:
		target, err := webhookURL(config[ConfigURL])
		if err != nil {
			return nil, err
		}
		return &webhook{url: target, secret: credentials[CredentialSecret], opts: opts}, nil
	}
	return nil, fmt.Errorf("dns provider: unknown kind %q", kind)
}

// APIError is a provider's refusal. It carries the provider's code and
// message, never a credential.
type APIError struct {
	Kind    string
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	text := fmt.Sprintf("%s: HTTP %d", e.Kind, e.Status)
	if e.Code != "" {
		text += " " + e.Code
	}
	if e.Message != "" {
		text += ": " + e.Message
	}
	return text
}

// ErrNotFound: the zone is not in the account.
var ErrNotFound = errors.New("dns zone not found in the provider account")

// ValidName reports whether name is a DNS name (no trailing dot).
func ValidName(name string) bool {
	if name == "" || len(name) > 253 || strings.HasSuffix(name, ".") {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
				return false
			}
		}
	}
	return true
}

// NormalizeName lowercases a DNS name and drops its trailing dot.
func NormalizeName(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}

// InZone reports whether name is zone or below it.
func InZone(name, zone string) bool {
	name, zone = NormalizeName(name), NormalizeName(zone)
	return name == zone || strings.HasSuffix(name, "."+zone)
}

// relative is name relative to zone: "@" for the apex.
func relative(name, zone string) string {
	name, zone = NormalizeName(name), NormalizeName(zone)
	if name == zone {
		return "@"
	}
	return strings.TrimSuffix(name, "."+zone)
}

// CheckSet validates a record set before it is sent.
func CheckSet(set RecordSet, allowEmpty bool) error {
	if !ValidName(NormalizeName(set.Zone)) || !ValidName(NormalizeName(set.Name)) || !InZone(set.Name, set.Zone) {
		return fmt.Errorf("record %q is not a name in zone %q", set.Name, set.Zone)
	}
	switch set.Type {
	case TypeA, TypeAAAA, TypeCNAME:
	default:
		return fmt.Errorf("record type %q is not A, AAAA or CNAME", set.Type)
	}
	if len(set.Values) == 0 && !allowEmpty {
		return errors.New("a record set needs at least one value")
	}
	if set.Type == TypeCNAME && len(set.Values) > 1 {
		return errors.New("a CNAME has one value")
	}
	for _, value := range set.Values {
		if _, err := normalizeValue(set.Type, value); err != nil {
			return err
		}
	}
	return nil
}

// normalizeValue is a record value as compared: an address in its
// canonical form, a name lowercased without its trailing dot.
func normalizeValue(rtype, value string) (string, error) {
	switch rtype {
	case TypeA, TypeAAAA:
		addr, err := netip.ParseAddr(strings.TrimSpace(value))
		if err != nil || addr.Zone() != "" || (rtype == TypeA) != addr.Unmap().Is4() {
			return "", fmt.Errorf("%q is not an %s record value", value, rtype)
		}
		return addr.Unmap().String(), nil
	case TypeCNAME:
		name := NormalizeName(value)
		if !ValidName(name) {
			return "", fmt.Errorf("%q is not a CNAME target", value)
		}
		return name, nil
	}
	return "", fmt.Errorf("record type %q is not supported", rtype)
}

// normalizedValues are a set's distinct values, normalized and sorted.
func normalizedValues(set RecordSet) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range set.Values {
		normal, err := normalizeValue(set.Type, value)
		if err != nil || seen[normal] {
			continue
		}
		seen[normal] = true
		out = append(out, normal)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// rfc3986 escapes as the signature algorithms require: every byte but
// A-Z a-z 0-9 - _ . ~ percent-encoded in upper case.
func rfc3986(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// canonicalQuery is the sorted, RFC 3986 encoded query string.
func canonicalQuery(query url.Values) string {
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var parts []string
	for _, key := range keys {
		values := append([]string(nil), query[key]...)
		sort.Strings(values)
		for _, value := range values {
			parts = append(parts, rfc3986(key)+"="+rfc3986(value))
		}
	}
	return strings.Join(parts, "&")
}

// readBody reads at most maxResponse bytes of an answer.
func readBody(response *http.Response) ([]byte, error) {
	defer func() { _ = response.Body.Close() }()
	return io.ReadAll(io.LimitReader(response.Body, maxResponse))
}

// clip bounds a provider message kept in an error.
func clip(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 300 {
		return message[:300] + "..."
	}
	return message
}
