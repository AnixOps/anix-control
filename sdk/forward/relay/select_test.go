package relay

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
)

// A fake clock for the retry interval.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)} }
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// selectorEnv is a Selector whose dials are fakes the test controls.
type selectorEnv struct {
	t     *testing.T
	clock *fakeClock
	sel   *Selector

	mu        sync.Mutex
	quicCalls int
	tlsCalls  int
	quicErr   error // what a QUIC dial returns, nil for success
	tlsErr    error
	quicHold  chan struct{} // when set, a QUIC dial waits for it (or its context)
	quicCtxs  []context.Context

	quicCarrier, tlsCarrier, plainCarrier Carrier
}

var errQUICDown = errors.New("quic: no route to host")

func newSelectorEnv(t *testing.T, choice CarrierChoice, mod ...func(*SelectorConfig)) *selectorEnv {
	t.Helper()
	ca := newTestPKI(t)
	cfg := SelectorConfig{
		Choice:  choice,
		Address: "198.51.100.1:7000",
		Link:    link.DialConfig{Credentials: linkCreds(t, ca, ca.CAPEM(), "forward-1"), ServerName: "forward-2", PeerIdentity: nodeID("forward-2")},
		Plain:   link.PlainDialConfig{TrustedLink: true},
	}
	e := &selectorEnv{t: t, clock: newFakeClock(), quicCarrier: &ConnCarrier{}, tlsCarrier: &ConnCarrier{}, plainCarrier: &ConnCarrier{}}
	cfg.Now = e.clock.Now
	for _, m := range mod {
		m(&cfg)
	}
	sel, err := NewSelector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.sel = sel
	sel.dialQUIC = func(ctx context.Context, _ SelectorConfig) (Carrier, error) {
		e.mu.Lock()
		e.quicCalls++
		e.quicCtxs = append(e.quicCtxs, ctx)
		hold := e.quicHold
		e.mu.Unlock()
		if hold != nil {
			select {
			case <-hold:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		e.mu.Lock()
		err := e.quicErr
		e.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return e.quicCarrier, nil
	}
	sel.dialTLS = func(ctx context.Context, _ SelectorConfig) (Carrier, error) {
		e.mu.Lock()
		e.tlsCalls++
		err := e.tlsErr
		e.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return e.tlsCarrier, nil
	}
	sel.dialPlain = func(ctx context.Context, _ SelectorConfig) (Carrier, error) { return e.plainCarrier, nil }
	return e
}

func (e *selectorEnv) setQUIC(err error) {
	e.mu.Lock()
	e.quicErr = err
	e.mu.Unlock()
}

func (e *selectorEnv) calls() (quic, tls int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.quicCalls, e.tlsCalls
}

func (e *selectorEnv) dial() Carrier {
	e.t.Helper()
	c, err := e.sel.Dial(ctxTimeout(e.t))
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

func TestSelectorAutoPrefersQUIC(t *testing.T) {
	e := newSelectorEnv(t, ChoiceAuto)
	for range 3 {
		if c := e.dial(); c != e.quicCarrier {
			t.Fatal("Auto did not return the QUIC carrier")
		}
	}
	if q, tl := e.calls(); q != 3 || tl != 0 {
		t.Fatalf("calls: quic %d tls %d", q, tl)
	}
	if st := e.sel.Stats(); st.QUICAttempts != 3 || st.QUICFailures != 0 || st.OnFallback || st.Fallbacks != 0 {
		t.Fatalf("stats %+v", st)
	}
}

func TestSelectorAutoFallsBackOnTheDialThatFailed(t *testing.T) {
	e := newSelectorEnv(t, ChoiceAuto)
	e.setQUIC(errQUICDown)
	// the dial that found QUIC down is made up for with TLS at once
	if c := e.dial(); c != e.tlsCarrier {
		t.Fatal("no fallback to TLS_TCP")
	}
	if q, tl := e.calls(); q != 1 || tl != 1 {
		t.Fatalf("calls: quic %d tls %d", q, tl)
	}
	st := e.sel.Stats()
	if st.QUICFailures != 1 || st.Fallbacks != 1 || st.ConsecutiveFailures != 1 || st.OnFallback {
		t.Fatalf("stats %+v", st)
	}
	// QUIC works again: the next dial uses it, and the count starts over
	e.setQUIC(nil)
	if c := e.dial(); c != e.quicCarrier {
		t.Fatal("Auto did not go back to QUIC")
	}
	if st := e.sel.Stats(); st.ConsecutiveFailures != 0 {
		t.Fatalf("stats %+v", st)
	}
}

// Three QUIC dials in a row that fail put Auto on the fallback: QUIC is not
// tried again until the retry interval has passed, and then by one dial.
func TestSelectorAutoStopsTryingQUICAndRetriesLater(t *testing.T) {
	e := newSelectorEnv(t, ChoiceAuto)
	e.setQUIC(errQUICDown)
	for range 3 {
		if c := e.dial(); c != e.tlsCarrier {
			t.Fatal("no TLS")
		}
	}
	st := e.sel.Stats()
	if !st.OnFallback || st.ConsecutiveFailures != 3 || !st.RetryAt.Equal(e.clock.Now().Add(DefaultRetryInterval)) {
		t.Fatalf("stats %+v", st)
	}
	q0, _ := e.calls()

	// on the fallback: straight to TLS_TCP, no probe, until the interval is up
	for range 5 {
		if c := e.dial(); c != e.tlsCarrier {
			t.Fatal("no TLS")
		}
	}
	e.clock.Advance(DefaultRetryInterval - time.Second)
	e.dial()
	if q, _ := e.calls(); q != q0 {
		t.Fatalf("QUIC was tried %d times on the fallback", q-q0)
	}
	if st := e.sel.Stats(); st.FallbackDials != 6 {
		t.Fatalf("stats %+v", st)
	}

	// the interval is up: one dial probes, and a failed probe restarts the wait
	e.clock.Advance(time.Second)
	if c := e.dial(); c != e.tlsCarrier {
		t.Fatal("no TLS after a failed probe")
	}
	if q, _ := e.calls(); q != q0+1 {
		t.Fatalf("%d QUIC probes, want 1", q-q0)
	}
	if st := e.sel.Stats(); !st.OnFallback || !st.RetryAt.Equal(e.clock.Now().Add(DefaultRetryInterval)) {
		t.Fatalf("stats %+v", st)
	}
	e.dial()
	if q, _ := e.calls(); q != q0+1 {
		t.Fatal("QUIC was probed again at once")
	}

	// the next probe succeeds: back on QUIC for good
	e.setQUIC(nil)
	e.clock.Advance(DefaultRetryInterval)
	if c := e.dial(); c != e.quicCarrier {
		t.Fatal("a successful probe did not return the QUIC carrier")
	}
	if st := e.sel.Stats(); st.OnFallback || st.ConsecutiveFailures != 0 || !st.RetryAt.IsZero() {
		t.Fatalf("stats %+v", st)
	}
	if c := e.dial(); c != e.quicCarrier {
		t.Fatal("Auto did not stay on QUIC")
	}
}

// Failures that are not in a row do not add up.
func TestSelectorAutoCountsFailuresInARow(t *testing.T) {
	e := newSelectorEnv(t, ChoiceAuto)
	pattern := []error{errQUICDown, errQUICDown, nil, errQUICDown, errQUICDown, nil}
	for _, err := range pattern {
		e.setQUIC(err)
		e.dial()
	}
	if st := e.sel.Stats(); st.OnFallback || st.ConsecutiveFailures != 0 || st.QUICFailures != 4 {
		t.Fatalf("stats %+v", st)
	}
}

// While one dial probes QUIC after the retry interval, the others keep using
// TLS_TCP.
func TestSelectorOnlyOneDialProbes(t *testing.T) {
	e := newSelectorEnv(t, ChoiceAuto, func(c *SelectorConfig) { c.FailLimit = 1 })
	e.setQUIC(errQUICDown)
	e.dial() // on the fallback
	e.clock.Advance(DefaultRetryInterval)
	e.mu.Lock()
	e.quicHold = make(chan struct{})
	e.mu.Unlock()
	probe := make(chan Carrier, 1)
	go func() { c, _ := e.sel.Dial(ctxTimeout(t)); probe <- c }()
	waitFor(t, "the probe to start", func() bool { q, _ := e.calls(); return q == 2 })
	if c := e.dial(); c != e.tlsCarrier {
		t.Fatal("a second dial did not use TLS while the probe ran")
	}
	if q, _ := e.calls(); q != 2 {
		t.Fatalf("a second QUIC dial started: %d", q)
	}
	e.setQUIC(nil)
	e.mu.Lock()
	close(e.quicHold)
	e.mu.Unlock()
	if c := <-probe; c != e.quicCarrier {
		t.Fatal("the probe did not return the QUIC carrier")
	}
	if st := e.sel.Stats(); st.OnFallback {
		t.Fatalf("stats %+v", st)
	}
}

// A QUIC attempt of Auto is bounded by the probe timeout, and a caller that
// gives up first is not evidence about QUIC.
func TestSelectorProbeIsBounded(t *testing.T) {
	e := newSelectorEnv(t, ChoiceAuto, func(c *SelectorConfig) { c.ProbeTimeout = 40 * time.Millisecond })
	e.mu.Lock()
	e.quicHold = make(chan struct{}) // never released: QUIC "does not answer"
	e.mu.Unlock()
	start := time.Now()
	if c := e.dial(); c != e.tlsCarrier {
		t.Fatal("no TLS after the probe timed out")
	}
	e.mu.Lock()
	dl, ok := e.quicCtxs[0].Deadline()
	e.mu.Unlock()
	if !ok || dl.Sub(start) > time.Second || dl.Sub(start) < 40*time.Millisecond {
		t.Fatalf("the QUIC attempt's deadline was %v after the dial began, want the 40 ms probe timeout", dl.Sub(start))
	}
	if st := e.sel.Stats(); st.QUICFailures != 1 || st.Fallbacks != 1 {
		t.Fatalf("stats %+v", st)
	}

	// the caller's own cancellation ends the dial, with no fallback and no
	// count against QUIC
	e2 := newSelectorEnv(t, ChoiceAuto)
	e2.mu.Lock()
	e2.quicHold = make(chan struct{})
	e2.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := e2.sel.Dial(ctx); done <- err }()
	waitFor(t, "the QUIC dial to start", func() bool { q, _ := e2.calls(); return q == 1 })
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Dial = %v", err)
	}
	if _, tl := e2.calls(); tl != 0 {
		t.Fatal("a cancelled dial fell back")
	}
	if st := e2.sel.Stats(); st.QUICFailures != 0 || st.ConsecutiveFailures != 0 || st.Fallbacks != 0 {
		t.Fatalf("stats %+v", st)
	}
}

func TestSelectorBothCarriersFail(t *testing.T) {
	e := newSelectorEnv(t, ChoiceAuto)
	errTLS := &link.HandshakeError{Reason: link.ReasonUnknownCA, Err: errors.New("tls: unknown authority")}
	e.setQUIC(errQUICDown)
	e.mu.Lock()
	e.tlsErr = errTLS
	e.mu.Unlock()
	_, err := e.sel.Dial(ctxTimeout(t))
	if !errors.Is(err, errQUICDown) || !errors.Is(err, errTLS) {
		t.Fatalf("Dial = %v: both failures should be visible", err)
	}
	if link.ReasonOf(err) != link.ReasonUnknownCA {
		t.Fatalf("reason %v: TLS_TCP's error should lead", link.ReasonOf(err))
	}
}

func TestSelectorFixedChoices(t *testing.T) {
	t.Run("TLS_TCP", func(t *testing.T) {
		e := newSelectorEnv(t, ChoiceTLS)
		e.setQUIC(nil)
		if c := e.dial(); c != e.tlsCarrier {
			t.Fatal("not the TLS carrier")
		}
		if q, _ := e.calls(); q != 0 {
			t.Fatal("QUIC was tried")
		}
	})
	t.Run("QUIC", func(t *testing.T) {
		e := newSelectorEnv(t, ChoiceQUIC)
		if c := e.dial(); c != e.quicCarrier {
			t.Fatal("not the QUIC carrier")
		}
		e.setQUIC(errQUICDown)
		for range 5 {
			if _, err := e.sel.Dial(ctxTimeout(t)); !errors.Is(err, errQUICDown) {
				t.Fatalf("Dial = %v", err)
			}
		}
		if _, tl := e.calls(); tl != 0 {
			t.Fatal("a QUIC-only link fell back to TLS")
		}
		if st := e.sel.Stats(); st.QUICFailures != 5 || st.OnFallback || st.Fallbacks != 0 {
			t.Fatalf("stats %+v: failures count, the fallback state does not move", st)
		}
	})
	t.Run("PLAIN", func(t *testing.T) {
		e := newSelectorEnv(t, ChoicePlain)
		if c := e.dial(); c != e.plainCarrier {
			t.Fatal("not the plain carrier")
		}
	})
	t.Run("AUTO never selects PLAIN", func(t *testing.T) {
		e := newSelectorEnv(t, ChoiceAuto)
		e.sel.dialPlain = func(context.Context, SelectorConfig) (Carrier, error) {
			t.Error("Auto dialled plaintext")
			return e.plainCarrier, nil
		}
		e.setQUIC(errQUICDown)
		e.mu.Lock()
		e.tlsErr = errors.New("tls down too")
		e.mu.Unlock()
		for range 5 {
			if _, err := e.sel.Dial(ctxTimeout(t)); err == nil {
				t.Fatal("a dial succeeded with both carriers down")
			}
		}
	})
}

func TestNewSelectorValidates(t *testing.T) {
	ca := newTestPKI(t)
	creds := linkCreds(t, ca, ca.CAPEM(), "forward-1")
	good := func() SelectorConfig {
		return SelectorConfig{Address: "198.51.100.1:7000", Link: link.DialConfig{Credentials: creds, ServerName: "forward-2", PeerIdentity: nodeID("forward-2")}}
	}
	for name, mod := range map[string]func(*SelectorConfig){
		"no credentials":          func(c *SelectorConfig) { c.Link.Credentials = nil },
		"a bad identity":          func(c *SelectorConfig) { c.Link.PeerIdentity = "forward-2" },
		"no address":              func(c *SelectorConfig) { c.Address = "" },
		"an unknown choice":       func(c *SelectorConfig) { c.Choice = 9 },
		"negative limits":         func(c *SelectorConfig) { c.ProbeTimeout = -1 },
		"plain without the label": func(c *SelectorConfig) { c.Choice = ChoicePlain },
	} {
		cfg := good()
		mod(&cfg)
		if _, err := NewSelector(cfg); err == nil {
			t.Errorf("%s: NewSelector succeeded", name)
		}
	}
	s, err := NewSelector(good())
	if err != nil {
		t.Fatal(err)
	}
	if s.Choice() != ChoiceAuto || s.cfg.ProbeTimeout != DefaultProbeTimeout || s.cfg.FailLimit != DefaultFailLimit || s.cfg.RetryInterval != DefaultRetryInterval {
		t.Fatalf("defaults: %+v", s.cfg)
	}
	if DefaultProbeTimeout != 3*time.Second || DefaultFailLimit != 3 || DefaultRetryInterval != 5*time.Minute {
		t.Fatal("the defaults of section 5.4 changed")
	}
}

func TestCarrierChoiceNames(t *testing.T) {
	for in, want := range map[string]CarrierChoice{
		"": ChoiceAuto, "AUTO": ChoiceAuto, "auto": ChoiceAuto, "ANIXOPS_CARRIER_UNSPECIFIED": ChoiceAuto, "ANIXOPS_CARRIER_AUTO": ChoiceAuto,
		"TLS_TCP": ChoiceTLS, "tls_tcp": ChoiceTLS, "QUIC": ChoiceQUIC, "quic": ChoiceQUIC, "PLAIN": ChoicePlain,
	} {
		got, err := ParseCarrierChoice(in)
		if err != nil || got != want {
			t.Errorf("ParseCarrierChoice(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseCarrierChoice("RAW"); err == nil {
		t.Error("RAW parsed")
	}
	if ChoiceAuto.String() != "auto" || ChoiceTLS.String() != "tls_tcp" || ChoiceQUIC.String() != "quic" || ChoicePlain.String() != "plain" || CarrierChoice(7).String() == "" {
		t.Error("names")
	}
}

// The real thing: an AUTO listener takes both kinds of carrier, and a link that
// asks for AUTO gets QUIC.
func TestAutoLinkOverTheNetwork(t *testing.T) {
	ca := newTestPKI(t)
	server := linkCreds(t, ca, ca.CAPEM(), "forward-2")
	l, err := ListenAuto("127.0.0.1:0", link.ListenerConfig{Credentials: server, Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}}, QUICConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	if l.TCP() == nil || l.QUIC() == nil || l.Addr().(*net.TCPAddr).Port != l.QUIC().Addr().(*net.UDPAddr).Port {
		t.Fatal("one port number for TCP and UDP")
	}
	client := linkCreds(t, ca, ca.CAPEM(), "forward-1")
	selector := func(choice CarrierChoice) *Selector {
		s, err := NewSelector(SelectorConfig{
			Choice:  choice,
			Address: l.Addr().String(),
			Link:    link.DialConfig{Credentials: client, ServerName: "forward-2", PeerIdentity: nodeID("forward-2")},
		})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	var carriers []Carrier
	for _, tc := range []struct {
		choice CarrierChoice
		want   CarrierType
	}{{ChoiceAuto, CarrierQUIC}, {ChoiceQUIC, CarrierQUIC}, {ChoiceTLS, CarrierTLS}} {
		d, err := selector(tc.choice).Dial(ctxTimeout(t))
		if err != nil {
			t.Fatalf("%v: %v", tc.choice, err)
		}
		if d.Type() != tc.want {
			t.Fatalf("%v gave a %v carrier", tc.choice, d.Type())
		}
		a, err := l.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		if a.Type() != tc.want {
			t.Fatalf("the listener took a %v carrier for %v", a.Type(), tc.choice)
		}
		serveEcho(t, a)
		data := pattern(60_000, byte(tc.want))
		eqBytes(t, roundTrip(t, d, data), data)
		carriers = append(carriers, d, a)
	}
	st := l.Stats()
	if st.Carriers != 3 || st.Accepted != 3 || st.Link.Accepted != 3 {
		t.Fatalf("stats %+v", st)
	}
	if ts, qs := l.TCP().Stats(), l.QUIC().Stats(); ts.Accepted != 1 || qs.Accepted != 2 {
		t.Fatalf("tcp %+v quic %+v", ts, qs)
	}

	// a change of peers or sources applies to both
	if err := l.SetSources([]string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	removed, err := l.SetPeers([]string{nodeID("forward-3")})
	if err != nil || len(removed) != 1 {
		t.Fatalf("removed %v, %v", removed, err)
	}
	for _, c := range carriers {
		waitDone(t, c)
	}
	for _, choice := range []CarrierChoice{ChoiceTLS, ChoiceQUIC} {
		if c, err := selector(choice).Dial(ctxTimeout(t)); err == nil {
			_ = c.Close()
			t.Fatalf("a removed identity dialled with %v", choice)
		}
	}
	if _, err := l.SetPeers(nil); err == nil {
		t.Fatal("SetPeers(nil) succeeded")
	}
	if err := l.SetSources(nil); err == nil {
		t.Fatal("SetSources(nil) succeeded")
	}
}

// With UDP blocked between the nodes AUTO falls back to TLS_TCP within the
// probe, never to plaintext, and counts it.
func TestAutoFallsBackWhenUDPIsBlocked(t *testing.T) {
	ca := newTestPKI(t)
	server := linkCreds(t, ca, ca.CAPEM(), "forward-2")
	// A TLS listener only: nothing answers on the UDP port.
	l, err := Listen("127.0.0.1:0", link.ListenerConfig{Credentials: server, Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}}, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	s, err := NewSelector(SelectorConfig{
		Address:      l.Addr().String(),
		Link:         link.DialConfig{Credentials: linkCreds(t, ca, ca.CAPEM(), "forward-1"), ServerName: "forward-2", PeerIdentity: nodeID("forward-2")},
		ProbeTimeout: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		d, err := s.Dial(ctxTimeout(t))
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		if d.Type() != CarrierTLS {
			t.Fatalf("dial %d gave a %v carrier", i, d.Type())
		}
		a, err := l.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		serveEcho(t, a)
		eqBytes(t, roundTrip(t, d, pattern(1000, 1)), pattern(1000, 1))
		_ = d.Close()
	}
	if st := s.Stats(); st.Fallbacks != 2 || st.QUICFailures != 2 || st.ConsecutiveFailures != 2 {
		t.Fatalf("stats %+v", st)
	}
	// a QUIC-only link to the same node fails: no fallback of its own
	q, err := NewSelector(SelectorConfig{
		Choice:       ChoiceQUIC,
		Address:      l.Addr().String(),
		Link:         link.DialConfig{Credentials: linkCreds(t, ca, ca.CAPEM(), "forward-1"), ServerName: "forward-2", PeerIdentity: nodeID("forward-2"), HandshakeTimeout: 300 * time.Millisecond},
		ProbeTimeout: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c, err := q.Dial(ctxTimeout(t)); err == nil {
		_ = c.Close()
		t.Fatal("a QUIC dial to a TCP-only listener succeeded")
	}
}

// The AUTO listener's own lifecycle: Close ends every carrier it accepted and
// frees both ports.
func TestAutoListenerCloseFreesBothPorts(t *testing.T) {
	ca := newTestPKI(t)
	l, err := ListenAuto("127.0.0.1:0", link.ListenerConfig{Credentials: linkCreds(t, ca, ca.CAPEM(), "forward-2"), Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}}, QUICConfig{Config: Config{DrainTimeout: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	client := linkCreds(t, ca, ca.CAPEM(), "forward-1")
	var dialled []Carrier
	for _, choice := range []CarrierChoice{ChoiceQUIC, ChoiceTLS} {
		s, _ := NewSelector(SelectorConfig{Choice: choice, Address: addr, Link: link.DialConfig{Credentials: client, ServerName: "forward-2", PeerIdentity: nodeID("forward-2")}})
		d, err := s.Dial(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := l.Accept(ctxTimeout(t)); err != nil {
			t.Fatal(err)
		}
		dialled = append(dialled, d)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
	for _, d := range dialled {
		waitDone(t, d)
		if r, ok := d.PeerGoAway(); !ok || r != GoAwayListenerClosed {
			t.Fatalf("a %v carrier was told %v %v", d.Type(), r, ok)
		}
	}
	if _, err := l.Accept(ctxTimeout(t)); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept after Close = %v", err)
	}
	tcp, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("the TCP port is still held: %v", err)
	}
	_ = tcp.Close()
	udp, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("the UDP port is still held: %v", err)
	}
	_ = udp.Close()
}

func TestListenAutoRefusesBadConfigs(t *testing.T) {
	ca := newTestPKI(t)
	creds := linkCreds(t, ca, ca.CAPEM(), "forward-2")
	ok := link.ListenerConfig{Credentials: creds, Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}}
	if _, err := ListenAuto("no port", ok, QUICConfig{}); err == nil {
		t.Fatal("an address without a port")
	}
	bad := ok
	bad.Peers = nil
	if _, err := ListenAuto("127.0.0.1:0", bad, QUICConfig{}); err == nil {
		t.Fatal("a listener without peers")
	}
	if _, err := ListenAuto("127.0.0.1:0", ok, QUICConfig{Config: Config{MaxFrame: 1}}); err == nil {
		t.Fatal("a bad carrier configuration")
	}
	// a port taken for UDP: the TCP listener must not be left behind
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Close() }()
	port := pc.LocalAddr().(*net.UDPAddr).Port
	if l, err := ListenAuto(net.JoinHostPort("127.0.0.1", itoa(port)), ok, QUICConfig{}); err == nil {
		_ = l.Close()
		t.Fatal("ListenAuto took a UDP port that is in use")
	}
	tcp, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", itoa(port)))
	if err != nil {
		t.Fatalf("a failed ListenAuto left its TCP listener behind: %v", err)
	}
	_ = tcp.Close()
}

// A dial is bounded by its context in the SETTINGS exchange too, not only in
// the handshake: a listener that completes the handshake and then says nothing
// must not hold an AUTO probe for the handshake timeout.
func TestDialsHonourTheContextDuringTheSettingsExchange(t *testing.T) {
	ca := newTestPKI(t)
	server := linkCreds(t, ca, ca.CAPEM(), "forward-2")
	client := linkCreds(t, ca, ca.CAPEM(), "forward-1")
	dcfg := link.DialConfig{Credentials: client, ServerName: "forward-2", PeerIdentity: nodeID("forward-2")}
	lcfg := link.ListenerConfig{Credentials: server, Protocol: ALPN, Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}}

	t.Run("QUIC", func(t *testing.T) {
		tr, err := (QUICConfig{}).Transport(RoleAcceptor)
		if err != nil {
			t.Fatal(err)
		}
		ll, err := link.ListenQUIC("127.0.0.1:0", link.QUICListenerConfig{ListenerConfig: lcfg, QUIC: tr})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = ll.Close() }()
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err = DialQUIC(ctx, ll.Addr().String(), dcfg, QUICConfig{})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("DialQUIC = %v, want the context's deadline", err)
		}
		if took := time.Since(start); took > testWait {
			t.Fatalf("DialQUIC took %v: it waited for the handshake timeout", took)
		}
	})
	t.Run("TLS", func(t *testing.T) {
		ll, err := link.Listen("127.0.0.1:0", lcfg)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = ll.Close() }()
		quit := make(chan struct{})
		defer close(quit)
		go func() { // takes the connection and says nothing
			if c, err := ll.Accept(); err == nil {
				defer func() { _ = c.Close() }()
				<-quit
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err = DialTLS(ctx, ll.Addr().String(), dcfg, Config{})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("DialTLS = %v, want the context's deadline", err)
		}
		if took := time.Since(start); took > testWait {
			t.Fatalf("DialTLS took %v: it waited for the handshake timeout", took)
		}
	})
}

// AUTO's probe covers the whole QUIC dial: a QUIC listener that completes the
// handshake and then hangs costs the probe, and the link gets its TLS carrier.
func TestAutoProbeIsBoundedWhenQUICHangsAfterTheHandshake(t *testing.T) {
	ca := newTestPKI(t)
	server := linkCreds(t, ca, ca.CAPEM(), "forward-2")
	lcfg := link.ListenerConfig{Credentials: server, Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}}
	tls, err := Listen("127.0.0.1:0", lcfg, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tls.Close() }()
	port := tls.Addr().(*net.TCPAddr).Port
	qlcfg := lcfg
	qlcfg.Protocol = ALPN
	tr, _ := (QUICConfig{}).Transport(RoleAcceptor)
	rogue, err := link.ListenQUIC(net.JoinHostPort("127.0.0.1", itoa(port)), link.QUICListenerConfig{ListenerConfig: qlcfg, QUIC: tr})
	if err != nil {
		t.Skipf("the UDP port of the TCP listener is taken: %v", err)
	}
	defer func() { _ = rogue.Close() }()
	s, err := NewSelector(SelectorConfig{
		Address:      tls.Addr().String(),
		Link:         link.DialConfig{Credentials: linkCreds(t, ca, ca.CAPEM(), "forward-1"), ServerName: "forward-2", PeerIdentity: nodeID("forward-2")},
		ProbeTimeout: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	d, err := s.Dial(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if d.Type() != CarrierTLS {
		t.Fatalf("got a %v carrier", d.Type())
	}
	if took := time.Since(start); took > testWait {
		t.Fatalf("the dial took %v, the probe is 300 ms", took)
	}
	if st := s.Stats(); st.Fallbacks != 1 || st.QUICFailures != 1 {
		t.Fatalf("stats %+v", st)
	}
}
