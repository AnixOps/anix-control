package gost

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// manifestNote heads the manifest of every artifact.
const manifestNote = "Rendered by sdk/forward/driver/gost; do not edit. gost ignores this key; Apply reads it."

// manifest is the "anixops" member of a rendered configuration: what the
// driver needs and the gost objects do not say. gost's configuration
// loader ignores members it does not know.
type manifest struct {
	Driver string        `json:"driver"`
	Note   string        `json:"note"`
	Hops   []manifestHop `json:"hops"`
}

type manifestHop struct {
	Route   string `json:"route"`
	Hop     uint32 `json:"hop"`
	Balance string `json:"balance"`
	Paused  bool   `json:"paused,omitempty"`
	// Quota is the hop's quota_bytes, which EnforceQuotas keeps (gost has
	// no byte quota of its own).
	Quota     uint64             `json:"quota,omitempty"`
	Services  []string           `json:"services"`
	Listeners []manifestListener `json:"listeners"`
	Upstreams []manifestUpstream `json:"upstreams"`
}

// manifestListener is a socket a hop's service holds: network tcp or udp,
// an empty address for every local address.
type manifestListener struct {
	Network string `json:"network"`
	Address string `json:"address,omitempty"`
	Port    uint32 `json:"port"`
}

// manifestUpstream is one rendered upstream with its weight and priority.
type manifestUpstream struct {
	Address  string `json:"address"`
	Port     uint32 `json:"port"`
	Weight   uint32 `json:"weight"`
	Priority uint32 `json:"priority"`
}

func (h manifestHop) key() driver.HopKey { return driver.HopKey{RouteID: h.Route, HopIndex: h.Hop} }

// parsed is what Apply reads from an artifact's content.
type parsed struct {
	hops        []manifestHop
	metricsPath string // the configuration's tag; empty for an empty artifact
}

// parseContent reads the manifest and the metrics path of an artifact the
// driver rendered, and checks that they agree with the artifact's hops.
func parseContent(a driver.Artifact) (*parsed, error) {
	var c struct {
		AnixOps *manifest     `json:"anixops"`
		Metrics *metricsBlock `json:"metrics"`
	}
	dec := json.NewDecoder(bytes.NewReader(a.Content))
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%w: %v", driver.ErrInvalidArtifact, err)
	}
	if c.AnixOps == nil || c.AnixOps.Driver != OwnerMark {
		return nil, fmt.Errorf("%w: not a configuration of %s", driver.ErrInvalidArtifact, OwnerMark)
	}
	p := &parsed{hops: c.AnixOps.Hops}
	keys := make([]driver.HopKey, len(p.hops))
	for i, h := range p.hops {
		keys[i] = h.key()
		if !routeIDPattern.MatchString(h.Route) || len(h.Listeners) == 0 || len(h.Upstreams) == 0 {
			return nil, fmt.Errorf("%w: manifest hop %s", driver.ErrInvalidArtifact, h.key())
		}
		for _, l := range h.Listeners {
			if l.Network != "tcp" && l.Network != "udp" || l.Port == 0 || l.Port > 65535 {
				return nil, fmt.Errorf("%w: manifest listener of %s", driver.ErrInvalidArtifact, h.key())
			}
			if l.Address != "" {
				if _, err := netip.ParseAddr(l.Address); err != nil {
					return nil, fmt.Errorf("%w: manifest listener of %s", driver.ErrInvalidArtifact, h.key())
				}
			}
		}
	}
	if !slices.Equal(keys, a.Hops) {
		return nil, fmt.Errorf("%w: manifest hops %v, artifact hops %v", driver.ErrInvalidArtifact, keys, a.Hops)
	}
	if len(p.hops) > 0 {
		if c.Metrics == nil || !strings.HasPrefix(c.Metrics.Path, "/anixops-") {
			return nil, fmt.Errorf("%w: no metrics path", driver.ErrInvalidArtifact)
		}
		p.metricsPath = c.Metrics.Path
	}
	return p, nil
}

// listenerSet answers the sockets of hops as "network address port".
func listenerSet(hops []manifestHop) []manifestListener {
	var out []manifestListener
	for _, h := range hops {
		for _, l := range h.Listeners {
			if !slices.Contains(out, l) {
				out = append(out, l)
			}
		}
	}
	return out
}
