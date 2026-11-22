//go:build unix

package pluginhost

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ArtifactRef describes files materialized by a caller that has already bound
// them to a signed package release. The host is the one linked into the
// Control binary under PackageID (the signed manifest's
// control_entrypoint.builtin). The manager verifies every supplied digest
// again immediately before starting the host.
type ArtifactRef struct {
	PackageID      string
	Version        string
	ArtifactPath   string
	ArtifactSHA256 string
	ManifestPath   string
	ManifestSHA256 string
	// MigrationIndexSHA256 and Migrations describe the verified migration
	// index of the release; Migrations is empty when it declares no steps.
	MigrationIndexSHA256 string
	Migrations           []MigrationStep
	// Storage is set when the signed manifest declares kernel.storage.v1.
	// Only such releases run their migration index through the kernel's
	// migration ledger when the host starts.
	Storage bool
}

// MigrationStep is one verified step of a package migration index.
type MigrationStep struct {
	ID     string
	Path   string
	SHA256 string
}

// sameArtifactRelease compares the immutable identity established by
// verifyArtifactRef. Materialization paths are deliberately excluded: each
// verified extraction may use a fresh private directory for the same bytes.
func sameArtifactRelease(left, right ArtifactRef) bool {
	return left.PackageID == right.PackageID &&
		left.Version == right.Version &&
		left.ArtifactSHA256 == right.ArtifactSHA256 &&
		left.ManifestSHA256 == right.ManifestSHA256
}

type artifactManifest struct {
	ID      string   `json:"id"`
	Version string   `json:"version"`
	Targets []string `json:"targets"`
}

func verifyArtifactRef(ref ArtifactRef) error {
	if ref.PackageID == "" || ref.Version == "" {
		return fmt.Errorf("%w: package identity is required", ErrHostIncompatible)
	}
	if _, err := verifyFileDigest(ref.ArtifactPath, ref.ArtifactSHA256); err != nil {
		return err
	}
	manifestBytes, err := verifyFileDigest(ref.ManifestPath, ref.ManifestSHA256)
	if err != nil {
		return err
	}
	var manifest artifactManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("%w: manifest is invalid", ErrHostIncompatible)
	}
	if manifest.ID != ref.PackageID || manifest.Version != ref.Version {
		return fmt.Errorf("%w: manifest identity does not match artifact reference", ErrHostIncompatible)
	}
	for _, target := range manifest.Targets {
		if target == "control" {
			return nil
		}
	}
	return fmt.Errorf("%w: manifest does not target control", ErrHostIncompatible)
}

func verifyFileDigest(filePath, expectedDigest string) ([]byte, error) {
	file, err := openVerifiedRegularFile(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return readVerifiedDigest(file, expectedDigest)
}

func openVerifiedRegularFile(filePath string) (*os.File, error) {
	return openVerifiedRegularFileWithOpen(filePath, os.Open)
}

func openVerifiedRegularFileWithOpen(filePath string, openFile func(string) (*os.File, error)) (*os.File, error) {
	if !filepath.IsAbs(filePath) {
		return nil, fmt.Errorf("%w: file path must be absolute", ErrHostIncompatible)
	}
	before, err := os.Lstat(filePath)
	if err != nil || !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: verified file is unavailable", ErrHostIncompatible)
	}
	if err := validateVerifiedFileMode(before); err != nil {
		return nil, err
	}
	file, err := openFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: verified file is unavailable", ErrHostIncompatible)
	}
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, fmt.Errorf("%w: verified file changed before open", ErrHostIncompatible)
	}
	if err := validateVerifiedFileMode(after); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func validateVerifiedFileMode(info os.FileInfo) error {
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%w: verified file is writable by group or others", ErrHostIncompatible)
	}
	return nil
}

func readVerifiedDigest(file *os.File, expectedDigest string) ([]byte, error) {
	expected, err := decodeDigest(expectedDigest)
	if err != nil {
		return nil, err
	}
	hasher := sha256.New()
	contents, err := io.ReadAll(io.TeeReader(file, hasher))
	if err != nil {
		return nil, fmt.Errorf("%w: verified file cannot be read", ErrHostIncompatible)
	}
	if subtle.ConstantTimeCompare(expected, hasher.Sum(nil)) != 1 {
		return nil, fmt.Errorf("%w: file digest does not match", ErrHostIncompatible)
	}
	return contents, nil
}

func decodeDigest(expectedDigest string) ([]byte, error) {
	expected, err := hex.DecodeString(expectedDigest)
	if err != nil || len(expected) != sha256.Size {
		return nil, fmt.Errorf("%w: file digest is invalid", ErrHostIncompatible)
	}
	return expected, nil
}
