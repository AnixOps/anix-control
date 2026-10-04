package agentinstall

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
)

// ErrUpgradeRelease means the Agent release cannot be pushed to nodes.
var ErrUpgradeRelease = errors.New("the Agent release cannot be pushed")

// UpgradeArtifacts verifies the Agent release tag in artifactDir for an
// upgrade campaign (forward-sdk.md section 9, O4) and returns what the
// agent.upgrade operation carries for every architecture: each asset must
// be there with its .sig, SHA256SUMS with SHA256SUMS.sig, both signatures
// valid with publicKeyBase64 (plugins.official_public_key), and each
// asset's SHA-256 listed in SHA256SUMS. controlURL is the https address
// nodes reach; the artifacts are served at
// <controlURL>/install/agent/<tag>/<asset> (the control mirror).
func UpgradeArtifacts(artifactDir, tag, publicKeyBase64, controlURL string) ([]agentcontrol.UpgradeArtifact, error) {
	if strings.TrimSpace(artifactDir) == "" {
		return nil, fmt.Errorf("%w: set agent_install.artifact_dir to a directory with the Agent release as <dir>/<tag>/<asset>", ErrUpgradeRelease)
	}
	if !ReleaseTagPattern.MatchString(tag) {
		return nil, fmt.Errorf("%w: %q is not a release tag", ErrUpgradeRelease, tag)
	}
	controlURL = strings.TrimRight(strings.TrimSpace(controlURL), "/")
	if !strings.HasPrefix(controlURL, "https://") {
		return nil, fmt.Errorf("%w: nodes download the release from Control over https: set agent_install.public_url", ErrUpgradeRelease)
	}
	read := func(name string) ([]byte, error) {
		path, err := ReleaseFilePath(artifactDir, tag, name)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUpgradeRelease, err)
		}
		data, err := os.ReadFile(path) // #nosec G304 -- ReleaseFilePath confines the name to the artifact directory.
		if err != nil {
			return nil, fmt.Errorf("%w: %s of the Agent release %s is missing from agent_install.artifact_dir", ErrUpgradeRelease, name, tag)
		}
		return data, nil
	}
	sums, err := read(SumsAssetName)
	if err != nil {
		return nil, err
	}
	sumsSignature, err := read(SumsSignatureAssetName)
	if err != nil {
		return nil, err
	}
	if err := VerifySignature(sums, sumsSignature, publicKeyBase64); err != nil {
		return nil, fmt.Errorf("%w: SHA256SUMS: %v", ErrUpgradeRelease, err)
	}
	arches := make([]string, 0, len(Assets))
	for arch := range Assets {
		arches = append(arches, arch)
	}
	sort.Strings(arches)
	artifacts := make([]agentcontrol.UpgradeArtifact, 0, len(arches))
	for _, arch := range arches {
		asset := Assets[arch]
		path, err := ArtifactPath(artifactDir, tag, asset)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUpgradeRelease, err)
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: %s of the Agent release %s is missing from agent_install.artifact_dir", ErrUpgradeRelease, asset, tag)
		}
		if info.Size() > agentcontrol.MaxUpgradeArtifactBytes {
			return nil, fmt.Errorf("%w: %s is larger than %d bytes", ErrUpgradeRelease, asset, agentcontrol.MaxUpgradeArtifactBytes)
		}
		data, err := os.ReadFile(path) // #nosec G304 -- ArtifactPath confines the name to the artifact directory.
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUpgradeRelease, err)
		}
		signature, err := read(asset + ".sig")
		if err != nil {
			return nil, err
		}
		if err := VerifySignature(data, signature, publicKeyBase64); err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrUpgradeRelease, asset, err)
		}
		digest, size := sha256Hex(data), int64(len(data))
		listed, ok := sumOf(sums, asset)
		if !ok {
			return nil, fmt.Errorf("%w: SHA256SUMS does not list %s", ErrUpgradeRelease, asset)
		}
		if listed != digest {
			return nil, fmt.Errorf("%w: %s does not match SHA256SUMS (%s, got %s)", ErrUpgradeRelease, asset, listed, digest)
		}
		artifacts = append(artifacts, agentcontrol.UpgradeArtifact{
			Arch: arch, Asset: asset, URL: controlURL + "/install/agent/" + tag + "/" + asset,
			SHA256: digest, Size: size, Signature: string(bytes.Join(bytes.Fields(signature), nil)),
		})
	}
	return artifacts, nil
}
