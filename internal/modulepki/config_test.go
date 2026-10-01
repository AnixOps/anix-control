package modulepki

import (
	"errors"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/stretchr/testify/require"
)

// TestFromConfigNeedsOnlyTheCAKey: the built-in CA does not depend on the
// module listener, so agent enrollment can use it with the module runtime
// off.
func TestFromConfigNeedsOnlyTheCAKey(t *testing.T) {
	db := openTestDB(t)
	const kek = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

	authority, err := FromConfig(config.ModuleRuntimeConfig{CAKEK: kek, Cluster: "edge"}, db)
	require.NoError(t, err, "module_runtime.enabled is not needed")
	require.Equal(t, "edge", authority.Cluster())
	_, err = FromConfig(config.ModuleRuntimeConfig{Enabled: true, CAKEK: kek}, db)
	require.NoError(t, err)

	for name, settings := range map[string]config.ModuleRuntimeConfig{
		"nothing configured":         {},
		"external PKI with a key":    {PKI: config.ModulePKIExternal, CAKEK: kek},
		"external module runtime":    {Enabled: true, PKI: config.ModulePKIExternal},
		"unknown PKI without module": {PKI: "other", CAKEK: kek},
	} {
		_, err := FromConfig(settings, db)
		require.True(t, errors.Is(err, ErrBuiltinPKIDisabled), name)
	}
	// An enabled runtime without a key, or a malformed key, is an error,
	// not "disabled".
	for _, settings := range []config.ModuleRuntimeConfig{{Enabled: true}, {CAKEK: "short"}} {
		_, err := FromConfig(settings, db)
		require.Error(t, err)
		require.False(t, errors.Is(err, ErrBuiltinPKIDisabled))
	}
}
