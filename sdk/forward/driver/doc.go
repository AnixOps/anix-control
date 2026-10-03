// Package driver is the boundary between a node's forwarding state and the
// engines that run it (docs/architecture/forward-sdk.md section 6). The
// Agent splits a NodeForwardState by engine (Registry.Render), applies each
// engine's Artifact through that engine's Driver, runs one health loop that
// calls SetUpstreams, and reports Observe. The nftables driver (F2b/F2c) and
// the gost driver (F4a/F4b) implement it; sdk/forward/driver/fake is an in-memory
// one for tests; sdk/forward/driver/conformance checks any of them against
// the rules below.
//
// # Ownership
//
// A driver touches only the objects it owns (the nftables driver: the table
// "inet anixops_fwd", its tc handles and sysctl drop-in; the gost driver:
// the anixops-gost unit and its files; owner decision H13). It marks what
// it creates so it can tell its objects from others' after a restart. An
// object with its name but not its mark is foreign: Apply refuses it with
// ErrNotOwned and Remove leaves it. Applying an artifact that would collide
// with a foreign object (a listen port another table or process holds) is
// ErrConflict, and neither side changes.
//
// # Render
//
// Render is pure and deterministic. It reads the hops of its engine only,
// in any order, and nothing but them and the driver's static
// configuration: Artifact.Content and Artifact.Digest do not depend on the
// state's generation, state_hash or node_ref, or on the other engines'
// hops. Hops the driver cannot run are rejected one by one as *HopError in
// a *RenderError, returned together with the artifact of the remaining
// hops. A state with none of the engine's hops renders an empty artifact,
// and applying it removes everything the driver owns.
//
// # Apply and generations
//
// Apply is atomic: the host runs either the whole artifact or what it ran
// before. It compares the host with the artifact rather than trusting what
// it applied last, so:
//
//   - applying what the host already runs changes nothing
//     (ApplyResult.Changed false); with a newer generation (another
//     engine's change bumped it) only the recorded generation and
//     state_hash move;
//   - applying after a crash or onto partial or damaged owned state
//     repairs it (Changed true).
//
// The driver records the applied generation, state_hash and digest on the
// host (a set in its own table, a config file), so a new driver instance after an
// Agent restart observes them. An artifact with an older generation than
// the host runs is ErrStaleGeneration (the contract's FAILED_PRECONDITION);
// the same generation with another digest is ErrGenerationConflict. Both
// leave the host alone. An empty artifact removes everything; the host then
// holds no generation and the Agent's persisted state is the guard.
//
// # Counters
//
// Observe answers one Counters per applied hop. Its cumulative fields
// (up/down bytes and packets, total_conns) never decrease within one
// counter_epoch. The epoch belongs to the hop's counter objects and ends
// only when they are re-created from zero: the hop was removed and added
// again, its objects were lost (repair after damage, a reboot, the engine
// restarted, Remove then Apply). A generation change alone, or an Apply
// that rewrites the hop, keeps the epoch and the values. Control adds the
// differences within an epoch and the full value of a new one
// (forward-sdk.md section 8.2).
//
// # Failover
//
// SetUpstreams puts a subset of one applied hop's rendered upstreams in
// rotation, with weights (0 keeps the rendered weight), without a full
// apply: no rule is rewritten, the digest, generation and counter epochs
// stay. An empty or duplicate selection is ErrInvalidArgument (keeping the
// last upstream when all are down is the health loop's decision, section
// 7.3); an unknown hop or an upstream the hop was not rendered with is
// ErrNotFound. The rotation lives on the host, so it survives a driver
// restart and a no-op Apply; an Apply that changes the host puts every
// rendered upstream back, and the health loop re-asserts its selection.
//
// # Paused hops
//
// A hop with paused set stays applied: its objects and counters remain, so
// its epoch and history survive a pause, and it appears in Observe with
// its rotation; the driver refuses or drops its traffic.
//
// # Remove
//
// Remove deletes every owned object and nothing else, and is idempotent.
//
// # Context and concurrency
//
// Every method takes a context except Engine and Render, which do no I/O.
// A call whose context is already done returns ctx.Err() (errors.Is
// context.Canceled or DeadlineExceeded) and changes nothing; Apply
// cancelled in flight still leaves the host on either the old or the new
// artifact. Every method is safe for concurrent use: Apply, SetUpstreams
// and Remove are serialised, and Observe sees the host before or after
// each of them, never in between.
//
// # Errors
//
// Errors wrap the sentinels of errors.go (ErrUnsupported, ErrConflict,
// ErrNotOwned, ErrStaleGeneration, ...); callers test with errors.Is.
package driver
