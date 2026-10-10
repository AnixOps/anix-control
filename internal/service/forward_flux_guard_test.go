package service

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// catalogProbe counts the catalog lookups a database runs
// (Migrator().HasTable reads sqlite_master) and, with fail set, breaks each
// one the way a lost connection or a statement timeout would. HasTable
// cannot tell that from a table that is not there: it answers false.
type catalogProbe struct {
	queries atomic.Int32
	fail    atomic.Bool
}

func newCatalogProbe(t *testing.T, db *gorm.DB) *catalogProbe {
	t.Helper()
	probe := &catalogProbe{}
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("test:catalog_probe", func(tx *gorm.DB) {
		if !strings.Contains(tx.Statement.SQL.String(), "sqlite_master") {
			return
		}
		probe.queries.Add(1)
		if probe.fail.Load() {
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString("SELECT count(*) FROM catalog_lookup_failed")
			tx.Statement.Vars = nil
		}
	}))
	return probe
}

// failStatements makes every query on table fail with err, as a statement
// that the database refuses for any reason but a missing table.
func failStatements(t *testing.T, db *gorm.DB, table string, err error) {
	t.Helper()
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:fail_"+table, func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			_ = tx.AddError(err)
		}
	}))
}

// LookupBridgeTask answers "not a bridge task" only for a table the
// database says is not there. A failed statement on a table that is there
// is an error: the result route then answers 500 and the agent retries,
// where a nil answer sent the report on as a diagnostic task, which
// created a row for an unknown id and acknowledged it with 200.
func TestLookupBridgeTaskKeepsFailuresOfAPresentTable(t *testing.T) {
	db := openFluxDB(t, nil)
	require.NoError(t, db.Create(&model.ForwardAgentBridgeTask{TaskID: "forward-runtime-job-7", RuntimeJobID: 7, NodeID: 3,
		Status: model.ForwardAgentBridgeTaskStatusPending}).Error)
	probe := newCatalogProbe(t, db)

	t.Run("a bridge task is found without a catalog lookup", func(t *testing.T) {
		task, err := NewForwardAgentBridgeService(db).LookupBridgeTask("forward-runtime-job-7")
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, uint(3), task.NodeID)
		assert.Zero(t, probe.queries.Load(), "the lookup is the statement, not a metadata query before it")
	})

	t.Run("an unknown task id is not a bridge task", func(t *testing.T) {
		task, err := NewForwardAgentBridgeService(db).LookupBridgeTask("diagnostic-task-1")
		require.NoError(t, err)
		assert.Nil(t, task)
	})

	t.Run("a cancelled context is an error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		task, err := NewForwardAgentBridgeService(db.WithContext(ctx)).LookupBridgeTask("forward-runtime-job-7")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, task)
	})

	t.Run("a failing catalog lookup changes nothing", func(t *testing.T) {
		probe.fail.Store(true)
		t.Cleanup(func() { probe.fail.Store(false) })
		task, err := NewForwardAgentBridgeService(db).LookupBridgeTask("forward-runtime-job-7")
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, "forward-runtime-job-7", task.TaskID)
	})

	t.Run("a statement the database refuses is an error", func(t *testing.T) {
		refused := errors.New("database is locked")
		failStatements(t, db, "v2_forward_agent_bridge_task", refused)
		task, err := NewForwardAgentBridgeService(db).LookupBridgeTask("forward-runtime-job-7")
		assert.ErrorIs(t, err, refused)
		assert.Nil(t, task)
	})
}

// The clean agent's heartbeat reads the pending jobs as the statement, and
// a failure of that statement is the error it was before the drop code: a
// 500 for the agent, which heartbeats again, not a quiet "no work".
func TestCleanAgentHeartbeatKeepsFailuresOfAPresentTable(t *testing.T) {
	db := openFluxDB(t, nil, &model.ForwardNode{}, &model.ForwardCleanAgent{})
	node := model.ForwardNode{Name: "relay", Type: model.ForwardNodeTypeRelay, Host: "192.0.2.30", Port: 22, APIToken: "node-token", Enabled: true}
	require.NoError(t, db.Create(&node).Error)
	issued, err := CreateCleanAgentTokenTx(db, ForwardCleanAgentCreateInput{Name: "agent", NodeID: &node.ID})
	require.NoError(t, err)
	nodeID := node.ID
	job := model.ForwardRuntimeJob{Backend: model.ForwardRuntimeBackendCleanAgent, Action: model.ForwardRuntimeJobActionSync,
		ResourceType: "panel_forward", NodeID: &nodeID, Status: model.ForwardRuntimeJobStatusPending, Payload: `{"action":"sync"}`}
	require.NoError(t, db.Create(&job).Error)
	probe := newCatalogProbe(t, db)
	heartbeat := func() ([]ForwardCleanAgentAction, error) {
		return NewForwardCleanAgentService(db).Heartbeat(ForwardCleanAgentHeartbeatInput{AgentID: issued.Agent.ID, Token: issued.Token, Version: "0.1.0"})
	}

	t.Run("a failing catalog lookup does not hide the job", func(t *testing.T) {
		probe.fail.Store(true)
		t.Cleanup(func() { probe.fail.Store(false) })
		actions, err := heartbeat()
		require.NoError(t, err)
		require.Len(t, actions, 1)
		assert.Equal(t, job.ID, actions[0].JobID)
	})

	t.Run("a job is claimed without a catalog lookup", func(t *testing.T) {
		require.NoError(t, db.Model(&model.ForwardRuntimeJob{}).Where("id = ?", job.ID).
			Updates(map[string]any{"status": model.ForwardRuntimeJobStatusPending, "agent_id": nil}).Error)
		probe.queries.Store(0)
		actions, err := heartbeat()
		require.NoError(t, err)
		require.Len(t, actions, 1)
		assert.Zero(t, probe.queries.Load(), "the heartbeat runs no metadata query")

		var claimed model.ForwardRuntimeJob
		require.NoError(t, db.First(&claimed, job.ID).Error)
		assert.Equal(t, model.ForwardRuntimeJobStatusRunning, claimed.Status)
		require.NotNil(t, claimed.AgentID)
		assert.Equal(t, issued.Agent.ID, *claimed.AgentID)
	})

	t.Run("a statement the database refuses is an error and the agent is not marked seen", func(t *testing.T) {
		var before model.ForwardCleanAgent
		require.NoError(t, db.First(&before, issued.Agent.ID).Error)
		require.NotNil(t, before.LastSeen)
		time.Sleep(10 * time.Millisecond)

		refused := errors.New("database is locked")
		failStatements(t, db, "v2_forward_runtime_job", refused)
		actions, err := heartbeat()
		assert.ErrorIs(t, err, refused)
		assert.Empty(t, actions)

		var after model.ForwardCleanAgent
		require.NoError(t, db.First(&after, issued.Agent.ID).Error)
		require.NotNil(t, after.LastSeen)
		assert.True(t, after.LastSeen.Equal(*before.LastSeen), "a heartbeat that failed leaves the agent as it was")
	})
}
