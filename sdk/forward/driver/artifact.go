package driver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// Artifact is what Render produces and Apply consumes: the engine's own
// configuration (an nft script, a gost YAML) as opaque bytes, with the
// identity of the state it came from.
//
// Content and Digest depend only on the engine's hops, never on Generation
// or StateHash: a generation bump caused by another engine's hop leaves this
// engine's digest unchanged, so applying it is a no-op that only records
// the new generation.
type Artifact struct {
	Engine forwardv1.Engine
	// NodeRef, Generation and StateHash are copied from the rendered
	// NodeForwardState.
	NodeRef    string
	Generation uint64
	StateHash  string
	// Hops are the (route, hop) pairs the artifact runs, sorted.
	Hops []HopKey
	// Content is the engine's configuration, opaque to everyone but the
	// driver that rendered it.
	Content []byte
	// Digest is the lowercase hex SHA-256 of Content.
	Digest string
}

// HopKey identifies one hop of one route on a node.
type HopKey struct {
	RouteID  string
	HopIndex uint32
}

// String formats the key as "<route>/<hop>".
func (k HopKey) String() string { return fmt.Sprintf("%s/%d", k.RouteID, k.HopIndex) }

// Compare orders keys by route, then hop index.
func (k HopKey) Compare(o HopKey) int {
	if c := strings.Compare(k.RouteID, o.RouteID); c != 0 {
		return c
	}
	switch {
	case k.HopIndex < o.HopIndex:
		return -1
	case k.HopIndex > o.HopIndex:
		return 1
	}
	return 0
}

// NewArtifact builds an artifact for the given state and content, sorting
// hops and computing the digest. state may be nil for a test artifact.
func NewArtifact(engine forwardv1.Engine, state *forwardv1.NodeForwardState, hops []HopKey, content []byte) Artifact {
	a := Artifact{
		Engine:  engine,
		Hops:    slices.Clone(hops),
		Content: slices.Clone(content),
		Digest:  Digest(content),
	}
	slices.SortFunc(a.Hops, HopKey.Compare)
	if state != nil {
		a.NodeRef = state.GetNodeRef()
		a.Generation = state.GetGeneration()
		a.StateHash = state.GetStateHash()
	}
	return a
}

// Digest is the lowercase hex SHA-256 of content.
func Digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// Empty reports whether the artifact runs no hop.
func (a Artifact) Empty() bool { return len(a.Hops) == 0 }

// Verify checks that the artifact is for engine and that its digest matches
// its content. Apply calls it first.
func (a Artifact) Verify(engine forwardv1.Engine) error {
	if a.Engine != engine {
		return fmt.Errorf("%w: artifact for %s, driver for %s", ErrEngineMismatch, a.Engine, engine)
	}
	if a.Digest != Digest(a.Content) {
		return fmt.Errorf("%w: digest does not match content", ErrInvalidArtifact)
	}
	if !slices.IsSortedFunc(a.Hops, HopKey.Compare) {
		return fmt.Errorf("%w: hops not sorted", ErrInvalidArtifact)
	}
	for i := 1; i < len(a.Hops); i++ {
		if a.Hops[i] == a.Hops[i-1] {
			return fmt.Errorf("%w: duplicate hop %s", ErrInvalidArtifact, a.Hops[i])
		}
	}
	return nil
}

// EngineHops answers the hops of state that run on engine, sorted by route
// and hop index, and an error for every (route, hop) that appears more than
// once (all its copies are dropped). Drivers' Render use it.
func EngineHops(state *forwardv1.NodeForwardState, engine forwardv1.Engine) ([]*forwardv1.NodeHop, []*HopError) {
	count := map[HopKey]int{}
	for _, h := range state.GetHops() {
		if h.GetEngine() == engine {
			count[KeyOf(h)]++
		}
	}
	var hops []*forwardv1.NodeHop
	var errs []*HopError
	for _, h := range state.GetHops() {
		if h.GetEngine() != engine {
			continue
		}
		k := KeyOf(h)
		switch n := count[k]; {
		case n == 1:
			hops = append(hops, h)
		case n > 1:
			errs = append(errs, &HopError{Key: k, Engine: engine, Err: fmt.Errorf("%w: hop appears %d times", ErrInvalidState, n)})
			count[k] = 0 // report once
		}
	}
	slices.SortFunc(hops, func(a, b *forwardv1.NodeHop) int { return KeyOf(a).Compare(KeyOf(b)) })
	slices.SortFunc(errs, func(a, b *HopError) int { return a.Key.Compare(b.Key) })
	return hops, errs
}

// KeyOf answers a NodeHop's key.
func KeyOf(h *forwardv1.NodeHop) HopKey {
	return HopKey{RouteID: h.GetRouteId(), HopIndex: h.GetHopIndex()}
}
