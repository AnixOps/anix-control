package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/adminclaim"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
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

// claimFirstAdministrator creates the first administrator as a fresh panel
// does: a claim link (anix-control admin claim-link) and its claim.
func claimFirstAdministrator(t testing.TB, email, password string) {
	t.Helper()

	db := database.Get()
	require.NoError(t, db.AutoMigrate(&migrations.MetaEntry{}, &model.AuditLog{}))
	issued, err := adminclaim.Issue(context.Background(), db, adminclaim.DefaultTTL, time.Now())
	require.NoError(t, err)
	_, err = (&adminclaim.Claimer{DB: db, Cost: bcrypt.MinCost}).Claim(context.Background(),
		adminclaim.Request{Token: issued.Token, Email: email, Password: password})
	require.NoError(t, err)
}
