package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const testVersion = "4.2.1"

// panel is a data directory with a migrated SQLite database, a module CA
// sealed with its root key, a configuration file and a few rows: what a
// running Control has.
type panel struct {
	dataDir    string
	dbPath     string
	configPath string
	key        rootkey.Key
	keyFile    []byte
	config     []byte
	db         *gorm.DB
}

func newPanel(t *testing.T) *panel {
	t.Helper()
	root := t.TempDir()
	p := &panel{dataDir: filepath.Join(root, "data"), dbPath: filepath.Join(root, "db", "control.db"), configPath: filepath.Join(root, "etc", "config.yaml")}
	require.NoError(t, os.MkdirAll(p.dataDir, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Dir(p.configPath), 0o700))
	key, err := rootkey.Generate()
	require.NoError(t, err)
	p.key = key
	p.keyFile = rootkey.File{Key: key}.Marshal()
	require.NoError(t, os.WriteFile(rootkey.Path(p.dataDir), p.keyFile, 0o600))
	p.config = []byte("domain: panel.example.com\n")
	require.NoError(t, os.WriteFile(p.configPath, p.config, 0o600))

	p.db = openPanelDB(t, p.dbPath)
	ctx := context.Background()
	_, err = migrations.Migrate(ctx, p.db, migrations.Options{ProgramVersion: testVersion})
	require.NoError(t, err)
	authority, err := modulepki.New(modulepki.Options{DB: p.db, Cluster: "default", KEK: key.Bytes()})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(ctx))
	require.NoError(t, p.db.Exec("INSERT INTO v2_user (email, password, uuid, token, created_at, updated_at) VALUES ('a@example.com', 'x', 'u-1', 't-1', datetime('now'), datetime('now')), ('b@example.com', 'x', 'u-2', 't-2', datetime('now'), datetime('now'))").Error)
	return p
}

// openPanelDB opens path the way Control does: one writer, a read pool.
func openPanelDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := database.Open(&config.DatabaseConfig{Driver: "sqlite", Database: path, LogLevel: "silent"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.CloseHandle(db) })
	return db
}

func (p *panel) source() Source {
	return Source{
		DataDir: p.dataDir, DatabasePath: p.dbPath, RootKey: p.keyFile, Config: p.config,
		Domain: "panel.example.com", ProgramVersion: testVersion, HostName: "panel-host",
	}
}

func (p *panel) manager() *Manager {
	return &Manager{
		DataDir: p.dataDir, Driver: "sqlite", DatabasePath: p.dbPath, ConfigPath: p.configPath,
		RootKey: func() ([]byte, error) { return os.ReadFile(rootkey.Path(p.dataDir)) },
		Domain:  "panel.example.com", ProgramVersion: testVersion, DB: func() *gorm.DB { return p.db },
		HostName: func() (string, error) { return "panel-host", nil },
	}
}

func (p *panel) write(t *testing.T, typ string, at time.Time) *Bundle {
	t.Helper()
	bundle, err := Write(context.Background(), p.source(), typ, at)
	require.NoError(t, err)
	return bundle
}

// tarMember is a member read back from a bundle.
type tarMember struct {
	header tar.Header
	data   []byte
}

func readMembers(t *testing.T, path string) []tarMember {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()
	reader := tar.NewReader(file)
	var members []tarMember
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		members = append(members, tarMember{header: *header, data: data})
	}
	return members
}

func writeMembers(t *testing.T, path string, members []tarMember) {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, member := range members {
		header := member.header
		header.Size = int64(len(member.data))
		require.NoError(t, writer.WriteHeader(&header))
		_, err := writer.Write(member.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(path, buffer.Bytes(), 0o600))
}

// tamper rewrites the bundle at path: change edits a member's data (nil
// drops it), and with fixManifest the manifest's size and digest follow the
// edit, so only the deeper checks can find it.
func tamper(t *testing.T, path string, change func(name string, data []byte) []byte, fixManifest bool, editManifest func(*Manifest)) {
	t.Helper()
	members := readMembers(t, path)
	var manifest Manifest
	require.NoError(t, json.Unmarshal(members[0].data, &manifest))
	var kept []tarMember
	for _, member := range members[1:] {
		data := change(member.header.Name, member.data)
		if data == nil {
			continue
		}
		member.data = data
		kept = append(kept, member)
		if fixManifest {
			for i := range manifest.Members {
				if manifest.Members[i].Name == member.header.Name {
					sum := sha256.Sum256(data)
					manifest.Members[i].Size, manifest.Members[i].SHA256 = int64(len(data)), hex.EncodeToString(sum[:])
				}
			}
		}
	}
	if editManifest != nil {
		editManifest(&manifest)
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	members[0].data = encoded
	writeMembers(t, path, append([]tarMember{members[0]}, kept...))
}

func unchanged(_ string, data []byte) []byte { return data }

func ls(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func backupRows(t *testing.T, db *gorm.DB) []model.Backup {
	t.Helper()
	var rows []model.Backup
	require.NoError(t, db.Order("created_at DESC").Find(&rows).Error)
	return rows
}
