package service

import "testing"

func TestParseSubscriptionDomainsJSON(t *testing.T) {
	got := parseSubscriptionDomains(`["sub-a.example.com", "https://sub-b.example.com", "sub-a.example.com"]`)
	want := []string{"sub-a.example.com", "sub-b.example.com"}

	if len(got) != len(want) {
		t.Fatalf("expected %d domains, got %d: %#v", len(want), len(got), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected domain %d to be %q, got %q", i, want[i], got[i])
		}
	}
}

func TestParseSubscriptionDomainsDelimited(t *testing.T) {
	got := parseSubscriptionDomains("sub-a.example.com, sub-b.example.com\nhttps://sub-c.example.com/path")
	want := []string{"sub-a.example.com", "sub-b.example.com", "sub-c.example.com"}

	if len(got) != len(want) {
		t.Fatalf("expected %d domains, got %d: %#v", len(want), len(got), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected domain %d to be %q, got %q", i, want[i], got[i])
		}
	}
}
