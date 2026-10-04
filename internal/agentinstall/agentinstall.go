// Package agentinstall is one-command node onboarding (forward-sdk.md,
// section 9): the install script Control serves at /install.sh, its release
// signature, the release metadata the script reads from
// /install/agent.env, and the install commands the node page copies.
//
// The script is static: the same bytes are embedded here, served by every
// Control and published as the release asset agent-install.sh, so one
// release signature (Ed25519 by the official package root, the format of
// the packages archive's .sig) verifies all copies. What differs per
// Control — its address, the node and the one-time token — travels in the
// command line; what differs per release — the Agent version, mirrors and
// checksums — comes from /install/agent.env.
package agentinstall

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Script is the install script, byte for byte the release asset.
//
//go:embed install.sh
var Script []byte

const (
	// ScriptAssetName is the script's name among Control's release assets.
	ScriptAssetName = "agent-install.sh"
	// SignatureAssetName is its detached signature.
	SignatureAssetName = ScriptAssetName + ".sig"
	// AgentReleaseBase is where GitHub serves Agent release assets.
	AgentReleaseBase = "https://github.com/AnixOps/anix-agent/releases/download"
	// ControlReleaseBase is where GitHub serves Control release assets.
	ControlReleaseBase = "https://github.com/AnixOps/anix-control/releases/download"
	// MetadataFormat is the version of the /install/agent.env format.
	MetadataFormat = "1"
)

// Mirrors the install command can name (--mirror).
const (
	MirrorControl = "control"
	MirrorCN      = "cn"
	MirrorGitHub  = "github"
)

// Mirrors lists the mirrors in the order the node page shows them.
var Mirrors = []string{MirrorControl, MirrorCN, MirrorGitHub}

// Assets maps an architecture to the Agent's release asset.
var Assets = map[string]string{
	"amd64": "anix-agent-linux-64.zip",
	"arm64": "anix-agent-linux-arm64-v8a.zip",
}

var (
	// ReleaseTagPattern matches release tags (Control's and the Agent's).
	ReleaseTagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$`)
	assetPattern      = regexp.MustCompile(`^anix-agent-linux-[a-z0-9-]+\.zip(\.dgst|\.sig)?$`)
	sha256Pattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	dgstLinePattern   = regexp.MustCompile(`^SHA(?:2-)?256(?:\([^)]*\))?=\s*([0-9a-fA-F]{64})\s*$`)
	nodePattern       = regexp.MustCompile(`^(proxy|forward)-[1-9][0-9]{0,9}$`)
)

// ErrNoSignature means no valid signature is available for Script.
var ErrNoSignature = errors.New("no valid install script signature")

// VerifySignature checks a detached signature of script: base64 of the raw
// Ed25519 signature over its exact bytes (whitespace ignored), by the
// base64 raw public key (plugins.official_public_key).
func VerifySignature(script, signature []byte, publicKeyBase64 string) error {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(publicKeyBase64))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: the official public key must be base64 of 32 raw Ed25519 bytes", ErrNoSignature)
	}
	raw, err := base64.StdEncoding.DecodeString(string(bytes.Join(bytes.Fields(signature), nil)))
	if err != nil || len(raw) != ed25519.SignatureSize {
		return fmt.Errorf("%w: the signature is not base64 of an Ed25519 signature", ErrNoSignature)
	}
	if !ed25519.Verify(ed25519.PublicKey(key), script, raw) {
		return fmt.Errorf("%w: the signature does not match the script", ErrNoSignature)
	}
	return nil
}

// LoadSignature reads the signature file and returns it when it verifies
// Script with the public key.
func LoadSignature(path, publicKeyBase64 string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%w: agent_install.signature_file is not set", ErrNoSignature)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoSignature, err)
	}
	if err := VerifySignature(Script, data, publicKeyBase64); err != nil {
		return nil, err
	}
	return data, nil
}

// ValidNode reports whether node is an Agent node name the command can
// carry: proxy-<id> or forward-<id>.
func ValidNode(node string) bool { return nodePattern.MatchString(node) }

// ShellQuote quotes value for a POSIX shell: single quotes, with embedded
// single quotes closed, escaped and reopened. Words made only of safe
// characters stay bare, so commands stay readable.
func ShellQuote(value string) string {
	if value != "" && strings.Trim(value, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.,/:=@+%") == "" {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// Command is one install command line for a mirror.
type Command struct {
	Mirror  string `json:"mirror"`
	Command string `json:"command"`
	// Available is false when the mirror is not set up; the command still
	// works, with the fallback Note describes.
	Available bool   `json:"available"`
	Note      string `json:"note,omitempty"`
}

// CommandInput is what an install command carries.
type CommandInput struct {
	// ControlURL is the Control address nodes reach (scheme://host[:port]).
	ControlURL string
	// ScriptURL is where the command downloads the script.
	ScriptURL string
	Node      string
	Token     string
	Mirror    string
}

// RenderCommand returns the one-line install command:
//
//	curl -fsSL <script> | sudo bash -s -- --control <url> --node <node> --token <t> [--mirror m]
//
// The "--" ends bash's own options, so every later word reaches the script.
// curl gets -g (no URL globbing) when the URL has an IPv6 literal.
func RenderCommand(input CommandInput) string {
	curl := "curl -fsSL"
	if strings.ContainsAny(input.ScriptURL, "[]") {
		curl += "g"
	}
	words := []string{
		curl, ShellQuote(input.ScriptURL), "|", "sudo", "bash", "-s", "--",
		"--control", ShellQuote(input.ControlURL), "--node", ShellQuote(input.Node), "--token", ShellQuote(input.Token),
	}
	if input.Mirror != "" && input.Mirror != MirrorControl {
		words = append(words, "--mirror", ShellQuote(input.Mirror))
	}
	return strings.Join(words, " ")
}

// Settings is a Control's onboarding configuration, resolved.
type Settings struct {
	// ControlURL is the Control address nodes reach, without a trailing
	// slash.
	ControlURL string
	// GRPCTarget is the gRPC listener's host:port nodes dial.
	GRPCTarget string
	// AgentVersion is the Agent release tag.
	AgentVersion string
	// ControlVersion is Control's release tag (where the github mirror's
	// script is published).
	ControlVersion string
	ArtifactDir    string
	CNMirrorURL    string
}

// ScriptURL is where the mirror's command downloads the script: Control
// for control and cn, the Control release on GitHub for github.
func (s Settings) ScriptURL(mirror string) string {
	if mirror == MirrorGitHub {
		return ControlReleaseBase + "/" + s.ControlVersion + "/" + ScriptAssetName
	}
	return s.ControlURL + "/install.sh"
}

// Commands renders the command for every mirror. node and token are
// validated by the caller.
func (s Settings) Commands(node, token string) []Command {
	metadata := s.Metadata()
	commands := make([]Command, 0, len(Mirrors))
	for _, mirror := range Mirrors {
		command := Command{Mirror: mirror, Available: true}
		switch mirror {
		case MirrorControl:
			if metadata.Sources[MirrorControl] == "" {
				command.Available = false
				command.Note = "Control has no Agent release in agent_install.artifact_dir; the script downloads it from GitHub releases"
			}
		case MirrorCN:
			if metadata.Sources[MirrorCN] == "" {
				command.Available = false
				command.Note = "agent_install.cn_mirror_url is not set; the script falls back to Control, then GitHub releases"
			}
		case MirrorGitHub:
			command.Note = "the script and the Agent come from GitHub releases; the token still enrolls with this Control"
		}
		command.Command = RenderCommand(CommandInput{
			ControlURL: s.ControlURL, ScriptURL: s.ScriptURL(mirror), Node: node, Token: token, Mirror: mirror,
		})
		commands = append(commands, command)
	}
	return commands
}

// Metadata is what /install/agent.env tells the script.
type Metadata struct {
	AgentVersion string
	GRPCTarget   string
	// Sources maps a mirror to the base URL of <tag>/<asset>; a mirror that
	// is not set up is absent.
	Sources map[string]string
	// SHA256 maps an asset to its digest, when Control holds the release.
	SHA256 map[string]string
}

// Metadata resolves the release metadata: the control source and digests
// only when ArtifactDir holds this version's assets with their .dgst files.
func (s Settings) Metadata() Metadata {
	metadata := Metadata{
		AgentVersion: s.AgentVersion, GRPCTarget: s.GRPCTarget,
		Sources: map[string]string{MirrorGitHub: AgentReleaseBase}, SHA256: map[string]string{},
	}
	if mirror := strings.TrimRight(strings.TrimSpace(s.CNMirrorURL), "/"); mirror != "" {
		metadata.Sources[MirrorCN] = mirror
	}
	if digests := s.localDigests(); len(digests) == len(Assets) {
		metadata.Sources[MirrorControl] = s.ControlURL + "/install/agent"
		metadata.SHA256 = digests
	}
	return metadata
}

// localDigests reads ArtifactDir/<tag>/<asset>.dgst for every asset that
// is present.
func (s Settings) localDigests() map[string]string {
	digests := map[string]string{}
	if strings.TrimSpace(s.ArtifactDir) == "" || !ReleaseTagPattern.MatchString(s.AgentVersion) {
		return digests
	}
	for _, asset := range Assets {
		path, err := ArtifactPath(s.ArtifactDir, s.AgentVersion, asset)
		if err != nil {
			continue
		}
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path + ".dgst") // #nosec G304 -- ArtifactPath confines the name to the artifact directory.
		if err != nil {
			continue
		}
		if digest, ok := ParseDigest(data); ok {
			digests[asset] = digest
		}
	}
	return digests
}

// ParseDigest returns the SHA-256 of an Agent .dgst file (`openssl dgst`
// lines with the file name removed: "SHA2-256= <hex>" or "SHA256= <hex>").
func ParseDigest(data []byte) (string, bool) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if match := dgstLinePattern.FindStringSubmatch(strings.TrimSpace(scanner.Text())); match != nil {
			return strings.ToLower(match[1]), true
		}
	}
	return "", false
}

// ArtifactPath confines a release asset to dir/<tag>/<asset>, refusing any
// name but a release tag and an Agent asset (or its .dgst and .sig).
func ArtifactPath(dir, tag, asset string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("no artifact directory")
	}
	if !ReleaseTagPattern.MatchString(tag) || !assetPattern.MatchString(asset) {
		return "", errors.New("not an Agent release asset")
	}
	return filepath.Join(filepath.Clean(dir), tag, asset), nil
}

// Env renders the metadata as the script reads it: one "key value..." per
// line, every value from a fixed alphabet (the script never evaluates it).
func (m Metadata) Env() (string, error) {
	var out strings.Builder
	line := func(words ...string) error {
		for _, word := range words {
			if word == "" || strings.ContainsAny(word, " \t\r\n'\"`$\\;&|<>(){}*?!#~") {
				return fmt.Errorf("metadata value %q is not safe", word)
			}
		}
		out.WriteString(strings.Join(words, " "))
		out.WriteByte('\n')
		return nil
	}
	if err := line("format", MetadataFormat); err != nil {
		return "", err
	}
	if !ReleaseTagPattern.MatchString(m.AgentVersion) {
		return "", fmt.Errorf("agent version %q is not a release tag", m.AgentVersion)
	}
	if err := line("agent_version", m.AgentVersion); err != nil {
		return "", err
	}
	if m.GRPCTarget != "" {
		if err := line("grpc_target", m.GRPCTarget); err != nil {
			return "", err
		}
	}
	for _, arch := range sortedKeys(Assets) {
		if err := line("asset", arch, Assets[arch]); err != nil {
			return "", err
		}
	}
	for _, mirror := range Mirrors {
		source, ok := m.Sources[mirror]
		if !ok {
			continue
		}
		if parsed, err := url.Parse(source); err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return "", fmt.Errorf("source %q of mirror %s is not an http(s) URL", source, mirror)
		}
		if err := line("source", mirror, source); err != nil {
			return "", err
		}
	}
	for _, asset := range sortedKeys(m.SHA256) {
		if !sha256Pattern.MatchString(m.SHA256[asset]) {
			return "", fmt.Errorf("digest of %s is not a SHA-256", asset)
		}
		if err := line("sha256", asset, m.SHA256[asset]); err != nil {
			return "", err
		}
	}
	return out.String(), nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
