package agentpki_test

import (
	"bytes"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyLinkKEKOpensTheLinkCAs(t *testing.T) {
	db := openSQLiteForConfig(t)
	require.NoError(t, db.AutoMigrate(&model.ForwardLinkCA{}))
	kek := bytes.Repeat([]byte{5}, 32)

	opened, err := agentpki.VerifyLinkKEK(t.Context(), db, kek)
	require.NoError(t, err)
	assert.Zero(t, opened)

	link, err := agentpki.NewLinkAuthority(agentpki.LinkAuthorityOptions{DB: db, Cluster: "default", KEK: kek})
	require.NoError(t, err)
	require.NoError(t, link.Ensure(t.Context()))
	opened, err = agentpki.VerifyLinkKEK(t.Context(), db, kek)
	require.NoError(t, err)
	assert.Equal(t, 1, opened)

	_, err = agentpki.VerifyLinkKEK(t.Context(), db, bytes.Repeat([]byte{6}, 32))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrong key-encryption key")
}
