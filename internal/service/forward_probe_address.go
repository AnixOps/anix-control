package service

import (
	"context"
	"net"
	"net/netip"
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
	if addr, err := netip.ParseAddr(host); err == nil {
		if !isPublicProbeAddress(addr) {
			return netip.Addr{}, "不能诊断内网或本机地址"
		}
		return addr.Unmap(), ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), diagnosisTimeout)
	defer cancel()
	addrs, err := probeLookup(ctx, host)
	if err != nil {
		return netip.Addr{}, err.Error()
	}
	if len(addrs) == 0 {
		return netip.Addr{}, "无法解析目标地址"
	}
	for _, addr := range addrs {
		if !isPublicProbeAddress(addr) {
			return netip.Addr{}, "不能诊断内网或本机地址"
		}
	}
	return addrs[0].Unmap(), ""
}
