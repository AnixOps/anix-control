package validate

import (
	"regexp"

	"github.com/AnixOps/anix-control/sdk/forward/model"
)

// networkIDPattern is a network model component id (a transport, an
// egress).
var networkIDPattern = regexp.MustCompile(`^[0-9A-Za-z_-]{1,64}$`)

// checkNetwork applies the rules of an ENGINE_NETWORK route (network-model
// implementation.md step 5): every hop is on the network model, on one
// device, except the entry, whose several devices are entry HA (each runs
// the port forward over a transport of its own into hop 1, or by an
// internet egress of its own, chosen by the network model); each link is
// a network model transport, which decides where its hop listens, or
// left to Control, which links the two devices with a transport of its
// own; the exit may name the internet egress that dials the targets, or
// leave it to Control (the exit device's first).
// The entry listens on an explicit port, since the network model allocates
// none.
func checkNetwork(c *collector, r *model.Route) {
	if r.Listen.Port == 0 {
		c.add("listen.port", CodeRequired, "an ENGINE_NETWORK route listens on an explicit port")
	}
	switch r.Policy.Direct {
	case model.DirectPreferred, model.DirectForced:
		c.add("policy.direct", CodeNotApplicable, "an ENGINE_NETWORK route always runs its hops")
	}
	last := len(r.Hops) - 1
	for i, h := range r.Hops {
		if h.Engine != model.EngineNetwork {
			c.add(hopField(i, "engine"), CodeInvalidRelation, "a route runs ENGINE_NETWORK on every hop or on none")
			continue
		}
		if i > 0 && len(h.NodeRefs) > 1 {
			c.add(hopField(i, "node_refs"), CodeRequiresSingleNode, "an ENGINE_NETWORK hop after the entry is one device")
		}
		switch {
		case i == last && h.NetworkEgressID == "":
			// Control picks the exit device's internet egress.
		case i == last && !networkIDPattern.MatchString(h.NetworkEgressID):
			c.add(hopField(i, "network_egress_id"), CodeInvalidFormat, "must be a network model component id")
		case i < last && h.NetworkEgressID != "":
			c.add(hopField(i, "network_egress_id"), CodeNotApplicable, "only the exit has an egress")
		}
		if i == 0 {
			continue
		}
		if h.Port != 0 {
			c.add(hopField(i, "port"), CodeNotApplicable, "the network model's transport decides where hop %d listens", i)
		}
		if h.DialAddress != "" {
			c.add(hopField(i, "dial_address"), CodeNotApplicable, "the network model's transport decides where hop %d is dialed", i)
		}
		t, field := h.Ingress, hopField(i, "ingress.")
		switch {
		case t.NetworkTransportID == "" && len(h.NodeRefs) == 0:
			c.add(hopField(i, "node_refs"), CodeRequired, "a LINK_SECURITY_NETWORK link names its device or its network model transport")
		case t.NetworkTransportID == "":
			// Control links the previous device to this one.
		case !networkIDPattern.MatchString(t.NetworkTransportID):
			c.add(field+"network_transport_id", CodeInvalidFormat, "must be a network model component id")
		}
		if t.Mux || t.ServerName != "" || t.Path != "" {
			c.add(field+"security", CodeNotApplicable, "the network model's transport has its own settings; mux, server_name and path are empty")
		}
	}
}

// checkNetworkFields refuses the network model's fields outside an
// ENGINE_NETWORK route.
func checkNetworkFields(c *collector, i int, h model.Hop) {
	if h.Engine == model.EngineNetwork {
		return
	}
	if h.NetworkEgressID != "" {
		c.add(hopField(i, "network_egress_id"), CodeNotApplicable, "only an ENGINE_NETWORK exit has an egress")
	}
	if h.Ingress.NetworkTransportID != "" && h.Ingress.Security != model.LinkSecurityNetwork {
		c.add(hopField(i, "ingress.network_transport_id"), CodeNotApplicable, "only a LINK_SECURITY_NETWORK link has a network transport")
	}
}
