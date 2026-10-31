package settingscatalog

import (
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/destinationstats"
	"github.com/stretchr/testify/require"
)

func TestItemsWithoutDatabaseHaveNoValues(t *testing.T) {
	items, err := Items(nil)
	require.NoError(t, err)
	seen := map[string]Item{}
	for _, item := range items {
		require.NotContains(t, seen, item.Key, "a key is offered once")
		require.Nil(t, item.Value)
		seen[item.Key] = item
	}
	retention := seen[destinationstats.RetentionDaysKey]
	require.Equal(t, KindChoice, retention.Kind)
	require.Equal(t, destinationstats.DefaultRetentionDays, retention.Default)
	for _, choice := range retention.Choices {
		require.GreaterOrEqual(t, choice, *retention.Min)
		require.LessOrEqual(t, choice, *retention.Max)
	}
	require.Contains(t, retention.Choices, destinationstats.DefaultRetentionDays)
}

func TestLinksNameARoute(t *testing.T) {
	items, err := Items(nil)
	require.NoError(t, err)
	for _, item := range items {
		if item.Kind == KindLink {
			require.NotEmpty(t, item.Link, item.Key)
		}
	}
}
