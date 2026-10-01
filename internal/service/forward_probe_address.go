package service

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
)

// probeLookup resolves a probe target's host name; tests replace it.
var probeLookup = func(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// nonPublicProbePrefixes are special-purpose ranges that netip's
// classifications below do not cover.
var nonPublicProbePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),      // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),  // shared address space (carrier-grade NAT)
	netip.MustParsePrefix("192.0.0.0/24"),   // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"),  // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),    // reserved, and the broadcast address
	netip.MustParsePrefix("64:ff9b::/96"),   // NAT64, which embeds an IPv4 address
	netip.MustParsePrefix("64:ff9b:1::/48"), // local-use NAT64
	netip.MustParsePrefix("2002::/16"),      // 6to4, which embeds an IPv4 address
}

// isPublicProbeAddress reports whether Control may connect to addr on a
// user's behalf: not loopback, private, link-local, multicast, unspecified
// or another special-purpose address.
func isPublicProbeAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.Zone() != "" || addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() || addr.IsMulticast() {
		return false
	}
	for _, prefix := range nonPublicProbePrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

var (
	// errNotPublicAddress: the host is, or resolves to, an address that is
	// not public (isPublicProbeAddress).
	errNotPublicAddress = errors.New("not a public address")
	// errNoAddress: the host name resolved to no address.
	errNoAddress = errors.New("no address")
)

// loopbackHostNames are names that mean the host itself without asking DNS:
// "localhost" and its subdomains (RFC 6761), and the loopback aliases that
// Linux distributions put in /etc/hosts.
var loopbackHostNames = []string{"localhost", "localhost.localdomain", "ip6-localhost", "ip6-loopback"}

// isLoopbackHostName reports whether name means the host itself.
func isLoopbackHostName(name string) bool {
	for _, loopback := range loopbackHostNames {
		if name == loopback {
			return true
		}
	}
	return strings.HasSuffix(name, ".localhost")
}

// isNumericHostName reports whether name, which is not an address netip
// parses, is a numeric IPv4 form that inet_aton and so most resolvers
// accept: "127.1", "2130706433", "0x7f000001" or "0177.0.0.1". A DNS name's
// last label is never numeric.
func isNumericHostName(name string) bool {
	last := name[strings.LastIndex(name, ".")+1:]
	if last == "" {
		return false
	}
	if digits := strings.TrimPrefix(strings.TrimPrefix(last, "0x"), "0X"); digits != last {
		return strings.Trim(digits, "0123456789abcdefABCDEF") == ""
	}
	return strings.Trim(last, "0123456789") == ""
}

// probeLookupFunc resolves a host name to its addresses.
type probeLookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)

// resolvePublicAddress returns the public address host names: host itself
// when it is an address, else the first address it resolves to. It fails
// with errNotPublicAddress when host is, or resolves to any, address that
// is not public, including loopback names and numeric IPv4 forms, which it
// refuses without asking DNS.
func resolvePublicAddress(host string) (netip.Addr, error) {
	return resolvePublicAddressWith(context.Background(), probeLookup, host)
}

// resolvePublicAddressWith is resolvePublicAddress with lookup asking DNS,
// for at most diagnosisTimeout and until ctx ends.
func resolvePublicAddressWith(ctx context.Context, lookup probeLookupFunc, host string) (netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		if !isPublicProbeAddress(addr) {
			return netip.Addr{}, errNotPublicAddress
		}
		return addr.Unmap(), nil
	}
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	if isLoopbackHostName(name) || isNumericHostName(name) {
		return netip.Addr{}, errNotPublicAddress
	}
	ctx, cancel := context.WithTimeout(ctx, diagnosisTimeout)
	defer cancel()
	addrs, err := lookup(ctx, host)
	if err != nil {
		return netip.Addr{}, err
	}
	if len(addrs) == 0 {
		return netip.Addr{}, errNoAddress
	}
	for _, addr := range addrs {
		if !isPublicProbeAddress(addr) {
			return netip.Addr{}, errNotPublicAddress
		}
	}
	return addrs[0].Unmap(), nil
}

// publicProbeAddress resolves the host of a probe that Control runs for a
// user (a forward's diagnosis) and returns the address to connect to, or
// why the probe is refused.
//
// A forward's target is the user's to choose, and the diagnosis connects to
// it from Control: without this check a user could reach Control's own
// network (its database, metadata service or loopback services) and read
// from the answer which ports are open. Every address the name resolves to
// must be public, and the probe connects to the checked address, so the
// name cannot resolve to another one between the check and the connection.
func publicProbeAddress(host string) (netip.Addr, string) {
	return publicProbeAddressWith(context.Background(), probeLookup, host)
}

// publicProbeAddressWith is publicProbeAddress with lookup asking DNS, until
// ctx ends.
func publicProbeAddressWith(ctx context.Context, lookup probeLookupFunc, host string) (netip.Addr, string) {
	addr, err := resolvePublicAddressWith(ctx, lookup, host)
	switch {
	case err == nil:
		return addr, ""
	case errors.Is(err, errNotPublicAddress):
		return netip.Addr{}, "不能诊断内网或本机地址"
	case errors.Is(err, errNoAddress):
		return netip.Addr{}, "无法解析目标地址"
	default:
		return netip.Addr{}, err.Error()
	}
}
