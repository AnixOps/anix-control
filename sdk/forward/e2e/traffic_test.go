//go:build linux

package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// The echo protocol of the target servers. TCP: the server first sends its
// id and a newline, then echoes every byte until the client half-closes,
// then closes. UDP: every datagram is answered with the id, a newline and
// the datagram. The id tells the client which target a connection reached;
// the byte counts stay exact.

// serveSpec is what one echo server process listens on.
type serveSpec struct {
	ID  string   `json:"id"`
	TCP []string `json:"tcp"`
	UDP []string `json:"udp"`
}

// serve is the echo server: the test binary re-executed inside a target
// namespace with envServe set. It prints "ready" once every socket
// listens, and exits when its standard input closes (the test stopped it,
// or the test process died).
func serve(spec string) int {
	var s serveSpec
	if err := json.Unmarshal([]byte(spec), &s); err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		return 2
	}
	greeting := []byte(s.ID + "\n")
	for _, addr := range s.TCP {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "serve %s: %v\n", s.ID, err)
			return 1
		}
		go serveTCP(ln, greeting)
	}
	for _, addr := range s.UDP {
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "serve %s: %v\n", s.ID, err)
			return 1
		}
		go serveUDP(pc, greeting)
	}
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	return 0
}

func serveTCP(ln net.Listener, greeting []byte) {
	for {
		c, err := ln.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "accept: %v\n", err)
			return
		}
		go func() {
			defer func() { _ = c.Close() }()
			if _, err := c.Write(greeting); err != nil {
				return
			}
			// A plain copy loop: no splice of the socket into itself.
			buf := make([]byte, 64<<10)
			if _, err := io.CopyBuffer(struct{ io.Writer }{c}, struct{ io.Reader }{c}, buf); err != nil {
				return
			}
			_ = c.(*net.TCPConn).CloseWrite()
		}()
	}
}

func serveUDP(pc net.PacketConn, greeting []byte) {
	buf := make([]byte, 64<<10)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "udp read: %v\n", err)
			return
		}
		reply := append(append([]byte{}, greeting...), buf[:n]...)
		if _, err := pc.WriteTo(reply, addr); err != nil {
			fmt.Fprintf(os.Stderr, "udp write: %v\n", err)
		}
	}
}

// server is one echo server process inside a target namespace.
type server struct {
	ns   *netns
	spec serveSpec

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr syncBuffer
}

// syncBuffer collects a server's standard error for the failure logs.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// start runs the server in its namespace and waits until it listens.
func (s *server) start(t testing.TB) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil {
		t.Fatalf("server %s already runs", s.spec.ID)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec, err := json.Marshal(s.spec)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("ip", "netns", "exec", s.ns.name, exe) // #nosec G204 -- the test binary itself, in a generated namespace
	cmd.Env = append(os.Environ(), envServe+"="+string(spec))
	cmd.Stderr = &s.stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server %s: %v", s.spec.ID, err)
	}
	ready := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err == nil && strings.TrimSpace(line) != "ready" {
			err = fmt.Errorf("printed %q", line)
		}
		ready <- err
		_, _ = io.Copy(io.Discard, stdout)
	}()
	select {
	case err = <-ready:
	case <-time.After(15 * time.Second):
		err = errors.New("not ready after 15s")
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("server %s in %s: %v: %s", s.spec.ID, s.ns.name, err, s.stderr.String())
	}
	s.cmd, s.stdin = cmd, stdin
}

// kill stops the server at once, as a crashed target: open connections are
// reset and new ones refused.
func (s *server) kill() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd == nil {
		return
	}
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
	_ = s.stdin.Close()
	s.cmd, s.stdin = nil, nil
}

// exchange is what one client exchange moved: the payload bytes the client
// sent (up) and received (down, the server's greeting included), in
// datagrams for UDP.
type exchange struct {
	target    string
	up, down  int
	datagrams int
}

// payload answers size bytes of a pattern that a misdelivered byte breaks.
func payload(size int) []byte {
	p := make([]byte, size)
	for i := range p {
		p[i] = byte(i*7 + i/251)
	}
	return p
}

// tcpEcho connects from the namespace (from local when it is valid), reads
// the greeting, sends size bytes, half-closes and reads them back.
func tcpEcho(from *netns, local netip.Addr, to netip.AddrPort, size int, timeout time.Duration) (exchange, error) {
	c, err := from.dialTCP(local, to, timeout)
	if err != nil {
		return exchange{}, err
	}
	defer func() { _ = c.Close() }()
	return echoOn(c, size, timeout)
}

// echoOn runs one TCP echo exchange on an open connection.
func echoOn(c *net.TCPConn, size int, timeout time.Duration) (exchange, error) {
	if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
		return exchange{}, err
	}
	r := bufio.NewReader(c)
	id, err := r.ReadString('\n')
	if err != nil {
		return exchange{}, fmt.Errorf("no greeting: %w", err)
	}
	ex := exchange{target: strings.TrimSuffix(id, "\n"), up: size, down: len(id) + size}
	data := payload(size)
	werr := make(chan error, 1)
	go func() {
		_, err := c.Write(data)
		if err == nil {
			err = c.CloseWrite()
		}
		werr <- err
	}()
	got, err := io.ReadAll(r)
	if e := <-werr; e != nil && err == nil {
		err = e
	}
	if err != nil {
		return ex, fmt.Errorf("echo from %s: %w after %d of %d bytes", ex.target, err, len(got), size)
	}
	if !bytes.Equal(got, data) {
		return ex, fmt.Errorf("echo from %s: %d bytes back, want the %d sent", ex.target, len(got), size)
	}
	return ex, nil
}

// heldConn is an open connection whose greeting was read: established
// through every hop.
type heldConn struct {
	*net.TCPConn
	target string
}

func holdTCP(from *netns, to netip.AddrPort, timeout time.Duration) (*heldConn, error) {
	c, err := from.dialTCP(netip.Addr{}, to, timeout)
	if err != nil {
		return nil, err
	}
	if err := c.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		_ = c.Close()
		return nil, err
	}
	// Byte by byte, so nothing after the greeting is consumed.
	var id []byte
	b := make([]byte, 1)
	for {
		if _, err := c.Read(b); err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("no greeting: %w", err)
		}
		if b[0] == '\n' {
			break
		}
		id = append(id, b[0])
	}
	return &heldConn{TCPConn: c, target: string(id)}, nil
}

// ping sends size bytes on a held connection and reads them back.
func (h *heldConn) ping(size int, timeout time.Duration) error {
	if err := h.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	data := payload(size)
	if _, err := h.Write(data); err != nil {
		return err
	}
	got := make([]byte, size)
	if _, err := io.ReadFull(h, got); err != nil {
		return err
	}
	if !bytes.Equal(got, data) {
		return errors.New("echo differs")
	}
	return nil
}

// udpEcho sends one datagram per size from the namespace and reads every
// answer.
func udpEcho(from *netns, to netip.AddrPort, sizes []int, timeout time.Duration) (exchange, error) {
	c, err := from.dialUDP(to)
	if err != nil {
		return exchange{}, err
	}
	defer func() { _ = c.Close() }()
	var ex exchange
	buf := make([]byte, 64<<10)
	for i, size := range sizes {
		data := payload(size)
		if _, err := c.Write(data); err != nil {
			return ex, err
		}
		if err := c.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return ex, err
		}
		n, err := c.Read(buf)
		if err != nil {
			return ex, fmt.Errorf("datagram %d: %w", i, err)
		}
		id, body, ok := bytes.Cut(buf[:n], []byte("\n"))
		if !ok || !bytes.Equal(body, data) {
			return ex, fmt.Errorf("datagram %d: answer of %d bytes is not the echo", i, n)
		}
		if ex.target != "" && ex.target != string(id) {
			return ex, fmt.Errorf("datagram %d answered by %s, earlier ones by %s", i, id, ex.target)
		}
		ex.target = string(id)
		ex.up += size
		ex.down += n
		ex.datagrams++
	}
	return ex, nil
}
