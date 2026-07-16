package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// The graph resolver operates on releases that have already passed signature
// and manifest admission. These errors describe graph-level failures so the
// caller can distinguish a malformed graph from a package-manager failure.
var (
	ErrPluginDependencyGraphEmpty             = errors.New("plugin dependency graph is empty")
	ErrPluginDependencyGraphRootRequired      = errors.New("plugin dependency graph root is required")
	ErrPluginDependencyGraphDuplicateManifest = errors.New("duplicate plugin manifest in dependency graph")
	ErrPluginDependencyGraphDuplicateRoot     = errors.New("duplicate plugin dependency graph root")
	ErrPluginDependencyGraphInvalid           = errors.New("invalid plugin dependency graph manifest")
	ErrPluginDependencyGraphSelf              = errors.New("plugin dependency graph contains a self dependency")
	ErrPluginDependencyGraphDuplicate         = errors.New("plugin dependency graph contains a duplicate relationship")
	ErrPluginDependencyGraphMissing           = errors.New("plugin dependency graph dependency is missing")
	ErrPluginDependencyGraphCycle             = errors.New("plugin dependency graph contains a cycle")
	ErrPluginDependencyGraphConflict          = errors.New("plugin dependency graph contains a conflict")
)

// PluginDependencyResolution is the deterministic execution plan for a set of
// selected plugin releases. Ordered and Order contain the same nodes; the
// former is convenient for lifecycle execution and the latter is convenient
// for persistence, logging, and test assertions.
//
// Dependencies always precede their dependents. Nodes that are otherwise
// unrelated are ordered by plugin ID, making the result independent of map,
// input-slice, and manifest-declaration order.
type PluginDependencyResolution struct {
	Roots   []string         `json:"roots"`
	Order   []string         `json:"order"`
	Ordered []PluginManifest `json:"ordered"`
}

// PluginDependencyResolver indexes one selected release per plugin ID. A
// release catalog may contain many versions, but callers must select the
// version to execute before constructing this resolver.
type PluginDependencyResolver struct {
	manifests map[string]PluginManifest
}

// NewPluginDependencyResolver validates and indexes the selected manifests.
// It intentionally performs graph relationship validation here, while full
// signature/artifact validation remains the responsibility of
// RegisterPluginRelease/VerifyStoredPluginRelease.
func NewPluginDependencyResolver(manifests []PluginManifest) (*PluginDependencyResolver, error) {
	if len(manifests) == 0 {
		return nil, ErrPluginDependencyGraphEmpty
	}

	indexed := make(map[string]PluginManifest, len(manifests))
	for _, manifest := range manifests {
		if err := validatePluginDependencyGraphManifest(manifest); err != nil {
			return nil, err
		}
		if _, exists := indexed[manifest.ID]; exists {
			return nil, fmt.Errorf("%w: %s", ErrPluginDependencyGraphDuplicateManifest, manifest.ID)
		}
		indexed[manifest.ID] = manifest
	}
	return &PluginDependencyResolver{manifests: indexed}, nil
}

// ResolvePluginDependencyGraph resolves root plugin IDs from a selected
// manifest set. The returned order is a stable dependency-first topological
// order. A shared dependency is emitted once, even when multiple roots depend
// on it.
func ResolvePluginDependencyGraph(manifests []PluginManifest, rootIDs []string) (*PluginDependencyResolution, error) {
	resolver, err := NewPluginDependencyResolver(manifests)
	if err != nil {
		return nil, err
	}
	return resolver.Resolve(rootIDs)
}

// ResolvePluginDependencies is an argument-order convenience wrapper for
// callers that naturally have roots before loading the selected manifests.
func ResolvePluginDependencies(rootIDs []string, manifests []PluginManifest) (*PluginDependencyResolution, error) {
	return ResolvePluginDependencyGraph(manifests, rootIDs)
}

// Resolve computes the dependency closure and a deterministic topological
// order for rootIDs.
func (r *PluginDependencyResolver) Resolve(rootIDs []string) (*PluginDependencyResolution, error) {
	if r == nil || len(r.manifests) == 0 {
		return nil, ErrPluginDependencyGraphEmpty
	}
	if len(rootIDs) == 0 {
		return nil, ErrPluginDependencyGraphRootRequired
	}

	roots := append([]string(nil), rootIDs...)
	seenRoots := make(map[string]struct{}, len(roots))
	for _, rootID := range roots {
		if !safePluginSegment(rootID) {
			return nil, fmt.Errorf("%w: %q", ErrPluginDependencyGraphRootRequired, rootID)
		}
		if _, exists := r.manifests[rootID]; !exists {
			return nil, fmt.Errorf("%w: %w: root %s", ErrPluginDependencyGraphMissing, ErrPluginDependencyUnsatisfied, rootID)
		}
		if _, exists := seenRoots[rootID]; exists {
			return nil, fmt.Errorf("%w: %s", ErrPluginDependencyGraphDuplicateRoot, rootID)
		}
		seenRoots[rootID] = struct{}{}
	}
	sort.Strings(roots)

	// A DFS gives a deterministic and useful cycle path. The subsequent Kahn
	// pass supplies the globally stable order for nodes with equal precedence.
	const (
		active = 1
		done   = 2
	)
	colors := make(map[string]uint8, len(r.manifests))
	stack := make([]string, 0, len(r.manifests))
	reachable := make(map[string]struct{}, len(r.manifests))
	var visit func(string) error
	visit = func(pluginID string) error {
		switch colors[pluginID] {
		case active:
			cycle := dependencyCyclePath(stack, pluginID)
			return fmt.Errorf("%w: %s", ErrPluginDependencyGraphCycle, strings.Join(cycle, " -> "))
		case done:
			return nil
		}

		manifest, exists := r.manifests[pluginID]
		if !exists {
			chain := append(append([]string(nil), stack...), pluginID)
			return fmt.Errorf("%w: %w: %s", ErrPluginDependencyGraphMissing, ErrPluginDependencyUnsatisfied, strings.Join(chain, " -> "))
		}
		colors[pluginID] = active
		stack = append(stack, pluginID)
		reachable[pluginID] = struct{}{}

		dependencies := append([]string(nil), manifest.Dependencies...)
		sort.Strings(dependencies)
		for _, dependency := range dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}

		stack = stack[:len(stack)-1]
		colors[pluginID] = done
		return nil
	}
	for _, rootID := range roots {
		if err := visit(rootID); err != nil {
			return nil, err
		}
	}

	if err := validateReachablePluginConflicts(r.manifests, reachable); err != nil {
		return nil, err
	}

	order, err := stablePluginTopologicalOrder(r.manifests, reachable)
	if err != nil {
		return nil, err
	}
	ordered := make([]PluginManifest, 0, len(order))
	for _, pluginID := range order {
		ordered = append(ordered, r.manifests[pluginID])
	}
	return &PluginDependencyResolution{Roots: roots, Order: order, Ordered: ordered}, nil
}

func validatePluginDependencyGraphManifest(manifest PluginManifest) error {
	if !safePluginSegment(manifest.ID) {
		return fmt.Errorf("%w: invalid plugin id %q", ErrPluginDependencyGraphInvalid, manifest.ID)
	}

	seenDependencies := make(map[string]struct{}, len(manifest.Dependencies))
	for _, dependency := range manifest.Dependencies {
		if !safePluginSegment(dependency) {
			return fmt.Errorf("%w: invalid dependency %q of %s", ErrPluginDependencyGraphInvalid, dependency, manifest.ID)
		}
		if dependency == manifest.ID {
			return fmt.Errorf("%w: %s", ErrPluginDependencyGraphSelf, manifest.ID)
		}
		if _, exists := seenDependencies[dependency]; exists {
			return fmt.Errorf("%w: %s depends on %s more than once", ErrPluginDependencyGraphDuplicate, manifest.ID, dependency)
		}
		seenDependencies[dependency] = struct{}{}
	}

	seenConflicts := make(map[string]struct{}, len(manifest.Conflicts))
	for _, conflict := range manifest.Conflicts {
		if !safePluginSegment(conflict) {
			return fmt.Errorf("%w: invalid conflict %q of %s", ErrPluginDependencyGraphInvalid, conflict, manifest.ID)
		}
		if conflict == manifest.ID {
			return fmt.Errorf("%w: %s", ErrPluginDependencyGraphSelf, manifest.ID)
		}
		if _, exists := seenConflicts[conflict]; exists {
			return fmt.Errorf("%w: %s conflicts with %s more than once", ErrPluginDependencyGraphDuplicate, manifest.ID, conflict)
		}
		if _, dependency := seenDependencies[conflict]; dependency {
			return fmt.Errorf("%w: %s is both a dependency and conflict of %s", ErrPluginDependencyGraphConflict, conflict, manifest.ID)
		}
		seenConflicts[conflict] = struct{}{}
	}

	// Keep the graph validator aligned with the manifest validator for any
	// relationship checks added there in the future.
	if err := validateManifestRelationships(manifest.ID, manifest.Dependencies, manifest.Conflicts); err != nil {
		return fmt.Errorf("%w: %v", ErrPluginDependencyGraphInvalid, err)
	}
	return nil
}

func validateReachablePluginConflicts(manifests map[string]PluginManifest, reachable map[string]struct{}) error {
	ids := make([]string, 0, len(reachable))
	for pluginID := range reachable {
		ids = append(ids, pluginID)
	}
	sort.Strings(ids)
	for _, pluginID := range ids {
		conflicts := append([]string(nil), manifests[pluginID].Conflicts...)
		sort.Strings(conflicts)
		for _, conflict := range conflicts {
			if _, exists := reachable[conflict]; !exists {
				continue
			}
			return fmt.Errorf("%w: %w: %s conflicts with %s", ErrPluginDependencyGraphConflict, ErrPluginConflict, pluginID, conflict)
		}
	}
	return nil
}

func stablePluginTopologicalOrder(manifests map[string]PluginManifest, reachable map[string]struct{}) ([]string, error) {
	indegree := make(map[string]int, len(reachable))
	dependents := make(map[string][]string, len(reachable))
	for pluginID := range reachable {
		indegree[pluginID] = 0
	}
	for pluginID := range reachable {
		dependencies := append([]string(nil), manifests[pluginID].Dependencies...)
		sort.Strings(dependencies)
		for _, dependency := range dependencies {
			if _, exists := reachable[dependency]; !exists {
				return nil, fmt.Errorf("%w: %w: %s requires %s", ErrPluginDependencyGraphMissing, ErrPluginDependencyUnsatisfied, pluginID, dependency)
			}
			indegree[pluginID]++
			dependents[dependency] = append(dependents[dependency], pluginID)
		}
	}
	for dependency := range dependents {
		sort.Strings(dependents[dependency])
	}

	ready := make([]string, 0, len(reachable))
	for pluginID, degree := range indegree {
		if degree == 0 {
			ready = append(ready, pluginID)
		}
	}
	sort.Strings(ready)
	order := make([]string, 0, len(reachable))
	for len(ready) > 0 {
		pluginID := ready[0]
		ready = ready[1:]
		order = append(order, pluginID)
		for _, dependent := range dependents[pluginID] {
			indegree[dependent]--
			if indegree[dependent] != 0 {
				continue
			}
			index := sort.SearchStrings(ready, dependent)
			ready = append(ready, "")
			copy(ready[index+1:], ready[index:])
			ready[index] = dependent
		}
	}
	if len(order) != len(reachable) {
		// DFS normally reports the cycle first. Keep this guard for callers that
		// construct a resolver through future alternate indexing paths.
		return nil, ErrPluginDependencyGraphCycle
	}
	return order, nil
}

func dependencyCyclePath(stack []string, repeated string) []string {
	start := 0
	for index, pluginID := range stack {
		if pluginID == repeated {
			start = index
			break
		}
	}
	cycle := append([]string(nil), stack[start:]...)
	return append(cycle, repeated)
}
