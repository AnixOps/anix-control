package agente2e

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The suite pushes real user traffic through the Agent's VLESS inbound (the
// sing-box core the Agent runs from Control's configuration) to a sink the
// test owns, so the traffic counters the Agent reports are the core's own.

// trafficSink answers a request "up length, down length, up bytes" with
// down bytes and closes.
type trafficSink struct {
	listener net.Listener
}

func newTrafficSink(t *testing.T) *trafficSink {
	t.Helper()
	return newTrafficSinkOn(t, "127.0.0.1")
}

func newTrafficSinkOn(t *testing.T, ip string) *trafficSink {
	t.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	require.NoError(t, err)
	sink := &trafficSink{listener: listener}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go sink.handle(conn)
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return sink
}

func (s *trafficSink) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	var header [16]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return
	}
	up := int64(binary.BigEndian.Uint64(header[:8]))
	down := int64(binary.BigEndian.Uint64(header[8:]))
	if _, err := io.CopyN(io.Discard, conn, up); err != nil {
		return
	}
	if _, err := io.CopyN(conn, zeroReader{}, down); err != nil {
		return
	}
	// The client closes once it has read everything: closing first lets
	// the proxy drop the tail it still buffers.
	_, _ = io.Copy(io.Discard, conn)
}

func (s *trafficSink) Port() int { return s.listener.Addr().(*net.TCPAddr).Port }

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// vlessTransfer sends up bytes to the sink and reads down bytes back
// through the VLESS inbound at proxyAddr as the user userUUID (VLESS
// version 0 over plain TCP, no flow).
func vlessTransfer(proxyAddr, userUUID string, sinkPort int, up, down int64) error {
	id, err := uuid.Parse(userUUID)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("tcp", proxyAddr, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	request := []byte{0}
	request = append(request, id[:]...)
	request = append(request, 0, 1) // no addons; TCP
	request = binary.BigEndian.AppendUint16(request, uint16(sinkPort))
	request = append(request, 1, 127, 0, 0, 1) // IPv4 127.0.0.1
	request = binary.BigEndian.AppendUint64(request, uint64(up))
	request = binary.BigEndian.AppendUint64(request, uint64(down))
	if _, err := conn.Write(request); err != nil {
		return err
	}
	if _, err := io.CopyN(conn, zeroReader{}, up); err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	var response [2]byte
	if _, err := io.ReadFull(conn, response[:]); err != nil {
		return fmt.Errorf("vless response: %w", err)
	}
	if response[1] > 0 {
		if _, err := io.CopyN(io.Discard, conn, int64(response[1])); err != nil {
			return err
		}
	}
	received, err := io.CopyN(io.Discard, conn, down)
	if err != nil {
		return fmt.Errorf("downloaded %d of %d bytes: %w", received, down, err)
	}
	return nil
}

// trafficChunk bounds one connection's transfer: the suite moves larger
// amounts over several connections (sing-box's VLESS inbound occasionally
// holds back the last bytes of a single megabyte-sized download).
const trafficChunk = 64 << 10

// pushTraffic moves up and down bytes for the user through the Agent's
// inbound, in chunks.
func (s *suite) pushTraffic(t *testing.T, user testUser, up, down int64) {
	t.Helper()
	for up > 0 || down > 0 {
		u, d := min(up, trafficChunk), min(down, trafficChunk)
		if d == 0 {
			d = 1 // the sink answers at least one byte, so the response header comes
		}
		require.NoError(t, vlessTransfer(fmt.Sprintf("127.0.0.1:%d", s.protocolPt), user.UUID, s.sink.Port(), u, d))
		up -= u
		down = max(0, down-d)
	}
}

// sinkTransfer is one plain request to a sink (or through a forward route
// to it): up bytes there, down bytes back.
func sinkTransfer(addr string, up, down int64) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	header := binary.BigEndian.AppendUint64(nil, uint64(up))
	header = binary.BigEndian.AppendUint64(header, uint64(down))
	if _, err := conn.Write(header); err != nil {
		return err
	}
	if _, err := io.CopyN(conn, zeroReader{}, up); err != nil {
		return err
	}
	received, err := io.CopyN(io.Discard, conn, down)
	if err != nil {
		return fmt.Errorf("received %d of %d bytes: %w", received, down, err)
	}
	return nil
}
