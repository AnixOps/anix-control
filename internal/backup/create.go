package backup

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/rootkey"
)

// Source is what a bundle copies.
type Source struct {
	// DataDir is the data directory: the bundle goes to DataDir/backups,
	// the snapshot is taken in DataDir/tmp.
	DataDir string
	// DatabasePath is the SQLite database file.
	DatabasePath string
	// RootKey is the content of root.key.
	RootKey []byte
	// Config is the configuration file Control loaded; nil when it loaded
	// none.
	Config []byte
	// Domain is the panel's domain, recorded in the manifest.
	Domain string
	// ProgramVersion is the running Control's version.
	ProgramVersion string
	// HostName names the machine in the manifest.
	HostName string
}

// Bundle is a bundle on disk.
type Bundle struct {
	// Path is the bundle's absolute path, Name its file name.
	Path string
	Name string
	Size int64
	// SHA256 is the hex digest of the whole file.
	SHA256   string
	Manifest Manifest
}

// failpoint, when set (tests only), is called at the named stages of
// Write; an error it answers fails the write there.
var failpoint func(stage string) error

func fail(stage string) error {
	if failpoint == nil {
		return nil
	}
	return failpoint(stage)
}

// freeSpaceFactor: Write refuses to start unless the file system of the
// data directory has this many times the database's size free (the
// snapshot and the bundle each need about one size, plus room to spare).
const freeSpaceFactor = 3

// Write takes a bundle of type t from source at now and answers it. It is
// atomic: the bundle appears in DataDir/backups under its final name only
// once complete and synced; on any error nothing is left behind, neither a
// partial bundle nor the snapshot.
func Write(ctx context.Context, source Source, t string, now time.Time) (*Bundle, error) {
	if !ValidType(t) {
		return nil, fmt.Errorf("unknown backup type %q (one of %s)", t, strings.Join(Types, ", "))
	}
	if strings.TrimSpace(source.DataDir) == "" {
		return nil, errors.New("data_dir is empty: backups are kept in data_dir/backups")
	}
	key, err := rootkey.Parse(source.RootKey)
	if err != nil {
		return nil, fmt.Errorf("root.key: %w", err)
	}
	dir, scratch := Dir(source.DataDir), ScratchDir(source.DataDir)
	for _, d := range []string{source.DataDir, dir, scratch} {
		if err := ensureDir(d); err != nil {
			return nil, fmt.Errorf("prepare %s: %w", d, err)
		}
	}
	if err := checkFreeSpace(dir, source.DatabasePath); err != nil {
		return nil, err
	}

	work, err := os.MkdirTemp(scratch, "backup-")
	if err != nil {
		return nil, fmt.Errorf("prepare the snapshot directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }()
	snapshot := filepath.Join(work, MemberDatabase)
	if err := Snapshot(ctx, source.DatabasePath, snapshot); err != nil {
		return nil, err
	}
	if err := fail("snapshot"); err != nil {
		return nil, err
	}
	facts, err := readSnapshotFacts(ctx, snapshot)
	if err != nil {
		return nil, fmt.Errorf("read the snapshot: %w", err)
	}

	manifest := Manifest{
		Format: ManifestFormat, ProgramVersion: source.ProgramVersion, SchemaVersion: facts.SchemaVersion,
		CreatedAt: now.UTC().Truncate(time.Second), Type: t, HostName: source.HostName, Domain: source.Domain,
		RootKeyFingerprint: key.Key.Fingerprint(), RowCounts: facts.RowCounts,
	}
	databaseMember, err := fileMember(MemberDatabase, snapshot)
	if err != nil {
		return nil, err
	}
	manifest.Members = append(manifest.Members, databaseMember, bytesMember(MemberRootKey, source.RootKey))
	if source.Config != nil {
		manifest.Members = append(manifest.Members, bytesMember(MemberConfig, source.Config))
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	manifestJSON = append(manifestJSON, '\n')

	partial, err := os.CreateTemp(dir, ".partial-*.tar")
	if err != nil {
		return nil, fmt.Errorf("create the bundle: %w", err)
	}
	partialPath := partial.Name()
	published := false
	defer func() {
		if !published {
			_ = partial.Close()
			_ = os.Remove(partialPath)
		}
	}()
	if err := partial.Chmod(fileMode); err != nil {
		return nil, err
	}
	digest := sha256.New()
	writer := tar.NewWriter(io.MultiWriter(partial, digest))
	add := func(name string, size int64, content io.Reader, want string) error {
		if err := writer.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg, Name: name, Size: size, Mode: int64(fileMode), ModTime: manifest.CreatedAt, Format: tar.FormatPAX,
		}); err != nil {
			return err
		}
		sum := sha256.New()
		written, err := io.Copy(writer, io.TeeReader(content, sum))
		if err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		if err := fail("member:" + name); err != nil {
			return err
		}
		if written != size || (want != "" && hex.EncodeToString(sum.Sum(nil)) != want) {
			return fmt.Errorf("%s changed while it was written to the bundle", name)
		}
		return nil
	}
	if err := add(MemberManifest, int64(len(manifestJSON)), strings.NewReader(string(manifestJSON)), ""); err != nil {
		return nil, err
	}
	snapshotFile, err := os.Open(snapshot) // #nosec G304 -- the snapshot Write took.
	if err != nil {
		return nil, err
	}
	err = add(MemberDatabase, databaseMember.Size, snapshotFile, databaseMember.SHA256)
	_ = snapshotFile.Close()
	if err != nil {
		return nil, err
	}
	if err := add(MemberRootKey, int64(len(source.RootKey)), strings.NewReader(string(source.RootKey)), ""); err != nil {
		return nil, err
	}
	if source.Config != nil {
		if err := add(MemberConfig, int64(len(source.Config)), strings.NewReader(string(source.Config)), ""); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish the bundle: %w", err)
	}
	if err := partial.Sync(); err != nil {
		return nil, fmt.Errorf("sync the bundle: %w", err)
	}
	info, err := partial.Stat()
	if err != nil {
		return nil, err
	}
	if err := partial.Close(); err != nil {
		return nil, err
	}
	if err := fail("publish"); err != nil {
		return nil, err
	}
	// Run as root (sudo anix-control backup create), the bundle gets the
	// owner of the backup directory, so the service can verify and remove
	// it.
	if err := ownTo(partialPath); err != nil {
		return nil, err
	}
	name, err := publish(partialPath, dir, manifest.CreatedAt, t)
	if err != nil {
		return nil, err
	}
	published = true
	return &Bundle{
		Path: filepath.Join(dir, name), Name: name, Size: info.Size(),
		SHA256: hex.EncodeToString(digest.Sum(nil)), Manifest: manifest,
	}, nil
}

// publish gives the complete bundle at partialPath its final name in dir,
// atomically and without replacing a bundle of the same name: two bundles
// of one type within one second get consecutive seconds. The bundle's
// manifest keeps its own time.
func publish(partialPath, dir string, at time.Time, t string) (string, error) {
	for attempt := 0; attempt < 60; attempt++ {
		name := FileName(at.Add(time.Duration(attempt)*time.Second), t)
		err := os.Link(partialPath, filepath.Join(dir, name))
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("publish the bundle: %w", err)
		}
		if err := os.Remove(partialPath); err != nil {
			return "", err
		}
		if err := syncDir(dir); err != nil {
			return "", fmt.Errorf("sync %s: %w", dir, err)
		}
		return name, nil
	}
	return "", errors.New("publish the bundle: no free name within a minute")
}

func bytesMember(name string, content []byte) Member {
	sum := sha256.Sum256(content)
	return Member{Name: name, Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:])}
}

func fileMember(name, path string) (Member, error) {
	file, err := os.Open(path) // #nosec G304 -- a file of the bundle being written.
	if err != nil {
		return Member{}, err
	}
	defer func() { _ = file.Close() }()
	size, sum, err := digestOf(file)
	if err != nil {
		return Member{}, fmt.Errorf("read %s: %w", name, err)
	}
	return Member{Name: name, Size: size, SHA256: sum}, nil
}

func digestOf(reader io.Reader) (int64, string, error) {
	sum := sha256.New()
	size, err := io.Copy(sum, reader)
	if err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(sum.Sum(nil)), nil
}

// checkFreeSpace refuses a backup when the file system of dir has less
// than freeSpaceFactor times the database's size (file and log) free.
func checkFreeSpace(dir, databasePath string) error {
	var size int64
	for _, suffix := range []string{"", "-wal"} {
		if info, err := os.Stat(sqliteFile(databasePath) + suffix); err == nil {
			size += info.Size()
		}
	}
	free, ok := freeBytes(dir)
	if !ok || size == 0 {
		return nil
	}
	if need := uint64(size) * freeSpaceFactor; free < need { // #nosec G115 -- a file size is never negative.
		return fmt.Errorf("not enough free space for a backup in %s: %d MiB free, %d MiB needed (%d times the database's %d MiB). Free some space or remove old bundles",
			dir, free>>20, need>>20, freeSpaceFactor, size>>20)
	}
	return nil
}
