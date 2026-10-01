package kernelnodeops

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A ConfigStatus is judged against the desired configuration: only the
// desired revision with its hash is applied or failed.
func TestConfigVerdict(t *testing.T) {
	desired := model.KernelNodeDesiredConfig{Revision: 4, ConfigHash: "abc"}
	for _, test := range []struct {
		name   string
		found  bool
		status *agentv1pb.ConfigStatus
		want   string
	}{
		{"applied", true, &agentv1pb.ConfigStatus{ConfigRevision: 4, ConfigHash: "abc", Applied: true}, model.ConfigVerdictApplied},
		{"failed", true, &agentv1pb.ConfigStatus{ConfigRevision: 4, ConfigHash: "abc", Error: "boom"}, model.ConfigVerdictFailed},
		{"stale", true, &agentv1pb.ConfigStatus{ConfigRevision: 3, ConfigHash: "abc", Applied: true}, model.ConfigVerdictStale},
		{"other hash", true, &agentv1pb.ConfigStatus{ConfigRevision: 4, ConfigHash: "abd", Applied: true}, model.ConfigVerdictMismatch},
		{"future revision", true, &agentv1pb.ConfigStatus{ConfigRevision: 5, ConfigHash: "abc", Applied: true}, model.ConfigVerdictMismatch},
		{"no desired configuration", false, &agentv1pb.ConfigStatus{ConfigRevision: 4, ConfigHash: "abc", Applied: true}, model.ConfigVerdictMismatch},
	} {
		assert.Equal(t, test.want, ConfigVerdict(desired, test.found, test.status), test.name)
	}
}

// A snapshot is answered by a status of its revision and hash, or by one
// the kernel verified as applied at a newer revision.
func TestAnswersSnapshot(t *testing.T) {
	snapshot := &agentv1pb.ConfigSnapshot{ConfigRevision: 4, ConfigHash: "abc"}
	report := func(revision uint64, hash, verdict string) agentstreams.ConfigStatusReport {
		return agentstreams.ConfigStatusReport{Status: &agentv1pb.ConfigStatus{ConfigRevision: revision, ConfigHash: hash}, Verdict: verdict}
	}
	assert.True(t, answersSnapshot(snapshot, report(4, "abc", model.ConfigVerdictApplied)))
	assert.True(t, answersSnapshot(snapshot, report(4, "abc", model.ConfigVerdictStale)), "its own revision, even when a newer one is desired")
	assert.True(t, answersSnapshot(snapshot, report(4, "abc", model.ConfigVerdictFailed)))
	assert.False(t, answersSnapshot(snapshot, report(4, "abd", model.ConfigVerdictMismatch)), "another hash")
	assert.False(t, answersSnapshot(snapshot, report(3, "abc", model.ConfigVerdictStale)), "an older revision")
	assert.True(t, answersSnapshot(snapshot, report(5, "def", model.ConfigVerdictApplied)), "a newer configuration, applied")
	assert.False(t, answersSnapshot(snapshot, report(5, "def", model.ConfigVerdictFailed)))
	assert.False(t, answersSnapshot(snapshot, report(5, "def", model.ConfigVerdictMismatch)))
	assert.False(t, answersSnapshot(snapshot, report(5, "def", "")))
}

func TestTruncateUTF8(t *testing.T) {
	assert.Equal(t, "short", truncateUTF8("short", 10))
	assert.Equal(t, "ab", truncateUTF8("abé", 3), "never half a rune")
	assert.Equal(t, strings.Repeat("x", 1024), truncateUTF8(strings.Repeat("x", 2000), 1024))
}

// RecordConfigStatus keeps one row per node: every status as the last
// report, and the applied revision only from a verified applied status.
func TestRecordConfigStatus(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		proxy := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}
		now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

		verdict, err := RecordConfigStatus(ctx, db, proxy, "s1", &agentv1pb.ConfigStatus{ConfigRevision: 1, ConfigHash: "x", Applied: true}, now)
		require.NoError(t, err)
		assert.Equal(t, model.ConfigVerdictMismatch, verdict, "no desired configuration yet")
		_, err = RecordConfigStatus(ctx, db, proxy, "s1", nil, now)
		require.Error(t, err)

		row, changed, err := RefreshDesiredConfig(ctx, db, proxy, now)
		require.NoError(t, err)
		assert.True(t, changed)
		again, changed, err := RefreshDesiredConfig(ctx, db, proxy, now.Add(time.Minute))
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, row.Revision, again.Revision)
		assert.Equal(t, row.BuiltAt.UTC(), again.BuiltAt.UTC(), "an unchanged refresh writes nothing")

		verdict, err = RecordConfigStatus(ctx, db, proxy, "s2", &agentv1pb.ConfigStatus{ConfigRevision: row.Revision, ConfigHash: row.ConfigHash, Applied: true}, now)
		require.NoError(t, err)
		assert.Equal(t, model.ConfigVerdictApplied, verdict)
		applied, err := appliedDesired(ctx, db, proxy, row)
		require.NoError(t, err)
		assert.True(t, applied)

		long := strings.Repeat("e", 3000)
		verdict, err = RecordConfigStatus(ctx, db, proxy, "s3", &agentv1pb.ConfigStatus{ConfigRevision: row.Revision, ConfigHash: row.ConfigHash, Error: long}, now.Add(time.Second))
		require.NoError(t, err)
		assert.Equal(t, model.ConfigVerdictFailed, verdict)
		status, found, err := LoadConfigStatus(ctx, db, proxy)
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, "s3", status.SessionID)
		assert.Len(t, status.ReportedError, maxConfigStatusError)
		assert.False(t, status.ReportedApplied)
		assert.Equal(t, row.Revision, status.AppliedRevision, "a failure keeps the applied revision")
		assert.Equal(t, row.ConfigHash, status.AppliedHash)

		var count int64
		require.NoError(t, db.Model(&model.KernelNodeConfigStatus{}).Count(&count).Error)
		assert.Equal(t, int64(1), count, "one row per node")

		lagging, err := ConfigLaggingNodes(ctx, db)
		require.NoError(t, err)
		assert.Zero(t, lagging)
		require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", 5).Update("port", 8443).Error)
		newer, changed, err := RefreshDesiredConfig(ctx, db, proxy, now)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, row.Revision+1, newer.Revision)
		lagging, err = ConfigLaggingNodes(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, int64(1), lagging)
		applied, err = appliedDesired(ctx, db, proxy, newer)
		require.NoError(t, err)
		assert.False(t, applied)

		_, found, err = LoadConfigStatus(ctx, db, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1})
		require.NoError(t, err)
		assert.False(t, found, "the forward node of the same id has no status")
		_, _, err = RefreshDesiredConfig(ctx, db, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 999}, now)
		assert.ErrorIs(t, err, ErrNodeGone)
	})
}
