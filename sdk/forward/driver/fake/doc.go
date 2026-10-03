// Package fake is an in-memory forward driver (sdk/forward/driver) for
// tests: the conformance suite's reference implementation and, later, the
// Agent's forward component tests.
//
// A Host is the simulated machine: the objects the driver owns (one per
// applied hop, with its counters, counter epoch, upstream rotation and
// engine health), objects that belong to others (PlantForeign,
// PlantConflict, PlantImpostor) and queued failures (FailNext). A Driver
// keeps no state of its own and reads everything back from its Host, so a
// second Driver on the same Host behaves as the Agent after a restart.
//
// Faithfulness. The fake keeps every rule of the driver contract:
//
//   - Render is pure and deterministic; its content is the deterministic
//     protobuf encoding of the accepted hops, sorted, and nothing else.
//     It refuses, per hop, what Options.Capabilities do not cover
//     (ErrUnsupported) and malformed hops (ErrInvalidState).
//   - Apply is atomic and compares the host with the artifact: a host that
//     runs it already is left alone (Changed false, a newer generation is
//     recorded), a damaged host (Damage) is repaired. Older generations are
//     refused, a known generation with another digest too. A foreign
//     object holding a listen port is ErrConflict, an impostor table
//     ErrNotOwned; neither is touched. A hop that survives an apply keeps
//     its counters and epoch; a new or re-created hop starts a new epoch.
//     An Apply that changes the host puts every upstream back in rotation.
//   - SetUpstreams only changes rotation and weights.
//   - Remove drops the owned objects only.
//
// Traffic and health are simulated with AddTraffic and SetHealth.
package fake
