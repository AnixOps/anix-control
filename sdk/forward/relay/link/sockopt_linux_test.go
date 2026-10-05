//go:build linux

package link

import (
	"net"
	"testing"

	"golang.org/x/sys/unix"
)

func TestUserTimeoutIsSet(t *testing.T) {
	f := newFixture(t)
	l := listen(t, f.creds("forward-2"), []string{id("forward-1")})
	c, err := dial(t, l, f.creds("forward-1"), "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	in := accept(t, l)
	defer func() { _ = in.Close() }()
	for name, conn := range map[string]*Conn{"dialled": c, "accepted": in} {
		raw := conn.Conn.(interface{ NetConn() net.Conn }).NetConn().(*net.TCPConn)
		rc, err := raw.SyscallConn()
		if err != nil {
			t.Fatal(err)
		}
		var got int
		var gerr error
		if err := rc.Control(func(fd uintptr) {
			got, gerr = unix.GetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_USER_TIMEOUT) // #nosec G115 -- a file descriptor fits an int
		}); err != nil || gerr != nil {
			t.Fatal(err, gerr)
		}
		if got != 30000 {
			t.Errorf("%s connection: TCP_USER_TIMEOUT = %d ms, want 30000", name, got)
		}
	}
}
