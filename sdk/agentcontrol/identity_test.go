package agentcontrol

import (
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"testing"
)

func TestAgentNodeNames(t *testing.T) {
	for name, want := range map[string]AgentNode{
		"proxy-12":           {Kind: NodeKindProxy, ID: 12},
		"forward-3":          {Kind: NodeKindForward, ID: 3},
		"proxy-4294967295":   {Kind: NodeKindProxy, ID: 4294967295},
		"forward-1000000000": {Kind: NodeKindForward, ID: 1000000000},
	} {
		got, err := ParseAgentNode(name)
		if err != nil || got != want || got.String() != name {
			t.Fatalf("ParseAgentNode(%q) = %+v, %v", name, got, err)
		}
	}
	for _, name := range []string{
		"", "proxy", "proxy-", "proxy-0", "proxy-012", "proxy-+1", "proxy--1", "proxy-1a", "proxy-4294967296",
		"clean-1", "Proxy-1", "module-1", "proxy-1-2",
	} {
		if _, err := ParseAgentNode(name); !errors.Is(err, ErrInvalidAgentIdentity) {
			t.Fatalf("ParseAgentNode(%q) error = %v", name, err)
		}
	}
}

func TestAgentIdentityRoundTrip(t *testing.T) {
	identity, err := NewAgentIdentity("default", AgentNode{Kind: NodeKindForward, ID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if got := identity.String(); got != "spiffe://anixops/default/agent/forward-7" {
		t.Fatalf("String() = %q", got)
	}
	if got := identity.URL().String(); got != identity.String() {
		t.Fatalf("URL() = %q", got)
	}
	parsed, err := ParseAgentIdentity(identity.String())
	if err != nil || parsed != identity {
		t.Fatalf("ParseAgentIdentity = %+v, %v", parsed, err)
	}
	for _, raw := range []string{
		"spiffe://anixops/default/kernel",
		"spiffe://anixops/default/module/identity-platform",
		"spiffe://other/default/agent/proxy-1",
		"spiffe://anixops/Default/agent/proxy-1",
		"spiffe://anixops/default/agent/proxy-1/extra",
		"spiffe://anixops/default/agent/proxy-1?x=1",
		"spiffe://anixops:443/default/agent/proxy-1",
		"spiffe://user@anixops/default/agent/proxy-1",
		"spiffe://anixops/default/agent/proxy-01",
		"spiffe://anixops/default/agent/proxy-%31",
		"https://anixops/default/agent/proxy-1",
		"spiffe://anixops//agent/proxy-1",
	} {
		if _, err := ParseAgentIdentity(raw); !errors.Is(err, ErrInvalidAgentIdentity) {
			t.Fatalf("ParseAgentIdentity(%q) error = %v", raw, err)
		}
	}
	if _, err := NewAgentIdentity("default", AgentNode{Kind: NodeKindProxy}); !errors.Is(err, ErrInvalidAgentIdentity) {
		t.Fatalf("node id 0 error = %v", err)
	}
}

func TestAgentIdentityFromCertificateRequiresOneURISAN(t *testing.T) {
	uri, _ := url.Parse("spiffe://anixops/default/agent/proxy-1")
	other, _ := url.Parse("spiffe://anixops/default/agent/proxy-2")
	if identity, err := AgentIdentityFromCertificate(&x509.Certificate{URIs: []*url.URL{uri}}); err != nil ||
		identity.Node != (AgentNode{Kind: NodeKindProxy, ID: 1}) {
		t.Fatalf("identity = %+v, %v", identity, err)
	}
	for name, certificate := range map[string]*x509.Certificate{
		"nil":      nil,
		"none":     {},
		"two URIs": {URIs: []*url.URL{uri, other}},
		"DNS":      {URIs: []*url.URL{uri}, DNSNames: []string{"node.example"}},
		"IP":       {URIs: []*url.URL{uri}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}},
		"email":    {URIs: []*url.URL{uri}, EmailAddresses: []string{"node@example.com"}},
	} {
		if _, err := AgentIdentityFromCertificate(certificate); !errors.Is(err, ErrInvalidAgentIdentity) {
			t.Fatalf("%s: error = %v", name, err)
		}
	}
}
