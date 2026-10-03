package driver

import (
	"context"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// Driver runs one engine's share of a node's forwarding state. The semantics
// every implementation must keep are in the package documentation and are
// checked by sdk/forward/driver/conformance.
//
// Every method is safe for concurrent use.
type Driver interface {
	// Engine is the engine the driver serves. It is constant and needs no
	// host access; Registry keys drivers by it.
	Engine() forwardv1.Engine
	// Capabilities probes the host: engine version, IPv6, UDP, strategies,
	// link securities, limits support. An unavailable driver answers
	// Available false with UnavailableReason, not an error; the error is for
	// a probe that could not run (a cancelled context).
	Capabilities(ctx context.Context) (*forwardv1.EngineCapabilities, error)
	// Render turns the hops of state whose engine is Engine() into the
	// engine's artifact. It is pure and deterministic: it reads nothing but
	// its argument and the driver's static configuration, and the same hops
	// in any order give the same bytes. Hops of other engines are ignored; a
	// state with none of this engine's hops renders a valid empty artifact,
	// which Apply turns into "own nothing".
	//
	// A hop the driver cannot run is left out of the artifact and reported
	// as a *HopError (errors.Is ErrUnsupported or ErrInvalidState). Render
	// then returns BOTH the artifact of the remaining hops and a non-nil
	// *RenderError listing the rejected hops: the caller applies the
	// artifact and reports the hop errors, so one bad hop never stops the
	// node's other hops. Any other error (a nil state) comes with a zero
	// Artifact.
	Render(state *forwardv1.NodeForwardState) (Artifact, error)
	// Apply makes the host run exactly the artifact, atomically, touching
	// only objects the driver owns. It compares the host with the artifact,
	// not with what the driver remembers: on a host that already runs the
	// artifact it changes nothing (ApplyResult.Changed false), and on a host
	// with partial or damaged owned state it repairs it. A failed Apply
	// leaves the previous owned state in place.
	Apply(ctx context.Context, artifact Artifact) (ApplyResult, error)
	// Observe reads what the host runs now: the applied generation and
	// digest, counters per hop and direction, the engine's own view of
	// upstream health and the upstreams in rotation. It reads the host, so
	// a new driver instance (an Agent restart) observes the state an
	// earlier instance applied.
	Observe(ctx context.Context) (Observation, error)
	// SetUpstreams changes which upstreams of one applied hop are in
	// rotation, and their weights, without a full apply: no rule is
	// rewritten, no counter is reset, the applied digest and generation stay.
	// The health loop calls it for failover and least-connections
	// re-weighting.
	SetUpstreams(ctx context.Context, routeID string, hopIndex uint32, active []Upstream) error
	// Remove deletes everything the driver owns and nothing else. It is
	// idempotent: removing when nothing is owned succeeds.
	Remove(ctx context.Context) error
}

// Upstream names one of a hop's rendered upstreams for SetUpstreams by its
// address and port, with the weight to use. Weight 0 keeps the weight the
// hop was rendered with.
type Upstream struct {
	Address string
	Port    uint32
	Weight  uint32
}

// ApplyResult says what an Apply did.
type ApplyResult struct {
	// Changed is false when the host already ran the artifact and Apply
	// touched nothing.
	Changed bool
	// Generation, StateHash and Digest are what the host runs now: the
	// artifact's.
	Generation uint64
	StateHash  string
	Digest     string
}

// Observation is a snapshot of what a driver's host runs.
type Observation struct {
	Engine forwardv1.Engine
	// Applied is false when the driver owns nothing (never applied, or
	// removed). Generation, StateHash and Digest are then empty.
	Applied    bool
	NodeRef    string
	Generation uint64
	StateHash  string
	Digest     string
	// Counters has one entry per applied hop. Cumulative fields (bytes,
	// packets, total_conns) never decrease within one counter_epoch; see
	// the package documentation for when an epoch ends.
	Counters []*forwardv1.Counters
	// Health is the engine's own view of its upstreams (gost's fail
	// marking, for example); an engine without one (nftables) leaves it
	// empty and the Agent's health loop is the source of truth.
	Health []*forwardv1.UpstreamHealth
	// Rotation lists, per applied hop, the upstreams in rotation now:
	// every rendered upstream after an Apply that changed something, the
	// last SetUpstreams selection otherwise.
	Rotation   []HopRotation
	ObservedAt time.Time
}

// HopRotation is the set of upstreams one hop balances over now.
type HopRotation struct {
	RouteID  string
	HopIndex uint32
	Active   []Upstream
}

// ToReport converts the observation into the contract's report, as the
// Agent sends it (without the HopErrors, which come from Render and Apply).
func (o Observation) ToReport() *forwardv1.NodeForwardReport {
	r := &forwardv1.NodeForwardReport{
		NodeRef:          o.NodeRef,
		Generation:       o.Generation,
		StateHash:        o.StateHash,
		Applied:          o.Applied,
		Counters:         o.Counters,
		Health:           o.Health,
		ObservedAtUnixMs: o.ObservedAt.UnixMilli(),
	}
	if o.ObservedAt.IsZero() {
		r.ObservedAtUnixMs = 0
	}
	return r
}
