package builtin

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	require.Len(t, packages, 15)
	require.Equal(t, packages, IDs())
	for _, host := range Hosts() {
		_, err := os.Stat(filepath.Join("..", host.ID, "control", "main.go"))
		require.NoError(t, err, "%s: own host", host.ID)
	}
}

func TestMainRejectsUsageErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"a", "b"}, {"-h"}, {"no-such-package"}} {
		var stderr bytes.Buffer
		code := Main(context.Background(), args, Options{Stderr: &stderr})
		require.Equal(t, 2, code, "%v", args)
		require.Contains(t, stderr.String(), "identity-platform")
	}
}

func TestMainNeedsAVersion(t *testing.T) {
	var stderr bytes.Buffer
	code := Main(context.Background(), []string{"knowledge"}, Options{Getenv: func(string) string { return "" }, Stderr: &stderr})
	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), VersionEnvironment)

	stderr.Reset()
	code = Main(context.Background(), []string{"knowledge"}, Options{
		Getenv: func(string) string { return "4.3.0" },
		Version: func(id, requested string) (string, error) {
			require.Equal(t, "knowledge", id)
			require.Equal(t, "4.3.0", requested)
			return "", os.ErrPermission
		},
		Stderr: &stderr,
	})
	require.Equal(t, 1, code)
	require.True(t, strings.Contains(stderr.String(), "knowledge control host"), stderr.String())
}

func TestRunRejectsUnknownPackagesAndEmptyVersions(t *testing.T) {
	require.ErrorIs(t, Run(context.Background(), "nope", "1.0.0"), ErrUnknownHost)
	require.ErrorContains(t, Run(context.Background(), "knowledge", " "), "version is required")
}
