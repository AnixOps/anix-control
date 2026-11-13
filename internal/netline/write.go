package netline

import (
	"context"
	"fmt"
	"slices"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/routesubscription"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// Draft is a line as the administrator writes it (the line API): its
// chain of devices, the first hop's are the entries and the last hop's
// device is the exit, and either the protocol it serves there (by id) or
// fixed targets. A device forwards in the kernel when it is on a cloud
// that scans its machines (its network settings), in userspace otherwise.
type Draft struct {
	Name       string                     `json:"name"`
	Enabled    bool                       `json:"enabled"`
	Show       bool                       `json:"show"`
	Port       uint32                     `json:"port"`
	L4         []string                   `json:"l4"`
	Chain      []Hop                      `json:"chain"`
	ProtocolID uint                       `json:"protocol_id,omitempty"`
	Targets    []*networkv1.ForwardTarget `json:"targets,omitempty"`
	// Policy, HopBalance, QuotaBytes and ExpiresAt are the line's, as the
	// line API reads them; they replace the stored ones.
	Policy     *networkv1.ForwardPolicy `json:"policy,omitempty"`
	HopBalance string                   `json:"hop_balance,omitempty"`
	QuotaBytes uint64                   `json:"quota_bytes,omitempty"`
	ExpiresAt  int64                    `json:"expires_at_unix_ms,omitempty"`
	// Revision is the stored line's on an update.
	Revision uint64 `json:"revision,omitempty"`
}

// Writer stores drafts as forward routes (kernelforward validates and
// plans them) and offers them in subscriptions (routesubscription).
type Writer struct {
	DB      *gorm.DB
	Forward *kernelforward.Service
	Offers  *routesubscription.Service
	Actor   routesubscription.Actor
}

// Create stores a new line and answers its id.
func (w *Writer) Create(ctx context.Context, requestID string, d Draft) (string, error) {
	route, err := w.route(d, &forwardv1.Route{Owner: "admin"})
	if err != nil {
		return "", err
	}
	created, err := w.Forward.CreateRoute(ctx, requestID, route)
	if err != nil {
		return "", err
	}
	return created.GetId(), w.offer(ctx, created.GetId(), d.Show)
}

// Update replaces line id at d.Revision. What a draft does not hold (the
// route's labels, a hop's transport while its devices stay) is kept.
func (w *Writer) Update(ctx context.Context, requestID, id string, d Draft) error {
	stored, err := w.Forward.GetRoute(ctx, id)
	if err != nil {
		return err
	}
	route, err := w.route(d, proto.Clone(stored).(*forwardv1.Route))
	if err != nil {
		return err
	}
	if _, err := w.Forward.UpdateRoute(ctx, requestID, route, d.Revision); err != nil {
		return err
	}
	return w.offer(ctx, id, d.Show)
}

// Delete removes line id and its subscription offer.
func (w *Writer) Delete(ctx context.Context, requestID, id string) error {
	if err := w.Forward.DeleteRoute(ctx, requestID, id); err != nil {
		return err
	}
	return w.DB.WithContext(ctx).Where("route_id = ?", id).Delete(&model.ForwardRouteSubscription{}).Error
}

func (w *Writer) offer(ctx context.Context, id string, show bool) error {
	current, err := w.Offers.Get(ctx, id)
	if err != nil || current.Published == show {
		return err
	}
	_, err = w.Offers.Set(ctx, id, show, current.DisplayName, w.Actor)
	return err
}

// route writes d onto route.
func (w *Writer) route(d Draft, route *forwardv1.Route) (*forwardv1.Route, error) {
	if len(d.Chain) == 0 {
		return nil, fmt.Errorf("%w: a line needs at least its entry hop", kernelforward.ErrInvalidRequest)
	}
	var kernel []string
	if err := w.DB.Model(&model.NodeNetwork{}).Where("scan_cloud = ?", true).Pluck("node_ref", &kernel).Error; err != nil {
		return nil, err
	}
	route.Name, route.Paused = d.Name, !d.Enabled
	if route.Listen == nil {
		route.Listen = &forwardv1.Listen{}
	}
	route.Listen.Port, route.Listen.Protocol = d.Port, l4Protocol(d.L4)
	if err := policy(d, route); err != nil {
		return nil, err
	}
	hops := make([]*forwardv1.Hop, len(d.Chain))
	for i, step := range d.Chain {
		hop := &forwardv1.Hop{}
		if i < len(route.Hops) && slices.Equal(route.Hops[i].GetNodeRefs(), step.Devices) {
			hop = route.Hops[i]
		}
		hop.Role, hop.NodeRefs, hop.Engine = role(i, len(d.Chain)), step.Devices, forwardv1.Engine_ENGINE_NETWORK
		if len(step.Devices) > 0 && slices.Contains(kernel, step.Devices[0]) {
			hop.Engine = forwardv1.Engine_ENGINE_NFTABLES
		}
		if i > 0 {
			hop.Port, hop.DialAddress = step.Port, step.Dial
			// A kernel hop sends the client's bytes as they are; the
			// others link over the network model.
			if hop.Ingress == nil {
				hop.Ingress = &forwardv1.LinkTransport{}
			}
			hop.Ingress.Security = forwardv1.LinkSecurity_LINK_SECURITY_NETWORK
			if hops[i-1].Engine == forwardv1.Engine_ENGINE_NFTABLES {
				hop.Ingress.Security = forwardv1.LinkSecurity_LINK_SECURITY_RAW
			}
		}
		hops[i] = hop
	}
	route.Hops, route.Targets = hops, nil
	if d.ProtocolID == 0 {
		for _, t := range d.Targets {
			route.Targets = append(route.Targets, &forwardv1.Target{Host: t.GetHost(), Port: t.GetPort(), Weight: t.GetWeight(), Priority: t.GetPriority()})
		}
		return route, nil
	}
	// The exit device serves the protocol: the line ends at its host and
	// port.
	var protocol model.NodeProtocol
	if err := w.DB.First(&protocol, d.ProtocolID).Error; err != nil {
		return nil, err
	}
	if exit := d.Chain[len(d.Chain)-1].Devices; len(exit) != 1 || exit[0] != ref(protocol.NodeID) {
		return nil, fmt.Errorf("%w: protocol %d is not on the line's exit device", kernelforward.ErrInvalidRequest, d.ProtocolID)
	}
	var node model.Node
	if err := w.DB.First(&node, protocol.NodeID).Error; err != nil {
		return nil, err
	}
	route.Targets = []*forwardv1.Target{{Host: node.Host, Port: uint32(protocol.Port)}} // #nosec G115 -- a port.
	return route, nil
}

// policy writes d's policy and limits onto route; the route's direct mode
// and target policy, which lines do not use, stay.
func policy(d Draft, route *forwardv1.Route) error {
	p := d.Policy
	target, ok := balances[p.GetBalance()]
	nextHop, hopOK := balances[d.HopBalance]
	if !ok || !hopOK {
		return fmt.Errorf("%w: unknown balance %q or hop_balance %q", kernelforward.ErrInvalidRequest, p.GetBalance(), d.HopBalance)
	}
	proxy, ok := proxyProtocols[p.GetProxyProtocol()]
	if !ok {
		return fmt.Errorf("%w: unknown proxy_protocol %q", kernelforward.ErrInvalidRequest, p.GetProxyProtocol())
	}
	if route.Policy == nil {
		route.Policy = &forwardv1.Policy{}
	}
	route.Policy.Target, route.Policy.NextHop, route.Policy.ProxyProtocol = target, nextHop, proxy
	route.Policy.Health, route.Policy.CircuitBreaker = nil, nil
	if h := p.GetHealth(); h != nil {
		route.Policy.Health = &forwardv1.HealthCheck{IntervalMs: h.GetIntervalMs(), TimeoutMs: h.GetTimeoutMs(), Disabled: h.GetDisabled()}
	}
	if b := p.GetCircuitBreaker(); b != nil {
		route.Policy.CircuitBreaker = &forwardv1.CircuitBreaker{FailureThreshold: b.GetFailureThreshold(), OpenMs: b.GetOpenMs()}
	}
	route.Limits = &forwardv1.Limits{BandwidthBps: p.GetBandwidthBps(), MaxConns: p.GetMaxConns(), QuotaBytes: d.QuotaBytes, ExpiresAtUnixMs: d.ExpiresAt}
	if proto.Equal(route.Limits, &forwardv1.Limits{}) {
		route.Limits = nil
	}
	return nil
}

// balances is kernelforward.NetworkBalances the other way; empty is the
// default.
var balances = func() map[string]forwardv1.BalanceStrategy {
	out := map[string]forwardv1.BalanceStrategy{"": forwardv1.BalanceStrategy_BALANCE_STRATEGY_UNSPECIFIED}
	for strategy, name := range kernelforward.NetworkBalances {
		out[name] = strategy
	}
	return out
}()

var proxyProtocols = map[string]forwardv1.ProxyProtocol{
	"":   forwardv1.ProxyProtocol_PROXY_PROTOCOL_UNSPECIFIED,
	"v1": forwardv1.ProxyProtocol_PROXY_PROTOCOL_V1,
	"v2": forwardv1.ProxyProtocol_PROXY_PROTOCOL_V2,
}

func role(i, n int) forwardv1.HopRole {
	switch i {
	case 0:
		return forwardv1.HopRole_HOP_ROLE_ENTRY
	case n - 1:
		return forwardv1.HopRole_HOP_ROLE_EXIT
	}
	return forwardv1.HopRole_HOP_ROLE_RELAY
}

// l4Protocol is kernelforward.NetworkProtocols the other way.
func l4Protocol(l4 []string) forwardv1.L4Protocol {
	switch {
	case slices.Contains(l4, "tcp") && slices.Contains(l4, "udp"):
		return forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP
	case slices.Contains(l4, "udp"):
		return forwardv1.L4Protocol_L4_PROTOCOL_UDP
	}
	return forwardv1.L4Protocol_L4_PROTOCOL_TCP
}
