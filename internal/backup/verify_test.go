package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func verifyOptions(p *panel) VerifyOptions {
	return VerifyOptions{ScratchDir: filepath.Join(p.dataDir, "tmp"), ProgramVersion: testVersion}
}

func TestVerifyPassesAGoodBundle(t *testing.T) {
	p := newPanel(t)
	bundle := p.write(t, TypeDaily, time.Now())
	verified, err := VerifyFile(context.Background(), bundle.Path, verifyOptions(p))
	require.NoError(t, err)
	assert.Equal(t, bundle.Manifest, verified.Manifest)
	assert.Empty(t, verified.Dir)
	assert.Empty(t, ls(t, filepath.Join(p.dataDir, "tmp")), "the extracted copy is removed")
}

// flipPages overwrites the start of every eighth page after the first two:
// b-tree page headers PRAGMA integrity_check reads.
func flipPages(_ string, data []byte) []byte {
	out := append([]byte(nil), data...)
	const page = 4096
	for offset := 2 * page; offset+16 < len(out); offset += 8 * page {
		for i := 0; i < 16; i++ {
			out[offset+i] ^= 0xA5
		}
	}
	return out
}

func otherKeyFile(t *testing.T) ([]byte, rootkey.Key) {
	t.Helper()
	key, err := rootkey.Generate()
	require.NoError(t, err)
	return rootkey.File{Key: key}.Marshal(), key
}

// Every kind of damage is found and named.
func TestVerifyFindsEachProblem(t *testing.T) {
	otherFile, otherKey := otherKeyFile(t)
	cases := []struct {
		name     string
		damage   func(t *testing.T, path string)
		kind     string
		contains string
	}{
		{"truncated archive", func(t *testing.T, path string) {
			info, err := os.Stat(path)
			require.NoError(t, err)
			require.NoError(t, os.Truncate(path, info.Size()/2))
		}, ProblemArchive, "read database.sqlite"},
		{"not an archive", func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte("this is not a tar archive, just some text that is long enough to fill a block"), 0o600))
		}, ProblemArchive, "not a readable tar archive"},
		{"empty file", func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, nil, 0o600))
		}, ProblemManifest, "has no manifest.json"},
		{"bad checksum", func(t *testing.T, path string) {
			tamper(t, path, func(name string, data []byte) []byte {
				if name == MemberConfig {
					return []byte("domain: x.example.com\n")
				}
				return data
			}, false, nil)
		}, ProblemChecksum, "config.yaml is 22 bytes, the manifest says 26"},
		{"same size, other bytes", func(t *testing.T, path string) {
			tamper(t, path, func(name string, data []byte) []byte {
				if name == MemberConfig {
					return []byte("domain: panel.example.org\n")
				}
				return data
			}, false, nil)
		}, ProblemChecksum, "config.yaml has SHA-256"},
		{"integrity_check fails", func(t *testing.T, path string) {
			tamper(t, path, func(name string, data []byte) []byte {
				if name == MemberDatabase {
					return flipPages(name, data)
				}
				return data
			}, true, nil)
		}, ProblemIntegrity, ""},
		{"another root key", func(t *testing.T, path string) {
			tamper(t, path, func(name string, data []byte) []byte {
				if name == MemberRootKey {
					return otherFile
				}
				return data
			}, true, nil)
		}, ProblemRootKey, "fingerprint"},
		{"root key and database not a pair", func(t *testing.T, path string) {
			tamper(t, path, func(name string, data []byte) []byte {
				if name == MemberRootKey {
					return otherFile
				}
				return data
			}, true, func(m *Manifest) { m.RootKeyFingerprint = otherKey.Fingerprint() })
		}, ProblemRootKey, "does not open the module CA key"},
		{"missing member", func(t *testing.T, path string) {
			tamper(t, path, func(name string, data []byte) []byte {
				if name == MemberRootKey {
					return nil
				}
				return data
			}, false, nil)
		}, ProblemMember, "root.key is missing"},
		{"member the manifest does not list", func(t *testing.T, path string) {
			members := readMembers(t, path)
			extra := members[len(members)-1]
			extra.header.Name, extra.data = "notes.txt", []byte("hello")
			writeMembers(t, path, append(members, extra))
		}, ProblemMember, `"notes.txt" is in the bundle but not in the manifest`},
		{"manifest of another format", func(t *testing.T, path string) {
			tamper(t, path, unchanged, false, func(m *Manifest) { m.Format = "anixops.backup/v9" })
		}, ProblemManifest, "anixops.backup/v9"},
		{"schema version not the manifest's", func(t *testing.T, path string) {
			tamper(t, path, unchanged, false, func(m *Manifest) { m.SchemaVersion++ })
		}, ProblemMigrations, "the manifest says"},
		{"row counts not the manifest's", func(t *testing.T, path string) {
			tamper(t, path, unchanged, false, func(m *Manifest) { m.RowCounts["v2_user"] = 3 })
		}, ProblemRowCounts, "v2_user has 2 rows, the manifest says 3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newPanel(t)
			bundle := p.write(t, TypeDaily, time.Now())
			tc.damage(t, bundle.Path)
			_, err := VerifyFile(context.Background(), bundle.Path, verifyOptions(p))
			var failed *VerifyError
			require.True(t, errors.As(err, &failed), "want a VerifyError, got %v", err)
			assert.Equal(t, tc.kind, failed.Kind, failed.Error())
			assert.Contains(t, failed.Error(), tc.contains)
			assert.NotContains(t, failed.Error(), p.key.Base64(), "no key material in the error")
			assert.Empty(t, ls(t, filepath.Join(p.dataDir, "tmp")), "the extracted copy is removed")
		})
	}
}

// A bundle whose database a newer Control migrated fails with the start's
// own message: this Control could not run on it.
func TestVerifyRefusesADatabaseANewerControlMigrated(t *testing.T) {
	p := newPanel(t)
	steps, err := migrations.Steps()
	require.NoError(t, err)
	require.NoError(t, p.db.Create(&migrations.MigrationRecord{
		Version: len(steps) + 1, Name: "from_the_future", Checksum: "sha256:00", AppliedAt: time.Now(), ProgramVersion: "5.1.0",
	}).Error)
	bundle := p.write(t, TypeManual, time.Now())
	assert.Equal(t, len(steps)+1, bundle.Manifest.SchemaVersion)

	_, err = VerifyFile(context.Background(), bundle.Path, verifyOptions(p))
	var failed *VerifyError
	require.True(t, errors.As(err, &failed), "%v", err)
	assert.Equal(t, ProblemMigrations, failed.Kind)
	var newer *migrations.RefusedError
	require.True(t, errors.As(err, &newer))
	assert.Contains(t, err.Error(), "this database was migrated by a newer AnixOps Control")
	assert.Contains(t, err.Error(), "Control 5.1.0")
}

func TestVerifyKeepsTheExtractedMembersOnRequest(t *testing.T) {
	p := newPanel(t)
	bundle := p.write(t, TypeDaily, time.Now())
	options := verifyOptions(p)
	options.Keep = true
	verified, err := VerifyFile(context.Background(), bundle.Path, options)
	require.NoError(t, err)
	require.NotEmpty(t, verified.Dir)
	assert.ElementsMatch(t, []string{MemberDatabase, MemberRootKey, MemberConfig}, ls(t, verified.Dir))
	key, err := os.ReadFile(filepath.Join(verified.Dir, MemberRootKey))
	require.NoError(t, err)
	assert.Equal(t, p.keyFile, key)
}
