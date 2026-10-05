package link

import (
	"net/netip"
	"testing"
)

func TestParseSources(t *testing.T) {
	s, err := ParseSources([]string{"192.0.2.7", "198.51.100.0/24", "198.51.100.9", "2001:db8::/32", "::ffff:203.0.113.5", "::ffff:10.0.0.0/104"})
	if err != nil {
		t.Fatal(err)
	}
	in := []string{"192.0.2.7", "198.51.100.200", "198.51.100.9", "2001:db8:ffff::1", "203.0.113.5", "::ffff:203.0.113.5", "10.1.2.3", "::ffff:10.9.9.9"}
	out := []string{"192.0.2.8", "198.51.101.1", "2001:db9::1", "203.0.113.6", "10.0.0.0/8", "", "fe80::1"}
	for _, a := range in {
		if addr, err := netip.ParseAddr(a); err != nil || !s.Contains(addr) {
			t.Errorf("%s not admitted", a)
		}
	}
	for _, a := range out {
		if addr, err := netip.ParseAddr(a); err == nil && s.Contains(addr) {
			t.Errorf("%s admitted", a)
		}
	}
	if s.Contains(netip.Addr{}) {
		t.Error("the zero address is admitted")
	}
	// A zone does not widen admission.
	if z := netip.MustParseAddr("fe80::1%eth0"); s.Contains(z) {
		t.Error("a zoned link-local address is admitted")
	}
	// redundant entries are dropped: 198.51.100.9 is inside 198.51.100.0/24
	for _, p := range s.Prefixes() {
		if p.String() == "198.51.100.9/32" {
			t.Errorf("redundant prefix kept: %v", s.Prefixes())
		}
	}
	for _, bad := range [][]string{nil, {}, {""}, {"example.com"}, {"192.0.2.0/33"}, {"fe80::1%eth0"}, {"192.0.2.1", "nope"}, {"::ffff:10.0.0.0/64"}, {"1.2.3.4/8/9"}} {
		if _, err := ParseSources(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	// A prefix is masked to its network address.
	m, err := ParseSources([]string{"192.0.2.77/24"})
	if err != nil || m.Prefixes()[0].String() != "192.0.2.0/24" {
		t.Fatalf("%v %v", m.Prefixes(), err)
	}
}

func TestAddrPort(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.1:80":       "127.0.0.1:80",
		"[::ffff:1.2.3.4]:5": "1.2.3.4:5",
		"[2001:db8::1]:443":  "[2001:db8::1]:443",
	} {
		a, err := netip.ParseAddrPort(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := addrPort(tcpAddrOf(a)); got.String() != want {
			t.Errorf("%s -> %v, want %s", in, got, want)
		}
	}
	if (addrPort(nil) != netip.AddrPort{}) {
		t.Error("nil address")
	}
}
