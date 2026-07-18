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
// them to a signed package release. The manager verifies every supplied digest
// again immediately before starting the entrypoint.
type ArtifactRef struct {
	PackageID        string
	Version          string
	ArtifactPath     string
	ArtifactSHA256   string
	EntrypointPath   string
	EntrypointSHA256 string
	ManifestPath     string
	ManifestSHA256   string
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
	if _, err := verifyFileDigest(ref.ArtifactPath, ref.ArtifactSHA256, false); err != nil {
		return err
	}
	if _, err := verifyFileDigest(ref.EntrypointPath, ref.EntrypointSHA256, true); err != nil {
		return err
	}
	manifestBytes, err := verifyFileDigest(ref.ManifestPath, ref.ManifestSHA256, false)
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

func verifyFileDigest(filePath, expectedDigest string, executable bool) ([]byte, error) {
	if !filepath.IsAbs(filePath) {
		return nil, fmt.Errorf("%w: file path must be absolute", ErrHostIncompatible)
	}
	expected, err := hex.DecodeString(expectedDigest)
	if err != nil || len(expected) != sha256.Size {
		return nil, fmt.Errorf("%w: file digest is invalid", ErrHostIncompatible)
	}
	info, err := os.Lstat(filePath)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: verified file is unavailable", ErrHostIncompatible)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("%w: verified file is writable by group or others", ErrHostIncompatible)
	}
	if executable && info.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("%w: entrypoint is not executable", ErrHostIncompatible)
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: verified file is unavailable", ErrHostIncompatible)
	}
	defer file.Close()
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
