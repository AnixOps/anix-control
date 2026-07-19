package v2

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRouteRequiresV2PathAndDeclaredEnvelope(t *testing.T) {
	valid := Route{
		Method:       http.MethodGet,
		LegacyPath:   "/api/v2/user/knowledge",
		PackageID:    "knowledge",
		Version:      "4.0.0",
		Generation:   1,
		PackageRoute: "knowledge.article.list",
		Envelope:     EnvelopeData,
	}
	require.NoError(t, valid.Validate())

	invalid := valid
	invalid.LegacyPath = "/api/v3/plugins/knowledge/articles"
	require.Error(t, invalid.Validate())
}

func TestRouteDeclarationBindsOwnerEnvelopeAndTransport(t *testing.T) {
	raw := []byte(`{"api_version":"v2","package_id":"knowledge","routes":[{"method":"GET","legacy_path":"/api/v2/user/knowledge/:id","package_route":"knowledge.article.get","envelope":"data"}]}`)
	routes, err := decodeRouteDeclaration(raw, "knowledge", "4.0.0", 7)
	require.NoError(t, err)
	require.Len(t, routes, 1)
	require.Equal(t, TransportHTTP, routes[0].Transport)
	require.True(t, routeMatches(routes[0].LegacyPath, "/api/v2/user/knowledge/42"))

	_, err = decodeRouteDeclaration(raw, "ticket", "4.0.0", 7)
	require.Error(t, err)

	invalidTransport := []byte(`{"api_version":"v2","package_id":"knowledge","routes":[{"method":"GET","legacy_path":"/api/v2/user/knowledge","package_route":"knowledge.article.list","envelope":"data","transport":"websocket"}]}`)
	_, err = decodeRouteDeclaration(invalidTransport, "knowledge", "4.0.0", 7)
	require.Error(t, err)
}
