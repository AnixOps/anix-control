package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestDBCloseWithErrorClosesDatabase(t *testing.T) {
	testDB := SetupTestDB()

	sqlDB, err := testDB.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping())

	require.NoError(t, testDB.CloseWithError())
	assert.Error(t, sqlDB.Ping())
}

func TestTestDBCloseWithErrorAllowsNilReceiver(t *testing.T) {
	var testDB *TestDB
	require.NoError(t, testDB.CloseWithError())
}
