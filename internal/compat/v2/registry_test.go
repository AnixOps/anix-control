package v2

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type registrySourceStub struct {
	route Route
	err   error
}

func (s registrySourceStub) ResolveV2Route(_ context.Context, method, path string) (Route, error) {
	if s.err != nil {
		return Route{}, s.err
	}
	if method != s.route.Method || path != s.route.LegacyPath {
		return Route{}, ErrPackageUnavailable
	}
	return s.route, nil
}

func TestRegistryResolvesOnlyDeclaredInstalledRoute(t *testing.T) {
	registry := NewRegistry(registrySourceStub{route: Route{
		Method:       http.MethodGet,
		LegacyPath:   "/api/v2/user/knowledge",
		PackageID:    "knowledge",
		Version:      "4.0.0",
		Generation:   7,
		PackageRoute: "knowledge.article.list",
		Envelope:     EnvelopeData,
	}})

	route, ok := registry.Resolve(http.MethodGet, "/api/v2/user/knowledge")
	require.True(t, ok)
	require.Equal(t, "knowledge.article.list", route.PackageRoute)

	_, ok = registry.Resolve(http.MethodGet, "/api/v2/user/knowledge/1")
	require.False(t, ok)
}

func TestRegistryDoesNotResolveDisabledPackage(t *testing.T) {
	registry := NewRegistry(registrySourceStub{err: errors.New("knowledge disabled")})

	_, ok := registry.Resolve(http.MethodGet, "/api/v2/user/knowledge")
	require.False(t, ok)
}
