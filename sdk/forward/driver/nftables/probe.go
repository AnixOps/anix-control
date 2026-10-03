package nftables

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// ProbeReport is what Probe found besides the configuration.
type ProbeReport struct {
	// Missing lists the features the host lacks, with the reason, as
	// "feature: reason".
	Missing []string
	// Warnings are host conditions the driver does not change but an
	// operator should know about: another table's forward chain that drops
	// by default (Docker's "ip filter FORWARD" does), which drops the
	// driver's forwarded packets unless that table accepts them.
	Warnings []string
}

// nftVersion matches `nft --version`: "nftables v1.0.9 (Old Doc Yak #3)".
var nftVersion = regexp.MustCompile(`nftables v([0-9]+(?:\.[0-9]+)*)`)

// probeTable is the name every check snippet uses: the driver's own table.
// The snippets run with `nft -c` only, which never commits, so they create
// nothing, even when the table does not exist yet.
const probeTable = "table inet " + Table

// probes are the kernel features the driver checks with `nft -c`, each a
// snippet inside the driver's own table. "core" is what every artifact
// needs (table, counter and set element comments: nft 0.9.7 and Linux 5.10
// or later; DNAT through a map; the conntrack mark); without it the driver
// is unavailable.
var probes = []struct {
	name, body string
}{
	{"core", `
	comment "probe"
	counter probe_up { comment "probe" }
	set probe_state { type mark; elements = { 0 comment "probe" } }
	map probe_lb4 { type mark : ipv4_addr . inet_service; flags interval; elements = { 0-1 : 192.0.2.1 . 1 } }
	chain probe_pre { type nat hook prerouting priority dstnat; policy accept; tcp dport 1 fib daddr type local ct mark set ct mark and 0xf000ffff or 0x00010000 meta nfproto ipv4 meta l4proto tcp dnat ip to numgen inc mod 2 map @probe_lb4; }
	chain probe_fwd { type filter hook forward priority filter; policy accept; ct mark and 0x0fff0000 vmap { 0x00010000 : accept }; ct direction original counter name "probe_up" meta mark set meta mark and 0xf000fffe or 0x00010000; }
	chain probe_post { type nat hook postrouting priority srcnat; policy accept; ct mark and 0x0fff0000 { 0x00010000 } masquerade; }`},
	{"random", `
	map probe_lb4 { type mark : ipv4_addr . inet_service; flags interval; }
	chain probe_pre { type nat hook prerouting priority dstnat; policy accept; meta nfproto ipv4 meta l4proto tcp dnat ip to numgen random mod 2 map @probe_lb4; }`},
	{"ip_hash", `
	map probe_lb4 { type mark : ipv4_addr . inet_service; flags interval; }
	chain probe_pre { type nat hook prerouting priority dstnat; policy accept; meta nfproto ipv4 meta l4proto tcp dnat ip to jhash ip saddr mod 2 seed 0x0 map @probe_lb4; }`},
	{"ipv6", `
	map probe_lb6 { type mark : ipv6_addr . inet_service; flags interval; elements = { 0-1 : 2001:db8::1 . 1 } }
	chain probe_pre { type nat hook prerouting priority dstnat; policy accept; meta nfproto ipv6 meta l4proto tcp dnat ip6 to numgen inc mod 2 map @probe_lb6; }`},
	{"udp", `
	map probe_lb4 { type mark : ipv4_addr . inet_service; flags interval; }
	chain probe_pre { type nat hook prerouting priority dstnat; policy accept; meta l4proto { tcp, udp } th dport 1 meta nfproto ipv4 meta l4proto { tcp, udp } dnat ip to numgen inc mod 2 map @probe_lb4; }`},
	{"quota", `
	quota probe_quota { over 1000 bytes }
	chain probe_fwd { type filter hook forward priority filter; policy accept; quota name "probe_quota" drop; }`},
	{"ct_count", `
	set probe_conns { type mark; flags dynamic; }
	chain probe_fwd { type filter hook forward priority filter; policy accept; ct state new add @probe_conns { ct mark ct count over 10 } reject; }`},
	{"mss", `
	chain probe_fwd { type filter hook forward priority filter; policy accept; tcp flags & (syn | rst) == syn tcp option maxseg size set rt mtu; }`},
}

// Probe checks the host the runner reaches and answers base with what the
// host offers: Version set (or empty with Unavailable saying why: nft
// missing, no CAP_NET_ADMIN, a kernel or nft too old), and IPv6, UDP,
// Strategies, Quota, MaxConns and BandwidthLimit turned off where the host
// lacks them (bandwidth limits need tc and Config.LimitInterfaces). It
// never changes the host: feature checks run with `nft -c` inside the
// driver's own table name, and the forward chains of other tables are only
// read. The Agent probes once at start and passes the result to New, so
// Render stays a function of the configuration. Only a done context is an
// error.
func Probe(ctx context.Context, r Runner, base Config) (Config, *ProbeReport, error) {
	cfg := base
	cfg.Strategies = slices.Clone(base.Strategies)
	rep := &ProbeReport{}
	unavailable := func(format string, args ...any) (Config, *ProbeReport, error) {
		cfg.Version = ""
		cfg.Unavailable = clip(fmt.Sprintf(format, args...))
		return cfg, rep, nil
	}
	if err := ctx.Err(); err != nil {
		return base, nil, err
	}
	out, err := r.Run(ctx, "nft", []string{"--version"}, nil)
	if err := ctx.Err(); err != nil {
		return base, nil, err
	}
	if err != nil {
		if missingBinary(err) {
			return unavailable("nft is not installed")
		}
		return unavailable("nft --version: %v", err)
	}
	m := nftVersion.FindSubmatch(out)
	if m == nil {
		return unavailable("nft --version printed no version: %q", clip(strings.TrimSpace(string(out))))
	}
	version := "nft " + string(m[1])

	out, err = r.Run(ctx, "nft", []string{"-j", "list", "chains"}, nil)
	if err := ctx.Err(); err != nil {
		return base, nil, err
	}
	if err != nil {
		if notPermitted(err) {
			return unavailable("nft cannot read the ruleset: the Agent needs CAP_NET_ADMIN (%s)", firstLine(stderrOf(err)))
		}
		return unavailable("nft -j list chains: %v", err)
	}
	if l, err := parseListing(out); err == nil {
		for _, it := range l.items {
			if it.kind != "chain" || it.str("hook") != "forward" || it.str("policy") != "drop" {
				continue
			}
			if it.family() == Family && it.tableName() == Table {
				continue
			}
			rep.Warnings = append(rep.Warnings, fmt.Sprintf(
				"table %s %s chain %s drops forwarded packets by default (Docker's FORWARD chain does); the driver does not change it, so forwarded connections need that table to accept them",
				it.family(), it.tableName(), it.str("name")))
		}
	}

	failed := map[string]string{}
	for _, p := range probes {
		script := probeTable + " {" + p.body + "\n}\n"
		if _, err := r.Run(ctx, "nft", []string{"-c", "-f", "-"}, []byte(script)); err != nil {
			if err := ctx.Err(); err != nil {
				return base, nil, err
			}
			failed[p.name] = firstLine(stderrOf(err))
			if failed[p.name] == "" {
				failed[p.name] = err.Error()
			}
			rep.Missing = append(rep.Missing, p.name+": "+failed[p.name])
		}
	}
	if reason, ok := failed["core"]; ok {
		return unavailable("%s: the kernel or nft lacks what every artifact needs (nft 0.9.7 and Linux 5.10 or later; tested with nft 1.0.9 and 1.1.3): %s", version, reason)
	}
	drop := func(ss ...forwardv1.BalanceStrategy) {
		cfg.Strategies = slices.DeleteFunc(cfg.Strategies, func(s forwardv1.BalanceStrategy) bool { return slices.Contains(ss, s) })
	}
	if _, ok := failed["random"]; ok {
		drop(forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM, forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN)
	}
	if _, ok := failed["ip_hash"]; ok {
		drop(forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH)
	}
	if _, ok := failed["ipv6"]; ok {
		cfg.IPv6 = false
	}
	if _, ok := failed["udp"]; ok {
		cfg.UDP = false
	}
	if _, ok := failed["quota"]; ok {
		cfg.Quota = false
	}
	if _, ok := failed["ct_count"]; ok {
		cfg.MaxConns = false
	}
	if _, ok := failed["mss"]; ok && len(cfg.MSSClampInterfaces) > 0 {
		rep.Warnings = append(rep.Warnings, "MSS clamping is not available; Config.MSSClampInterfaces dropped")
		cfg.MSSClampInterfaces = nil
	}
	if cfg.BandwidthLimit {
		switch {
		case len(cfg.LimitInterfaces) == 0:
			cfg.BandwidthLimit = false
			rep.Missing = append(rep.Missing, "bandwidth_limit: no Config.LimitInterfaces")
		default:
			if _, err := r.Run(ctx, "tc", []string{"-V"}, nil); err != nil {
				if err := ctx.Err(); err != nil {
					return base, nil, err
				}
				cfg.BandwidthLimit = false
				rep.Missing = append(rep.Missing, "bandwidth_limit: tc: "+clip(err.Error()))
			}
		}
	}
	cfg.Version = version
	cfg.Unavailable = ""
	return cfg, rep, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return clip(s)
}

// clip keeps printable ASCII and at most 200 bytes, for Config's text
// fields.
func clip(s string) string {
	b := []byte(s)
	out := b[:0]
	for _, c := range b {
		if c >= ' ' && c <= '~' {
			out = append(out, c)
		}
	}
	if len(out) > 200 {
		out = out[:200]
	}
	return string(out)
}
