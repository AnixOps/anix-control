package service

import (
	"encoding/json"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentreports"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func maintenanceEventForTest(nodeID uint, eventID, status, severity string) agentcontrol.MaintenanceEvent {
	occurred := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	event := agentcontrol.MaintenanceEvent{
		SchemaVersion: 1, EventID: eventID, OccurredAt: occurred, Environment: "production", Source: "agent",
		NodeID: strconv.FormatUint(uint64(nodeID), 10), PluginID: "machine-telemetry", PluginVersion: "1.1.0",
		InstanceID: "machine-telemetry", ErrorCode: "HEALTH_CHECK_FAILED", Severity: severity, Status: status,
	}
	if status == "recovered" {
		healthy := occurred.Add(-5 * time.Minute)
		event.HealthySince = &healthy
	} else {
		first := occurred.Add(-2 * time.Minute)
		event.FirstFailedAt, event.ConsecutiveFailures = &first, 3
	}
	return event
}

// recordMaintenanceOnce checks RecordAgentMaintenanceEvent on db: one node
// log row per node and event id, whatever the number of deliveries, and a
// node that does not exist refused.
func recordMaintenanceOnce(t *testing.T, db *gorm.DB) {
	t.Helper()
	node := model.Node{Name: "maintenance", Host: "198.51.100.30", APIKeyHash: "hash-maintenance", Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&node).Error)
	logs := &NodeLogService{db: db}

	incident := maintenanceEventForTest(node.ID, "event-incident", "open", "P2")
	recorded, err := logs.RecordAgentMaintenanceEvent(agentcontrol.NodeKindProxy, node.ID, incident)
	require.NoError(t, err)
	assert.True(t, recorded)
	recorded, err = logs.RecordAgentMaintenanceEvent(agentcontrol.NodeKindProxy, node.ID, incident)
	require.NoError(t, err)
	assert.False(t, recorded, "a resend is not stored again")

	// Concurrent deliveries of one event store it once.
	concurrent := maintenanceEventForTest(node.ID, "event-concurrent", "degraded", "P0")
	var wg sync.WaitGroup
	results := make(chan bool, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recorded, err := logs.RecordAgentMaintenanceEvent(agentcontrol.NodeKindProxy, node.ID, concurrent)
			if err == nil {
				results <- recorded
			}
		}()
	}
	wg.Wait()
	close(results)
	stored := 0
	for recorded := range results {
		if recorded {
			stored++
		}
	}
	assert.Equal(t, 1, stored)

	recovery := maintenanceEventForTest(node.ID, "event-recovery", "recovered", "P1")
	recorded, err = logs.RecordAgentMaintenanceEvent(agentcontrol.NodeKindProxy, node.ID, recovery)
	require.NoError(t, err)
	assert.True(t, recorded)

	var rows []model.NodeLog
	require.NoError(t, db.Where("node_id = ? AND source = ?", node.ID, NodeLogSourceMaintenance).Order("trace_id").Find(&rows).Error)
	require.Len(t, rows, 3)
	levels := map[string]string{}
	for _, row := range rows {
		levels[row.TraceID] = row.Level
		var event agentcontrol.MaintenanceEvent
		require.NoError(t, json.Unmarshal([]byte(row.FieldsJSON), &event))
		assert.Equal(t, row.TraceID, event.EventID)
		require.NotNil(t, row.LoggedAt)
		assert.True(t, row.LoggedAt.Equal(event.OccurredAt))
	}
	assert.Equal(t, map[string]string{
		"event-incident": NodeLogLevelWarning, "event-concurrent": NodeLogLevelError, "event-recovery": NodeLogLevelInfo,
	}, levels)
	var claims int64
	require.NoError(t, db.Model(&model.AgentReportBatch{}).Where("kind = ?", agentreports.KindMaintenance).Count(&claims).Error)
	assert.Equal(t, int64(3), claims)

	_, err = logs.RecordAgentMaintenanceEvent(agentcontrol.NodeKindProxy, node.ID+100, maintenanceEventForTest(node.ID, "event-gone", "open", "P3"))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, db.Model(&model.AgentReportBatch{}).Where("kind = ?", agentreports.KindMaintenance).Count(&claims).Error)
	assert.Equal(t, int64(3), claims, "a refused event leaves no claim")
}

func TestRecordAgentMaintenanceEventOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared&_pragma=busy_timeout(5000)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeLog{}, &model.AgentReportBatch{}))
	recordMaintenanceOnce(t, db)

	assert.Equal(t, "maintenance open: plugin machine-telemetry instance machine-telemetry HEALTH_CHECK_FAILED (P2)",
		maintenanceLogMessage(maintenanceEventForTest(1, "e", "open", "P2")))
	withSummary := maintenanceEventForTest(1, "e", "open", "P3")
	withSummary.RedactedSummary = " probe timed out "
	assert.Equal(t, NodeLogLevelInfo, maintenanceLogLevel(withSummary))
	assert.Contains(t, maintenanceLogMessage(withSummary), "(P3): probe timed out")
}

// On PostgreSQL too, an event is stored once per node and event id, also
// when deliveries race.
func TestPostgresAgentMaintenanceEventRecordedOnce(t *testing.T) {
	db := openPostgresTestDB(t, &model.Node{}, &model.NodeLog{}, &model.AgentReportBatch{})
	recordMaintenanceOnce(t, db)
}
