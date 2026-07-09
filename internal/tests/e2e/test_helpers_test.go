package e2e

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/anixops/v2board/internal/database"
	"github.com/stretchr/testify/require"
)

func requireJSONUnmarshal(t testing.TB, data []byte, target any) {
	t.Helper()

	require.NoError(t, json.Unmarshal(data, target))
}

func requirePanelEnvelope(t testing.TB, data []byte, expectedCode float64) map[string]any {
	t.Helper()

	var response map[string]any
	requireJSONUnmarshal(t, data, &response)
	require.Equal(t, expectedCode, response["code"])
	require.NotEmpty(t, response["msg"])
	require.NotZero(t, response["ts"])
	return response
}

func requirePanelDataMap(t testing.TB, data []byte) map[string]any {
	t.Helper()

	response := requirePanelEnvelope(t, data, 0)
	payload, ok := response["data"].(map[string]any)
	require.True(t, ok)
	return payload
}

func requirePanelErrorResponse(t testing.TB, statusCode int, data []byte, msgContains string) {
	t.Helper()

	require.Equal(t, http.StatusOK, statusCode)
	response := requirePanelEnvelope(t, data, -1)
	message, ok := response["msg"].(string)
	require.True(t, ok)
	if msgContains != "" {
		require.Contains(t, message, msgContains)
	}
	require.Nil(t, response["data"])
}

func requireDatabaseClosed(t testing.TB) {
	t.Helper()

	require.NoError(t, database.Close())
}
