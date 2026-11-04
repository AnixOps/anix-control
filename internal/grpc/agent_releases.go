package grpc

import (
	"errors"
	"io"
	"io/fs"
	"os"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/agentinstall"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Release downloads (releases.v1; PROTOCOL.md, "Release downloads"): the
// files of an agent.upgrade operation, read from agent_install.artifact_dir
// as /install/agent/... and /install/kernel/... serve them, streamed on the
// session's mTLS connection. The Agent verifies them as it verifies the
// https download.

// releaseArtifactDir is agent_install.artifact_dir; tests replace it.
var releaseArtifactDir = func() string { return config.Get().AgentInstall.ArtifactDir }

// servesReleases tells whether the HelloAck advertises releases.v1: the
// session is offered upgrade.v1 and the Agent lists releases.v1.
func servesReleases(principal agentPrincipal, agent []*agentv1pb.Capability) bool {
	return servesUpgrade(principal.Node, agent) &&
		agentcontrol.HasCapabilityVersion(agent, agentcontrol.CapabilityReleases, agentcontrol.CapabilityVersionV1)
}

// DownloadRelease streams a release file from the request's offset, in
// chunks of at most artifactChunkSize bytes.
func (s *AgentArtifactsGRPCServer) DownloadRelease(request *agentv1pb.DownloadReleaseRequest, stream agentv1pb.AgentArtifacts_DownloadReleaseServer) error {
	err := s.downloadRelease(request, stream)
	setRefusalTrailer(err, stream.SetTrailer)
	return err
}

func (s *AgentArtifactsGRPCServer) downloadRelease(request *agentv1pb.DownloadReleaseRequest, stream agentv1pb.AgentArtifacts_DownloadReleaseServer) error {
	node, err := s.certificateNode(stream.Context())
	if err != nil {
		return err
	}
	if node.Kind != agentcontrol.NodeKindProxy && node.Kind != agentcontrol.NodeKindForward {
		return refuseAgent(agentcontrol.ErrorCodeCertInvalid, codes.PermissionDenied, "only proxy and forward nodes download releases")
	}
	var path string
	switch request.GetKind() {
	case agentcontrol.ReleaseKindAgent:
		path, err = agentinstall.ArtifactPath(releaseArtifactDir(), request.GetVersion(), request.GetAsset())
	case agentcontrol.ReleaseKindKernel:
		path, err = agentinstall.KernelPath(releaseArtifactDir(), request.GetVersion(), request.GetAsset())
	default:
		err = errors.New("kind must be agent or anx-node")
	}
	if err != nil || request.GetOffset() < 0 {
		return refuseAgent(agentcontrol.ErrorCodeReleaseRequestInvalid, codes.InvalidArgument, "the request names no release file")
	}
	if !s.acquire(node.ID) {
		return refuseAgent(agentcontrol.ErrorCodePluginReleaseBusy, codes.ResourceExhausted, "the node runs as many downloads as allowed; retry later")
	}
	defer s.release(node.ID)
	file, err := os.Open(path) // #nosec G304 -- ArtifactPath and KernelPath confine the name to the artifact directory.
	if errors.Is(err, fs.ErrNotExist) {
		return refuseAgent(agentcontrol.ErrorCodeReleaseNotFound, codes.NotFound, "no such release file on this Control")
	}
	if err != nil {
		return status.Error(codes.Unavailable, "the release file could not be read")
	}
	defer func() { _ = file.Close() }()
	offset := request.GetOffset()
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return status.Error(codes.Unavailable, "the release file could not be read")
	}
	buffer := make([]byte, artifactChunkSize)
	for {
		n, err := io.ReadFull(file, buffer)
		if n > 0 {
			if sendErr := stream.Send(&agentv1pb.ReleaseChunk{Offset: offset, Data: buffer[:n]}); sendErr != nil {
				return sendErr
			}
			offset += int64(n)
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil
		}
		if err != nil {
			return status.Error(codes.Unavailable, "the release file could not be read")
		}
	}
}
