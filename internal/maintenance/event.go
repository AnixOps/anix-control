// Package maintenance defines the authenticated, bounded diagnostic wire contract.
package maintenance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const WireVersion = "anixops.maintenance/v1"
const MaxBatch = 50
const MaxPayloadBytes = 256 * 1024
const MaxEventBytes = 16 * 1024

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var version = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+\-]{0,127}$`)
var errorCode = regexp.MustCompile(`^[A-Z][A-Z0-9_.-]{0,127}$`)
var earliest = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

type Event struct {
	SchemaVersion       int        `json:"schema_version"`
	EventID             string     `json:"event_id"`
	OccurredAt          time.Time  `json:"occurred_at"`
	Environment         string     `json:"environment"`
	Source              string     `json:"source"`
	NodeID              string     `json:"node_id"`
	PluginID            string     `json:"plugin_id"`
	InstanceID          string     `json:"instance_id"`
	PluginVersion       string     `json:"plugin_version"`
	AgentVersion        string     `json:"agent_version,omitempty"`
	ConfigVersion       string     `json:"config_version,omitempty"`
	ErrorCode           string     `json:"error_code"`
	Severity            string     `json:"severity"`
	Status              string     `json:"status"`
	FirstFailedAt       *time.Time `json:"first_failed_at,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures,omitempty"`
	HealthySince        *time.Time `json:"healthy_since,omitempty"`
	SelfHealAction      string     `json:"self_heal_action,omitempty"`
	SelfHealResult      string     `json:"self_heal_result,omitempty"`
	DiagnosticRef       string     `json:"diagnostic_ref,omitempty"`
	TicketKey           string     `json:"ticket_key,omitempty"`
	RedactedSummary     string     `json:"redacted_summary,omitempty"`
}
type Batch struct {
	Version string            `json:"version"`
	Events  []json.RawMessage `json:"events"`
}
type Ack struct {
	EventID   string `json:"event_id"`
	Persisted bool   `json:"persisted"`
	Error     string `json:"error,omitempty"`
}
type Acknowledgement struct {
	Version string `json:"version"`
	Events  []Ack  `json:"events"`
}

func Decode(raw []byte, value any, limit int) error {
	if len(raw) > limit {
		return errors.New("payload_too_large")
	}
	// Duplicate JSON keys are ambiguous across implementations and signatures.
	scanner := json.NewDecoder(bytes.NewReader(raw))
	var keyErr error
	if _, batch := value.(*Batch); batch {
		keyErr = uniqueBatchKeys(scanner)
	} else {
		keyErr = uniqueJSONKeys(scanner, 0)
	}
	if keyErr != nil {
		return errors.New("invalid_json")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return errors.New("invalid_json")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("invalid_json")
	}
	return nil
}
func (e Event) Validate() error { return e.ValidateAt(time.Now()) }
func (e Event) ValidateAt(now time.Time) error {
	if e.SchemaVersion != 1 {
		return errors.New("unsupported_schema")
	}
	for _, id := range []string{e.EventID, e.PluginID, e.InstanceID} {
		if !identifier.MatchString(id) {
			return errors.New("invalid_identity")
		}
	}
	node, err := strconv.ParseUint(e.NodeID, 10, 64)
	if err != nil || node == 0 || strconv.FormatUint(node, 10) != e.NodeID {
		return errors.New("invalid_node")
	}
	if !version.MatchString(e.PluginVersion) {
		return errors.New("invalid_version")
	}
	for _, v := range []string{e.AgentVersion, e.ConfigVersion} {
		if v != "" && !version.MatchString(v) {
			return errors.New("invalid_version")
		}
	}
	if !errorCode.MatchString(e.ErrorCode) {
		return errors.New("invalid_error_code")
	}
	if !oneOf(e.Environment, "production", "staging", "development") || !oneOf(e.Source, "agent", "control", "networkcore", "deployment") || !oneOf(e.Severity, "P0", "P1", "P2", "P3") || !oneOf(e.Status, "open", "degraded", "recovered") {
		return errors.New("invalid_enum")
	}
	if !oneOf(e.SelfHealAction, "", "none", "retry", "restart", "circuit_break") || !oneOf(e.SelfHealResult, "", "not_attempted", "succeeded", "failed", "blocked") {
		return errors.New("invalid_self_heal")
	}
	if len(e.DiagnosticRef) > 256 || len(e.TicketKey) > 256 || len(e.RedactedSummary) > 2000 {
		return errors.New("field_too_long")
	}
	if e.OccurredAt.Before(earliest) || e.OccurredAt.After(now.Add(5*time.Minute)) {
		return errors.New("invalid_time")
	}
	for _, t := range []*time.Time{e.FirstFailedAt, e.HealthySince} {
		if t != nil && (t.Before(earliest) || t.After(e.OccurredAt)) {
			return errors.New("invalid_time")
		}
	}
	if e.ConsecutiveFailures < 0 || e.ConsecutiveFailures > 1000000 {
		return errors.New("invalid_failure_count")
	}
	if e.Status == "recovered" && (e.HealthySince == nil || e.ConsecutiveFailures != 0) {
		return errors.New("invalid_recovery")
	}
	if e.Status != "recovered" && (e.FirstFailedAt == nil || e.ConsecutiveFailures == 0 || e.HealthySince != nil) {
		return errors.New("invalid_failure")
	}
	return nil
}
func oneOf(v string, items ...string) bool {
	for _, item := range items {
		if v == item {
			return true
		}
	}
	return false
}
func (e Event) DedupeKey() string {
	sum := sha256.Sum256([]byte(strings.Join([]string{e.Environment, e.NodeID, e.PluginID, e.InstanceID}, "\x00")))
	return hex.EncodeToString(sum[:])
}
func (e Event) Manual() bool {
	for _, s := range []string{"CREDENTIAL", "PERMISSION", "SIGNATURE", "CONFIG", "SECURITY", "INTEGRITY", "CONTROL_UNAVAILABLE"} {
		if strings.Contains(e.ErrorCode, s) {
			return true
		}
	}
	return false
}
func (e Event) Major() bool {
	for _, s := range []string{"SECURITY", "INTEGRITY", "CONTROL_UNAVAILABLE"} {
		if strings.Contains(e.ErrorCode, s) {
			return true
		}
	}
	return false
}
func (e Event) Fault() bool {
	return e.Status != "recovered" && (e.Manual() || (e.FirstFailedAt != nil && e.ConsecutiveFailures >= 3 && e.OccurredAt.Sub(*e.FirstFailedAt) >= 2*time.Minute))
}
func (e Event) Recovered() bool {
	return e.Status == "recovered" && e.HealthySince != nil && e.OccurredAt.Sub(*e.HealthySince) >= 5*time.Minute
}

// Safe discards all free-form client diagnostics, even fields named "redacted".
func (e Event) Safe() Event { e.RedactedSummary = ""; e.DiagnosticRef = ""; e.TicketKey = ""; return e }

// uniqueJSONKeys bounds nesting and rejects repeated object keys, including in
// raw per-event payloads before JSON's default last-key-wins decoding.
func uniqueJSONKeys(d *json.Decoder, depth int) error {
	if depth > 16 {
		return errors.New("json nesting too deep")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("duplicate key")
			}
			seen[name] = true
			if err := uniqueJSONKeys(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueJSONKeys(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid delimiter")
	}
	_, err = d.Token()
	return err
}

// Batch children are validated independently so one invalid event cannot prevent
// acknowledging durable siblings. Only duplicate envelope keys reject the batch.
func uniqueBatchKeys(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return errors.New("expected object")
	}
	seen := map[string]bool{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return err
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return errors.New("duplicate key")
		}
		seen[name] = true
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}
