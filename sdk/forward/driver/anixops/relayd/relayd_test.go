package relayd_test

import (
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayd"
)

func TestRawTCPEchoCountsAndHalfCloses(t *testing.T) {
	p := newPKI(t)
	n := newNode(t, p, "forward-1", relayd.Options{})
	host, port := echoTCP(t)
	entry := freePort(t)
	n.mustApply(rawHop(entry, true, false, target(host, port)))

	c := dialTCP(t, entry)
	data := pattern(200_000)
	roundTrip(t, c, data)
	// Half-close: the target echoes what is left, then closes; the client
	// sees EOF after its own CloseWrite.
	if err := c.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(testWait))
	if _, err := io.ReadAll(c); err != nil {
		t.Fatalf("after CloseWrite: %v", err)
	}
	waitFor(t, "the connection to end", func() bool { return n.hopObs(route, 0).Active == 0 })
	o := n.hopObs(route, 0)
	if o.UpBytes != uint64(len(data)) || o.DownBytes != uint64(len(data)) || o.Total != 1 {
		t.Fatalf("counters up %d down %d total %d, want %d %d 1", o.UpBytes, o.DownBytes, o.Total, len(data), len(data))
	}
	if o.Epoch == "" || len(o.Rotation) != 1 {
		t.Fatalf("observation %+v", o)
	}
}

func TestChainsOverEveryCarrier(t *testing.T) {
	for _, carrier := range []string{relayctl.CarrierTLSTCP, relayctl.CarrierQUIC, relayctl.CarrierAuto, relayctl.CarrierPlain} {
		t.Run(carrier, func(t *testing.T) {
			c := newChain(t, carrier, relayd.Options{})
			conn := dialTCP(t, c.entryPort)
			data := pattern(300_000)
			roundTrip(t, conn, data)
			// Two more connections share the carriers.
			for range 2 {
				extra := dialTCP(t, c.entryPort)
				roundTrip(t, extra, pattern(5000))
				_ = extra.Close()
			}
			if err := conn.(*net.TCPConn).CloseWrite(); err != nil {
				t.Fatal(err)
			}
			_ = conn.SetReadDeadline(time.Now().Add(testWait))
			if _, err := io.ReadAll(conn); err != nil {
				t.Fatalf("EOF through three hops: %v", err)
			}
			waitFor(t, "every connection to end", func() bool {
				return c.entry.hopObs(route, 0).Active == 0 && c.mid.hopObs(route, 1).Active == 0 && c.exit.hopObs(route, 2).Active == 0
			})
			want := uint64(300_000 + 2*5000)
			for _, h := range []struct {
				n   *node
				hop uint32
			}{{c.entry, 0}, {c.mid, 1}, {c.exit, 2}} {
				o := h.n.hopObs(route, h.hop)
				if o.UpBytes != want || o.DownBytes != want || o.Total != 3 {
					t.Errorf("hop %d: up %d down %d total %d, want %d %d 3", h.hop, o.UpBytes, o.DownBytes, o.Total, want, want)
				}
			}
		})
	}
}

func TestUDPThroughTheChain(t *testing.T) {
	for _, carrier := range []string{relayctl.CarrierTLSTCP, relayctl.CarrierQUIC} {
		t.Run(carrier, func(t *testing.T) {
			p := newPKI(t)
			entry, mid, exit := newNode(t, p, "forward-1", relayd.Options{}), newNode(t, p, "forward-2", relayd.Options{}), newNode(t, p, "forward-3", relayd.Options{})
			host, port := echoUDP(t)
			midPort, exitPort, entryPort := freePort(t), freePort(t), freePort(t)
			x := ingress(2, exitPort, carrier, "exit", []string{identity("forward-2")}, target(host, port))
			x.Listen.UDP, x.Listen.TCP = true, true
			exit.mustApply(x)
			m := ingress(1, midPort, carrier, "relay", []string{identity("forward-1")}, next(exitPort, carrier, "forward-3"))
			m.Listen.UDP, m.Listen.TCP = true, true
			mid.mustApply(m)
			entry.mustApply(rawHop(entryPort, false, true, next(midPort, carrier, "forward-2")))

			uc, err := net.Dial("udp", "127.0.0.1:"+strconv.Itoa(int(entryPort)))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = uc.Close() }()
			for i := range 5 {
				msg := []byte{byte('a' + i), 'x', 'y', 'z'}
				var got []byte
				// UDP may lose the first datagrams while the association opens.
				waitFor(t, "an echo", func() bool {
					_, _ = uc.Write(msg)
					_ = uc.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
					buf := make([]byte, 100)
					n, err := uc.Read(buf)
					got = buf[:n]
					return err == nil
				})
				if string(got) != string(msg) {
					t.Fatalf("echo %q, want %q", got, msg)
				}
			}
			o := entry.hopObs(route, 0)
			if o.UpPackets < 5 || o.DownPackets < 5 || o.UpBytes == 0 || o.DownBytes == 0 {
				t.Fatalf("entry counters %+v", o)
			}
			if x := exit.hopObs(route, 2); x.UpPackets < 5 || x.DownPackets < 5 {
				t.Fatalf("exit counters %+v", x)
			}
		})
	}
}
