package exitpolicy

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestExitPolicy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "exit.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NodeExitPolicy{}))
	now := time.Date(2026, 11, 7, 8, 0, 0, 0, time.UTC)

	// Off by default: no member, nothing recorded.
	member, err := NodeMember(db, 3)
	require.NoError(t, err)
	require.Nil(t, member)
	require.NoError(t, RecordBlocked(db, 3, 5, now))
	row, err := Get(db, 3)
	require.NoError(t, err)
	require.False(t, row.BlockMainland)
	require.Zero(t, row.MainlandBlockedTotal)

	row, err = Set(db, 3, true)
	require.NoError(t, err)
	require.True(t, row.BlockMainland)
	member, err = NodeMember(db, 3)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"block_mainland": true}, member)
	require.NoError(t, RecordBlocked(db, 3, 5, now))
	require.NoError(t, RecordBlocked(db, 3, 2, now.Add(time.Minute)))
	row, err = Get(db, 3)
	require.NoError(t, err)
	require.Equal(t, int64(7), row.MainlandBlockedTotal)
	require.Equal(t, now.Add(time.Minute), row.MainlandBlockedAt.UTC())

	// Off again keeps the counter and stops counting.
	row, err = Set(db, 3, false)
	require.NoError(t, err)
	require.False(t, row.BlockMainland)
	require.Equal(t, int64(7), row.MainlandBlockedTotal)
	require.NoError(t, RecordBlocked(db, 3, 5, now))
	member, err = NodeMember(db, 3)
	require.NoError(t, err)
	require.Nil(t, member)
	row, err = Get(db, 3)
	require.NoError(t, err)
	require.Equal(t, int64(7), row.MainlandBlockedTotal)
}
