package maintenance

import (
	"os"
	"testing"
	"time"
)

func TestSharedMaintenanceContractFixture(t *testing.T) {
	data, err := os.ReadFile("../../docs/contracts/maintenance-event.fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	var event Event
	if err := Decode(data, &event, MaxEventBytes); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 22, 0, 3, 0, 0, time.UTC)
	if err := event.ValidateAt(now); err != nil {
		t.Fatal(err)
	}
	if !event.Fault() {
		t.Fatal("fixture must establish a sustained fault")
	}
}

func TestDuplicateFieldsAndMalformedBatchSibling(t *testing.T) {
	var event Event
	if Decode([]byte(`{"event_id":"a","event_id":"b"}`), &event, MaxEventBytes) == nil {
		t.Fatal("duplicate fields accepted")
	}
	var batch Batch
	if err := Decode([]byte(`{"version":"anixops.maintenance/v1","events":[{"event_id":"a","event_id":"b"}]}`), &batch, MaxPayloadBytes); err != nil {
		t.Fatal("event validation must be isolated from batch:", err)
	}
	if Decode([]byte(`{"version":"a","version":"b","events":[]}`), &batch, MaxPayloadBytes) == nil {
		t.Fatal("duplicate batch version accepted")
	}
}
