package agentupgrade

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/lease"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ARC-003: once another Control process took the singleton lease, the
// campaign worker of the process that lost it changes nothing.
func TestWorkerWritesNothingAfterItsLeaseIsTaken(t *testing.T) {
	db := openSQLite(t)
	migrate(t, db)
	require.NoError(t, db.AutoMigrate(&lease.Record{}))
	campaign := model.AgentUpgradeCampaign{ID: "c-1", TargetVersion: "v5.0.9", ArtifactsJSON: "[]", BatchesJSON: "[]", ExcludeJSON: "{}", Status: model.AgentUpgradeRunning}
	require.NoError(t, db.Create(&campaign).Error)
	worker := &Worker{Service: &Service{DB: db}}

	results := make(chan error, 2)
	elector := &lease.Elector{DB: db, Name: "workers", Holder: "a", TTL: time.Minute}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go elector.Run(ctx, func(ctx context.Context) {
		leading := campaign
		results <- worker.update(ctx, &leading, "", func(c *model.AgentUpgradeCampaign) { c.StatusReason = "leader" })
		// Another process takes the lease over.
		now := time.Now().UTC()
		if err := lease.Release(context.Background(), db, "workers", "a", now); err != nil {
			results <- err
			return
		}
		if _, _, err := lease.TryAcquire(context.Background(), db, "workers", "b", time.Minute, now); err != nil {
			results <- err
			return
		}
		results <- worker.update(ctx, &leading, "", func(c *model.AgentUpgradeCampaign) { c.StatusReason = "stale leader" })
		<-ctx.Done()
	})
	for _, want := range []error{nil, lease.ErrLost} {
		select {
		case err := <-results:
			assert.ErrorIs(t, err, want)
		case <-time.After(5 * time.Second):
			t.Fatal("elector never led")
		}
	}
	var stored model.AgentUpgradeCampaign
	require.NoError(t, db.First(&stored, "id = ?", "c-1").Error)
	assert.Equal(t, "leader", stored.StatusReason)
}
