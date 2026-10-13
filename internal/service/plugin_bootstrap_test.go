package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/builtinpkg"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type builtinTestKey struct {
	public  ed25519.PublicKey
	private ed25519.PrivateKey
}

func newBuiltinTestKey(t *testing.T) builtinTestKey {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return builtinTestKey{public: public, private: private}
}

func (k builtinTestKey) encoded() string { return base64.StdEncoding.EncodeToString(k.public) }

// builtinTestPackage builds one embedded package the way build_package.py
// --builtin does: a slim artifact without a host binary and a canonical
// manifest whose control entrypoint is the built-in host, signed with key.
func builtinTestPackage(t *testing.T, key builtinTestKey, id, version string, mutate func(*PluginManifest)) builtinpkg.Package {
	t.Helper()
	migrations := []byte(`{"format":"anixops.migrations/v1","migrations":[],"package_id":"` + id + `","version":"` + version + `"}`)
	routes := []byte(`{"api_version":"v2","package_id":"` + id + `","routes":[]}`)
	artifact := kernelTestV2Package(t, map[string][]byte{
		"compat/v2-routes.json": routes,
		"migrations/index.json": migrations,
	})
	digest := func(value []byte) string {
		sum := sha256.Sum256(value)
		return hex.EncodeToString(sum[:])
	}
	manifest := PluginManifest{
		ID: id, Name: id, Version: version, APIVersion: pluginManifestAPIVersionV2,
		Publisher: "AnixOps", Targets: []string{"control"}, Architectures: []string{"linux/amd64", "linux/arm64"},
		ArtifactSHA256:      digest(artifact),
		ControlEntrypoint:   &PluginEntrypoint{Builtin: id},
		Migrations:          &PluginMigrations{Index: "migrations/index.json", SHA256: digest(migrations)},
		CompatibilityRoutes: &PluginCompatibilityRoutes{Path: "compat/v2-routes.json", SHA256: digest(routes)},
		RouteContractDigest: digest(routes),
	}
	if mutate != nil {
		mutate(&manifest)
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	return builtinpkg.Package{
		ID: id, Version: version, Manifest: canonical, Artifact: artifact,
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key.private, canonical)),
	}
}

func builtinTestSet(packages ...builtinpkg.Package) *builtinpkg.Set {
	return &builtinpkg.Set{Packages: packages}
}

func newBuiltinTestDB(t *testing.T, key builtinTestKey) *gorm.DB {
	t.Helper()
	db := newKernelTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.OperationLog{}))
	_, err := EnsurePluginTrustRoot(db, key.public)
	require.NoError(t, err)
	return db
}

func builtinResults(results []BuiltinPackageResult) map[string]BuiltinPackageResult {
	out := map[string]BuiltinPackageResult{}
	for _, result := range results {
		out[result.ID] = result
	}
	return out
}

func builtinInstallation(t *testing.T, db *gorm.DB, id string) model.PluginInstallation {
	t.Helper()
	var installation model.PluginInstallation
	require.NoError(t, db.First(&installation, "plugin_id = ? AND target = ?", id, "control").Error)
	return installation
}

func builtinAuditCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&model.OperationLog{}).Where("action = ?", builtinInstallationAction).Count(&count).Error)
	return count
}

func TestBuiltinBootstrapColdStartInstallsEveryPackageInOrder(t *testing.T) {
	key := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, key)
	set := builtinTestSet(
		builtinTestPackage(t, key, "affiliate", "5.0.0", func(m *PluginManifest) { m.Dependencies = []string{"order"} }),
		builtinTestPackage(t, key, "identity-platform", "5.0.0", nil),
		builtinTestPackage(t, key, "order", "5.0.0", nil),
		builtinTestPackage(t, key, "platform", "5.0.0", nil),
	)

	results := BootstrapBuiltinPackages(db, key.encoded(), set)

	order := make([]string, 0, len(results))
	for _, result := range results {
		require.NoError(t, result.Err, result.ID)
		require.Equal(t, BuiltinActionInstalled, result.Action, result.ID)
		order = append(order, result.ID)
	}
	// identity first, then platform, then by id with dependencies first.
	require.Equal(t, []string{"identity-platform", "platform", "order", "affiliate"}, order)
	for _, id := range order {
		installation := builtinInstallation(t, db, id)
		require.Equal(t, "5.0.0", installation.DesiredVersion)
		require.True(t, installation.Enabled)
		require.Equal(t, "pending", installation.State)
		require.EqualValues(t, 1, installation.LifecycleGeneration)
		var release model.PluginRelease
		require.NoError(t, db.First(&release, "plugin_id = ? AND version = ?", id, "5.0.0").Error)
		require.Equal(t, PluginTrustRootFingerprint(key.public), release.TrustRootFingerprint)
		artifact, err := GetPluginArtifact(db, release.ID)
		require.NoError(t, err)
		require.NotEmpty(t, artifact.Data)
	}
	require.EqualValues(t, 4, builtinAuditCount(t, db))
}

func TestBuiltinBootstrapSameVersionRestartIsANoOp(t *testing.T) {
	key := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, key)
	set := builtinTestSet(builtinTestPackage(t, key, "knowledge", "5.0.0", nil), builtinTestPackage(t, key, "plan", "5.0.0", nil))
	BootstrapBuiltinPackages(db, key.encoded(), set)
	require.NoError(t, db.Model(&model.PluginInstallation{}).Where("plugin_id = ?", "knowledge").
		Updates(map[string]any{"state": "healthy", "observed_version": "5.0.0"}).Error)
	before := builtinInstallation(t, db, "knowledge")
	audits := builtinAuditCount(t, db)

	for _, result := range BootstrapBuiltinPackages(db, key.encoded(), set) {
		require.NoError(t, result.Err)
		require.Equal(t, BuiltinActionUnchanged, result.Action, result.ID)
	}

	after := builtinInstallation(t, db, "knowledge")
	require.Equal(t, before.LifecycleGeneration, after.LifecycleGeneration)
	require.Equal(t, "healthy", after.State)
	require.Equal(t, audits, builtinAuditCount(t, db))
	var releases, artifacts int64
	require.NoError(t, db.Model(&model.PluginRelease{}).Count(&releases).Error)
	require.NoError(t, db.Model(&model.PluginArtifact{}).Count(&artifacts).Error)
	require.EqualValues(t, 2, releases)
	require.EqualValues(t, 2, artifacts)
}

func TestBuiltinBootstrapUpgradesToANewVersion(t *testing.T) {
	key := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, key)
	BootstrapBuiltinPackages(db, key.encoded(), builtinTestSet(builtinTestPackage(t, key, "knowledge", "5.0.0", nil)))
	require.NoError(t, db.Model(&model.PluginInstallation{}).Where("plugin_id = ?", "knowledge").
		Updates(map[string]any{"state": "healthy", "observed_version": "5.0.0"}).Error)

	results := BootstrapBuiltinPackages(db, key.encoded(), builtinTestSet(builtinTestPackage(t, key, "knowledge", "5.1.0", nil)))

	require.Len(t, results, 1)
	require.NoError(t, results[0].Err)
	require.Equal(t, BuiltinActionUpgraded, results[0].Action)
	installation := builtinInstallation(t, db, "knowledge")
	require.Equal(t, "5.1.0", installation.DesiredVersion)
	require.Equal(t, "5.0.0", installation.PreviousVersion)
	require.Equal(t, "5.0.0", installation.ObservedVersion, "the lifecycle worker moves the host")
	require.Equal(t, "pending", installation.State)
	require.EqualValues(t, 2, installation.LifecycleGeneration, "a new generation runs the migrations again")
	var releases int64
	require.NoError(t, db.Model(&model.PluginRelease{}).Where("plugin_id = ?", "knowledge").Count(&releases).Error)
	require.EqualValues(t, 2, releases, "the old release stays for audit")
	var audit model.OperationLog
	require.NoError(t, db.Where("action = ?", builtinInstallationAction).Order("id DESC").First(&audit).Error)
	require.Contains(t, audit.Content, `"from_version":"5.0.0"`)
	require.Contains(t, audit.Content, `"to_version":"5.1.0"`)
}

func TestBuiltinBootstrapEnablesADisabledInstallation(t *testing.T) {
	key := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, key)
	set := builtinTestSet(builtinTestPackage(t, key, "ticket", "5.0.0", nil))
	BootstrapBuiltinPackages(db, key.encoded(), set)
	require.NoError(t, db.Model(&model.PluginInstallation{}).Where("plugin_id = ?", "ticket").
		Updates(map[string]any{"state": "disabled", "enabled": false, "lifecycle_generation": 4}).Error)

	results := BootstrapBuiltinPackages(db, key.encoded(), set)

	require.Equal(t, BuiltinActionEnabled, results[0].Action)
	installation := builtinInstallation(t, db, "ticket")
	require.True(t, installation.Enabled)
	require.Equal(t, "pending", installation.State)
	require.EqualValues(t, 5, installation.LifecycleGeneration)
}

func TestBuiltinBootstrapNeverDowngrades(t *testing.T) {
	key := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, key)
	BootstrapBuiltinPackages(db, key.encoded(), builtinTestSet(builtinTestPackage(t, key, "plan", "5.1.0", nil)))

	results := BootstrapBuiltinPackages(db, key.encoded(), builtinTestSet(builtinTestPackage(t, key, "plan", "5.0.0", nil)))

	require.Equal(t, BuiltinActionFailed, results[0].Action)
	require.ErrorContains(t, results[0].Err, "newer than the 5.0.0 this binary carries")
	installation := builtinInstallation(t, db, "plan")
	require.Equal(t, "5.1.0", installation.DesiredVersion)
	require.Equal(t, "pending", installation.State, "the installation is left to its own release")

	problems, err := BuiltinPackageProblems(db, key.encoded(), builtinTestSet(builtinTestPackage(t, key, "plan", "5.0.0", nil)), nil)
	require.NoError(t, err)
	require.Len(t, problems, 1)
	require.Contains(t, problems[0].Reason, "wants 5.1.0")
}

func TestBuiltinBootstrapMarksAFailedInstallation(t *testing.T) {
	key := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, key)
	BootstrapBuiltinPackages(db, key.encoded(), builtinTestSet(builtinTestPackage(t, key, "plan", "5.0.0", nil)))
	// The next binary's release cannot be validated: a conflicting package
	// is enabled.
	require.NoError(t, db.Model(&model.PluginInstallation{}).Where("plugin_id = ?", "plan").Update("state", "healthy").Error)
	conflicting := builtinTestPackage(t, key, "ticket", "5.0.0", nil)
	BootstrapBuiltinPackages(db, key.encoded(), builtinTestSet(conflicting))

	results := BootstrapBuiltinPackages(db, key.encoded(), builtinTestSet(
		builtinTestPackage(t, key, "plan", "5.1.0", func(m *PluginManifest) { m.Conflicts = []string{"ticket"} }),
	))

	require.Equal(t, BuiltinActionFailed, results[0].Action)
	require.ErrorContains(t, results[0].Err, "conflicts with ticket")
	installation := builtinInstallation(t, db, "plan")
	require.Equal(t, "5.0.0", installation.DesiredVersion)
	require.Equal(t, "failed", installation.State)
	require.Contains(t, installation.LastError, "built-in package bootstrap")
}

func TestBuiltinBootstrapRejectsBadSignaturesAndDigestsAndKeepsTheRest(t *testing.T) {
	key := newBuiltinTestKey(t)
	other := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, key)
	badSignature := builtinTestPackage(t, key, "order", "5.0.0", nil)
	badSignature.Signature = builtinTestPackage(t, other, "order", "5.0.0", nil).Signature
	badDigest := builtinTestPackage(t, key, "payment", "5.0.0", nil)
	badDigest.Artifact = append(append([]byte(nil), badDigest.Artifact...), 0)
	notCanonical := builtinTestPackage(t, key, "notification", "5.0.0", nil)
	notCanonical.Manifest = append([]byte(" "), notCanonical.Manifest...)
	wrongIndex := builtinTestPackage(t, key, "wireguard", "5.0.0", nil)
	wrongIndex.Version = "5.0.1"
	fileHost := builtinTestPackage(t, key, "forward", "5.0.0", func(m *PluginManifest) {
		m.ControlEntrypoint = &PluginEntrypoint{Path: "bin/control-host", SHA256: m.ArtifactSHA256}
	})
	dependent := builtinTestPackage(t, key, "affiliate", "5.0.0", func(m *PluginManifest) { m.Dependencies = []string{"order"} })
	good := builtinTestPackage(t, key, "knowledge", "5.0.0", nil)

	results := builtinResults(BootstrapBuiltinPackages(db, key.encoded(), builtinTestSet(badSignature, badDigest, notCanonical, wrongIndex, fileHost, dependent, good)))

	require.Equal(t, BuiltinActionInvalid, results["order"].Action)
	require.ErrorContains(t, results["order"].Err, "signature verification failed")
	require.Equal(t, BuiltinActionInvalid, results["payment"].Action)
	require.ErrorContains(t, results["payment"].Err, "hash mismatch")
	require.Equal(t, BuiltinActionInvalid, results["notification"].Action)
	require.Equal(t, BuiltinActionInvalid, results["wireguard"].Action)
	require.Equal(t, BuiltinActionInvalid, results["forward"].Action)
	require.ErrorContains(t, results["forward"].Err, "built-in host")
	require.Equal(t, BuiltinActionFailed, results["affiliate"].Action)
	require.ErrorContains(t, results["affiliate"].Err, "depends on order")
	require.Equal(t, BuiltinActionInstalled, results["knowledge"].Action)
	require.NoError(t, results["knowledge"].Err)

	var installations []model.PluginInstallation
	require.NoError(t, db.Find(&installations).Error)
	require.Len(t, installations, 1)
	require.Equal(t, "knowledge", installations[0].PluginID)
	var releases int64
	require.NoError(t, db.Model(&model.PluginRelease{}).Count(&releases).Error)
	require.EqualValues(t, 1, releases, "nothing of an invalid package is written")
}

func TestBuiltinBootstrapWithAnotherKeyRejectsEveryPackage(t *testing.T) {
	key := newBuiltinTestKey(t)
	configured := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, configured)
	set := builtinTestSet(builtinTestPackage(t, key, "knowledge", "5.0.0", nil), builtinTestPackage(t, key, "plan", "5.0.0", nil))

	for _, result := range BootstrapBuiltinPackages(db, configured.encoded(), set) {
		require.Equal(t, BuiltinActionInvalid, result.Action, result.ID)
	}
	for _, result := range BootstrapBuiltinPackages(db, "not base64", set) {
		require.Equal(t, BuiltinActionInvalid, result.Action, result.ID)
		require.ErrorContains(t, result.Err, "plugins.official_public_key")
	}
	var count int64
	require.NoError(t, db.Model(&model.PluginInstallation{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestBuiltinBootstrapReportsMissingDependenciesAndCycles(t *testing.T) {
	key := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, key)
	results := builtinResults(BootstrapBuiltinPackages(db, key.encoded(), builtinTestSet(
		builtinTestPackage(t, key, "order", "5.0.0", func(m *PluginManifest) { m.Dependencies = []string{"payment"} }),
		builtinTestPackage(t, key, "payment", "5.0.0", func(m *PluginManifest) { m.Dependencies = []string{"order"} }),
		builtinTestPackage(t, key, "affiliate", "5.0.0", func(m *PluginManifest) { m.Dependencies = []string{"no-such-package"} }),
		builtinTestPackage(t, key, "plan", "5.0.0", nil),
	)))
	require.ErrorContains(t, results["order"].Err, "cycle")
	require.ErrorContains(t, results["payment"].Err, "cycle")
	require.ErrorContains(t, results["affiliate"].Err, "does not carry")
	require.Equal(t, BuiltinActionInstalled, results["plan"].Action)
}

func TestBuiltinBootstrapRefusesAReleaseRegisteredWithOtherContents(t *testing.T) {
	key := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, key)
	imported := builtinTestPackage(t, key, "knowledge", "5.0.0", func(m *PluginManifest) { m.Name = "Imported" })
	_, err := RegisterPluginRelease(db, string(imported.Manifest), imported.Signature, key.public)
	require.NoError(t, err)

	results := BootstrapBuiltinPackages(db, key.encoded(), builtinTestSet(builtinTestPackage(t, key, "knowledge", "5.0.0", nil)))

	require.Equal(t, BuiltinActionFailed, results[0].Action)
	require.ErrorContains(t, results[0].Err, "other contents")
}

func TestBuiltinBootstrapWithoutPackagesDoesNothing(t *testing.T) {
	key := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, key)
	require.Nil(t, BootstrapBuiltinPackages(db, key.encoded(), &builtinpkg.Set{}))
	require.Nil(t, BootstrapBuiltinPackages(db, key.encoded(), nil))
}

func TestBuiltinPackageProblemsReportsWhatNeedsAnAdministrator(t *testing.T) {
	key := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, key)
	invalid := builtinTestPackage(t, key, "order", "5.0.0", nil)
	invalid.Signature = builtinTestPackage(t, newBuiltinTestKey(t), "order", "5.0.0", nil).Signature
	set := builtinTestSet(
		builtinTestPackage(t, key, "knowledge", "5.0.0", nil),
		builtinTestPackage(t, key, "plan", "5.0.0", nil),
		builtinTestPackage(t, key, "ticket", "5.0.0", nil),
		invalid,
	)
	BootstrapBuiltinPackages(db, key.encoded(), set)
	require.NoError(t, db.Model(&model.PluginInstallation{}).Where("plugin_id = ?", "plan").
		Updates(map[string]any{"state": "failed", "last_error": "host exited"}).Error)
	require.NoError(t, db.Model(&model.PluginInstallation{}).Where("plugin_id = ?", "ticket").Update("enabled", false).Error)

	problems, err := BuiltinPackageProblems(db, key.encoded(), set, nil)
	require.NoError(t, err)
	byID := map[string]BuiltinPackageStatus{}
	for _, problem := range problems {
		byID[problem.ID] = problem
	}
	require.Len(t, byID, 3, "knowledge is pending: its host is starting")
	require.True(t, byID["order"].Invalid)
	require.Equal(t, "failed", byID["plan"].State)
	require.Equal(t, "host exited", byID["plan"].LastError)
	require.Contains(t, byID["ticket"].Reason, "not enabled")

	problems, err = BuiltinPackageProblems(db, key.encoded(), nil, errors.New("broken index"))
	require.NoError(t, err)
	require.Equal(t, []BuiltinPackageStatus{{ID: "index", Invalid: true, Reason: "broken index"}}, problems)
}

// A built-in host release materializes without an entrypoint file, and only
// for the release this binary carries.
func TestMaterializeBuiltinHostReleaseRequiresTheEmbeddedManifest(t *testing.T) {
	key := newBuiltinTestKey(t)
	db := newBuiltinTestDB(t, key)
	embedded := builtinTestPackage(t, key, "knowledge", "5.0.0", nil)
	set := builtinTestSet(embedded)
	BootstrapBuiltinPackages(db, key.encoded(), set)

	_, err := MaterializePluginControlArtifact(db, key.public, "knowledge", "5.0.0", t.TempDir())
	require.ErrorIs(t, err, ErrBuiltinReleaseNotEmbedded, "the test binary embeds no knowledge package")

	restore := builtinpkg.SetDefault(set)
	defer restore()
	root := t.TempDir()
	ref, err := MaterializePluginControlArtifact(db, key.public, "knowledge", "5.0.0", root)
	require.NoError(t, err)
	require.True(t, ref.Builtin)
	require.Empty(t, ref.EntrypointPath)
	require.Empty(t, ref.EntrypointSHA256)
	require.FileExists(t, ref.ArtifactPath)
	require.FileExists(t, ref.ManifestPath)
	require.NoFileExists(t, filepath.Join(filepath.Dir(ref.ArtifactPath), "control-host"))
	manifest, err := os.ReadFile(ref.ManifestPath)
	require.NoError(t, err)
	require.Equal(t, embedded.Manifest, manifest)

	// Another release of the package, even one signed with the official key,
	// has no code in this binary.
	newer := builtinTestPackage(t, key, "knowledge", "5.1.0", nil)
	release, err := RegisterPluginRelease(db, string(newer.Manifest), newer.Signature, key.public)
	require.NoError(t, err)
	_, err = StorePluginArtifact(db, release.ID, newer.Artifact)
	require.NoError(t, err)
	_, err = MaterializePluginControlArtifact(db, key.public, "knowledge", "5.1.0", root)
	require.ErrorIs(t, err, ErrBuiltinReleaseNotEmbedded)
}

func TestBuiltinControlEntrypointManifestRules(t *testing.T) {
	key := newBuiltinTestKey(t)
	valid := builtinTestPackage(t, key, "knowledge", "5.0.0", nil)
	_, err := VerifyPluginRelease(string(valid.Manifest), valid.Signature, key.public)
	require.NoError(t, err)
	require.Contains(t, string(valid.Manifest), `"control_entrypoint":{"builtin":"knowledge"}`)

	for name, mutate := range map[string]func(*PluginManifest){
		"other package": func(m *PluginManifest) { m.ControlEntrypoint = &PluginEntrypoint{Builtin: "plan"} },
		"also a file":   func(m *PluginManifest) { m.ControlEntrypoint.Path = "bin/control-host" },
		"agent target": func(m *PluginManifest) {
			m.Targets = []string{"control", "agent"}
			m.AgentEntrypoint = &PluginEntrypoint{Path: "agent/entrypoints.json", SHA256: m.ArtifactSHA256}
			m.RuntimeAPIVersion = agentRuntimeAPIVersionV110
		},
		"builtin agent entrypoint": func(m *PluginManifest) {
			m.Targets = []string{"control", "agent"}
			m.ControlEntrypoint = &PluginEntrypoint{Path: "bin/control-host", SHA256: m.ArtifactSHA256}
			m.AgentEntrypoint = &PluginEntrypoint{Builtin: "knowledge"}
			m.RuntimeAPIVersion = agentRuntimeAPIVersionV110
		},
	} {
		invalid := builtinTestPackage(t, key, "knowledge", "5.0.0", mutate)
		_, err := VerifyPluginRelease(string(invalid.Manifest), invalid.Signature, key.public)
		require.Error(t, err, name)
	}
}
