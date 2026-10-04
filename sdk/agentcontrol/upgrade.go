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
}

// UpgradeState is the state_json of an agent.upgrade ObservedState.
type UpgradeState struct {
	Phase     string `json:"phase,omitempty"`
	Version   string `json:"version,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

var (
	upgradeVersionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$`)
	upgradeArchPattern    = regexp.MustCompile(`^[a-z0-9]{1,16}$`)
	upgradeAssetPattern   = regexp.MustCompile(`^anix-agent-linux-[a-z0-9-]+\.zip$`)
	upgradeSHA256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
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
		if len(r.Artifacts) != 0 {
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
	return nil
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
