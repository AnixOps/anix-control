package agentcontrol

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func validMaintenanceEvent(now time.Time) MaintenanceEvent {
	firstFailed := now.Add(-3 * time.Minute)
	return MaintenanceEvent{
		SchemaVersion: 1, EventID: "0123456789abcdef0123456789abcdef", OccurredAt: now.Add(-time.Minute),
		Environment: "production", Source: "agent", NodeID: "12", AgentVersion: "4.1.0", ConfigVersion: "7",
		PluginID: "machine-telemetry", PluginVersion: "1.1.0", InstanceID: "machine-telemetry",
		ErrorCode: "PLUGIN_PROCESS_EXITED", Severity: "P2", Status: "open", FirstFailedAt: &firstFailed,
		ConsecutiveFailures: 3, SelfHealAction: "restart", SelfHealResult: "succeeded", RedactedSummary: "exited with status 1",
	}
}

func encodeMaintenanceEvent(t *testing.T, event MaintenanceEvent) []byte {
	t.Helper()
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParseMaintenanceEventAcceptsTheAgentSchema(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	node := AgentNode{Kind: NodeKindProxy, ID: 12}
	raw := encodeMaintenanceEvent(t, validMaintenanceEvent(now))
	// A field a later schema adds is dropped, not refused.
	raw = append(raw[:len(raw)-1], []byte(`,"later_field":"x"}`)...)
	event, err := ParseMaintenanceEvent(raw, node, now)
	if err != nil {
		t.Fatalf("ParseMaintenanceEvent() error = %v", err)
	}
	if event.EventID != "0123456789abcdef0123456789abcdef" || event.ConsecutiveFailures != 3 {
		t.Fatalf("event = %+v", event)
	}

	healthy := now.Add(-2 * time.Minute)
	recovered := validMaintenanceEvent(now)
	recovered.Status, recovered.FirstFailedAt, recovered.ConsecutiveFailures, recovered.HealthySince = "recovered", nil, 0, &healthy
	if _, err := ParseMaintenanceEvent(encodeMaintenanceEvent(t, recovered), node, now); err != nil {
		t.Fatalf("a recovery: %v", err)
	}
}

func TestParseMaintenanceEventRefusals(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	node := AgentNode{Kind: NodeKindProxy, ID: 12}
	healthy := now.Add(-time.Minute)
	tests := map[string]func(*MaintenanceEvent){
		"schema version":      func(e *MaintenanceEvent) { e.SchemaVersion = 2 },
		"another node":        func(e *MaintenanceEvent) { e.NodeID = "13" },
		"node zero":           func(e *MaintenanceEvent) { e.NodeID = "0" },
		"event id":            func(e *MaintenanceEvent) { e.EventID = "-bad" },
		"plugin id":           func(e *MaintenanceEvent) { e.PluginID = "" },
		"instance id":         func(e *MaintenanceEvent) { e.InstanceID = "a b" },
		"agent version":       func(e *MaintenanceEvent) { e.AgentVersion = "1 0" },
		"config version":      func(e *MaintenanceEvent) { e.ConfigVersion = "/" },
		"plugin version":      func(e *MaintenanceEvent) { e.PluginVersion = "" },
		"error code":          func(e *MaintenanceEvent) { e.ErrorCode = "lower" },
		"before 2020":         func(e *MaintenanceEvent) { e.OccurredAt = time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC) },
		"in the future":       func(e *MaintenanceEvent) { e.OccurredAt = now.Add(10 * time.Minute) },
		"environment":         func(e *MaintenanceEvent) { e.Environment = "prod" },
		"source":              func(e *MaintenanceEvent) { e.Source = "user" },
		"severity":            func(e *MaintenanceEvent) { e.Severity = "P4" },
		"status":              func(e *MaintenanceEvent) { e.Status = "closed" },
		"self-heal action":    func(e *MaintenanceEvent) { e.SelfHealAction = "reboot" },
		"self-heal result":    func(e *MaintenanceEvent) { e.SelfHealResult = "maybe" },
		"failures":            func(e *MaintenanceEvent) { e.ConsecutiveFailures = -1 },
		"first failed after":  func(e *MaintenanceEvent) { later := e.OccurredAt.Add(time.Second); e.FirstFailedAt = &later },
		"healthy since after": func(e *MaintenanceEvent) { later := e.OccurredAt.Add(time.Second); e.HealthySince = &later },
		"summary":             func(e *MaintenanceEvent) { e.RedactedSummary = strings.Repeat("x", 2001) },
		"nul":                 func(e *MaintenanceEvent) { e.TicketKey = "a\x00b" },
		"recovery with failures": func(e *MaintenanceEvent) {
			e.Status, e.HealthySince = "recovered", &healthy
		},
		"failure without first failure": func(e *MaintenanceEvent) { e.FirstFailedAt = nil },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			event := validMaintenanceEvent(now)
			mutate(&event)
			if _, err := ParseMaintenanceEvent(encodeMaintenanceEvent(t, event), node, now); !errors.Is(err, ErrInvalidMaintenanceEvent) {
				t.Fatalf("ParseMaintenanceEvent() error = %v, want ErrInvalidMaintenanceEvent", err)
			}
		})
	}
	for name, raw := range map[string][]byte{
		"not JSON":   []byte("{"),
		"not object": []byte(`[1]`),
		"oversize":   []byte(`{"redacted_summary":"` + strings.Repeat("x", MaxMaintenanceEventBytes) + `"}`),
	} {
		if _, err := ParseMaintenanceEvent(raw, node, now); !errors.Is(err, ErrInvalidMaintenanceEvent) {
			t.Fatalf("%s: error = %v, want ErrInvalidMaintenanceEvent", name, err)
		}
	}
}

func TestMaintenanceEventIDEchoesOnlyValidIDs(t *testing.T) {
	for raw, want := range map[string]string{
		`{"event_id":"abc-1"}`:                    "abc-1",
		`{"event_id":"abc-1","schema_version":9}`: "abc-1",
		`{"event_id":"-bad"}`:                     "",
		`{"event_id":7}`:                          "",
		`{`:                                       "",
		`{"event_id":"` + strings.Repeat("a", 129) + `"}`: "",
	} {
		if got := MaintenanceEventID([]byte(raw)); got != want {
			t.Fatalf("MaintenanceEventID(%s) = %q, want %q", raw, got, want)
		}
	}
	if got := MaintenanceEventID([]byte(`{"event_id":"a","x":"` + strings.Repeat("x", MaxMaintenanceEventBytes) + `"}`)); got != "" {
		t.Fatalf("an oversize event's id = %q, want empty", got)
	}
}
