package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func dependencyTestManifest(id string, dependencies ...string) PluginManifest {
	return PluginManifest{ID: id, Dependencies: dependencies}
}

func dependencyTestIDs(manifests []PluginManifest) []string {
	ids := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		ids = append(ids, manifest.ID)
	}
	return ids
}

func TestResolvePluginDependencyGraphReturnsStableDependencyFirstOrder(t *testing.T) {
	manifests := []PluginManifest{
		dependencyTestManifest("zeta", "beta", "alpha"),
		dependencyTestManifest("delta"),
		dependencyTestManifest("alpha", "delta"),
		dependencyTestManifest("beta", "delta"),
		dependencyTestManifest("unreachable"),
	}
	original := append([]PluginManifest(nil), manifests...)

	resolution, err := ResolvePluginDependencyGraph(manifests, []string{"zeta"})
	require.NoError(t, err)
	require.Equal(t, []string{"zeta"}, resolution.Roots)
	require.Equal(t, []string{"delta", "alpha", "beta", "zeta"}, resolution.Order)
	require.Equal(t, resolution.Order, dependencyTestIDs(resolution.Ordered))
	require.Equal(t, original, manifests, "resolver must not reorder or mutate its input")
}

func TestResolvePluginDependencyGraphDeduplicatesSharedDependenciesAndSortsRoots(t *testing.T) {
	manifests := []PluginManifest{
		dependencyTestManifest("root-b", "shared"),
		dependencyTestManifest("shared"),
		dependencyTestManifest("root-a", "shared"),
	}

	resolution, err := ResolvePluginDependencies([]string{"root-b", "root-a"}, manifests)
	require.NoError(t, err)
	require.Equal(t, []string{"root-a", "root-b"}, resolution.Roots)
	require.Equal(t, []string{"shared", "root-a", "root-b"}, resolution.Order)
	require.Equal(t, 1, countString(resolution.Order, "shared"))
}

func TestResolvePluginDependencyGraphDetectsCycleWithStablePath(t *testing.T) {
	manifests := []PluginManifest{
		dependencyTestManifest("a", "c", "b"),
		dependencyTestManifest("b", "a"),
		dependencyTestManifest("c"),
	}

	_, err := ResolvePluginDependencyGraph(manifests, []string{"a"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPluginDependencyGraphCycle)
	require.Contains(t, err.Error(), "a -> b -> a")
}

func TestResolvePluginDependencyGraphDetectsSelfDependency(t *testing.T) {
	_, err := ResolvePluginDependencyGraph([]PluginManifest{
		dependencyTestManifest("self", "self"),
	}, []string{"self"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPluginDependencyGraphSelf)
}

func TestResolvePluginDependencyGraphDetectsDuplicateManifest(t *testing.T) {
	_, err := ResolvePluginDependencyGraph([]PluginManifest{
		dependencyTestManifest("duplicate"),
		dependencyTestManifest("duplicate"),
	}, []string{"duplicate"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPluginDependencyGraphDuplicateManifest)
}

func TestResolvePluginDependencyGraphDetectsDuplicateAndOverlappingRelationships(t *testing.T) {
	duplicate := dependencyTestManifest("duplicate", "base", "base")
	_, err := ResolvePluginDependencyGraph([]PluginManifest{duplicate}, []string{"duplicate"})
	require.ErrorIs(t, err, ErrPluginDependencyGraphDuplicate)

	overlap := dependencyTestManifest("overlap", "base")
	overlap.Conflicts = []string{"base"}
	_, err = ResolvePluginDependencyGraph([]PluginManifest{overlap, dependencyTestManifest("base")}, []string{"overlap"})
	require.ErrorIs(t, err, ErrPluginDependencyGraphConflict)
}

func TestResolvePluginDependencyGraphDetectsMissingDependency(t *testing.T) {
	_, err := ResolvePluginDependencyGraph([]PluginManifest{
		dependencyTestManifest("root", "missing"),
	}, []string{"root"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPluginDependencyGraphMissing)
	require.Contains(t, err.Error(), "root -> missing")
}

func TestResolvePluginDependencyGraphDetectsDirectAndReverseConflicts(t *testing.T) {
	tests := []struct {
		name      string
		manifests []PluginManifest
	}{
		{
			name: "direct",
			manifests: []PluginManifest{
				{ID: "a", Conflicts: []string{"b"}},
				{ID: "b"},
			},
		},
		{
			name: "reverse",
			manifests: []PluginManifest{
				{ID: "a"},
				{ID: "b", Conflicts: []string{"a"}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ResolvePluginDependencyGraph(test.manifests, []string{"a", "b"})
			require.Error(t, err)
			require.ErrorIs(t, err, ErrPluginDependencyGraphConflict)
			require.ErrorIs(t, err, ErrPluginConflict)
		})
	}
}

func TestResolvePluginDependencyGraphRejectsDuplicateRootsAndInvalidRootRequests(t *testing.T) {
	manifests := []PluginManifest{dependencyTestManifest("root")}

	_, err := ResolvePluginDependencyGraph(manifests, []string{"root", "root"})
	require.ErrorIs(t, err, ErrPluginDependencyGraphDuplicateRoot)

	_, err = ResolvePluginDependencyGraph(manifests, nil)
	require.ErrorIs(t, err, ErrPluginDependencyGraphRootRequired)

	_, err = ResolvePluginDependencyGraph(manifests, []string{"missing"})
	require.ErrorIs(t, err, ErrPluginDependencyGraphMissing)
}

func TestResolvePluginDependencyGraphEmptyCatalogAndNilResolver(t *testing.T) {
	_, err := NewPluginDependencyResolver(nil)
	require.ErrorIs(t, err, ErrPluginDependencyGraphEmpty)

	var resolver *PluginDependencyResolver
	_, err = resolver.Resolve([]string{"root"})
	require.ErrorIs(t, err, ErrPluginDependencyGraphEmpty)
}

func countString(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}
