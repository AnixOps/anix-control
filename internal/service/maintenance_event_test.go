package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/maintenance"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func maintenanceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ops.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(append(append(model.MaintenanceModels(), model.MaintenanceOperationModels()...), &model.Node{}, &model.User{}, &model.KernelOperation{}, &model.NodePluginLifecycle{})...))
	return db
}
func maintenanceEvent(id string, now time.Time) maintenance.Event {
	first := now.Add(-2 * time.Minute)
	return maintenance.Event{SchemaVersion: 1, EventID: id, OccurredAt: now, Environment: "production", Source: "agent", NodeID: "1", PluginID: "machine-telemetry", InstanceID: "machine-telemetry", PluginVersion: "1.1.0", AgentVersion: "4.0.0", ConfigVersion: "1", ErrorCode: "PLUGIN_HEALTH_FAILED", Severity: "P1", Status: "open", FirstFailedAt: &first, ConsecutiveFailures: 3}
}
func maintenanceBatch(t *testing.T, events ...maintenance.Event) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"version": maintenance.WireVersion, "events": events})
	require.NoError(t, err)
	return b
}
func seedMaintenanceSettings(t *testing.T, db *gorm.DB, now time.Time, technicians bool) {
	t.Helper()
	s := model.MaintenanceSettings{ID: 1, Enabled: true, OwnerID: 10, Revision: 1, OwnerEmailVerifiedAt: &now, OwnerTelegramVerifiedAt: &now, Contacts: []model.MaintenanceContact{{UserID: 10, Email: "owner@example.invalid", TelegramChatID: "10"}}}
	if technicians {
		s.TechnicianIDs = []uint{20}
		s.Contacts = append(s.Contacts, model.MaintenanceContact{UserID: 20, Email: "tech@example.invalid", TelegramChatID: "20"})
	}
	require.NoError(t, db.Create(&s).Error)
	require.NoError(t, db.Create(&model.Node{ID: 1, Name: "test node", Status: model.NodeStatusOnline}).Error)
}
func ticketCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&model.MaintenanceTicket{}).Count(&count).Error)
	return count
}
func TestMaintenanceDurableConcurrentReceiveAndAggregation(t *testing.T) {
	db := maintenanceDB(t)
	now := time.Now().UTC()
	seedMaintenanceSettings(t, db, now, true)
	event := maintenanceEvent("event-1", now)
	event.RedactedSummary = "password=canary-secret https://example.invalid/sub?token=hidden"
	event.DiagnosticRef = "Bearer hidden-ref"
	raw := maintenanceBatch(t, event)
	var wg sync.WaitGroup
	var failed atomic.Int32
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ack := ReceiveMaintenanceEvents(db, 1, raw, now)
			if len(ack.Events) != 1 || !ack.Events[0].Persisted {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Zero(t, failed.Load())
	var stored model.MaintenanceEvent
	require.NoError(t, db.First(&stored).Error)
	require.NotContains(t, stored.Body, "canary-secret")
	require.NotContains(t, stored.Body, "hidden-ref")
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ProcessMaintenanceEvents(db, 50, now)
			if err != nil {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Zero(t, failed.Load())
	require.EqualValues(t, 1, ticketCount(t, db))
	var deliveries int64
	require.NoError(t, db.Model(&model.MaintenanceDelivery{}).Count(&deliveries).Error)
	require.EqualValues(t, 2, deliveries)
	// A consumed event is still acknowledged and cannot open another ticket.
	require.True(t, ReceiveMaintenanceEvents(db, 1, raw, now).Events[0].Persisted)
	n, err := ProcessMaintenanceEvents(db, 50, now)
	require.NoError(t, err)
	require.Zero(t, n)
	changed := event
	changed.PluginVersion = "9.9.9"
	require.Equal(t, "event_id_conflict", ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, changed), now).Events[0].Error)
}
func TestMaintenanceRejectsForgeryInvalidFieldsAndDBFailure(t *testing.T) {
	db := maintenanceDB(t)
	now := time.Now()
	event := maintenanceEvent("safe-id", now)
	require.Equal(t, "identity_mismatch", ReceiveMaintenanceEvents(db, 2, maintenanceBatch(t, event), now).Events[0].Error)
	changes := []func(*maintenance.Event){func(e *maintenance.Event) { e.SchemaVersion = 0 }, func(e *maintenance.Event) { e.OccurredAt = now.Add(6 * time.Minute) }, func(e *maintenance.Event) { e.PluginVersion = "../../secret" }, func(e *maintenance.Event) { e.SelfHealAction = "rollback" }, func(e *maintenance.Event) { e.Status = "closed" }, func(e *maintenance.Event) { e.RedactedSummary = strings.Repeat("x", 2001) }, func(e *maintenance.Event) { e.ErrorCode = "TOKEN\nvalue" }}
	for i, change := range changes {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			copy := event
			change(&copy)
			require.False(t, ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, copy), now).Events[0].Persisted)
		})
	}

	raw := maintenanceBatch(t, event)
	raw = bytesReplaceField(raw, `"schema_version":1`, `"schema_version":1,"unexpected":"secret"`)
	require.False(t, ReceiveMaintenanceEvents(db, 1, raw, now).Events[0].Persisted)
	huge := make([]maintenance.Event, 51)
	for i := range huge {
		huge[i] = event
	}
	require.Equal(t, "invalid_batch", ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, huge...), now).Events[0].Error)
	require.Equal(t, "invalid_batch", ReceiveMaintenanceEvents(db, 1, []byte(strings.Repeat(" ", maintenance.MaxPayloadBytes+1)), now).Events[0].Error)
	require.NoError(t, db.Migrator().DropTable(&model.MaintenanceEvent{}))
	require.Equal(t, "persistence_failed", ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, event), now).Events[0].Error)
}
func bytesReplaceField(raw []byte, old, new string) []byte {
	return []byte(strings.Replace(string(raw), old, new, 1))
}
func TestMaintenanceFaultTimingRecoveryRecurrenceAndPoison(t *testing.T) {
	db := maintenanceDB(t)
	now := time.Now().UTC()
	seedMaintenanceSettings(t, db, now, true)
	event := maintenanceEvent("early", now)
	event.ConsecutiveFailures = 2
	require.True(t, ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, event), now).Events[0].Persisted)
	_, err := ProcessMaintenanceEvents(db, 50, now)
	require.NoError(t, err)
	require.Zero(t, ticketCount(t, db))
	event.EventID = "fault"
	event.ConsecutiveFailures = 3
	event.OccurredAt = now.Add(time.Second)
	require.True(t, ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, event), now).Events[0].Persisted)
	_, err = ProcessMaintenanceEvents(db, 50, now)
	require.NoError(t, err)
	require.EqualValues(t, 1, ticketCount(t, db))
	recovery := event
	recovery.EventID = "recovery-short"
	recovery.Status = "recovered"
	recovery.ConsecutiveFailures = 0
	healthy := now.Add(time.Minute)
	recovery.HealthySince = &healthy
	recovery.OccurredAt = healthy.Add(4 * time.Minute)
	require.True(t, ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, recovery), recovery.OccurredAt).Events[0].Persisted)
	_, err = ProcessMaintenanceEvents(db, 50, recovery.OccurredAt)
	require.NoError(t, err)
	var ticket model.MaintenanceTicket
	require.NoError(t, db.First(&ticket).Error)
	require.Equal(t, "open", ticket.Status)
	recovery.EventID = "recovery"
	recovery.OccurredAt = healthy.Add(5 * time.Minute)
	require.True(t, ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, recovery), recovery.OccurredAt).Events[0].Persisted)
	_, err = ProcessMaintenanceEvents(db, 50, recovery.OccurredAt)
	require.NoError(t, err)
	require.NoError(t, db.First(&ticket).Error)
	require.Equal(t, "recovered", ticket.Status)
	require.NoError(t, TickMaintenanceTickets(db, recovery.OccurredAt.Add(3*time.Hour)))
	var escalations int64
	require.NoError(t, db.Model(&model.MaintenanceDelivery{}).Where("reason = ?", "escalated").Count(&escalations).Error)
	require.Zero(t, escalations)
	event.EventID = "recurrence"
	event.OccurredAt = recovery.OccurredAt.Add(3 * time.Minute)
	first := recovery.OccurredAt
	event.FirstFailedAt = &first
	require.True(t, ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, event), event.OccurredAt).Events[0].Persisted)
	_, err = ProcessMaintenanceEvents(db, 50, event.OccurredAt)
	require.NoError(t, err)
	require.EqualValues(t, 1, ticketCount(t, db))
	require.NoError(t, db.First(&ticket).Error)
	require.Equal(t, 2, ticket.Episode)
	require.NoError(t, db.Model(&ticket).Updates(map[string]any{"status": "closed", "closed_at": event.OccurredAt}).Error)
	event.EventID = "after-close"
	event.OccurredAt = event.OccurredAt.Add(3 * time.Minute)
	require.True(t, ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, event), event.OccurredAt).Events[0].Persisted)
	require.NoError(t, db.Create(&model.MaintenanceEvent{EventID: "poison", NodeID: 1, Body: "{broken", StreamKey: "bad", OccurredAt: now, CreatedAt: now, RetryAt: now}).Error)
	n, err := ProcessMaintenanceEvents(db, 50, event.OccurredAt)
	require.Error(t, err)
	require.Equal(t, 1, n)
	require.EqualValues(t, 2, ticketCount(t, db))
	var latest model.MaintenanceTicket
	require.NoError(t, db.Order("id DESC").First(&latest).Error)
	require.Equal(t, &ticket.ID, latest.PreviousTicketID)
}
func TestMaintenanceEscalationRemindersAndNodeExclusions(t *testing.T) {
	db := maintenanceDB(t)
	now := time.Now().UTC()
	seedMaintenanceSettings(t, db, now, true)
	e := maintenanceEvent("fault", now)
	require.True(t, ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, e), now).Events[0].Persisted)
	_, err := ProcessMaintenanceEvents(db, 50, now)
	require.NoError(t, err)
	require.NoError(t, TickMaintenanceTickets(db, e.FirstFailedAt.Add(30*time.Minute)))
	require.NoError(t, TickMaintenanceTickets(db, e.FirstFailedAt.Add(30*time.Minute)))
	var ticket model.MaintenanceTicket
	require.NoError(t, db.First(&ticket).Error)
	require.NotNil(t, ticket.EscalatedAt)
	var count int64
	require.NoError(t, db.Model(&model.MaintenanceDelivery{}).Where("reason = ?", "escalated").Count(&count).Error)
	require.EqualValues(t, 4, count)
	require.NoError(t, TickMaintenanceTickets(db, ticket.EscalatedAt.Add(30*time.Minute)))
	require.NoError(t, TickMaintenanceTickets(db, ticket.EscalatedAt.Add(31*time.Minute)))
	require.NoError(t, db.Model(&model.MaintenanceDelivery{}).Where("reason = ?", "reminder:1").Count(&count).Error)
	require.EqualValues(t, 4, count)
	claimed := now.Add(10 * time.Minute)
	owner := uint(20)
	require.NoError(t, db.Model(&ticket).Updates(map[string]any{"claimed_at": claimed, "claimed_by": owner}).Error)
	require.NoError(t, TickMaintenanceTickets(db, claimed.Add(2*time.Hour)))
	require.NoError(t, db.Model(&model.MaintenanceDelivery{}).Where("reason = ?", "summary:1").Count(&count).Error)
	require.EqualValues(t, 4, count)
	// Two out of five eligible nodes cross 20%; maintenance excludes one fault.
	for i := 2; i <= 5; i++ {
		require.NoError(t, db.Create(&model.Node{ID: uint(i), APIKey: fmt.Sprint(i), Status: model.NodeStatusOnline}).Error)
	}
	e.EventID = "node-two"
	e.NodeID = "2"
	e.OccurredAt = now.Add(time.Second)
	require.True(t, ReceiveMaintenanceEvents(db, 2, maintenanceBatch(t, e), now).Events[0].Persisted)
	_, err = ProcessMaintenanceEvents(db, 50, now)
	require.NoError(t, err)
	major, err := maintenanceMultiNodeFault(db, ticket, now)
	require.NoError(t, err)
	require.True(t, major)
	require.NoError(t, TickMaintenanceTickets(db, now.Add(time.Minute)))
	require.NoError(t, db.First(&ticket, ticket.ID).Error)
	require.Equal(t, "P0", ticket.Severity)
	require.NoError(t, db.Model(&model.MaintenanceDelivery{}).Where("reason = ?", "major").Count(&count).Error)
	require.EqualValues(t, 8, count)
	require.NoError(t, TickMaintenanceTickets(db, now.Add(2*time.Minute)))
	require.NoError(t, db.Model(&model.MaintenanceDelivery{}).Where("reason = ?", "major").Count(&count).Error)
	require.EqualValues(t, 8, count)
	require.NoError(t, db.Create(&model.MaintenanceNodeState{NodeID: 2, Maintenance: true}).Error)
	major, err = maintenanceMultiNodeFault(db, ticket, now)
	require.NoError(t, err)
	require.False(t, major)
}

type maintenanceFakeSender struct {
	mu        sync.Mutex
	calls     map[string]int
	failEmail bool
}

func (s *maintenanceFakeSender) Send(_ context.Context, d model.MaintenanceDelivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[d.Channel]++
	if d.Channel == "email" && s.failEmail {
		return errors.New("sensitive-provider-error-token")
	}
	return nil
}
func TestMaintenanceIndependentDeliveryRetryAndConcurrentClaims(t *testing.T) {
	db := maintenanceDB(t)
	now := time.Now().UTC()
	seedMaintenanceSettings(t, db, now, false)
	e := maintenanceEvent("fault", now)
	require.True(t, ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, e), now).Events[0].Persisted)
	_, err := ProcessMaintenanceEvents(db, 50, now)
	require.NoError(t, err)
	sender := &maintenanceFakeSender{calls: map[string]int{}, failEmail: true}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = DeliverMaintenanceNotifications(context.Background(), db, sender, now, 50)
		}()
	}
	wg.Wait()
	require.Equal(t, 1, sender.calls["email"])
	require.Equal(t, 1, sender.calls["telegram"])
	var failed model.MaintenanceDelivery
	require.NoError(t, db.First(&failed, "channel = ?", "email").Error)
	require.Equal(t, "pending", failed.Status)
	require.Equal(t, "delivery_failed", failed.LastError)
	sender.failEmail = false
	n, err := DeliverMaintenanceNotifications(context.Background(), db, sender, now.Add(time.Minute), 50)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 1, sender.calls["telegram"])
	n, err = DeliverMaintenanceNotifications(context.Background(), db, sender, now.Add(2*time.Minute), 50)
	require.NoError(t, err)
	require.Zero(t, n)
}
func TestMaintenanceWorkerStartupStopAndRetention(t *testing.T) {
	db := maintenanceDB(t)
	now := time.Now().UTC()
	seedMaintenanceSettings(t, db, now, false)
	e := maintenanceEvent("fault", now)
	require.True(t, ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, e), now).Events[0].Persisted)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	sender := &maintenanceFakeSender{calls: map[string]int{}}
	go func() {
		defer close(done)
		MaintenanceEventWorker{DB: db, Sender: sender, Interval: time.Hour}.Start(ctx)
	}()
	require.Eventually(t, func() bool { return ticketCount(t, db) == 1 }, 5*time.Second, 10*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
	require.NoError(t, CleanupMaintenance(db, now.AddDate(2, 0, 0)))
	require.EqualValues(t, 1, ticketCount(t, db))
	var count int64
	require.NoError(t, db.Model(&model.MaintenanceEvent{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestMaintenanceInvalidSiblingCannotBlockDurableAcknowledgement(t *testing.T) {
	db := maintenanceDB(t)
	now := time.Now()
	event := maintenanceEvent("valid-sibling", now)
	valid, err := json.Marshal(event)
	require.NoError(t, err)
	raw := []byte(`{"version":"anixops.maintenance/v1","events":[{"event_id":"bad","event_id":"ambiguous"},` + string(valid) + `]}`)
	ack := ReceiveMaintenanceEvents(db, 1, raw, now)
	require.Len(t, ack.Events, 2)
	require.False(t, ack.Events[0].Persisted)
	require.True(t, ack.Events[1].Persisted)
}
