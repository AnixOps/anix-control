package kernelforward

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
)

// Diagnosis of an ENGINE_NETWORK route. Its devices run the network
// model, not the forward component, so the stages read the network model:
//
//  1. Control's records: per device, that it negotiated network.v1, that
//     its latest status runs the plan generation Control wants, and that
//     the route's components on it are up: the entry's port forward, the
//     anx_line transports between the hops and the exit's egress
//     (PROBE_KIND_CONFIG); each target's health from the entry's port
//     forward (PROBE_KIND_HEALTH). The entry's checks dial through the
//     route's whole path, so a healthy target is delivery proven end to
//     end. The target config steps are the forward routes' (stage 1 of
//     diagnose.go).
//  2. No node checks run: agent.diagnostic's forward checks read the
//     forward component's state. The nodes are listed with a note.
//  3. Control vantage: a TCP connect to the entry's listener, and to the
//     targets only when the entry reports no health for them.

// Step codes of network routes.
const (
	CodeNotOnNetwork      = "not_on_network_model"
	CodeComponentDown     = "component_down"
	CodeComponentDegraded = "component_degraded"
)

func (d *diagnosis) diagnoseNetwork(ctx context.Context, started time.Time) (*forwardv1.DiagnoseRouteResponse, error) {
	db, err := d.s.db(ctx)
	if err != nil {
		return nil, err
	}
	forward, _ := networkForward(d.route)
	refs := routeNodes(d.route)
	devices, err := loadNetworkDevices(db, refs)
	if err != nil {
		return nil, err
	}
	// The route's components: the transports into each later hop and the
	// exit's egress.
	components := map[string]bool{}
	for _, id := range forward.Segments {
		if id != "" {
			components[id] = true
		}
	}
	if forward.Egress != "" {
		components[forward.Egress] = true
	}
	for index, hop := range d.route.GetHops() {
		if len(hop.GetNodeRefs()) == 0 {
			continue
		}
		ref := hop.GetNodeRefs()[0]
		d.networkConfigStep(uint32(index), ref, devices[ref], components, index == 0) // #nosec G115 -- validate bounds hops to MaxHops.
	}
	entry := devices[forward.Entry]
	reported := d.networkHealthSteps(forward.Entry, entry)
	d.targetSteps(ctx)

	nodes := make([]*forwardv1.DiagnoseNode, 0, len(refs))
	for _, ref := range refs {
		node := &forwardv1.DiagnoseNode{NodeRef: ref,
			Note: "ANX routes are probed by the entry's target checks through the whole path; the forward checks of agent.diagnostic do not apply"}
		if agentNode, err := agentcontrol.ParseAgentNode(ref); err == nil && d.s.Checks != nil {
			node.Connected = d.s.Checks.Vantage(agentNode).Connected
		}
		nodes = append(nodes, node)
	}

	d.networkControlProbes(ctx, forward.Entry, deviceAddresses(db, forward.Entry, entry.Status), !reported)
	return d.answer(started, nodes), nil
}

func (d *diagnosis) networkConfigStep(index uint32, ref string, device networkDevice, components map[string]bool, entry bool) {
	step := &forwardv1.DiagnoseStep{NodeRef: ref, HopIndex: index, Kind: forwardv1.ProbeKind_PROBE_KIND_CONFIG, Vantage: vantageControl}
	defer d.add(step)
	status := device.Status
	if status != nil && !device.StatusAt.IsZero() {
		defer func() { step.Result.ObservedAtUnixMs = device.StatusAt.UnixMilli() }()
	}
	switch {
	case !device.Known || !device.Negotiated:
		step.Result = result(statusFailed, CodeNotOnNetwork, "the device does not run the network model: its Agent has not negotiated network.v1 (update the Agent)")
		return
	case status == nil:
		step.Result = result(statusFailed, CodeNeverReported, fmt.Sprintf("the device never reported its network status: Control wants plan generation %d", device.Generation))
		return
	case status.GetGeneration() < device.Generation:
		step.Result = result(statusFailed, CodeNotApplied, fmt.Sprintf("the device runs plan generation %d, Control wants %d", status.GetGeneration(), device.Generation))
		return
	}
	if entry {
		forward := device.portForward(d.route.GetId())
		switch {
		case forward == nil:
			step.Result = result(statusFailed, CodeNotApplied, "the device's status has no port forward for this route")
			return
		case forward.GetState() != networkv1.ComponentState_COMPONENT_STATE_APPLIED:
			step.Result = result(statusFailed, CodeHopError, "the port forward is "+componentState(forward.GetState())+detailSuffix(forward.GetDetail()))
			return
		}
	}
	var failed, degraded []string
	for _, component := range status.GetComponents() {
		if !components[component.GetId()] {
			continue
		}
		name := strings.ToLower(strings.TrimPrefix(component.GetComponent().String(), "COMPONENT_")) + " " + component.GetId()
		switch component.GetState() {
		case networkv1.ComponentState_COMPONENT_STATE_APPLIED:
		case networkv1.ComponentState_COMPONENT_STATE_DEGRADED:
			degraded = append(degraded, name+detailSuffix(component.GetDetail()))
		default:
			failed = append(failed, name+" is "+componentState(component.GetState())+detailSuffix(component.GetDetail()))
		}
	}
	switch {
	case len(failed) > 0:
		step.Result = result(statusFailed, CodeComponentDown, strings.Join(failed, "; "))
	case len(degraded) > 0:
		step.Result = result(statusInconclusive, CodeComponentDegraded, "degraded: "+strings.Join(degraded, "; "))
	default:
		step.Result = result(statusOK, "", fmt.Sprintf("plan generation %d applied", status.GetGeneration()))
	}
}

func componentState(state networkv1.ComponentState) string {
	return strings.ToLower(strings.TrimPrefix(state.String(), "COMPONENT_STATE_"))
}

func detailSuffix(detail string) string {
	if detail == "" {
		return ""
	}
	return ": " + detail
}

// networkHealthSteps adds a HEALTH step per target from the entry's port
// forward; reported is false when the entry reports no target health.
func (d *diagnosis) networkHealthSteps(ref string, device networkDevice) (reported bool) {
	forward := device.portForward(d.route.GetId())
	if forward == nil {
		return false
	}
	health := networkHealth(ref, forward)
	if len(health) == 0 {
		message := "the entry reports no target health: its Agent or anixcore predates target health"
		if tcp, _ := routeProtocols(d.route); !tcp {
			message = "a UDP-only route has no active health checks"
		} else if d.route.GetPolicy().GetHealth().GetDisabled() {
			message = "active health checks are disabled on this route"
		}
		d.add(&forwardv1.DiagnoseStep{NodeRef: ref, Kind: forwardv1.ProbeKind_PROBE_KIND_HEALTH, Vantage: vantageControl,
			Result: result(statusInconclusive, CodeNoHealth, message)})
		return false
	}
	for _, upstream := range health {
		step := &forwardv1.DiagnoseStep{NodeRef: ref, Kind: forwardv1.ProbeKind_PROBE_KIND_HEALTH, Vantage: vantageControl,
			Target: hostPort(upstream.GetAddress(), upstream.GetPort())}
		if upstream.GetState() == forwardv1.HealthState_HEALTH_STATE_UNSPECIFIED {
			step.Result = result(statusInconclusive, CodeNoHealth, "no check reached the target yet")
		} else {
			step.Result = healthResult(upstream, "reached from the entry through the route's whole path")
		}
		d.add(step)
	}
	return true
}

// networkControlProbes dials the entry's listener from Control, and the
// targets when withTargets (the entry reports no health for them).
func (d *diagnosis) networkControlProbes(ctx context.Context, entry string, addresses []string, withTargets bool) {
	tcp, _ := routeProtocols(d.route)
	port := d.route.GetListen().GetPort()
	host := d.route.GetListen().GetAddress()
	if addr, err := netip.ParseAddr(host); err != nil || addr.IsUnspecified() {
		host = ""
		if len(addresses) > 0 {
			host = addresses[0]
		}
	}
	var dials []*forwardv1.DiagnoseStep
	if host != "" && port != 0 {
		address := hostPort(host, port)
		if tcp {
			dials = append(dials, &forwardv1.DiagnoseStep{NodeRef: entry, Kind: forwardv1.ProbeKind_PROBE_KIND_LISTEN, Vantage: vantageControl, Target: address, Protocol: protoTCP})
		} else {
			d.add(&forwardv1.DiagnoseStep{NodeRef: entry, Kind: forwardv1.ProbeKind_PROBE_KIND_LISTEN, Vantage: vantageControl, Target: address, Protocol: protoUDP,
				Result: result(statusSkipped, CodeControlUDP, "Control does not probe a UDP listener")})
		}
	}
	if withTargets && tcp {
		last := uint32(len(d.route.GetHops()) - 1) // #nosec G115 -- validate bounds hops to MaxHops.
		for _, target := range d.route.GetTargets() {
			dials = append(dials, &forwardv1.DiagnoseStep{HopIndex: last, Kind: forwardv1.ProbeKind_PROBE_KIND_TCP_CONNECT, Vantage: vantageControl,
				Target: hostPort(target.GetHost(), target.GetPort()), Protocol: protoTCP})
		}
	}
	slots := make(chan struct{}, diagnoseProbeConcurrency)
	var wg sync.WaitGroup
	for _, step := range dials {
		wg.Add(1)
		go func(step *forwardv1.DiagnoseStep) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
			}
			step.Result = d.controlDial(ctx, step.GetTarget())
		}(step)
	}
	wg.Wait()
	for _, step := range dials {
		d.add(step)
	}
}
