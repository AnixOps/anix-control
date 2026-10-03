package model

import "time"

// The defaults a zero field stands for. They are the single source for the
// planner, the drivers and the UI.
//
// Decided, H21 (owner, 2026-10-03): the health-check, circuit-breaker,
// balancing and least-connections defaults are the design's recommendation
// (forward-sdk.md section 7.3, from RelayPanel's practice): checks every 5 s
// with a 2 s timeout, 3 failures in a row open the breaker for 30 s, and
// least-connections re-weights every 10 s. Change them here only.
const (
	// DefaultHealthInterval is HealthCheck.interval_ms when 0 (H21).
	DefaultHealthInterval = 5 * time.Second
	// DefaultHealthTimeout is HealthCheck.timeout_ms when 0 (H21).
	DefaultHealthTimeout = 2 * time.Second
	// DefaultFailureThreshold is CircuitBreaker.failure_threshold when 0:
	// failures in a row that open the breaker (H21).
	DefaultFailureThreshold uint32 = 3
	// DefaultBreakerOpen is CircuitBreaker.open_ms when 0: how long an open
	// upstream is skipped before one trial (H21).
	DefaultBreakerOpen = 30 * time.Second
	// DefaultBalance is Policy.next_hop and Policy.target when unspecified
	// (H21).
	DefaultBalance = BalanceRoundRobin
	// DefaultLeastConnReweight is how often the Agent re-weights a
	// LEAST_CONN hop's upstreams from their live connection counts
	// (forward-sdk.md section 7.1, H21).
	DefaultLeastConnReweight = 10 * time.Second
	// DefaultDirect is Policy.direct when unspecified: the chain as written.
	DefaultDirect = DirectOff
	// DefaultTargetPolicy is Policy.target_policy when unspecified, as the
	// contract says.
	DefaultTargetPolicy = TargetPolicyPublicOnly
	// DefaultWeight is Target.weight when 0, as the contract says.
	DefaultWeight uint32 = 1
)

// WithDefaults answers h with every zero field set to its default.
// Disabled is kept as is.
func (h HealthCheck) WithDefaults() HealthCheck {
	if h.Interval == 0 {
		h.Interval = DefaultHealthInterval
	}
	if h.Timeout == 0 {
		h.Timeout = DefaultHealthTimeout
	}
	return h
}

// WithDefaults answers c with every zero field set to its default.
func (c CircuitBreaker) WithDefaults() CircuitBreaker {
	if c.FailureThreshold == 0 {
		c.FailureThreshold = DefaultFailureThreshold
	}
	if c.OpenFor == 0 {
		c.OpenFor = DefaultBreakerOpen
	}
	return c
}

// WithDefaults answers p with every unspecified field set to its default.
func (p Policy) WithDefaults() Policy {
	if p.NextHop == BalanceUnspecified {
		p.NextHop = DefaultBalance
	}
	if p.Target == BalanceUnspecified {
		p.Target = DefaultBalance
	}
	if p.Direct == DirectUnspecified {
		p.Direct = DefaultDirect
	}
	if p.TargetPolicy == TargetPolicyUnspecified {
		p.TargetPolicy = DefaultTargetPolicy
	}
	p.Health = p.Health.WithDefaults()
	p.CircuitBreaker = p.CircuitBreaker.WithDefaults()
	return p
}

// EffectiveWeight answers the target's weight, 1 when unset.
func (t Target) EffectiveWeight() uint32 {
	if t.Weight == 0 {
		return DefaultWeight
	}
	return t.Weight
}
