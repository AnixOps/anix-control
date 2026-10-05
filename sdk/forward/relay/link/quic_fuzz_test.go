package link

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/relay/relaytest"
)

// garbagePackets are what a QUIC listener on a public port receives besides
// QUIC: random bytes, plausible long headers, a version it does not speak, and
// Initials that cannot be decrypted.
func garbagePackets() [][]byte {
	initial := func(version uint32, size int) []byte {
		b := []byte{0xc0}
		b = binary.BigEndian.AppendUint32(b, version)
		b = append(b, 8) // destination connection id length
		b = append(b, 1, 2, 3, 4, 5, 6, 7, 8)
		b = append(b, 0) // source connection id length
		b = append(b, 0) // token length
		b = binary.BigEndian.AppendUint16(b, 0x4000|uint16(size-len(b)-2))
		return append(b, bytes.Repeat([]byte{0x5a}, size-len(b))...)
	}
	return [][]byte{
		nil,
		{0},
		{0xc0},
		{0x40, 1, 2, 3},
		bytes.Repeat([]byte{0xff}, 1200),
		bytes.Repeat([]byte{0x00}, 1200),
		initial(1, 1200), // an Initial of QUIC version 1 that does not decrypt
		initial(2, 1200), // QUIC version 2: not spoken here
		initial(0x1a2a3a4a, 1200),
		initial(0, 1200), // a version negotiation request
		initial(1, 100),  // an Initial that is too small
		[]byte("GET / HTTP/1.1\r\n\r\n"),
	}
}

var (
	garbageOnce sync.Once
	garbageL    *QUICListener
	garbageConn *net.UDPConn
	garbageErr  error
)

// garbageListener is one listener for the whole fuzz run.
func garbageListener() (*QUICListener, *net.UDPConn, error) {
	garbageOnce.Do(func() {
		ca, err := relaytest.New("garbage")
		if err != nil {
			garbageErr = err
			return
		}
		certPEM, keyPEM, err := ca.Issue(relaytest.Cert{Node: "forward-2"})
		if err != nil {
			garbageErr = err
			return
		}
		creds, err := NewCredentials(certPEM, keyPEM, ca.CAPEM())
		if err != nil {
			garbageErr = err
			return
		}
		l, err := ListenQUIC("127.0.0.1:0", QUICListenerConfig{
			ListenerConfig: ListenerConfig{Credentials: creds, Protocol: testProto, Sources: []string{"127.0.0.1"}, Peers: []string{id("forward-1")}, MaxPending: 8},
			QUIC:           quicTestConfig(),
		})
		if err != nil {
			garbageErr = err
			return
		}
		c, err := net.DialUDP("udp", nil, l.Addr().(*net.UDPAddr))
		if err != nil {
			garbageErr = err
			return
		}
		garbageL, garbageConn = l, c
	})
	return garbageL, garbageConn, garbageErr
}

// No datagram that is not a valid handshake makes a connection, and the
// listener keeps serving.
func FuzzQUICListenerPackets(f *testing.F) {
	for _, s := range garbagePackets() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		l, c, err := garbageListener()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = c.Write(data)
		if st := l.Stats(); st.Accepted != 0 || st.Pending > 8 {
			t.Fatalf("stats %+v", st)
		}
	})
}

// The same inputs as a unit test: the listener takes them all and still makes
// the next real connection.
func TestQUICListenerSurvivesGarbage(t *testing.T) {
	f := newFixture(t)
	// (a short handshake deadline: the garbage Initials hold slots until it)
	l := listenQUIC(t, f.creds("forward-2"), []string{id("forward-1")}, func(c *QUICListenerConfig) { c.HandshakeTimeout = time.Second })
	c, err := net.DialUDP("udp", nil, l.Addr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	for _, p := range garbagePackets() {
		_, _ = c.Write(p)
	}
	// a flood of random-looking Initials from one address
	for i := range 200 {
		b := garbagePackets()[6]
		binary.BigEndian.PutUint32(b[7:], uint32(i))
		_, _ = c.Write(b)
	}
	conn, err := dialQUIC(t, l, f.creds("forward-1"), "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = acceptQUIC(t, l)
	waitFor(t, "the garbage to be dealt with", func() bool { return l.Stats().Pending == 0 })
	if st := l.Stats(); st.Accepted != 1 {
		t.Fatalf("stats %+v", st)
	}
}
