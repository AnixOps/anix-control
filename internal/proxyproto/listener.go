package proxyproto

import (
	"bufio"
	"net"
	"sync"
	"time"
)

// DefaultHeaderTimeout bounds how long a trusted peer may take to send its
// header. The relay writes it before the first byte of the node's stream.
const DefaultHeaderTimeout = 10 * time.Second

// Listener reads a PROXY v2 header from the connections of trusted peers and
// reports the address in it as the connection's remote address. Connections
// from any other peer pass through untouched: they are nodes that dial
// Control directly, and a header they sent would not be believed (it would
// be read as the start of their TLS handshake, which then fails).
//
// The header is read in the connection's own goroutine, on its first Read
// or RemoteAddr, never in Accept: a slow peer cannot stall the accept loop.
// A trusted peer that sends no valid header has its connection closed.
type Listener struct {
	net.Listener
	// Trusted tells whether a peer is a relay whose header is believed.
	Trusted func(net.IP) bool
	// HeaderTimeout bounds the wait for the header; zero is
	// DefaultHeaderTimeout.
	HeaderTimeout time.Duration
	// OnRefused, when set, is told of every trusted connection closed for a
	// missing or malformed header.
	OnRefused func(remote net.Addr, err error)
}

// Accept returns the next connection, wrapped when its peer is trusted.
func (l *Listener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if l.Trusted == nil || !l.Trusted(peerIP(conn.RemoteAddr())) {
		return conn, nil
	}
	timeout := l.HeaderTimeout
	if timeout <= 0 {
		timeout = DefaultHeaderTimeout
	}
	return &Conn{Conn: conn, reader: bufio.NewReaderSize(conn, 256), timeout: timeout, onRefused: l.OnRefused}, nil
}

func peerIP(addr net.Addr) net.IP {
	if addr == nil {
		return nil
	}
	if tcp, ok := addr.(*net.TCPAddr); ok {
		return tcp.IP
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

// Conn is a trusted peer's connection: its remote address is the one in the
// header (the relay's own when the header is LOCAL).
type Conn struct {
	net.Conn
	reader    *bufio.Reader
	timeout   time.Duration
	onRefused func(net.Addr, error)

	once    sync.Once
	remote  net.Addr
	relayed bool
	err     error
}

// Relayed tells whether the connection's remote address came from a relay's
// header.
func (c *Conn) Relayed() bool {
	c.once.Do(c.readHeader)
	return c.relayed
}

func (c *Conn) readHeader() {
	_ = c.SetReadDeadline(time.Now().Add(c.timeout))
	header, err := Read(c.reader)
	_ = c.SetReadDeadline(time.Time{})
	if err != nil {
		c.err = err
		c.remote = c.Conn.RemoteAddr()
		if c.onRefused != nil {
			c.onRefused(c.Conn.RemoteAddr(), err)
		}
		_ = c.Close()
		return
	}
	if header.Local || header.Source == nil {
		c.remote = c.Conn.RemoteAddr()
		return
	}
	c.remote = header.Source
	c.relayed = true
}

// Read reads the stream after the header.
func (c *Conn) Read(p []byte) (int, error) {
	c.once.Do(c.readHeader)
	if c.err != nil {
		return 0, c.err
	}
	return c.reader.Read(p)
}

// RemoteAddr is the address in the header.
func (c *Conn) RemoteAddr() net.Addr {
	c.once.Do(c.readHeader)
	return c.remote
}
