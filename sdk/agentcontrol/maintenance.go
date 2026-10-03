package agentcontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The maintenance events of maintenance.v1 (PROTOCOL.md, "Maintenance
// events"): the Agent's durable maintenance outbox, plugin health
// incidents and recoveries, in the schema anixops.maintenance/v1 the
// legacy agent WebSocket's maintenance_events message carries. A
// MaintenanceEvents message holds one JSON object per event.
const (
	// MaintenanceSchemaV1 is MaintenanceEvents.version.
	MaintenanceSchemaV1 = "anixops.maintenance/v1"
	// MaxMaintenanceBatchEvents caps the events of one MaintenanceEvents.
	MaxMaintenanceBatchEvents = 50
	// MaxMaintenanceEventBytes caps one event's JSON.
	MaxMaintenanceEventBytes = 16 << 10
	// MaxMaintenanceBatchBytes caps the events of one batch together.
	MaxMaintenanceBatchBytes = 256 << 10
	// MaxMaintenanceEventIDLength caps an event_id.
	MaxMaintenanceEventIDLength = 128
)

// ErrInvalidMaintenanceEvent reports an event that is not a valid
// anixops.maintenance/v1 event of the stream's node. Control refuses it for
// good.
var ErrInvalidMaintenanceEvent = errors.New("invalid maintenance event")

var (
	maintenanceIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	maintenanceErrorPattern    = regexp.MustCompile(`^[A-Z][A-Z0-9_.-]{0,127}$`)
	maintenanceVersionPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
	maintenanceNodePattern     = regexp.MustCompile(`^[1-9][0-9]{0,127}$`)
	maintenanceEpoch           = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
)

// MaintenanceEvent is one anixops.maintenance/v1 event.
type MaintenanceEvent struct {
	SchemaVersion       int        `json:"schema_version"`
	EventID             string     `json:"event_id"`
	OccurredAt          time.Time  `json:"occurred_at"`
	Environment         string     `json:"environment"`
	Source              string     `json:"source"`
	NodeID              string     `json:"node_id"`
	AgentVersion        string     `json:"agent_version,omitempty"`
	ConfigVersion       string     `json:"config_version,omitempty"`
	PluginID            string     `json:"plugin_id,omitempty"`
	PluginVersion       string     `json:"plugin_version,omitempty"`
	InstanceID          string     `json:"instance_id,omitempty"`
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

// ParseMaintenanceEvent reads one event of a MaintenanceEvents batch sent
// on node's stream, at now: a JSON object of at most
// MaxMaintenanceEventBytes, valid by Validate, naming node. Fields the
// schema does not know are dropped. Errors wrap ErrInvalidMaintenanceEvent.
func ParseMaintenanceEvent(raw []byte, node AgentNode, now time.Time) (MaintenanceEvent, error) {
	if len(raw) > MaxMaintenanceEventBytes {
		return MaintenanceEvent{}, fmt.Errorf("%w: the event exceeds %d bytes", ErrInvalidMaintenanceEvent, MaxMaintenanceEventBytes)
	}
	var event MaintenanceEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return MaintenanceEvent{}, fmt.Errorf("%w: %v", ErrInvalidMaintenanceEvent, err)
	}
	if err := event.Validate(now); err != nil {
		return MaintenanceEvent{}, err
	}
	if event.NodeID != strconv.FormatUint(uint64(node.ID), 10) {
		return MaintenanceEvent{}, fmt.Errorf("%w: %w: node_id %s is not the stream's node", ErrInvalidMaintenanceEvent, ErrMaintenanceEventWrongNode, event.NodeID)
	}
	return event, nil
}

// Validate applies the anixops.maintenance/v1 rules the Agent's outbox
// applies before it queues an event, at now.
func (e MaintenanceEvent) Validate(now time.Time) error {
	invalid := func(reason string) error { return fmt.Errorf("%w: %s", ErrInvalidMaintenanceEvent, reason) }
	if e.SchemaVersion != 1 {
		return invalid("schema_version must be 1")
	}
	if node, err := strconv.ParseUint(e.NodeID, 10, 64); err != nil || node == 0 || !maintenanceNodePattern.MatchString(e.NodeID) {
		return invalid("invalid node_id")
	}
	for _, field := range []struct{ name, value string }{{"event_id", e.EventID}, {"plugin_id", e.PluginID}, {"instance_id", e.InstanceID}} {
		if !maintenanceIdentityPattern.MatchString(field.value) {
			return invalid("invalid " + field.name)
		}
	}
	for _, field := range []struct{ name, value string }{{"agent_version", e.AgentVersion}, {"config_version", e.ConfigVersion}} {
		if field.value != "" && !maintenanceVersionPattern.MatchString(field.value) {
			return invalid("invalid " + field.name)
		}
	}
	if !maintenanceVersionPattern.MatchString(e.PluginVersion) {
		return invalid("invalid plugin_version")
	}
	if !maintenanceErrorPattern.MatchString(e.ErrorCode) {
		return invalid("invalid error_code")
	}
	if e.OccurredAt.Before(maintenanceEpoch) || e.OccurredAt.After(now.Add(5*time.Minute)) {
		return invalid("invalid occurred_at")
	}
	switch {
	case !oneOf(e.Environment, "production", "staging", "development"):
		return invalid("invalid environment")
	case !oneOf(e.Source, "agent", "control", "networkcore", "deployment"):
		return invalid("invalid source")
	case !oneOf(e.Severity, "P0", "P1", "P2", "P3"):
		return invalid("invalid severity")
	case !oneOf(e.Status, "open", "degraded", "recovered"):
		return invalid("invalid status")
	case !oneOf(e.SelfHealAction, "", "none", "retry", "restart", "circuit_break"):
		return invalid("invalid self_heal_action")
	case !oneOf(e.SelfHealResult, "", "not_attempted", "succeeded", "failed", "blocked"):
		return invalid("invalid self_heal_result")
	case e.ConsecutiveFailures < 0 || e.ConsecutiveFailures > 1000000:
		return invalid("invalid consecutive_failures")
	}
	if e.FirstFailedAt != nil && (e.FirstFailedAt.Before(maintenanceEpoch) || e.FirstFailedAt.After(e.OccurredAt)) {
		return invalid("invalid first_failed_at")
	}
	if e.HealthySince != nil && (e.HealthySince.Before(maintenanceEpoch) || e.HealthySince.After(e.OccurredAt)) {
		return invalid("invalid healthy_since")
	}
	if len(e.DiagnosticRef) > 256 || len(e.TicketKey) > 256 || len(e.RedactedSummary) > 2000 ||
		strings.ContainsRune(e.DiagnosticRef+e.TicketKey+e.RedactedSummary, 0) {
		return invalid("invalid diagnostic fields")
	}
	if e.Status == "recovered" && (e.HealthySince == nil || e.ConsecutiveFailures != 0) {
		return invalid("a recovery needs healthy_since and no consecutive_failures")
	}
	if e.Status != "recovered" && (e.FirstFailedAt == nil || e.ConsecutiveFailures == 0 || e.HealthySince != nil) {
		return invalid("a failure needs first_failed_at and consecutive_failures, and no healthy_since")
	}
	return nil
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

// MaintenanceEventID returns the event_id of a raw event when it is a
// valid event id, for answering an event that is otherwise refused; empty
// when the event cannot be read or its id is malformed.
func MaintenanceEventID(raw []byte) string {
	if len(raw) > MaxMaintenanceEventBytes {
		return ""
	}
	var event struct {
		EventID string `json:"event_id"`
	}
	if json.Unmarshal(raw, &event) != nil || !maintenanceIdentityPattern.MatchString(event.EventID) {
		return ""
	}
	return event.EventID
}
