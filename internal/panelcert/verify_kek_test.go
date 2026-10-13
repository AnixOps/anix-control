package panelcert_test

import (
	"bytes"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyKEKOpensASealedEntry(t *testing.T) {
	db := openDB(t)
	checked, err := panelcert.VerifyKEK(t.Context(), db, testKEK)
	require.NoError(t, err)
	assert.False(t, checked, "nothing sealed yet")

	store, err := panelcert.NewStore(db, testKEK, nil)
	require.NoError(t, err)
	require.NoError(t, store.Put(t.Context(), "acme-000000000000/account+key", []byte("an account key")))

	checked, err = panelcert.VerifyKEK(t.Context(), db, testKEK)
	require.NoError(t, err)
	assert.True(t, checked)

	checked, err = panelcert.VerifyKEK(t.Context(), db, bytes.Repeat([]byte{8}, 32))
	assert.True(t, checked)
	require.ErrorIs(t, err, panelcert.ErrSealed)
}
