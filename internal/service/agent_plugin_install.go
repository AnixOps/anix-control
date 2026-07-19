package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

const AgentPluginInstallAPIVersion = "anixops.io/plugin-install/v1alpha1"

const maxAgentPluginArtifactBytes = MaxPluginArtifactBytes

var (
	ErrAgentPluginAssignmentDenied   = errors.New("node is not assigned the requested plugin release")
	ErrAgentPluginAssignmentConflict = errors.New("node plugin assignments have conflicting desired state")
	ErrAgentPluginReleaseIntegrity   = errors.New("agent plugin release integrity check failed")
	ErrAgentPluginReconcileNotReady  = errors.New("agent plugin assignment is not ready to reconcile")
	ErrAgentPluginAddressMismatch    = errors.New("agent plugin download address does not match verified content")
)

type AgentPluginInstallConfig struct {
	APIVersion string                     `json:"api_version"`
	PluginID   string                     `json:"plugin_id"`
	Version    string                     `json:"version"`
	Artifact   AgentPluginArtifactAddress `json:"artifact"`
	Manifest   AgentPluginManifestAddress `json:"manifest"`
}

type AgentPluginArtifactAddress struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type AgentPluginManifestAddress struct {
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	Signature  string `json:"signature"`
	Publisher  string `json:"publisher"`
	KeyID      string `json:"key_id"`
	APIVersion string `json:"api_version"`
}

// AgentPluginReleaseDownload is verified immediately before a node-facing
// response is written. Data is intentionally kept out of JSON response types.
type AgentPluginReleaseDownload struct {
	Release      model.PluginRelease
	Manifest     PluginManifest
	ManifestData []byte
	Artifact     model.PluginArtifact
	ManifestHash string
}

// LoadAuthorizedAgentPluginMetadata verifies assignment, global installation,
// official signature and persisted size metadata without loading the package
// blob. Reconcile and manifest requests must stay on this bounded path.
func LoadAuthorizedAgentPluginMetadata(db *gorm.DB, nodeID uint, pluginID, version string) (*AgentPluginReleaseDownload, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	pluginID, version = strings.TrimSpace(pluginID), strings.TrimSpace(version)
	if nodeID == 0 || !safePluginSegment(pluginID) || !safePluginSegment(version) {
		return nil, ErrAgentPluginAssignmentDenied
	}

	var assignmentCount int64
	if err := db.Model(&model.NodeServiceAssignment{}).
		Where("node_id = ? AND plugin_id = ? AND desired_version = ? AND enabled = ? AND delete_pending = ?", nodeID, pluginID, version, true, false).
		Count(&assignmentCount).Error; err != nil {
		return nil, err
	}
	if assignmentCount == 0 {
		return nil, ErrAgentPluginAssignmentDenied
	}
	var installation model.PluginInstallation
	if err := db.First(&installation, "plugin_id = ? AND target = ? AND desired_version = ? AND enabled = ?", pluginID, "agent", version, true).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentPluginAssignmentDenied
		}
		return nil, err
	}

	var plugin model.Plugin
	if err := db.First(&plugin, "id = ? AND official = ? AND publisher = ?", pluginID, true, "AnixOps").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentPluginAssignmentDenied
		}
		return nil, err
	}
	var release model.PluginRelease
	if err := db.First(&release, "plugin_id = ? AND version = ?", pluginID, version).Error; err != nil {
		return nil, err
	}
	manifest, err := VerifyStoredPluginRelease(db, release, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAgentPluginReleaseIntegrity, err)
	}
	if !manifestSupportsTarget(*manifest, "agent") {
		return nil, fmt.Errorf("%w: release does not support the agent target", ErrAgentPluginReleaseIntegrity)
	}
	manifestData, err := CanonicalPluginManifest(*manifest)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAgentPluginReleaseIntegrity, err)
	}
	if !bytes.Equal(manifestData, []byte(release.ManifestJSON)) {
		return nil, fmt.Errorf("%w: stored manifest is not canonical", ErrAgentPluginReleaseIntegrity)
	}

	var artifactMetadata struct {
		ID             uint
		ReleaseID      uint
		PluginID       string
		Version        string
		ArtifactSHA256 string
		SizeBytes      int64
		StorageKey     string
		CreatedAt      time.Time
		DataLength     int64
	}
	if err := db.Model(&model.PluginArtifact{}).
		Select("id, release_id, plugin_id, version, artifact_sha256, size_bytes, storage_key, created_at, LENGTH(data) AS data_length").
		Where("release_id = ?", release.ID).Scan(&artifactMetadata).Error; err != nil {
		return nil, err
	}
	if artifactMetadata.ID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	artifact := model.PluginArtifact{
		ID: artifactMetadata.ID, ReleaseID: artifactMetadata.ReleaseID, PluginID: artifactMetadata.PluginID,
		Version: artifactMetadata.Version, ArtifactSHA256: artifactMetadata.ArtifactSHA256,
		SizeBytes: artifactMetadata.SizeBytes, StorageKey: artifactMetadata.StorageKey, CreatedAt: artifactMetadata.CreatedAt,
	}
	if artifact.PluginID != pluginID || artifact.Version != version ||
		!strings.EqualFold(artifact.ArtifactSHA256, release.ArtifactSHA256) ||
		artifact.SizeBytes != artifactMetadata.DataLength || artifact.SizeBytes <= 0 ||
		artifact.SizeBytes > maxAgentPluginArtifactBytes {
		return nil, fmt.Errorf("%w: artifact metadata or size is invalid", ErrAgentPluginReleaseIntegrity)
	}
	manifestDigest := sha256.Sum256(manifestData)
	return &AgentPluginReleaseDownload{
		Release: release, Manifest: *manifest, ManifestData: manifestData,
		Artifact: artifact, ManifestHash: hex.EncodeToString(manifestDigest[:]),
	}, nil
}

// LoadAuthorizedAgentPluginRelease is the unbounded blob path used only for
// the authenticated artifact response. It re-hashes bytes immediately before
// streaming so database corruption cannot cross the trust boundary.
func LoadAuthorizedAgentPluginRelease(db *gorm.DB, nodeID uint, pluginID, version string) (*AgentPluginReleaseDownload, error) {
	metadata, err := LoadAuthorizedAgentPluginMetadata(db, nodeID, pluginID, version)
	if err != nil {
		return nil, err
	}
	return LoadAgentPluginArtifactBlob(db, metadata)
}

// LoadAgentPluginArtifactBlob is intentionally separate from authorization and
// content-address validation. Node-facing handlers call it only after the
// bounded metadata path has accepted the exact immutable address.
func LoadAgentPluginArtifactBlob(db *gorm.DB, metadata *AgentPluginReleaseDownload) (*AgentPluginReleaseDownload, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if metadata == nil || metadata.Release.ID == 0 || metadata.Artifact.ID == 0 {
		return nil, fmt.Errorf("%w: verified artifact metadata is required", ErrAgentPluginReleaseIntegrity)
	}
	artifact, err := GetPluginArtifact(db, metadata.Release.ID)
	if err != nil {
		return nil, err
	}
	if artifact.SizeBytes != int64(len(artifact.Data)) || artifact.SizeBytes != metadata.Artifact.SizeBytes ||
		artifact.PluginID != metadata.Artifact.PluginID || artifact.Version != metadata.Artifact.Version ||
		!strings.EqualFold(artifact.ArtifactSHA256, metadata.Artifact.ArtifactSHA256) {
		return nil, fmt.Errorf("%w: artifact blob does not match metadata", ErrAgentPluginReleaseIntegrity)
	}
	if err := VerifyPluginArtifact(metadata.Manifest, artifact.Data); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAgentPluginReleaseIntegrity, err)
	}
	metadata.Artifact = *artifact
	return metadata, nil
}

func (download *AgentPluginReleaseDownload) ValidateArtifactAddress(sha256Hex string, size int64) error {
	if download == nil || !strings.EqualFold(strings.TrimSpace(sha256Hex), download.Artifact.ArtifactSHA256) || size != download.Artifact.SizeBytes {
		return ErrAgentPluginAddressMismatch
	}
	return nil
}

func (download *AgentPluginReleaseDownload) ValidateManifestAddress(sha256Hex string, size int64) error {
	if download == nil || !strings.EqualFold(strings.TrimSpace(sha256Hex), download.ManifestHash) || size != int64(len(download.ManifestData)) {
		return ErrAgentPluginAddressMismatch
	}
	return nil
}

func BuildAgentPluginInstallConfig(db *gorm.DB, nodeID uint, pluginID, version string) (string, error) {
	download, err := LoadAuthorizedAgentPluginMetadata(db, nodeID, pluginID, version)
	if err != nil {
		return "", err
	}
	basePath := "/api/v3/agent/plugin-releases/" + url.PathEscape(download.Release.PluginID) + "/" + url.PathEscape(download.Release.Version)
	artifactQuery := url.Values{}
	artifactQuery.Set("sha256", strings.ToLower(download.Artifact.ArtifactSHA256))
	artifactQuery.Set("size", strconv.FormatInt(download.Artifact.SizeBytes, 10))
	manifestQuery := url.Values{}
	manifestQuery.Set("sha256", download.ManifestHash)
	manifestQuery.Set("size", strconv.FormatInt(int64(len(download.ManifestData)), 10))
	payload := AgentPluginInstallConfig{
		APIVersion: AgentPluginInstallAPIVersion,
		PluginID:   download.Release.PluginID,
		Version:    download.Release.Version,
		Artifact: AgentPluginArtifactAddress{
			URL: basePath + "/artifact?" + artifactQuery.Encode(), SHA256: strings.ToLower(download.Artifact.ArtifactSHA256), Size: download.Artifact.SizeBytes,
		},
		Manifest: AgentPluginManifestAddress{
			URL: basePath + "/manifest?" + manifestQuery.Encode(), SHA256: download.ManifestHash, Size: int64(len(download.ManifestData)),
			Signature: download.Release.Signature, Publisher: download.Manifest.Publisher,
			KeyID: download.Release.TrustRootKeyID, APIVersion: download.Manifest.APIVersion,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return CanonicalKernelOperationConfig(string(raw))
}

type AgentAssignmentOperationChain struct {
	Install *model.KernelOperation
	Update  *model.KernelOperation
	Enable  *model.KernelOperation
	Disable *model.KernelOperation
}
