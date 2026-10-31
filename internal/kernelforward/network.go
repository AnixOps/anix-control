package kernelforward

import (
	"context"
	"fmt"
	"math"
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
// it is not an ENGINE_NETWORK route. The route was validated.
func networkForward(route *forwardv1.Route) (networkmodel.Forward, bool) {
	hops := route.GetHops()
	if len(hops) == 0 || hops[0].GetEngine() != forwardv1.Engine_ENGINE_NETWORK || len(hops[0].GetNodeRefs()) == 0 {
		return networkmodel.Forward{}, false
	}
	address := route.GetListen().GetAddress()
	if address == "" {
		address = "::"
	}
	forward := networkmodel.Forward{
		ID:        route.GetId(),
		Entries:   append([]string(nil), hops[0].GetNodeRefs()...),
		Listen:    networkmodel.Endpoint{Address: address, Port: route.GetListen().GetPort()},
		Protocols: networkProtocols(route.GetListen().GetProtocol()),
		Policy:    networkPolicy(route),
		Egress:    hops[len(hops)-1].GetNetworkEgressId(),
		Paused:    route.GetPaused(),
	}
	for _, hop := range hops[1:] {
		forward.Segments = append(forward.Segments, hop.GetIngress().GetNetworkTransportId())
		forward.Links = append(forward.Links, networkLink(hop.GetIngress()))
		if len(hop.GetNodeRefs()) > 0 {
			forward.Via = append(forward.Via, hop.GetNodeRefs()[0])
		}
	}
	for _, target := range route.GetTargets() {
		forward.Targets = append(forward.Targets, &networkv1.ForwardTarget{
			Host: target.GetHost(), Port: target.GetPort(), Weight: target.GetWeight(), Priority: target.GetPriority(),
		})
	}
	return forward, true
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

func networkProtocols(protocol forwardv1.L4Protocol) []string {
	switch protocol {
	case forwardv1.L4Protocol_L4_PROTOCOL_UDP:
		return []string{"udp"}
	case forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP:
		return []string{"tcp", "udp"}
	default:
		return []string{"tcp"}
	}
}

var networkBalances = map[forwardv1.BalanceStrategy]string{
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN: "round_robin",
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM:      "random",
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH:     "ip_hash",
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN:  "least_conn",
	forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER:    "failover",
}

// networkPolicy answers the route's target policy and limits as a port
// forward policy; nil when everything is the default.
func networkPolicy(route *forwardv1.Route) *networkv1.ForwardPolicy {
	policy := route.GetPolicy()
	out := &networkv1.ForwardPolicy{
		Balance:      networkBalances[policy.GetTarget()],
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
			var known int64
			if err := tx.Model(&model.KernelForwardRoute{}).Where("id = ?", forward.GetId()).Count(&known).Error; err != nil {
				return err
			}
			if known == 0 {
				continue
			}
			if err := foldLatency(tx, ref, networkHealth(ref, forward), s.now()); err != nil {
				return err
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
