// Package nftables is the forward driver for ENGINE_NFTABLES
// (docs/architecture/forward-sdk.md section 6.1): kernel DNAT, masquerade,
// counters per direction, balancing maps, named quotas and connection
// limits in one table, "inet anixops_fwd", which is the only nftables
// object it ever touches (owner decision H13).
//
// F2b implements Engine, Capabilities (static, from Config; probing the
// host comes with F2c) and Render. Apply, Observe, SetUpstreams and Remove
// answer driver.ErrUnsupported until F2c.
//
// # Rendered script
//
// Render produces one `nft -f` script, a single transaction (golden
// examples in contracts/forward/v1/nft):
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
// re-created. The script deletes nothing. Apply (F2c) must check the
// ownership comment before running it, because step 2 would empty a foreign
// table of the same name, and deletes the objects of removed hops after
// reading their final counters. nft cannot change a table comment in place,
// so the comment is constant and the applied generation and digest are
// recorded elsewhere. A state without nftables hops renders a script of
// comments only; applying it removes the table.
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
// Config.DirectionBit set on reply packets, for the tc classes of F2c.
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
// modulus is fixed, SetUpstreams (F2c) changes the upstreams in rotation and
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
