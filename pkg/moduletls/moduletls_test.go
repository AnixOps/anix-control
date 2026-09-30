package moduletls

import (
	"crypto/x509"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIdentitiesRoundTrip(t *testing.T) {
	kernel, err := Kernel("prod-eu")
	require.NoError(t, err)
	require.Equal(t, "spiffe://anixops/prod-eu/kernel", kernel.String())
	module, err := Module("prod-eu", "identity-platform")
	require.NoError(t, err)
	require.Equal(t, "spiffe://anixops/prod-eu/module/identity-platform", module.String())
	for _, identity := range []Identity{kernel, module} {
		parsed, err := Parse(identity.String())
		require.NoError(t, err)
		require.Equal(t, identity, parsed)
	}
}

func TestParseRejectsForeignIdentities(t *testing.T) {
	for _, raw := range []string{
		"spiffe://example.org/prod/kernel",
		"https://anixops/prod/kernel",
		"spiffe://anixops/prod/module",
		"spiffe://anixops/prod/module/Identity",
		"spiffe://anixops/prod/module/identity/extra",
		"spiffe://anixops/Prod/kernel",
		"spiffe://anixops:8443/prod/kernel",
		"spiffe://user@anixops/prod/kernel",
		"spiffe://anixops/prod/kernel?x=1",
		"spiffe://anixops/prod/agent/x",
	} {
		_, err := Parse(raw)
		require.ErrorIs(t, err, ErrInvalidIdentity, raw)
	}
}

func TestFromCertificateRequiresExactlyOneURISAN(t *testing.T) {
	id, _ := Module("prod", "identity-platform")
	require.NoError(t, func() error { _, err := FromCertificate(&x509.Certificate{URIs: []*url.URL{id.URL()}}); return err }())
	for name, certificate := range map[string]*x509.Certificate{
		"none":       {},
		"two URIs":   {URIs: []*url.URL{id.URL(), id.URL()}},
		"with DNS":   {URIs: []*url.URL{id.URL()}, DNSNames: []string{"identity.local"}},
		"with email": {URIs: []*url.URL{id.URL()}, EmailAddresses: []string{"a@b.c"}},
	} {
		_, err := FromCertificate(certificate)
		require.ErrorIs(t, err, ErrInvalidIdentity, name)
	}
}

func TestAcceptors(t *testing.T) {
	kernel, _ := Kernel("prod")
	module, _ := Module("prod", "identity-platform")
	other, _ := Module("staging", "identity-platform")
	require.NoError(t, AcceptExactly(kernel)(kernel))
	require.ErrorIs(t, AcceptExactly(kernel)(module), ErrUnexpectedPeer)
	require.NoError(t, AcceptModules("prod")(module))
	require.ErrorIs(t, AcceptModules("prod")(kernel), ErrUnexpectedPeer)
	require.ErrorIs(t, AcceptModules("prod")(other), ErrUnexpectedPeer)
}
