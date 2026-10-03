// Package conformance is the forward driver conformance suite
// (docs/architecture/forward-sdk.md section 13): one list of scenarios that
// every driver in sdk/forward/driver must pass, the in-memory fake now, the
// nftables (F2b/F2c) and gost (F4) drivers later.
//
// A driver's test plugs in an environment and runs the suite:
//
//	func TestConformance(t *testing.T) {
//		conformance.Run(t, func(t *testing.T) conformance.Env { return newEnv(t) })
//	}
//
// Env is the host the driver runs on: it makes driver instances (a second
// one is an Agent restart), lists the objects the driver owns, and plants
// and lists foreign objects. Optional interfaces (Damager, TrafficSource,
// ApplyFaulter, ConflictPlanter, ImpostorPlanter, ApplyCounter) unlock the
// scenarios that need them; without one, those scenarios skip and say
// which interface is missing. A netns harness (F2d) starts with the
// required four methods and adds the rest as it learns to.
//
// The suite is data-driven twice over. StateCases are the states it
// renders and applies (single target, every strategy, TCP and UDP, IPv4,
// IPv6 and dual stack, limits, several routes, a paused hop), each run only
// when the driver's Capabilities cover it, built on a Topology the harness
// may override with its own addresses and ports (WithTopology). Scenarios
// are the behaviours (determinism, idempotency, generations, restart and
// repair, ownership, counters, failover, removal, context, concurrency);
// Scenarios() lists them with one line each.
//
// Two checks run in every scenario: foreign objects planted before it must
// be unchanged after it, and every observation must be well formed with no
// cumulative counter lower than an earlier observation of the same hop and
// counter_epoch, across driver restarts.
//
// The package imports "testing" and is meant for tests only.
package conformance
