package rootkey

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vectorKey is the key 0x00 0x01 ... 0x1f.
func vectorKey(t *testing.T) Key {
	t.Helper()
	raw := make([]byte, Size)
	for i := range raw {
		raw[i] = byte(i)
	}
	key, err := FromBytes(raw)
	require.NoError(t, err)
	return key
}

// The derivations are part of the stored data's format: a change makes the
// sealed join tokens, the ACME keys, every issued JWT and the identity
// signing keys unreadable. The vectors were computed with an independent
// HKDF-SHA256 (RFC 5869, no salt, 32 bytes) and must never change.
func TestDerivationVectorsArePinned(t *testing.T) {
	key := vectorKey(t)
	vectors := map[string]string{
		LabelJWT:         "65f666d6083ad1f96c7c9ab2bea109298c251204b6e56c542b70b856627388df",
		LabelIdentityKEK: "21d8803b23adf3f896a385e041f97feb206b9c6fae85290476295c1e5002308c",
		LabelJoinToken:   "6a5cfab8e87ece9007324999e3063c613a48d36e82c64ae97f4f67bf010b2a49",
		LabelACMECache:   "8ff73fe535bfc80c51848fe8a7f0df8fbb97dd4e93a699d555a521c315418290",
	}
	for label, want := range vectors {
		assert.Equal(t, want, fmt.Sprintf("%x", key.Derive(label)), label)
	}
	assert.Equal(t, "ZfZm1gg60flsfJqyvqEJKYwlEgS25WxUK3C4VmJziN8=", key.DerivedJWTSecret())
	assert.Equal(t, "IdiAOyOt8/iWo4XgQfl/6yBrnG+uhSkEdilcHlACMIw=", key.DerivedIdentityKEK())
	assert.Equal(t, "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=", key.Base64())
	// The labels themselves are pinned too.
	assert.Equal(t, "anixops jwt v1", LabelJWT)
	assert.Equal(t, "anixops identity kek v1", LabelIdentityKEK)
	assert.Equal(t, "anixops node-group join token v1", LabelJoinToken)
	assert.Equal(t, "anixops acme cache v1", LabelACMECache)
}

func TestKeyNeverFormatsItsBytes(t *testing.T) {
	key := vectorKey(t)
	file := File{Key: key, AdoptedJWTSecret: "a-jwt-secret-that-must-not-leak", AdoptedIdentityKEK: key.Base64()}
	resolved := Resolved{Key: key, JWTSecret: "a-jwt-secret-that-must-not-leak", IdentityKEK: key.Base64()}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x", "%q"} {
		for _, value := range []any{key, file, resolved, &resolved} {
			text := fmt.Sprintf(verb, value)
			assert.NotContains(t, text, key.Base64(), verb)
			assert.NotContains(t, text, "a-jwt-secret-that-must-not-leak", verb)
			assert.NotContains(t, text, "000102030405", verb)
		}
	}
}

func TestFileRoundTrip(t *testing.T) {
	key := vectorKey(t)
	for _, file := range []File{
		{Key: key},
		{Key: key, AdoptedJWTSecret: "s3cret with spaces: and colons\n"},
		{Key: key, AdoptedIdentityKEK: key.DerivedIdentityKEK()},
	} {
		parsed, err := Parse(file.Marshal())
		require.NoError(t, err)
		assert.True(t, parsed.Key.Equal(file.Key))
		assert.Equal(t, file.AdoptedJWTSecret, parsed.AdoptedJWTSecret)
		assert.Equal(t, file.AdoptedIdentityKEK, parsed.AdoptedIdentityKEK)
	}
	assert.Equal(t, "# AnixOps Control root key. Keep it secret; back it up with the database.\n"+
		"# Format: docs/reference/configuration.md, \"The root key\".\n"+
		"anixops-root-key: v1\n"+
		"key: AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=\n", string(File{Key: key}.Marshal()))
	// The derived values are the file's when nothing was adopted.
	assert.Equal(t, key.DerivedJWTSecret(), File{Key: key}.JWTSecret())
	assert.Equal(t, "adopted", File{Key: key, AdoptedJWTSecret: "adopted"}.JWTSecret())
}

func TestParseRefusesWhatItDoesNotKnow(t *testing.T) {
	good := "anixops-root-key: v1\nkey: AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=\n"
	for name, text := range map[string]string{
		"empty":           "",
		"comments only":   "# nothing\n\n",
		"no version":      "key: AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=\n",
		"newer version":   "anixops-root-key: v2\nkey: AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=\n",
		"no key":          "anixops-root-key: v1\n",
		"short key":       "anixops-root-key: v1\nkey: AAECAw==\n",
		"zero key":        "anixops-root-key: v1\nkey: " + base64.StdEncoding.EncodeToString(make([]byte, Size)) + "\n",
		"not base64":      "anixops-root-key: v1\nkey: not-base64!\n",
		"unknown field":   good + "color: blue\n",
		"repeated field":  good + "key: AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=\n",
		"not a field":     good + "just text\n",
		"bad adopted kek": good + "adopted-identity-kek: short\n",
	} {
		_, err := Parse([]byte(text))
		assert.Error(t, err, name)
		if err != nil {
			assert.NotContains(t, err.Error(), "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", name)
		}
	}
	file, err := Parse([]byte("# comment\n\n" + good))
	require.NoError(t, err)
	assert.Equal(t, vectorKey(t).Base64(), file.Key.Base64())
}

func TestParseKEKTakesBase64AndHex(t *testing.T) {
	key := vectorKey(t)
	for _, raw := range []string{key.Base64(), " " + key.Base64() + "\n", fmt.Sprintf("%x", key.Bytes())} {
		parsed, err := ParseKEK(raw)
		require.NoError(t, err)
		assert.True(t, parsed.Equal(key))
	}
	for _, raw := range []string{"", "short", base64.StdEncoding.EncodeToString(make([]byte, 31))} {
		_, err := ParseKEK(raw)
		assert.Error(t, err)
	}
}

func readMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}

func TestResolveGeneratesAPrivateKeyOnTheFirstStart(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	resolved, err := Resolve(Options{DataDir: dataDir, Write: true})
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, SourceGenerated, resolved.KeySource)
	assert.True(t, resolved.Stored)
	assert.False(t, resolved.Key.IsZero())
	assert.Equal(t, os.FileMode(0o600), readMode(t, resolved.Path))
	assert.Equal(t, os.FileMode(0o700), readMode(t, dataDir))
	assert.Equal(t, resolved.Key.DerivedJWTSecret(), resolved.JWTSecret)
	assert.Equal(t, SourceDerived, resolved.JWTSource)
	assert.Equal(t, resolved.Key.DerivedIdentityKEK(), resolved.IdentityKEK)
	assert.Equal(t, SourceDerived, resolved.IdentitySource)
	for _, note := range resolved.Notes {
		assert.NotContains(t, note, resolved.Key.Base64())
	}

	// The next start reads the same key and derives the same values.
	again, err := Resolve(Options{DataDir: dataDir, Write: true})
	require.NoError(t, err)
	assert.Equal(t, SourceFile, again.KeySource)
	assert.True(t, again.Key.Equal(resolved.Key))
	assert.Equal(t, resolved.JWTSecret, again.JWTSecret)
	assert.Equal(t, resolved.IdentityKEK, again.IdentityKEK)
	assert.Empty(t, again.Notes)
}

func TestResolveWithoutWriteNeverCreatesTheFile(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	resolved, err := Resolve(Options{DataDir: dataDir})
	require.NoError(t, err)
	assert.Nil(t, resolved, "without a key, a command has no root key")
	_, err = os.Stat(dataDir)
	assert.True(t, os.IsNotExist(err))

	// A legacy key is used in memory, and nothing is written.
	legacy := vectorKey(t)
	resolved, err = Resolve(Options{DataDir: dataDir, LegacyCAKEK: legacy.Base64()})
	require.NoError(t, err)
	assert.True(t, resolved.Key.Equal(legacy))
	assert.False(t, resolved.Stored)
	_, err = os.Stat(dataDir)
	assert.True(t, os.IsNotExist(err))
}

func TestResolveAdoptsTheLegacyKeysAndKeepsThemAfterTheConfigurationDropsThem(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	legacy := vectorKey(t)
	identity, err := Generate()
	require.NoError(t, err)
	hexIdentity := fmt.Sprintf("%x", identity.Bytes())
	resolved, err := Resolve(Options{DataDir: dataDir, Write: true,
		LegacyCAKEK: fmt.Sprintf("%x", legacy.Bytes()), LegacyJWTSecret: "the-old-jwt-secret-of-this-panel", LegacyIdentityKEK: hexIdentity})
	require.NoError(t, err)
	assert.Equal(t, SourceLegacyCAKEK, resolved.KeySource)
	assert.True(t, resolved.Stored)
	assert.True(t, resolved.Key.Equal(legacy), "the root key takes the value of module_runtime.ca_kek")
	assert.Equal(t, "the-old-jwt-secret-of-this-panel", resolved.JWTSecret)
	assert.Equal(t, SourceConfigJWT, resolved.JWTSource)
	assert.Equal(t, identity.Base64(), resolved.IdentityKEK, "a hex identity.kek is kept as the same 32 bytes")
	assert.Equal(t, SourceConfigIdentity, resolved.IdentitySource)

	file, err := ReadFile(resolved.Path)
	require.NoError(t, err)
	assert.True(t, file.Key.Equal(legacy))
	assert.Equal(t, "the-old-jwt-secret-of-this-panel", file.AdoptedJWTSecret)
	assert.Equal(t, identity.Base64(), file.AdoptedIdentityKEK)

	// After config rewrite the configuration sets none of them: the adopted
	// values stay in use.
	later, err := Resolve(Options{DataDir: dataDir, Write: true})
	require.NoError(t, err)
	assert.True(t, later.Key.Equal(legacy))
	assert.Equal(t, "the-old-jwt-secret-of-this-panel", later.JWTSecret)
	assert.Equal(t, SourceAdopted, later.JWTSource)
	assert.Equal(t, identity.Base64(), later.IdentityKEK)
	assert.Equal(t, SourceAdopted, later.IdentitySource)

	// The same legacy key next to root.key is accepted with a note.
	same, err := Resolve(Options{DataDir: dataDir, Write: true, LegacyCAKEK: legacy.Base64()})
	require.NoError(t, err)
	require.Len(t, same.Notes, 1)
	assert.Contains(t, same.Notes[0], "remove it from the configuration")
}

func TestResolveRecordsAJWTSecretSetAfterTheKeyExists(t *testing.T) {
	dataDir := t.TempDir()
	first, err := Resolve(Options{DataDir: dataDir, Write: true})
	require.NoError(t, err)
	second, err := Resolve(Options{DataDir: dataDir, Write: true, LegacyJWTSecret: "configured-later"})
	require.NoError(t, err)
	assert.Equal(t, "configured-later", second.JWTSecret)
	third, err := Resolve(Options{DataDir: dataDir, Write: true})
	require.NoError(t, err)
	assert.Equal(t, "configured-later", third.JWTSecret)
	assert.Equal(t, SourceAdopted, third.JWTSource)
	assert.True(t, third.Key.Equal(first.Key))
}

func TestResolveRefusesALegacyKeyThatDiffersFromRootKey(t *testing.T) {
	dataDir := t.TempDir()
	_, err := Resolve(Options{DataDir: dataDir, Write: true})
	require.NoError(t, err)
	other := vectorKey(t)
	_, err = Resolve(Options{DataDir: dataDir, Write: true, LegacyCAKEK: other.Base64()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hold different keys")
	assert.NotContains(t, err.Error(), other.Base64())
}

func TestResolveRefusesInvalidLegacyValues(t *testing.T) {
	_, err := Resolve(Options{DataDir: t.TempDir(), Write: true, LegacyCAKEK: "short"})
	require.ErrorContains(t, err, "module_runtime.ca_kek")
	_, err = Resolve(Options{DataDir: t.TempDir(), Write: true, LegacyIdentityKEK: "short"})
	require.ErrorContains(t, err, "identity.kek")
	_, err = Resolve(Options{Write: true})
	require.ErrorContains(t, err, "data_dir")
}

func TestResolveRefusesADamagedFile(t *testing.T) {
	dataDir := t.TempDir()
	path := Path(dataDir)
	require.NoError(t, os.WriteFile(path, []byte("anixops-root-key: v1\nkey: AAEC\n"), 0o600))
	_, err := Resolve(Options{DataDir: dataDir, Write: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Restore root.key from the backup")
}

func TestResolveRestrictsALooseFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes")
	}
	dataDir := t.TempDir()
	first, err := Resolve(Options{DataDir: dataDir, Write: true})
	require.NoError(t, err)
	require.NoError(t, os.Chmod(first.Path, 0o644))

	// A command only warns.
	resolved, err := Resolve(Options{DataDir: dataDir})
	require.NoError(t, err)
	require.Len(t, resolved.Notes, 1)
	assert.Contains(t, resolved.Notes[0], "readable by other users")
	assert.Equal(t, os.FileMode(0o644), readMode(t, first.Path))

	// The server restricts it.
	resolved, err = Resolve(Options{DataDir: dataDir, Write: true})
	require.NoError(t, err)
	require.Len(t, resolved.Notes, 1)
	assert.Contains(t, resolved.Notes[0], "it is 0600 now")
	assert.Equal(t, os.FileMode(0o600), readMode(t, first.Path))
}

func TestResolveRestrictsAnOpenDataDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes")
	}
	dataDir := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.Mkdir(dataDir, 0o755))
	require.NoError(t, os.Chmod(dataDir, 0o755))
	_, err := Resolve(Options{DataDir: dataDir, Write: true})
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), readMode(t, dataDir))
}

func TestResolveInAnUnwritableDirectory(t *testing.T) {
	// A data directory below a regular file cannot be created, even by
	// root.
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(parent, nil, 0o600))
	dataDir := filepath.Join(parent, "data")

	// No legacy key: Control cannot start without a root key.
	_, err := Resolve(Options{DataDir: dataDir, Write: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be created")
	assert.Contains(t, err.Error(), "ANIX_CONTROL_DATA_DIR")

	// A legacy key keeps working from the configuration, with a warning.
	legacy := vectorKey(t)
	resolved, err := Resolve(Options{DataDir: dataDir, Write: true, LegacyCAKEK: legacy.Base64()})
	require.NoError(t, err)
	assert.True(t, resolved.Key.Equal(legacy))
	assert.False(t, resolved.Stored)
	require.NotEmpty(t, resolved.Notes)
	assert.True(t, strings.HasPrefix(resolved.Notes[0], "WARNING: the root key cannot be written"))
}

func TestCreateExclusiveKeepsTheFirstKey(t *testing.T) {
	dataDir := t.TempDir()
	first, err := Generate()
	require.NoError(t, err)
	second, err := Generate()
	require.NoError(t, err)
	existing, err := createExclusive(dataDir, Path(dataDir), File{Key: first})
	require.NoError(t, err)
	assert.Nil(t, existing)
	existing, err = createExclusive(dataDir, Path(dataDir), File{Key: second})
	require.NoError(t, err)
	require.NotNil(t, existing)
	assert.True(t, existing.Key.Equal(first), "a second writer never replaces the key")
	entries, err := os.ReadDir(dataDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left behind")
}
