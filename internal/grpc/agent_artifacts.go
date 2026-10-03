package grpc

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// AgentArtifacts (artifacts.v1; PROTOCOL.md, "Plugin artifacts") serves an
// enrolled agent the signed plugin releases assigned to its node, as
// GET /api/v3/agent/plugin-releases/... serves them to the node API key:
// the same authorization (service.LoadAuthorizedAgentPluginMetadata), the
// same content address check and the same verified bytes
// (service.LoadAgentPluginArtifactBlob), so the agent verifies them as it
// verifies the HTTP download. Only a client certificate authenticates a
// call; the node it names must be an enabled proxy node.

// Bounds of the artifact download. Variables so tests can lower them.
var (
	// artifactChunkSize is the most artifact bytes in one chunk, well under
	// gRPC's default 4 MiB message limit.
	artifactChunkSize = 1 << 20
	// artifactDownloadsPerNode is the most artifact downloads a node may
	// run at once (the HTTP path has its per-node rate limiter).
	artifactDownloadsPerNode = 2
)

// AgentArtifactsGRPCServer serves AgentArtifacts on the agent listener.
type AgentArtifactsGRPCServer struct {
	agentv1pb.UnimplementedAgentArtifactsServer
	auth      *AgentAuthenticator
	mu        sync.Mutex
	downloads map[uint32]int
}

var _ agentv1pb.AgentArtifactsServer = (*AgentArtifactsGRPCServer)(nil)

// NewAgentArtifactsGRPCServer returns the artifact service of auth's agent
// PKI. Without one, every call is refused with agent_cert_invalid.
func NewAgentArtifactsGRPCServer(auth *AgentAuthenticator) *AgentArtifactsGRPCServer {
	return &AgentArtifactsGRPCServer{auth: auth, downloads: map[uint32]int{}}
}

// servesArtifacts tells whether the HelloAck advertises artifacts.v1: when
// the agent lists it and the session authenticated a proxy node by client
// certificate, as AgentArtifacts requires.
func servesArtifacts(principal agentPrincipal, agent []*agentv1pb.Capability) bool {
	return principal.Certificate && principal.Node.Kind == agentcontrol.NodeKindProxy &&
		agentcontrol.HasCapabilityVersion(agent, agentcontrol.CapabilityArtifacts, agentcontrol.CapabilityVersionV1)
}

// GetPluginManifest returns the canonical manifest of a release assigned to
// the certificate's node.
func (s *AgentArtifactsGRPCServer) GetPluginManifest(ctx context.Context, request *agentv1pb.GetPluginManifestRequest) (*agentv1pb.GetPluginManifestResponse, error) {
	response, err := s.getPluginManifest(ctx, request)
	return answer(ctx, response, err)
}

func (s *AgentArtifactsGRPCServer) getPluginManifest(ctx context.Context, request *agentv1pb.GetPluginManifestRequest) (*agentv1pb.GetPluginManifestResponse, error) {
	node, err := s.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	address, err := artifactAddress(request.GetManifest())
	if err != nil {
		return nil, err
	}
	download, err := service.LoadAuthorizedAgentPluginMetadata(databaseForAgentChecks().WithContext(ctx), uint(node.ID), address.PluginId, address.Version)
	if err != nil {
		return nil, pluginReleaseStatus(node, err)
	}
	if err := download.ValidateManifestAddress(address.Sha256, address.Size); err != nil {
		return nil, pluginReleaseStatus(node, err)
	}
	return &agentv1pb.GetPluginManifestResponse{Release: pluginReleaseMessage(download), ManifestJson: download.ManifestData}, nil
}

// DownloadPluginArtifact streams the artifact of a release assigned to the
// certificate's node, in chunks of at most artifactChunkSize bytes; the
// first carries the release.
func (s *AgentArtifactsGRPCServer) DownloadPluginArtifact(request *agentv1pb.DownloadPluginArtifactRequest, stream agentv1pb.AgentArtifacts_DownloadPluginArtifactServer) error {
	err := s.downloadPluginArtifact(request, stream)
	setRefusalTrailer(err, stream.SetTrailer)
	return err
}

func (s *AgentArtifactsGRPCServer) downloadPluginArtifact(request *agentv1pb.DownloadPluginArtifactRequest, stream agentv1pb.AgentArtifacts_DownloadPluginArtifactServer) error {
	ctx := stream.Context()
	node, err := s.authenticate(ctx)
	if err != nil {
		return err
	}
	address, err := artifactAddress(request.GetArtifact())
	if err != nil {
		return err
	}
	if !s.acquire(node.ID) {
		return refuseAgent(agentcontrol.ErrorCodePluginReleaseBusy, codes.ResourceExhausted, "the node runs as many plugin downloads as allowed; retry later")
	}
	defer s.release(node.ID)
	db := databaseForAgentChecks().WithContext(ctx)
	download, err := service.LoadAuthorizedAgentPluginMetadata(db, uint(node.ID), address.PluginId, address.Version)
	if err != nil {
		return pluginReleaseStatus(node, err)
	}
	if err := download.ValidateArtifactAddress(address.Sha256, address.Size); err != nil {
		return pluginReleaseStatus(node, err)
	}
	download, err = service.LoadAgentPluginArtifactBlob(db, download)
	if err != nil {
		return pluginReleaseStatus(node, err)
	}
	data := download.Artifact.Data
	release := pluginReleaseMessage(download)
	for offset := 0; offset < len(data); offset += artifactChunkSize {
		if err := ctx.Err(); err != nil {
			return status.FromContextError(err).Err()
		}
		chunk := &agentv1pb.PluginArtifactChunk{Offset: int64(offset), Data: data[offset:min(offset+artifactChunkSize, len(data))]}
		if offset == 0 {
			chunk.Release = release
		}
		if err := stream.Send(chunk); err != nil {
			return err
		}
	}
	return nil
}

// authenticate returns the node of the call's client certificate: an
// enabled proxy node. No node API key is read.
func (s *AgentArtifactsGRPCServer) authenticate(ctx context.Context) (agentcontrol.AgentNode, error) {
	principal, ok, err := s.auth.certificatePrincipal(ctx)
	if err != nil {
		return agentcontrol.AgentNode{}, err
	}
	if !ok {
		return agentcontrol.AgentNode{}, refuseAgent(agentcontrol.ErrorCodeCertInvalid, codes.Unauthenticated, "AgentArtifacts requires an agent client certificate")
	}
	if err := agentpki.CheckNodeEnabled(ctx, databaseForAgentChecks(), principal.Node); err != nil {
		if errors.Is(err, agentpki.ErrInvalidNode) {
			return agentcontrol.AgentNode{}, refuseAgent(agentcontrol.ErrorCodeCertRevoked, codes.PermissionDenied, "node is disabled or no longer exists")
		}
		return agentcontrol.AgentNode{}, status.Error(codes.Unavailable, "node check failed")
	}
	if principal.Node.Kind != agentcontrol.NodeKindProxy {
		return agentcontrol.AgentNode{}, refuseAgent(agentcontrol.ErrorCodePluginReleaseNotAssigned, codes.PermissionDenied, "only proxy nodes have plugin assignments")
	}
	return principal.Node, nil
}

func (s *AgentArtifactsGRPCServer) acquire(nodeID uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.downloads[nodeID] >= artifactDownloadsPerNode {
		return false
	}
	s.downloads[nodeID]++
	return true
}

func (s *AgentArtifactsGRPCServer) release(nodeID uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.downloads[nodeID] <= 1 {
		delete(s.downloads, nodeID)
		return
	}
	s.downloads[nodeID]--
}

// artifactAddress checks a content address as the HTTP download checks its
// query: a plugin id and version, a 64-hex sha256 and a positive size. The
// sha256 is returned lowercase.
func artifactAddress(address *agentv1pb.PluginReleaseAddress) (*agentv1pb.PluginReleaseAddress, error) {
	invalid := func(message string) error {
		return refuseAgent(agentcontrol.ErrorCodePluginReleaseAddressInvalid, codes.InvalidArgument, message)
	}
	if address == nil {
		return nil, invalid("the release address is required")
	}
	pluginID, version := strings.TrimSpace(address.GetPluginId()), strings.TrimSpace(address.GetVersion())
	if pluginID == "" || version == "" {
		return nil, invalid("plugin_id and version are required")
	}
	sha256Hex := strings.ToLower(strings.TrimSpace(address.GetSha256()))
	if decoded, err := hex.DecodeString(sha256Hex); err != nil || len(decoded) != 32 {
		return nil, invalid("sha256 must be a 64-character hexadecimal digest")
	}
	if address.GetSize() <= 0 {
		return nil, invalid("size must be a positive integer")
	}
	return &agentv1pb.PluginReleaseAddress{PluginId: pluginID, Version: version, Sha256: sha256Hex, Size: address.GetSize()}, nil
}

// pluginReleaseStatus maps the release errors as the HTTP download does;
// a failure to read the database is Unavailable, without a code.
func pluginReleaseStatus(node agentcontrol.AgentNode, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentPluginAssignmentDenied):
		return refuseAgent(agentcontrol.ErrorCodePluginReleaseNotAssigned, codes.PermissionDenied, err.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		return refuseAgent(agentcontrol.ErrorCodePluginReleaseNotFound, codes.NotFound, "assigned plugin release is unavailable")
	case errors.Is(err, service.ErrAgentPluginReleaseIntegrity):
		slog.Warn("agent artifacts: the release does not verify", "component", "agent-artifacts", "node", node.String(), "error", err)
		return refuseAgent(agentcontrol.ErrorCodePluginReleaseIntegrityFailed, codes.FailedPrecondition, err.Error())
	case errors.Is(err, service.ErrAgentPluginAddressMismatch):
		return refuseAgent(agentcontrol.ErrorCodePluginReleaseAddressMismatch, codes.InvalidArgument, err.Error())
	case status.Code(err) == codes.Canceled || status.Code(err) == codes.DeadlineExceeded:
		return err
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	}
	slog.Warn("agent artifacts: the release could not be read", "component", "agent-artifacts", "node", node.String(), "error", err)
	return status.Error(codes.Unavailable, "the plugin release could not be read")
}

// pluginReleaseMessage is the release as the HTTP download's X-AnixOps-*
// headers describe it.
func pluginReleaseMessage(download *service.AgentPluginReleaseDownload) *agentv1pb.PluginRelease {
	return &agentv1pb.PluginRelease{
		PluginId: download.Release.PluginID, Version: download.Release.Version,
		ArtifactSha256: strings.ToLower(download.Artifact.ArtifactSHA256), ArtifactSize: download.Artifact.SizeBytes,
		ManifestSha256: download.ManifestHash, ManifestSize: int64(len(download.ManifestData)),
		Signature: download.Release.Signature, SignatureAlgorithm: "ed25519", Publisher: download.Manifest.Publisher,
		KeyId: download.Release.TrustRootKeyID, PluginApiVersion: download.Manifest.APIVersion,
	}
}
