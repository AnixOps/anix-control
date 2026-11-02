package validate

import (
	"github.com/AnixOps/anix-control/sdk/forward/model"
)

// checkPorts refuses explicit ports outside a node's range (with an
// inventory) or reserved on it (forward-sdk.md sections 4.4 and 14). The
// entry listens on listen.port on every entry node; another hop on its own
// port on each of its nodes.
func checkPorts(c *collector, r *model.Route, opts Options) {
	nodes := nodeIndex(opts.Nodes)
	for i, h := range r.Hops {
		field, port := hopField(i, "port"), h.Port
		if i == 0 {
			field, port = "listen.port", r.Listen.Port
		}
		if port == 0 || port > MaxPort {
			continue
		}
		for _, ref := range h.NodeRefs {
			if n, ok := nodes[ref]; ok && !n.PortRange.IsZero() && !n.PortRange.Contains(port) {
				c.add(field, CodePortOutOfRange, "%d is outside %s's range %d-%d", port, ref, n.PortRange.First, n.PortRange.Last)
			}
			for _, reserved := range opts.ReservedPorts[ref] {
				if reserved == port {
					c.add(field, CodePortReserved, "%d is reserved on %s", port, ref)
					break
				}
			}
		}
	}
}

func nodeIndex(nodes []model.NodeInfo) map[string]*model.NodeInfo {
	index := make(map[string]*model.NodeInfo, len(nodes))
	for i := range nodes {
		index[nodes[i].NodeRef] = &nodes[i]
	}
	return index
}
