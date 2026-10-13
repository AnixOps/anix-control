//go:build unix

package backup

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run as root (sudo anix-control backup create), the bundle and the
// directories it creates get the data directory's owner, so the service
// user can verify and remove them.
func TestWriteAsRootGivesTheDataDirectorysOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	p := newPanel(t)
	const nobody = 65534
	require.NoError(t, os.Chown(p.dataDir, nobody, nobody))
	bundle := p.write(t, TypeManual, time.Now())
	for _, path := range []string{filepath.Join(p.dataDir, "backups"), filepath.Join(p.dataDir, "tmp"), bundle.Path} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		owner, ok := info.Sys().(*syscall.Stat_t)
		require.True(t, ok)
		assert.EqualValues(t, nobody, owner.Uid, path)
	}
}
