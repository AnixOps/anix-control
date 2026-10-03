package kernelnodeops

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The desired configuration (section 5.5): the same rows render the same
// document and hash; the revision starts at 1, grows by one when the hash
// changes and never otherwise; a rebuild of an unchanged configuration
// refreshes built_at and keeps the revision.
func TestDesiredConfigRevisionIsMonotonicAndHashIsStable(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		proxy := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}
		first, err := BuildDesiredConfig(db, proxy)
		require.NoError(t, err)
		again, err := BuildDesiredConfig(db, proxy)
		require.NoError(t, err)
		assert.Equal(t, first.Hash, again.Hash, "the same configuration yields the same hash")
		assert.Equal(t, string(first.JSON), string(again.JSON))
		assert.Len(t, first.Hash, 64)
		assert.Equal(t, DesiredConfigFormat, first.Format)
		assert.Contains(t, string(first.JSON), `"kind":"proxy"`)
		assert.Contains(t, string(first.JSON), `"protocols":[{`)

		t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		row, changed, err := StoreDesiredConfig(ctx, db, first, t0)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, uint64(1), row.Revision)
		assert.Equal(t, first.Hash, row.ConfigHash)

		row, changed, err = StoreDesiredConfig(ctx, db, again, t0.Add(time.Minute))
		require.NoError(t, err)
		assert.False(t, changed, "an unchanged configuration is not a change")
		assert.Equal(t, uint64(1), row.Revision, "the revision stays")
		assert.Equal(t, t0.Add(time.Minute), row.BuiltAt.UTC(), "the rebuild is stamped")

		require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", 5).Update("port", 8443).Error)
		third, err := BuildDesiredConfig(db, proxy)
		require.NoError(t, err)
		assert.NotEqual(t, first.Hash, third.Hash)
		row, changed, err = StoreDesiredConfig(ctx, db, third, t0.Add(2*time.Minute))
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, uint64(2), row.Revision)

		// Back to the first configuration: a change again, the revision
		// keeps growing, never returns to 1.
		require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", 5).Update("port", 443).Error)
		back, err := BuildDesiredConfig(db, proxy)
		require.NoError(t, err)
		assert.Equal(t, first.Hash, back.Hash)
		row, changed, err = StoreDesiredConfig(ctx, db, back, t0.Add(3*time.Minute))
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, uint64(3), row.Revision)

		stored, found, err := LoadDesiredConfig(ctx, db, proxy)
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, uint64(3), stored.Revision)
		assert.Equal(t, string(back.JSON), stored.ConfigJSON)

		// Another node kind of the same id has its own row.
		forward := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 10}
		forwardConfig, err := BuildDesiredConfig(db, forward)
		require.NoError(t, err)
		assert.Contains(t, string(forwardConfig.JSON), `"kind":"forward"`)
		assert.Contains(t, string(forwardConfig.JSON), `"legacy_rules":[{`)
		assert.Contains(t, string(forwardConfig.JSON), `"tunnels":[{`)
		assert.NotContains(t, string(forwardConfig.JSON), "api_token")
		row, changed, err = StoreDesiredConfig(ctx, db, forwardConfig, t0)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, uint64(1), row.Revision)
		_, found, err = LoadDesiredConfig(ctx, db, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 10})
		require.NoError(t, err)
		assert.False(t, found)

		_, err = BuildDesiredConfig(db, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 999})
		assert.ErrorIs(t, err, ErrNodeGone)
	})
}

// Concurrent stores of one node serialize on the row's revision: every
// change is counted once and the revision ends where the changes do.
func TestDesiredConfigStoreSerializesWriters(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 2}
		base, err := BuildDesiredConfig(db, node)
		require.NoError(t, err)
		variants := make([]*DesiredConfig, 0, 4)
		for i := 0; i < 4; i++ {
			document := map[string]any{"kind": "proxy", "variant": i}
			cfg, err := newDesiredConfig(node, DesiredConfigFormat, document)
			require.NoError(t, err)
			variants = append(variants, cfg)
		}
		_, _, err = StoreDesiredConfig(ctx, db, base, time.Now())
		require.NoError(t, err)

		var wg sync.WaitGroup
		results := make(chan bool, 16)
		for round := 0; round < 4; round++ {
			for _, cfg := range variants {
				wg.Add(1)
				go func(cfg *DesiredConfig) {
					defer wg.Done()
					_, changed, err := StoreDesiredConfig(ctx, db, cfg, time.Now())
					if err != nil {
						// SQLite can refuse a concurrent writer outright; the
						// row's state stays consistent either way.
						results <- false
						return
					}
					results <- changed
				}(cfg)
			}
		}
		wg.Wait()
		close(results)
		changes := 0
		for changed := range results {
			if changed {
				changes++
			}
		}
		stored, found, err := LoadDesiredConfig(ctx, db, node)
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, uint64(1+changes), stored.Revision, "one revision per change that won")
		assert.True(t, strings.HasPrefix(stored.ConfigJSON, `{"kind":"proxy","variant":`))
	})
}
