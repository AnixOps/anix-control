package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// logLines collects log lines.
type logLines []string

func (l *logLines) logf(format string, args ...any) { *l = append(*l, fmt.Sprintf(format, args...)) }

func (l logLines) String() string { return strings.Join(l, "\n") }

func TestResolveRootKeyGeneratesItAndFillsTheReaders(t *testing.T) {
	cfg := config.Defaults()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	var lines logLines
	resolved, err := resolveRootKey(cfg, true, lines.logf)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, rootkey.SourceGenerated, resolved.KeySource)
	assert.Equal(t, resolved.Key.Base64(), cfg.ModuleRuntime.CAKEK, "every reader of module_runtime.ca_kek gets the root key")
	assert.Equal(t, resolved.Key.DerivedJWTSecret(), cfg.JWT.Secret)
	assert.Equal(t, resolved.Key.DerivedIdentityKEK(), cfg.Identity.KEK)
	assert.True(t, cfg.ModuleRuntime.BuiltinCA(), "the built-in CA always runs")
	require.NoError(t, cfg.ValidateForServer())
	assert.Contains(t, lines.String(), "Generated the root key "+filepath.Join(cfg.DataDir, "root.key"))
	assert.Contains(t, lines.String(), "JWT secret: derived from the root key; identity KEK: derived from the root key")
	for _, secret := range []string{cfg.ModuleRuntime.CAKEK, cfg.JWT.Secret, cfg.Identity.KEK} {
		assert.NotContains(t, lines.String(), secret, "a key is never logged")
	}
	info, err := os.Stat(filepath.Join(cfg.DataDir, "root.key"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestResolveRootKeyAdoptsTheLegacyKeysAndDropsATemplateSecret(t *testing.T) {
	legacy := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
	cfg := config.Defaults()
	cfg.DataDir = t.TempDir()
	cfg.ModuleRuntime.CAKEK = legacy
	cfg.JWT.Secret = "your-jwt-secret-key-change-in-production"
	var lines logLines
	resolved, err := resolveRootKey(cfg, true, lines.logf)
	require.NoError(t, err)
	assert.Equal(t, legacy, cfg.ModuleRuntime.CAKEK, "the root key takes the value of module_runtime.ca_kek")
	assert.Equal(t, resolved.Key.DerivedJWTSecret(), cfg.JWT.Secret, "a template secret is not adopted")
	assert.Contains(t, lines.String(), "jwt.secret is a template value and is not used")
	file, err := rootkey.ReadFile(resolved.Path)
	require.NoError(t, err)
	assert.Empty(t, file.AdoptedJWTSecret)

	// The next start without the legacy key in the configuration is the same.
	again := config.Defaults()
	again.DataDir = cfg.DataDir
	_, err = resolveRootKey(again, true, lines.logf)
	require.NoError(t, err)
	assert.Equal(t, legacy, again.ModuleRuntime.CAKEK)
	assert.Equal(t, cfg.JWT.Secret, again.JWT.Secret)
}

func TestResolveRootKeyOfACommandNeverWrites(t *testing.T) {
	cfg := config.Defaults()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	resolved, err := resolveRootKey(cfg, false, func(string, ...any) {})
	require.NoError(t, err)
	assert.Nil(t, resolved)
	assert.Empty(t, cfg.ModuleRuntime.CAKEK, "without root.key a command has no key, as without module_runtime.ca_kek before")
	_, err = os.Stat(cfg.DataDir)
	assert.True(t, os.IsNotExist(err))
}

func TestResolveRootKeyRefusesAConflict(t *testing.T) {
	cfg := config.Defaults()
	cfg.DataDir = t.TempDir()
	_, err := resolveRootKey(cfg, true, func(string, ...any) {})
	require.NoError(t, err)
	other := config.Defaults()
	other.DataDir = cfg.DataDir
	other.ModuleRuntime.CAKEK = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	_, err = resolveRootKey(other, true, func(string, ...any) {})
	require.ErrorContains(t, err, "hold different keys")
}

// rootKeyConfig is a configuration whose root key is key.
func rootKeyConfig(key []byte) *config.Config {
	cfg := config.Defaults()
	cfg.DataDir = "/var/lib/anix-control"
	cfg.ModuleRuntime.CAKEK = base64.StdEncoding.EncodeToString(key)
	return cfg
}

func TestVerifyRootKeyRefusesAKeyThatCannotOpenTheCA(t *testing.T) {
	ctx := context.Background()
	db := openCommandDB(t)
	right, wrong := bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{3}, 32)
	saved := activeRootKey
	activeRootKey = nil
	t.Cleanup(func() { activeRootKey = saved })

	// An empty database: nothing to open.
	require.NoError(t, verifyRootKey(ctx, rootKeyConfig(wrong), db, t.Logf))

	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: "default", KEK: right})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(ctx))
	require.NoError(t, verifyRootKey(ctx, rootKeyConfig(right), db, t.Logf))

	err = verifyRootKey(ctx, rootKeyConfig(wrong), db, t.Logf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not open the module CA key stored in this database")
	assert.Contains(t, err.Error(), "restore the root.key that belongs to this database")
	assert.Contains(t, err.Error(), "/var/lib/anix-control/root.key")
	assert.NotContains(t, err.Error(), base64.StdEncoding.EncodeToString(wrong))

	// The forward link CA is checked the same way.
	link, err := agentpki.NewLinkAuthority(agentpki.LinkAuthorityOptions{DB: db, Cluster: "default", KEK: wrong})
	require.NoError(t, err)
	require.NoError(t, link.Ensure(ctx))
	require.ErrorContains(t, verifyRootKey(ctx, rootKeyConfig(right), db, t.Logf), "forward link CA key")

	// Without a root key there is nothing to check.
	require.NoError(t, verifyRootKey(ctx, config.Defaults(), db, t.Logf))
}

// Without a CA, the sealed ACME entry decides; with one, it only warns.
func TestVerifyRootKeyChecksTheACMECache(t *testing.T) {
	ctx := context.Background()
	db := openCommandDB(t)
	right, wrong := bytes.Repeat([]byte{4}, 32), bytes.Repeat([]byte{5}, 32)
	store, err := panelcert.NewStore(db, right, nil)
	require.NoError(t, err)
	require.NoError(t, store.Put(ctx, "acme-000000000000/panel.example.com", []byte("a certificate and its key")))

	require.NoError(t, verifyRootKey(ctx, rootKeyConfig(right), db, t.Logf))
	require.ErrorContains(t, verifyRootKey(ctx, rootKeyConfig(wrong), db, t.Logf), "ACME account or certificate key")

	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: "default", KEK: wrong})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(ctx))
	var lines logLines
	require.NoError(t, verifyRootKey(ctx, rootKeyConfig(wrong), db, lines.logf))
	assert.Contains(t, lines.String(), "opens the CA keys but not a stored ACME account or certificate key")
}

// A start with another root key than the one that sealed the database
// stops before anything uses the key.
func TestBootstrapDatabaseRefusesAnotherRootKey(t *testing.T) {
	bootstrapDatabases(t, func(t *testing.T, cfg *config.Config) {
		ctx := context.Background()
		savedDir := forwardLegacyDataDir
		forwardLegacyDataDir = t.TempDir()
		t.Cleanup(func() { forwardLegacyDataDir = savedDir })

		cfg.ModuleRuntime.CAKEK = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{6}, 32))
		require.NoError(t, bootstrapDatabase(ctx, cfg), "the first start creates the CA with the key")
		authority, err := modulepki.FromConfig(cfg.ModuleRuntime, database.Get())
		require.NoError(t, err)
		bundle, err := authority.TrustBundle(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, bundle, "the CA exists")

		cfg.ModuleRuntime.CAKEK = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
		err = bootstrapDatabase(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not open the module CA key")

		// doctor reports it instead of stopping.
		deferRootKeyCheck = true
		t.Cleanup(func() { deferRootKeyCheck = false })
		require.NoError(t, bootstrapDatabase(ctx, cfg))
		byID := doctorByID(doctorChecks(ctx, cfg, database.Get(), doctorEnvironment{
			resolver: fakeResolver{}, localAddresses: panelcert.LocalAddresses,
			checkPort: func(string) (string, string) { return panelcert.PortFree, "" },
		}))
		require.Equal(t, doctorFail, byID["root_key"].Level)
		assert.Contains(t, byID["root_key"].Message, "does not open the module CA key")
	})
}
