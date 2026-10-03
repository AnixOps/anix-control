package service

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/sdk/telemetry/systemdreport"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
)

// On PostgreSQL too, a package report replaces the stored one only when it
// is at least as new, and a node keeps one row per plugin and kind.
func TestPostgresPackageReportUpsertKeepsTheLatest(t *testing.T) {
	db := openPostgresTestDB(t, &model.PackageReportState{})
	service := &NodeService{db: db}
	base := time.Unix(1_800_000_000, 0).UTC()
	input := PackageReportInput{NodeKind: agentcontrol.NodeKindProxy, NodeID: 5, PluginID: "machine-telemetry", Kind: systemdreport.Kind, Version: "4.1.0"}

	stored, err := service.persistPackageReport(input, []byte(`{"n":1}`), base, base)
	require.NoError(t, err)
	require.True(t, stored)
	stored, err = service.persistPackageReport(input, []byte(`{"n":0}`), base.Add(-time.Minute), base)
	require.NoError(t, err)
	require.False(t, stored, "an older report is dropped")
	stored, err = service.persistPackageReport(input, []byte(`{"n":2}`), base.Add(time.Minute), base.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, stored)
	other := input
	other.NodeID = 6
	stored, err = service.persistPackageReport(other, []byte(`{"n":9}`), base, base)
	require.NoError(t, err)
	require.True(t, stored)

	var rows []model.PackageReportState
	require.NoError(t, db.Order("node_id").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.JSONEq(t, `{"n":2}`, rows[0].PayloadJSON)
	require.True(t, rows[0].ObservedAt.Equal(base.Add(time.Minute)))

	report, err := service.LatestPackageReport(agentcontrol.NodeKindProxy, 5, "machine-telemetry", systemdreport.Kind, base.Add(30*time.Minute))
	require.NoError(t, err)
	require.True(t, report.Stale)
	require.JSONEq(t, `{"n":2}`, string(report.Payload))
}
