package anixops

import (
	"fmt"
	"slices"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
)

// manifestNote heads every document the driver renders.
const manifestNote = "Rendered by sdk/forward/driver/anixops; do not edit. The anixops relay runs it."

// Render turns the state's anixops hops into one relay configuration (JSON,
// relayctl.Config); see the package documentation for its shape. Hops it
// cannot run come back as hop errors in a *driver.RenderError next to the
// artifact of the others.
func (d *Driver) Render(state *forwardv1.NodeForwardState) (driver.Artifact, error) {
	if state == nil {
		return driver.Artifact{}, fmt.Errorf("%w: nil state", driver.ErrInvalidState)
	}
	hops, hopErrs := driver.EngineHops(state, d.Engine())
	var plans []relayctl.Hop
	for _, h := range hops {
		p, err := d.planHop(h)
		if err != nil {
			hopErrs = append(hopErrs, &driver.HopError{Key: driver.KeyOf(h), Engine: d.Engine(), Err: err})
			continue
		}
		plans = append(plans, p)
	}
	plans, collisions := rejectCollisions(plans)
	hopErrs = append(hopErrs, collisions...)
	slices.SortStableFunc(hopErrs, func(a, b *driver.HopError) int { return a.Key.Compare(b.Key) })

	keys := make([]driver.HopKey, len(plans))
	for i, p := range plans {
		keys[i] = driver.HopKey{RouteID: p.Route, HopIndex: p.Hop}
	}
	content, err := d.document(plans)
	if err != nil {
		return driver.Artifact{}, err
	}
	return driver.NewArtifact(d.Engine(), state, keys, content), driver.NewRenderError(hopErrs)
}

// document writes the configuration of the planned hops, sorted by key.
func (d *Driver) document(plans []relayctl.Hop) ([]byte, error) {
	doc := &relayctl.Config{Format: relayctl.Format, Owner: OwnerMark, Note: manifestNote, Hops: []relayctl.Hop{}}
	if len(plans) > 0 {
		doc.Hops = plans
		if d.cfg.linkFiles() {
			doc.Link = &relayctl.LinkFiles{Cert: d.cfg.LinkCert, Key: d.cfg.LinkKey, CA: d.cfg.LinkCA}
			doc.StatelessResetKeyFile = d.cfg.resetPath()
		}
	}
	return relayctl.Encode(doc)
}
