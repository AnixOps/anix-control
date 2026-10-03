package gost

import (
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

func TestSpread(t *testing.T) {
	for _, c := range []struct {
		weights []uint32
		max     int
		want    []int
	}{
		{[]uint32{1}, 128, []int{0}},
		{[]uint32{1, 1, 1}, 128, []int{0, 1, 2}},
		{[]uint32{4, 4}, 128, []int{0, 1}},
		{[]uint32{1, 2, 3}, 128, []int{2, 1, 0, 2, 1, 2}},
		{[]uint32{5, 1, 1}, 128, []int{0, 0, 1, 0, 2, 0, 0}},
		{[]uint32{3, 1}, 128, []int{0, 0, 1, 0}},
	} {
		if got := spread(c.weights, c.max); !slices.Equal(got, c.want) {
			t.Errorf("spread(%v) = %v, want %v", c.weights, got, c.want)
		}
	}
	// Scaled down to about max entries, each upstream at least once, in
	// proportion.
	got := spread([]uint32{1000, 1, 999}, 128)
	count := make([]int, 3)
	for _, i := range got {
		count[i]++
	}
	if len(got) > 130 || count[1] != 1 || count[0] < 60 || count[2] < 60 {
		t.Fatalf("scaled spread: %d entries, counts %v", len(got), count)
	}
}

func TestParseSS(t *testing.T) {
	out := []byte(`tcp   LISTEN 0      4096         0.0.0.0:22         0.0.0.0:*
tcp   LISTEN 0      4096            [::]:30001          [::]:*
tcp   LISTEN 0      4096               *:30002             *:*
udp   UNCONN 0      0          127.0.0.53%lo:53         0.0.0.0:*
udp   UNCONN 0      0      [2001:db8::11]:30003          [::]:*
tcp   LISTEN 0      4096     172.16.5.31:20000       0.0.0.0:*    users:(("gost",pid=1,fd=3))
`)
	socks, err := parseSS(out)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tcp :22", "tcp :30001", "tcp :30002", "udp 127.0.0.53:53", "udp 2001:db8::11:30003", "tcp 172.16.5.31:20000"}
	var got []string
	for _, s := range socks {
		a := ""
		if s.addr.IsValid() {
			a = s.addr.String()
		}
		got = append(got, s.network+" "+a+":"+strconv.FormatUint(uint64(s.port), 10))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("parseSS\n got  %q\n want %q", got, want)
	}
	for _, bad := range []string{"tcp LISTEN 0 1\n", "tcp LISTEN 0 1 nocolon *:*\n", "tcp LISTEN 0 1 1.2.3.4:http *:*\n", "tcp LISTEN 0 1 [zz]:1 *:*\n"} {
		if _, err := parseSS([]byte(bad)); err == nil {
			t.Errorf("parseSS(%q) accepted", bad)
		}
	}
}

func TestConflictsAndBound(t *testing.T) {
	socks, err := parseSS([]byte("tcp LISTEN 0 1 *:30001 *:*\nudp UNCONN 0 0 10.0.0.1:30002 *:*\n"))
	if err != nil {
		t.Fatal(err)
	}
	l := func(network, addr string, port uint32) manifestListener {
		return manifestListener{Network: network, Address: addr, Port: port}
	}
	for _, c := range []struct {
		name       string
		want, ours []manifestListener
		conflict   bool
	}{
		{"free port", []manifestListener{l("tcp", "", 30003)}, nil, false},
		{"other network", []manifestListener{l("udp", "", 30001)}, nil, false},
		{"wildcard holds every address", []manifestListener{l("tcp", "10.0.0.9", 30001)}, nil, true},
		{"held by our configuration", []manifestListener{l("tcp", "", 30001)}, []manifestListener{l("tcp", "", 30001)}, false},
		{"other address", []manifestListener{l("udp", "10.0.0.2", 30002)}, nil, false},
		{"same address", []manifestListener{l("udp", "10.0.0.1", 30002)}, nil, true},
		{"wildcard wants a held address", []manifestListener{l("udp", "", 30002)}, nil, true},
	} {
		err := conflicts(socks, c.want, c.ours)
		if c.conflict != errors.Is(err, driver.ErrConflict) || !c.conflict && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if !bound(socks, []manifestListener{l("tcp", "", 30001), l("udp", "10.0.0.1", 30002)}) {
		t.Error("bound listeners not found")
	}
	if bound(socks, []manifestListener{l("udp", "", 30002)}) || bound(socks, []manifestListener{l("tcp", "", 30004)}) {
		t.Error("unbound listener found")
	}
}

func TestParseContentRefusesForeignContent(t *testing.T) {
	for name, content := range map[string]string{
		"not json":         "nope",
		"no manifest":      `{"services": []}`,
		"another driver":   `{"anixops": {"driver": "x", "hops": []}}`,
		"hops differ":      `{"anixops": {"driver": "` + OwnerMark + `", "hops": []}}`,
		"bad route":        `{"anixops": {"driver": "` + OwnerMark + `", "hops": [{"route": "a b", "hop": 0, "listeners": [{"network": "tcp", "port": 1}], "upstreams": [{"address": "192.0.2.1", "port": 1}]}]}}`,
		"bad listener":     `{"anixops": {"driver": "` + OwnerMark + `", "hops": [{"route": "A", "hop": 0, "listeners": [{"network": "sctp", "port": 1}], "upstreams": [{"address": "192.0.2.1", "port": 1}]}]}}`,
		"no metrics path":  `{"anixops": {"driver": "` + OwnerMark + `", "hops": [{"route": "A", "hop": 0, "listeners": [{"network": "tcp", "port": 1}], "upstreams": [{"address": "192.0.2.1", "port": 1}]}]}}`,
		"listener address": `{"anixops": {"driver": "` + OwnerMark + `", "hops": [{"route": "A", "hop": 0, "listeners": [{"network": "tcp", "address": "x", "port": 1}], "upstreams": [{"address": "192.0.2.1", "port": 1}]}]}}`,
	} {
		hops := []driver.HopKey{{RouteID: "A"}}
		if name == "not json" || name == "no manifest" || name == "another driver" {
			hops = nil
		}
		a := driver.Artifact{Content: []byte(content), Hops: hops}
		if _, err := parseContent(a); !errors.Is(err, driver.ErrInvalidArtifact) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
