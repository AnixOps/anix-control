package conformance

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// Scenarios answers the suite's scenarios in the order Run runs them.
func Scenarios() []Scenario {
	return []Scenario{
		{"capabilities", "Capabilities names the driver's engine; an unavailable driver says why", scCapabilities},
		{"render-determinism", "every state case renders to the same bytes every time", scRenderDeterminism},
		{"render-order-independent", "hop order in the state does not change the artifact", scRenderOrder},
		{"render-ignores-identity", "generation, state_hash and node_ref ride along but do not change content or digest", scRenderIdentity},
		{"render-ignores-other-engines", "hops of other engines are left out silently", scRenderOtherEngines},
		{"render-empty-state", "a state with none of the engine's hops renders an empty artifact that owns nothing", scRenderEmpty},
		{"render-does-not-mutate", "Render leaves its input unchanged", scRenderNoMutate},
		{"render-unsupported-capability", "a hop needing a feature the capabilities lack is rejected alone with ErrUnsupported", scRenderUnsupported},
		{"render-unknown-enum", "unknown enum values are ErrUnsupported", scRenderUnknownEnum},
		{"render-invalid-state", "a nil state, a duplicate hop or a missing listen port is ErrInvalidState", scRenderInvalid},
		{"apply-cases", "every state case applies, observes as applied and removes", scApplyCases},
		{"apply-idempotent", "applying what the host runs changes nothing", scApplyIdempotent},
		{"apply-newer-generation-same-content", "a newer generation with the same hops is a no-op that records the generation", scApplyNewerSame},
		{"apply-stale-generation", "an older generation is ErrStaleGeneration and changes nothing", scApplyStale},
		{"apply-generation-conflict", "the same generation with other content is ErrGenerationConflict", scApplyGenConflict},
		{"apply-invalid-artifact", "a tampered or foreign-engine artifact is refused", scApplyInvalidArtifact},
		{"apply-restart", "a new driver instance observes and guards what an earlier one applied", scApplyRestart},
		{"apply-repair-partial-state", "after damage and a restart, re-applying the same artifact repairs the host", scApplyRepair},
		{"apply-failure-keeps-previous", "a failed apply leaves the previous state running", scApplyFailure},
		{"apply-conflict-foreign", "a foreign object holding a listen port is ErrConflict, nothing changes", scApplyConflict},
		{"apply-not-owned", "an object with the driver's name but not its mark is ErrNotOwned and left alone", scApplyNotOwned},
		{"apply-empty-removes", "applying an empty artifact removes every owned object", scApplyEmpty},
		{"counters-kept-across-reapply", "hops kept by a changing apply keep their counter epoch and values", scCountersKept},
		{"apply-leaves-unrelated-hops", "adding, changing and removing other hops keeps a hop's counter epoch, values and traffic", scUnrelatedHops},
		{"counters-recreated-hop", "a hop removed and added again never reports lower counters in the same epoch", scCountersRecreated},
		{"observe-monotonic", "counters never go down within an epoch across observations and restarts", scObserveMonotonic},
		{"set-upstreams-failover", "SetUpstreams changes rotation without a full apply", scSetUpstreamsFailover},
		{"set-upstreams-weights", "SetUpstreams sets weights; 0 keeps the rendered weight", scSetUpstreamsWeights},
		{"set-upstreams-errors", "unknown hops and upstreams are ErrNotFound, empty or duplicate selections ErrInvalidArgument", scSetUpstreamsErrors},
		{"set-upstreams-reset-by-apply", "a no-op apply keeps rotation, a changing apply restores every upstream", scSetUpstreamsReset},
		{"set-upstreams-survives-restart", "rotation lives on the host", scSetUpstreamsRestart},
		{"remove", "Remove leaves no owned object, never touches foreign ones, and is idempotent", scRemove},
		{"context-cancelled", "a done context returns its error and changes nothing", scContext},
		{"concurrent-calls", "concurrent Apply, Observe, SetUpstreams, Render and Capabilities are safe and observations are consistent", scConcurrent},
	}
}

func scCapabilities(t *testing.T, h *H) {
	if h.Caps.GetEngine() != h.D.Engine() {
		t.Fatalf("Capabilities engine %v, Engine() %v", h.Caps.GetEngine(), h.D.Engine())
	}
	if h.D.Engine() == forwardv1.Engine_ENGINE_UNSPECIFIED {
		t.Fatal("driver for ENGINE_UNSPECIFIED")
	}
	if !h.Caps.GetAvailable() && h.Caps.GetUnavailableReason() == "" {
		t.Fatal("unavailable without a reason")
	}
	if h.Caps.GetAvailable() && h.Caps.GetVersion() == "" {
		t.Fatal("available without a version")
	}
	reg := driver.NewRegistry()
	if err := reg.Register(h.D); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if d, ok := reg.Get(h.D.Engine()); !ok || d != h.D {
		t.Fatal("Registry.Get does not answer the registered driver")
	}
	h.WantErr("Register twice", reg.Register(h.D), driver.ErrDuplicateEngine)
}

// forCases runs f for every state case the capabilities cover.
func forCases(t *testing.T, h *H, f func(t *testing.T, h *H, hops []*forwardv1.NodeHop)) {
	for _, c := range StateCases() {
		t.Run(c.Name, func(t *testing.T) {
			if reason := c.Requires(h.Caps); reason != "" {
				t.Skip(reason)
			}
			f(t, h.Sub(t), c.Hops(h.B))
		})
	}
}

func scRenderDeterminism(t *testing.T, h *H) {
	forCases(t, h, func(t *testing.T, h *H, hops []*forwardv1.NodeHop) {
		s := h.State(1, hops...)
		a1, err1 := h.D.Render(s)
		a2, err2 := h.D.Render(proto.Clone(s).(*forwardv1.NodeForwardState))
		if err1 != nil || err2 != nil {
			t.Fatalf("Render: %v / %v", err1, err2)
		}
		if a1.Digest != a2.Digest || !slices.Equal(a1.Content, a2.Content) || !slices.Equal(a1.Hops, a2.Hops) {
			t.Fatal("two renders of one state differ")
		}
		if err := a1.Verify(h.D.Engine()); err != nil {
			t.Fatal(err)
		}
		if len(a1.Hops) != len(hops) {
			t.Fatalf("artifact runs %d hops, state has %d", len(a1.Hops), len(hops))
		}
	})
}

func scRenderOrder(t *testing.T, h *H) {
	hops := []*forwardv1.NodeHop{h.B.Simple(RouteA, 0), h.B.Simple(RouteB, 1), h.B.Simple(RouteC, 2)}
	rev := slices.Clone(hops)
	slices.Reverse(rev)
	a1 := h.Render(h.State(1, hops...))
	a2 := h.Render(h.State(1, rev...))
	if a1.Digest != a2.Digest {
		t.Fatal("reordering hops changed the digest")
	}
}

func scRenderIdentity(t *testing.T, h *H) {
	hop := h.B.Simple(RouteA, 0)
	a1 := h.Render(h.State(1, hop))
	s2 := h.State(7, hop)
	s2.NodeRef = "forward-99"
	s2.StateHash = "0000"
	a2 := h.Render(s2)
	if a1.Digest != a2.Digest || !slices.Equal(a1.Content, a2.Content) {
		t.Fatal("generation, state_hash or node_ref changed the content")
	}
	if a2.Generation != 7 || a2.NodeRef != "forward-99" || a2.StateHash != "0000" {
		t.Fatalf("artifact identity not carried: %+v", a2)
	}
}

func otherEngine(e forwardv1.Engine) forwardv1.Engine {
	if e == forwardv1.Engine_ENGINE_GOST {
		return forwardv1.Engine_ENGINE_NFTABLES
	}
	return forwardv1.Engine_ENGINE_GOST
}

func scRenderOtherEngines(t *testing.T, h *H) {
	hop := h.B.Simple(RouteA, 0)
	other := h.B.Simple(RouteB, 1)
	other.Engine = otherEngine(h.D.Engine())
	other.Ingress = &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_TLS}
	a1 := h.Render(h.State(1, hop))
	a2 := h.Render(h.State(1, hop, other))
	if a1.Digest != a2.Digest || !slices.Equal(a2.Hops, []driver.HopKey{keyOf(hop)}) {
		t.Fatal("another engine's hop changed the artifact")
	}
}

func scRenderEmpty(t *testing.T, h *H) {
	other := h.B.Simple(RouteB, 1)
	other.Engine = otherEngine(h.D.Engine())
	for _, s := range []*forwardv1.NodeForwardState{h.State(1), h.State(1, other)} {
		a := h.Render(s)
		if !a.Empty() {
			t.Fatalf("empty state rendered hops %v", a.Hops)
		}
		h.Apply(a)
		if got := h.Owned(); len(got) != 0 {
			t.Fatalf("empty artifact left owned objects %q", got)
		}
		if o := h.Observe(); o.Applied {
			t.Fatal("empty artifact observed as applied")
		}
	}
}

func scRenderNoMutate(t *testing.T, h *H) {
	forCases(t, h, func(t *testing.T, h *H, hops []*forwardv1.NodeHop) {
		s := h.State(1, hops...)
		before := proto.Clone(s)
		if _, err := h.D.Render(s); err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(before, s) {
			t.Fatal("Render changed its input")
		}
	})
}

// unsupportedHops answers, for every feature the capabilities lack, a hop
// that needs it.
func unsupportedHops(h *H) map[string]*forwardv1.NodeHop {
	out := map[string]*forwardv1.NodeHop{}
	c := h.Caps
	for _, s := range []forwardv1.LinkSecurity{
		forwardv1.LinkSecurity_LINK_SECURITY_TLS,
		forwardv1.LinkSecurity_LINK_SECURITY_WSS,
		forwardv1.LinkSecurity_LINK_SECURITY_QUIC,
		forwardv1.LinkSecurity_LINK_SECURITY_GRPC,
		forwardv1.LinkSecurity_LINK_SECURITY_ANIXOPS,
	} {
		if !slices.Contains(c.GetLinkSecurities(), s) {
			hop := h.B.Simple(RouteB, 1)
			hop.Ingress = &forwardv1.LinkTransport{Security: s}
			out["ingress-"+s.String()] = hop
		}
	}
	for _, s := range []forwardv1.BalanceStrategy{
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN,
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM,
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH,
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN,
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER,
	} {
		if !slices.Contains(c.GetStrategies(), s) {
			hop := h.B.Simple(RouteB, 1)
			hop.Balance = s
			out["strategy-"+s.String()] = hop
		}
	}
	if !c.GetUdp() {
		hop := h.B.Simple(RouteB, 1)
		hop.Listen.Protocol = forwardv1.L4Protocol_L4_PROTOCOL_UDP
		out["udp"] = hop
	}
	if !c.GetIpv6() {
		hop := h.B.Simple(RouteB, 1)
		hop.Upstreams = h.B.Upstreams(h.B.Top.UpstreamsV6, 1)
		out["ipv6"] = hop
	}
	if !c.GetBandwidthLimit() {
		hop := h.B.Simple(RouteB, 1)
		hop.Limits = &forwardv1.Limits{BandwidthBps: 1_000_000}
		out["bandwidth"] = hop
	}
	if !c.GetQuota() {
		hop := h.B.Simple(RouteB, 1)
		hop.Limits = &forwardv1.Limits{QuotaBytes: 1 << 30}
		out["quota"] = hop
	}
	if !c.GetMaxConns() {
		hop := h.B.Simple(RouteB, 1)
		hop.Limits = &forwardv1.Limits{MaxConns: 10}
		out["max-conns"] = hop
	}
	return out
}

// wantRejected renders good+bad and checks bad alone is rejected with want.
func wantRejected(t *testing.T, h *H, bad *forwardv1.NodeHop, want error) {
	t.Helper()
	good := h.B.Simple(RouteA, 0)
	a, err := h.D.Render(h.State(1, good, bad))
	if !errors.Is(err, want) {
		t.Fatalf("Render: error %v, want %v", err, want)
	}
	var re *driver.RenderError
	if !errors.As(err, &re) || len(re.Hops) != 1 || re.Hops[0].Key != keyOf(bad) {
		t.Fatalf("Render: want one *HopError for %s, got %v", keyOf(bad), err)
	}
	if p := re.Hops[0].ToProto(); p.GetRouteId() != bad.GetRouteId() || p.GetHopIndex() != bad.GetHopIndex() || p.GetEngine() != h.D.Engine() || p.GetMessage() == "" {
		t.Fatalf("HopError.ToProto: %v", p)
	}
	if !slices.Equal(a.Hops, []driver.HopKey{keyOf(good)}) {
		t.Fatalf("artifact runs %v, want only the good hop %s", a.Hops, keyOf(good))
	}
	if err := a.Verify(h.D.Engine()); err != nil {
		t.Fatal(err)
	}
	h.Apply(a)
	if o := h.Observe(); !slices.Equal(keysOf(o), []driver.HopKey{keyOf(good)}) {
		t.Fatalf("applied hops %v", keysOf(o))
	}
}

func scRenderUnsupported(t *testing.T, h *H) {
	hops := unsupportedHops(h)
	if len(hops) == 0 {
		t.Skip("the driver supports every feature the suite can ask for")
	}
	names := make([]string, 0, len(hops))
	for n := range hops {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		t.Run(n, func(t *testing.T) { wantRejected(t, h.Sub(t), hops[n], driver.ErrUnsupported) })
	}
}

func scRenderUnknownEnum(t *testing.T, h *H) {
	mk := map[string]func(*forwardv1.NodeHop){
		"balance":  func(x *forwardv1.NodeHop) { x.Balance = 99 },
		"protocol": func(x *forwardv1.NodeHop) { x.Listen.Protocol = 99 },
		"ingress":  func(x *forwardv1.NodeHop) { x.Ingress.Security = 99 },
		"egress":   func(x *forwardv1.NodeHop) { x.Upstreams[0].Egress.Security = 99 },
	}
	for _, n := range []string{"balance", "protocol", "ingress", "egress"} {
		t.Run(n, func(t *testing.T) {
			bad := h.B.Simple(RouteB, 1)
			mk[n](bad)
			a, err := h.D.Render(h.State(1, h.B.Simple(RouteA, 0), bad))
			if !errors.Is(err, driver.ErrUnsupported) {
				t.Fatalf("Render: error %v, want ErrUnsupported", err)
			}
			if slices.Contains(a.Hops, keyOf(bad)) {
				t.Fatal("rejected hop is in the artifact")
			}
		})
	}
}

func scRenderInvalid(t *testing.T, h *H) {
	a, err := h.D.Render(nil)
	h.WantErr("Render(nil)", err, driver.ErrInvalidState)
	if a.Digest != "" || len(a.Content) != 0 {
		t.Fatal("Render(nil) answered an artifact")
	}
	t.Run("duplicate-hop", func(t *testing.T) {
		dup := h.B.Simple(RouteB, 1)
		a, err := h.D.Render(h.State(1, h.B.Simple(RouteA, 0), dup, proto.Clone(dup).(*forwardv1.NodeHop)))
		if !errors.Is(err, driver.ErrInvalidState) {
			t.Fatalf("error %v, want ErrInvalidState", err)
		}
		if !slices.Equal(a.Hops, []driver.HopKey{{RouteID: RouteA}}) {
			t.Fatalf("artifact runs %v", a.Hops)
		}
	})
	t.Run("listen-port-zero", func(t *testing.T) {
		bad := h.B.Simple(RouteB, 1)
		bad.Listen.Port = 0
		wantRejected(t, h.Sub(t), bad, driver.ErrInvalidState)
	})
}

func scApplyCases(t *testing.T, h *H) {
	forCases(t, h, func(t *testing.T, h *H, hops []*forwardv1.NodeHop) {
		s := h.State(1, hops...)
		a, r := h.RenderApply(s)
		if !r.Changed {
			t.Fatal("first apply reported no change")
		}
		o := h.Observe()
		if !o.Applied || o.Generation != 1 || o.Digest != a.Digest || o.StateHash != s.GetStateHash() || o.NodeRef != s.GetNodeRef() {
			t.Fatalf("observation %+v does not match the applied artifact", o)
		}
		if !slices.Equal(keysOf(o), a.Hops) {
			t.Fatalf("observed hops %v, applied %v", keysOf(o), a.Hops)
		}
		for _, hop := range hops {
			if got, want := rotation(o, keyOf(hop)), renderedRotation(hop); !slices.Equal(got, want) {
				t.Fatalf("rotation of %s = %v, want every rendered upstream %v", keyOf(hop), got, want)
			}
		}
		if len(h.Owned()) == 0 {
			t.Fatal("applied state lists no owned object")
		}
		if r := h.Apply(a); r.Changed {
			t.Fatal("re-apply reported a change")
		}
		ctx, cancel := h.Ctx()
		defer cancel()
		if err := h.D.Remove(ctx); err != nil {
			t.Fatal(err)
		}
		if got := h.Owned(); len(got) != 0 {
			t.Fatalf("Remove left %q", got)
		}
	})
}

func scApplyIdempotent(t *testing.T, h *H) {
	s := h.State(1, h.B.Simple(RouteA, 0), h.B.Simple(RouteB, 1))
	a, _ := h.RenderApply(s)
	before, applies := h.Owned(), h.Applies()
	for range 3 {
		if r := h.Apply(h.Render(s)); r.Changed {
			t.Fatal("applying the same artifact again reported a change")
		}
	}
	if after := h.Owned(); !slices.Equal(before, after) {
		t.Fatalf("no-op apply changed owned objects%s", diff(before, after))
	}
	if applies >= 0 && h.Applies() != applies {
		t.Fatal("no-op apply ran a full apply")
	}
	if o := h.Observe(); o.Digest != a.Digest {
		t.Fatal("digest moved")
	}
}

func scApplyNewerSame(t *testing.T, h *H) {
	hop := h.B.Simple(RouteA, 0)
	h.RenderApply(h.State(1, hop))
	e1 := epochs(h.Observe())
	other := h.B.Simple(RouteB, 1)
	other.Engine = otherEngine(h.D.Engine())
	s2 := h.State(2, hop, other) // another engine's change bumped the generation
	a2 := h.Render(s2)
	if r := h.Apply(a2); r.Changed {
		t.Fatal("newer generation with the same hops reported a change")
	}
	o := h.Observe()
	if o.Generation != 2 || o.StateHash != s2.GetStateHash() {
		t.Fatalf("observed generation %d hash %s, want 2 %s", o.Generation, o.StateHash, s2.GetStateHash())
	}
	if e2 := epochs(o); !mapsEqual(e1, e2) {
		t.Fatalf("epochs moved: %v -> %v", e1, e2)
	}
}

func mapsEqual(a, b map[driver.HopKey]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func scApplyStale(t *testing.T, h *H) {
	a5, _ := h.RenderApply(h.State(5, h.B.Simple(RouteA, 0)))
	before := h.Owned()
	ctx, cancel := h.Ctx()
	defer cancel()
	for _, hops := range [][]*forwardv1.NodeHop{{h.B.Simple(RouteB, 1)}, {h.B.Simple(RouteA, 0)}} {
		_, err := h.D.Apply(ctx, h.Render(h.State(4, hops...)))
		h.WantErr("Apply generation 4 over 5", err, driver.ErrStaleGeneration)
	}
	if after := h.Owned(); !slices.Equal(before, after) {
		t.Fatalf("stale apply changed the host%s", diff(before, after))
	}
	if o := h.Observe(); o.Generation != 5 || o.Digest != a5.Digest {
		t.Fatalf("observed %d %s", o.Generation, o.Digest)
	}
}

func scApplyGenConflict(t *testing.T, h *H) {
	a, _ := h.RenderApply(h.State(3, h.B.Simple(RouteA, 0)))
	before := h.Owned()
	ctx, cancel := h.Ctx()
	defer cancel()
	_, err := h.D.Apply(ctx, h.Render(h.State(3, h.B.Simple(RouteB, 1))))
	h.WantErr("Apply generation 3 with other content", err, driver.ErrGenerationConflict)
	if after := h.Owned(); !slices.Equal(before, after) {
		t.Fatalf("conflicting apply changed the host%s", diff(before, after))
	}
	if o := h.Observe(); o.Digest != a.Digest {
		t.Fatal("digest moved")
	}
}

func scApplyInvalidArtifact(t *testing.T, h *H) {
	h.RenderApply(h.State(1, h.B.Simple(RouteA, 0)))
	before := h.Owned()
	a := h.Render(h.State(2, h.B.Simple(RouteB, 1)))
	ctx, cancel := h.Ctx()
	defer cancel()

	tampered := a
	tampered.Content = append(slices.Clone(a.Content), 0)
	_, err := h.D.Apply(ctx, tampered)
	h.WantErr("Apply tampered artifact", err, driver.ErrInvalidArtifact)

	foreign := a
	foreign.Engine = otherEngine(h.D.Engine())
	_, err = h.D.Apply(ctx, foreign)
	h.WantErr("Apply other engine's artifact", err, driver.ErrEngineMismatch)

	if after := h.Owned(); !slices.Equal(before, after) {
		t.Fatalf("refused artifact changed the host%s", diff(before, after))
	}
}

func scApplyRestart(t *testing.T, h *H) {
	s := h.State(5, h.B.Simple(RouteA, 0), h.B.Simple(RouteB, 1))
	a, _ := h.RenderApply(s)
	h.Restart()
	o := h.Observe()
	if !o.Applied || o.Generation != 5 || o.Digest != a.Digest || o.StateHash != s.GetStateHash() || o.NodeRef != s.GetNodeRef() {
		t.Fatalf("restarted driver observes %+v", o)
	}
	if r := h.Apply(h.Render(s)); r.Changed {
		t.Fatal("restarted driver re-applied an unchanged host")
	}
	ctx, cancel := h.Ctx()
	defer cancel()
	_, err := h.D.Apply(ctx, h.Render(h.State(4, h.B.Simple(RouteA, 0))))
	h.WantErr("restarted driver applies an older generation", err, driver.ErrStaleGeneration)
}

func scApplyRepair(t *testing.T, h *H) {
	dm, ok := h.Env.(Damager)
	if !ok {
		t.Skip("Env is not a Damager")
	}
	s := h.State(1, h.B.Simple(RouteA, 0), h.B.Simple(RouteB, 1))
	a, _ := h.RenderApply(s)
	clean := h.Owned()
	dm.Damage(t)
	if damaged := h.Owned(); slices.Equal(clean, damaged) {
		t.Fatal("Damage changed nothing Owned lists")
	}
	h.Restart()
	if r := h.Apply(a); !r.Changed {
		t.Fatal("apply over damaged state reported no change")
	}
	if repaired := h.Owned(); !slices.Equal(clean, repaired) {
		t.Fatalf("repair differs from a clean apply%s", diff(clean, repaired))
	}
	if o := h.Observe(); !slices.Equal(keysOf(o), a.Hops) || o.Digest != a.Digest {
		t.Fatalf("observed after repair %+v", o)
	}
}

func scApplyFailure(t *testing.T, h *H) {
	f, ok := h.Env.(ApplyFaulter)
	if !ok {
		t.Skip("Env is not an ApplyFaulter")
	}
	a1, _ := h.RenderApply(h.State(1, h.B.Simple(RouteA, 0)))
	before := h.Owned()
	f.FailNextApply(t)
	ctx, cancel := h.Ctx()
	defer cancel()
	a2 := h.Render(h.State(2, h.B.Simple(RouteA, 0), h.B.Simple(RouteB, 1)))
	if _, err := h.D.Apply(ctx, a2); err == nil {
		t.Fatal("Apply succeeded despite the injected failure")
	}
	if after := h.Owned(); !slices.Equal(before, after) {
		t.Fatalf("failed apply changed the host%s", diff(before, after))
	}
	if o := h.Observe(); o.Generation != 1 || o.Digest != a1.Digest {
		t.Fatalf("observed %d %s after failed apply", o.Generation, o.Digest)
	}
	h.Apply(a2) // the retry succeeds
}

func scApplyConflict(t *testing.T, h *H) {
	cp, ok := h.Env.(ConflictPlanter)
	if !ok {
		t.Skip("Env is not a ConflictPlanter")
	}
	h.RenderApply(h.State(1, h.B.Simple(RouteA, 0)))
	before := h.Owned()
	cp.PlantConflict(t, h.B.Top.ListenPorts[1])
	h.Refreeze()
	ctx, cancel := h.Ctx()
	defer cancel()
	_, err := h.D.Apply(ctx, h.Render(h.State(2, h.B.Simple(RouteA, 0), h.B.Simple(RouteB, 1))))
	h.WantErr("Apply onto a foreign listener", err, driver.ErrConflict)
	if after := h.Owned(); !slices.Equal(before, after) {
		t.Fatalf("conflicting apply changed the host%s", diff(before, after))
	}
}

func scApplyNotOwned(t *testing.T, h *H) {
	ip, ok := h.Env.(ImpostorPlanter)
	if !ok {
		t.Skip("Env is not an ImpostorPlanter")
	}
	ip.PlantImpostor(t)
	h.Refreeze()
	ctx, cancel := h.Ctx()
	defer cancel()
	_, err := h.D.Apply(ctx, h.Render(h.State(1, h.B.Simple(RouteA, 0))))
	h.WantErr("Apply over an impostor", err, driver.ErrNotOwned)
	if err := h.D.Remove(ctx); err != nil {
		t.Fatalf("Remove with an impostor present: %v", err)
	}
	if got := h.Owned(); len(got) != 0 {
		t.Fatalf("owned objects %q", got)
	}
}

func scApplyEmpty(t *testing.T, h *H) {
	h.RenderApply(h.State(1, h.B.Simple(RouteA, 0), h.B.Simple(RouteB, 1)))
	if r := h.Apply(h.Render(h.State(2))); !r.Changed {
		t.Fatal("empty artifact over owned state reported no change")
	}
	if got := h.Owned(); len(got) != 0 {
		t.Fatalf("empty artifact left %q", got)
	}
	if o := h.Observe(); o.Applied {
		t.Fatal("observed as applied")
	}
}

func scCountersKept(t *testing.T, h *H) {
	a, b := h.B.Simple(RouteA, 0), h.B.Simple(RouteB, 1)
	h.RenderApply(h.State(1, a, b))
	h.Traffic(keyOf(a))
	h.Traffic(keyOf(b))
	o1 := h.Observe()
	a2 := proto.Clone(a).(*forwardv1.NodeHop)
	a2.Upstreams = a2.Upstreams[:2] // route A's upstreams change, B is untouched
	if _, r := h.RenderApply(h.State(2, a2, b)); !r.Changed {
		t.Fatal("changed state reported no change")
	}
	o2 := h.Observe() // CheckObservation proves values did not go down
	if e1, e2 := epochs(o1), epochs(o2); !mapsEqual(e1, e2) {
		t.Fatalf("a re-apply re-created counters: epochs %v -> %v", e1, e2)
	}
}

func scUnrelatedHops(t *testing.T, h *H) {
	a, b := h.B.Simple(RouteA, 0), h.B.Simple(RouteB, 1)
	ka := keyOf(a)
	h.RenderApply(h.State(1, a, b))
	h.Traffic(ka)
	h.Traffic(keyOf(b))
	epoch := epochs(h.Observe())[ka]
	moved := proto.Clone(b).(*forwardv1.NodeHop) // route B moves to another port
	moved.Listen.Port = h.B.Top.ListenPorts[3]
	for i, step := range []struct {
		what string
		hops []*forwardv1.NodeHop
	}{
		{"route C added", []*forwardv1.NodeHop{a, b, h.B.Simple(RouteC, 2)}},
		{"route B moved to another port", []*forwardv1.NodeHop{a, moved, h.B.Simple(RouteC, 2)}},
		{"route C removed", []*forwardv1.NodeHop{a, moved}},
		{"route B removed", []*forwardv1.NodeHop{a}},
	} {
		if _, r := h.RenderApply(h.State(uint64(i)+2, step.hops...)); !r.Changed { // #nosec G115 -- a few steps
			t.Fatalf("%s: the apply reported no change", step.what)
		}
		o := h.Observe() // CheckObservation proves values did not go down
		if got := epochs(o)[ka]; got != epoch {
			t.Fatalf("%s: route A's counter epoch changed from %s to %s", step.what, epoch, got)
		}
		h.Traffic(ka)
	}
}

func scCountersRecreated(t *testing.T, h *H) {
	a, b := h.B.Simple(RouteA, 0), h.B.Simple(RouteB, 1)
	h.RenderApply(h.State(1, a, b))
	h.Traffic(keyOf(b))
	h.Observe()
	h.RenderApply(h.State(2, a))
	h.Observe()
	h.RenderApply(h.State(3, a, b))
	h.Observe() // same epoch as before only with counters not lower
	ctx, cancel := h.Ctx()
	defer cancel()
	if err := h.D.Remove(ctx); err != nil {
		t.Fatal(err)
	}
	h.RenderApply(h.State(4, a, b))
	h.Observe()
}

func scObserveMonotonic(t *testing.T, h *H) {
	a := h.B.Simple(RouteA, 0)
	h.RenderApply(h.State(1, a))
	for i := range 5 {
		h.Traffic(keyOf(a))
		h.Observe()
		if i == 2 {
			h.Restart()
		}
	}
	first := h.Observe()
	traffic := h.Traffic(keyOf(a))
	last := h.Observe()
	if traffic && epochs(first)[keyOf(a)] == epochs(last)[keyOf(a)] {
		c0, c1 := first.Counters[0], last.Counters[0]
		if c1.GetUpBytes()+c1.GetDownBytes() <= c0.GetUpBytes()+c0.GetDownBytes() {
			t.Fatal("traffic did not grow the counters")
		}
	}
}

// failoverHop answers a hop with three upstreams.
func failoverHop(h *H) *forwardv1.NodeHop { return h.B.Simple(RouteA, 0) }

func scSetUpstreamsFailover(t *testing.T, h *H) {
	hop := failoverHop(h)
	k := keyOf(hop)
	h.RenderApply(h.State(1, hop))
	o1, owned1, applies := h.Observe(), h.Owned(), h.Applies()
	ctx, cancel := h.Ctx()
	defer cancel()
	second := hop.GetUpstreams()[1]
	if err := h.D.SetUpstreams(ctx, k.RouteID, k.HopIndex, []driver.Upstream{{Address: second.GetAddress(), Port: second.GetPort()}}); err != nil {
		t.Fatal(err)
	}
	o2 := h.Observe()
	if got, want := rotation(o2, k), fmtUpstreams([]driver.Upstream{{Address: second.GetAddress(), Port: second.GetPort(), Weight: second.GetWeight()}}); !slices.Equal(got, want) {
		t.Fatalf("rotation %v, want %v", got, want)
	}
	if o2.Digest != o1.Digest || o2.Generation != o1.Generation || !mapsEqual(epochs(o1), epochs(o2)) {
		t.Fatal("SetUpstreams changed digest, generation or counter epochs")
	}
	if applies >= 0 && h.Applies() != applies {
		t.Fatal("SetUpstreams ran a full apply")
	}
	// recovery: everything back
	all := make([]driver.Upstream, 0, 3)
	for _, u := range hop.GetUpstreams() {
		all = append(all, driver.Upstream{Address: u.GetAddress(), Port: u.GetPort()})
	}
	if err := h.D.SetUpstreams(ctx, k.RouteID, k.HopIndex, all); err != nil {
		t.Fatal(err)
	}
	if got, want := rotation(h.Observe(), k), renderedRotation(hop); !slices.Equal(got, want) {
		t.Fatalf("after recovery rotation %v, want %v", got, want)
	}
	if owned3 := h.Owned(); !slices.Equal(owned1, owned3) {
		t.Fatalf("recovery does not restore the applied objects%s", diff(owned1, owned3))
	}
}

func scSetUpstreamsWeights(t *testing.T, h *H) {
	hop := failoverHop(h)
	k := keyOf(hop)
	h.RenderApply(h.State(1, hop))
	u0, u1 := hop.GetUpstreams()[0], hop.GetUpstreams()[1]
	ctx, cancel := h.Ctx()
	defer cancel()
	if err := h.D.SetUpstreams(ctx, k.RouteID, k.HopIndex, []driver.Upstream{
		{Address: u0.GetAddress(), Port: u0.GetPort(), Weight: 7},
		{Address: u1.GetAddress(), Port: u1.GetPort()},
	}); err != nil {
		t.Fatal(err)
	}
	want := fmtUpstreams([]driver.Upstream{
		{Address: u0.GetAddress(), Port: u0.GetPort(), Weight: 7},
		{Address: u1.GetAddress(), Port: u1.GetPort(), Weight: u1.GetWeight()},
	})
	if got := rotation(h.Observe(), k); !slices.Equal(got, want) {
		t.Fatalf("rotation %v, want %v", got, want)
	}
}

func scSetUpstreamsErrors(t *testing.T, h *H) {
	hop := failoverHop(h)
	k := keyOf(hop)
	u := hop.GetUpstreams()[0]
	one := []driver.Upstream{{Address: u.GetAddress(), Port: u.GetPort()}}
	ctx, cancel := h.Ctx()
	defer cancel()
	h.WantErr("SetUpstreams before any apply", h.D.SetUpstreams(ctx, k.RouteID, k.HopIndex, one), driver.ErrNotFound)
	h.RenderApply(h.State(1, hop))
	before := rotation(h.Observe(), k)
	h.WantErr("unknown route", h.D.SetUpstreams(ctx, RouteC, 0, one), driver.ErrNotFound)
	h.WantErr("unknown hop index", h.D.SetUpstreams(ctx, k.RouteID, 3, one), driver.ErrNotFound)
	h.WantErr("unknown upstream", h.D.SetUpstreams(ctx, k.RouteID, k.HopIndex, []driver.Upstream{{Address: "192.0.2.99", Port: u.GetPort()}}), driver.ErrNotFound)
	h.WantErr("unknown port", h.D.SetUpstreams(ctx, k.RouteID, k.HopIndex, []driver.Upstream{{Address: u.GetAddress(), Port: u.GetPort() + 1}}), driver.ErrNotFound)
	h.WantErr("empty selection", h.D.SetUpstreams(ctx, k.RouteID, k.HopIndex, nil), driver.ErrInvalidArgument)
	h.WantErr("duplicate upstream", h.D.SetUpstreams(ctx, k.RouteID, k.HopIndex, append(one, one...)), driver.ErrInvalidArgument)
	if after := rotation(h.Observe(), k); !slices.Equal(before, after) {
		t.Fatalf("refused SetUpstreams changed rotation %v -> %v", before, after)
	}
}

func scSetUpstreamsReset(t *testing.T, h *H) {
	hop := failoverHop(h)
	k := keyOf(hop)
	s1 := h.State(1, hop)
	a1, _ := h.RenderApply(s1)
	u := hop.GetUpstreams()[2]
	sel := []driver.Upstream{{Address: u.GetAddress(), Port: u.GetPort()}}
	ctx, cancel := h.Ctx()
	defer cancel()
	if err := h.D.SetUpstreams(ctx, k.RouteID, k.HopIndex, sel); err != nil {
		t.Fatal(err)
	}
	want := fmtUpstreams([]driver.Upstream{{Address: u.GetAddress(), Port: u.GetPort(), Weight: u.GetWeight()}})
	if r := h.Apply(a1); r.Changed {
		t.Fatal("no-op apply reported a change")
	}
	if got := rotation(h.Observe(), k); !slices.Equal(got, want) {
		t.Fatalf("no-op apply changed rotation to %v", got)
	}
	if _, r := h.RenderApply(h.State(2, hop, h.B.Simple(RouteB, 1))); !r.Changed {
		t.Fatal("changing apply reported no change")
	}
	if got, want := rotation(h.Observe(), k), renderedRotation(hop); !slices.Equal(got, want) {
		t.Fatalf("changing apply left rotation %v, want %v", got, want)
	}
}

func scSetUpstreamsRestart(t *testing.T, h *H) {
	hop := failoverHop(h)
	k := keyOf(hop)
	h.RenderApply(h.State(1, hop))
	u := hop.GetUpstreams()[1]
	ctx, cancel := h.Ctx()
	defer cancel()
	if err := h.D.SetUpstreams(ctx, k.RouteID, k.HopIndex, []driver.Upstream{{Address: u.GetAddress(), Port: u.GetPort()}}); err != nil {
		t.Fatal(err)
	}
	want := rotation(h.Observe(), k)
	h.Restart()
	if got := rotation(h.Observe(), k); !slices.Equal(got, want) {
		t.Fatalf("after restart rotation %v, want %v", got, want)
	}
}

func scRemove(t *testing.T, h *H) {
	ctx, cancel := h.Ctx()
	defer cancel()
	if err := h.D.Remove(ctx); err != nil {
		t.Fatalf("Remove with nothing owned: %v", err)
	}
	s := h.State(1, h.B.Simple(RouteA, 0), h.B.Simple(RouteB, 1), h.B.Simple(RouteC, 2))
	a, _ := h.RenderApply(s)
	h.Traffic(driver.HopKey{RouteID: RouteA})
	h.Observe()
	if err := h.D.Remove(ctx); err != nil {
		t.Fatal(err)
	}
	if got := h.Owned(); len(got) != 0 {
		t.Fatalf("Remove left %q", got)
	}
	if o := h.Observe(); o.Applied {
		t.Fatal("observed as applied after Remove")
	}
	h.Restart()
	if err := h.D.Remove(ctx); err != nil {
		t.Fatalf("second Remove: %v", err)
	}
	h.WantErr("SetUpstreams after Remove", h.D.SetUpstreams(ctx, RouteA, 0, []driver.Upstream{{Address: h.B.Top.UpstreamsV4[0], Port: h.B.Top.UpstreamPort}}), driver.ErrNotFound)
	if r := h.Apply(a); !r.Changed {
		t.Fatal("apply after Remove reported no change")
	}
	h.Observe()
}

func scContext(t *testing.T, h *H) {
	hop := failoverHop(h)
	k := keyOf(hop)
	a1, _ := h.RenderApply(h.State(1, hop))
	o1, before := h.Observe(), h.Owned()
	a2 := h.Render(h.State(2, hop, h.B.Simple(RouteB, 1)))
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancel2 := context.WithTimeout(context.Background(), -1)
	defer cancel2()
	for _, c := range []struct {
		ctx  context.Context
		want error
	}{{cancelled, context.Canceled}, {expired, context.DeadlineExceeded}} {
		if _, err := h.D.Capabilities(c.ctx); !errors.Is(err, c.want) {
			t.Fatalf("Capabilities: %v, want %v", err, c.want)
		}
		if _, err := h.D.Apply(c.ctx, a2); !errors.Is(err, c.want) {
			t.Fatalf("Apply: %v, want %v", err, c.want)
		}
		if _, err := h.D.Observe(c.ctx); !errors.Is(err, c.want) {
			t.Fatalf("Observe: %v, want %v", err, c.want)
		}
		u := hop.GetUpstreams()[1]
		if err := h.D.SetUpstreams(c.ctx, k.RouteID, k.HopIndex, []driver.Upstream{{Address: u.GetAddress(), Port: u.GetPort()}}); !errors.Is(err, c.want) {
			t.Fatalf("SetUpstreams: %v, want %v", err, c.want)
		}
		if err := h.D.Remove(c.ctx); !errors.Is(err, c.want) {
			t.Fatalf("Remove: %v, want %v", err, c.want)
		}
	}
	if after := h.Owned(); !slices.Equal(before, after) {
		t.Fatalf("calls with a done context changed the host%s", diff(before, after))
	}
	o2 := h.Observe()
	if o2.Digest != a1.Digest || !slices.Equal(rotation(o1, k), rotation(o2, k)) {
		t.Fatal("calls with a done context changed the observed state")
	}
}

func scConcurrent(t *testing.T, h *H) {
	const gens = 24
	hop := failoverHop(h)
	k := keyOf(hop)
	// Odd generations add route B on one port, even ones on another, so
	// every apply changes the host and the digest tells the parity.
	states := make([]*forwardv1.NodeForwardState, gens+1)
	arts := make([]driver.Artifact, gens+1)
	for g := 1; g <= gens; g++ {
		states[g] = h.State(uint64(g), hop, h.B.Simple(RouteB, 1+g%2)) // #nosec G115 -- g > 0
		arts[g] = h.Render(states[g])
	}
	digestParity := map[string]int{arts[1].Digest: 1, arts[2].Digest: 0}
	if len(digestParity) != 2 {
		t.Fatal("alternating states render the same digest")
	}
	h.Apply(arts[1])

	ctx, cancel := h.Ctx()
	defer cancel()
	done := make(chan struct{})
	var wg sync.WaitGroup
	loop := func(f func() bool) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if !f() {
					return
				}
			}
		}()
	}
	for range 2 {
		var last uint64
		loop(func() bool {
			o, err := h.D.Observe(ctx)
			if err != nil {
				t.Errorf("Observe: %v", err)
				return false
			}
			if err := h.CheckObservation(o); err != nil {
				t.Error(err)
				return false
			}
			if !o.Applied || o.Generation < last {
				t.Errorf("observed generation %d after %d (applied %v)", o.Generation, last, o.Applied)
				return false
			}
			last = o.Generation
			if p, ok := digestParity[o.Digest]; !ok || p != int(o.Generation%2) { // #nosec G115 -- 0 or 1
				t.Errorf("observed generation %d with digest of the other parity: torn observation", o.Generation)
				return false
			}
			return true
		})
	}
	ups := hop.GetUpstreams()
	i := 0
	loop(func() bool {
		u := ups[i%len(ups)]
		i++
		if err := h.D.SetUpstreams(ctx, k.RouteID, k.HopIndex, []driver.Upstream{{Address: u.GetAddress(), Port: u.GetPort()}}); err != nil {
			t.Errorf("SetUpstreams: %v", err)
			return false
		}
		return true
	})
	loop(func() bool {
		a, err := h.D.Render(states[2])
		if err != nil || a.Digest != arts[2].Digest {
			t.Errorf("concurrent Render: %v / digest %s", err, a.Digest)
			return false
		}
		if _, err := h.D.Capabilities(ctx); err != nil {
			t.Errorf("Capabilities: %v", err)
			return false
		}
		return true
	})
	for g := 2; g <= gens; g++ {
		r, err := h.D.Apply(ctx, arts[g])
		if err != nil {
			t.Errorf("Apply generation %d: %v", g, err)
			break
		}
		if !r.Changed {
			t.Errorf("Apply generation %d reported no change", g)
		}
	}
	close(done)
	wg.Wait()
	if o := h.Observe(); o.Generation != gens || o.Digest != arts[gens].Digest {
		t.Fatalf("final observation %d %s", o.Generation, o.Digest)
	}
}
