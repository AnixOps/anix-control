package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/cache"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/online"
	"github.com/AnixOps/anix-control/v5/internal/subscriber"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
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
		&model.IdentityRevocation{}, &model.WireGuardPeer{}, &model.UserSS2022Key{}))
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
	node := &model.Node{ID: 7, Rate: 1}
	applied, refused, err := svc.RecordAgentTrafficReport("node", node, "batch-1", map[uint][2]int64{2: {1, 1}})
	require.NoError(t, err)
	require.True(t, applied)
	require.Empty(t, refused)
	require.Equal(t, []uint{2}, seenOnline(t, db))

	require.NoError(t, db.Exec("DELETE FROM v4_kernel_user_activity").Error)
	subscriber.ResetActivityThrottleForTest(db)
	applied, _, err = svc.RecordAgentTrafficReport("node", node, "batch-1", map[uint][2]int64{2: {1, 1}})
	require.NoError(t, err)
	require.False(t, applied)
	require.Empty(t, seenOnline(t, db))
}

// A node's traffic counts only for the users it serves: those of its plan
// group, active or not; another group's users and unknown ids are refused
// and the rest of the batch counts (CT-004).
func TestAgentTrafficReportCountsOnlyTheNodesUsers(t *testing.T) {
	svc, db := activityService(t)
	group, other := uint(5), uint(6)
	require.NoError(t, db.Model(&model.User{}).Where("id IN ?", []uint{1, 2}).Update("group_id", group).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 3).Update("group_id", other).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 2).Update("banned", 1).Error)

	node := &model.Node{ID: 7, Rate: 2, GroupID: &group}
	applied, refused, err := svc.RecordAgentTrafficReport("node", node, "batch-1",
		map[uint][2]int64{1: {10, 20}, 2: {1, 1}, 3: {100, 100}, 99: {5, 5}})
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, []uint{3, 99}, refused)

	used := func(id uint) [2]int64 {
		var user model.User
		require.NoError(t, db.First(&user, id).Error)
		return [2]int64{user.U, user.D}
	}
	assert.Equal(t, [2]int64{20, 40}, used(1), "counted at the node's rate")
	assert.Equal(t, [2]int64{2, 2}, used(2), "a banned user's traffic still counts")
	assert.Equal(t, [2]int64{0, 0}, used(3), "another group's user is untouched")

	// A node without a group serves every known user.
	applied, refused, err = svc.RecordAgentTrafficReport("node", &model.Node{ID: 7, Rate: 1}, "batch-2",
		map[uint][2]int64{3: {1, 1}, 99: {1, 1}})
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, []uint{99}, refused)
	assert.Equal(t, [2]int64{1, 1}, used(3))
}

// An alive report marks the users with a connection, and not the ones it
// lists with none.
func TestAliveReportRecordsTheUsersSeenOnline(t *testing.T) {
	svc, db := activityService(t)
	online.Reset()
	require.True(t, svc.UpdateOnlineStatus(7, time.Time{}, map[uint][]string{
		1: {"198.51.100.1", "198.51.100.2"}, 2: {}, 3: {"198.51.100.3"},
	}))
	require.Equal(t, []uint{1, 3}, seenOnline(t, db))
}
