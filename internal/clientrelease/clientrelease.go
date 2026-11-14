// Package clientrelease holds the releases of the ANX edition of the
// AnixOps Net client, which Control hosts so its subscribers can download
// them (docs/reference/client-downloads.md).
//
// The client has two editions (owner decision, 2026-11-14): the
// open-source one, without ANX, is published on GitHub; the ANX edition is
// closed source and never published anywhere public, so Control is where
// subscribers get it. The private anx release workflows build it and upload
// a release here (POST /api/v4/kernel/client-releases), signed like an
// anx-node release: SHA256SUMS and SHA256SUMS.sig, `ssh-keygen -Y sign` by
// agentinstall.KernelReleaseKey in the "anx-release" namespace. Control
// stores a release only when that signature verifies and every file is
// listed in SHA256SUMS with its digest. agent_install.artifact_dir holds
// them as
//
//	<dir>/anixops-net-anx/<version>/anixops-net-anx-<version>-<platform suffix>
//	<dir>/anixops-net-anx/<version>/SHA256SUMS
//	<dir>/anixops-net-anx/<version>/SHA256SUMS.sig
//
// The newest version (semver, prereleases included: every client release
// so far is a beta) is the one subscribers are offered.
package clientrelease

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/agentinstall"
	"golang.org/x/mod/semver"
)

const (
	// Product is the name of the ANX edition's files and of its directory
	// in the artifact directory.
	Product = "anixops-net-anx"
	// SumsName and SignatureName are the release's checksum list and its
	// signature.
	SumsName      = agentinstall.SumsAssetName
	SignatureName = agentinstall.SumsSignatureAssetName
	// MaxFileBytes bounds one file of a release.
	MaxFileBytes = 512 << 20
	// MaxSumsBytes bounds SHA256SUMS and its signature.
	MaxSumsBytes = 64 << 10
	// maxLinkBytes bounds the TestFlight link file.
	maxLinkBytes = 2 << 10
)

// Platform is one kind of file a release can hold.
type Platform struct {
	// Key names the platform in the API and the download page.
	Key string `json:"key"`
	// Suffix ends the file name: anixops-net-anx-<version><Suffix>.
	Suffix string `json:"suffix"`
	// OS groups the platforms the download page shows together.
	OS string `json:"os"`
	// Link is a text file holding an https address (the iOS TestFlight
	// invitation) instead of a package.
	Link bool `json:"link,omitempty"`
}

// Platforms are the files a release can hold, in the order the download
// page shows them.
var Platforms = []Platform{
	{Key: "windows", Suffix: "-windows-x64.msi", OS: "windows"},
	{Key: "windows-portable", Suffix: "-windows-x64-portable.zip", OS: "windows"},
	{Key: "macos", Suffix: "-macos.pkg", OS: "macos"},
	{Key: "macos-dmg", Suffix: "-macos.dmg", OS: "macos"},
	{Key: "linux-deb", Suffix: "-linux-amd64.deb", OS: "linux"},
	{Key: "linux", Suffix: "-linux-x64.tar.gz", OS: "linux"},
	{Key: "android", Suffix: "-android-arm64.apk", OS: "android"},
	{Key: "ios", Suffix: "-ios-testflight.url", OS: "ios", Link: true},
}

// VersionPattern is a release version: semver without the leading v.
var VersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$`)

// releaseKey is the only key a release is checked against; tests trust
// their own.
var releaseKey = agentinstall.KernelReleaseKey

var (
	// ErrInvalid means an upload is not a verifiable release.
	ErrInvalid = errors.New("not a valid client release")
	// ErrConflict means another SHA256SUMS is stored for the version.
	ErrConflict = errors.New("another release of this client version is stored")
	// ErrNotFound means there is no such release or file.
	ErrNotFound = errors.New("no such client release")
)

// FileName is the file of version for platform.
func FileName(version string, platform Platform) string {
	return Product + "-" + version + platform.Suffix
}

// parseName returns the version and platform of a release file name.
func parseName(name string) (string, Platform, bool) {
	rest, ok := strings.CutPrefix(name, Product+"-")
	if !ok {
		return "", Platform{}, false
	}
	for _, platform := range Platforms {
		version, ok := strings.CutSuffix(rest, platform.Suffix)
		if ok && VersionPattern.MatchString(version) && len(version) <= 64 {
			return version, platform, true
		}
	}
	return "", Platform{}, false
}

// Path confines a release file to dir/anixops-net-anx/<version>/<name>:
// one of the version's files, SHA256SUMS or SHA256SUMS.sig.
func Path(dir, version, name string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("no artifact directory")
	}
	if !VersionPattern.MatchString(version) || len(version) > 64 {
		return "", ErrNotFound
	}
	if name != SumsName && name != SignatureName {
		v, _, ok := parseName(name)
		if !ok || v != version {
			return "", ErrNotFound
		}
	}
	return filepath.Join(filepath.Clean(dir), Product, version, name), nil
}

// File is one file of a stored release.
type File struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
	OS       string `json:"os"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

// Release is a stored release: its files that are there and listed in
// its SHA256SUMS, in the order of Platforms, and the TestFlight address
// when it has one.
type Release struct {
	Version       string `json:"version"`
	Files         []File `json:"files"`
	TestFlightURL string `json:"testflight_url,omitempty"`
}

// Versions are the versions with a complete release (SHA256SUMS and its
// signature stored) in dir, newest first.
func Versions(dir string) []string {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(filepath.Clean(dir), Product))
	if err != nil {
		return nil
	}
	var versions []string
	for _, entry := range entries {
		version := entry.Name()
		if !entry.IsDir() || !VersionPattern.MatchString(version) {
			continue
		}
		complete := true
		for _, name := range []string{SumsName, SignatureName} {
			path, err := Path(dir, version, name)
			if err != nil {
				complete = false
				break
			}
			if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
				complete = false
				break
			}
		}
		if complete {
			versions = append(versions, version)
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		if c := semver.Compare("v"+versions[i], "v"+versions[j]); c != 0 {
			return c > 0
		}
		return versions[i] > versions[j]
	})
	return versions
}

// Latest is the newest complete release in dir that holds at least one
// file; ErrNotFound when there is none.
func Latest(dir string) (Release, error) {
	for _, version := range Versions(dir) {
		release, err := Load(dir, version)
		if err == nil && len(release.Files) > 0 {
			return release, nil
		}
	}
	return Release{}, ErrNotFound
}

// Load reads the stored release of version.
func Load(dir, version string) (Release, error) {
	sumsPath, err := Path(dir, version, SumsName)
	if err != nil {
		return Release{}, err
	}
	sums, err := os.ReadFile(sumsPath) // #nosec G304 -- Path confines the name to the artifact directory.
	if err != nil {
		return Release{}, ErrNotFound
	}
	release := Release{Version: version, Files: []File{}}
	for _, platform := range Platforms {
		name := FileName(version, platform)
		digest, ok := agentcontrol.KernelSum(string(sums), name)
		if !ok {
			continue
		}
		path, err := Path(dir, version, name)
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if platform.Link {
			link, err := readLink(path)
			if err != nil {
				continue
			}
			release.TestFlightURL = link
		}
		release.Files = append(release.Files, File{
			Name: name, Platform: platform.Key, OS: platform.OS, Size: info.Size(), SHA256: digest,
		})
	}
	return release, nil
}

// readLink reads a link file: one https address.
func readLink(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- Path confines the name to the artifact directory.
	if err != nil {
		return "", err
	}
	return checkLink(data)
}

func checkLink(data []byte) (string, error) {
	if len(data) > maxLinkBytes {
		return "", errors.New("the link file is too large")
	}
	text := strings.TrimSpace(string(data))
	parsed, err := url.Parse(text)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || strings.ContainsAny(text, " \t\r\n") {
		return "", errors.New("the link file does not hold one https address")
	}
	return text, nil
}

// Upload is one uploaded file.
type Upload struct {
	Name string
	Open func() (io.ReadCloser, error)
}

// Store stores an uploaded release in dir/anixops-net-anx/<version>/:
// SHA256SUMS, its signature by the release key and files of one version,
// each listed in SHA256SUMS with its SHA-256. A release can come in
// several uploads with the same SHA256SUMS; another SHA256SUMS for a
// stored version is ErrConflict. It returns the version.
func Store(dir string, sums []byte, signature string, files []Upload) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("no artifact directory")
	}
	if len(files) == 0 {
		return "", fmt.Errorf("%w: no client package", ErrInvalid)
	}
	if len(sums) > MaxSumsBytes || len(signature) > MaxSumsBytes {
		return "", fmt.Errorf("%w: SHA256SUMS or its signature is too large", ErrInvalid)
	}
	if err := agentinstall.VerifyReleaseSums(sums, signature, releaseKey); err != nil {
		return "", fmt.Errorf("%w: SHA256SUMS: %v", ErrInvalid, err)
	}
	version := ""
	seen := map[string]bool{}
	for _, file := range files {
		v, _, ok := parseName(file.Name)
		if !ok || (version != "" && v != version) {
			return "", fmt.Errorf("%w: %q is not a %s file of one release", ErrInvalid, file.Name, Product)
		}
		if seen[file.Name] {
			return "", fmt.Errorf("%w: %s is uploaded twice", ErrInvalid, file.Name)
		}
		if _, ok := agentcontrol.KernelSum(string(sums), file.Name); !ok {
			return "", fmt.Errorf("%w: SHA256SUMS does not list %s", ErrInvalid, file.Name)
		}
		seen[file.Name] = true
		version = v
	}
	sumsPath, err := Path(dir, version, SumsName)
	if err != nil {
		return "", err
	}
	if stored, err := os.ReadFile(sumsPath); err == nil && !bytes.Equal(stored, sums) { // #nosec G304 -- Path confines the name to the artifact directory.
		return "", fmt.Errorf("%w: %s", ErrConflict, version)
	}
	if err := os.MkdirAll(filepath.Dir(sumsPath), 0o750); err != nil {
		return "", err
	}
	// Check every file into a temporary file first, so a bad upload stores
	// nothing, then move them in; SHA256SUMS last, so a release is listed
	// (Versions) only once its files of this upload are there.
	staged := make([]string, 0, len(files))
	defer func() {
		for _, path := range staged {
			_ = os.Remove(path)
		}
	}()
	targets := make([]string, 0, len(files))
	for _, file := range files {
		listed, _ := agentcontrol.KernelSum(string(sums), file.Name)
		_, platform, _ := parseName(file.Name)
		temp, err := stage(filepath.Dir(sumsPath), file, listed, platform.Link)
		if temp != "" {
			staged = append(staged, temp)
		}
		if err != nil {
			return "", err
		}
		target, err := Path(dir, version, file.Name)
		if err != nil {
			return "", err
		}
		targets = append(targets, target)
	}
	for i, temp := range staged {
		if err := os.Rename(temp, targets[i]); err != nil {
			return "", err
		}
	}
	staged = nil
	sigPath, err := Path(dir, version, SignatureName)
	if err != nil {
		return "", err
	}
	if err := writeAtomic(sigPath, []byte(signature)); err != nil {
		return "", err
	}
	if err := writeAtomic(sumsPath, sums); err != nil {
		return "", err
	}
	return version, nil
}

// stage copies one upload into a temporary file in dir and checks its
// size and digest (and, for a link file, its address).
func stage(dir string, file Upload, digest string, link bool) (string, error) {
	reader, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrInvalid, file.Name, err)
	}
	defer func() { _ = reader.Close() }()
	temp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	var head bytes.Buffer
	sinks := []io.Writer{temp, hash}
	if link {
		sinks = append(sinks, &head)
	}
	written, err := io.Copy(io.MultiWriter(sinks...), io.LimitReader(reader, MaxFileBytes+1))
	if err == nil {
		err = temp.Chmod(0o640)
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return temp.Name(), err
	}
	if written > MaxFileBytes {
		return temp.Name(), fmt.Errorf("%w: %s is larger than %d bytes", ErrInvalid, file.Name, MaxFileBytes)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != digest {
		return temp.Name(), fmt.Errorf("%w: SHA256SUMS lists %s with %s, the upload is %s", ErrInvalid, file.Name, digest, got)
	}
	if link {
		if _, err := checkLink(head.Bytes()); err != nil {
			return temp.Name(), fmt.Errorf("%w: %s: %v", ErrInvalid, file.Name, err)
		}
	}
	return temp.Name(), nil
}

// Delete removes the stored release of version.
func Delete(dir, version string) error {
	sumsPath, err := Path(dir, version, SumsName)
	if err != nil {
		return err
	}
	root := filepath.Dir(sumsPath)
	if _, err := os.Stat(root); err != nil {
		return ErrNotFound
	}
	return os.RemoveAll(root)
}

// writeAtomic writes data to path through a temporary file in its
// directory, so a reader never sees half a file.
func writeAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, err = file.Write(data)
	if err == nil {
		err = file.Chmod(0o640)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
