package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nftPlanForTest(t *testing.T, action string, forward panelForwardAnsibleForwardPayload, tunnel panelForwardAnsibleTunnelPayload) *panelForwardNftablesPayload {
	t.Helper()
	plan, err := buildForwardNftablesPayload(action, forward, tunnel)
	require.NoError(t, err)
	return plan
}

func TestForwardNftablesPlan_SingleIPv4TargetTCPAndUDP(t *testing.T) {
	plan := nftPlanForTest(t, model.ForwardRuntimeJobActionCreate,
		panelForwardAnsibleForwardPayload{ID: 12, InPort: 8080, RemoteAddr: "10.0.0.1:80", Strategy: "fifo"},
		panelForwardAnsibleTunnelPayload{Protocol: "both", TCPListenAddr: "[::]", UDPListenAddr: "[::]"},
	)

	assert.Equal(t, "inet", plan.Family)
	assert.Equal(t, "v2b_forward", plan.Table)
	assert.Equal(t, "ip", plan.LegacyFamily)
	assert.Equal(t, []string{"tcp", "udp"}, plan.Protocols)
	assert.False(t, plan.IPv6)
	assert.False(t, plan.DeleteCounters)
	assert.Equal(t, []panelForwardNftablesCounterPayload{
		{Protocol: "tcp", Up: "fwd_12_tcp_up", Down: "fwd_12_tcp_down"},
		{Protocol: "udp", Up: "fwd_12_udp_up", Down: "fwd_12_udp_down"},
	}, plan.Counters)

	expected := strings.Join([]string{
		"add table inet v2b_forward",
		"add chain inet v2b_forward prerouting { type nat hook prerouting priority -100 ; policy accept ; }",
		"add chain inet v2b_forward postrouting { type nat hook postrouting priority 100 ; policy accept ; }",
		"add chain inet v2b_forward forward { type filter hook forward priority 0 ; policy accept ; }",
		"add counter inet v2b_forward fwd_12_tcp_up",
		"add counter inet v2b_forward fwd_12_tcp_down",
		"add chain inet v2b_forward v2b_fwd_12_tcp",
		"add rule inet v2b_forward v2b_fwd_12_tcp meta nfproto ipv4 meta l4proto tcp dnat ip to 10.0.0.1:80",
		"add chain inet v2b_forward v2b_acct_12_tcp",
		`add rule inet v2b_forward v2b_acct_12_tcp ct direction original counter name "fwd_12_tcp_up"`,
		`add rule inet v2b_forward v2b_acct_12_tcp ct direction reply counter name "fwd_12_tcp_down"`,
		`add rule inet v2b_forward prerouting meta l4proto tcp th dport 8080 jump v2b_fwd_12_tcp comment "v2b-forward-12-tcp-prerouting"`,
		`add rule inet v2b_forward forward ct status dnat meta l4proto tcp ct original proto-dst 8080 jump v2b_acct_12_tcp comment "v2b-forward-12-tcp-forward"`,
		`add rule inet v2b_forward postrouting ct status dnat meta l4proto tcp ct original proto-dst 8080 masquerade comment "v2b-forward-12-tcp-postrouting"`,
		"add counter inet v2b_forward fwd_12_udp_up",
		"add counter inet v2b_forward fwd_12_udp_down",
		"add chain inet v2b_forward v2b_fwd_12_udp",
		"add rule inet v2b_forward v2b_fwd_12_udp meta nfproto ipv4 meta l4proto udp dnat ip to 10.0.0.1:80",
		"add chain inet v2b_forward v2b_acct_12_udp",
		`add rule inet v2b_forward v2b_acct_12_udp ct direction original counter name "fwd_12_udp_up"`,
		`add rule inet v2b_forward v2b_acct_12_udp ct direction reply counter name "fwd_12_udp_down"`,
		`add rule inet v2b_forward prerouting meta l4proto udp th dport 8080 jump v2b_fwd_12_udp comment "v2b-forward-12-udp-prerouting"`,
		`add rule inet v2b_forward forward ct status dnat meta l4proto udp ct original proto-dst 8080 jump v2b_acct_12_udp comment "v2b-forward-12-udp-forward"`,
		`add rule inet v2b_forward postrouting ct status dnat meta l4proto udp ct original proto-dst 8080 masquerade comment "v2b-forward-12-udp-postrouting"`,
	}, "\n") + "\n"
	assert.Equal(t, expected, plan.Script)
}

func TestForwardNftablesPlan_RoundOverMixedFamiliesBalancesPerFamily(t *testing.T) {
	plan := nftPlanForTest(t, model.ForwardRuntimeJobActionUpdate,
		panelForwardAnsibleForwardPayload{
			ID: 7, InPort: 443, Strategy: "round", InterfaceName: "eth0",
			RemoteAddr: "10.0.0.1:443,[2001:db8::1]:8443,10.0.0.2:4443,[2001:db8::2]:443",
		},
		panelForwardAnsibleTunnelPayload{Protocol: "tcp", TCPListenAddr: "[::]"},
	)

	assert.True(t, plan.IPv6)
	assert.Contains(t, plan.Script, "add rule inet v2b_forward v2b_fwd_7_tcp_v4_0 meta l4proto tcp dnat ip to 10.0.0.1:443\n")
	assert.Contains(t, plan.Script, "add rule inet v2b_forward v2b_fwd_7_tcp_v4_1 meta l4proto tcp dnat ip to 10.0.0.2:4443\n")
	assert.Contains(t, plan.Script, "add rule inet v2b_forward v2b_fwd_7_tcp_v6_0 meta l4proto tcp dnat ip6 to [2001:db8::1]:8443\n")
	assert.Contains(t, plan.Script, "add rule inet v2b_forward v2b_fwd_7_tcp_v6_1 meta l4proto tcp dnat ip6 to [2001:db8::2]:443\n")
	assert.Contains(t, plan.Script, "add rule inet v2b_forward v2b_fwd_7_tcp meta nfproto ipv4 numgen inc mod 2 vmap { 0 : goto v2b_fwd_7_tcp_v4_0, 1 : goto v2b_fwd_7_tcp_v4_1 }\n")
	assert.Contains(t, plan.Script, "add rule inet v2b_forward v2b_fwd_7_tcp meta nfproto ipv6 numgen inc mod 2 vmap { 0 : goto v2b_fwd_7_tcp_v6_0, 1 : goto v2b_fwd_7_tcp_v6_1 }\n")
	assert.Contains(t, plan.Script, `add rule inet v2b_forward prerouting iifname "eth0" meta l4proto tcp th dport 443 jump v2b_fwd_7_tcp comment "v2b-forward-7-tcp-prerouting"`)
	assert.NotContains(t, plan.Script, "udp")
}

func TestForwardNftablesPlan_RandUsesRandomGenerator(t *testing.T) {
	plan := nftPlanForTest(t, model.ForwardRuntimeJobActionCreate,
		panelForwardAnsibleForwardPayload{ID: 3, InPort: 1000, Strategy: "rand", RemoteAddr: "10.0.0.1:1,10.0.0.2:2,10.0.0.3:3"},
		panelForwardAnsibleTunnelPayload{Protocol: "udp", UDPListenAddr: "0.0.0.0"},
	)
	assert.Contains(t, plan.Script, "meta nfproto ipv4 numgen random mod 3 vmap { 0 : goto v2b_fwd_3_udp_v4_0, 1 : goto v2b_fwd_3_udp_v4_1, 2 : goto v2b_fwd_3_udp_v4_2 }")
	assert.Contains(t, plan.Script, "add rule inet v2b_forward prerouting meta nfproto ipv4 meta l4proto udp th dport 1000 jump v2b_fwd_3_udp")
}

func TestForwardNftablesPlan_FifoAndHashUseFirstTargetPerFamily(t *testing.T) {
	for _, strategy := range []string{"fifo", "hash"} {
		plan := nftPlanForTest(t, model.ForwardRuntimeJobActionCreate,
			panelForwardAnsibleForwardPayload{ID: 5, InPort: 2000, Strategy: strategy, RemoteAddr: "[2001:db8::9]:9,10.0.0.1:1,10.0.0.2:2,[2001:db8::8]:8"},
			panelForwardAnsibleTunnelPayload{Protocol: "tcp"},
		)
		assert.Contains(t, plan.Script, "add rule inet v2b_forward v2b_fwd_5_tcp meta nfproto ipv4 meta l4proto tcp dnat ip to 10.0.0.1:1\n", strategy)
		assert.Contains(t, plan.Script, "add rule inet v2b_forward v2b_fwd_5_tcp meta nfproto ipv6 meta l4proto tcp dnat ip6 to [2001:db8::9]:9\n", strategy)
		assert.NotContains(t, plan.Script, "numgen", strategy)
		assert.NotContains(t, plan.Script, "10.0.0.2", strategy)
	}
}

func TestForwardNftablesPlan_SpecificListenAddresses(t *testing.T) {
	plan := nftPlanForTest(t, model.ForwardRuntimeJobActionCreate,
		panelForwardAnsibleForwardPayload{ID: 9, InPort: 3000, RemoteAddr: "10.0.0.1:1,[2001:db8::1]:1"},
		panelForwardAnsibleTunnelPayload{Protocol: "both", TCPListenAddr: "203.0.113.5", UDPListenAddr: "[2001:db8:ffff::5]"},
	)
	assert.Contains(t, plan.Script, "add rule inet v2b_forward prerouting ip daddr 203.0.113.5 meta l4proto tcp th dport 3000 jump v2b_fwd_9_tcp")
	assert.Contains(t, plan.Script, "ct status dnat meta l4proto tcp ct original proto-dst 3000 ct original ip daddr 203.0.113.5 jump v2b_acct_9_tcp")
	assert.Contains(t, plan.Script, "add rule inet v2b_forward prerouting ip6 daddr 2001:db8:ffff::5 meta l4proto udp th dport 3000 jump v2b_fwd_9_udp")
	assert.Contains(t, plan.Script, "ct original ip6 daddr 2001:db8:ffff::5 masquerade")
	assert.NotContains(t, plan.Script, "v2b_fwd_9_tcp meta nfproto ipv6")
	assert.NotContains(t, plan.Script, "v2b_fwd_9_udp meta nfproto ipv4")
}

func TestForwardNftablesPlan_RejectsFamilyMismatchAndUnsafeInput(t *testing.T) {
	_, err := buildForwardNftablesPayload(model.ForwardRuntimeJobActionCreate,
		panelForwardAnsibleForwardPayload{ID: 1, InPort: 1, RemoteAddr: "[2001:db8::1]:1"},
		panelForwardAnsibleTunnelPayload{Protocol: "tcp", TCPListenAddr: "0.0.0.0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts no address family")

	_, err = buildForwardNftablesPayload(model.ForwardRuntimeJobActionCreate,
		panelForwardAnsibleForwardPayload{ID: 1, InPort: 1, RemoteAddr: "x;flush ruleset:1"},
		panelForwardAnsibleTunnelPayload{Protocol: "tcp"})
	require.Error(t, err)

	_, err = buildForwardNftablesPayload(model.ForwardRuntimeJobActionCreate,
		panelForwardAnsibleForwardPayload{ID: 1, InPort: 1, RemoteAddr: "10.0.0.1:1", InterfaceName: `eth0" accept`},
		panelForwardAnsibleTunnelPayload{Protocol: "tcp"})
	require.Error(t, err)

	_, err = buildForwardNftablesPayload(model.ForwardRuntimeJobActionCreate,
		panelForwardAnsibleForwardPayload{ID: 1, InPort: 1, RemoteAddr: "[fe80::1%eth0]:1"},
		panelForwardAnsibleTunnelPayload{Protocol: "tcp"})
	require.Error(t, err)
}

func TestForwardNftablesPlan_RemoveActionsHaveNoScript(t *testing.T) {
	// A target the panel cannot parse must not block a pause or a delete.
	broken := panelForwardAnsibleForwardPayload{ID: 4, InPort: 1, RemoteAddr: "not a target"}
	pause := nftPlanForTest(t, model.ForwardRuntimeJobActionPause, broken, panelForwardAnsibleTunnelPayload{Protocol: "both"})
	assert.Empty(t, pause.Script)
	assert.False(t, pause.DeleteCounters)

	del := nftPlanForTest(t, model.ForwardRuntimeJobActionDelete, broken, panelForwardAnsibleTunnelPayload{Protocol: "both"})
	assert.Empty(t, del.Script)
	assert.True(t, del.DeleteCounters)
	assert.Len(t, del.Counters, 2)
}

func TestForwardNftablesPlan_HostnameTargetsStayIPv4(t *testing.T) {
	plan := nftPlanForTest(t, model.ForwardRuntimeJobActionCreate,
		panelForwardAnsibleForwardPayload{ID: 2, InPort: 1, RemoteAddr: "example.com:443"},
		panelForwardAnsibleTunnelPayload{Protocol: "tcp"})
	assert.Contains(t, plan.Script, "meta nfproto ipv4 meta l4proto tcp dnat ip to example.com:443")
	assert.False(t, plan.IPv6)
}

func TestBuildPanelForwardAnsibleTargets_BracketsIPv6(t *testing.T) {
	targets, err := buildPanelForwardAnsibleTargets("10.0.0.1:80\n[2001:db8::1]:443")
	require.NoError(t, err)
	require.Len(t, targets, 2)
	assert.Equal(t, panelForwardAnsibleTargetPayload{Name: "target-1", Addr: "10.0.0.1:80", Host: "10.0.0.1", Port: 80}, targets[0])
	assert.Equal(t, panelForwardAnsibleTargetPayload{Name: "target-2", Addr: "[2001:db8::1]:443", Host: "2001:db8::1", Port: 443}, targets[1])
}

// nftCheckCommand returns a command that runs nft in a private network
// namespace, so tests never read or change the host ruleset. It skips when
// nft or an unprivileged namespace is not available.
func nftCheckCommand(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	nft, err := exec.LookPath("nft")
	if err != nil {
		t.Skip("nft is not installed")
	}
	unshare, err := exec.LookPath("unshare")
	if err != nil {
		t.Skip("unshare is not installed")
	}
	if out, err := exec.Command(unshare, "-n", nft, "list", "ruleset").CombinedOutput(); err != nil { // #nosec G204 -- test helper, fixed binaries
		t.Skipf("cannot run nft in a private network namespace: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return exec.Command(unshare, append([]string{"-n", nft}, args...)...) // #nosec G204 -- test helper, fixed binaries
}

func TestForwardNftablesPlan_ScriptPassesNftCheck(t *testing.T) {
	cases := map[string]struct {
		forward panelForwardAnsibleForwardPayload
		tunnel  panelForwardAnsibleTunnelPayload
	}{
		"ipv4-both":   {panelForwardAnsibleForwardPayload{ID: 12, InPort: 8080, RemoteAddr: "10.0.0.1:80"}, panelForwardAnsibleTunnelPayload{Protocol: "both", TCPListenAddr: "[::]", UDPListenAddr: "0.0.0.0"}},
		"ipv6-single": {panelForwardAnsibleForwardPayload{ID: 13, InPort: 8081, RemoteAddr: "[2001:db8::1]:443"}, panelForwardAnsibleTunnelPayload{Protocol: "tcp", TCPListenAddr: "[2001:db8:ffff::5]"}},
		"round-mixed": {panelForwardAnsibleForwardPayload{ID: 14, InPort: 8082, Strategy: "round", InterfaceName: "eth*", RemoteAddr: "10.0.0.1:1,10.0.0.2:2,[2001:db8::1]:3,[2001:db8::2]:4"}, panelForwardAnsibleTunnelPayload{Protocol: "both"}},
		"rand-v4":     {panelForwardAnsibleForwardPayload{ID: 15, InPort: 8083, Strategy: "rand", RemoteAddr: "10.0.0.1:1,10.0.0.2:2"}, panelForwardAnsibleTunnelPayload{Protocol: "udp", UDPListenAddr: "203.0.113.5"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			plan := nftPlanForTest(t, model.ForwardRuntimeJobActionCreate, tc.forward, tc.tunnel)
			path := filepath.Join(t.TempDir(), "forward.nft")
			require.NoError(t, os.WriteFile(path, []byte(plan.Script), 0o600))
			out, err := nftCheckCommand(t, "-c", "-f", path).CombinedOutput()
			require.NoError(t, err, "nft -c rejected the script:\n%s\n%s", plan.Script, out)
		})
	}
}

const forwardNftHelperScript = "../../config/deploy/ansible/playbooks/files/v2b_forward_nft.sh"

// TestForwardNftablesHelper_LifecycleInPrivateNamespace drives the playbook
// helper through migrate, re-apply, stats, pause and delete inside a private
// network namespace (never the host ruleset).
func TestForwardNftablesHelper_LifecycleInPrivateNamespace(t *testing.T) {
	helper, err := filepath.Abs(forwardNftHelperScript)
	require.NoError(t, err)
	dir := t.TempDir()

	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}
	planFor := func(id uint, remote, strategy string) string {
		plan := nftPlanForTest(t, model.ForwardRuntimeJobActionUpdate,
			panelForwardAnsibleForwardPayload{ID: id, InPort: int(9000 + id), RemoteAddr: remote, Strategy: strategy},
			panelForwardAnsibleTunnelPayload{Protocol: "both"})
		return write(fmt.Sprintf("fwd%d-%s.nft", id, strategy), plan.Script)
	}
	fwd12 := planFor(12, "10.0.0.1:80,10.0.0.2:80,[2001:db8::1]:80", "round")
	fwd12fifo := planFor(12, "10.0.0.9:80", "fifo")
	fwd13 := planFor(13, "10.0.0.3:80", "fifo")

	// The legacy playbook layout for forwards 12 and 13.
	legacy := write("legacy.nft", `table ip v2b_forward {
	chain prerouting { type nat hook prerouting priority dstnat; policy accept;
		meta l4proto tcp th dport 9012 jump v2b_fwd_12_tcp comment "v2b-forward-12-tcp-prerouting"
		meta l4proto tcp th dport 9013 jump v2b_fwd_13_tcp comment "v2b-forward-13-tcp-prerouting"
	}
	chain postrouting { type nat hook postrouting priority srcnat; policy accept;
		meta l4proto tcp ip daddr 10.0.0.1 th dport 80 masquerade comment "v2b-forward-12-tcp-postrouting-1"
		meta l4proto tcp ip daddr 10.0.0.3 th dport 80 masquerade comment "v2b-forward-13-tcp-postrouting-1"
	}
	chain v2b_fwd_12_tcp { meta l4proto tcp counter packets 2 bytes 120 dnat to 10.0.0.1:80; }
	chain v2b_fwd_13_tcp { meta l4proto tcp counter packets 4 bytes 240 dnat to 10.0.0.3:80; }
}
`)
	seed := write("seed.nft", "add table inet v2b_forward\nadd counter inet v2b_forward fwd_12_tcp_up { packets 3 bytes 300 }\nadd counter inet v2b_forward fwd_12_tcp_down { packets 5 bytes 5000 }\n")

	driver := write("driver.sh", fmt.Sprintf(`set -euo pipefail
H=%q
step() { echo "### $*"; }
nft -f %q
step legacy-stats; bash "$H" stats 12
nft -f %q
step apply-12; bash "$H" apply 12 %q
step after-apply-12; nft list tables
nft list table ip v2b_forward | grep -c v2b_fwd_12 || true
step stats-12; bash "$H" stats 12
step reapply-12; bash "$H" apply 12 %q
bash "$H" apply 12 %q
step stats-12-after-reapply; bash "$H" stats 12
nft list table inet v2b_forward | grep -c 'chain v2b_fwd_12_' || true
step apply-13; bash "$H" apply 13 %q
step tables-after-13; nft list tables
step stats-13; bash "$H" stats 13
step pause-12; bash "$H" remove 12
nft list table inet v2b_forward | grep -c 'v2b-forward-12-' || true
step stats-12-after-pause; bash "$H" stats 12
step delete-12; bash "$H" remove 12 delete-counters
nft list counters table inet v2b_forward | grep -c fwd_12_ || true
step delete-13; bash "$H" remove 13 delete-counters
step tables-final; nft list tables
`, helper, legacy, seed, fwd12, fwd12fifo, fwd12, fwd13))

	out, err := nftCheckCommand(t, "list", "ruleset").CombinedOutput()
	require.NoError(t, err, string(out))
	unshare, _ := exec.LookPath("unshare")
	cmd := exec.Command(unshare, "-n", "bash", driver) // #nosec G204 -- test helper, fixed binaries
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	got := string(output)

	section := func(name string) string {
		start := strings.Index(got, "### "+name+"\n")
		require.GreaterOrEqual(t, start, 0, "missing section %s in:\n%s", name, got)
		rest := got[start+len("### "+name+"\n"):]
		if end := strings.Index(rest, "### "); end >= 0 {
			rest = rest[:end]
		}
		return rest
	}

	assert.Equal(t, `STATS_JSON {"protocol":"tcp","bytes":120,"legacy":true}`+"\n", section("legacy-stats"))
	// Forward 12 left the legacy table; forward 13 still uses it.
	assert.Equal(t, "table ip v2b_forward\ntable inet v2b_forward\n0\n", section("after-apply-12"))
	// Apply keeps existing named counters (seeded with 300/5000 bytes).
	assert.Contains(t, section("stats-12"), `STATS_JSON {"protocol":"tcp","upload":300,"download":5000}`)
	assert.Contains(t, section("stats-12"), `STATS_JSON {"protocol":"udp","upload":0,"download":0}`)
	assert.Contains(t, section("stats-12-after-reapply"), `STATS_JSON {"protocol":"tcp","upload":300,"download":5000}`)
	// The fifo re-apply removed the round target chains and the round one
	// rebuilt them: per protocol the DNAT chain plus two IPv4 target chains
	// (the single IPv6 target needs none).
	assert.Contains(t, section("stats-12-after-reapply"), "\n6\n")
	// The last legacy forward migrated: the legacy table is gone.
	assert.Equal(t, "table inet v2b_forward\n", section("tables-after-13"))
	assert.Contains(t, section("stats-13"), `STATS_JSON {"protocol":"tcp","upload":0,"download":0}`)
	// Pause removes the rules but keeps the counters.
	assert.Equal(t, "0\n", strings.TrimPrefix(section("pause-12"), "V2B_NFT removed forward 12\n"))
	assert.Contains(t, section("stats-12-after-pause"), `STATS_JSON {"protocol":"tcp","upload":300,"download":5000}`)
	assert.Equal(t, "V2B_NFT removed forward 12\n0\n", section("delete-12"))
	// Deleting the last forward drops the table.
	assert.Equal(t, "", section("tables-final"))
}
