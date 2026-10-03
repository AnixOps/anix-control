package gost

import (
	"context"
	"fmt"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

var _ driver.QuotaEnforcer = (*Driver)(nil)

// EnforceQuotas keeps the soft quota of every applied hop with
// quota_bytes (gost has no byte quota of its own; forward-sdk.md section
// 6.2): a hop whose up and down bytes in its current counter epoch reached
// the quota admits nobody (its admission is replaced through the web API,
// as a pause is: the listener, the counters and their epoch stay, and
// established connections run until they close), and a hop held so whose
// quota was raised above its bytes admits its sources again. It answers
// the hops it holds for their quota. The Agent calls it after every
// Observe and every Apply, so the quota holds within one observation
// interval.
//
// The quota counts the current epoch, as nftables' named quota counts
// since its creation: a gost start or reload starts it again, and an
// Apply that changes a held hop's admission opens it until the next call.
// Control's ledger, summed over every epoch and entry, stays the authority
// (it plans the route paused, forward-sdk.md section 5.3).
func (d *Driver) EnforceQuotas(ctx context.Context) ([]driver.HopKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	h, err := d.readHost()
	if err != nil {
		return nil, err
	}
	if !h.owned || !h.state.applied() {
		return nil, nil
	}
	quotas := false
	for _, mh := range h.state.Hops {
		quotas = quotas || mh.Quota > 0
	}
	if !quotas {
		return nil, nil
	}
	status, err := d.sup.Status(ctx)
	if err != nil {
		return nil, err
	}
	if !status.Running {
		return nil, nil
	}
	live, err := d.readLive(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := decodeConfig(h.config)
	if err != nil {
		return nil, err
	}
	var held []driver.HopKey
	for _, mh := range h.state.Hops {
		if mh.Quota == 0 || mh.Paused {
			continue
		}
		name := hopName(mh.key())
		var used uint64
		for _, s := range mh.Services {
			if ls := live.service(s); ls != nil && ls.Status != nil && ls.Status.Stats != nil {
				used += ls.Status.Stats.InputBytes + ls.Status.Stats.OutputBytes
			}
		}
		closed := false
		if la := live.admission(name); la != nil {
			closed = la.Whitelist && len(la.Matchers) == 0
		}
		switch {
		case used >= mh.Quota:
			if !closed {
				if err := d.put(ctx, "admissions", name, admission{Name: name, Whitelist: true, Matchers: []string{}}); err != nil {
					return held, err
				}
			}
			held = append(held, mh.key())
		case closed:
			rendered := cfg.admission(name)
			if rendered == nil {
				return held, fmt.Errorf("%w: the applied configuration has no admission %s", driver.ErrInvalidArtifact, name)
			}
			if err := d.put(ctx, "admissions", name, *rendered); err != nil {
				return held, err
			}
		}
	}
	return held, nil
}
