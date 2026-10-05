package relay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
)

// CarrierChoice is the carrier a link asks for (anixops-protocol.md section
// 5.4). The zero value is Auto, as the contract's unspecified carrier means
// AUTO.
type CarrierChoice uint8

const (
	// ChoiceAuto tries QUIC first and falls back to TLS_TCP; never to
	// plaintext (the contract's AUTO).
	ChoiceAuto CarrierChoice = iota
	// ChoiceTLS is TLS over TCP only (TLS_TCP).
	ChoiceTLS
	// ChoiceQUIC is QUIC only: failures count against the upstream like any
	// dial failure.
	ChoiceQUIC
	// ChoicePlain is plaintext TCP on a trusted link (PLAIN), never selected
	// by Auto and never a fallback.
	ChoicePlain
)

// String returns the lower-case name used in logs and metrics labels.
func (c CarrierChoice) String() string {
	switch c {
	case ChoiceAuto:
		return "auto"
	case ChoiceTLS:
		return "tls_tcp"
	case ChoiceQUIC:
		return "quic"
	case ChoicePlain:
		return "plain"
	}
	return fmt.Sprintf("choice(%d)", uint8(c))
}

// ParseCarrierChoice parses the names of the contract's AnixOpsCarrier enum
// ("AUTO", "TLS_TCP", "QUIC", "PLAIN") and of String, in any case; the empty
// string is Auto.
func ParseCarrierChoice(s string) (CarrierChoice, error) {
	switch strings.ToLower(strings.TrimPrefix(strings.ToUpper(s), "ANIXOPS_CARRIER_")) {
	case "", "auto", "unspecified":
		return ChoiceAuto, nil
	case "tls_tcp", "tls":
		return ChoiceTLS, nil
	case "quic":
		return ChoiceQUIC, nil
	case "plain":
		return ChoicePlain, nil
	}
	return 0, fmt.Errorf("relay: unknown carrier %q", s)
}

// Defaults of the Auto choice (anixops-protocol.md section 5.4).
const (
	// DefaultProbeTimeout bounds a QUIC attempt of Auto: a QUIC handshake
	// (and the SETTINGS exchange after it) that has not completed in this
	// time is given up for TLS_TCP.
	DefaultProbeTimeout = 3 * time.Second
	// DefaultFailLimit is the number of QUIC dials in a row that fail before
	// Auto stops trying QUIC.
	DefaultFailLimit = 3
	// DefaultRetryInterval is how often Auto tries QUIC again while it is on
	// the fallback.
	DefaultRetryInterval = 5 * time.Minute
)

// SelectorConfig configures a Selector: how one link of a route, from this
// hop to one upstream, makes its carriers.
type SelectorConfig struct {
	// Choice is the link's carrier; the zero value is Auto.
	Choice CarrierChoice
	// Address is the upstream's host and port: TCP for TLS_TCP and PLAIN, UDP
	// for QUIC, at the same port number.
	Address string
	// Link holds this node's credentials, the link's server_name and the
	// upstream's peer_identity, for TLS_TCP and QUIC. Its Protocol is always
	// ALPN.
	Link link.DialConfig
	// Plain is the plaintext link's configuration, for PLAIN only; its
	// TrustedLink must be set, which is the caller asserting that validation
	// allowed PLAIN for this link.
	Plain link.PlainDialConfig
	// Carrier configures TLS and plaintext carriers, QUIC the QUIC ones.
	Carrier Config
	QUIC    QUICConfig

	// ProbeTimeout, FailLimit and RetryInterval tune Auto; the defaults are the
	// document's 3 s, 3 and 5 minutes.
	ProbeTimeout  time.Duration
	FailLimit     int
	RetryInterval time.Duration

	// Now is the clock of the retry interval; time.Now when nil.
	Now func() time.Time
}

// SelectorStats are a Selector's counters and state.
type SelectorStats struct {
	// QUICAttempts and QUICFailures count the QUIC dials made and the ones that
	// failed (by Auto or by ChoiceQUIC).
	QUICAttempts uint64
	QUICFailures uint64
	// Fallbacks counts the dials of Auto that tried QUIC, failed, and used
	// TLS_TCP instead; FallbackDials counts those that went straight to TLS_TCP
	// because Auto was on the fallback (QUIC to TLS fallbacks, section 7.3).
	Fallbacks     uint64
	FallbackDials uint64
	// ConsecutiveFailures is the number of QUIC dials in a row that failed.
	ConsecutiveFailures int
	// OnFallback says that Auto has stopped trying QUIC (until RetryAt).
	OnFallback bool
	// RetryAt is when Auto tries QUIC again; the zero time when it is not on
	// the fallback.
	RetryAt time.Time
}

// Selector makes the carriers of one link. With Auto it dials QUIC first and
// falls back to TLS_TCP: a QUIC dial gets ProbeTimeout, and one that fails or
// times out is made up for at once with a TLS_TCP dial, so a firewall that
// drops UDP costs one probe, not a failed dial. After FailLimit failures in a
// row Auto stops trying QUIC and dials TLS_TCP directly, and tries QUIC again
// once RetryInterval has passed: one dial at a time probes, and the others
// keep using TLS_TCP. The fallback is only ever between the two authenticated
// carriers; nothing falls back to plaintext, and an Auto link never selects
// PLAIN. Safe for concurrent use.
type Selector struct {
	cfg SelectorConfig

	dialQUIC  func(ctx context.Context, cfg SelectorConfig) (Carrier, error)
	dialTLS   func(ctx context.Context, cfg SelectorConfig) (Carrier, error)
	dialPlain func(ctx context.Context, cfg SelectorConfig) (Carrier, error)

	mu       sync.Mutex
	fails    int
	fallback bool
	retryAt  time.Time
	probing  bool
	stats    SelectorStats
}

// NewSelector validates cfg and returns the selector of a link.
func NewSelector(cfg SelectorConfig) (*Selector, error) {
	switch cfg.Choice {
	case ChoiceAuto, ChoiceTLS, ChoiceQUIC:
		if cfg.Link.Credentials == nil {
			return nil, errors.New("relay: a selector for an encrypted carrier needs credentials")
		}
		if _, err := link.ParseIdentity(cfg.Link.PeerIdentity); err != nil {
			return nil, err
		}
	case ChoicePlain:
		if !cfg.Plain.TrustedLink {
			return nil, errors.New("relay: a plaintext link is for trusted links only (SelectorConfig.Plain.TrustedLink)")
		}
	default:
		return nil, fmt.Errorf("relay: unknown carrier choice %d", uint8(cfg.Choice))
	}
	if cfg.Address == "" {
		return nil, errors.New("relay: a selector needs the upstream's address")
	}
	if cfg.ProbeTimeout < 0 || cfg.FailLimit < 0 || cfg.RetryInterval < 0 {
		return nil, errors.New("relay: negative selector limits")
	}
	if cfg.ProbeTimeout == 0 {
		cfg.ProbeTimeout = DefaultProbeTimeout
	}
	if cfg.FailLimit == 0 {
		cfg.FailLimit = DefaultFailLimit
	}
	if cfg.RetryInterval == 0 {
		cfg.RetryInterval = DefaultRetryInterval
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Selector{
		cfg: cfg,
		dialQUIC: func(ctx context.Context, c SelectorConfig) (Carrier, error) {
			car, err := DialQUIC(ctx, c.Address, c.Link, c.QUIC)
			if err != nil {
				return nil, err
			}
			return car, nil
		},
		dialTLS: func(ctx context.Context, c SelectorConfig) (Carrier, error) {
			car, err := DialTLS(ctx, c.Address, c.Link, c.Carrier)
			if err != nil {
				return nil, err
			}
			return car, nil
		},
		dialPlain: func(ctx context.Context, c SelectorConfig) (Carrier, error) {
			car, err := DialPlain(ctx, c.Address, c.Plain, c.Carrier)
			if err != nil {
				return nil, err
			}
			return car, nil
		},
	}, nil
}

// Choice returns the link's carrier choice.
func (s *Selector) Choice() CarrierChoice { return s.cfg.Choice }

// Stats returns a snapshot of the selector's counters and state.
func (s *Selector) Stats() SelectorStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.stats
	st.ConsecutiveFailures, st.OnFallback, st.RetryAt = s.fails, s.fallback, s.retryAt
	return st
}

// Dial makes one carrier to the upstream with the link's carrier choice.
func (s *Selector) Dial(ctx context.Context) (Carrier, error) {
	switch s.cfg.Choice {
	case ChoiceTLS:
		return s.dialTLS(ctx, s.cfg)
	case ChoicePlain:
		return s.dialPlain(ctx, s.cfg)
	case ChoiceQUIC:
		return s.tryQUIC(ctx, ctx, false)
	}
	return s.dialAuto(ctx)
}

// tryQUIC makes one QUIC dial under dialCtx (the probe's, for Auto; ctx is the
// caller's) and records its outcome; only Auto's outcomes move the fallback.
func (s *Selector) tryQUIC(ctx, dialCtx context.Context, auto bool) (Carrier, error) {
	s.mu.Lock()
	s.stats.QUICAttempts++
	s.mu.Unlock()
	c, err := s.dialQUIC(dialCtx, s.cfg)
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case err == nil:
		if auto {
			s.fails, s.fallback, s.retryAt = 0, false, time.Time{}
		}
	case ctx.Err() != nil:
		// The caller gave up: not evidence about QUIC.
	default:
		s.stats.QUICFailures++
		if auto {
			s.fails++
			if s.fails >= s.cfg.FailLimit {
				s.fallback, s.retryAt = true, s.cfg.Now().Add(s.cfg.RetryInterval)
			}
		}
	}
	if auto {
		s.probing = false
	}
	return c, err
}

// shouldProbe says whether this Auto dial tries QUIC: always while QUIC works,
// and while on the fallback, only for the first dial after the retry time.
func (s *Selector) shouldProbe() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallback {
		if s.probing || s.cfg.Now().Before(s.retryAt) {
			s.stats.FallbackDials++
			return false
		}
		s.probing = true
	}
	return true
}

func (s *Selector) dialAuto(ctx context.Context) (Carrier, error) {
	if s.shouldProbe() {
		pctx, cancel := context.WithTimeout(ctx, s.cfg.ProbeTimeout)
		c, qerr := s.tryQUIC(ctx, pctx, true)
		cancel()
		if qerr == nil {
			return c, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		s.mu.Lock()
		s.stats.Fallbacks++
		s.mu.Unlock()
		tc, terr := s.dialTLS(ctx, s.cfg)
		if terr != nil {
			// TLS_TCP's error leads: it is the one that decides the link.
			return nil, errors.Join(terr, fmt.Errorf("relay: QUIC before it: %w", qerr))
		}
		return tc, nil
	}
	return s.dialTLS(ctx, s.cfg)
}
