package v2

import (
	"container/list"
	"context"
	"strconv"
	"sync"

	"golang.org/x/sync/singleflight"
)

// DefaultVerifiedRouteCacheEntries bounds the verified-route cache. One entry
// holds the small decoded route list of one signed release, so the bound is
// about release churn, not memory.
const DefaultVerifiedRouteCacheEntries = 256

// verifiedRouteCacheKey identifies one successful verification. Release rows
// are immutable after registration and unique by (plugin, version); the
// artifact digest and trust-root fingerprint are part of the key anyway so a
// row that did change could never be served from a stale entry. The configured
// key fingerprint covers releases registered without a stored trust root.
type verifiedRouteCacheKey struct {
	releaseID            uint
	artifactSHA256       string
	trustRootFingerprint string
	configuredKey        string
}

func (k verifiedRouteCacheKey) String() string {
	return strconv.FormatUint(uint64(k.releaseID), 10) + "\x00" + k.artifactSHA256 + "\x00" + k.trustRootFingerprint + "\x00" + k.configuredKey
}

// VerifiedRouteCache memoizes successfully verified package route
// declarations. It never caches a failure (an artifact or WebUI asset may be
// uploaded after the release is registered) and it never caches authorization
// state: installations, the official plugin row, and trust-root activity are
// read on every resolution. It is safe for concurrent use; concurrent misses
// for one release share a single verification.
type VerifiedRouteCache struct {
	mu       sync.Mutex
	capacity int
	entries  map[verifiedRouteCacheKey]*list.Element
	order    *list.List
	group    singleflight.Group
}

type verifiedRouteCacheEntry struct {
	key    verifiedRouteCacheKey
	routes []Route
}

// NewVerifiedRouteCache returns a cache bounded to capacity entries with
// least-recently-used eviction. A capacity of zero or less selects
// DefaultVerifiedRouteCacheEntries.
func NewVerifiedRouteCache(capacity int) *VerifiedRouteCache {
	if capacity <= 0 {
		capacity = DefaultVerifiedRouteCacheEntries
	}
	return &VerifiedRouteCache{capacity: capacity, entries: make(map[verifiedRouteCacheKey]*list.Element), order: list.New()}
}

// Len reports the number of cached releases.
func (c *VerifiedRouteCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// load returns the cached routes for key or runs verify once for all
// concurrent callers. The returned slice is shared and must not be mutated.
// A nil cache always verifies.
func (c *VerifiedRouteCache) load(ctx context.Context, key verifiedRouteCacheKey, verify func(context.Context) ([]Route, error)) ([]Route, error) {
	if c == nil {
		return verify(ctx)
	}
	if routes, ok := c.get(key); ok {
		return routes, nil
	}
	value, err, _ := c.group.Do(key.String(), func() (any, error) {
		if routes, ok := c.get(key); ok {
			return routes, nil
		}
		// The verification result is shared with every waiter, so one
		// canceled request must not fail the others.
		routes, err := verify(context.WithoutCancel(ctx))
		if err != nil {
			return nil, err
		}
		c.put(key, routes)
		return routes, nil
	})
	if err != nil {
		return nil, err
	}
	routes, _ := value.([]Route)
	return routes, nil
}

func (c *VerifiedRouteCache) get(key verifiedRouteCacheKey) ([]Route, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(element)
	entry, _ := element.Value.(*verifiedRouteCacheEntry)
	return entry.routes, true
}

func (c *VerifiedRouteCache) put(key verifiedRouteCacheKey, routes []Route) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[key]; ok {
		c.order.MoveToFront(element)
		return
	}
	c.entries[key] = c.order.PushFront(&verifiedRouteCacheEntry{key: key, routes: routes})
	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		if entry, ok := oldest.Value.(*verifiedRouteCacheEntry); ok {
			delete(c.entries, entry.key)
		}
	}
}
