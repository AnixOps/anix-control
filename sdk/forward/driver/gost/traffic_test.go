package gost_test

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
)

// envEcho turns the test binary into a TCP and UDP echo server on the
// address it names, which TestNetnsTraffic starts inside its namespace.
const envEcho = "ANIXOPS_GOST_ECHO"

func TestMain(m *testing.M) {
	if addr := os.Getenv(envEcho); addr != "" {
		os.Exit(echo(addr))
	}
	os.Exit(m.Run())
}

// echo answers every TCP stream and UDP datagram with "echo:" and what it
// got, until its standard input closes.
func echo(addr string) int {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if _, err := io.WriteString(c, "echo:"+line); err != nil {
						return
					}
				}
			}()
		}
	}()
	go func() {
		buf := make([]byte, 64<<10)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(append([]byte("echo:"), buf[:n]...), from)
		}
	}()
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	return 0
}

// do runs f on an OS thread that joined the namespace, so the sockets f
// creates belong to it. The thread is never unlocked: it ends with its
// goroutine.
func (n *netns) do(f func() error) error {
	errc := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		fd, err := unix.Open(filepath.Join("/run/netns", n.name), unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			errc <- err
			return
		}
		defer func() { _ = unix.Close(fd) }()
		if err := unix.Setns(fd, unix.CLONE_NEWNET); err != nil {
			errc <- err
			return
		}
		errc <- f()
	}()
	return <-errc
}

func (n *netns) dial(network, addr string) (net.Conn, error) {
	var c net.Conn
	err := n.do(func() error {
		var err error
		c, err = net.DialTimeout(network, addr, 3*time.Second)
		return err
	})
	return c, err
}

// exchange writes a line and reads the echo.
func exchange(c net.Conn, msg string) (string, error) {
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.WriteString(c, msg+"\n"); err != nil {
		return "", err
	}
	buf := make([]byte, 256)
	n, err := c.Read(buf)
	return strings.TrimSpace(string(buf[:n])), err
}

// TestNetnsTraffic moves TCP and UDP through a gost relay (RAW in, mutual
// TLS out) and a gost exit (TLS in, RAW to the target) in one namespace,
// checks that an apply that changes the relay reloads gost without
// dropping an established connection or ending the counter epoch, that a
// paused hop refuses new connections, and that the exit refuses a TLS
// client without a link certificate of its CA.
func TestNetnsTraffic(t *testing.T) {
	ns := newNetns(t)
	for _, a := range []string{"10.231.0.1", "10.231.0.2", "10.231.0.10"} {
		ns.addAddress(t, a)
	}
	pki := linkPKI(t, shortDir(t), "forward-31", "forward-41")
	relay, exit := newNode(t, ns, "forward-31", pki), newNode(t, ns, "forward-41", pki)

	srv := exec.Command(ns.ip, "netns", "exec", ns.name, os.Args[0], "-test.run=^$") // #nosec G204 -- the test binary as echo server
	srv.Env = append(os.Environ(), envEcho+"=10.231.0.10:7000")
	stdin, _ := srv.StdinPipe()
	out, _ := srv.StdoutPipe()
	srv.Stderr = os.Stderr
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = srv.Wait() })
	if line, _ := bufio.NewReader(out).ReadString('\n'); line != "ready\n" {
		t.Fatalf("echo server: %q", line)
	}

	const route = "01JF4A000000000000000000A1"
	both := forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP
	link := &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_TLS, Mux: true, ServerName: "forward-41"}
	exitHop := &forwardv1.NodeHop{
		RouteId: route, HopIndex: 2, Role: forwardv1.HopRole_HOP_ROLE_EXIT, Engine: gostE,
		Listen:  &forwardv1.Listen{Address: "10.231.0.2", Port: 20000, Protocol: both},
		Ingress: link,
		Upstreams: []*forwardv1.Upstream{{
			Address: "10.231.0.10", Port: 7000, Weight: 1,
			Egress: &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		}},
		Balance:        failover,
		TargetPolicy:   forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
		IngressSources: []string{"10.231.0.0/24"},
		IngressPeers:   []string{"spiffe://anixops/example/agent/forward-31"},
		Mark:           1,
	}
	relayHop := &forwardv1.NodeHop{
		RouteId: route, HopIndex: 1, Role: forwardv1.HopRole_HOP_ROLE_RELAY, Engine: gostE,
		Listen:  &forwardv1.Listen{Address: "10.231.0.1", Port: 30001, Protocol: both},
		Ingress: &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		Upstreams: []*forwardv1.Upstream{{
			Address: "10.231.0.2", Port: 20000, Weight: 1, Egress: link,
			NodeRef: "forward-41", PeerIdentity: "spiffe://anixops/example/agent/forward-41",
		}},
		Balance:        failover,
		TargetPolicy:   forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
		IngressSources: []string{"10.231.0.0/24"},
		Mark:           1,
	}
	ed, rd := exit.driver(t), relay.driver(t)
	if _, err := ed.Apply(t.Context(), render(t, ed, conformance.State("forward-41", 1, exitHop))); err != nil {
		t.Fatal(err)
	}
	if _, err := rd.Apply(t.Context(), render(t, rd, conformance.State("forward-31", 1, relayHop))); err != nil {
		t.Fatal(err)
	}

	tcpConn, err := ns.dial("tcp", "10.231.0.1:30001")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tcpConn.Close() }()
	if got, err := exchange(tcpConn, "one"); err != nil || got != "echo:one" {
		t.Fatalf("TCP through relay and exit: %q %v", got, err)
	}
	udpConn, err := ns.dial("udp", "10.231.0.1:30001")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = udpConn.Close() }()
	if got, err := exchange(udpConn, "dgram"); err != nil || got != "echo:dgram" {
		t.Fatalf("UDP through relay and exit: %q %v", got, err)
	}

	// A changing apply reloads gost: the established connection and the
	// counter epoch survive.
	before, err := rd.Observe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	relay2 := conformance.State("forward-31", 2, relayHop, &forwardv1.NodeHop{
		RouteId: "01JF4A000000000000000000B1", HopIndex: 0, Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: gostE,
		Listen:       &forwardv1.Listen{Address: "10.231.0.1", Port: 30002, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Upstreams:    []*forwardv1.Upstream{{Address: "10.231.0.10", Port: 7000}},
		TargetPolicy: forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE,
	})
	if r, err := rd.Apply(t.Context(), render(t, rd, relay2)); err != nil || !r.Changed {
		t.Fatalf("changing apply: %+v %v", r, err)
	}
	if got, err := exchange(tcpConn, "two"); err != nil || got != "echo:two" {
		t.Fatalf("established TCP connection after the reload: %q %v", got, err)
	}
	after, err := rd.Observe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if b, a := before.Counters[0].GetCounterEpoch(), after.Counters[0].GetCounterEpoch(); a != b {
		t.Fatalf("the reload ended the counter epoch: %s -> %s", b, a)
	}
	direct, err := ns.dial("tcp", "10.231.0.1:30002")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = direct.Close() }()
	if got, err := exchange(direct, "three"); err != nil || got != "echo:three" {
		t.Fatalf("the added hop: %q %v", got, err)
	}

	// A paused hop keeps its listener and refuses new connections.
	paused := proto.Clone(relayHop).(*forwardv1.NodeHop)
	paused.Paused = true
	if _, err := rd.Apply(t.Context(), render(t, rd, conformance.State("forward-31", 3, paused))); err != nil {
		t.Fatal(err)
	}
	if c, err := ns.dial("tcp", "10.231.0.1:30001"); err == nil {
		got, err := exchange(c, "four")
		_ = c.Close()
		if err == nil {
			t.Fatalf("a paused hop forwarded a new connection: %q", got)
		}
	}

	// Mutual TLS: the exit refuses a client without a certificate.
	pool := x509.NewCertPool()
	ca, _ := os.ReadFile(pki["forward-41"][2]) // #nosec G304 -- the test's CA
	pool.AppendCertsFromPEM(ca)
	raw, err := ns.dial("tcp", "10.231.0.2:20000")
	if err != nil {
		t.Fatal(err)
	}
	c := tls.Client(raw, &tls.Config{ServerName: "forward-41", RootCAs: pool, MinVersion: tls.VersionTLS12})
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	err = c.Handshake()
	if err == nil {
		_, err = c.Read(make([]byte, 1))
	}
	if err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("the exit accepted a TLS client without a certificate: %v", err)
	}
}

func render(t testing.TB, d *gost.Driver, s *forwardv1.NodeForwardState) driver.Artifact {
	t.Helper()
	a, err := d.Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return a
}
