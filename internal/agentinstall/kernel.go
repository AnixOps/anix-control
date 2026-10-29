package agentinstall

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
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
// embeds; Control only checks that the files are there and agree. A
// campaign installs the newest release there (LatestKernelVersion), so
// copying a release in is all it takes to roll it out.

// KernelArches maps an architecture to anx-node's release target.
var KernelArches = map[string]string{
	"amd64": "x86_64",
}

// ErrKernelRelease means the kernel release cannot be pushed to nodes.
var ErrKernelRelease = errors.New("the kernel release cannot be pushed")

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
