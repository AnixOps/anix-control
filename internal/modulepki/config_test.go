package modulepki

import (
	"errors"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/stretchr/testify/require"
)

// TestFromConfigNeedsOnlyTheCAKey: module_runtime.ca_kek alone configures
// the built-in CA.
func TestFromConfigNeedsOnlyTheCAKey(t *testing.T) {
	db := openTestDB(t)
	const kek = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

	authority, err := FromConfig(config.ModuleRuntimeConfig{CAKEK: kek, Cluster: "edge"}, db)
	require.NoError(t, err)
	require.Equal(t, "edge", authority.Cluster())

	for name, settings := range map[string]config.ModuleRuntimeConfig{
		"nothing configured":         {},
		"external PKI with a key":    {PKI: config.ModulePKIExternal, CAKEK: kek},
		"external PKI":               {PKI: config.ModulePKIExternal},
		"unknown PKI without module": {PKI: "other", CAKEK: kek},
	} {
		_, err := FromConfig(settings, db)
		require.True(t, errors.Is(err, ErrBuiltinPKIDisabled), name)
	}
	// A malformed key is an error, not "disabled".
	_, err = FromConfig(config.ModuleRuntimeConfig{CAKEK: "short"}, db)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrBuiltinPKIDisabled))
}
