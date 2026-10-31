package nodegroup

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyKEKOpensAJoinToken(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	kek, err := base64.StdEncoding.DecodeString(f.kek)
	require.NoError(t, err)

	checked, err := VerifyKEK(f.db, kek)
	require.NoError(t, err)
	assert.False(t, checked, "no group, no token")

	f.createGroup(t, GroupInput{Name: "HK"})
	checked, err = VerifyKEK(f.db, kek)
	require.NoError(t, err)
	assert.True(t, checked)

	other := make([]byte, 32)
	checked, err = VerifyKEK(f.db, other)
	assert.True(t, checked)
	require.ErrorIs(t, err, ErrSealedToken)
}
