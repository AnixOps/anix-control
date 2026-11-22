package builtin

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHostsMatchThePackageMatrix keeps the linked hosts equal to the
// official packages: every packages/<id> with a manifest template has a
// host that runs from its own control/ directory.
func TestHostsMatchThePackageMatrix(t *testing.T) {
	manifests, err := filepath.Glob(filepath.Join("..", "*", "manifest.template.json"))
	require.NoError(t, err)
	var packages []string
	for _, manifest := range manifests {
		packages = append(packages, filepath.Base(filepath.Dir(manifest)))
	}
	sort.Strings(packages)
	require.Equal(t, packages, IDs())
	for _, host := range Hosts() {
		_, err := os.Stat(filepath.Join("..", host.ID, "control", "main.go"))
		require.NoError(t, err, "%s: own host", host.ID)
	}
}

func TestRunRejectsUnknownPackagesAndEmptyVersions(t *testing.T) {
	require.ErrorIs(t, Run(context.Background(), "nope", "1.0.0"), ErrUnknownHost)
	require.ErrorContains(t, Run(context.Background(), "knowledge", " "), "version is required")
}
