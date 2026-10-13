package main

import (
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/pluginhost"
	"github.com/AnixOps/anix-control/v5/packages/builtin"
	"github.com/stretchr/testify/require"
)

// The launcher (internal/pluginhost) and the command (packages/builtin)
// name the subcommand and the version variable separately: internal/
// packages may not import every package host.
func TestPackageHostCommandMatchesTheLauncher(t *testing.T) {
	require.Equal(t, builtin.CommandName, pluginhost.BuiltinHostCommand)
	require.Equal(t, builtin.VersionEnvironment, pluginhost.BuiltinHostVersionEnvironment)
}

func TestPackageHostCommandOnlyHandlesItsName(t *testing.T) {
	_, handled := runPackageHostCommand(nil)
	require.False(t, handled)
	_, handled = runPackageHostCommand([]string{"migrate"})
	require.False(t, handled)

	code, handled := runPackageHostCommand([]string{builtin.CommandName, "no-such-package"})
	require.True(t, handled)
	require.Equal(t, 2, code)
	code, handled = runPackageHostCommand([]string{builtin.CommandName})
	require.True(t, handled)
	require.Equal(t, 2, code)
}
