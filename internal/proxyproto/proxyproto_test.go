package proxyproto

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestEncodeReadRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		src  *net.TCPAddr
		dst  *net.TCPAddr
	}{
		{"ipv4", &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 51000}, &net.TCPAddr{IP: net.ParseIP("198.51.100.1"), Port: 50051}},
		{"ipv6", &net.TCPAddr{IP: net.ParseIP("2001:db8::7"), Port: 1}, &net.TCPAddr{IP: net.ParseIP("2001:db8::1"), Port: 65535}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := Encode(tc.src, tc.dst)
			if err != nil {
				t.Fatal(err)
			}
			stream := append(raw, []byte("rest")...)
			reader := bytes.NewReader(stream)
			header, err := Read(reader)
			if err != nil {
				t.Fatal(err)
			}
			if header.Local || !header.Source.IP.Equal(tc.src.IP) || header.Source.Port != tc.src.Port ||
				!header.Destination.IP.Equal(tc.dst.IP) || header.Destination.Port != tc.dst.Port {
				t.Fatalf("header = %+v", header)
			}
			rest, _ := io.ReadAll(reader)
			if string(rest) != "rest" {
				t.Fatalf("Read consumed past the header: %q left", rest)
			}
		})
	}
}

func TestEncodeRefusesMixedFamilies(t *testing.T) {
	if _, err := Encode(&net.TCPAddr{IP: net.ParseIP("203.0.113.7")}, &net.TCPAddr{IP: net.ParseIP("2001:db8::1")}); err == nil {
		t.Fatal("mixed families encoded")
	}
}

func TestReadLocal(t *testing.T) {
	header, err := Read(bytes.NewReader(EncodeLocal()))
	if err != nil || !header.Local {
		t.Fatalf("header = %+v, err = %v", header, err)
	}
}

func TestReadRefusesBadInput(t *testing.T) {
	good, err := Encode(&net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 1}, &net.TCPAddr{IP: net.ParseIP("198.51.100.1"), Port: 2})
	if err != nil {
		t.Fatal(err)
	}
	mutate := func(index int, value byte) []byte {
		out := append([]byte(nil), good...)
		out[index] = value
		return out
	}
	oversized := append([]byte(nil), good[:16]...)
	oversized[14], oversized[15] = 0xFF, 0xFF
	shortIPv4 := append([]byte(nil), good[:14]...)
	shortIPv4 = append(shortIPv4, 0x00, 0x04, 1, 2, 3, 4)
	cases := map[string][]byte{
		"tls hello":       {0x16, 0x03, 0x01, 0x00, 0x10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		"empty":           nil,
		"short":           good[:10],
		"truncated block": good[:20],
		"v1 command":      mutate(12, 0x11),
		"udp family":      mutate(13, 0x12),
		"oversized block": oversized,
		"short ipv4":      shortIPv4,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Read(bytes.NewReader(input)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	if _, err := Read(bytes.NewReader(nil)); !errors.Is(err, ErrNoHeader) {
		t.Fatalf("empty stream: %v", err)
	}
}

// listenerPair accepts one connection on a Listener that trusts the peer
// when trusted is set.
func listenerPair(t *testing.T, trusted bool, timeout time.Duration) (net.Conn, net.Conn) {
	t.Helper()
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = inner.Close() })
	listener := &Listener{Listener: inner, Trusted: func(net.IP) bool { return trusted }, HeaderTimeout: timeout}
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
		close(accepted)
	}()
	client, err := net.Dial("tcp", inner.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	server := <-accepted
	if server == nil {
		t.Fatal("no connection accepted")
	}
	t.Cleanup(func() { _ = server.Close() })
	return client, server
}

func TestListenerTakesTheTrustedPeersHeader(t *testing.T) {
	client, server := listenerPair(t, true, time.Second)
	header, err := Encode(&net.TCPAddr{IP: net.ParseIP("203.0.113.9"), Port: 40000}, &net.TCPAddr{IP: net.ParseIP("198.51.100.1"), Port: 50051})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write(append(header, []byte("hello")...)); err != nil {
		t.Fatal(err)
	}
	if got := server.RemoteAddr().String(); got != "203.0.113.9:40000" {
		t.Fatalf("RemoteAddr = %s", got)
	}
	if !server.(*Conn).Relayed() {
		t.Fatal("not marked as relayed")
	}
	buffer := make([]byte, 5)
	if _, err := io.ReadFull(server, buffer); err != nil || string(buffer) != "hello" {
		t.Fatalf("stream = %q, %v", buffer, err)
	}
}

func TestListenerKeepsTheRelayAddressOnLocal(t *testing.T) {
	client, server := listenerPair(t, true, time.Second)
	if _, err := client.Write(EncodeLocal()); err != nil {
		t.Fatal(err)
	}
	if got := server.RemoteAddr().String(); got != client.LocalAddr().String() {
		t.Fatalf("RemoteAddr = %s, want %s", got, client.LocalAddr())
	}
	if server.(*Conn).Relayed() {
		t.Fatal("LOCAL marked as relayed")
	}
}

func TestListenerClosesATrustedPeerWithoutHeader(t *testing.T) {
	client, server := listenerPair(t, true, time.Second)
	if _, err := client.Write([]byte{0x16, 0x03, 0x01, 0x00, 0x10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Read(make([]byte, 1)); err == nil {
		t.Fatal("read succeeded without a header")
	}
}

func TestListenerTimesOutASilentTrustedPeer(t *testing.T) {
	_, server := listenerPair(t, true, 50*time.Millisecond)
	start := time.Now()
	if _, err := server.Read(make([]byte, 1)); err == nil {
		t.Fatal("read succeeded without a header")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("the header wait was not bounded")
	}
}

func TestListenerLeavesOtherPeersAlone(t *testing.T) {
	client, server := listenerPair(t, false, time.Second)
	header, err := Encode(&net.TCPAddr{IP: net.ParseIP("203.0.113.9"), Port: 40000}, &net.TCPAddr{IP: net.ParseIP("198.51.100.1"), Port: 50051})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write(header); err != nil {
		t.Fatal(err)
	}
	if _, ok := server.(*Conn); ok {
		t.Fatal("an untrusted peer's connection was wrapped")
	}
	if got := server.RemoteAddr().String(); got != client.LocalAddr().String() {
		t.Fatalf("RemoteAddr = %s: a forged header was believed", got)
	}
	buffer := make([]byte, len(header))
	if _, err := io.ReadFull(server, buffer); err != nil || !bytes.Equal(buffer, header) {
		t.Fatal("the untrusted peer's bytes were consumed")
	}
}
