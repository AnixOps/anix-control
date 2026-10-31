package kernelforward

import (
	"fmt"
	"net/netip"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/AnixOps/anix-control/v5/internal/nodeentry"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
)

// Health of ENGINE_NETWORK routes. Each entry device runs the route as a
// port forward, whose checks dial each target through the route's whole
// path (the anx_line transports and the exit's egress) and report in the
// device's NetworkStatus (PortForwardStatus.targets). So a route's health
// is its entries', as hop 0's upstreams being the targets checked by each
// entry, and the same checks feed the latency history, the diagnosis and
// the entry HA DNS controller. Nothing here reads the forward.v1 reports.

var networkHealthStates = map[networkv1.TargetHealthState]forwardv1.HealthState{
	networkv1.TargetHealthState_TARGET_HEALTH_STATE_HEALTHY:      forwardv1.HealthState_HEALTH_STATE_HEALTHY,
	networkv1.TargetHealthState_TARGET_HEALTH_STATE_UNHEALTHY:    forwardv1.HealthState_HEALTH_STATE_UNHEALTHY,
	networkv1.TargetHealthState_TARGET_HEALTH_STATE_CIRCUIT_OPEN: forwardv1.HealthState_HEALTH_STATE_CIRCUIT_OPEN,
}

// networkHealth answers a port forward's target health as its route's
// upstream health, checked by nodeRef on hop 0.
func networkHealth(nodeRef string, forward *networkv1.PortForwardStatus) []*forwardv1.UpstreamHealth {
	out := make([]*forwardv1.UpstreamHealth, 0, len(forward.GetTargets()))
	for _, target := range forward.GetTargets() {
		out = append(out, &forwardv1.UpstreamHealth{
			RouteId: forward.GetId(), Address: target.GetHost(), Port: target.GetPort(),
			State: networkHealthStates[target.GetState()], ConsecutiveFailures: target.GetConsecutiveFailures(),
			CircuitOpenUntilUnixMs: target.GetCircuitOpenUntilUnixMs(), RttUs: target.GetRttUs(),
			CheckedAtUnixMs: target.GetCheckedAtUnixMs(), NodeRef: nodeRef,
		})
	}
	return out
}

// networkDevice is what Control records of a device on the network model.
type networkDevice struct {
	// Known: the device has a row (it said Hello on network.v1 once).
	Known      bool
	Negotiated bool
	// Generation is the plan Control wants it to run.
	Generation uint64
	// Status is its latest NetworkStatus, nil without one; StatusAt when
	// it was observed.
	Status   *networkv1.NetworkStatus
	StatusAt time.Time
}

func loadNetworkDevices(db *gorm.DB, refs []string) (map[string]networkDevice, error) {
	out := map[string]networkDevice{}
	if len(refs) == 0 || !db.Migrator().HasTable(&model.NetworkDevice{}) {
		return out, nil
	}
	var rows []model.NetworkDevice
	if err := db.Where("node_ref IN ?", refs).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: load network devices: %w", err)
	}
	for _, row := range rows {
		device := networkDevice{Known: true, Negotiated: row.Negotiated, Generation: row.Generation}
		if row.StatusJSON != "" {
			status := &networkv1.NetworkStatus{}
			if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(row.StatusJSON), status); err == nil {
				device.Status = status
				if row.StatusAt != nil {
					device.StatusAt = *row.StatusAt
				}
			}
		}
		out[row.NodeRef] = device
	}
	return out, nil
}

// portForward answers the device's status of the route's port forward, nil
// when its status has none.
func (d networkDevice) portForward(routeID string) *networkv1.PortForwardStatus {
	for _, forward := range d.Status.GetPortForwards() {
		if forward.GetId() == routeID {
			return forward
		}
	}
	return nil
}

// networkRouteHealth answers an ENGINE_NETWORK route's health from its
// entry devices' latest status, in the order of the entries.
func networkRouteHealth(db *gorm.DB, route *forwardv1.Route) ([]*forwardv1.UpstreamHealth, error) {
	forward, _ := networkForward(route)
	devices, err := loadNetworkDevices(db, forward.Entries)
	if err != nil {
		return nil, err
	}
	var out []*forwardv1.UpstreamHealth
	for _, entry := range forward.Entries {
		if status := devices[entry].portForward(route.GetId()); status != nil {
			out = append(out, networkHealth(entry, status)...)
		}
	}
	return append(out, networkHopHealth(db, route, forward)...), nil
}

var componentHealthStates = map[networkv1.ComponentState]forwardv1.HealthState{
	networkv1.ComponentState_COMPONENT_STATE_APPLIED:  forwardv1.HealthState_HEALTH_STATE_HEALTHY,
	networkv1.ComponentState_COMPONENT_STATE_DEGRADED: forwardv1.HealthState_HEALTH_STATE_UNHEALTHY,
}

// networkHopHealth answers the health of each hop's own links: only the
// entries check targets, so a relay's or the exit's health is what its
// device reports of the route's transports (the link into the hop and the
// one out of it) and, on the last hop, of the exit egress, with the
// round trip the device measured. Address is the component id and Port 0.
func networkHopHealth(db *gorm.DB, route *forwardv1.Route, forward networkmodel.Forward) []*forwardv1.UpstreamHealth {
	var refs []string
	for _, hop := range route.GetHops() {
		refs = append(refs, hop.GetNodeRefs()...)
	}
	devices, err := loadNetworkDevices(db, refs)
	if err != nil {
		return nil
	}
	var out []*forwardv1.UpstreamHealth
	last := len(route.GetHops()) - 1
	for index, hop := range route.GetHops() {
		wanted := map[string]bool{}
		if index > 0 && index-1 < len(forward.Segments) {
			wanted[forward.Segments[index-1]] = true
		}
		if index < len(forward.Segments) {
			wanted[forward.Segments[index]] = true
		}
		if index == last && forward.Egress != "" {
			wanted[forward.Egress] = true
		}
		delete(wanted, "")
		for _, ref := range hop.GetNodeRefs() {
			device := devices[ref]
			if device.Status == nil {
				continue
			}
			for _, component := range device.Status.GetComponents() {
				if !wanted[component.GetId()] {
					continue
				}
				state := componentHealthStates[component.GetState()]
				if state == forwardv1.HealthState_HEALTH_STATE_UNSPECIFIED {
					state = forwardv1.HealthState_HEALTH_STATE_UNHEALTHY
				}
				out = append(out, &forwardv1.UpstreamHealth{
					RouteId: route.GetId(), HopIndex: uint32(index), Address: component.GetId(), // #nosec G115 -- bounded by MaxHops.
					State: state, RttUs: component.GetRttMs() * 1000, NodeRef: ref,
					CheckedAtUnixMs: device.StatusAt.UnixMilli(),
				})
			}
		}
	}
	return out
}

// deviceAddresses answers a device's public addresses, in their canonical
// form: its node's address (a proxy node's host, or server_ip for a host
// name), then the global unicast addresses of the interfaces its status
// reports.
func deviceAddresses(db *gorm.DB, ref string, status *networkv1.NetworkStatus) []string {
	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			prefix, err := netip.ParsePrefix(raw)
			if err != nil {
				return
			}
			addr = prefix.Addr()
		}
		addr = addr.Unmap()
		if !addr.IsGlobalUnicast() || addr.IsPrivate() || seen[addr.String()] {
			return
		}
		seen[addr.String()] = true
		out = append(out, addr.String())
	}
	if node, err := agentcontrol.ParseAgentNode(ref); err == nil && node.Kind == agentcontrol.NodeKindProxy {
		var rows []proxyRow
		if err := db.Model(&model.Node{}).Select("id", "host", "server_ip").Where("id = ?", node.ID).Limit(1).Find(&rows).Error; err == nil && len(rows) > 0 {
			add(rows[0].Host)
			if rows[0].ServerIP != nil {
				add(*rows[0].ServerIP)
			}
		}
	}
	for _, iface := range status.GetInterfaces() {
		for _, address := range iface.GetAddresses() {
			add(address)
		}
	}
	return out
}

// entryNAT answers the enabled node entries (NAT mappings) of the proxy
// nodes among refs, by node ref; empty when the table or a node is missing.
func entryNAT(db *gorm.DB, refs []string) map[string][]model.NodeEntry {
	out := map[string][]model.NodeEntry{}
	if !db.Migrator().HasTable(&model.NodeEntry{}) {
		return out
	}
	var ids []uint
	byID := map[uint]string{}
	for _, ref := range refs {
		if node, err := agentcontrol.ParseAgentNode(ref); err == nil && node.Kind == agentcontrol.NodeKindProxy {
			ids = append(ids, uint(node.ID))
			byID[uint(node.ID)] = ref
		}
	}
	rows, err := nodeentry.EnabledByNode(db, ids)
	if err != nil {
		return out
	}
	for id, entries := range rows {
		out[byID[id]] = entries
	}
	return out
}

// fillNetworkReports answers the nodes that run the network model with its
// report: their NetworkStatus stands in for the forward.v1 report an ANX
// device never sends. A forward.v1 report that is newer wins.
func fillNetworkReports(db *gorm.DB, summaries []*forwardv1.NodeSummary) error {
	refs := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		refs = append(refs, summary.GetNodeRef())
	}
	devices, err := loadNetworkDevices(db, refs)
	if err != nil {
		return err
	}
	for _, summary := range summaries {
		device, ok := devices[summary.GetNodeRef()]
		if !ok || device.Status == nil || device.StatusAt.IsZero() {
			continue
		}
		at := device.StatusAt.UnixMilli()
		if summary.GetReported() && summary.GetReportedAtUnixMs() >= at {
			continue
		}
		status := device.Status
		failed := 0
		for _, component := range status.GetComponents() {
			switch component.GetState() {
			case networkv1.ComponentState_COMPONENT_STATE_APPLIED, networkv1.ComponentState_COMPONENT_STATE_DEGRADED:
			default:
				failed++
			}
		}
		summary.Reported, summary.ReportedAtUnixMs = true, at
		summary.ReportedGeneration, summary.ReportedStateHash = status.GetGeneration(), status.GetPlanHash()
		summary.Applied = failed == 0 && status.GetGeneration() >= device.Generation
		summary.HopErrors = nonNegative32(failed)
		if summary.GetDesiredGeneration() == 0 {
			summary.DesiredGeneration = device.Generation
		}
	}
	return nil
}
