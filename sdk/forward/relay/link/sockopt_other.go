//go:build !linux

package link

import "net"

// tuneConn does nothing where TCP_USER_TIMEOUT does not exist; the carrier's
// own idle timeout still ends a dead link.
func tuneConn(net.Conn) error { return nil }
