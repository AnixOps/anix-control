//go:build linux

package link

import (
	"net"
	"time"

	"golang.org/x/sys/unix"
)

// tuneConn sets TCP_USER_TIMEOUT on a TCP connection so a write into a dead
// path fails after UserTimeout instead of retransmitting for minutes
// (anixops-protocol.md section 4.6, L4). Go already sets TCP_NODELAY on every
// TCP connection (the carrier's writer batches frames itself). Other kinds of
// connection are left alone.
func tuneConn(c net.Conn) error {
	tc, ok := c.(*net.TCPConn)
	if !ok {
		return nil
	}
	raw, err := tc.SyscallConn()
	if err != nil {
		return err
	}
	var serr error
	if err := raw.Control(func(fd uintptr) {
		// #nosec G115 -- a file descriptor fits an int, and 30000 ms fits an int
		serr = unix.SetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_USER_TIMEOUT, int(UserTimeout/time.Millisecond))
	}); err != nil {
		return err
	}
	return serr
}
