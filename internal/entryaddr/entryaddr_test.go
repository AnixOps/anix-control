package entryaddr

import (
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
)

func TestParseOverride(t *testing.T) {
	for in, want := range map[string]Override{
		"203.0.113.9":        {Host: "203.0.113.9"},
		"203.0.113.9:25001":  {Host: "203.0.113.9", Port: 25001},
		"[2001:db8::1]:443":  {Host: "2001:db8::1", Port: 443},
		"edge.example.net:1": {Host: "edge.example.net", Port: 1},
	} {
		got, ok := ParseOverride(in)
		if !ok || got != want {
			t.Errorf("ParseOverride(%q) = %+v, %v; want %+v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "1.2.3.4:0", "1.2.3.4:70000", "1.2.3.4:x", "[::1", "bad host"} {
		if _, ok := ParseOverride(in); ok {
			t.Errorf("ParseOverride(%q) accepted", in)
		}
	}
}

func TestResolveOrder(t *testing.T) {
	nat := []model.NodeEntry{{Kind: model.NodeEntryNAT, Enabled: true, Host: "198.51.100.7", PortMap: `[{"from":24000,"to":24010,"external":31000}]`}}
	labels := map[string]string{LabelKey("proxy-1"): "203.0.113.9:25001"}
	if h, p, s := Resolve(labels, "proxy-1", "10.0.0.2", 24005, nat); h != "203.0.113.9" || p != 25001 || s != SourceRoute {
		t.Errorf("route override: %s %d %s", h, p, s)
	}
	if h, p, s := Resolve(nil, "proxy-1", "10.0.0.2", 24005, nat); h != "198.51.100.7" || p != 31005 || s != SourceNAT {
		t.Errorf("nat: %s %d %s", h, p, s)
	}
	if h, p, s := Resolve(nil, "proxy-1", "10.0.0.2", 30000, nat); h != "10.0.0.2" || p != 30000 || s != SourceNode {
		t.Errorf("fallback: %s %d %s", h, p, s)
	}
	if h, p, _ := Resolve(map[string]string{LabelKey("proxy-1"): "203.0.113.9"}, "proxy-1", "x", 24005, nat); h != "203.0.113.9" || p != 24005 {
		t.Errorf("host only: %s %d", h, p)
	}
}
