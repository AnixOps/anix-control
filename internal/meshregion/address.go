package meshregion

import (
	"encoding/binary"
	"net/netip"
	"regexp"
	"strings"
)

// privatePrefixes are the IPv4 ranges a region, a host group or a device
// address may use (R17): RFC 1918 and the shared address space.
var privatePrefixes = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("100.64.0.0/10"),
}

// isPrivatePrefix tells whether prefix lies inside one private range.
func isPrivatePrefix(prefix netip.Prefix) bool {
	for _, private := range privatePrefixes {
		if private.Bits() <= prefix.Bits() && private.Contains(prefix.Addr()) {
			return true
		}
	}
	return false
}

// isPrivateAddr tells whether addr lies inside a private range.
func isPrivateAddr(addr netip.Addr) bool {
	for _, private := range privatePrefixes {
		if private.Contains(addr) {
			return true
		}
	}
	return false
}

// parsePrefix reads an IPv4 range in its canonical form; the address must
// be the range's first address.
func parsePrefix(field, raw string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
	if err != nil || !prefix.Addr().Is4() {
		return netip.Prefix{}, invalid("%s: an IPv4 range such as 10.121.0.0/16", field)
	}
	if prefix.Masked() != prefix {
		return netip.Prefix{}, invalid("%s: %s is not the start of its range; %s", field, prefix, prefix.Masked())
	}
	return prefix, nil
}

// parsePrivatePrefix is parsePrefix limited to the private ranges and to
// ranges of 8 to 30 bits.
func parsePrivatePrefix(field, raw string) (netip.Prefix, error) {
	prefix, err := parsePrefix(field, raw)
	if err != nil {
		return prefix, err
	}
	if prefix.Bits() < 8 || prefix.Bits() > 30 {
		return netip.Prefix{}, invalid("%s: a prefix length of 8 to 30", field)
	}
	if !isPrivatePrefix(prefix) {
		return netip.Prefix{}, invalid("%s: a private range (10/8, 172.16/12, 192.168/16 or 100.64/10)", field)
	}
	return prefix, nil
}

// mustPrefix reads a stored range; stored ranges were checked on the way
// in.
func mustPrefix(raw string) netip.Prefix {
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		return netip.Prefix{}
	}
	return prefix
}

// parseAddr reads an IPv4 address.
func parseAddr(field, raw string) (netip.Addr, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil || !addr.Is4() {
		return netip.Addr{}, invalid("%s: an IPv4 address", field)
	}
	return addr, nil
}

// lastAddr answers the last address of an IPv4 range.
func lastAddr(prefix netip.Prefix) netip.Addr {
	host := uint32(1)<<(32-prefix.Bits()) - 1
	return addrFromUint(addrToUint(prefix.Masked().Addr()) | host)
}

// addrToUint answers an IPv4 address as a big-endian number.
func addrToUint(addr netip.Addr) uint32 {
	bytes := addr.As4()
	return binary.BigEndian.Uint32(bytes[:])
}

// addrFromUint answers the IPv4 address of a big-endian number.
func addrFromUint(value uint32) netip.Addr {
	var bytes [4]byte
	binary.BigEndian.PutUint32(bytes[:], value)
	return netip.AddrFrom4(bytes)
}

// usableIn tells whether addr may be a member's address in a region with
// subnet: inside it, and neither its network nor its broadcast address.
func usableIn(subnet netip.Prefix, addr netip.Addr) bool {
	return subnet.Contains(addr) && addr != subnet.Masked().Addr() && addr != lastAddr(subnet)
}

var (
	labelPattern  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	codePattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,14}[a-z0-9]$|^[a-z]$`)
	suffixPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

// normalizeLabel reads a host or device name as one DNS label: lowercase
// ASCII letters, digits and inner hyphens, at most 63 bytes.
func normalizeLabel(field, raw string) (string, error) {
	label := strings.ToLower(strings.TrimSpace(raw))
	if !labelPattern.MatchString(label) {
		return "", invalid("%s: letters, digits and inner hyphens, 1 to 63 characters (one DNS label)", field)
	}
	return label, nil
}

// normalizeCode reads a region code: a lowercase DNS label of 1 to 16
// characters that starts with a letter.
func normalizeCode(raw string) (string, error) {
	code := strings.ToLower(strings.TrimSpace(raw))
	if !codePattern.MatchString(code) {
		return "", invalid("code: 1 to 16 lowercase letters, digits and inner hyphens, starting with a letter")
	}
	return code, nil
}

// normalizeSuffix reads the intranet domain suffix: DNS labels, at most 200
// bytes, without the leading or trailing dot.
func normalizeSuffix(raw string) (string, error) {
	suffix := strings.Trim(strings.ToLower(strings.TrimSpace(raw)), ".")
	if suffix == "" {
		suffix = DefaultNameSuffix
	}
	if len(suffix) > 200 || !suffixPattern.MatchString(suffix) {
		return "", invalid("name_suffix: DNS labels separated by dots, such as mesh.internal")
	}
	return suffix, nil
}
