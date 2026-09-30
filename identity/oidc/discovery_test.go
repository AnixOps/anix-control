package oidc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoveryNamesTheKeysOfTheIssuer(t *testing.T) {
	document, err := NewDiscovery("https://id.example.test/", "https://id.example.test/jwks.json")
	require.NoError(t, err)
	require.Equal(t, "https://id.example.test", document.Issuer)
	require.Equal(t, "https://id.example.test/jwks.json", document.JWKSURI)
	require.Equal(t, []string{"EdDSA"}, document.IDTokenSigningAlgValuesSupported)

	_, err = NewDiscovery("anixops-identity", "https://id.example.test/jwks.json")
	require.Error(t, err, "the issuer must be a URL")
}
