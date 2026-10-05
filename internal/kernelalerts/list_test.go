package kernelalerts

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestParseQuery(t *testing.T) {
	query, err := ParseQuery("", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, Query{Status: StatusActive, Limit: DefaultLimit}, query)
	query, err = ParseQuery("all", "ca_expiring", "critical", "5")
	require.NoError(t, err)
	assert.Equal(t, Query{Status: StatusAll, Kind: "ca_expiring", Severity: "critical", Limit: 5}, query)
	for _, bad := range [][4]string{{"bogus", "", "", ""}, {"", "", "info", ""}, {"", "", "", "0"}, {"", "", "", "501"}, {"", "", "", "12abc"}, {"", "", "", "-3"}} {
		_, err := ParseQuery(bad[0], bad[1], bad[2], bad[3])
		require.ErrorIs(t, err, ErrInvalidQuery, "%v", bad)
	}
}

func TestListFiltersOrdersAndSummarizes(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		soon, later := start.Add(hours(5)), start.Add(hours(50))
		resolved := start.Add(-time.Hour)
		for _, alert := range []model.KernelAlert{
			{Key: "agent_certificate_expiring/proxy-1", Kind: KindAgentCertificate, Severity: "warning", Subject: "proxy-1", ExpiresAt: &later, Detail: `{"node":"proxy-1"}`},
			{Key: "agent_certificate_expiring/proxy-2", Kind: KindAgentCertificate, Severity: "critical", Subject: "proxy-2", ExpiresAt: &later},
			{Key: "agent_certificate_expiring/proxy-3", Kind: KindAgentCertificate, Severity: "critical", Subject: "proxy-3", ExpiresAt: &soon},
			{Key: "node_secrets_split_stalled/v2_node", Kind: KindNodeSecretsSplitStalled, Severity: "warning", Subject: "v2_node"},
			{Key: "ca_expiring/old", Kind: KindCAExpiring, Severity: "warning", Subject: "old", ResolvedAt: &resolved},
		} {
			alert.SubjectKind, alert.Message, alert.FirstSeenAt, alert.LastSeenAt = "x", "m", start, start
			require.NoError(t, db.Create(&alert).Error)
		}

		page, err := List(t.Context(), db, Query{Status: StatusActive})
		require.NoError(t, err)
		assert.Equal(t, Summary{Active: 4, Critical: 2, Warning: 2}, page.Summary)
		keys := []string{}
		for _, alert := range page.Alerts {
			keys = append(keys, alert.Subject)
			assert.Equal(t, StatusActive, alert.Status)
		}
		assert.Equal(t, []string{"proxy-3", "proxy-2", "v2_node", "proxy-1"}, keys, "critical first; within a severity the soonest end, or first seen, first")
		assert.Equal(t, "proxy-1", page.Alerts[3].Detail["node"])
		assert.NotNil(t, page.Alerts[0].Detail, "detail is always an object")

		page, err = List(t.Context(), db, Query{Status: StatusResolved})
		require.NoError(t, err)
		require.Len(t, page.Alerts, 1)
		assert.Equal(t, StatusResolved, page.Alerts[0].Status)
		assert.Equal(t, int64(4), page.Summary.Active, "the summary counts every active alert")

		page, err = List(t.Context(), db, Query{Status: StatusAll, Kind: KindAgentCertificate, Severity: "critical", Limit: 1})
		require.NoError(t, err)
		require.Len(t, page.Alerts, 1)
		assert.Equal(t, "proxy-3", page.Alerts[0].Subject)
		page, err = List(t.Context(), db, Query{Status: StatusAll})
		require.NoError(t, err)
		assert.Len(t, page.Alerts, 5)
	})
}

func TestListWithoutTheTable(t *testing.T) {
	page, err := List(t.Context(), openBare(t), Query{})
	require.NoError(t, err)
	assert.Empty(t, page.Alerts)
	assert.NotNil(t, page.Alerts, "an empty list answers [] not null")
}
