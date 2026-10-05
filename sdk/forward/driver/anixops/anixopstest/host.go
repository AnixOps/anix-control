package anixopstest

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayd"
	"github.com/AnixOps/anix-control/sdk/forward/relay"
)

// Host is one node's anixops state in a temporary directory, supervised
// in-process. It implements anixops.Supervisor.
type Host struct {
	t testing.TB
	// Dir and RuntimeDir are the driver's directories; Config is the driver
	// configuration for them (every feature on, PLAIN carrier only).
	Dir, RuntimeDir string
	Config          anixops.Config

	mu      sync.Mutex
	relay   *relayd.Relay
	cancel  context.CancelFunc
	served  chan struct{}
	applies uint64 // of relays that ended
	starts  int

	failNext atomic.Bool
	// Dial redirects the relay's dials to targets: every dial, whatever its
	// address, connects to the address Target answers for the network.
	targetMu sync.Mutex
	targets  map[string]string
}

var _ anixops.Supervisor = (*Host)(nil)

// NewHost makes a host whose driver configuration offers every feature and the
// PLAIN carrier, the probed version "anixops-relay test". The relay is not
// started until the driver applies something.
func NewHost(t testing.TB) *Host {
	t.Helper()
	root := t.TempDir()
	h := &Host{t: t, Dir: filepath.Join(root, "lib"), RuntimeDir: filepath.Join(root, "run"), targets: map[string]string{}}
	for _, d := range []string{h.Dir, h.RuntimeDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	cfg := anixops.DefaultConfig()
	cfg.Dir, cfg.RuntimeDir = h.Dir, h.RuntimeDir
	cfg.LinkCert, cfg.LinkKey, cfg.LinkCA = "", "", ""
	cfg.Carriers = []forwardv1.AnixOpsCarrier{forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_PLAIN}
	cfg.Version = "anixops-relay test"
	h.Config = cfg
	t.Cleanup(h.Close)
	return h
}

// Driver makes a driver instance on the host; calling it again simulates an
// Agent restart.
func (h *Host) Driver(opts ...anixops.Option) *anixops.Driver {
	h.t.Helper()
	d, err := anixops.New(h.Config, append([]anixops.Option{anixops.WithSupervisor(h)}, opts...)...)
	if err != nil {
		h.t.Fatal(err)
	}
	return d
}

// SetTarget makes every dial of the given network ("tcp" or "udp") connect to
// addr.
func (h *Host) SetTarget(network, addr string) {
	h.targetMu.Lock()
	h.targets[network] = addr
	h.targetMu.Unlock()
}

func (h *Host) dial(ctx context.Context, network, address string) (net.Conn, error) {
	h.targetMu.Lock()
	to, ok := h.targets[network]
	h.targetMu.Unlock()
	if !ok {
		to = address
	}
	var d net.Dialer
	return d.DialContext(ctx, network, to)
}

// FailNextApply makes the relay's next Apply fail before it changes anything.
func (h *Host) FailNextApply() { h.failNext.Store(true) }

// Check implements anixops.Supervisor: the unit is installed.
func (h *Host) Check(ctx context.Context) error { return ctx.Err() }

// Status implements anixops.Supervisor.
func (h *Host) Status(ctx context.Context) (anixops.Status, error) {
	if err := ctx.Err(); err != nil {
		return anixops.Status{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return anixops.Status{Running: h.relay != nil}, nil
}

// Start implements anixops.Supervisor: it starts a relay that loads the
// configuration file and serves the control socket, as the unit does.
func (h *Host) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.relay != nil {
		return nil
	}
	r := relayd.New(relayd.Options{
		Dial:    h.dial,
		UDPIdle: 5 * time.Second,
		Carrier: relay.Config{},
		Fault: func() error {
			if h.failNext.CompareAndSwap(true, false) {
				return errors.New("injected apply failure")
			}
			return nil
		},
	})
	if err := r.LoadFile(filepath.Join(h.Dir, anixops.ConfigFile)); err != nil {
		return err
	}
	rctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = r.Serve(rctx, filepath.Join(h.RuntimeDir, anixops.ControlSocket))
	}()
	h.relay, h.cancel, h.served = r, cancel, served
	h.starts++
	return nil
}

// Stop implements anixops.Supervisor.
func (h *Host) Stop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.stop()
	return nil
}

// Kill stops the relay without the driver knowing, as a crash would.
func (h *Host) Kill() { h.stop() }

func (h *Host) stop() {
	h.mu.Lock()
	r, cancel, served := h.relay, h.cancel, h.served
	h.relay, h.cancel, h.served = nil, nil, nil
	h.mu.Unlock()
	if r == nil {
		return
	}
	cancel()
	<-served
	st := r.Status()
	r.Close()
	h.mu.Lock()
	h.applies += st.Applies
	h.mu.Unlock()
}

// Close stops the relay (it is registered with t.Cleanup).
func (h *Host) Close() { h.stop() }

// Relay answers the running relay, or nil.
func (h *Host) Relay() *relayd.Relay {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.relay
}

// Starts counts the relays started.
func (h *Host) Starts() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.starts
}

// Applies counts the configurations the relays applied, across restarts: the
// applies that changed what runs.
func (h *Host) Applies() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := h.applies
	if h.relay != nil {
		n += h.relay.Status().Applies
	}
	return int(n) // #nosec G115 -- a test counter
}

// AppliedHops reads the hops of the configuration file the driver last wrote.
func (h *Host) AppliedHops() []relayctl.Hop {
	b, err := os.ReadFile(filepath.Join(h.Dir, anixops.ConfigFile))
	if err != nil {
		return nil
	}
	c, err := relayctl.Parse(b)
	if err != nil {
		return nil
	}
	return c.Hops
}
