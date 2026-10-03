package validate

import (
	"errors"
	"net/netip"
	"strings"

	"github.com/AnixOps/anix-control/sdk/forward/model"
)

// Target address errors, answered by CheckTargetAddress.
var (
	// ErrLoopbackTarget: loopback is refused under every policy.
	ErrLoopbackTarget = errors.New("loopback targets are never allowed")
	// ErrSpecialTarget: unspecified, link-local, multicast and other
	// special-purpose addresses are refused under every policy.
	ErrSpecialTarget = errors.New("unspecified, link-local, multicast and special-purpose targets are never allowed")
	// ErrPrivateTarget: a private address needs TARGET_POLICY_ALLOW_PRIVATE.
	ErrPrivateTarget = errors.New("private targets need TARGET_POLICY_ALLOW_PRIVATE on an administrator's route")
	// ErrZonedTarget: an IPv6 address with a zone names an interface of the
	// node, not a target.
	ErrZonedTarget = errors.New("an address with a zone is not a target")
)

// sharedAddressSpace is carrier-grade NAT (RFC 6598): not public, but a
// provider's internal network, so ALLOW_PRIVATE allows it as it does
// RFC 1918 and ULA addresses.
var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

// specialPurposePrefixes are special-purpose ranges that netip's
// classifications do not cover. The list mirrors the v4.1 forward target
// policy (Control's isPublicProbeAddress) so both refuse the same
// addresses; the documentation ranges stay allowed there and here.
var specialPurposePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),      // "this network"
	netip.MustParsePrefix("192.0.0.0/24"),   // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"),  // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),    // reserved, and the broadcast address
	netip.MustParsePrefix("64:ff9b::/96"),   // NAT64, which embeds an IPv4 address
	netip.MustParsePrefix("64:ff9b:1::/48"), // local-use NAT64
	netip.MustParsePrefix("2002::/16"),      // 6to4, which embeds an IPv4 address
}

// CheckTargetAddress answers whether a route under policy may reach addr,
// nil when it may. It is the check the Agent repeats on every DNS answer
// for a target name (forward-sdk.md section 14). An unspecified or unknown
// policy is TARGET_POLICY_PUBLIC_ONLY. It does not know the route's owner:
// callers refuse ALLOW_PRIVATE on a user's route first (Route does).
//
// PUBLIC_ONLY refuses loopback, private (RFC 1918, ULA), carrier-grade NAT,
// link-local, multicast, unspecified and the other special-purpose ranges,
// as the v4.1 forward path does. ALLOW_PRIVATE allows private and
// carrier-grade NAT addresses and still refuses the rest.
func CheckTargetAddress(addr netip.Addr, policy model.TargetPolicy) error {
	if addr.Zone() != "" {
		return ErrZonedTarget
	}
	addr = addr.Unmap()
	switch {
	case !addr.IsValid():
		return ErrSpecialTarget
	case addr.IsLoopback():
		return ErrLoopbackTarget
	case addr.IsUnspecified(), addr.IsLinkLocalUnicast(), addr.IsLinkLocalMulticast(),
		addr.IsInterfaceLocalMulticast(), addr.IsMulticast():
		return ErrSpecialTarget
	}
	for _, prefix := range specialPurposePrefixes {
		if prefix.Contains(addr) {
			return ErrSpecialTarget
		}
	}
	if addr.IsPrivate() || sharedAddressSpace.Contains(addr) {
		if policy == model.TargetPolicyAllowPrivate {
			return nil
		}
		return ErrPrivateTarget
	}
	return nil
}

// loopbackHostNames mean the host itself without asking DNS: "localhost"
// and its subdomains (RFC 6761), and the aliases Linux distributions put in
// /etc/hosts.
var loopbackHostNames = []string{"localhost", "localhost.localdomain", "ip6-localhost", "ip6-loopback"}

func isLoopbackHostName(name string) bool {
	for _, loopback := range loopbackHostNames {
		if name == loopback {
			return true
		}
	}
	return strings.HasSuffix(name, ".localhost")
}

// isNumericHostName reports whether name, which netip does not parse, is a
// numeric IPv4 form that inet_aton and so most resolvers accept ("127.1",
// "2130706433", "0x7f000001", "0177.0.0.1"). A DNS name's last label is
// never numeric.
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

// isHostname reports whether s is a DNS host name (RFC 1123 labels of
// letters, digits and hyphens, optionally with a final dot).
func isHostname(s string) bool {
	s = strings.TrimSuffix(s, ".")
	if s == "" || len(s) > MaxHostnameBytes {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			if !isAlnum(label[i]) && label[i] != '-' {
				return false
			}
		}
	}
	return true
}

func isAlnum(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// isHostOrAddress reports whether s is an IP address without a zone or a
// DNS host name that is not a numeric IPv4 form.
func isHostOrAddress(s string) bool {
	if addr, err := netip.ParseAddr(s); err == nil {
		return addr.Zone() == ""
	}
	return isHostname(s) && !isNumericHostName(strings.TrimSuffix(s, "."))
}

// checkTargetHost answers the code and message for a target host the
// policy refuses, or "" when it is allowed. Host names are checked by
// syntax only; the node re-checks every address they resolve to.
func checkTargetHost(host string, policy model.TargetPolicy) (Code, string) {
	if addr, err := netip.ParseAddr(host); err == nil {
		if err := CheckTargetAddress(addr, policy); err != nil {
			if errors.Is(err, ErrZonedTarget) {
				return CodeInvalidFormat, err.Error()
			}
			return CodeTargetNotAllowed, err.Error()
		}
		return "", ""
	}
	if strings.ContainsAny(host, ":[]/") {
		return CodeInvalidFormat, "must be an IP address or a DNS name, without port or brackets"
	}
	if !isHostname(host) {
		return CodeInvalidFormat, "must be an IP address or a DNS name"
	}
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	if isNumericHostName(name) {
		return CodeInvalidFormat, "numeric IPv4 forms are refused; write the address in dotted-quad form"
	}
	if isLoopbackHostName(name) {
		return CodeTargetNotAllowed, ErrLoopbackTarget.Error()
	}
	return "", ""
}
