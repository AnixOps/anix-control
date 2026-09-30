package v2

import (
	"context"
	"sync"
)

// RouteFreeze pauses package routes. While a route is frozen the gateway
// answers 503 with Retry-After instead of dispatching it, and Drain waits
// for the requests already dispatched to finish. The identity cutover
// freezes identity-platform's group A with it while authority moves.
type RouteFreeze struct {
	mu       sync.Mutex
	frozen   map[routeKey]bool
	inFlight map[routeKey]int
	// released is closed and replaced whenever a request finishes.
	released chan struct{}
}

type routeKey struct{ packageID, route string }

var defaultRouteFreeze = &RouteFreeze{}

// DefaultRouteFreeze is the freeze a Gateway without its own uses.
func DefaultRouteFreeze() *RouteFreeze { return defaultRouteFreeze }

// Freeze pauses the package's routes.
func (f *RouteFreeze) Freeze(packageID string, routes []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.frozen == nil {
		f.frozen = map[routeKey]bool{}
	}
	for _, route := range routes {
		f.frozen[routeKey{packageID, route}] = true
	}
}

// Unfreeze resumes the package's routes.
func (f *RouteFreeze) Unfreeze(packageID string, routes []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, route := range routes {
		delete(f.frozen, routeKey{packageID, route})
	}
}

// Frozen reports whether the route is paused.
func (f *RouteFreeze) Frozen(packageID, route string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.frozen[routeKey{packageID, route}]
}

// enter admits a request unless its route is frozen; call release when the
// request is done.
func (f *RouteFreeze) enter(packageID, route string) (release func(), admitted bool) {
	key := routeKey{packageID, route}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.frozen[key] {
		return nil, false
	}
	if f.inFlight == nil {
		f.inFlight = map[routeKey]int{}
	}
	f.inFlight[key]++
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.inFlight[key]--; f.inFlight[key] <= 0 {
			delete(f.inFlight, key)
		}
		if f.released != nil {
			close(f.released)
			f.released = nil
		}
	}, true
}

// Drain waits until no request of the routes is in flight.
func (f *RouteFreeze) Drain(ctx context.Context, packageID string, routes []string) error {
	for {
		f.mu.Lock()
		busy := false
		for _, route := range routes {
			if f.inFlight[routeKey{packageID, route}] > 0 {
				busy = true
				break
			}
		}
		if !busy {
			f.mu.Unlock()
			return nil
		}
		if f.released == nil {
			f.released = make(chan struct{})
		}
		released := f.released
		f.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-released:
		}
	}
}
