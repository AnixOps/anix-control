package service

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsPublicProbeAddress(t *testing.T) {
	for _, raw := range []string{"203.0.113.7", "8.8.8.8", "2001:db8::1", "2606:4700::1111"} {
		require.True(t, isPublicProbeAddress(netip.MustParseAddr(raw)), raw)
	}
	for _, raw := range []string{
		"127.0.0.1", "127.8.9.10", "::1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "fe80::1",
		"fe80::1%eth0", "fc00::1", "fd12::1", "0.0.0.0", "::", "0.1.2.3", "100.64.0.1", "192.0.0.8", "198.18.0.1",
		"240.0.0.1", "255.255.255.255", "224.0.0.1", "ff02::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1",
		"64:ff9b::a00:1", "2002:a00:1::1",
	} {
		require.False(t, isPublicProbeAddress(netip.MustParseAddr(raw)), raw)
	}
}

func TestPublicProbeAddressChecksEveryResolvedAddress(t *testing.T) {
	previous := probeLookup
	t.Cleanup(func() { probeLookup = previous })
	answers := map[string][]netip.Addr{
		"public.example.test": {netip.MustParseAddr("203.0.113.7")},
		"mixed.example.test":  {netip.MustParseAddr("203.0.113.7"), netip.MustParseAddr("10.0.0.5")},
		"mapped.example.test": {netip.MustParseAddr("::ffff:203.0.113.9")},
	}
	probeLookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		if addrs, ok := answers[host]; ok {
			return addrs, nil
		}
		return nil, errors.New("no such host")
	}

	addr, refusal := publicProbeAddress("public.example.test")
	require.Empty(t, refusal)
	require.Equal(t, "203.0.113.7", addr.String())

	addr, refusal = publicProbeAddress("mapped.example.test")
	require.Empty(t, refusal)
	require.Equal(t, "203.0.113.9", addr.String())

	_, refusal = publicProbeAddress("mixed.example.test")
	require.Equal(t, "不能诊断内网或本机地址", refusal)

	_, refusal = publicProbeAddress("missing.example.test")
	require.Equal(t, "no such host", refusal)

	addr, refusal = publicProbeAddress("198.51.100.4")
	require.Empty(t, refusal)
	require.Equal(t, "198.51.100.4", addr.String())

	_, refusal = publicProbeAddress("127.0.0.1")
	require.Equal(t, "不能诊断内网或本机地址", refusal)
}
