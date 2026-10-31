package validate

import "testing"

func TestValidEntryPublic(t *testing.T) {
	for _, ok := range []string{"203.0.113.9", "203.0.113.9:25001", "[2001:db8::1]:443", "edge.example.net"} {
		if !validEntryPublic(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "1.2.3.4:0", "1.2.3.4:70000", "1.2.3.4:x", "[::1"} {
		if validEntryPublic(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
