package nftables

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// Conflicts with foreign nftables rules. Before an apply that changes the
// host, Apply reads the whole ruleset (read only: it never changes another
// table) and refuses with ErrConflict when a rule of another table both
// rewrites destinations (dnat, redirect, tproxy, or iptables-nft's DNAT and
// REDIRECT targets) and matches the destination port of one of the
// artifact's listeners (a number, an anonymous set or a range). Named sets
// and rules nft cannot decode are not followed, and a process bound to the
// port is not detected: the planner reserves the node's own ports.

// checkConflicts answers ErrConflict for the first foreign rule that
// claims one of the manifest's listeners.
func (d *Driver) checkConflicts(ctx context.Context, m *manifest) error {
	if m == nil || len(m.hops) == 0 {
		return nil
	}
	out, err := d.run(ctx, "nft", nil, "-j", "list", "ruleset")
	if err != nil {
		return err
	}
	l, err := parseListing(out)
	if err != nil {
		return err
	}
	for _, it := range l.items {
		if it.kind != "rule" || (it.family() == Family && it.tableName() == Table) {
			continue
		}
		exprs, _ := it.fields["expr"].([]any)
		if !rewritesDestination(exprs) {
			continue
		}
		for _, port := range destinationPorts(exprs) {
			for _, h := range m.hops {
				if port.covers(h) {
					return fmt.Errorf("%w: table %s %s chain %s rewrites destination port %d, which hop %s listens on",
						driver.ErrConflict, it.family(), it.tableName(), it.str("chain"), h.port, h.key)
				}
			}
		}
	}
	return nil
}

// rewritesDestination reports whether a rule's statements rewrite the
// destination.
func rewritesDestination(exprs []any) bool {
	for _, e := range exprs {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		for _, k := range []string{"dnat", "redirect", "tproxy"} {
			if _, ok := m[k]; ok {
				return true
			}
		}
		if xt, ok := m["xt"].(map[string]any); ok {
			if name, _ := xt["name"].(string); name == "DNAT" || name == "REDIRECT" || name == "TPROXY" {
				return true
			}
		}
	}
	return false
}

// portMatch is a destination port match: protocol "tcp", "udp" or "th"
// (either), and the ports it accepts.
type portMatch struct {
	proto  string
	lo, hi uint64
}

func (p portMatch) covers(h *manifestHopInfo) bool {
	if uint64(h.port) < p.lo || uint64(h.port) > p.hi {
		return false
	}
	switch p.proto {
	case "tcp":
		return h.tcp
	case "udp":
		return h.udp
	}
	return true
}

// destinationPorts answers the rule's destination port matches.
func destinationPorts(exprs []any) []portMatch {
	var out []portMatch
	for _, e := range exprs {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		match, ok := m["match"].(map[string]any)
		if !ok {
			continue
		}
		if op, _ := match["op"].(string); op != "==" && op != "in" {
			continue
		}
		left, _ := match["left"].(map[string]any)
		payload, _ := left["payload"].(map[string]any)
		if field, _ := payload["field"].(string); field != "dport" {
			continue
		}
		proto, _ := payload["protocol"].(string)
		for _, r := range portRanges(match["right"]) {
			out = append(out, portMatch{proto: proto, lo: r[0], hi: r[1]})
		}
	}
	return out
}

// portRanges reads a match's right side: a number, a range, or an
// anonymous set of them.
func portRanges(v any) [][2]uint64 {
	switch x := v.(type) {
	case json.Number:
		if n, err := strconv.ParseUint(x.String(), 10, 16); err == nil {
			return [][2]uint64{{n, n}}
		}
	case map[string]any:
		if r, ok := x["range"].([]any); ok && len(r) == 2 {
			lo, hi := portRanges(r[0]), portRanges(r[1])
			if len(lo) == 1 && len(hi) == 1 {
				return [][2]uint64{{lo[0][0], hi[0][1]}}
			}
		}
		if s, ok := x["set"].([]any); ok {
			var out [][2]uint64
			for _, e := range s {
				out = append(out, portRanges(e)...)
			}
			return out
		}
	}
	return nil
}
