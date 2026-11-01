package agentcontrol

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Agent upgrades (upgrade.v1, PROTOCOL.md "Agent upgrades"): Control pushes
// an Agent release in canary batches (forward-sdk.md section 9, H19) as the
// DesiredOperation kind OperationKindAgentUpgrade, whose payload_json is an
// UpgradeRequest. Control sends it only on a session that negotiated
// CapabilityUpgrade, and an Agent never upgrades on its own.
const (
	// CapabilityUpgrade: the Agent applies agent.upgrade operations through
	// its privileged updater (anixops-agent-updater.path). It adds no
	// stream payload.
	CapabilityUpgrade = "upgrade"
	// OperationKindAgentUpgrade is the DesiredOperation kind of an upgrade
	// or a rollback. It is not a capability name: the session must
	// negotiate upgrade.v1.
	OperationKindAgentUpgrade = "agent.upgrade"
	// UpgradeSchemaV1 is UpgradeRequest.Schema.
	UpgradeSchemaV1 = "anixops.agent-upgrade/v1"
	// UpgradeActionUpgrade installs TargetVersion from Artifacts.
	UpgradeActionUpgrade = "upgrade"
	// UpgradeActionRollback reinstates the binary the Agent kept from its
	// last upgrade, which must be TargetVersion; it carries no artifacts.
	UpgradeActionRollback = "rollback"
	// MaxUpgradeRequestBytes bounds payload_json.
	MaxUpgradeRequestBytes = 16 << 10
	// MaxUpgradeArtifactBytes bounds an artifact's size (the release zip).
	MaxUpgradeArtifactBytes = 256 << 20
	// KernelAnxNode is UpgradeKernel.Name of anx-node, AnixOps' own node
	// kernel with ANX (the private anx release), which the Agent runs as
	// its anixcore core.
	KernelAnxNode = "anx-node"
	// MaxUpgradeKernelBytes bounds a kernel package's size.
	MaxUpgradeKernelBytes = 64 << 20
	// maxKernelManifestBytes bounds UpgradeKernel.Sums and SumsSignature.
	maxKernelManifestBytes = 4 << 10
)

// Phases an Agent reports in the ObservedState.state_json of an
// agent.upgrade operation (UpgradeState.Phase).
const (
	// UpgradePhaseDownloading: APPLYING, fetching the artifact.
	UpgradePhaseDownloading = "downloading"
	// UpgradePhaseVerifying: APPLYING, checking size, SHA-256 and the
	// Ed25519 signature.
	UpgradePhaseVerifying = "verifying"
	// UpgradePhaseHandedOff: SUCCEEDED, the staged release and the request
	// are with the updater, which restarts the Agent. Control closes the
	// loop on the next Hello's agent_version.
	UpgradePhaseHandedOff = "handed_off"
	// UpgradePhaseCurrent: SUCCEEDED, the Agent already runs TargetVersion
	// (a replay after the restart, or a host that was upgraded through
	// another node identity).
	UpgradePhaseCurrent = "current"
)

// Error codes of a FAILED agent.upgrade (UpgradeState.ErrorCode).
const (
	UpgradeErrorInvalidRequest = "upgrade_invalid_request"
	UpgradeErrorNoArtifact     = "upgrade_no_artifact"
	UpgradeErrorDownload       = "upgrade_download_failed"
	UpgradeErrorDigest         = "upgrade_digest_mismatch"
	UpgradeErrorSignature      = "upgrade_signature_invalid"
	UpgradeErrorUpdater        = "upgrade_updater_unavailable"
	UpgradeErrorNoPrevious     = "upgrade_no_previous_release"
	UpgradeErrorBusy           = "upgrade_in_progress"
	UpgradeErrorDowngrade      = "upgrade_downgrade_refused"
)

// UpgradeArtifact is one architecture's release asset: the Agent picks the
// one whose Arch is its GOARCH, downloads URL, and verifies Size, SHA256
// and Signature (base64 Ed25519 over the exact bytes, the release's .sig)
// with the official release key it embeds, never a key Control sends.
type UpgradeArtifact struct {
	Arch      string `json:"arch"`
	Asset     string `json:"asset"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
}

// UpgradeKernel is one architecture's kernel package that comes with an
// upgrade: the Agent's updater installs it next to the Agent binary, and
// the Agent then runs it as its anixcore core. Name is KernelAnxNode,
// Version its release without "v" (0.1.0-alpha.2), Asset the release
// tarball, served at URL. The trust is the kernel release's own signature:
// Sums is its SHA256SUMS, SumsSignature the SSH signature of Sums
// (`ssh-keygen -Y sign`, the release's SHA256SUMS.sig), which the Agent
// verifies with the kernel release key it embeds, never a key Control
// sends; SHA256 must be Asset's line in Sums.
type UpgradeKernel struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Arch          string `json:"arch"`
	Asset         string `json:"asset"`
	URL           string `json:"url"`
	SHA256        string `json:"sha256"`
	Size          int64  `json:"size"`
	Sums          string `json:"sums"`
	SumsSignature string `json:"sums_signature"`
}

// UpgradeRequest is the payload_json of an agent.upgrade operation.
// Fields may be added; an Agent ignores the ones it does not know.
type UpgradeRequest struct {
	Schema     string `json:"schema"`
	CampaignID string `json:"campaign_id"`
	Action     string `json:"action"`
	// TargetVersion is the release to run afterwards (v4.2.0).
	TargetVersion string `json:"target_version"`
	// PreviousVersion is the release the node ran when the campaign
	// offered it the upgrade; the updater keeps that binary for a
	// rollback. Empty when Control did not know it.
	PreviousVersion string            `json:"previous_version,omitempty"`
	Artifacts       []UpgradeArtifact `json:"artifacts,omitempty"`
	// Kernels are the kernel packages to install with the upgrade, at most
	// one per architecture. An Agent that already runs TargetVersion
	// installs a kernel it does not have yet; one that predates them
	// ignores the field.
	Kernels []UpgradeKernel `json:"kernels,omitempty"`
}

// UpgradeState is the state_json of an agent.upgrade ObservedState.
type UpgradeState struct {
	Phase     string `json:"phase,omitempty"`
	Version   string `json:"version,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

var (
	upgradeVersionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+(\.(dev|hf)\.[0-9]+)?)?)?$`)
	upgradeArchPattern    = regexp.MustCompile(`^[a-z0-9]{1,16}$`)
	upgradeAssetPattern   = regexp.MustCompile(`^anix-agent-linux-[a-z0-9-]+\.zip$`)
	upgradeSHA256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	// KernelVersionPattern matches a kernel release version (no "v").
	KernelVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?$`)
	kernelAssetPattern   = regexp.MustCompile(`^anx-node-[0-9a-z.-]+-[a-z0-9_]+-linux-musl\.tar\.gz$`)
)

// ErrInvalidUpgradeRequest wraps every reason ParseUpgradeRequest refuses.
var ErrInvalidUpgradeRequest = errors.New("invalid agent upgrade request")

// SameAgentVersion reports whether two Agent versions name one release,
// with or without the leading "v".
func SameAgentVersion(a, b string) bool {
	a, b = strings.TrimPrefix(strings.TrimSpace(a), "v"), strings.TrimPrefix(strings.TrimSpace(b), "v")
	return a != "" && a == b
}

// ParseUpgradeRequest decodes and checks an agent.upgrade payload as
// both sides must: the schema, the action, release tags, and for an
// upgrade at least one artifact with an https URL, a SHA-256, a size and
// an Ed25519 signature, each architecture once.
func ParseUpgradeRequest(data []byte) (UpgradeRequest, error) {
	var request UpgradeRequest
	if len(data) > MaxUpgradeRequestBytes {
		return request, fmt.Errorf("%w: payload over %d bytes", ErrInvalidUpgradeRequest, MaxUpgradeRequestBytes)
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return request, fmt.Errorf("%w: %v", ErrInvalidUpgradeRequest, err)
	}
	return request, request.Validate()
}

// Validate checks the request (ParseUpgradeRequest).
func (r UpgradeRequest) Validate() error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidUpgradeRequest, fmt.Sprintf(format, args...))
	}
	if r.Schema != UpgradeSchemaV1 {
		return invalid("schema %q is not %s", r.Schema, UpgradeSchemaV1)
	}
	if strings.TrimSpace(r.CampaignID) == "" || len(r.CampaignID) > 64 {
		return invalid("campaign_id must be 1 to 64 bytes")
	}
	if !upgradeVersionPattern.MatchString(r.TargetVersion) {
		return invalid("target_version %q is not a release tag", r.TargetVersion)
	}
	if r.PreviousVersion != "" && !upgradeVersionPattern.MatchString(r.PreviousVersion) {
		return invalid("previous_version %q is not a release tag", r.PreviousVersion)
	}
	switch r.Action {
	case UpgradeActionRollback:
		if len(r.Artifacts) != 0 || len(r.Kernels) != 0 {
			return invalid("a rollback carries no artifacts")
		}
		return nil
	case UpgradeActionUpgrade:
	default:
		return invalid("action %q is not upgrade or rollback", r.Action)
	}
	if len(r.Artifacts) == 0 || len(r.Artifacts) > 8 {
		return invalid("an upgrade carries 1 to 8 artifacts")
	}
	seen := map[string]bool{}
	for _, artifact := range r.Artifacts {
		if !upgradeArchPattern.MatchString(artifact.Arch) || seen[artifact.Arch] {
			return invalid("artifact architecture %q is malformed or repeated", artifact.Arch)
		}
		seen[artifact.Arch] = true
		if !upgradeAssetPattern.MatchString(artifact.Asset) {
			return invalid("artifact asset %q is not an Agent release zip", artifact.Asset)
		}
		parsed, err := url.Parse(artifact.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || len(artifact.URL) > 2048 {
			return invalid("artifact url of %s must be an https URL", artifact.Arch)
		}
		if !upgradeSHA256Pattern.MatchString(artifact.SHA256) {
			return invalid("artifact sha256 of %s is not lowercase hex SHA-256", artifact.Arch)
		}
		if artifact.Size <= 0 || artifact.Size > MaxUpgradeArtifactBytes {
			return invalid("artifact size of %s must be 1 to %d bytes", artifact.Arch, MaxUpgradeArtifactBytes)
		}
		signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(artifact.Signature))
		if err != nil || len(signature) != 64 {
			return invalid("artifact signature of %s is not base64 of an Ed25519 signature", artifact.Arch)
		}
	}
	return validateKernels(r.Kernels, invalid)
}

func validateKernels(kernels []UpgradeKernel, invalid func(string, ...any) error) error {
	if len(kernels) > 8 {
		return invalid("an upgrade carries at most 8 kernels")
	}
	seen := map[string]bool{}
	for _, kernel := range kernels {
		if kernel.Name != KernelAnxNode {
			return invalid("kernel %q is not %s", kernel.Name, KernelAnxNode)
		}
		if !KernelVersionPattern.MatchString(kernel.Version) {
			return invalid("kernel version %q is not a release version", kernel.Version)
		}
		if !upgradeArchPattern.MatchString(kernel.Arch) || seen[kernel.Arch] {
			return invalid("kernel architecture %q is malformed or repeated", kernel.Arch)
		}
		seen[kernel.Arch] = true
		if !kernelAssetPattern.MatchString(kernel.Asset) || !strings.HasPrefix(kernel.Asset, KernelAnxNode+"-"+kernel.Version+"-") {
			return invalid("kernel asset %q is not the %s %s package", kernel.Asset, KernelAnxNode, kernel.Version)
		}
		parsed, err := url.Parse(kernel.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || len(kernel.URL) > 2048 {
			return invalid("kernel url of %s must be an https URL", kernel.Arch)
		}
		if !upgradeSHA256Pattern.MatchString(kernel.SHA256) {
			return invalid("kernel sha256 of %s is not lowercase hex SHA-256", kernel.Arch)
		}
		if kernel.Size <= 0 || kernel.Size > MaxUpgradeKernelBytes {
			return invalid("kernel size of %s must be 1 to %d bytes", kernel.Arch, MaxUpgradeKernelBytes)
		}
		if kernel.Sums == "" || len(kernel.Sums) > maxKernelManifestBytes {
			return invalid("kernel sums of %s must be 1 to %d bytes", kernel.Arch, maxKernelManifestBytes)
		}
		if sum, ok := KernelSum(kernel.Sums, kernel.Asset); !ok || sum != kernel.SHA256 {
			return invalid("kernel sums of %s do not list %s with its sha256", kernel.Arch, kernel.Asset)
		}
		if !strings.HasPrefix(strings.TrimSpace(kernel.SumsSignature), "-----BEGIN SSH SIGNATURE-----") || len(kernel.SumsSignature) > maxKernelManifestBytes {
			return invalid("kernel sums_signature of %s is not an SSH signature", kernel.Arch)
		}
	}
	return nil
}

// KernelSum returns the SHA-256 that a SHA256SUMS manifest (sha256sum
// output: "<hex>  <name>", or "<hex> *<name>") lists for name.
func KernelSum(sums, name string) (string, bool) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name && upgradeSHA256Pattern.MatchString(fields[0]) {
			return fields[0], true
		}
	}
	return "", false
}

// Kernel returns the kernel package of arch.
func (r UpgradeRequest) Kernel(arch string) (UpgradeKernel, bool) {
	for _, kernel := range r.Kernels {
		if kernel.Arch == arch {
			return kernel, true
		}
	}
	return UpgradeKernel{}, false
}

// Artifact returns the artifact of arch.
func (r UpgradeRequest) Artifact(arch string) (UpgradeArtifact, bool) {
	for _, artifact := range r.Artifacts {
		if artifact.Arch == arch {
			return artifact, true
		}
	}
	return UpgradeArtifact{}, false
}
