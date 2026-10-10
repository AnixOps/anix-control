package service

import (
	"testing"

	appconfig "github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fluxModels are the GORM models of forwardlegacy.DropTables.
func fluxModels() []any {
	return []any{
		&model.ForwardTunnel{}, &model.ForwardUserTunnel{}, &model.Forward{}, &model.ForwardPortBinding{},
		&model.SpeedLimit{}, &model.ForwardRuntimeJob{}, &model.ForwardTrafficCursor{}, &model.ForwardAgentBridgeTask{},
		&model.ForwardRule{}, &model.ForwardRoute{}, &model.ForwardLog{}, &model.ForwardStats{},
	}
}

// openFluxDB opens a v4.1 database: the flux tables and keep. The tables
// named in drop are then dropped, as `forward legacy drop` does.
func openFluxDB(t *testing.T, drop []string, keep ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(append(fluxModels(), keep...)...))
	for _, table := range drop {
		require.NoError(t, db.Migrator().DropTable(table), table)
	}
	return db
}

// The settings seeding at start (bootstrapDatabase) normalizes the retired
// iptables backend in the flux tables. A dropped table has nothing to
// normalize: it used to fail the start with "no such table: v2_forward".
func TestInitForwardRuntimeSystemConfigAfterTheFluxTablesWereDropped(t *testing.T) {
	const ipt = model.ForwardRuntimeBackendIptablesAnsible
	const nft = model.ForwardRuntimeBackendNftablesAnsible
	previous := appconfig.Get()
	appconfig.Set(nil)
	t.Cleanup(func() { appconfig.Set(previous) })

	t.Run("every flux table is gone", func(t *testing.T) {
		db := openFluxDB(t, forwardlegacy.DropTables, &model.SystemConfig{})
		for _, table := range forwardlegacy.DropTables {
			require.False(t, db.Migrator().HasTable(table), table)
		}
		configService := NewSystemConfigService(db)
		require.NoError(t, configService.Set(forwardRuntimeBackendConfigKey, ipt, "string", forwardRuntimeConfigGroup, "legacy"))

		require.NoError(t, InitForwardRuntimeSystemConfig(db))

		// What does not live in a flux table is still normalized.
		value, err := configService.Get(forwardRuntimeBackendConfigKey)
		require.NoError(t, err)
		assert.Equal(t, nft, value)
	})

	// One table missing at a time (a release that removed it first, F5d):
	// the statements on the others still run.
	for _, missing := range []string{"v2_forward", "v2_forward_runtime_job", "v2_forward_traffic_cursor"} {
		t.Run(missing+" is gone", func(t *testing.T) {
			db := openFluxDB(t, []string{missing}, &model.SystemConfig{})
			require.False(t, db.Migrator().HasTable(missing))
			if missing != "v2_forward" {
				require.NoError(t, db.Create(&model.Forward{UserID: 1, Name: "f", TunnelID: 1, InPort: 20001, RemoteAddr: "198.51.100.7:443", RuntimeBackend: ipt}).Error)
			}
			if missing != "v2_forward_runtime_job" {
				require.NoError(t, db.Create(&model.ForwardRuntimeJob{Backend: ipt, Action: model.ForwardRuntimeJobActionCreate, Status: model.ForwardRuntimeJobStatusPending}).Error)
			}
			if missing != "v2_forward_traffic_cursor" {
				require.NoError(t, db.Create(&model.ForwardTrafficCursor{ForwardID: 9, Backend: ipt, UploadTotal: 1}).Error)
			}

			require.NoError(t, InitForwardRuntimeSystemConfig(db))

			if missing != "v2_forward" {
				var backend string
				require.NoError(t, db.Model(&model.Forward{}).Select("runtime_backend").Scan(&backend).Error)
				assert.Equal(t, nft, backend)
			}
			if missing != "v2_forward_runtime_job" {
				var backend string
				require.NoError(t, db.Model(&model.ForwardRuntimeJob{}).Select("backend").Scan(&backend).Error)
				assert.Equal(t, nft, backend)
			}
			if missing != "v2_forward_traffic_cursor" {
				var backend string
				require.NoError(t, db.Model(&model.ForwardTrafficCursor{}).Select("backend").Scan(&backend).Error)
				assert.Equal(t, nft, backend)
			}
		})
	}
}

// Every task result a legacy agent reports is looked up in the bridge task
// table, which a dropped database no longer has: the lookup answers "not a
// bridge task" (it answered an error, so the result route answered 500).
func TestLookupBridgeTaskAfterTheFluxTablesWereDropped(t *testing.T) {
	db := openFluxDB(t, forwardlegacy.DropTables)
	require.False(t, db.Migrator().HasTable(&model.ForwardAgentBridgeTask{}))

	task, err := NewForwardAgentBridgeService(db).LookupBridgeTask("forward-runtime-job-1")
	require.NoError(t, err)
	assert.Nil(t, task)
}

// A clean agent that is still running on an abandoned node keeps sending
// heartbeats: it is seen, and it has no job to run.
func TestCleanAgentHeartbeatAfterTheFluxTablesWereDropped(t *testing.T) {
	db := openFluxDB(t, forwardlegacy.DropTables, &model.ForwardNode{}, &model.ForwardCleanAgent{})
	node := model.ForwardNode{Name: "abandoned", Type: model.ForwardNodeTypeRelay, Host: "192.0.2.20", Port: 22, APIToken: "node-token", Enabled: true}
	require.NoError(t, db.Create(&node).Error)
	issued, err := CreateCleanAgentTokenTx(db, ForwardCleanAgentCreateInput{Name: "old-agent", NodeID: &node.ID})
	require.NoError(t, err)
	require.False(t, db.Migrator().HasTable(&model.ForwardRuntimeJob{}))

	actions, err := NewForwardCleanAgentService(db).Heartbeat(ForwardCleanAgentHeartbeatInput{AgentID: issued.Agent.ID, Token: issued.Token, Version: "0.1.0"})
	require.NoError(t, err)
	assert.Empty(t, actions)

	var agent model.ForwardCleanAgent
	require.NoError(t, db.First(&agent, issued.Agent.ID).Error)
	assert.Equal(t, model.ForwardCleanAgentStatusOnline, agent.Status)
	assert.NotNil(t, agent.LastSeen)
}
