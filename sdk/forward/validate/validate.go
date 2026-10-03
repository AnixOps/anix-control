package validate

import (
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/model"
	"google.golang.org/protobuf/proto"
)

// Options are what validation knows beyond the route itself. The zero
// Options run every rule that needs only the route.
type Options struct {
	// Nodes is the node inventory (Control's, or PlanRouteRequest.nodes).
	// When it is empty the inventory rules are skipped, not passed: node
	// existence, advertised engines and their capabilities, port ranges.
	Nodes []model.NodeInfo
	// ReservedPorts are the ports each node keeps for itself (SSH, the
	// Agent's ports, a per-node list), by node reference.
	ReservedPorts map[string][]uint32
	// EnableAnixOps allows ENGINE_ANIXOPS and LINK_SECURITY_ANIXOPS. It
	// stays false until the AnixOps protocol ships (v4.3); even then a node
	// must advertise the engine.
	EnableAnixOps bool
	// OnCreate with a non-zero Now requires limits.expires_at_unix_ms, when
	// set, to be after Now. An update may keep an expiry that has passed.
	OnCreate bool
	Now      time.Time
}

// Route validates a route and answers every violation; none means valid.
// It is pure: no DNS, no clock but Options.Now.
func Route(r *model.Route, opts Options) Violations {
	c := &collector{}
	if r == nil {
		c.add("", CodeRequired, "a route is required")
		return c.out
	}
	if size := proto.Size(r.ToProto()); size > MaxRouteBytes {
		// A route over the cap is not checked further.
		c.add("", CodeTooLarge, "the route encodes to %d bytes, more than %d", size, MaxRouteBytes)
		return c.out
	}
	checkMeta(c, r)
	checkListen(c, r)
	checkHops(c, r, opts)
	checkLinks(c, r)
	checkTargets(c, r)
	checkPolicy(c, r)
	checkLimits(c, r, opts)
	checkLabels(c, r)
	checkPorts(c, r, opts)
	if len(opts.Nodes) > 0 {
		checkInventory(c, r, opts)
	}
	return c.out
}

// Proto validates a contract route, as Route does.
func Proto(r *forwardv1.Route, opts Options) Violations {
	if r == nil {
		return Route(nil, opts)
	}
	if size := proto.Size(r); size > MaxRouteBytes {
		c := &collector{}
		c.add("", CodeTooLarge, "the route encodes to %d bytes, more than %d", size, MaxRouteBytes)
		return c.out
	}
	m := model.FromProto(r)
	return Route(&m, opts)
}

// PlanRequest validates a PlanRouteRequest's route with its nodes added to
// opts.Nodes (they replace inventory nodes of the same reference, as in
// PlanRoute).
func PlanRequest(req *forwardv1.PlanRouteRequest, opts Options) Violations {
	if len(req.GetNodes()) > 0 {
		replaced := map[string]bool{}
		nodes := model.NodesFromProto(req.GetNodes())
		for _, n := range nodes {
			replaced[n.NodeRef] = true
		}
		for _, n := range opts.Nodes {
			if !replaced[n.NodeRef] {
				nodes = append(nodes, n)
			}
		}
		opts.Nodes = nodes
	}
	return Proto(req.GetRoute(), opts)
}
