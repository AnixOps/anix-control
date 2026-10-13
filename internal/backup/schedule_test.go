package backup

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyTimeIsStableAndSpreadOverTheWindow(t *testing.T) {
	seen := map[time.Duration]bool{}
	for i := 0; i < 200; i++ {
		domain := "panel-" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".example.com"
		offset := DailyOffset(domain)
		assert.GreaterOrEqual(t, offset, 3*time.Hour)
		assert.Less(t, offset, 5*time.Hour)
		assert.Zero(t, offset%time.Minute)
		assert.Equal(t, offset, DailyOffset(domain), "the same domain, the same time")
		seen[offset] = true
	}
	assert.Greater(t, len(seen), 50, "a fleet does not back up at one minute")
	assert.Equal(t, DailyOffset("Panel.Example.com"), DailyOffset("panel.example.com"))
}

func TestNextDaily(t *testing.T) {
	zone := time.FixedZone("UTC+8", 8*3600)
	offset := DailyOffset("panel.example.com")
	before := time.Date(2026, 10, 13, 1, 0, 0, 0, zone)
	assert.Equal(t, time.Date(2026, 10, 13, 0, 0, 0, 0, zone).Add(offset), NextDaily("panel.example.com", before))
	after := time.Date(2026, 10, 13, 6, 0, 0, 0, zone)
	assert.Equal(t, time.Date(2026, 10, 14, 0, 0, 0, 0, zone).Add(offset), NextDaily("panel.example.com", after))
	at := time.Date(2026, 10, 13, 0, 0, 0, 0, zone).Add(offset)
	assert.Equal(t, at.AddDate(0, 0, 1), NextDaily("panel.example.com", at), "a time that has come is tomorrow's")
}

// The worker verifies what the start left unverified, backs up soon after
// its start when the last daily backup is old, then daily.
func TestWorkerRunsTheDailyBackup(t *testing.T) {
	p := newPanel(t)
	m := p.manager()
	_ = p.write(t, TypePreMigration, time.Now().Add(-time.Minute))
	ticks := make(chan time.Time)
	waits := make(chan time.Duration, 4)
	worker := &Worker{Manager: m, Seed: "panel.example.com", After: func(d time.Duration) <-chan time.Time {
		waits <- d
		return ticks
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	first := <-waits
	assert.InDelta(t, float64(catchUpDelay), float64(first), float64(5*time.Second), "no daily backup yet: one soon after the start")
	rows := backupRows(t, p.db)
	require.Len(t, rows, 1)
	assert.Equal(t, model.BackupVerifyPassed, rows[0].VerifyStatus, "the pre-migration bundle is verified at the start")

	ticks <- time.Now()
	second := <-waits
	assert.Greater(t, second, time.Duration(0))
	assert.LessOrEqual(t, second, 24*time.Hour)
	rows = backupRows(t, p.db)
	require.Len(t, rows, 2)
	assert.Equal(t, TypeDaily, rows[0].Type)
	assert.Equal(t, model.BackupVerifyPassed, rows[0].VerifyStatus, "the daily bundle is verified at once")
	cancel()
	<-done
}
