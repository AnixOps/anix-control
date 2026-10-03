package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackageRoutesDefaultMode(t *testing.T) {
	require.Equal(t, PackageRoutesDefaultRehearsed, PackageRoutesConfig{}.DefaultModeOrDefault(), "4.1 defaults to the rehearsed set (H8)")
	require.Equal(t, PackageRoutesDefaultRehearsed, Defaults().PackageRoutes.DefaultModeOrDefault())
	require.Equal(t, PackageRoutesDefaultLegacy, PackageRoutesConfig{DefaultMode: " Legacy "}.DefaultModeOrDefault())

	loaded, err := load("", nil)
	require.NoError(t, err)
	require.Equal(t, PackageRoutesDefaultRehearsed, loaded.PackageRoutes.DefaultModeOrDefault())

	// The kill switch through the environment.
	loaded, err = load("", []string{EnvPrefix + "PACKAGE_ROUTES_DEFAULT_MODE=legacy"})
	require.NoError(t, err)
	require.Equal(t, PackageRoutesDefaultLegacy, loaded.PackageRoutes.DefaultModeOrDefault())

	_, err = load("", []string{EnvPrefix + "PACKAGE_ROUTES_DEFAULT_MODE=native"})
	require.ErrorContains(t, err, "package_routes.default_mode")
}
