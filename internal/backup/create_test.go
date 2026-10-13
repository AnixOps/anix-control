package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteBundleContentsAndManifest(t *testing.T) {
	p := newPanel(t)
	at := time.Date(2026, 10, 13, 3, 41, 7, 0, time.UTC)
	bundle := p.write(t, TypeDaily, at)

	assert.Equal(t, "anix-control-20261013T034107Z-daily.tar", bundle.Name)
	assert.Equal(t, filepath.Join(p.dataDir, "backups", bundle.Name), bundle.Path)
	info, err := os.Stat(bundle.Path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assert.Equal(t, info.Size(), bundle.Size)
	dirInfo, err := os.Stat(filepath.Join(p.dataDir, "backups"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	whole, err := os.ReadFile(bundle.Path)
	require.NoError(t, err)
	sum := sha256.Sum256(whole)
	assert.Equal(t, hex.EncodeToString(sum[:]), bundle.SHA256)

	members := readMembers(t, bundle.Path)
	names := make([]string, 0, len(members))
	for _, member := range members {
		names = append(names, member.header.Name)
		assert.Equal(t, int64(0o600), member.header.Mode, member.header.Name)
	}
	assert.Equal(t, []string{MemberManifest, MemberDatabase, MemberRootKey, MemberConfig}, names)

	var manifest Manifest
	require.NoError(t, json.Unmarshal(members[0].data, &manifest))
	steps, err := migrations.Steps()
	require.NoError(t, err)
	assert.Equal(t, ManifestFormat, manifest.Format)
	assert.Equal(t, testVersion, manifest.ProgramVersion)
	assert.Equal(t, len(steps), manifest.SchemaVersion)
	assert.True(t, manifest.CreatedAt.Equal(at))
	assert.Equal(t, TypeDaily, manifest.Type)
	assert.Equal(t, "panel-host", manifest.HostName)
	assert.Equal(t, "panel.example.com", manifest.Domain)
	assert.Equal(t, p.key.Fingerprint(), manifest.RootKeyFingerprint)
	assert.NotContains(t, string(members[0].data), p.key.Base64(), "the manifest never holds the key")
	assert.Equal(t, map[string]int64{"v2_user": 2, "v2_node": 0, "v2_order": 0}, manifest.RowCounts)
	require.Len(t, manifest.Members, 3)
	for i, member := range manifest.Members {
		data := members[i+1].data
		digest := sha256.Sum256(data)
		assert.Equal(t, members[i+1].header.Name, member.Name)
		assert.Equal(t, int64(len(data)), member.Size)
		assert.Equal(t, hex.EncodeToString(digest[:]), member.SHA256)
	}
	assert.Equal(t, p.keyFile, members[2].data)
	assert.Equal(t, p.config, members[3].data)
	assert.Equal(t, manifest, bundle.Manifest)

	// The scratch directory is empty again.
	assert.Empty(t, ls(t, filepath.Join(p.dataDir, "tmp")))
}

func TestWriteWithoutAConfigurationFile(t *testing.T) {
	p := newPanel(t)
	source := p.source()
	source.Config = nil
	bundle, err := Write(context.Background(), source, TypeManual, time.Now())
	require.NoError(t, err)
	members := readMembers(t, bundle.Path)
	require.Len(t, members, 3)
	assert.Equal(t, MemberRootKey, members[2].header.Name)
	_, listed := bundle.Manifest.member(MemberConfig)
	assert.False(t, listed)
}

// Two bundles of one type within a second get consecutive names; neither
// replaces the other.
func TestWriteNeverReplacesABundle(t *testing.T) {
	p := newPanel(t)
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	first := p.write(t, TypeManual, at)
	second := p.write(t, TypeManual, at)
	assert.Equal(t, "anix-control-20261013T120000Z-manual.tar", first.Name)
	assert.Equal(t, "anix-control-20261013T120001Z-manual.tar", second.Name)
	assert.Len(t, ls(t, filepath.Join(p.dataDir, "backups")), 2)
}

// A failure anywhere leaves neither a partial bundle nor a snapshot.
func TestWriteIsAtomic(t *testing.T) {
	for _, stage := range []string{"snapshot", "member:" + MemberDatabase, "member:" + MemberConfig, "publish"} {
		t.Run(stage, func(t *testing.T) {
			p := newPanel(t)
			injected := errors.New("injected failure at " + stage)
			failpoint = func(at string) error {
				if at == stage {
					return injected
				}
				return nil
			}
			t.Cleanup(func() { failpoint = nil })
			_, err := Write(context.Background(), p.source(), TypeDaily, time.Now())
			require.ErrorIs(t, err, injected)
			assert.Empty(t, ls(t, filepath.Join(p.dataDir, "backups")), "no bundle, no partial file")
			assert.Empty(t, ls(t, filepath.Join(p.dataDir, "tmp")), "no snapshot left")
		})
	}
}

func TestWriteRefusesWhatIsNotABackup(t *testing.T) {
	p := newPanel(t)
	_, err := Write(context.Background(), p.source(), "weekly", time.Now())
	require.ErrorContains(t, err, `unknown backup type "weekly"`)
	source := p.source()
	source.RootKey = []byte("not a root key")
	_, err = Write(context.Background(), source, TypeDaily, time.Now())
	require.ErrorContains(t, err, "root.key")
	source = p.source()
	source.DatabasePath = ":memory:"
	_, err = Write(context.Background(), source, TypeDaily, time.Now())
	require.ErrorContains(t, err, "in-memory")
	assert.Empty(t, ls(t, filepath.Join(p.dataDir, "backups")))
}

func TestFileNamesRoundTrip(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, typ := range Types {
		name := FileName(at, typ)
		parsed, parsedType, ok := ParseFileName(name)
		require.True(t, ok, name)
		assert.True(t, parsed.Equal(at))
		assert.Equal(t, typ, parsedType)
	}
	for _, other := range []string{".partial-1.tar", "anix-control-20260102T030405Z-weekly.tar", "anix-control-20260102T030405Z-daily.tar.gz", "notes.txt"} {
		_, _, ok := ParseFileName(other)
		assert.False(t, ok, other)
	}
}
