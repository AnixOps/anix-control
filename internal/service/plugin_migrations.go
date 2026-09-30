package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
)

const (
	pluginMigrationIndexFormat = "anixops.migrations/v1"
	// maxPluginMigrationBytes bounds one migration script and the index.
	maxPluginMigrationBytes int64 = 4 << 20
	maxPluginMigrationSteps       = 256
)

var pluginMigrationIDPattern = regexp.MustCompile(`^[0-9a-z][0-9a-z_]{0,63}$`)

type pluginMigrationIndex struct {
	Format     string                     `json:"format"`
	PackageID  string                     `json:"package_id"`
	Version    string                     `json:"version"`
	Migrations []pluginMigrationIndexStep `json:"migrations"`
}

type pluginMigrationIndexStep struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	// SHA256 is written by build_package.py since the migration index
	// digests change; v4.0.0 indexes omit it and the digest is computed from
	// the packaged script instead.
	SHA256 string `json:"sha256,omitempty"`
}

// ParsePluginMigrationIndex verifies the signed migration index of a package
// artifact and returns its steps in order. The index must match the manifest
// digest, name the manifest's package and version, and every step script must
// exist in the artifact with the declared digest.
func ParsePluginMigrationIndex(manifest PluginManifest, artifact []byte) ([]pluginhost.MigrationStep, error) {
	if manifest.Migrations == nil {
		return nil, errors.New("manifest does not declare a migration index")
	}
	raw, err := extractPluginArtifactFile(artifact, manifest.Migrations.Index, maxPluginMigrationBytes)
	if err != nil {
		return nil, fmt.Errorf("extract migrations index: %w", err)
	}
	if !strings.EqualFold(sha256Bytes(raw), manifest.Migrations.SHA256) {
		return nil, errors.New("migrations index digest does not match the package artifact")
	}
	var index pluginMigrationIndex
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index); err != nil {
		return nil, fmt.Errorf("decode migrations index: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("migrations index must contain one JSON document")
	}
	if index.Format != pluginMigrationIndexFormat {
		return nil, fmt.Errorf("migrations index format %q is unsupported", index.Format)
	}
	if index.PackageID != manifest.ID || index.Version != manifest.Version {
		return nil, errors.New("migrations index does not match the manifest package and version")
	}
	if len(index.Migrations) > maxPluginMigrationSteps {
		return nil, errors.New("migrations index has too many steps")
	}
	steps := make([]pluginhost.MigrationStep, 0, len(index.Migrations))
	seenIDs := make(map[string]struct{}, len(index.Migrations))
	seenPaths := make(map[string]struct{}, len(index.Migrations))
	for _, entry := range index.Migrations {
		if !pluginMigrationIDPattern.MatchString(entry.ID) {
			return nil, fmt.Errorf("migration id %q is invalid", entry.ID)
		}
		if !safePluginRelativePath(entry.Path) || !strings.HasPrefix(entry.Path, "migrations/") || entry.Path == manifest.Migrations.Index {
			return nil, fmt.Errorf("migration %s path is invalid", entry.ID)
		}
		if _, duplicate := seenIDs[entry.ID]; duplicate {
			return nil, fmt.Errorf("duplicate migration id %q", entry.ID)
		}
		if _, duplicate := seenPaths[entry.Path]; duplicate {
			return nil, fmt.Errorf("duplicate migration path %q", entry.Path)
		}
		seenIDs[entry.ID], seenPaths[entry.Path] = struct{}{}, struct{}{}
		script, err := extractPluginArtifactFile(artifact, entry.Path, maxPluginMigrationBytes)
		if err != nil {
			return nil, fmt.Errorf("extract migration %s: %w", entry.ID, err)
		}
		digest := sha256Bytes(script)
		if entry.SHA256 != "" && !strings.EqualFold(entry.SHA256, digest) {
			return nil, fmt.Errorf("migration %s digest does not match the package artifact", entry.ID)
		}
		steps = append(steps, pluginhost.MigrationStep{ID: entry.ID, Path: entry.Path, SHA256: digest})
	}
	return steps, nil
}
