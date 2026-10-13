//go:build unix

package backup

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// A process that cannot create the lock file (a container with a read-only
// root file system and no volume at data_dir: the Compose migrate service)
// is not refused: it has nothing to protect.
func TestLockUnavailableClassification(t *testing.T) {
	for _, errno := range []syscall.Errno{unix.EROFS, unix.EACCES, unix.EPERM} {
		wrapped := fmt.Errorf("open x: %w", &fs.PathError{Op: "open", Path: "x", Err: errno})
		require.True(t, lockUnavailable(wrapped), errno.Error())
	}
	require.False(t, lockUnavailable(ErrRestoreRunning))
	require.False(t, lockUnavailable(fmt.Errorf("lock x: %w", unix.EINVAL)))
}

func TestLockServiceOnAnUnwritableDirectoryIsANoOp(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes to any directory; the classification test covers the error path")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	lock, err := LockService(dir)
	require.NoError(t, err)
	require.NotNil(t, lock)
	lock.Release()
	_, statErr := os.Stat(filepath.Join(dir, LockFileName))
	require.True(t, os.IsNotExist(statErr))
}
