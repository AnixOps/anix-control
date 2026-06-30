package service

import (
	"testing"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
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

func TestStatsSchemaSuite(t *testing.T) {
	suite.Run(t, new(StatsSchemaTestSuite))
}
