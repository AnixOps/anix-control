package link

import (
	"cmp"
	"fmt"
	"net/netip"
	"slices"
)

// Sources is the set of addresses a listener admits: the ingress_sources of a
// hop, every address of the previous hop's nodes. It is checked on the raw
// accepted connection, before any handshake bytes are processed
// (anixops-protocol.md section 2.5). The empty set admits nobody.
type Sources struct {
	prefixes []netip.Prefix
}

// ParseSources parses addresses and prefixes ("192.0.2.7", "198.51.100.0/24",
// "2001:db8::/32"). IPv4-mapped IPv6 addresses count as IPv4, zones are
// refused, and at least one entry is required: a listener open to nobody is a
// configuration error, not a state.
func ParseSources(items []string) (Sources, error) {
	if len(items) == 0 {
		return Sources{}, fmt.Errorf("link: no ingress sources: a relay or exit listener admits only the previous hop's addresses")
	}
	var ps []netip.Prefix
	for _, s := range items {
		p, err := parseSource(s)
		if err != nil {
			return Sources{}, err
		}
		ps = append(ps, p)
	}
	return Sources{prefixes: minimalPrefixes(ps)}, nil
}

func parseSource(s string) (netip.Prefix, error) {
	if a, err := netip.ParseAddr(s); err == nil {
		if a.Zone() != "" {
			return netip.Prefix{}, fmt.Errorf("link: source %q has a zone", s)
		}
		a = a.Unmap()
		return netip.PrefixFrom(a, a.BitLen()), nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil || p.Addr().Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("link: source %q is not an IP address or prefix", s)
	}
	if p.Addr().Is4In6() {
		if p.Bits() < 96 {
			return netip.Prefix{}, fmt.Errorf("link: source %q is not an IP address or prefix", s)
		}
		p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
	}
	return p.Masked(), nil
}

// minimalPrefixes sorts prefixes and drops the ones another covers.
func minimalPrefixes(ps []netip.Prefix) []netip.Prefix {
	slices.SortFunc(ps, func(a, b netip.Prefix) int {
		return cmp.Or(a.Addr().Compare(b.Addr()), cmp.Compare(a.Bits(), b.Bits()))
	})
	var out []netip.Prefix
	for _, p := range ps {
		if n := len(out); n > 0 && out[n-1].Contains(p.Addr()) && out[n-1].Bits() <= p.Bits() {
			continue
		}
		out = append(out, p)
	}
	return out
}

// Contains reports whether a is admitted. An IPv4-mapped address is checked
// as IPv4 and a zone is ignored.
func (s Sources) Contains(a netip.Addr) bool {
	a = a.Unmap().WithZone("")
	for _, p := range s.prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Prefixes returns the admitted prefixes, sorted, without redundant ones.
func (s Sources) Prefixes() []netip.Prefix { return slices.Clone(s.prefixes) }
