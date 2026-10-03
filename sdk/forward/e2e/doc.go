// Package e2e is the forwarding SDK's multi-namespace end-to-end suite
// (docs/architecture/forward-sdk.md section 13, F2d). It has no API: its
// tests build networks of Linux network namespaces joined by veth pairs
// (a client, an entry node, relay nodes and target hosts, IPv4 and IPv6)
// and drive them the way Control and the Agents will: routes from
// sdk/forward/model, checked by sdk/forward/validate against an inventory
// of the namespace nodes with the capabilities their drivers probed,
// planned by sdk/forward/planner and stamped with generations, then
// rendered and applied by the nftables driver inside each node's
// namespace. Real TCP and UDP traffic between the client and echo servers
// in the target namespaces then checks forwarding, counters, balancing,
// failover, admission, limits, pausing, Agent restarts and removal. The
// TestGost tests put a gost entry in the star instead (the gost driver,
// gost running in the entry's namespace): exact payload counters and
// failover through gost's web API (F4b; mixed-engine chains are F4c).
//
// The tests need root (CAP_NET_ADMIN, and CAP_SYS_ADMIN for `ip netns
// add`), nft, tc, iproute2 and gost (ANIXOPS_GOST_BIN, or gost on PATH),
// and run only with ANIXOPS_FORWARD_E2E=1; otherwise they skip. They never
// touch the host's own ruleset, qdiscs, sysctls or interfaces. Run them
// with
//
//	go -C sdk test -c -o /tmp/forward-e2e.test ./forward/e2e
//	sudo ANIXOPS_FORWARD_E2E=1 ANIXOPS_GOST_BIN=/path/to/gost /tmp/forward-e2e.test -test.v
//
// CI runs them in the "Forward Netns E2E" job.
package e2e
