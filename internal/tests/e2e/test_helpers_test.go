package e2e

import (
	"encoding/json"
	"testing"

	"github.com/anixops/v2board/internal/database"
	"github.com/stretchr/testify/require"
)

func requireJSONUnmarshal(t testing.TB, data []byte, target any) {
	t.Helper()

	require.NoError(t, json.Unmarshal(data, target))
}

func requireDatabaseClosed(t testing.TB) {
	t.Helper()

	require.NoError(t, database.Close())
}
