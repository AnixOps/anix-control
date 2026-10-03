// Package nftables is the forward driver for ENGINE_NFTABLES
// (docs/architecture/forward-sdk.md section 6.1): kernel DNAT, masquerade,
// counters per direction, balancing maps, named quotas and connection
// limits in one table, "inet anixops_fwd", which is the only nftables
// object it ever touches (owner decision H13).
//
// Render (F2b) is pure; Apply, Observe, SetUpstreams and Remove (F2c) run
// nft and tc through a Runner (ExecRunner on a host; tests run them in a
// network namespace). Probe checks the host once and fills the Config that
// New takes, so Render stays a function of its configuration.
//
// # Rendered script
//
// Render produces one `nft -f` script, a single transaction (golden
// examples in contracts/forward/v1/nft). After the header comes the
// manifest, comment lines Apply reads ("# anixops-hop ...", "# anixops-upstream
// ..."): every rendered upstream with its weight and priority (failover's
// backups are not in any map), the strategy, the listener and the
// bandwidth. Then:
//
//  1. a "table inet anixops_fwd" block that declares the table with its
//     ownership comment (OwnerComment) and every object of every hop, with
//     no elements and no rules, so the next steps find them on a host that
//     has none yet;
//  2. "flush table inet anixops_fwd" (the rules of every chain) and a
//     "flush set" or "flush map" for each admission set and balancing map
//     (flush table keeps elements);
//  3. a second block that adds the elements and the rules.
//
// Counters, quotas (whose limit is updated in place) and the connection
// count sets are declared but never flushed, so they keep their values
// across renders: the counter epoch of a hop only ends when its objects are
// re-created. The script deletes nothing. A state without nftables hops
// renders a script of comments only; applying it removes the table.
//
// # Apply
//
// Apply reads `nft -j list table inet anixops_fwd` first. A table of that
// name without OwnerComment is foreign: ErrNotOwned, before anything runs
// (step 2 of the script would empty it). The generation is checked against
// the one recorded on the host (ErrStaleGeneration, ErrGenerationConflict).
// The host runs the artifact when the recorded digest is the artifact's,
// the table's fingerprint matches the seal recorded after the last change,
// and the tc objects are the ones the artifact needs: then Apply changes
// nothing but, for a newer generation, the recorded generation and
// state_hash. Otherwise it reads the whole ruleset (read only) and refuses
// a foreign rule that DNATs or redirects one of the artifact's listen ports
// (ErrConflict), checks the transaction with `nft -c`, adds the tc qdisc
// and classes it needs, and runs one `nft -f` transaction:
//
//   - a table block that declares the hops' counters with the comment
//     "anixops epoch <nonce>" (nft keeps the comment of a counter that
//     exists, so only new counters take the new nonce) and the state sets;
//   - the rendered script;
//   - "delete" of every object of the table the script does not declare:
//     the objects of removed hops (their last counters go to the
//     WithRetiredCounters hook) and of limits a hop no longer has;
//   - the state document.
//
// When nft refuses the transaction the tc additions are undone, so the
// host keeps its previous state. After it, Apply records the table's new
// fingerprint (the seal) and deletes the tc classes no longer needed.
//
// # State on the host
//
// The driver keeps no state in memory: a new instance (an Agent restart)
// reads everything from the table. Set anixops_state holds the state
// document (node_ref, generation, state_hash, digest, and per hop the
// strategy, the rendered upstreams and the rotation SetUpstreams chose) as
// base64url JSON cut into 120-character element comments (nft allows 128),
// rewritten in the same transaction as the rules it describes. Set
// anixops_seal holds the fingerprint: the SHA-256 of the normalized
// listing (handles, counter values, quota usage, dynamic set elements and
// the state sets' elements left out). A table someone damaged no longer
// matches its seal, and the next Apply repairs it.
//
// # Observe and counters
//
// Observe answers the recorded identity, the named counters of every hop
// (up: original direction, down: reply; packets and bytes) and the
// rotation. The counter epoch is the nonce of the _up and _down counters'
// comments (both, joined with a dot when they differ), so it ends exactly
// when a counter is re-created: a removed and re-added hop, a lost table,
// a reboot. active_conns and total_conns are not counted by nftables and
// stay 0.
//
// # SetUpstreams
//
// SetUpstreams rewrites the elements of the hop's balancing maps from the
// selected upstreams with the same slot layout as Render (FAILOVER keeps
// the best priority among the selected ones, so a selection without the
// primaries fails over to the backups), records the rotation in the state
// document in the same transaction, and re-seals the table. Selecting
// every upstream with weight 0 gives back the rendered elements. A family
// with no selected upstream has an empty map and its connections are
// dropped in the _dnat chain.
//
// # Bandwidth limits (tc)
//
// On each Config.LimitInterfaces device the driver owns one HTB root qdisc,
// handle Config.TCHandle (default af00:), and per rate-limited hop two
// classes at bandwidth_bps: minor 2*mark for the original direction and
// 2*mark+1 for replies, each selected by a fw filter on the packet mark the
// _acct chain sets (the hop's mark, plus Config.DirectionBit on replies,
// under MarkMask|DirectionBit). Unclassified traffic is not shaped. A root
// qdisc with another handle is foreign: an artifact with rate-limited hops
// is then ErrConflict and the qdisc is left alone. The kernel's default
// root qdisc (handle 0:) is replaced, and returns when the driver deletes
// its own (no limited hop left, Remove).
//
// # Versions
//
// The table, counter and set element comments need nft 0.9.7 and Linux
// 5.10 or later; Probe checks every feature with `nft -c` inside the
// driver's own table name (never committed) and reports the driver
// unavailable when the core ones fail. Tested with nft 1.0.9 (Ubuntu 24.04,
// CI) and nft 1.1.3 on Linux 6.12, on which re-declaring a quota updates
// its limit and keeps its usage (TestNetnsQuotaKeepsUsage).
// iproute2 before 6.3 (6.1 on Ubuntu 24.04) prints tc classes and fw
// filters as text even with -j: the driver reads classes with -j and falls
// back to the text form (comparing rates as tc prints them), and reads
// filters as text, which every version prints alike.
//
// # Names
//
// Every object of a hop is named r_<route id>_h<hop index>_<suffix>. The
// route id must be 1 to 64 ASCII letters and digits (Control's ids are
// ULIDs); any other id is rejected, never escaped. Suffixes:
//
//	_up, _down    named counters, original and reply direction
//	_quota        named quota (Limits.quota_bytes, both directions)
//	_conns        dynamic set of the hop's connection mark for ct count
//	_src4, _src6  admission sets (ingress_sources), interval sets
//	_lb4, _lb6    balancing maps: slot -> address . port, interval maps
//	_dnat         chain: admission, ct mark, DNAT (nat prerouting)
//	_acct         chain: limits and counters (filter forward)
//
// The base chains are prerouting (nat, dstnat: one rule per listener that
// jumps to the hop's _dnat chain), forward (filter: MSS clamping, then a
// verdict map from the connection mark to the hop's _acct chain) and
// postrouting (nat, srcnat: masquerade of the hops' marks).
//
// Only checked literals reach the script: the names above, numbers, and
// addresses and prefixes parsed by net/netip. Labels, host names, node
// references and the state's identity never do. Upstreams must be IP
// literals (the Agent resolves target names before Render) and targets are
// checked against the hop's target policy again.
//
// # Marks
//
// NodeHop.mark is an index from the planner, 1 up to the width of
// Config.MarkMask (4095 for the default 0x0fff0000), shifted into the mask.
// The _dnat chain sets it in the connection mark, keeping the other bits;
// the forward and postrouting chains select the hop by it. For a hop with a
// bandwidth limit the _acct chain also copies it to the packet mark, with
// Config.DirectionBit set on reply packets, for the tc classes.
//
// # Balancing
//
// Each hop has one map per address family with upstreams, of Config.Slots
// slots (default 128). The slot is "numgen inc" for ROUND_ROBIN and
// FAILOVER, "numgen random" for RANDOM and LEAST_CONN, "jhash ip saddr" or
// "jhash ip6 saddr" for IP_HASH, always modulo Slots. Slots go to
// upstreams in proportion to their weights (at least one each): one run
// per upstream for random and hash, interleaved in smooth weighted
// round-robin order for numgen inc. FAILOVER fills the map with the
// upstreams of the best (lowest) priority only. LEAST_CONN is weighted
// random until the Agent re-weights it from connection counts. Because the
// modulus is fixed, SetUpstreams changes the upstreams in rotation and
// their weights by rewriting map elements only. A family without upstreams,
// and any source an admission set does not hold, is dropped in the _dnat
// chain, so it never reaches local input on the listen port.
//
// # Paused hops
//
// A paused hop keeps every object (counters, quota, maps): its _dnat chain
// drops new connections and its _acct chain drops established ones,
// uncounted.
//
// # Hop errors
//
// A hop is rejected alone, as a *driver.HopError in a *driver.RenderError
// next to the artifact of the other hops: ErrUnsupported for what the
// configuration or nftables cannot do (a link other than RAW, a strategy,
// UDP, IPv6 or a limit that is not enabled, more upstreams than slots, an
// upstream given by name, an unknown enum value), ErrInvalidState for what
// validation should have caught (a malformed route id, no listener, port 0,
// no upstream, an address that is not a literal, a target the policy
// refuses, a relay or exit without ingress sources, a mark outside the
// mask). Hops that share a mark or an overlapping listener are all
// rejected, so the result does not depend on their order.
package nftables
