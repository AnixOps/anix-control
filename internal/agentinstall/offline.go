package agentinstall

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SettingsInput is what ResolveSettings needs from a Control's
// configuration (agent_install, grpc.port) and build.
type SettingsInput struct {
	// ControlURL is agent_install.public_url, or the request's origin.
	ControlURL     string
	GRPCTarget     string
	GRPCPort       int
	AgentVersion   string
	ReleaseVersion string
	ArtifactDir    string
	CNMirrorURL    string
}

// ResolveSettings resolves a Control's onboarding settings: the https
// address nodes reach, the gRPC target (agent_install.grpc_target, else
// that address's host and grpc.port), and the Agent release (agent_install.
// agent_version, else "v" + Control's version, H25).
func ResolveSettings(in SettingsInput) (Settings, error) {
	controlURL := strings.TrimRight(strings.TrimSpace(in.ControlURL), "/")
	parsed, err := url.Parse(controlURL)
	if controlURL == "" || err != nil || parsed.Host == "" {
		return Settings{}, errors.New("set agent_install.public_url: Control cannot tell the address nodes reach it at")
	}
	if parsed.Scheme != "https" {
		return Settings{}, errors.New("nodes must reach Control over https: set agent_install.public_url to its https:// address")
	}
	controlVersion := "v" + strings.TrimPrefix(strings.TrimSpace(in.ReleaseVersion), "v")
	agentVersion := strings.TrimSpace(in.AgentVersion)
	if agentVersion == "" {
		agentVersion = controlVersion
	}
	if !ReleaseTagPattern.MatchString(agentVersion) {
		return Settings{}, errors.New("this Control is not a release build: set agent_install.agent_version to the Agent release tag")
	}
	grpcTarget := strings.TrimSpace(in.GRPCTarget)
	if grpcTarget == "" {
		port := in.GRPCPort
		if port <= 0 {
			port = 50051
		}
		grpcTarget = net.JoinHostPort(parsed.Hostname(), strconv.Itoa(port))
	}
	return Settings{
		ControlURL: controlURL, GRPCTarget: grpcTarget, AgentVersion: agentVersion, ControlVersion: controlVersion,
		ArtifactDir: in.ArtifactDir, CNMirrorURL: in.CNMirrorURL,
	}, nil
}

// Release file names besides the per-architecture assets: the checksums
// of every asset and its signature (anix-agent's release job).
const (
	SumsAssetName          = "SHA256SUMS"
	SumsSignatureAssetName = SumsAssetName + ".sig"
	// MetadataFileName is the bundle's copy of /install/agent.env.
	MetadataFileName = "agent.env"
	// BundleScriptName is the install script inside a bundle.
	BundleScriptName = "install.sh"
)

// ErrOfflineBundle means an offline bundle cannot be made.
var ErrOfflineBundle = errors.New("cannot make the offline bundle")

// ReleaseFilePath confines a release file to dir/<tag>/<name>: an Agent
// asset, its .dgst or .sig, SHA256SUMS or SHA256SUMS.sig. Unlike
// ArtifactPath it admits the checksum files, which Control does not serve.
func ReleaseFilePath(dir, tag, name string) (string, error) {
	if name == SumsAssetName || name == SumsSignatureAssetName {
		if strings.TrimSpace(dir) == "" || !ReleaseTagPattern.MatchString(tag) {
			return "", errors.New("not an Agent release file")
		}
		return filepath.Join(filepath.Clean(dir), tag, name), nil
	}
	return ArtifactPath(dir, tag, name)
}

// OfflineBundle describes a written bundle.
type OfflineBundle struct {
	AgentVersion string   `json:"agent_version"`
	Arch         string   `json:"arch"`
	Asset        string   `json:"asset"`
	SHA256       string   `json:"sha256"`
	GRPCTarget   string   `json:"grpc_target"`
	Files        []string `json:"files"`
}

// WriteOfflineBundle writes the offline bundle of arch to w: a tar.gz of
// flat files that `install.sh --offline` reads,
//
//	agent.env                  /install/agent.env of this Control
//	<asset>, <asset>.sig       the Agent release zip of arch and its signature
//	SHA256SUMS, SHA256SUMS.sig the release checksums and their signature
//	install.sh[, install.sh.sig] the install script (and its release signature)
//
// from the release in s.ArtifactDir/<tag>/. Both signatures must verify
// with publicKeyBase64 (plugins.official_public_key) and SHA256SUMS must
// list the asset's digest, which the installer checks again on the node.
// scriptSignature is the script's verified release signature, or nil.
func WriteOfflineBundle(w io.Writer, s Settings, arch, publicKeyBase64 string, scriptSignature []byte) (OfflineBundle, error) {
	asset, ok := Assets[arch]
	if !ok {
		return OfflineBundle{}, fmt.Errorf("%w: unknown architecture %q (amd64 or arm64)", ErrOfflineBundle, arch)
	}
	if strings.TrimSpace(s.ArtifactDir) == "" {
		return OfflineBundle{}, fmt.Errorf("%w: set agent_install.artifact_dir to a directory with the Agent release as <dir>/<tag>/<asset>", ErrOfflineBundle)
	}
	read := func(name string) ([]byte, error) {
		path, err := ReleaseFilePath(s.ArtifactDir, s.AgentVersion, name)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrOfflineBundle, err)
		}
		data, err := os.ReadFile(path) // #nosec G304 -- ReleaseFilePath confines the name to the artifact directory.
		if err != nil {
			return nil, fmt.Errorf("%w: %s of the Agent release %s is missing from agent_install.artifact_dir: %v", ErrOfflineBundle, name, s.AgentVersion, err)
		}
		return data, nil
	}
	files := map[string][]byte{}
	for _, name := range []string{asset, asset + ".sig", SumsAssetName, SumsSignatureAssetName} {
		data, err := read(name)
		if err != nil {
			return OfflineBundle{}, err
		}
		files[name] = data
	}
	if err := VerifySignature(files[SumsAssetName], files[SumsSignatureAssetName], publicKeyBase64); err != nil {
		return OfflineBundle{}, fmt.Errorf("%w: SHA256SUMS: %v", ErrOfflineBundle, err)
	}
	if err := VerifySignature(files[asset], files[asset+".sig"], publicKeyBase64); err != nil {
		return OfflineBundle{}, fmt.Errorf("%w: %s: %v", ErrOfflineBundle, asset, err)
	}
	digest, ok := sumOf(files[SumsAssetName], asset)
	if !ok {
		return OfflineBundle{}, fmt.Errorf("%w: SHA256SUMS does not list %s", ErrOfflineBundle, asset)
	}
	if actual := sha256Hex(files[asset]); actual != digest {
		return OfflineBundle{}, fmt.Errorf("%w: %s does not match SHA256SUMS (%s, got %s)", ErrOfflineBundle, asset, digest, actual)
	}
	env, err := s.Metadata().Env()
	if err != nil {
		return OfflineBundle{}, fmt.Errorf("%w: %v", ErrOfflineBundle, err)
	}
	files[MetadataFileName] = []byte(env)
	files[BundleScriptName] = Script
	order := []string{MetadataFileName, asset, asset + ".sig", SumsAssetName, SumsSignatureAssetName, BundleScriptName}
	if len(scriptSignature) > 0 {
		files[BundleScriptName+".sig"] = scriptSignature
		order = append(order, BundleScriptName+".sig")
	}

	gz := gzip.NewWriter(w)
	archive := tar.NewWriter(gz)
	modTime := time.Now().UTC().Truncate(time.Second)
	for _, name := range order {
		mode := int64(0o644)
		if name == BundleScriptName {
			mode = 0o755
		}
		header := &tar.Header{Name: name, Mode: mode, Size: int64(len(files[name])), ModTime: modTime, Typeflag: tar.TypeReg, Format: tar.FormatPAX}
		if err := archive.WriteHeader(header); err != nil {
			return OfflineBundle{}, err
		}
		if _, err := archive.Write(files[name]); err != nil {
			return OfflineBundle{}, err
		}
	}
	if err := archive.Close(); err != nil {
		return OfflineBundle{}, err
	}
	if err := gz.Close(); err != nil {
		return OfflineBundle{}, err
	}
	return OfflineBundle{
		AgentVersion: s.AgentVersion, Arch: arch, Asset: asset, SHA256: digest, GRPCTarget: s.GRPCTarget, Files: order,
	}, nil
}

// sumOf finds name's digest in a sha256sum listing ("<hex>  <name>", or
// "<hex> *<name>" in binary mode).
func sumOf(sums []byte, name string) (string, bool) {
	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			digest := strings.ToLower(fields[0])
			return digest, sha256Pattern.MatchString(digest)
		}
	}
	return "", false
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
