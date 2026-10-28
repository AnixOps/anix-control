package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func decodePanelTestResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, resp, "code")
	require.Contains(t, resp, "msg")
	require.Contains(t, resp, "ts")
	return resp
}
