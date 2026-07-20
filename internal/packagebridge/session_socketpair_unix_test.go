//go:build unix

package packagebridge

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestNewSessionSocketpairMarksEndpointsCloseOnExec(t *testing.T) {
	fds, err := newSessionSocketpair()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, closeSessionSocket(fds[0]))
		require.NoError(t, closeSessionSocket(fds[1]))
	})

	for _, fd := range fds {
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		require.NoError(t, err)
		require.NotZero(t, flags&unix.FD_CLOEXEC)
	}
}
