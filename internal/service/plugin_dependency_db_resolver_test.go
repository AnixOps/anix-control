package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/stretchr/testify/require"
)

func TestResolvePluginInstallationDependencyGraphLoadsDesiredVersionsRecursively(t *testing.T) {
	db := newKernelTestDB(t)
	root := dependencyDBTestManifest("root", "middle")
	middle := dependencyDBTestManifest("middle", "leaf")
	leaf := dependencyDBTestManifest("leaf")
	rootRelease := seedKernelTestManifestRelease(t, db, root)
	middleRelease := seedKernelTestManifestRelease(t, db, middle)
	leafRelease := seedKernelTestManifestRelease(t, db, leaf)

	// Disabled dependencies are intentionally accepted by the graph preflight;
	// ValidatePluginInstallationPlan still rejects the enable until they are
	// enabled, preserving the legacy package-manager contract.
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: middleRelease.PluginID, Target: "control", DesiredVersion: middleRelease.Version,
		State: "disabled", Enabled: false,
	}).Error)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: leafRelease.PluginID, Target: "control", DesiredVersion: leafRelease.Version,
		State: "disabled", Enabled: false,
	}).Error)

	resolution, err := ResolvePluginInstallationDependencyGraph(db, rootRelease, "control")
	require.NoError(t, err)
	require.Equal(t, []string{"leaf", "middle", "root"}, resolution.Order)
}

func TestResolvePluginInstallationDependencyGraphRejectsMissingInstallationOrRelease(t *testing.T) {
	db := newKernelTestDB(t)
	root := dependencyDBTestManifest("root", "missing")
	rootRelease := seedKernelTestManifestRelease(t, db, root)

	_, err := ResolvePluginInstallationDependencyGraph(db, rootRelease, "control")
	require.ErrorIs(t, err, ErrPluginDependencyGraphMissing)
	require.ErrorIs(t, err, ErrPluginDependencyUnsatisfied)

	// An installation without its desired release is also a missing graph
	// vertex, rather than an implicit latest-version selection.
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: "missing", Target: "control", DesiredVersion: "9.9.9", State: "disabled",
	}).Error)
	_, err = ResolvePluginInstallationDependencyGraph(db, rootRelease, "control")
	require.ErrorIs(t, err, ErrPluginDependencyGraphMissing)
	require.Contains(t, err.Error(), "missing@9.9.9")
}

func TestResolvePluginInstallationDependencyGraphRejectsCycleAndTransitiveConflict(t *testing.T) {
	t.Run("cycle", func(t *testing.T) {
		db := newKernelTestDB(t)
		root := dependencyDBTestManifest("root", "middle")
		middle := dependencyDBTestManifest("middle", "root")
		rootRelease := seedKernelTestManifestRelease(t, db, root)
		middleRelease := seedKernelTestManifestRelease(t, db, middle)
		require.NoError(t, db.Create(&model.PluginInstallation{
			PluginID: middleRelease.PluginID, Target: "control", DesiredVersion: middleRelease.Version,
			State: "disabled", Enabled: false,
		}).Error)

		_, err := ResolvePluginInstallationDependencyGraph(db, rootRelease, "control")
		require.ErrorIs(t, err, ErrPluginDependencyGraphCycle)
		require.Contains(t, err.Error(), "root -> middle -> root")
	})

	t.Run("transitive conflict", func(t *testing.T) {
		db := newKernelTestDB(t)
		root := dependencyDBTestManifest("root", "middle", "other")
		middle := dependencyDBTestManifest("middle")
		middle.Conflicts = []string{"other"}
		rootRelease := seedKernelTestManifestRelease(t, db, root)
		middleRelease := seedKernelTestManifestRelease(t, db, middle)
		otherRelease := seedKernelTestManifestRelease(t, db, dependencyDBTestManifest("other"))
		for _, release := range []model.PluginRelease{middleRelease, otherRelease} {
			require.NoError(t, db.Create(&model.PluginInstallation{
				PluginID: release.PluginID, Target: "control", DesiredVersion: release.Version,
				State: "disabled", Enabled: false,
			}).Error)
		}

		_, err := ResolvePluginInstallationDependencyGraph(db, rootRelease, "control")
		require.ErrorIs(t, err, ErrPluginDependencyGraphConflict)
		require.ErrorIs(t, err, ErrPluginConflict)
	})
}

func TestResolvePluginInstallationDependencyGraphReverifiesRegisteredTrustRoots(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sign := func(manifest PluginManifest) *model.PluginRelease {
		canonical, canonicalErr := CanonicalPluginManifest(manifest)
		require.NoError(t, canonicalErr)
		signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
		release, registerErr := RegisterPluginRelease(db, string(canonical), signature, publicKey)
		require.NoError(t, registerErr)
		return release
	}

	leaf := sign(dependencyDBTestManifest("signed-leaf"))
	root := dependencyDBTestManifest("signed-root", "signed-leaf")
	rootRelease := sign(root)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: leaf.PluginID, Target: "control", DesiredVersion: leaf.Version,
		State: "disabled", Enabled: false,
	}).Error)

	resolution, err := ResolvePluginInstallationDependencyGraph(db, *rootRelease, "control")
	require.NoError(t, err)
	require.Equal(t, []string{"signed-leaf", "signed-root"}, resolution.Order)
}

func TestValidatePluginInstallationPlanRunsGraphPreflightWithoutChangingEnableSemantics(t *testing.T) {
	db := newKernelTestDB(t)
	root := dependencyDBTestManifest("root", "missing")
	artifact := kernelTestArtifact("root")
	root.ArtifactSHA256 = kernelTestArtifactSHA256(artifact)
	rootRelease := seedKernelTestManifestRelease(t, db, root)
	_, err := StorePluginArtifact(db, rootRelease.ID, artifact)
	require.NoError(t, err)

	err = ValidatePluginInstallationPlan(db, rootRelease, "control", true, 0)
	require.ErrorIs(t, err, ErrPluginDependencyGraphMissing)
	require.ErrorIs(t, err, ErrPluginDependencyUnsatisfied)

	// Once the dependency graph is complete, the legacy enabled-state check is
	// still the gate that rejects a disabled dependency.
	missing := dependencyDBTestManifest("missing")
	missingRelease := seedKernelTestManifestRelease(t, db, missing)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: missingRelease.PluginID, Target: "control", DesiredVersion: missingRelease.Version,
		State: "disabled", Enabled: false,
	}).Error)
	err = ValidatePluginInstallationPlan(db, rootRelease, "control", true, 0)
	require.ErrorIs(t, err, ErrPluginDependencyUnsatisfied)
	require.NotErrorIs(t, err, ErrPluginDependencyGraphMissing)
}

func dependencyDBTestManifest(id string, dependencies ...string) PluginManifest {
	artifact := kernelTestArtifact(id)
	return PluginManifest{
		ID: id, Name: id, Version: "1.0.0", APIVersion: pluginManifestAPIVersion,
		Publisher: "AnixOps", Targets: []string{"control"}, ArtifactSHA256: kernelTestArtifactSHA256(artifact),
		Dependencies: dependencies,
	}
}
