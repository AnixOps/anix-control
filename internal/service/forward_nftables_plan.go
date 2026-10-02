package service

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// The nftables_ansible forward path keeps every forward in one inet table.
// Per forward and protocol it owns:
//
//   - a DNAT chain v2b_fwd_<id>_<proto> (plus v2b_fwd_<id>_<proto>_<fam>_<n>
//     target chains when round/rand balance over several targets of a family),
//   - an accounting chain v2b_acct_<id>_<proto> with two named counters,
//     fwd_<id>_<proto>_up (ct direction original) and
//     fwd_<id>_<proto>_down (ct direction reply),
//   - one commented rule in each shared base chain (prerouting, postrouting,
//     forward) that jumps to or masquerades for the forward.
//
// The named counters are objects of the table, not rule counters, so a
// re-apply (which flushes and rebuilds the chains) keeps them. Only a delete
// removes them. config/deploy/ansible/playbooks/files/v2b_forward_nft.sh does
// the handle lookups and runs the whole change as one nft transaction.
const (
	forwardNftablesFamily       = "inet"
	forwardNftablesTable        = "v2b_forward"
	forwardNftablesLegacyFamily = "ip"
	forwardNftablesLegacyTable  = "v2b_forward"

	forwardNftablesPriorityDstnat = -100
	forwardNftablesPrioritySrcnat = 100
	forwardNftablesPriorityFilter = 0

	forwardNftFamilyIPv4 = "ipv4"
	forwardNftFamilyIPv6 = "ipv6"
)

var (
	forwardNftHostnamePattern  = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*\.?$`)
	forwardNftInterfacePattern = regexp.MustCompile(`^[A-Za-z0-9_.:@-]{1,15}\*?$`)
)

type panelForwardNftablesCounterPayload struct {
	Protocol string `json:"protocol"`
	Up       string `json:"up"`
	Down     string `json:"down"`
}

// panelForwardNftablesPayload is the nftables plan the Ansible playbooks run.
// Script is the nft text that (re)creates the forward; it is empty for
// pause and delete, which only remove rules.
type panelForwardNftablesPayload struct {
	Family         string                               `json:"family"`
	Table          string                               `json:"table"`
	LegacyFamily   string                               `json:"legacyFamily"`
	LegacyTable    string                               `json:"legacyTable"`
	IPv6           bool                                 `json:"ipv6"`
	Protocols      []string                             `json:"protocols"`
	Counters       []panelForwardNftablesCounterPayload `json:"counters"`
	DeleteCounters bool                                 `json:"deleteCounters"`
	Script         string                               `json:"script,omitempty"`
}

type forwardNftTarget struct {
	family string
	host   string
	port   int
}

func (t forwardNftTarget) dnatArg() string {
	if t.family == forwardNftFamilyIPv6 {
		return fmt.Sprintf("[%s]:%d", t.host, t.port)
	}
	return fmt.Sprintf("%s:%d", t.host, t.port)
}

func (t forwardNftTarget) dnatFamily() string {
	if t.family == forwardNftFamilyIPv6 {
		return "ip6"
	}
	return "ip"
}

func forwardNftProtocols(protocol string) []string {
	switch normalizePanelRuntimeProtocol(protocol) {
	case "both":
		return []string{"tcp", "udp"}
	case "udp":
		return []string{"udp"}
	default:
		return []string{"tcp"}
	}
}

func forwardNftDNATChain(forwardID uint, proto string) string {
	return fmt.Sprintf("v2b_fwd_%d_%s", forwardID, proto)
}

func forwardNftAcctChain(forwardID uint, proto string) string {
	return fmt.Sprintf("v2b_acct_%d_%s", forwardID, proto)
}

func forwardNftCounterName(forwardID uint, proto, direction string) string {
	return fmt.Sprintf("fwd_%d_%s_%s", forwardID, proto, direction)
}

func forwardNftRuleComment(forwardID uint, proto, chain string) string {
	return fmt.Sprintf("v2b-forward-%d-%s-%s", forwardID, proto, chain)
}

// parseForwardNftTarget classifies one "host:port" or "[v6]:port" target.
// Host names are passed to nft as IPv4 targets (nft resolves them when it
// loads the rules), which is what the IPv4-only playbook did.
func parseForwardNftTarget(raw string) (forwardNftTarget, error) {
	host, port, err := splitRuntimeTarget(strings.TrimSpace(raw))
	if err != nil {
		return forwardNftTarget{}, fmt.Errorf("invalid remote address %s: %w", raw, err)
	}
	if strings.Contains(host, "%") {
		return forwardNftTarget{}, fmt.Errorf("invalid remote address %s: zoned IPv6 addresses are not supported by the nftables backend", raw)
	}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return forwardNftTarget{family: forwardNftFamilyIPv4, host: v4.String(), port: port}, nil
		}
		return forwardNftTarget{family: forwardNftFamilyIPv6, host: ip.String(), port: port}, nil
	}
	if !forwardNftHostnamePattern.MatchString(host) {
		return forwardNftTarget{}, fmt.Errorf("invalid remote address %s: host is neither an IP address nor a host name", raw)
	}
	return forwardNftTarget{family: forwardNftFamilyIPv4, host: host, port: port}, nil
}

// forwardNftListenMatch turns a tunnel listen address into the ingress
// families it accepts and the nft match for the prerouting rule. Empty, "::"
// and "[::]" accept both families; "0.0.0.0" accepts IPv4 only; a concrete
// address matches that address. Values that are not IP addresses are
// ignored, as the IPv4-only playbook ignored the listen address entirely.
func forwardNftListenMatch(raw string) (map[string]bool, string) {
	both := map[string]bool{forwardNftFamilyIPv4: true, forwardNftFamilyIPv6: true}
	value := strings.TrimSpace(raw)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	if value == "" {
		return both, ""
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return both, ""
	}
	if v4 := ip.To4(); v4 != nil {
		if v4.IsUnspecified() {
			return map[string]bool{forwardNftFamilyIPv4: true}, "meta nfproto ipv4 "
		}
		return map[string]bool{forwardNftFamilyIPv4: true}, "ip daddr " + v4.String() + " "
	}
	if ip.IsUnspecified() {
		return both, ""
	}
	return map[string]bool{forwardNftFamilyIPv6: true}, "ip6 daddr " + ip.String() + " "
}

// buildForwardNftablesPayload builds the nftables plan for one forward. The
// script is built only for actions that install rules; pause and delete
// remove by name and never fail on a target the panel cannot parse.
func buildForwardNftablesPayload(action string, forward panelForwardAnsibleForwardPayload, tunnel panelForwardAnsibleTunnelPayload) (*panelForwardNftablesPayload, error) {
	protocols := forwardNftProtocols(tunnel.Protocol)
	plan := &panelForwardNftablesPayload{
		Family:         forwardNftablesFamily,
		Table:          forwardNftablesTable,
		LegacyFamily:   forwardNftablesLegacyFamily,
		LegacyTable:    forwardNftablesLegacyTable,
		Protocols:      protocols,
		DeleteCounters: action == model.ForwardRuntimeJobActionDelete,
	}
	for _, proto := range protocols {
		plan.Counters = append(plan.Counters, panelForwardNftablesCounterPayload{
			Protocol: proto,
			Up:       forwardNftCounterName(forward.ID, proto, "up"),
			Down:     forwardNftCounterName(forward.ID, proto, "down"),
		})
	}
	if action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause {
		return plan, nil
	}

	script, ipv6, err := renderForwardNftablesScript(forward, tunnel, protocols)
	if err != nil {
		return nil, err
	}
	plan.Script = script
	plan.IPv6 = ipv6
	return plan, nil
}

func renderForwardNftablesScript(forward panelForwardAnsibleForwardPayload, tunnel panelForwardAnsibleTunnelPayload, protocols []string) (string, bool, error) {
	if forward.ID == 0 {
		return "", false, errors.New("forward id is required for the nftables backend")
	}
	if forward.InPort <= 0 || forward.InPort > 65535 {
		return "", false, fmt.Errorf("forward %d has an invalid in port %d", forward.ID, forward.InPort)
	}

	var targets []forwardNftTarget
	for _, raw := range strings.Split(normalizeRuntimeRemoteAddr(forward.RemoteAddr), ",") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		target, err := parseForwardNftTarget(raw)
		if err != nil {
			return "", false, err
		}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		return "", false, fmt.Errorf("forward %d has no remote targets", forward.ID)
	}

	iface := strings.TrimSpace(forward.InterfaceName)
	if iface == "" {
		iface = strings.TrimSpace(tunnel.InterfaceName)
	}
	if iface != "" && !forwardNftInterfacePattern.MatchString(iface) {
		return "", false, fmt.Errorf("forward %d has an invalid interface name %q", forward.ID, iface)
	}

	strategy := strings.ToLower(strings.TrimSpace(forward.Strategy))
	table := forwardNftablesFamily + " " + forwardNftablesTable

	var b strings.Builder
	line := func(format string, args ...any) {
		fmt.Fprintf(&b, format, args...)
		b.WriteByte('\n')
	}

	line("add table %s", table)
	line("add chain %s prerouting { type nat hook prerouting priority %d ; policy accept ; }", table, forwardNftablesPriorityDstnat)
	line("add chain %s postrouting { type nat hook postrouting priority %d ; policy accept ; }", table, forwardNftablesPrioritySrcnat)
	line("add chain %s forward { type filter hook forward priority %d ; policy accept ; }", table, forwardNftablesPriorityFilter)

	usesIPv6 := false
	for _, proto := range protocols {
		listen := tunnel.TCPListenAddr
		if proto == "udp" {
			listen = tunnel.UDPListenAddr
		}
		ingress, listenMatch := forwardNftListenMatch(listen)

		byFamily := map[string][]forwardNftTarget{}
		for _, target := range targets {
			if ingress[target.family] {
				byFamily[target.family] = append(byFamily[target.family], target)
			}
		}
		if len(byFamily) == 0 {
			return "", false, fmt.Errorf("forward %d (%s): listen address %q accepts no address family of its targets; an IPv4 client cannot be forwarded to an IPv6 target or the reverse", forward.ID, proto, strings.TrimSpace(listen))
		}

		dnatChain := forwardNftDNATChain(forward.ID, proto)
		acctChain := forwardNftAcctChain(forward.ID, proto)
		up := forwardNftCounterName(forward.ID, proto, "up")
		down := forwardNftCounterName(forward.ID, proto, "down")

		line("add counter %s %s", table, up)
		line("add counter %s %s", table, down)
		line("add chain %s %s", table, dnatChain)
		for _, family := range []string{forwardNftFamilyIPv4, forwardNftFamilyIPv6} {
			familyTargets := byFamily[family]
			if len(familyTargets) == 0 {
				continue
			}
			if family == forwardNftFamilyIPv6 {
				usesIPv6 = true
			}
			if (strategy == "round" || strategy == "rand") && len(familyTargets) > 1 {
				generator := "inc"
				if strategy == "rand" {
					generator = "random"
				}
				short := "v4"
				if family == forwardNftFamilyIPv6 {
					short = "v6"
				}
				entries := make([]string, 0, len(familyTargets))
				for i, target := range familyTargets {
					targetChain := fmt.Sprintf("%s_%s_%d", dnatChain, short, i)
					line("add chain %s %s", table, targetChain)
					line("add rule %s %s meta l4proto %s dnat %s to %s", table, targetChain, proto, target.dnatFamily(), target.dnatArg())
					entries = append(entries, fmt.Sprintf("%d : goto %s", i, targetChain))
				}
				line("add rule %s %s meta nfproto %s numgen %s mod %d vmap { %s }", table, dnatChain, family, generator, len(familyTargets), strings.Join(entries, ", "))
				continue
			}
			// fifo and hash: only the first target of the family is used.
			line("add rule %s %s meta nfproto %s meta l4proto %s dnat %s to %s", table, dnatChain, family, proto, familyTargets[0].dnatFamily(), familyTargets[0].dnatArg())
		}

		line("add chain %s %s", table, acctChain)
		line("add rule %s %s ct direction original counter name %s", table, acctChain, strconv.Quote(up))
		line("add rule %s %s ct direction reply counter name %s", table, acctChain, strconv.Quote(down))

		ifaceMatch := ""
		if iface != "" {
			ifaceMatch = "iifname " + strconv.Quote(iface) + " "
		}
		ctMatch := fmt.Sprintf("ct status dnat meta l4proto %s ct original proto-dst %d", proto, forward.InPort)
		if strings.HasPrefix(listenMatch, "ip daddr ") {
			ctMatch += " ct original ip daddr " + strings.TrimSpace(strings.TrimPrefix(listenMatch, "ip daddr "))
		} else if strings.HasPrefix(listenMatch, "ip6 daddr ") {
			ctMatch += " ct original ip6 daddr " + strings.TrimSpace(strings.TrimPrefix(listenMatch, "ip6 daddr "))
		}
		line("add rule %s prerouting %s%smeta l4proto %s th dport %d jump %s comment %s", table, ifaceMatch, listenMatch, proto, forward.InPort, dnatChain, strconv.Quote(forwardNftRuleComment(forward.ID, proto, "prerouting")))
		line("add rule %s forward %s jump %s comment %s", table, ctMatch, acctChain, strconv.Quote(forwardNftRuleComment(forward.ID, proto, "forward")))
		line("add rule %s postrouting %s masquerade comment %s", table, ctMatch, strconv.Quote(forwardNftRuleComment(forward.ID, proto, "postrouting")))
	}
	return b.String(), usesIPv6, nil
}
