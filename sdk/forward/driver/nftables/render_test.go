package nftables_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/nftables"
)

func TestNewRejectsBadConfig(t *testing.T) {
	cases := map[string]func(*nftables.Config){
		"mask zero":             func(c *nftables.Config) { c.MarkMask = 0 },
		"mask not contiguous":   func(c *nftables.Config) { c.MarkMask = 0x0f0f0000 },
		"direction two bits":    func(c *nftables.Config) { c.DirectionBit = 0x3 },
		"direction zero":        func(c *nftables.Config) { c.DirectionBit = 0 },
		"direction in mask":     func(c *nftables.Config) { c.DirectionBit = 0x00010000 },
		"slots zero":            func(c *nftables.Config) { c.Slots = 0 },
		"slots too many":        func(c *nftables.Config) { c.Slots = nftables.MaxSlots + 1 },
		"unknown strategy":      func(c *nftables.Config) { c.Strategies = append(c.Strategies, 99) },
		"unspecified strategy":  func(c *nftables.Config) { c.Strategies = []forwardv1.BalanceStrategy{0} },
		"interface injection":   func(c *nftables.Config) { c.MSSClampInterfaces = []string{`wg0" } drop; #`} },
		"interface too long":    func(c *nftables.Config) { c.MSSClampInterfaces = []string{"abcdefghijklmnop"} },
		"interface twice":       func(c *nftables.Config) { c.MSSClampInterfaces = []string{"wg0", "wg0"} },
		"version not printable": func(c *nftables.Config) { c.Version = "nft\n1.0" },
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			c := testConfig()
			mod(&c)
			if _, err := nftables.New(c); !errors.Is(err, nftables.ErrInvalidConfig) {
				t.Fatalf("New: %v, want ErrInvalidConfig", err)
			}
		})
	}
	if _, err := nftables.New(nftables.DefaultConfig()); err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
}

func TestConfigIsCopied(t *testing.T) {
	c := testConfig()
	c.MSSClampInterfaces = []string{"wg1", "wg0"}
	d, err := nftables.New(c)
	if err != nil {
		t.Fatal(err)
	}
	c.MSSClampInterfaces[0] = "evil"
	c.Strategies[0] = forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER
	got := d.Config()
	if !slices.Equal(got.MSSClampInterfaces, []string{"wg0", "wg1"}) || got.Strategies[0] != rr {
		t.Fatalf("configuration shares memory with the caller: %+v", got)
	}
}

func TestCapabilities(t *testing.T) {
	d, err := nftables.New(nftables.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	c, err := d.Capabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if c.GetAvailable() || c.GetUnavailableReason() == "" || c.GetEngine() != nftE {
		t.Fatalf("unprobed driver: %v", c)
	}
	c, err = newDriver(t, nil).Capabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := &forwardv1.EngineCapabilities{
		Engine: nftE, Version: "nft (static test configuration)", Available: true, Ipv6: true, Udp: true,
		Strategies:     nftables.AllStrategies(),
		LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		BandwidthLimit: true, Quota: true, MaxConns: true,
	}
	if !proto.Equal(c, want) {
		t.Fatalf("capabilities %v, want %v", c, want)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.Capabilities(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

// TestHostMethodsDoneContext: Apply, Observe, SetUpstreams and Remove
// honour a done context before touching the host.
func TestHostMethodsDoneContext(t *testing.T) {
	d := newDriver(t, nil)
	ctx := t.Context()
	done, cancel := context.WithCancel(ctx)
	cancel()
	_, errA := d.Apply(done, driver.Artifact{})
	_, errO := d.Observe(done)
	errS := d.SetUpstreams(done, conformance.RouteA, 0, nil)
	errR := d.Remove(done)
	for i, err := range []error{errA, errO, errS, errR} {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("method %d with a done context: %v", i, err)
		}
	}
}

func TestRenderNil(t *testing.T) {
	a, err := newDriver(t, nil).Render(nil)
	if !errors.Is(err, driver.ErrInvalidState) || a.Digest != "" || a.Content != nil {
		t.Fatalf("Render(nil) = %+v, %v", a, err)
	}
}

// TestRenderEmpty: no nftables hop, no table statement; the artifact is
// empty and applying it will remove the table.
func TestRenderEmpty(t *testing.T) {
	b := builder(t)
	other := b.Simple(conformance.RouteB, 1)
	other.Engine = forwardv1.Engine_ENGINE_GOST
	d := newDriver(t, nil)
	var first []byte
	for _, s := range []*forwardv1.NodeForwardState{conformance.State("forward-11", 1), conformance.State("forward-11", 2, other)} {
		a, err := d.Render(s)
		if err != nil || !a.Empty() {
			t.Fatalf("Render: %+v, %v", a, err)
		}
		for _, l := range strings.Split(strings.TrimSpace(string(a.Content)), "\n") {
			if !strings.HasPrefix(l, "#") {
				t.Fatalf("empty artifact has a statement: %q", l)
			}
		}
		if first != nil && string(first) != string(a.Content) {
			t.Fatal("empty renders differ")
		}
		first = a.Content
	}
}

// wantAlone renders good and bad together and checks bad alone is rejected
// with want, and that nothing of it is in the script.
func wantAlone(t *testing.T, d *nftables.Driver, good, bad *forwardv1.NodeHop, want error) {
	t.Helper()
	a, err := d.Render(conformance.State("forward-11", 1, good, bad))
	if !errors.Is(err, want) {
		t.Fatalf("Render: %v, want %v", err, want)
	}
	var re *driver.RenderError
	if !errors.As(err, &re) || len(re.Hops) != 1 || re.Hops[0].Key != driver.KeyOf(bad) || re.Hops[0].Engine != nftE {
		t.Fatalf("want one hop error for %s, got %v", driver.KeyOf(bad), err)
	}
	if !slices.Equal(a.Hops, []driver.HopKey{driver.KeyOf(good)}) {
		t.Fatalf("artifact runs %v", a.Hops)
	}
	if err := a.Verify(nftE); err != nil {
		t.Fatal(err)
	}
	if bad.GetRouteId() != good.GetRouteId() && strings.Contains(string(a.Content), "r_"+bad.GetRouteId()+"_") {
		t.Fatal("the rejected hop is in the script")
	}
	// The good hop renders exactly as it does alone.
	alone, err := d.Render(conformance.State("forward-11", 1, good))
	if err != nil || alone.Digest != a.Digest {
		t.Fatalf("the rejected hop changed the good hop's script (%v)", err)
	}
}

func TestRenderRejectsHopsAlone(t *testing.T) {
	b := builder(t)
	v4, v6 := b.Top.UpstreamsV4, b.Top.UpstreamsV6
	type tc struct {
		mod  func(h *forwardv1.NodeHop)
		cfg  func(c *nftables.Config)
		want error
	}
	sec := func(s forwardv1.LinkSecurity) *forwardv1.LinkTransport { return &forwardv1.LinkTransport{Security: s} }
	unsup, invalid := driver.ErrUnsupported, driver.ErrInvalidState
	cases := map[string]tc{
		"ingress tls":          {mod: func(h *forwardv1.NodeHop) { h.Ingress = sec(forwardv1.LinkSecurity_LINK_SECURITY_TLS) }, want: unsup},
		"ingress anixops":      {mod: func(h *forwardv1.NodeHop) { h.Ingress = sec(forwardv1.LinkSecurity_LINK_SECURITY_ANIXOPS) }, want: unsup},
		"ingress unknown":      {mod: func(h *forwardv1.NodeHop) { h.Ingress = sec(42) }, want: unsup},
		"ingress mux":          {mod: func(h *forwardv1.NodeHop) { h.Ingress.Mux = true }, want: unsup},
		"egress quic":          {mod: func(h *forwardv1.NodeHop) { h.Upstreams[1].Egress = sec(forwardv1.LinkSecurity_LINK_SECURITY_QUIC) }, want: unsup},
		"role unspecified":     {mod: func(h *forwardv1.NodeHop) { h.Role = 0 }, want: invalid},
		"role unknown":         {mod: func(h *forwardv1.NodeHop) { h.Role = 9 }, want: unsup},
		"protocol unknown":     {mod: func(h *forwardv1.NodeHop) { h.Listen.Protocol = 7 }, want: unsup},
		"protocol unspecified": {mod: func(h *forwardv1.NodeHop) { h.Listen.Protocol = 0 }, want: invalid},
		"no listen":            {mod: func(h *forwardv1.NodeHop) { h.Listen = nil }, want: invalid},
		"listen port zero":     {mod: func(h *forwardv1.NodeHop) { h.Listen.Port = 0 }, want: invalid},
		"listen port too big":  {mod: func(h *forwardv1.NodeHop) { h.Listen.Port = 70000 }, want: invalid},
		"listen address name":  {mod: func(h *forwardv1.NodeHop) { h.Listen.Address = "eth0" }, want: invalid},
		"listen address zone":  {mod: func(h *forwardv1.NodeHop) { h.Listen.Address = "fe80::1%eth0" }, want: invalid},
		"listen multicast":     {mod: func(h *forwardv1.NodeHop) { h.Listen.Address = "224.0.0.1" }, want: invalid},
		"listen family unused": {mod: func(h *forwardv1.NodeHop) { h.Listen.Address = "2001:db8::11" }, want: invalid},
		"udp disabled": {mod: func(h *forwardv1.NodeHop) { h.Listen.Protocol = udp },
			cfg: func(c *nftables.Config) { c.UDP = false }, want: unsup},
		"ipv6 upstream disabled": {mod: func(h *forwardv1.NodeHop) { h.Upstreams = b.Upstreams(v6, 1) },
			cfg: func(c *nftables.Config) { c.IPv6 = false }, want: unsup},
		"ipv6 listen disabled": {mod: func(h *forwardv1.NodeHop) { h.Listen.Address = "2001:db8::11" },
			cfg: func(c *nftables.Config) { c.IPv6 = false }, want: unsup},
		"strategy disabled": {mod: func(h *forwardv1.NodeHop) { h.Balance = ipHash },
			cfg: func(c *nftables.Config) { c.Strategies = []forwardv1.BalanceStrategy{failover} }, want: unsup},
		"strategy unknown": {mod: func(h *forwardv1.NodeHop) { h.Balance = 99 }, want: unsup},
		"no upstream":      {mod: func(h *forwardv1.NodeHop) { h.Upstreams = nil }, want: invalid},
		"more upstreams than slots": {mod: func(h *forwardv1.NodeHop) { h.Upstreams = append(h.Upstreams, b.Upstreams(v6, 1)...) },
			cfg: func(c *nftables.Config) { c.Slots = 3 }, want: unsup},
		"upstream name":       {mod: func(h *forwardv1.NodeHop) { h.Upstreams[0].Address = "db.example.com" }, want: unsup},
		"upstream garbage":    {mod: func(h *forwardv1.NodeHop) { h.Upstreams[0].Address = "1.2.3.4 . 80, 0 : 6.6.6.6" }, want: invalid},
		"upstream zone":       {mod: func(h *forwardv1.NodeHop) { h.Upstreams[0].Address = "2001:db8::1%eth0" }, want: invalid},
		"upstream loopback":   {mod: func(h *forwardv1.NodeHop) { h.Upstreams[0].Address = "::1" }, want: invalid},
		"upstream any":        {mod: func(h *forwardv1.NodeHop) { h.Upstreams[0].Address = "0.0.0.0" }, want: invalid},
		"upstream link-local": {mod: func(h *forwardv1.NodeHop) { h.Upstreams[0].Address = "169.254.1.1" }, want: invalid},
		"loopback node":       {mod: func(h *forwardv1.NodeHop) { h.Upstreams[0].Address, h.Upstreams[0].NodeRef = "127.0.0.2", "forward-31" }, want: invalid},
		"private target, public only": {mod: func(h *forwardv1.NodeHop) {
			h.Upstreams[0].Address = "10.0.0.1"
			h.TargetPolicy = forwardv1.TargetPolicy_TARGET_POLICY_PUBLIC_ONLY
		}, want: invalid},
		"upstream port zero": {mod: func(h *forwardv1.NodeHop) { h.Upstreams[0].Port = 0 }, want: invalid},
		"upstream port big":  {mod: func(h *forwardv1.NodeHop) { h.Upstreams[0].Port = 65536 }, want: invalid},
		"upstream twice":     {mod: func(h *forwardv1.NodeHop) { h.Upstreams[1].Address = h.Upstreams[0].Address }, want: invalid},
		"upstream mapped twice": {mod: func(h *forwardv1.NodeHop) {
			h.Upstreams[1].Address = "::ffff:" + h.Upstreams[0].Address
		}, want: invalid},
		"weight too big": {mod: func(h *forwardv1.NodeHop) { h.Upstreams[0].Weight = 1001 }, want: invalid},
		"relay without sources": {mod: func(h *forwardv1.NodeHop) {
			h.Role, h.HopIndex = forwardv1.HopRole_HOP_ROLE_RELAY, 1
		}, want: invalid},
		"bad source":  {mod: func(h *forwardv1.NodeHop) { h.IngressSources = []string{"198.51.100.0/24", "any"} }, want: invalid},
		"source zone": {mod: func(h *forwardv1.NodeHop) { h.IngressSources = []string{"fe80::1%eth0"} }, want: invalid},
		"bandwidth disabled": {mod: func(h *forwardv1.NodeHop) { h.Limits = &forwardv1.Limits{BandwidthBps: 1} },
			cfg: func(c *nftables.Config) { c.BandwidthLimit = false }, want: unsup},
		"quota disabled": {mod: func(h *forwardv1.NodeHop) { h.Limits = &forwardv1.Limits{QuotaBytes: 1} },
			cfg: func(c *nftables.Config) { c.Quota = false }, want: unsup},
		"max conns disabled": {mod: func(h *forwardv1.NodeHop) { h.Limits = &forwardv1.Limits{MaxConns: 1} },
			cfg: func(c *nftables.Config) { c.MaxConns = false }, want: unsup},
		"mark zero":        {mod: func(h *forwardv1.NodeHop) { h.Mark = 0 }, want: invalid},
		"mark beyond mask": {mod: func(h *forwardv1.NodeHop) { h.Mark = 4096 }, want: invalid},
		"mark beyond custom mask": {mod: func(h *forwardv1.NodeHop) { h.Mark = 256 },
			cfg: func(c *nftables.Config) { c.MarkMask = 0x0000ff00 }, want: invalid},
	}
	for _, id := range []string{"", "has space", `quote"`, "semi;colon", "new\nline", "dash-ed", "under_score", "ünï", strings.Repeat("A", 65)} {
		cases["route id "+strings.ReplaceAll(id, "\n", `\n`)] = tc{mod: func(h *forwardv1.NodeHop) { h.RouteId = id }, want: invalid}
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			good := b.Simple(conformance.RouteA, 0)
			bad := b.Simple(conformance.RouteB, 1)
			bad.Upstreams = b.Upstreams(v4, 3)
			c.mod(bad)
			wantAlone(t, newDriver(t, c.cfg), good, bad, c.want)
		})
	}
}

// TestRenderCollisions: hops sharing a mark or a listener are all rejected,
// whatever their order; hops that do not overlap are kept.
func TestRenderCollisions(t *testing.T) {
	b := builder(t)
	d := newDriver(t, nil)
	render := func(hops ...*forwardv1.NodeHop) ([]driver.HopKey, []driver.HopKey) {
		t.Helper()
		var firstHops, firstErrs []driver.HopKey
		for i := range 2 {
			if i == 1 {
				slices.Reverse(hops)
			}
			a, err := d.Render(conformance.State("forward-11", 1, hops...))
			var errKeys []driver.HopKey
			var re *driver.RenderError
			if errors.As(err, &re) {
				for _, h := range re.Hops {
					if !errors.Is(h, driver.ErrInvalidState) {
						t.Fatalf("collision error %v, want ErrInvalidState", h)
					}
					errKeys = append(errKeys, h.Key)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				firstHops, firstErrs = a.Hops, errKeys
			} else if !slices.Equal(firstHops, a.Hops) || !slices.Equal(firstErrs, errKeys) {
				t.Fatal("the result depends on the order of the hops")
			}
		}
		return firstHops, firstErrs
	}
	key := func(h *forwardv1.NodeHop) driver.HopKey { return driver.KeyOf(h) }
	a, c := b.Simple(conformance.RouteA, 0), b.Simple(conformance.RouteC, 2)

	sameMark := b.Simple(conformance.RouteB, 1)
	sameMark.Mark = a.Mark
	if ok, bad := render(a, sameMark, c); !slices.Equal(ok, []driver.HopKey{key(c)}) || len(bad) != 2 {
		t.Fatalf("same mark: ran %v, rejected %v", ok, bad)
	}

	samePort := b.Simple(conformance.RouteB, 1)
	samePort.Listen.Port = a.Listen.Port
	if ok, bad := render(a, samePort, c); !slices.Equal(ok, []driver.HopKey{key(c)}) || len(bad) != 2 {
		t.Fatalf("same port: ran %v, rejected %v", ok, bad)
	}

	both := b.Simple(conformance.RouteB, 1)
	both.Listen.Port, both.Listen.Protocol = a.Listen.Port, forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP
	if _, bad := render(a, both); len(bad) != 2 {
		t.Fatalf("tcp+udp over tcp: rejected %v", bad)
	}

	otherProto := b.Simple(conformance.RouteB, 1)
	otherProto.Listen.Port, otherProto.Listen.Protocol = a.Listen.Port, udp
	if ok, bad := render(a, otherProto); len(ok) != 2 || len(bad) != 0 {
		t.Fatalf("tcp and udp on one port: ran %v, rejected %v", ok, bad)
	}

	addrA, addrB := b.Simple(conformance.RouteA, 0), b.Simple(conformance.RouteB, 1)
	addrA.Listen.Address, addrB.Listen.Address = "192.0.2.11", "192.0.2.12"
	addrB.Listen.Port = addrA.Listen.Port
	if ok, bad := render(addrA, addrB); len(ok) != 2 || len(bad) != 0 {
		t.Fatalf("two addresses on one port: ran %v, rejected %v", ok, bad)
	}
	wild := b.Simple(conformance.RouteC, 2)
	wild.Listen.Port = addrA.Listen.Port
	if ok, bad := render(addrA, addrB, wild); len(ok) != 0 || len(bad) != 3 {
		t.Fatalf("every address and one address on one port: ran %v, rejected %v", ok, bad)
	}
	sameAddr := b.Simple(conformance.RouteB, 1)
	sameAddr.Listen.Address, sameAddr.Listen.Port = "192.0.2.11", addrA.Listen.Port
	if _, bad := render(addrA, sameAddr); len(bad) != 2 {
		t.Fatalf("one address twice: rejected %v", bad)
	}
}

func TestRenderDuplicateHop(t *testing.T) {
	b := builder(t)
	dup := b.Simple(conformance.RouteB, 1)
	a, err := newDriver(t, nil).Render(conformance.State("forward-11", 1, b.Simple(conformance.RouteA, 0), dup, proto.Clone(dup).(*forwardv1.NodeHop)))
	if !errors.Is(err, driver.ErrInvalidState) || !slices.Equal(a.Hops, []driver.HopKey{{RouteID: conformance.RouteA}}) {
		t.Fatalf("Render: %v, hops %v", err, a.Hops)
	}
}

// TestRenderIgnoresFreeText: names, identities, hostnames and paths of the
// state never reach the script.
func TestRenderIgnoresFreeText(t *testing.T) {
	b := builder(t)
	plain := b.Relay(conformance.RouteA, 0, b.Upstreams(b.Top.UpstreamsV4, 2))
	d := newDriver(t, nil)
	want, err := d.Render(conformance.State("forward-11", 1, plain))
	if err != nil {
		t.Fatal(err)
	}
	evil := `evil"; flush ruleset; table ip x { chain y { } } #`
	h := proto.Clone(plain).(*forwardv1.NodeHop)
	h.Listen.EntryHostname = evil
	h.Ingress.ServerName, h.Ingress.Path = evil, evil
	h.IngressPeers = []string{evil}
	for _, u := range h.Upstreams {
		u.NodeRef, u.PeerIdentity = evil, evil
		u.Egress.ServerName, u.Egress.Path = evil, evil
	}
	h.Health = &forwardv1.HealthCheck{IntervalMs: 1, TimeoutMs: 1}
	h.Limits = &forwardv1.Limits{ExpiresAtUnixMs: 1}
	s := conformance.State(evil, 99, h)
	s.StateHash = evil
	got, err := d.Render(s)
	if err != nil {
		t.Fatal(err)
	}
	if got.Digest != want.Digest {
		t.Fatal("free text or identity changed the script")
	}
}

// TestRegistry: the driver registers and the registry splits a mixed state.
func TestRegistry(t *testing.T) {
	b := builder(t)
	reg := driver.NewRegistry()
	if err := reg.Register(newDriver(t, nil)); err != nil {
		t.Fatal(err)
	}
	gost := b.Simple(conformance.RouteB, 1)
	gost.Engine = forwardv1.Engine_ENGINE_GOST
	arts, hopErrs, err := reg.Render(conformance.State("forward-11", 3, b.Simple(conformance.RouteA, 0), gost))
	if err != nil {
		t.Fatal(err)
	}
	a := arts[nftE]
	if !slices.Equal(a.Hops, []driver.HopKey{{RouteID: conformance.RouteA}}) || a.Generation != 3 {
		t.Fatalf("nftables artifact %+v", a)
	}
	if len(hopErrs) != 1 || hopErrs[0].Key.RouteID != conformance.RouteB || !errors.Is(hopErrs[0], driver.ErrUnsupported) {
		t.Fatalf("hop errors %v", hopErrs)
	}
}
