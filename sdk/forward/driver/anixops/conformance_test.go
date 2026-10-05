package anixops_test

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/anixopstest"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
)

// freePort answers a port free for TCP and UDP on every address.
func freePort(t testing.TB) uint32 {
	t.Helper()
	for range 50 {
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		pc, err := net.ListenPacket("udp", ":"+strconv.Itoa(port))
		_ = l.Close()
		if err != nil {
			continue
		}
		_ = pc.Close()
		return uint32(port) // #nosec G115 -- a port
	}
	t.Fatal("no free port")
	return 0
}

// echo serves TCP and UDP echo servers and answers their addresses.
func echo(t testing.TB) (tcp, udp string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 65535)
		for {
			n, a, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buf[:n], a)
		}
	}()
	return l.Addr().String(), pc.LocalAddr().String()
}

// env is one conformance host: the in-process relay and its directories.
type env struct {
	t        testing.TB
	host     *anixopstest.Host
	foreign  []string
	conflict []net.Listener
}

func newEnv(t *testing.T) conformance.Env {
	host := anixopstest.NewHost(t)
	tcp, udp := echo(t)
	host.SetTarget("tcp", tcp)
	host.SetTarget("udp", udp)
	return &env{t: t, host: host}
}

func (e *env) NewDriver(t testing.TB) driver.Driver { return e.host.Driver() }

func (e *env) fileExists(name string) bool {
	_, err := os.Stat(filepath.Join(e.host.Dir, name))
	return err == nil
}

// Owned lists the files the driver wrote (when its state file is there) and
// the hops the relay runs with whether they listen.
func (e *env) Owned(t testing.TB) []string {
	var out []string
	if e.fileExists(anixops.StateFile) {
		for _, name := range []string{anixops.ConfigFile, anixops.StateFile, anixops.ResetKeyFile} {
			if e.fileExists(name) {
				out = append(out, "file "+name)
			}
		}
	}
	if r := e.host.Relay(); r != nil {
		for _, h := range r.Status().Hops {
			out = append(out, fmt.Sprintf("hop %s/%d listening=%t", h.Route, h.Hop, h.Listening))
		}
	}
	slices.Sort(out)
	return out
}

func (e *env) PlantForeign(t testing.TB) {
	if err := os.WriteFile(filepath.Join(e.host.Dir, "foreign.json"), []byte(`{"someone":"else"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	e.conflict = append(e.conflict, l)
	e.foreign = append(e.foreign, "file foreign.json", "listener "+l.Addr().String())
}

func (e *env) Foreign(t testing.TB) []string {
	out := slices.Clone(e.foreign)
	if e.fileExists(anixops.ConfigFile) && !e.fileExists(anixops.StateFile) {
		out = append(out, "file relay.json (not the driver's)")
	}
	slices.Sort(out)
	return out
}

func (e *env) Damage(t testing.TB) {
	if err := os.Remove(filepath.Join(e.host.Dir, anixops.ConfigFile)); err != nil {
		t.Fatal(err)
	}
}

func (e *env) FailNextApply(t testing.TB) { e.host.FailNextApply() }

func (e *env) Applies(t testing.TB) int { return e.host.Applies() }

func (e *env) PlantConflict(t testing.TB, port uint32) {
	addr := ":" + strconv.FormatUint(uint64(port), 10)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close(); _ = pc.Close() })
	e.foreign = append(e.foreign, "conflict "+addr)
}

func (e *env) PlantImpostor(t testing.TB) {
	if err := os.WriteFile(filepath.Join(e.host.Dir, anixops.ConfigFile), []byte(`{"impostor":true}`), 0o640); err != nil {
		t.Fatal(err)
	}
}

// Traffic sends a payload through the hop's listener and waits for the echo,
// then for the connection to end, so the counters have it.
func (e *env) Traffic(t testing.TB, hop driver.HopKey) {
	var port uint32
	var tcp bool
	for _, h := range e.host.AppliedHops() {
		if h.Route == hop.RouteID && h.Hop == hop.HopIndex {
			port, tcp = h.Listen.Port, h.Listen.TCP
		}
	}
	if port == 0 || !tcp {
		return
	}
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(int(port)), 2*time.Second)
	if err != nil {
		return
	}
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	msg := []byte("anixops conformance traffic")
	if _, err := c.Write(msg); err == nil {
		buf := make([]byte, len(msg))
		_, _ = io.ReadFull(c, buf)
	}
	_ = c.Close()
	if r := e.host.Relay(); r != nil {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			active := uint32(0)
			for _, h := range r.Observe(false).Hops {
				active += h.Active
			}
			if active == 0 {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestConformance(t *testing.T) {
	top := conformance.DefaultTopology()
	top.ListenPorts = []uint32{freePort(t), freePort(t), freePort(t), freePort(t)}
	top.IngressSources = []string{"127.0.0.1"}
	conformance.Run(t, newEnv, conformance.WithTopology(top))
}
