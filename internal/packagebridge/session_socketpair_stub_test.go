//go:build !unix

package packagebridge

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSessionSocketpairRejectsUnsupportedPlatforms(t *testing.T) {
	_, err := newSessionSocketpair()
	require.Error(t, err)
}
