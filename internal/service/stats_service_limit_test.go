package service

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStatsServiceUserTrafficRankingClampsRequestLimits(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "stats-ranking-limit.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TrafficLog{}))

	svc := &StatsService{db: db}
	now := time.Now()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.Local).Unix()

	trafficUsers := make([]model.User, 250)
	for i := range trafficUsers {
		trafficUsers[i] = model.User{
			Email: fmt.Sprintf("rank-limit-traffic-%03d@example.com", i),
			Token: fmt.Sprintf("rank-limit-token-%03d", i),
			UUID:  fmt.Sprintf("rank-limit-uuid-%03d", i),
		}
	}
	require.NoError(t, db.CreateInBatches(&trafficUsers, 100).Error)

	logs := make([]model.TrafficLog, len(trafficUsers))
	for i := range trafficUsers {
		logs[i] = model.TrafficLog{
			UserID:     trafficUsers[i].ID,
			ServerID:   1,
			ServerType: "node",
			U:          int64(i + 1),
			D:          0,
			Rate:       1,
			LogAt:      currentHour + int64(i%60),
		}
	}
	require.NoError(t, db.CreateInBatches(&logs, 100).Error)

	trafficOnly, err := svc.GetUserTrafficRanking(maxHourlyWindow*2, 999999, false)
	require.NoError(t, err)
	require.Len(t, trafficOnly, 200)
	require.Equal(t, int64(250), trafficOnly[0].Traffic)
	require.Equal(t, int64(51), trafficOnly[len(trafficOnly)-1].Traffic)

	zeroUsers := make([]model.User, 1005)
	for i := range zeroUsers {
		zeroUsers[i] = model.User{
			Email: fmt.Sprintf("rank-limit-zero-%04d@example.com", i),
			Token: fmt.Sprintf("zero-limit-token-%04d", i),
			UUID:  fmt.Sprintf("zero-limit-uuid-%04d", i),
		}
	}
	require.NoError(t, db.CreateInBatches(&zeroUsers, 100).Error)

	withZeroUsers, err := svc.GetUserTrafficRanking(maxHourlyWindow*2, 999999, true)
	require.NoError(t, err)
	require.Len(t, withZeroUsers, 1000)
	require.Equal(t, int64(250), withZeroUsers[0].Traffic)
}
