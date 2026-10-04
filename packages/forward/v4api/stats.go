package v4api

import (
	"context"
	"net"
	"net/http"
	"sort"
	"strconv"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// The traffic ledger and the observability views. Traffic is the raw
// metered bytes the nodes reported (no billing multiplier). The kernel
// keeps each node's latest health and latency probe, not their history:
// the trend is the hourly traffic, and latency is the latest probe.

// maxObservedRoutes bounds the routes one observability answer reads.
const maxObservedRoutes = 2000

// Total is one route hop's metered traffic on one node over a window.
type Total struct {
	RouteID     string `json:"route_id"`
	HopIndex    uint32 `json:"hop_index"`
	NodeRef     string `json:"node_ref"`
	UpBytes     uint64 `json:"up_bytes"`
	DownBytes   uint64 `json:"down_bytes"`
	UpPackets   uint64 `json:"up_packets"`
	DownPackets uint64 `json:"down_packets"`
	NewConns    uint64 `json:"new_conns"`
}

func (t *Total) add(bucket *forwardv1.TrafficBucket) {
	t.UpBytes += bucket.GetUpBytes()
	t.DownBytes += bucket.GetDownBytes()
	t.UpPackets += bucket.GetUpPackets()
	t.DownPackets += bucket.GetDownPackets()
	t.NewConns += bucket.GetNewConns()
}

func (s *Service) traffic(ctx context.Context, request Request) (*forwardv1.GetTrafficResponse, *Response) {
	since, until, bad := window(request)
	if bad != nil {
		return nil, bad
	}
	answer, err := s.Forward.GetTraffic(ctx, &forwardv1.GetTrafficRequest{
		RouteId: query(request, "route_id"), NodeRef: query(request, "node_ref"), SinceUnixMs: since, UntilUnixMs: until,
	})
	if err != nil {
		failed := fromStatus(err)
		return nil, &failed
	}
	return answer, nil
}

// stats answers the window's totals per route, hop and node, per node, and
// the hourly buckets. Filters: route_id, node_ref, since, until (Unix
// milliseconds; the last 24 hours by default, at most 31 days).
func (s *Service) stats(ctx context.Context, request Request, _ map[string]string) Response {
	answer, bad := s.traffic(ctx, request)
	if bad != nil {
		return *bad
	}
	byHop := map[[3]string]*Total{}
	byNode := map[string]*Total{}
	for _, bucket := range answer.GetBuckets() {
		key := [3]string{bucket.GetRouteId(), strconv.FormatUint(uint64(bucket.GetHopIndex()), 10), bucket.GetNodeRef()}
		if byHop[key] == nil {
			byHop[key] = &Total{RouteID: bucket.GetRouteId(), HopIndex: bucket.GetHopIndex(), NodeRef: bucket.GetNodeRef()}
		}
		byHop[key].add(bucket)
		if byNode[bucket.GetNodeRef()] == nil {
			byNode[bucket.GetNodeRef()] = &Total{NodeRef: bucket.GetNodeRef()}
		}
		byNode[bucket.GetNodeRef()].add(bucket)
	}
	totals := make([]*Total, 0, len(byHop))
	for _, total := range byHop {
		totals = append(totals, total)
	}
	sort.Slice(totals, func(i, j int) bool {
		a, b := totals[i], totals[j]
		if a.RouteID != b.RouteID {
			return a.RouteID < b.RouteID
		}
		if a.HopIndex != b.HopIndex {
			return a.HopIndex < b.HopIndex
		}
		return a.NodeRef < b.NodeRef
	})
	nodes := make([]*Total, 0, len(byNode))
	for _, total := range byNode {
		nodes = append(nodes, total)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeRef < nodes[j].NodeRef })
	return data(http.StatusOK, map[string]any{
		"totals": totals, "nodes": nodes, "series": pjList(answer.GetBuckets()), "truncated": answer.GetTruncated(),
	})
}

// TrendPoint is one hour of traffic summed over the filter.
type TrendPoint struct {
	HourStartUnixMs int64  `json:"hour_start_unix_ms"`
	UpBytes         uint64 `json:"up_bytes"`
	DownBytes       uint64 `json:"down_bytes"`
	NewConns        uint64 `json:"new_conns"`
}

// observabilityTrend answers the hourly traffic summed over the filter
// (route_id, node_ref, since, until).
func (s *Service) observabilityTrend(ctx context.Context, request Request, _ map[string]string) Response {
	answer, bad := s.traffic(ctx, request)
	if bad != nil {
		return *bad
	}
	byHour := map[int64]*TrendPoint{}
	for _, bucket := range answer.GetBuckets() {
		hour := bucket.GetHourStartUnixMs()
		if byHour[hour] == nil {
			byHour[hour] = &TrendPoint{HourStartUnixMs: hour}
		}
		byHour[hour].UpBytes += bucket.GetUpBytes()
		byHour[hour].DownBytes += bucket.GetDownBytes()
		byHour[hour].NewConns += bucket.GetNewConns()
	}
	points := make([]*TrendPoint, 0, len(byHour))
	for _, point := range byHour {
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].HourStartUnixMs < points[j].HourStartUnixMs })
	return data(http.StatusOK, map[string]any{"points": points, "truncated": answer.GetTruncated()})
}

// allRoutes pages through every route, at most maxObservedRoutes.
func (s *Service) allRoutes(ctx context.Context, nodeRef string) ([]*forwardv1.Route, bool, error) {
	var out []*forwardv1.Route
	token := ""
	for {
		page, err := s.Forward.ListRoutes(ctx, &forwardv1.ListRoutesRequest{NodeRef: nodeRef, PageSize: 1000, PageToken: token})
		if err != nil {
			return nil, false, err
		}
		out = append(out, page.GetRoutes()...)
		if len(out) >= maxObservedRoutes {
			return out[:maxObservedRoutes], true, nil
		}
		if token = page.GetNextPageToken(); token == "" {
			return out, false, nil
		}
	}
}

// TargetHealth is one target of one route with what the last hop's nodes
// report about it.
type TargetHealth struct {
	RouteID   string `json:"route_id"`
	RouteName string `json:"route_name"`
	Paused    bool   `json:"paused"`
	// Key is "host:port".
	Key      string `json:"key"`
	Host     string `json:"host"`
	Port     uint32 `json:"port"`
	Weight   uint32 `json:"weight"`
	Priority uint32 `json:"priority"`
	// State is the worst state the reporting nodes see (HEALTH_STATE_*);
	// HEALTH_STATE_UNSPECIFIED before any report.
	State string `json:"state"`
	// Healthy and Reports count the nodes that see it healthy and that
	// report it at all.
	Healthy int `json:"healthy"`
	Reports int `json:"reports"`
	// RttUs is the best latest connect round trip; 0 when none succeeded.
	RttUs            uint32 `json:"rtt_us"`
	CheckedAtUnixMs  int64  `json:"checked_at_unix_ms"`
	CircuitOpenUntil int64  `json:"circuit_open_until_unix_ms,omitempty"`
}

var healthRank = map[forwardv1.HealthState]int{
	forwardv1.HealthState_HEALTH_STATE_UNSPECIFIED:  0,
	forwardv1.HealthState_HEALTH_STATE_HEALTHY:      1,
	forwardv1.HealthState_HEALTH_STATE_UNHEALTHY:    2,
	forwardv1.HealthState_HEALTH_STATE_CIRCUIT_OPEN: 3,
}

// observabilityTargets answers every route's targets with their health
// (node_ref keeps the routes with a hop on one node).
func (s *Service) observabilityTargets(ctx context.Context, request Request, _ map[string]string) Response {
	routes, truncated, err := s.allRoutes(ctx, query(request, "node_ref"))
	if err != nil {
		return fromStatus(err)
	}
	targets := []*TargetHealth{}
	for _, route := range routes {
		health, err := s.Forward.GetRouteHealth(ctx, &forwardv1.GetRouteHealthRequest{RouteId: route.GetId()})
		if err != nil {
			return fromStatus(err)
		}
		last := uint32(0)
		if hops := len(route.GetHops()); hops > 0 {
			last = uint32(hops - 1) // #nosec G115 -- validation caps the hops.
		}
		for _, target := range route.GetTargets() {
			item := &TargetHealth{
				RouteID: route.GetId(), RouteName: route.GetName(), Paused: route.GetPaused(),
				Key:  net.JoinHostPort(target.GetHost(), strconv.FormatUint(uint64(target.GetPort()), 10)),
				Host: target.GetHost(), Port: target.GetPort(), Weight: target.GetWeight(), Priority: target.GetPriority(),
				State: forwardv1.HealthState_HEALTH_STATE_UNSPECIFIED.String(),
			}
			worst := forwardv1.HealthState_HEALTH_STATE_UNSPECIFIED
			for _, upstream := range health.GetHealth() {
				if upstream.GetHopIndex() != last || upstream.GetAddress() != target.GetHost() || upstream.GetPort() != target.GetPort() {
					continue
				}
				item.Reports++
				if upstream.GetState() == forwardv1.HealthState_HEALTH_STATE_HEALTHY {
					item.Healthy++
				}
				if healthRank[upstream.GetState()] > healthRank[worst] {
					worst = upstream.GetState()
				}
				if rtt := upstream.GetRttUs(); rtt > 0 && (item.RttUs == 0 || rtt < item.RttUs) {
					item.RttUs = rtt
				}
				if checked := upstream.GetCheckedAtUnixMs(); checked > item.CheckedAtUnixMs {
					item.CheckedAtUnixMs = checked
				}
				if open := upstream.GetCircuitOpenUntilUnixMs(); open > item.CircuitOpenUntil {
					item.CircuitOpenUntil = open
				}
			}
			item.State = worst.String()
			targets = append(targets, item)
		}
	}
	return data(http.StatusOK, map[string]any{"targets": targets, "total": len(targets), "truncated": truncated})
}

// TopologyNode is a node of the graph.
type TopologyNode struct {
	ID string `json:"id"`
	// Kind is "forward", "proxy" or "target".
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	InInventory bool   `json:"in_inventory"`
	Reported    bool   `json:"reported"`
	Applied     bool   `json:"applied"`
	HopErrors   uint32 `json:"hop_errors"`
	// Lagging is true when the node runs an older generation than its
	// desired one, or never reported one.
	Lagging bool `json:"lagging"`
}

// TopologyEdge is one link of one route: hop HopIndex's node to the next
// hop's node, or the last hop's node to a target.
type TopologyEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	RouteID  string `json:"route_id"`
	HopIndex uint32 `json:"hop_index"`
	Engine   string `json:"engine"`
	// Security is the link's transport (LINK_SECURITY_*); empty to a
	// target.
	Security string `json:"security,omitempty"`
	Paused   bool   `json:"paused"`
}

// observabilityTopology answers the nodes, targets and links of every
// route as a graph.
func (s *Service) observabilityTopology(ctx context.Context, _ Request, _ map[string]string) Response {
	listed, err := s.Forward.ListNodes(ctx, &forwardv1.ListNodesRequest{})
	if err != nil {
		return fromStatus(err)
	}
	routes, truncated, err := s.allRoutes(ctx, "")
	if err != nil {
		return fromStatus(err)
	}
	nodes := []*TopologyNode{}
	seen := map[string]bool{}
	for _, node := range listed.GetNodes() {
		seen[node.GetNodeRef()] = true
		nodes = append(nodes, &TopologyNode{
			ID: node.GetNodeRef(), Kind: node.GetKind(), Name: node.GetName(), InInventory: node.GetInInventory(),
			Reported: node.GetReported(), Applied: node.GetApplied(), HopErrors: node.GetHopErrors(),
			Lagging: node.GetDesiredGeneration() > 0 && (!node.GetReported() || node.GetReportedGeneration() < node.GetDesiredGeneration()),
		})
	}
	edges := []*TopologyEdge{}
	for _, route := range routes {
		hops := route.GetHops()
		for i, hop := range hops {
			index := uint32(i) // #nosec G115 -- validation caps the hops.
			if i+1 < len(hops) {
				next := hops[i+1]
				for _, from := range hop.GetNodeRefs() {
					for _, to := range next.GetNodeRefs() {
						edges = append(edges, &TopologyEdge{
							From: from, To: to, RouteID: route.GetId(), HopIndex: index, Engine: hop.GetEngine().String(),
							Security: next.GetIngress().GetSecurity().String(), Paused: route.GetPaused(),
						})
					}
				}
				continue
			}
			for _, target := range route.GetTargets() {
				key := net.JoinHostPort(target.GetHost(), strconv.FormatUint(uint64(target.GetPort()), 10))
				if !seen[key] {
					seen[key] = true
					nodes = append(nodes, &TopologyNode{ID: key, Kind: "target", Name: key})
				}
				for _, from := range hop.GetNodeRefs() {
					edges = append(edges, &TopologyEdge{From: from, To: key, RouteID: route.GetId(), HopIndex: index, Engine: hop.GetEngine().String(), Paused: route.GetPaused()})
				}
			}
		}
	}
	return data(http.StatusOK, map[string]any{"nodes": nodes, "edges": edges, "truncated": truncated})
}
