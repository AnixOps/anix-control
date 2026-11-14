package agentinstall

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"golang.org/x/crypto/ssh"
	"golang.org/x/mod/semver"
)

// The kernel the Agent runs as its anixcore core: anx-node from the
// private anx release (AnixOps' own kernel with ANX), which Control hosts
// next to the Agent release so nodes need no access to that repository.
// agent_install.artifact_dir holds a release as
//
//	<dir>/anx-node/<version>/anx-node-<version>-<target>-linux-musl.tar.gz
//	<dir>/anx-node/<version>/SHA256SUMS
//	<dir>/anx-node/<version>/SHA256SUMS.sig
//
// exactly as the release workflow uploads them, and Control serves them at
// /install/kernel/anx-node/<version>/<file>. The trust is the release's SSH
// signature over SHA256SUMS, which the Agent checks with the key it
// embeds; a campaign only checks that the files are there and agree. A
// campaign installs the newest release there (LatestKernelVersion), so
// copying a release in is all it takes to roll it out. An administrator
// can also upload one (StoreKernelRelease), which is checked against the
// same built-in key before it is stored.

// KernelArches maps an architecture to anx-node's release target.
var KernelArches = map[string]string{
	"amd64": "x86_64",
	"arm64": "aarch64",
}

const (
	// KernelReleaseKey is the kernel release signing key, the one
	// install.sh (KERNEL_RELEASE_KEY) and the Agent embed: SHA256SUMS.sig
	// is `ssh-keygen -Y sign` by it in KernelSignatureNamespace. It is the
	// only key an uploaded kernel release is checked against.
	KernelReleaseKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIH9RCOAuDwKyWAXIp0szDWKN5t98NKsCK4isHLR05yLT"
	// KernelSignatureNamespace is the namespace of the kernel signature.
	KernelSignatureNamespace = "anx-release"
)

// kernelReleaseKey is KernelReleaseKey; tests sign with their own.
var kernelReleaseKey = KernelReleaseKey

var (
	// ErrKernelRelease means the kernel release cannot be pushed to nodes,
	// or an uploaded one does not verify.
	ErrKernelRelease = errors.New("the kernel release cannot be pushed")
	// ErrKernelReleaseFile means an uploaded file is not a file of a
	// kernel release.
	ErrKernelReleaseFile = errors.New("not a kernel release file")
	// ErrKernelReleaseConflict means another SHA256SUMS is stored for the
	// uploaded version.
	ErrKernelReleaseConflict = errors.New("another release of this kernel version is stored")
)

// KernelAsset is the anx-node package of version for target.
func KernelAsset(version, target string) string {
	return agentcontrol.KernelAnxNode + "-" + version + "-" + target + "-linux-musl.tar.gz"
}

// KernelPath confines a kernel release file to
// dir/anx-node/<version>/<name>: one of the version's packages,
// SHA256SUMS or SHA256SUMS.sig.
func KernelPath(dir, version, name string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("no artifact directory")
	}
	if !agentcontrol.KernelVersionPattern.MatchString(version) {
		return "", errors.New("not a kernel release version")
	}
	allowed := name == SumsAssetName || name == SumsSignatureAssetName
	for _, target := range KernelArches {
		allowed = allowed || name == KernelAsset(version, target)
	}
	if !allowed {
		return "", errors.New("not a kernel release file")
	}
	return filepath.Join(filepath.Clean(dir), agentcontrol.KernelAnxNode, version, name), nil
}

// LatestKernelVersion is the newest anx-node release version that has a
// directory in artifactDir, "" for none.
func LatestKernelVersion(artifactDir string) string {
	if strings.TrimSpace(artifactDir) == "" {
		return ""
	}
	entries, err := os.ReadDir(filepath.Join(filepath.Clean(artifactDir), agentcontrol.KernelAnxNode))
	if err != nil {
		return ""
	}
	latest := ""
	for _, entry := range entries {
		version := entry.Name()
		if !entry.IsDir() || !agentcontrol.KernelVersionPattern.MatchString(version) {
			continue
		}
		if latest == "" || semver.Compare("v"+version, "v"+latest) > 0 {
			latest = version
		}
	}
	return latest
}

// KernelReleaseHeld is the newest anx-node release in artifactDir
// (LatestKernelVersion) when its SHA256SUMS and SHA256SUMS.sig are there,
// so an installer can fetch and verify it; "" otherwise.
func KernelReleaseHeld(artifactDir string) string {
	version := LatestKernelVersion(artifactDir)
	if version == "" {
		return ""
	}
	for _, name := range []string{SumsAssetName, SumsSignatureAssetName} {
		path, err := KernelPath(artifactDir, version, name)
		if err != nil {
			return ""
		}
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			return ""
		}
	}
	return version
}

// UpgradeKernels checks the newest anx-node release in artifactDir
// (LatestKernelVersion) for an upgrade campaign and returns what
// agent.upgrade carries for every architecture the release has a package
// for: SHA256SUMS with its signature, and each package listed there with
// its SHA-256. No release is no kernel.
func UpgradeKernels(artifactDir, controlURL string) ([]agentcontrol.UpgradeKernel, error) {
	version := LatestKernelVersion(artifactDir)
	if version == "" {
		return nil, nil
	}
	controlURL = strings.TrimRight(strings.TrimSpace(controlURL), "/")
	if !strings.HasPrefix(controlURL, "https://") {
		return nil, fmt.Errorf("%w: nodes download the kernel from Control over https: set agent_install.public_url", ErrKernelRelease)
	}
	read := func(name string) ([]byte, error) {
		path, err := KernelPath(artifactDir, version, name)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrKernelRelease, err)
		}
		data, err := os.ReadFile(path) // #nosec G304 -- KernelPath confines the name to the artifact directory.
		if err != nil {
			return nil, fmt.Errorf("%w: %s of anx-node %s is missing from agent_install.artifact_dir (%s)", ErrKernelRelease, name, version, filepath.Dir(path))
		}
		return data, nil
	}
	sums, err := read(SumsAssetName)
	if err != nil {
		return nil, err
	}
	signature, err := read(SumsSignatureAssetName)
	if err != nil {
		return nil, err
	}
	arches := make([]string, 0, len(KernelArches))
	for arch := range KernelArches {
		arches = append(arches, arch)
	}
	sort.Strings(arches)
	var kernels []agentcontrol.UpgradeKernel
	for _, arch := range arches {
		asset := KernelAsset(version, KernelArches[arch])
		listed, ok := agentcontrol.KernelSum(string(sums), asset)
		if !ok {
			continue
		}
		data, err := read(asset)
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > agentcontrol.MaxUpgradeKernelBytes {
			return nil, fmt.Errorf("%w: %s is larger than %d bytes", ErrKernelRelease, asset, agentcontrol.MaxUpgradeKernelBytes)
		}
		if digest := sha256Hex(data); digest != listed {
			return nil, fmt.Errorf("%w: %s does not match SHA256SUMS (%s, got %s)", ErrKernelRelease, asset, listed, digest)
		}
		kernels = append(kernels, agentcontrol.UpgradeKernel{
			Name: agentcontrol.KernelAnxNode, Version: version, Arch: arch, Asset: asset,
			URL:    controlURL + "/install/kernel/" + agentcontrol.KernelAnxNode + "/" + version + "/" + asset,
			SHA256: listed, Size: int64(len(data)), Sums: string(sums), SumsSignature: string(signature),
		})
	}
	if len(kernels) == 0 {
		return nil, fmt.Errorf("%w: SHA256SUMS of anx-node %s lists no package for %s", ErrKernelRelease, version, strings.Join(arches, ", "))
	}
	return kernels, nil
}

// VerifyKernelSums checks SHA256SUMS.sig, an armored SSH signature
// (`ssh-keygen -Y sign`, PROTOCOL.sshsig) of sums by KernelReleaseKey in
// KernelSignatureNamespace, as the Agent does.
func VerifyKernelSums(sums []byte, signature string) error {
	return VerifyReleaseSums(sums, signature, kernelReleaseKey)
}

// VerifyReleaseSums checks that signature is an armored SSH signature of
// sums in KernelSignatureNamespace by trustedKey (an authorized_keys line).
// Every anx release (anx-node, the ANX clients: internal/clientrelease) is
// signed this way by KernelReleaseKey.
func VerifyReleaseSums(sums []byte, signature, trustedKey string) error {
	const begin, end = "-----BEGIN SSH SIGNATURE-----", "-----END SSH SIGNATURE-----"
	text := strings.TrimSpace(signature)
	if !strings.HasPrefix(text, begin) || !strings.HasSuffix(text, end) {
		return errors.New("not an armored SSH signature")
	}
	blob, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(text[len(begin):len(text)-len(end)]), ""))
	if err != nil {
		return fmt.Errorf("the signature is not base64: %w", err)
	}
	if !bytes.HasPrefix(blob, []byte("SSHSIG")) {
		return errors.New("the signature has no SSHSIG preamble")
	}
	var wire struct {
		Version       uint32
		PublicKey     []byte
		Namespace     string
		Reserved      []byte
		HashAlgorithm string
		Signature     []byte
	}
	if err := ssh.Unmarshal(blob[6:], &wire); err != nil {
		return fmt.Errorf("the signature is malformed: %w", err)
	}
	if wire.Version != 1 {
		return fmt.Errorf("signature version %d is not 1", wire.Version)
	}
	if wire.Namespace != KernelSignatureNamespace {
		return fmt.Errorf("the signature's namespace is %q, not %q", wire.Namespace, KernelSignatureNamespace)
	}
	signer, err := ssh.ParsePublicKey(wire.PublicKey)
	if err != nil {
		return fmt.Errorf("the signature's key: %w", err)
	}
	trusted, _, _, _, err := ssh.ParseAuthorizedKey([]byte(trustedKey))
	if err != nil {
		return fmt.Errorf("the trusted key: %w", err)
	}
	if !bytes.Equal(signer.Marshal(), trusted.Marshal()) {
		return errors.New("the signature is not by the release key")
	}
	var digest []byte
	switch wire.HashAlgorithm {
	case "sha512":
		sum := sha512.Sum512(sums)
		digest = sum[:]
	case "sha256":
		sum := sha256.Sum256(sums)
		digest = sum[:]
	default:
		return fmt.Errorf("hash algorithm %q is not sha256 or sha512", wire.HashAlgorithm)
	}
	var sig ssh.Signature
	if err := ssh.Unmarshal(wire.Signature, &sig); err != nil {
		return fmt.Errorf("the signature is malformed: %w", err)
	}
	signed := append([]byte("SSHSIG"), ssh.Marshal(struct {
		Namespace     string
		Reserved      []byte
		HashAlgorithm string
		Hash          []byte
	}{KernelSignatureNamespace, wire.Reserved, wire.HashAlgorithm, digest})...)
	if err := trusted.Verify(signed, &sig); err != nil {
		return fmt.Errorf("the signature does not verify: %w", err)
	}
	return nil
}

// kernelPackageVersion is the version of an anx-node package name,
// "" when name is not one.
func kernelPackageVersion(name string) string {
	for _, target := range KernelArches {
		version, ok := strings.CutSuffix(strings.TrimPrefix(name, agentcontrol.KernelAnxNode+"-"), "-"+target+"-linux-musl.tar.gz")
		if ok && agentcontrol.KernelVersionPattern.MatchString(version) && name == KernelAsset(version, target) {
			return version
		}
	}
	return ""
}

// StoreKernelRelease stores an uploaded anx-node release in
// artifactDir/anx-node/<version>/: SHA256SUMS, its signature by the
// built-in KernelReleaseKey and packages (file name to contents), all of
// one version and each listed in SHA256SUMS with its SHA-256. A release
// can come in several uploads with the same SHA256SUMS, one package each;
// another SHA256SUMS for a stored version is ErrKernelReleaseConflict.
func StoreKernelRelease(artifactDir string, sums []byte, signature string, packages map[string][]byte) (string, error) {
	if len(packages) == 0 {
		return "", fmt.Errorf("%w: no anx-node package", ErrKernelReleaseFile)
	}
	if err := VerifyKernelSums(sums, signature); err != nil {
		return "", fmt.Errorf("%w: SHA256SUMS: %v", ErrKernelRelease, err)
	}
	// Names first, so a bad name is ErrKernelReleaseFile whatever the order.
	version := ""
	for name := range packages {
		v := kernelPackageVersion(name)
		if v == "" || (version != "" && v != version) {
			return "", fmt.Errorf("%w: %q is not an anx-node package of one release", ErrKernelReleaseFile, name)
		}
		version = v
	}
	for name, data := range packages {
		if int64(len(data)) > agentcontrol.MaxUpgradeKernelBytes {
			return "", fmt.Errorf("%w: %s is larger than %d bytes", ErrKernelRelease, name, agentcontrol.MaxUpgradeKernelBytes)
		}
		listed, ok := agentcontrol.KernelSum(string(sums), name)
		if digest := sha256Hex(data); !ok || listed != digest {
			return "", fmt.Errorf("%w: SHA256SUMS does not list %s with %s", ErrKernelRelease, name, digest)
		}
	}
	sumsPath, err := KernelPath(artifactDir, version, SumsAssetName)
	if err != nil {
		return "", err
	}
	if stored, err := os.ReadFile(sumsPath); err == nil && !bytes.Equal(stored, sums) { // #nosec G304 -- KernelPath confines the name to the artifact directory.
		return "", fmt.Errorf("%w: anx-node %s", ErrKernelReleaseConflict, version)
	}
	if err := os.MkdirAll(filepath.Dir(sumsPath), 0o755); err != nil { // #nosec G301 -- Control serves these files to nodes.
		return "", err
	}
	write := func(name string, data []byte) error {
		path, err := KernelPath(artifactDir, version, name)
		if err != nil {
			return err
		}
		return writeFileAtomic(path, data)
	}
	names := make([]string, 0, len(packages))
	for name := range packages {
		names = append(names, name)
	}
	sort.Strings(names)
	// Packages first and SHA256SUMS last: once SHA256SUMS is there
	// (KernelReleaseHeld), the packages of this upload are too. A package
	// SHA256SUMS lists but no upload brought yet keeps campaigns refusing
	// the release (UpgradeKernels) until it comes.
	for _, name := range names {
		if err := write(name, packages[name]); err != nil {
			return "", err
		}
	}
	if err := write(SumsSignatureAssetName, []byte(signature)); err != nil {
		return "", err
	}
	if err := write(SumsAssetName, sums); err != nil {
		return "", err
	}
	return version, nil
}

// writeFileAtomic writes data to path through a temporary file in its
// directory, so a reader never sees half a file.
func writeFileAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, err = file.Write(data)
	if err == nil {
		err = file.Chmod(0o644)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
