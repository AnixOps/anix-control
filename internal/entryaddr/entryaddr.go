// Package entryaddr answers the public address a route's entry node is
// reached at, for the clients of its subscription and for Control's own
// probe. A machine behind provider NAT cannot be reached at its node host
// and the route's listen port, so the address comes from, in order:
//
//  1. the route's own override, a route label "entry-public/<node ref>"
//     whose value is "host", "host:port" or "[v6]:port" (the public address
//     and port typed in by hand);
//  2. an enabled NAT entry of the node whose port map covers the port
//     (nodeentry), at the mapped public port;
//  3. the route's listen address when it names one, else the node's host,
//     on the port itself.
package entryaddr

import (
	"net"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodeentry"
)

// LabelPrefix starts the route label key of a node's override.
const LabelPrefix = "entry-public/"

// Source says where a resolved address came from.
type Source string

const (
	SourceRoute Source = "route"
	SourceNAT   Source = "nat"
	SourceNode  Source = "node"
)

// Override is an address typed in by hand; Port 0 keeps the route's port.
type Override struct {
	Host string
	Port int
}

// LabelKey is the route label key holding ref's override.
func LabelKey(ref string) string { return LabelPrefix + ref }

// ParseOverride reads "host", "host:port" or "[v6]:port"; false when the
// host is not an address or name, or the port is not 1-65535.
func ParseOverride(value string) (Override, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Override{}, false
	}
	host, portText := value, ""
	if h, p, err := net.SplitHostPort(value); err == nil {
		host, portText = h, p
	} else if strings.HasPrefix(value, "[") || strings.Count(value, ":") == 1 {
		return Override{}, false
	}
	host, err := nodeentry.NormalizeHost(host)
	if err != nil {
		return Override{}, false
	}
	out := Override{Host: strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")}
	if portText != "" {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return Override{}, false
		}
		out.Port = port
	}
	return out, true
}

// FromLabels answers ref's override in a route's labels.
func FromLabels(labels map[string]string, ref string) (Override, bool) {
	value, ok := labels[LabelKey(ref)]
	if !ok {
		return Override{}, false
	}
	return ParseOverride(value)
}

// Resolve answers the host and port ref is reached at for a route port
// port, with the host the caller would use alone (listen address or node
// host) as fallback. nat are the node's enabled entries.
func Resolve(labels map[string]string, ref, fallback string, port int, nat []model.NodeEntry) (string, int, Source) {
	if o, ok := FromLabels(labels, ref); ok {
		if o.Port != 0 {
			return o.Host, o.Port, SourceRoute
		}
		return o.Host, port, SourceRoute
	}
	if port > 0 {
		for _, row := range nat {
			if row.Kind != model.NodeEntryNAT || !row.Enabled {
				continue
			}
			if public, ok := nodeentry.Port(row, port); ok {
				return strings.TrimSuffix(strings.TrimPrefix(row.Host, "["), "]"), public, SourceNAT
			}
		}
	}
	return fallback, port, SourceNode
}
