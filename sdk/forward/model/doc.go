// Package model holds the Go domain types of the forwarding contract
// (anixops.forward.v1, sdk/api/forward/v1): a Route is a chain of Hops from
// an entry node through optional relays to an exit, and on to Targets, with
// a Policy (balancing, health checks, circuit breaker, direct mode, target
// policy) and Limits enforced on the entry. Counters, NodeInfo and
// EngineCapabilities describe what nodes report and what the planner knows
// about them. The design is docs/architecture/forward-sdk.md (section 4).
//
// Conversion. FromProto and ToProto convert both ways without loss, with
// these conventions:
//
//   - Enum types share the contract's numbers, so a value this package does
//     not know survives a round trip; IsKnown says whether it is one of the
//     contract's values.
//   - Sub-messages are plain values. An absent sub-message and an empty one
//     mean the same in the contract ("0 for the default", "empty for none"),
//     and ToProto writes an empty value as absent. A round trip is therefore
//     exact up to that one difference, which changes no field's meaning.
//   - Times (*_unix_ms) are time.Time in UTC; 0 is the zero time.Time (so
//     the one instant -62135596800000, which is the zero time.Time, reads
//     back as 0; validation refuses negative times anyway).
//     Durations (*_ms) are time.Duration; ToProto rounds down to whole
//     milliseconds and clamps to the field's range, which never matters for
//     a value that came from FromProto.
//
// Defaults. A zero field means "the default" wherever the contract says so.
// The defaults are in one place, defaults.go; WithDefaults fills them in.
// The health-check, circuit-breaker and least-connections defaults are
// owner decision H21.
//
// Validation lives in sdk/forward/validate, which Control, the planner and
// the Agent share.
package model
