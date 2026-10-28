package kernelforward

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"regexp"
	"sort"
	"strings"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Vantage probes (L4, forward-sdk.md section 7.4). Entry HA judges an entry
// node by its own reports, which cannot see what the clients see: an entry
// address blocked or null-routed on the clients' networks still looks
// healthy from the node. A DNS binding with a vantage selector ("key=value")
// has the forwarding nodes that carry that label, typically nodes in the
// clients' country, dial every published address of the route's entry
// nodes:
//
//   - The probes are part of the vantage nodes' desired state
//     (NodeForwardState.vantage_probes), so a node dials only what its state
//     says, and only global unicast addresses; every plan recomputes them.
//   - A node reports each probe's state (VantageResult): UNHEALTHY after 3
//     failed dials in a row, HEALTHY after 3 successful ones.
//   - The entry HA controller leaves out an address that more vantage nodes
//     with a fresh report (DNSReportStaleAfter) find UNHEALTHY than
//     HEALTHY; an entry node left without an address is unhealthy
//     (vantage_unreachable), so the node-level hysteresis and the never
//     empty rule apply as before.

const (
	// VantageInterval and VantageTimeout are a probe's defaults.
	VantageInterval = 30 * time.Second
	VantageTimeout  = 3 * time.Second
	// MaxVantageProbes bounds the probes of one node.
	MaxVantageProbes = 512
	// maxVantageSelector bounds a selector (the column's size).
	maxVantageSelector = 200
	// dnsReasonVantageUnreachable: every address of the node is
	// unreachable from the vantage nodes.
	dnsReasonVantageUnreachable = "vantage_unreachable"
)

var vantageKeyPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._/-]{0,61}[A-Za-z0-9])?$`)

// parseVantage splits a selector "key=value"; ok is false for anything
// else. An empty selector is not a selector.
func parseVantage(selector string) (key, value string, ok bool) {
	key, value, found := strings.Cut(strings.TrimSpace(selector), "=")
	key, value = strings.TrimSpace(key), strings.TrimSpace(value)
	if !found || !vantageKeyPattern.MatchString(key) || value == "" || strings.ContainsAny(value, "=\x00\r\n\t") {
		return "", "", false
	}
	return key, value, true
}

// checkVantage normalizes a binding's selector ("" when none).
func checkVantage(selector string) (string, *forwardv1.Violation) {
	if strings.TrimSpace(selector) == "" {
		return "", nil
	}
	key, value, ok := parseVantage(selector)
	if !ok || len(key)+1+len(value) > maxVantageSelector {
		return "", violation("binding.vantage", CodeInvalidFormat, fmt.Sprintf("must be a node label key=value, at most %d bytes", maxVantageSelector))
	}
	return key + "=" + value, nil
}

// loadVantage answers the selectors of bindings by binding id.
func loadVantage(tx *gorm.DB, ids ...uint64) (map[uint64]string, error) {
	query := tx.Model(&model.KernelForwardDNSVantage{})
	if len(ids) > 0 {
		query = query.Where("binding_id IN ?", ids)
	}
	var rows []model.KernelForwardDNSVantage
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: load dns vantage: %w", err)
	}
	out := make(map[uint64]string, len(rows))
	for _, row := range rows {
		out[row.BindingID] = row.Selector
	}
	return out, nil
}

// saveVantage stores or removes a binding's selector.
func saveVantage(tx *gorm.DB, bindingID uint64, selector string, now time.Time) error {
	if err := tx.Delete(&model.KernelForwardDNSVantage{}, "binding_id = ?", bindingID).Error; err != nil {
		return fmt.Errorf("kernel forward: save dns vantage: %w", err)
	}
	if selector == "" {
		return nil
	}
	if err := tx.Create(&model.KernelForwardDNSVantage{BindingID: bindingID, Selector: selector, UpdatedAt: now}).Error; err != nil {
		return fmt.Errorf("kernel forward: save dns vantage: %w", err)
	}
	return nil
}

// addVantageProbes adds to states the vantage probes of every unpaused
// binding with a selector: for each published address of each entry node
// of its route (on the entry hop's planned listen port, for routes with a
// TCP listener), one probe on every inventory node whose labels match,
// other than the route's entry nodes. routes are the effective routes of
// the plan.
func addVantageProbes(tx *gorm.DB, states map[string]*forwardv1.NodeForwardState, nodes []*forwardv1.NodeInfo, routes []*forwardv1.Route) error {
	selectors, err := loadVantage(tx)
	if err != nil || len(selectors) == 0 {
		return err
	}
	ids := make([]uint64, 0, len(selectors))
	for id := range selectors {
		ids = append(ids, id)
	}
	var bindings []model.KernelForwardDNSBinding
	if err := tx.Where("id IN ? AND paused = ?", ids, false).Order("id").Find(&bindings).Error; err != nil {
		return fmt.Errorf("kernel forward: load dns bindings: %w", err)
	}
	byRoute := make(map[string]*forwardv1.Route, len(routes))
	for _, route := range routes {
		byRoute[route.GetId()] = route
	}
	addresses := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		addresses[node.GetNodeRef()] = node.GetAddresses()
	}
	probes := map[string][]*forwardv1.VantageProbe{}
	for _, binding := range bindings {
		key, value, ok := parseVantage(selectors[binding.ID])
		route := byRoute[binding.RouteID]
		if !ok || route == nil || route.GetPaused() || len(route.GetHops()) == 0 ||
			route.GetListen().GetProtocol() == forwardv1.L4Protocol_L4_PROTOCOL_UDP {
			continue
		}
		entries := route.GetHops()[0].GetNodeRefs()
		isEntry := make(map[string]bool, len(entries))
		for _, ref := range entries {
			isEntry[ref] = true
		}
		var targets []*forwardv1.VantageProbe
		for _, ref := range entries {
			port := entryPort(states[ref], route.GetId())
			if port == 0 {
				continue
			}
			for _, address := range publicAddresses(addresses[ref], bindingTypes(binding)) {
				targets = append(targets, &forwardv1.VantageProbe{
					BindingId: binding.ID, RouteId: route.GetId(), NodeRef: ref, Address: address, Port: port,
					IntervalMs: uint32(VantageInterval.Milliseconds()), TimeoutMs: uint32(VantageTimeout.Milliseconds()),
				})
			}
		}
		for _, node := range nodes {
			ref := node.GetNodeRef()
			if isEntry[ref] || node.GetLabels()[key] != value {
				continue
			}
			probes[ref] = append(probes[ref], targets...)
		}
	}
	for ref, list := range probes {
		sort.SliceStable(list, func(i, j int) bool {
			a, b := list[i], list[j]
			if a.GetBindingId() != b.GetBindingId() {
				return a.GetBindingId() < b.GetBindingId()
			}
			if a.GetNodeRef() != b.GetNodeRef() {
				return a.GetNodeRef() < b.GetNodeRef()
			}
			return a.GetAddress() < b.GetAddress()
		})
		if len(list) > MaxVantageProbes {
			list = list[:MaxVantageProbes]
		}
		if len(list) == 0 {
			continue
		}
		state := states[ref]
		if state == nil {
			state = &forwardv1.NodeForwardState{NodeRef: ref}
			states[ref] = state
		}
		state.VantageProbes = list
	}
	return nil
}

// entryPort answers the listen port of routeID's entry hop in a node's
// planned state, 0 when the node does not run it.
func entryPort(state *forwardv1.NodeForwardState, routeID string) uint32 {
	for _, hop := range state.GetHops() {
		if hop.GetRouteId() == routeID && hop.GetHopIndex() == 0 {
			return hop.GetListen().GetPort()
		}
	}
	return 0
}

// vantageView is what the vantage nodes' fresh reports say of one address.
type vantageView struct {
	up, down int
	nodes    map[string]bool
}

// vantageResults indexes the fresh vantage results of every report by
// binding and address.
func vantageResults(reports map[string]reportFacts, now time.Time) map[uint64]map[string]*vantageView {
	out := map[uint64]map[string]*vantageView{}
	for ref, facts := range reports {
		if facts.report == nil || now.Sub(facts.receivedAt) > DNSReportStaleAfter {
			continue
		}
		for _, result := range facts.report.GetVantage() {
			addr, err := netip.ParseAddr(result.GetAddress())
			if err != nil {
				continue
			}
			byAddress := out[result.GetBindingId()]
			if byAddress == nil {
				byAddress = map[string]*vantageView{}
				out[result.GetBindingId()] = byAddress
			}
			view := byAddress[addr.Unmap().String()]
			if view == nil {
				view = &vantageView{nodes: map[string]bool{}}
				byAddress[addr.Unmap().String()] = view
			}
			switch result.GetState() {
			case forwardv1.HealthState_HEALTH_STATE_HEALTHY:
				view.up++
			case forwardv1.HealthState_HEALTH_STATE_UNHEALTHY, forwardv1.HealthState_HEALTH_STATE_CIRCUIT_OPEN:
				view.down++
			}
			view.nodes[ref] = true
		}
	}
	return out
}

// unreachable splits addresses into those most vantage nodes reach (or
// none judged) and those more of them find down than up.
func unreachable(views map[string]*vantageView, addresses []string) (kept, down []string, vantageNodes int) {
	seen := map[string]bool{}
	for _, address := range addresses {
		view := views[address]
		if view != nil {
			for ref := range view.nodes {
				seen[ref] = true
			}
		}
		if view != nil && view.down > view.up {
			down = append(down, address)
			continue
		}
		kept = append(kept, address)
	}
	return kept, down, len(seen)
}

// reportFacts is a node's latest report and when it arrived.
type reportFacts struct {
	report     *forwardv1.NodeForwardReport
	receivedAt time.Time
}

// decodeReports decodes the stored latest reports by node.
func decodeReports(rows map[string]model.KernelForwardNodeReport) map[string]reportFacts {
	out := make(map[string]reportFacts, len(rows))
	for ref, row := range rows {
		report := &forwardv1.NodeForwardReport{}
		if err := jsonRead.Unmarshal([]byte(row.ReportJSON), report); err != nil {
			continue
		}
		report.Generation, report.Applied = row.Generation, row.Applied
		out[ref] = reportFacts{report: report, receivedAt: row.ReceivedAt}
	}
	return out
}

// replanVantage replans after a binding's vantage probes changed, so the
// vantage nodes get them. A failed replan is logged: the next plan (any
// route, inventory or binding change) carries them.
func (s *Service) replanVantage(ctx context.Context) {
	if _, err := s.Replan(ctx, "dns_vantage"); err != nil {
		slog.Warn("forward entry HA: replan for vantage probes failed", "component", "kernel-forward", "error", err)
	}
}
