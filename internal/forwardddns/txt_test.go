package forwardddns

import (
	"reflect"
	"strings"
	"testing"
)

func TestTXTValues(t *testing.T) {
	for raw, want := range map[string]string{
		"abc-DEF_123":   "abc-DEF_123",
		`"abc-DEF_123"`: "abc-DEF_123",
		" token= ":      "token=",
	} {
		got, err := normalizeValue(TypeTXT, raw)
		if err != nil || got != want {
			t.Fatalf("normalizeValue(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, bad := range []string{"", `""`, `a"b`, `a\b`, "a\nb", strings.Repeat("x", 256)} {
		if _, err := normalizeValue(TypeTXT, bad); err == nil {
			t.Fatalf("normalizeValue(%q) took it", bad)
		}
	}
	set := RecordSet{Zone: "example.com", Name: "_acme-challenge.example.com", Type: TypeTXT, Values: []string{"a", "b"}, TTL: 60}
	if err := CheckSet(set, false); err != nil {
		t.Fatalf("CheckSet: %v", err)
	}
	if got := huaweiValues(TypeTXT, []string{"a", "b"}); !reflect.DeepEqual(got, []string{`"a"`, `"b"`}) {
		t.Fatalf("huaweiValues = %q", got)
	}
	if !sameValues(TypeTXT, []string{`"b"`, `"a"`}, []string{"a", "b"}) {
		t.Fatal("quoted stored TXT values do not compare equal")
	}
}
