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
	file, err := openVerifiedRegularFile(filePath, executable)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readVerifiedDigest(file, expectedDigest)
}

func stageVerifiedEntrypoint(ref ArtifactRef, runtimeDir string) (string, error) {
	if !filepath.IsAbs(runtimeDir) {
		return "", fmt.Errorf("%w: host runtime directory must be absolute", ErrHostIncompatible)
	}
	source, err := openVerifiedRegularFile(ref.EntrypointPath, true)
	if err != nil {
		return "", err
	}
	defer source.Close()
	expected, err := decodeDigest(ref.EntrypointSHA256)
	if err != nil {
		return "", err
	}
	stagedPath := filepath.Join(runtimeDir, "entrypoint")
	destination, err := os.OpenFile(stagedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return "", fmt.Errorf("%w: create staged entrypoint: %v", ErrHostUnavailable, err)
	}
	cleanup := func() {
		_ = destination.Close()
		_ = os.Remove(stagedPath)
	}
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(destination, hasher), source); err != nil {
		cleanup()
		return "", fmt.Errorf("%w: stage entrypoint: %v", ErrHostIncompatible, err)
	}
	if err := destination.Close(); err != nil {
		_ = os.Remove(stagedPath)
		return "", fmt.Errorf("%w: stage entrypoint: %v", ErrHostUnavailable, err)
	}
	if subtle.ConstantTimeCompare(expected, hasher.Sum(nil)) != 1 {
		_ = os.Remove(stagedPath)
		return "", fmt.Errorf("%w: file digest does not match", ErrHostIncompatible)
	}
	if err := os.Chmod(stagedPath, 0o700); err != nil {
		_ = os.Remove(stagedPath)
		return "", fmt.Errorf("%w: secure staged entrypoint: %v", ErrHostUnavailable, err)
	}
	return stagedPath, nil
}

func openVerifiedRegularFile(filePath string, executable bool) (*os.File, error) {
	return openVerifiedRegularFileWithOpen(filePath, executable, os.Open)
}

func openVerifiedRegularFileWithOpen(filePath string, executable bool, openFile func(string) (*os.File, error)) (*os.File, error) {
	if !filepath.IsAbs(filePath) {
		return nil, fmt.Errorf("%w: file path must be absolute", ErrHostIncompatible)
	}
	before, err := os.Lstat(filePath)
	if err != nil || !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: verified file is unavailable", ErrHostIncompatible)
	}
	if err := validateVerifiedFileMode(before, executable); err != nil {
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
	if err := validateVerifiedFileMode(after, executable); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func validateVerifiedFileMode(info os.FileInfo, executable bool) error {
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%w: verified file is writable by group or others", ErrHostIncompatible)
	}
	if executable && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%w: entrypoint is not executable", ErrHostIncompatible)
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
