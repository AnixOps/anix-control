package kernelforward

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// ENGINE_NETWORK routes (network-model implementation.md step 5) are
// validated here but planned on the network model: the entry becomes a
// port forward of each entry device's NetworkPlan, each link a network model
// transport, the exit an internet egress. The network model asks this
// service for them; every accepted plan replans the network model in the
// same transaction, so a route and its port forward change together.

func init() { networkmodel.SetForwardSource(networkForwards) }

// networkForwards answers the stored ENGINE_NETWORK routes as the network
// model's forwards; a route the kernel enforces (expired, over its quota)
// is paused.
func networkForwards(db *gorm.DB) ([]networkmodel.Forward, error) {
	routes, err := loadRoutes(db)
	if err != nil {
		return nil, err
	}
	var forwards []networkmodel.Forward
	for _, stored := range routes {
		if forward, ok := networkForward(stored.route); ok {
			forward.Paused = forward.Paused || stored.row.Enforced != ""
			forwards = append(forwards, forward)
		}
	}
	return forwards, nil
}

// networkForward answers route as a network model forward, or false when
// its first hop is on neither the network model nor the kernel. The route
// was validated: its kernel hops (ENGINE_NFTABLES) come first, and the
// network hops after them start where the last kernel hop forwards to.
func networkForward(route *forwardv1.Route) (networkmodel.Forward, bool) {
	hops := route.GetHops()
	if len(hops) == 0 || len(hops[0].GetNodeRefs()) == 0 {
		return networkmodel.Forward{}, false
	}
	if engine := hops[0].GetEngine(); engine != forwardv1.Engine_ENGINE_NETWORK && engine != forwardv1.Engine_ENGINE_NFTABLES {
		return networkmodel.Forward{}, false
	}
	address := route.GetListen().GetAddress()
	if address == "" {
		address = "::"
	}
	listen := networkmodel.Endpoint{Address: address, Port: route.GetListen().GetPort()}
	forward := networkmodel.Forward{
		ID:        route.GetId(),
		Protocols: NetworkProtocols(route.GetListen().GetProtocol()),
		Policy:    NetworkPolicy(route),
		Paused:    route.GetPaused(),
	}
	for _, target := range route.GetTargets() {
		forward.Targets = append(forward.Targets, &networkv1.ForwardTarget{
			Host: target.GetHost(), Port: target.GetPort(), Weight: target.GetWeight(), Priority: target.GetPriority(),
		})
	}
	// hopListen answers where hop i listens: the route's listen on the
	// entry, else the hop's port (the route's listen port when 0).
	hopListen := func(i int) networkmodel.Endpoint {
		switch {
		case i == 0:
			return listen
		case hops[i].GetPort() == 0:
			return networkmodel.Endpoint{Address: "::", Port: listen.Port}
		}
		return networkmodel.Endpoint{Address: "::", Port: hops[i].GetPort()}
	}
	k := 0
	for k < len(hops) && hops[k].GetEngine() == forwardv1.Engine_ENGINE_NFTABLES {
		forward.Kernel = append(forward.Kernel, networkmodel.KernelHop{
			Nodes: append([]string(nil), hops[k].GetNodeRefs()...), Listen: hopListen(k),
			DialAddress: hops[k].GetDialAddress(), Policy: kernelPolicy(route, k == len(hops)-1),
		})
		k++
	}
	if k == len(hops) {
		return forward, true
	}
	forward.Entries = append([]string(nil), hops[k].GetNodeRefs()...)
	forward.Listen = hopListen(k)
	if k > 0 {
		forward.EntryDial = hops[k].GetDialAddress()
	}
	forward.Egress = hops[len(hops)-1].GetNetworkEgressId()
	for _, hop := range hops[k+1:] {
		forward.Segments = append(forward.Segments, hop.GetIngress().GetNetworkTransportId())
		forward.Links = append(forward.Links, networkLink(hop.GetIngress()))
		if len(hop.GetNodeRefs()) > 0 {
			forward.Via = append(forward.Via, hop.GetNodeRefs()[0])
		}
	}
	return forward, true
}

// kernelPolicy answers how a kernel hop balances and checks where it
// forwards: the next hop's nodes by policy.next_hop, the targets on the
// last hop by policy.target.
func kernelPolicy(route *forwardv1.Route, last bool) *networkv1.ForwardPolicy {
	policy := route.GetPolicy()
	balance := policy.GetNextHop()
	if last {
		balance = policy.GetTarget()
	}
	out := &networkv1.ForwardPolicy{Balance: NetworkBalances[balance]}
	if health := policy.GetHealth(); health != nil {
		out.Health = &networkv1.ForwardHealth{IntervalMs: health.GetIntervalMs(), TimeoutMs: health.GetTimeoutMs(), Disabled: health.GetDisabled()}
	}
	if proto.Equal(out, &networkv1.ForwardPolicy{}) {
		return nil
	}
	return out
}

// networkLinkModes maps a link's network_mode to the network model's.
var networkLinkModes = map[forwardv1.NetworkTransportMode]networkmodel.LinkMode{
	forwardv1.NetworkTransportMode_NETWORK_TRANSPORT_MODE_ANX_LINE:         networkmodel.LinkAnxLine,
	forwardv1.NetworkTransportMode_NETWORK_TRANSPORT_MODE_ANX_LINE_REVERSE: networkmodel.LinkAnxLineReverse,
}

// networkCarriers maps a bare ANX-Line's carriers to their ANX names.
var networkCarriers = map[forwardv1.NetworkCarrier]string{
	forwardv1.NetworkCarrier_NETWORK_CARRIER_TCP:  "c1",
	forwardv1.NetworkCarrier_NETWORK_CARRIER_QUIC: "c2",
}

// networkLink answers how Control links a hop that names no transport;
// the A2 jacket unless the link says otherwise.
func networkLink(link *forwardv1.LinkTransport) networkmodel.Link {
	out := networkmodel.Link{Mode: networkLinkModes[link.GetNetworkMode()]}
	if out.Mode != networkmodel.LinkAnxLine {
		return out
	}
	for _, carrier := range link.GetNetworkCarriers() {
		if name, ok := networkCarriers[carrier]; ok {
			out.Carriers = append(out.Carriers, name)
		}
	}
	out.Multipath, out.FEC = link.GetNetworkMultipath(), link.GetNetworkFec()
	return out
}

// NetworkProtocols answers the transports of a listen protocol.
func NetworkProtocols(protocol forwardv1.L4Protocol) []string {
	switch protocol {
	case forwardv1.L4Protocol_L4_PROTOCOL_UDP:
		return []string{"udp"}
	case forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP:
		return []string{"tcp", "udp"}
	default:
		return []string{"tcp"}
	}
}

// NetworkBalances names forward.v1 balance strategies as network.v1 does.
var NetworkBalances = map[forwardv1.BalanceStrategy]string{
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN: "round_robin",
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM:      "random",
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH:     "ip_hash",
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN:  "least_conn",
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER:    "failover",
}

// NetworkPolicy answers the route's target policy and limits as a port
// forward policy; nil when everything is the default.
func NetworkPolicy(route *forwardv1.Route) *networkv1.ForwardPolicy {
	policy := route.GetPolicy()
	out := &networkv1.ForwardPolicy{
		Balance:      NetworkBalances[policy.GetTarget()],
		BandwidthBps: route.GetLimits().GetBandwidthBps(),
		MaxConns:     route.GetLimits().GetMaxConns(),
	}
	if health := policy.GetHealth(); health != nil {
		out.Health = &networkv1.ForwardHealth{IntervalMs: health.GetIntervalMs(), TimeoutMs: health.GetTimeoutMs(), Disabled: health.GetDisabled()}
	}
	if breaker := policy.GetCircuitBreaker(); breaker != nil {
		out.CircuitBreaker = &networkv1.ForwardBreaker{FailureThreshold: breaker.GetFailureThreshold(), OpenMs: breaker.GetOpenMs()}
	}
	switch policy.GetProxyProtocol() {
	case forwardv1.ProxyProtocol_PROXY_PROTOCOL_V1:
		out.ProxyProtocol = "v1"
	case forwardv1.ProxyProtocol_PROXY_PROTOCOL_V2:
		out.ProxyProtocol = "v2"
	}
	if proto.Equal(out, &networkv1.ForwardPolicy{}) {
		return nil
	}
	return out
}

// networkEpochPrefix starts the counter epochs of port forwards.
const networkEpochPrefix = "network-"

// RecordNetworkCounters meters the port forwards of node's NetworkStatus
// as the entry hops of their ENGINE_NETWORK routes, and folds their target
// health checks into the latency history (latency.go). A port forward's
// bytes grow from the start of the device's kernel, so a lower value than
// the stored one starts a new counter epoch, named after when it was seen;
// a status older than the stored counter is skipped. Growth that reaches a
// route's quota replans, as a forward.v1 report does.
func (s *Service) RecordNetworkCounters(ctx context.Context, node agentcontrol.AgentNode, forwards []*networkv1.PortForwardStatus, observedAt time.Time) (ReportResult, error) {
	db, err := s.db(ctx)
	if err != nil {
		return ReportResult{}, err
	}
	for _, forward := range forwards {
		if forward.GetUpBytes() > math.MaxInt64 || forward.GetDownBytes() > math.MaxInt64 {
			return ReportResult{}, fmt.Errorf("%w: a counter exceeds %d", ErrInvalidReport, int64(math.MaxInt64))
		}
	}
	ref, observedAt := node.String(), observedAt.UTC()
	var result ReportResult
	var quotaRoutes []string
	err = db.Transaction(func(tx *gorm.DB) error {
		entryGrew := map[string]bool{}
		for _, forward := range forwards {
			var stored []model.Line
			if err := tx.Where("id = ?", forward.GetId()).Limit(1).Find(&stored).Error; err != nil {
				return err
			}
			if len(stored) == 0 {
				continue
			}
			if err := foldLatency(tx, ref, networkHealth(ref, forward), s.now()); err != nil {
				return err
			}
			// Only the devices clients connect to meter the route: the
			// later kernel hops and the network entry behind them carry the
			// same bytes again.
			route, err := decodeRoute(stored[0])
			if err != nil {
				return err
			}
			if hops := route.GetHops(); len(hops) == 0 || !slices.Contains(hops[0].GetNodeRefs(), ref) {
				continue
			}
			var rows []model.KernelForwardCounter
			if err := tx.Where("route_id = ? AND hop_index = ? AND node_ref = ? AND counter_epoch LIKE ?", forward.GetId(), 0, ref, networkEpochPrefix+"%").
				Order("observed_at DESC").Limit(1).Find(&rows).Error; err != nil {
				return fmt.Errorf("kernel forward: load counter: %w", err)
			}
			epoch := fmt.Sprintf("%s%d", networkEpochPrefix, observedAt.UnixMilli())
			if len(rows) > 0 {
				last := rows[0]
				if !observedAt.After(last.ObservedAt) {
					continue
				}
				if forward.GetUpBytes() >= last.UpBytes && forward.GetDownBytes() >= last.DownBytes {
					epoch = last.CounterEpoch
				}
			}
			grew, err := meter(tx, ref, &forwardv1.Counters{
				RouteId: forward.GetId(), CounterEpoch: epoch, UpBytes: forward.GetUpBytes(), DownBytes: forward.GetDownBytes(),
			}, observedAt)
			if err != nil {
				return err
			}
			if grew {
				result.Metered++
				entryGrew[forward.GetId()] = true
			}
		}
		quotaRoutes, err = quotaCandidates(tx, entryGrew)
		return err
	})
	if err != nil {
		return ReportResult{}, err
	}
	if len(quotaRoutes) > 0 {
		outcome, err := s.Replan(ctx, "quota")
		if err != nil {
			return result, err
		}
		result.Replanned = !outcome.Refused
	}
	return result, nil
}
