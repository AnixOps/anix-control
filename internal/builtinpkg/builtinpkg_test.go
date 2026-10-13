package builtinpkg

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		IndexName: {Data: []byte(`{"format":"anixops.builtin-packages/v1","public_key":"official-public-key.pem","packages":[` +
			`{"id":"plan","version":"4.3.0","artifact":"plan-4.3.0.anxp","manifest":"plan-4.3.0.manifest.json","signature":"plan-4.3.0.manifest.sig"},` +
			`{"id":"knowledge","version":"4.3.0","artifact":"knowledge-4.3.0.anxp","manifest":"knowledge-4.3.0.manifest.json","signature":"knowledge-4.3.0.manifest.sig"}]}`)},
		"official-public-key.pem":       {Data: []byte("-----BEGIN PUBLIC KEY-----\n")},
		"plan-4.3.0.anxp":               {Data: []byte("plan artifact")},
		"plan-4.3.0.manifest.json":      {Data: []byte(`{"id":"plan"}`)},
		"plan-4.3.0.manifest.sig":       {Data: []byte("c2lnbmF0dXJl\n")},
		"knowledge-4.3.0.anxp":          {Data: []byte("knowledge artifact")},
		"knowledge-4.3.0.manifest.json": {Data: []byte(`{"id":"knowledge"}`)},
		"knowledge-4.3.0.manifest.sig":  {Data: []byte("c2lnbmF0dXJl")},
	}
}

func TestLoadReadsTheIndexSortedByID(t *testing.T) {
	set, err := Load(testFS())
	require.NoError(t, err)
	require.Len(t, set.Packages, 2)
	require.Equal(t, "knowledge", set.Packages[0].ID)
	require.Equal(t, "plan", set.Packages[1].ID)
	require.Equal(t, "c2lnbmF0dXJl", set.Packages[1].Signature)
	require.Equal(t, []byte("plan artifact"), set.Packages[1].Artifact)
	require.NotEmpty(t, set.PublicKeyPEM)
	found, ok := set.Lookup("plan")
	require.True(t, ok)
	require.Equal(t, "4.3.0", found.Version)
	_, ok = set.Lookup("order")
	require.False(t, ok)
}

func TestLoadWithoutAnIndexIsEmpty(t *testing.T) {
	set, err := Load(fstest.MapFS{"README.md": {Data: []byte("placeholder")}})
	require.NoError(t, err)
	require.True(t, set.Empty())
}

func TestLoadRejectsMalformedIndexes(t *testing.T) {
	for name, index := range map[string]string{
		"format":    `{"format":"other","packages":[]}`,
		"unknown":   `{"format":"anixops.builtin-packages/v1","packages":[],"extra":1}`,
		"traversal": `{"format":"anixops.builtin-packages/v1","packages":[{"id":"plan","version":"4.3.0","artifact":"../plan.anxp","manifest":"plan-4.3.0.manifest.json","signature":"plan-4.3.0.manifest.sig"}]}`,
		"duplicate": `{"format":"anixops.builtin-packages/v1","packages":[{"id":"plan","version":"4.3.0","artifact":"plan-4.3.0.anxp","manifest":"plan-4.3.0.manifest.json","signature":"plan-4.3.0.manifest.sig"},{"id":"plan","version":"4.3.0","artifact":"plan-4.3.0.anxp","manifest":"plan-4.3.0.manifest.json","signature":"plan-4.3.0.manifest.sig"}]}`,
		"missing":   `{"format":"anixops.builtin-packages/v1","packages":[{"id":"order","version":"4.3.0","artifact":"order-4.3.0.anxp","manifest":"order-4.3.0.manifest.json","signature":"order-4.3.0.manifest.sig"}]}`,
		"two":       `{"format":"anixops.builtin-packages/v1","packages":[]} {}`,
	} {
		fsys := testFS()
		fsys[IndexName] = &fstest.MapFile{Data: []byte(index)}
		_, err := Load(fsys)
		require.Error(t, err, name)
	}
}

func TestEmbeddedPlaceholderLoads(t *testing.T) {
	// A checkout without built packages embeds only the placeholder; a
	// build that wrote dist/ embeds a valid index.
	_, err := Embedded()
	require.NoError(t, err)
}

func TestSetDefaultOverridesAndRestores(t *testing.T) {
	set, err := Load(testFS())
	require.NoError(t, err)
	restore := SetDefault(set)
	current, err := Default()
	require.NoError(t, err)
	require.Same(t, set, current)
	restore()
	current, err = Default()
	require.NoError(t, err)
	embedded, err := Embedded()
	require.NoError(t, err)
	require.Same(t, embedded, current)
}

func TestHostVersionWithoutEmbeddedPackageTakesTheKernelVersion(t *testing.T) {
	embedded, err := Embedded()
	require.NoError(t, err)
	if _, ok := embedded.Lookup("no-such-package"); ok {
		t.Skip("unexpected package")
	}
	version, err := HostVersion("no-such-package", "4.3.0")
	require.NoError(t, err)
	require.Equal(t, "4.3.0", version)
	for _, candidate := range embedded.Packages {
		version, err := HostVersion(candidate.ID, "")
		require.NoError(t, err)
		require.Equal(t, candidate.Version, version)
		_, err = HostVersion(candidate.ID, candidate.Version+"-other")
		require.Error(t, err)
	}
}
