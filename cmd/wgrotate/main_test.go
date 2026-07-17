package main

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOptionsDefaultsToDryRun(t *testing.T) {
	opts, err := parseOptions(nil)
	require.NoError(t, err)
	require.True(t, opts.dryRun)
	require.Zero(t, opts.protocolID)
}

func TestRunRequiresExplicitDestructiveConfirmation(t *testing.T) {
	err := run([]string{"-dry-run=false"}, io.Discard)
	require.ErrorContains(t, err, rotationConfirmation)
}
