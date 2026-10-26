package service

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A relay node forwards a port to its parent and never heartbeats itself: it
// serves subscribers while the landing node it forwards to is online.
func TestRelayNodeServesWhileItsLandingNodeIsOnline(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	svc := &SubscriptionService{db: db}

	fresh := time.Now().Unix()
	stale := fresh - 3600
	uintPtr := func(v uint) *uint { return &v }
	nodes := []model.Node{
		{ID: 1, Name: "landing online", LastCheckAt: &fresh},
		{ID: 2, Name: "landing offline", LastCheckAt: &stale},
		{ID: 3, Name: "relay to online", ParentID: uintPtr(1)},
		{ID: 4, Name: "relay to offline", ParentID: uintPtr(2)},
		{ID: 5, Name: "relay of a relay", ParentID: uintPtr(3)},
		{ID: 6, Name: "relay with its own stale heartbeat", ParentID: uintPtr(1), LastCheckAt: &stale},
		{ID: 7, Name: "relay to a missing node", ParentID: uintPtr(99)},
		{ID: 8, Name: "standalone, never seen"},
	}
	for i := range nodes {
		nodes[i].APIKey = nodes[i].Name
	}
	require.NoError(t, db.Create(&nodes).Error)

	liveness := make(map[uint]bool)
	want := map[uint]bool{1: true, 2: false, 3: true, 4: false, 5: true, 6: false, 7: false, 8: false}
	for i := range nodes {
		require.Equal(t, want[nodes[i].ID], svc.nodeServing(&nodes[i], liveness), nodes[i].Name)
	}
}
