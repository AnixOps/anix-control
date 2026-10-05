package service

import (
	"fmt"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// activityService is a ServerService on a database with the tables a traffic
// report and an alive report touch, and subscribers 1 to 3.
func activityService(t *testing.T) (*ServerService, *gorm.DB) {
	t.Helper()
	cache.InitMemory()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.User{}, &model.TrafficLog{}, &model.StatServer{},
		&model.SubscriberRequest{}, &model.SubscriberChange{}, &model.UserActivity{}, &model.AgentReportBatch{},
		&model.IdentityRevocation{}, &model.WireGuardPeer{}))
	require.NoError(t, db.Create(&model.Node{ID: 7, Name: "entry", Host: "entry.example.com"}).Error)
	for _, id := range []uint{1, 2, 3} {
		require.NoError(t, db.Create(&model.User{ID: id, Email: fmt.Sprintf("activity%d@example.test", id), Token: fmt.Sprintf("tok%d", id), UUID: fmt.Sprintf("uuid%d", id)}).Error)
	}
	return &ServerService{db: db}, db
}

func seenOnline(t *testing.T, db *gorm.DB) []uint {
	t.Helper()
	var ids []uint
	require.NoError(t, db.Model(&model.UserActivity{}).Order("user_id").Pluck("user_id", &ids).Error)
	return ids
}

// A legacy traffic report marks the users it carried traffic for, and no one
// whose entry is empty.
func TestTrafficReportRecordsTheUsersSeenOnline(t *testing.T) {
	svc, db := activityService(t)
	require.NoError(t, svc.RecordNodeTrafficReport(model.ServerType("vmess"), 7, map[uint][2]int64{1: {10, 20}, 2: {0, 0}, 3: {0, 5}}, 1))
	require.Equal(t, []uint{1, 3}, seenOnline(t, db))
	seen, err := subscriber.LastOnline(db, []uint{1, 2, 3})
	require.NoError(t, err)
	require.NotZero(t, seen[1])
	require.Equal(t, seen[1], seen[3])
}

// A report that is refused leaves no activity: nothing was applied.
func TestRefusedTrafficReportRecordsNothing(t *testing.T) {
	svc, db := activityService(t)
	require.Error(t, svc.RecordNodeTrafficReport(model.ServerType("vmess"), 7, map[uint][2]int64{1: {10, 20}, 2: {-1, 5}}, 1))
	require.Empty(t, seenOnline(t, db))
}

// An Agent's traffic batch marks its users once, and a batch the Agent sends
// again, which the kernel had applied, marks no one.
func TestAgentTrafficReportRecordsTheUsersSeenOnline(t *testing.T) {
	svc, db := activityService(t)
	applied, err := svc.RecordAgentTrafficReport("node", 7, "batch-1", map[uint][2]int64{2: {1, 1}}, 1)
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, []uint{2}, seenOnline(t, db))

	require.NoError(t, db.Exec("DELETE FROM v4_kernel_user_activity").Error)
	subscriber.ResetActivityThrottleForTest(db)
	applied, err = svc.RecordAgentTrafficReport("node", 7, "batch-1", map[uint][2]int64{2: {1, 1}}, 1)
	require.NoError(t, err)
	require.False(t, applied)
	require.Empty(t, seenOnline(t, db))
}

// An alive report marks the users with a connection, and not the ones it
// lists with none.
func TestAliveReportRecordsTheUsersSeenOnline(t *testing.T) {
	svc, db := activityService(t)
	require.NoError(t, svc.UpdateOnlineStatus(model.ServerType("vmess"), 7, map[uint][]string{
		1: {"198.51.100.1", "198.51.100.2"}, 2: {}, 3: {"198.51.100.3"},
	}))
	require.Equal(t, []uint{1, 3}, seenOnline(t, db))
}

// Deleting a user drops their time.
func TestDeletingAUserDropsTheirActivity(t *testing.T) {
	svc, db := activityService(t)
	require.NoError(t, svc.UpdateOnlineStatus(model.ServerType("vmess"), 7, map[uint][]string{1: {"198.51.100.1"}, 2: {"198.51.100.2"}}))
	require.NoError(t, (&UserService{db: db}).Delete(1))
	require.Equal(t, []uint{2}, seenOnline(t, db))
}
