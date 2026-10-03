package driver

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// Registry holds at most one driver per engine. It is safe for concurrent
// use.
type Registry struct {
	mu      sync.RWMutex
	drivers map[forwardv1.Engine]Driver
}

// NewRegistry answers an empty registry.
func NewRegistry() *Registry {
	return &Registry{drivers: map[forwardv1.Engine]Driver{}}
}

// Register adds d under d.Engine(). It refuses a second driver for an
// engine (ErrDuplicateEngine) and ENGINE_UNSPECIFIED (ErrInvalidArgument).
func (r *Registry) Register(d Driver) error {
	if d == nil {
		return fmt.Errorf("%w: nil driver", ErrInvalidArgument)
	}
	e := d.Engine()
	if e == forwardv1.Engine_ENGINE_UNSPECIFIED {
		return fmt.Errorf("%w: driver for ENGINE_UNSPECIFIED", ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.drivers[e]; ok {
		return fmt.Errorf("%w: %s", ErrDuplicateEngine, e)
	}
	r.drivers[e] = d
	return nil
}

// Get answers the driver for engine.
func (r *Registry) Get(engine forwardv1.Engine) (Driver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.drivers[engine]
	return d, ok
}

// Engines answers the registered engines in contract order.
func (r *Registry) Engines() []forwardv1.Engine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]forwardv1.Engine, 0, len(r.drivers))
	for e := range r.drivers {
		out = append(out, e)
	}
	slices.Sort(out)
	return out
}

// Render splits a node's state by engine: every registered driver renders
// its share (an empty artifact when the state has none of its hops, so
// applying it removes what that engine ran before). Hops whose engine has
// no driver, and hops a driver left out, come back as hop errors (wrapping
// ErrUnsupported or the driver's reason); the artifacts are still usable.
// A driver error other than a *RenderError fails the whole call.
func (r *Registry) Render(state *forwardv1.NodeForwardState) (map[forwardv1.Engine]Artifact, []*HopError, error) {
	if state == nil {
		return nil, nil, fmt.Errorf("%w: nil state", ErrInvalidState)
	}
	engines := r.Engines()
	arts := make(map[forwardv1.Engine]Artifact, len(engines))
	var hopErrs []*HopError
	for _, e := range engines {
		d, _ := r.Get(e)
		a, err := d.Render(state)
		var re *RenderError
		switch {
		case err == nil:
		case errors.As(err, &re):
			hopErrs = append(hopErrs, re.Hops...)
		default:
			return nil, nil, fmt.Errorf("render %s: %w", e, err)
		}
		arts[e] = a
	}
	seen := map[HopKey]bool{}
	for _, h := range state.GetHops() {
		if _, ok := r.Get(h.GetEngine()); ok {
			continue
		}
		k := KeyOf(h)
		if seen[k] {
			continue
		}
		seen[k] = true
		hopErrs = append(hopErrs, &HopError{Key: k, Engine: h.GetEngine(), Err: fmt.Errorf("%w: no driver for engine %s", ErrUnsupported, h.GetEngine())})
	}
	slices.SortStableFunc(hopErrs, func(a, b *HopError) int { return a.Key.Compare(b.Key) })
	return arts, hopErrs, nil
}
