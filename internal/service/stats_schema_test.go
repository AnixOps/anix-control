package service

import (
	"testing"

	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type StatsSchemaTestSuite struct {
	ServiceTestSuite
}

// TestEnsureStatsSchema_CreatesTablesAndIsIdempotent 验证缺失时建表、再次调用幂等、nil 安全
func (s *StatsSchemaTestSuite) TestEnsureStatsSchema_CreatesTablesAndIsIdempotent() {
	db := database.Get()

	tables := []any{
		&model.TrafficLog{},
		&model.OnlineLog{},
		&model.StatUser{},
		&model.StatServer{},
	}

	// 调用后四张表都应存在 (即便此前某张缺失, 如 OnlineLog)
	assert.NoError(s.T(), EnsureStatsSchema(db))
	for _, t := range tables {
		assert.True(s.T(), db.Migrator().HasTable(t))
	}

	// 幂等: 表已存在时再次调用不应报错, 表仍在
	assert.NoError(s.T(), EnsureStatsSchema(db))
	for _, t := range tables {
		assert.True(s.T(), db.Migrator().HasTable(t))
	}

	// nil db 安全返回
	assert.NoError(s.T(), EnsureStatsSchema(nil))
}

// TestEnsureStatsSchema_CompletesLegacyTrafficLogTable verifies production
// upgrades where v2_server_log already exists but lacks newer stats columns.
func (s *StatsSchemaTestSuite) TestEnsureStatsSchema_CompletesLegacyTrafficLogTable() {
	db := database.Get()

	assert.NoError(s.T(), db.Migrator().DropTable(&model.TrafficLog{}))
	assert.NoError(s.T(), db.Exec(`
		CREATE TABLE v2_server_log (
			id integer primary key autoincrement,
			user_id integer,
			server_id integer
		)
	`).Error)
	assert.True(s.T(), db.Migrator().HasTable(&model.TrafficLog{}))
	assert.False(s.T(), db.Migrator().HasColumn(&model.TrafficLog{}, "rate"))
	assert.False(s.T(), db.Migrator().HasColumn(&model.TrafficLog{}, "log_at"))

	assert.NoError(s.T(), EnsureStatsSchema(db))

	for _, column := range []string{"server_type", "u", "d", "rate", "log_at", "created_at"} {
		assert.True(s.T(), db.Migrator().HasColumn(&model.TrafficLog{}, column), column)
	}
	assert.True(s.T(), db.Migrator().HasIndex(&model.TrafficLog{}, "idx_server_log_log_at_user_id"))
	assert.NoError(s.T(), db.Create(&model.TrafficLog{
		UserID:     1,
		ServerID:   1,
		ServerType: "node",
		U:          100,
		D:          200,
		Rate:       1,
		LogAt:      123456,
	}).Error)
}

func TestStatsSchemaSuite(t *testing.T) {
	suite.Run(t, new(StatsSchemaTestSuite))
}
