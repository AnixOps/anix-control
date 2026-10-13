// Package builtinpkg holds the official packages embedded in the Control
// binary. The build writes them into dist/ before it compiles Control:
//
//	python3 packages/shared/build_package.py --all --builtin \
//	  --version <version> --out internal/builtinpkg/dist
//
// For every package that is the slim signed artifact (compatibility routes,
// migration index and scripts, configuration schema, WebUI bundle; no host
// binary), its canonical manifest, whose control_entrypoint names the host
// linked into the binary (packages/builtin), and the manifest's signature.
// index.json lists them. This package only reads them: the kernel verifies
// every signature against the configured official key and bootstraps the
// packages at every start (internal/service, BootstrapBuiltinPackages).
//
// A binary built without index.json (go build ./... from a fresh checkout)
// embeds no package; Control then starts without installing any.
package builtinpkg

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
)

//go:embed dist
var embedded embed.FS

const (
	// IndexFormat is the format of dist/index.json.
	IndexFormat = "anixops.builtin-packages/v1"
	// IndexName is the index file in the embedded directory.
	IndexName = "index.json"

	maxIndexBytes     = 1 << 20
	maxManifestBytes  = 1 << 20
	maxSignatureBytes = 16 << 10
	// MaxArtifactBytes matches the kernel's artifact limit.
	MaxArtifactBytes  = 64 << 20
	maxPublicKeyBytes = 16 << 10
)

// Package is one embedded official package.
type Package struct {
	ID      string
	Version string
	// Manifest is the canonical signed manifest.
	Manifest []byte
	// Signature is the manifest signature (Base64 Ed25519).
	Signature string
	// Artifact is the slim package archive the manifest's artifact_sha256
	// names.
	Artifact []byte
}

// Set is the packages a binary embeds.
type Set struct {
	// Packages is sorted by id.
	Packages []Package
	// PublicKeyPEM is the key the build signed with. The kernel never trusts
	// it: it verifies with plugins.official_public_key and uses this key
	// only to say which key signed packages it rejects.
	PublicKeyPEM []byte
}

// Lookup returns the package of an id.
func (s *Set) Lookup(id string) (Package, bool) {
	if s == nil {
		return Package{}, false
	}
	for _, candidate := range s.Packages {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return Package{}, false
}

// Empty reports a set without packages.
func (s *Set) Empty() bool { return s == nil || len(s.Packages) == 0 }

type indexEntry struct {
	ID        string `json:"id"`
	Version   string `json:"version"`
	Artifact  string `json:"artifact"`
	Manifest  string `json:"manifest"`
	Signature string `json:"signature"`
}

type indexDocument struct {
	Format    string       `json:"format"`
	PublicKey string       `json:"public_key"`
	Packages  []indexEntry `json:"packages"`
}

// Load reads a package set from a directory holding index.json and the
// files it names. A directory without index.json is an empty set.
func Load(fsys fs.FS) (*Set, error) {
	raw, err := readLimited(fsys, IndexName, maxIndexBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return &Set{}, nil
	}
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var index indexDocument
	if err := decoder.Decode(&index); err != nil {
		return nil, fmt.Errorf("built-in package index: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("built-in package index must hold one JSON value")
	}
	if index.Format != IndexFormat {
		return nil, fmt.Errorf("built-in package index format %q is not %s", index.Format, IndexFormat)
	}
	set := &Set{}
	if index.PublicKey != "" {
		if !safeFileName(index.PublicKey) {
			return nil, fmt.Errorf("built-in package index names an unsafe public key file %q", index.PublicKey)
		}
		if set.PublicKeyPEM, err = readLimited(fsys, index.PublicKey, maxPublicKeyBytes); err != nil {
			return nil, err
		}
	}
	seen := map[string]bool{}
	for _, entry := range index.Packages {
		if !safeSegment(entry.ID) || !safeSegment(entry.Version) {
			return nil, fmt.Errorf("built-in package index has an invalid entry %q %q", entry.ID, entry.Version)
		}
		if seen[entry.ID] {
			return nil, fmt.Errorf("built-in package index lists %s twice", entry.ID)
		}
		seen[entry.ID] = true
		for _, name := range []string{entry.Artifact, entry.Manifest, entry.Signature} {
			if !safeFileName(name) {
				return nil, fmt.Errorf("built-in package %s names an unsafe file %q", entry.ID, name)
			}
		}
		manifest, err := readLimited(fsys, entry.Manifest, maxManifestBytes)
		if err != nil {
			return nil, err
		}
		signature, err := readLimited(fsys, entry.Signature, maxSignatureBytes)
		if err != nil {
			return nil, err
		}
		artifact, err := readLimited(fsys, entry.Artifact, MaxArtifactBytes)
		if err != nil {
			return nil, err
		}
		set.Packages = append(set.Packages, Package{
			ID: entry.ID, Version: entry.Version, Manifest: manifest,
			Signature: strings.TrimSpace(string(signature)), Artifact: artifact,
		})
	}
	sort.Slice(set.Packages, func(i, j int) bool { return set.Packages[i].ID < set.Packages[j].ID })
	return set, nil
}

func readLimited(fsys fs.FS, name string, maximum int64) ([]byte, error) {
	file, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("built-in package file %s: %w", name, err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("built-in package file %s is not a regular file", name)
	}
	contents, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, fmt.Errorf("read built-in package file %s: %w", name, err)
	}
	if int64(len(contents)) > maximum {
		return nil, fmt.Errorf("built-in package file %s exceeds %d bytes", name, maximum)
	}
	if len(contents) == 0 {
		return nil, fmt.Errorf("built-in package file %s is empty", name)
	}
	return contents, nil
}

func safeFileName(name string) bool {
	return name != "" && name == path.Base(name) && name != "." && name != ".." && !strings.ContainsAny(name, "/\\") && safeSegment(name)
}

func safeSegment(value string) bool {
	if value == "" || len(value) > 160 || value == "." || value == ".." {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9':
		case strings.ContainsRune("._+-", character):
		default:
			return false
		}
	}
	return true
}

var (
	embeddedOnce sync.Once
	embeddedSet  *Set
	embeddedErr  error

	defaultMu       sync.RWMutex
	defaultOverride *Set
)

// Embedded returns the set compiled into this binary.
func Embedded() (*Set, error) {
	embeddedOnce.Do(func() {
		dist, err := fs.Sub(embedded, "dist")
		if err != nil {
			embeddedErr = err
			return
		}
		embeddedSet, embeddedErr = Load(dist)
	})
	return embeddedSet, embeddedErr
}

// Default returns the set the kernel bootstraps and admits built-in hosts
// for: the embedded one, unless a test installed another with SetDefault.
func Default() (*Set, error) {
	defaultMu.RLock()
	override := defaultOverride
	defaultMu.RUnlock()
	if override != nil {
		return override, nil
	}
	return Embedded()
}

// SetDefault replaces the set Default returns, for tests that bootstrap
// packages they built; restore puts the previous one back.
func SetDefault(set *Set) (restore func()) {
	defaultMu.Lock()
	previous := defaultOverride
	defaultOverride = set
	defaultMu.Unlock()
	return func() {
		defaultMu.Lock()
		defaultOverride = previous
		defaultMu.Unlock()
	}
}

// HostVersion resolves the release a `package-host <id>` process serves.
// The kernel passes the version it verified; when this binary embeds the
// package, that version must be the embedded one (the kernel admits a
// built-in host only for the release this binary carries). Without a
// version, a network module started by an operator serves the embedded one.
func HostVersion(id, requested string) (string, error) {
	set, err := Embedded()
	if err != nil {
		return "", fmt.Errorf("read the embedded packages: %w", err)
	}
	embeddedPackage, ok := set.Lookup(id)
	switch {
	case !ok:
		return requested, nil
	case requested == "":
		return embeddedPackage.Version, nil
	case requested != embeddedPackage.Version:
		return "", fmt.Errorf("this binary embeds %s %s, not %s", id, embeddedPackage.Version, requested)
	default:
		return requested, nil
	}
}
