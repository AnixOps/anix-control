// Package leastconn approximates LEAST_CONN balancing (forward-sdk.md
// section 7.1, L1). Neither engine has a least-connections scheduler, so a
// LEAST_CONN hop renders as weighted random, and the Agent's health loop
// re-weights its upstreams every model.DefaultLeastConnReweight (10 s,
// owner decision H21) from their live connections, through the driver's
// SetUpstreams: no apply, no new generation, no counter epoch ends.
//
// # Weights
//
// Weighted random spreads the next interval's new connections in
// proportion to the weights. Weights gives each upstream its rendered
// weight divided by its live connections plus one, so the upstream least
// loaded relative to its weight gets the most new connections (weighted
// least connections), scaled so the largest is Scale and none is below 1.
// While no upstream has a connection the rendered weights stay, so an idle
// hop is never re-weighted.
//
// # Sources
//
// A Source answers the live connections per upstream address and port.
// What each driver can provide:
//
//   - gost: (*gost.Driver).ActiveConns counts the host's established TCP
//     connections and connected UDP sockets to each upstream (ss, no
//     privilege). It is exact for RAW upstreams; an encrypted or
//     multiplexed link counts its carriers, not the streams inside them;
//     an upstream that several hops share counts once for all of them.
//   - nftables: the forwarded connections are conntrack entries, not
//     sockets, and reading conntrack needs netlink with CAP_NET_ADMIN,
//     which this standard-library-only SDK does not do. The Agent (F3b)
//     supplies a Source from conntrack (the original direction's
//     destination after DNAT, per address and port); without one a
//     LEAST_CONN hop on nftables stays weighted random by its rendered
//     weights.
//   - fake: (*fake.Driver).ActiveConns answers Host.SetUpstreamConns.
//
// # The health loop decides rotation
//
// Which upstreams are in rotation stays the health loop's decision
// (section 7.3): Reweighter weights only the upstreams the loop hands it
// in Hop.Upstreams, so it never puts back an upstream the loop took out,
// and it calls SetUpstreams only when the weighting differs from the
// rotation the driver reports (Hop.Rotation).
package leastconn

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"slices"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/model"
)

// Scale is the weight of the least loaded upstream of a re-weighted hop.
const Scale = 100

// Source answers the live connections to each upstream address and port
// it sees; an upstream it does not list has none.
type Source interface {
	ActiveConns(ctx context.Context) (map[netip.AddrPort]uint64, error)
}

// Setter puts upstreams in rotation with weights: driver.Driver.
type Setter interface {
	SetUpstreams(ctx context.Context, routeID string, hopIndex uint32, active []driver.Upstream) error
}

// Hop is one LEAST_CONN hop as the health loop hands it over.
type Hop struct {
	RouteID  string
	HopIndex uint32
	// Upstreams are the upstreams the health loop keeps in rotation, each
	// with its rendered weight (0 counts as 1).
	Upstreams []driver.Upstream
	// Rotation is what the driver runs now (driver.Observation's
	// Rotation): a weighting equal to it is not set again. Nil always
	// sets it.
	Rotation []driver.Upstream
}

// Key answers the hop's key.
func (h Hop) Key() driver.HopKey { return driver.HopKey{RouteID: h.RouteID, HopIndex: h.HopIndex} }

// Weights answers ups with the weights for the next interval: each
// rendered weight (0 counts as 1) divided by the upstream's connections
// plus one, scaled so the largest is Scale, at least 1. Without any
// connection it answers the rendered weights.
func Weights(ups []driver.Upstream, conns map[netip.AddrPort]uint64) []driver.Upstream {
	norm := make(map[netip.AddrPort]uint64, len(conns))
	for k, v := range conns {
		norm[netip.AddrPortFrom(k.Addr().Unmap(), k.Port())] += v
	}
	out := slices.Clone(ups)
	loads := make([]uint64, len(out))
	busy := false
	for i, u := range out {
		out[i].Weight = max(u.Weight, 1)
		if a, err := netip.ParseAddr(u.Address); err == nil && u.Port <= math.MaxUint16 {
			loads[i] = norm[netip.AddrPortFrom(a.Unmap(), uint16(u.Port))] // #nosec G115 -- checked
		}
		busy = busy || loads[i] > 0
	}
	if !busy {
		return out
	}
	ratios := make([]float64, len(out))
	top := 0.0
	for i, u := range out {
		ratios[i] = float64(u.Weight) / float64(loads[i]+1)
		top = max(top, ratios[i])
	}
	for i := range out {
		out[i].Weight = uint32(max(1, math.Round(Scale*ratios[i]/top))) // #nosec G115 -- at most Scale
	}
	return out
}

// Reweighter re-weights LEAST_CONN hops from a Source through a Setter.
type Reweighter struct {
	Source Source
	Setter Setter
	// OnError, when set, receives the errors Run meets; Run goes on.
	OnError func(error)
}

// Tick reads the connections once and sets the weights of every hop
// whose weighting differs from its rotation. It answers the errors
// joined; one hop's error does not stop the others.
func (r Reweighter) Tick(ctx context.Context, hops []Hop) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(hops) == 0 {
		return nil
	}
	conns, err := r.Source.ActiveConns(ctx)
	if err != nil {
		return fmt.Errorf("leastconn: %w", err)
	}
	var errs []error
	for _, h := range hops {
		if len(h.Upstreams) == 0 {
			continue
		}
		want := Weights(h.Upstreams, conns)
		if h.Rotation != nil && sameRotation(want, h.Rotation) {
			continue
		}
		if err := r.Setter.SetUpstreams(ctx, h.RouteID, h.HopIndex, want); err != nil {
			errs = append(errs, fmt.Errorf("leastconn: hop %s: %w", h.Key(), err))
		}
	}
	return errors.Join(errs...)
}

// Run calls Tick every interval (model.DefaultLeastConnReweight when 0)
// with the hops hops answers, until ctx is done, and answers ctx's error.
func (r Reweighter) Run(ctx context.Context, interval time.Duration, hops func(context.Context) []Hop) error {
	if interval <= 0 {
		interval = model.DefaultLeastConnReweight
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := r.Tick(ctx, hops(ctx)); err != nil && r.OnError != nil && ctx.Err() == nil {
				r.OnError(err)
			}
		}
	}
}

// sameRotation reports whether two selections hold the same upstreams
// with the same weights, in any order.
func sameRotation(a, b []driver.Upstream) bool {
	if len(a) != len(b) {
		return false
	}
	key := func(u driver.Upstream) string {
		if p, err := netip.ParseAddr(u.Address); err == nil {
			u.Address = p.Unmap().String()
		}
		return fmt.Sprintf("%s|%d|%d", u.Address, u.Port, max(u.Weight, 1))
	}
	ka, kb := make([]string, len(a)), make([]string, len(b))
	for i := range a {
		ka[i], kb[i] = key(a[i]), key(b[i])
	}
	slices.Sort(ka)
	slices.Sort(kb)
	return slices.Equal(ka, kb)
}
