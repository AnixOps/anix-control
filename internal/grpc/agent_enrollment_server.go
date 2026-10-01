package grpc

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// databaseForAgentChecks is the kernel database the agent listener reads
// node state from.
func databaseForAgentChecks() *gorm.DB { return database.Get() }

// AgentEnrollmentGRPCServer serves AgentEnrollment on the agent listener.
type AgentEnrollmentGRPCServer struct {
	agentv1pb.UnimplementedAgentEnrollmentServer
	auth *AgentAuthenticator
}

var _ agentv1pb.AgentEnrollmentServer = (*AgentEnrollmentGRPCServer)(nil)

// NewAgentEnrollmentGRPCServer returns the enrollment service of auth's
// agent PKI. Without one, every call answers FailedPrecondition.
func NewAgentEnrollmentGRPCServer(auth *AgentAuthenticator) *AgentEnrollmentGRPCServer {
	return &AgentEnrollmentGRPCServer{auth: auth}
}

func (s *AgentEnrollmentGRPCServer) service() (*agentpki.Service, error) {
	if pki := s.auth.pki(); pki != nil {
		return pki, nil
	}
	return nil, status.Error(codes.FailedPrecondition, agentpki.ErrDisabled.Error())
}

// Enroll issues a node's first certificate for a bootstrap credential: a
// one-time enrollment credential, or the node credential in metadata
// (refused when agent_control.mtls is required).
func (s *AgentEnrollmentGRPCServer) Enroll(ctx context.Context, request *agentv1pb.EnrollAgentRequest) (*agentv1pb.EnrollAgentResponse, error) {
	pki, err := s.service()
	if err != nil {
		return nil, err
	}
	bootstrap, err := s.bootstrap(ctx, request.GetEnrollmentCredential())
	if err != nil {
		return nil, err
	}
	issued, err := pki.Enroll(ctx, agentpki.EnrollRequest{
		Bootstrap: bootstrap, CSRDER: request.GetCsrDer(), AgentVersion: request.GetAgentVersion(),
		InstanceID: request.GetInstanceId(), RemoteAddr: remoteHost(ctx),
	})
	if err != nil {
		if errors.Is(err, agentpki.ErrEnrollmentRejected) {
			slog.Warn("agent enrollment rejected", "component", "agent-pki", "method", bootstrap.Method, "addr", remoteHost(ctx))
		}
		return nil, agentPKIStatus(err)
	}
	slog.Info("agent enrolled", "component", "agent-pki", "node", issued.Identity.Node.String(), "method", bootstrap.Method,
		"serial", issued.Serial, "not_after", issued.NotAfter)
	return &agentv1pb.EnrollAgentResponse{Certificate: agentCertificateMessage(issued)}, nil
}

// bootstrap reads the bootstrap credential of an Enroll call. A node id in
// the metadata is the node the agent claims; with an enrollment credential
// it must match the credential's node.
func (s *AgentEnrollmentGRPCServer) bootstrap(ctx context.Context, credential string) (agentpki.Bootstrap, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	first := func(key string) string {
		if values := md.Get(key); len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
		return ""
	}
	kind := first(agentcontrol.MetadataNodeKind)
	if kind == "" {
		kind = agentcontrol.NodeKindProxy
	}
	var node agentcontrol.AgentNode
	if rawID := first(agentcontrol.MetadataNodeID); rawID != "" {
		id, err := strconv.ParseUint(rawID, 10, 32)
		if err != nil || id == 0 {
			return agentpki.Bootstrap{}, status.Error(codes.Unauthenticated, "invalid x-node-id")
		}
		node = agentcontrol.AgentNode{Kind: kind, ID: uint32(id)}
		if !node.Valid() {
			return agentpki.Bootstrap{}, status.Error(codes.Unauthenticated, "invalid x-node-kind")
		}
	}
	if credential != "" {
		return agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Node: node, Secret: credential}, nil
	}
	if s.auth.mode() == config.AgentMTLSRequired {
		return agentpki.Bootstrap{}, status.Error(codes.Unauthenticated, "an enrollment credential is required (agent_control.mtls: required)")
	}
	secret := first(agentcontrol.MetadataAPIKey)
	if secret == "" || node == (agentcontrol.AgentNode{}) {
		return agentpki.Bootstrap{}, status.Error(codes.Unauthenticated, "an enrollment credential or x-node-id and x-api-key are required")
	}
	method := model.AgentEnrollmentMethodNodeAPIKey
	if node.Kind == agentcontrol.NodeKindForward {
		method = model.AgentEnrollmentMethodForwardToken
	}
	return agentpki.Bootstrap{Method: method, Node: node, Secret: secret}, nil
}

// Renew issues a new certificate to the holder of a valid one.
func (s *AgentEnrollmentGRPCServer) Renew(ctx context.Context, request *agentv1pb.RenewAgentCertificateRequest) (*agentv1pb.RenewAgentCertificateResponse, error) {
	pki, err := s.service()
	if err != nil {
		return nil, err
	}
	chain := peerCertificateChain(ctx)
	if len(chain) == 0 {
		return nil, status.Error(codes.Unauthenticated, "renewal requires the current agent client certificate")
	}
	_, leaf, err := pki.VerifyPeer(ctx, chain)
	if err != nil {
		return nil, agentPKIStatus(err)
	}
	issued, err := pki.Renew(ctx, leaf, request.GetCsrDer())
	if err != nil {
		return nil, agentPKIStatus(err)
	}
	return &agentv1pb.RenewAgentCertificateResponse{Certificate: agentCertificateMessage(issued)}, nil
}

// GetTrustBundle returns the CAs of agent certificates to an agent that
// holds one.
func (s *AgentEnrollmentGRPCServer) GetTrustBundle(ctx context.Context, _ *agentv1pb.GetAgentTrustBundleRequest) (*agentv1pb.GetAgentTrustBundleResponse, error) {
	pki, err := s.service()
	if err != nil {
		return nil, err
	}
	if _, ok, err := s.auth.certificatePrincipal(ctx); err != nil {
		return nil, err
	} else if !ok {
		return nil, status.Error(codes.Unauthenticated, "an agent client certificate is required")
	}
	bundle, err := pki.TrustBundle(ctx)
	if err != nil {
		return nil, agentPKIStatus(err)
	}
	response := &agentv1pb.GetAgentTrustBundleResponse{}
	for _, certificate := range bundle {
		response.TrustBundleDer = append(response.TrustBundleDer, certificate.Raw)
	}
	return response, nil
}

func agentCertificateMessage(issued agentpki.Issued) *agentv1pb.AgentCertificate {
	return &agentv1pb.AgentCertificate{
		CertificateDer: issued.CertificateDER, TrustBundleDer: issued.TrustBundleDER, SpiffeId: issued.Identity.String(),
		Node: issued.Identity.Node.String(), Serial: issued.Serial, NotAfterUnix: issued.NotAfter.Unix(),
		RenewAfterUnix: issued.RenewAfter.Unix(),
	}
}

// agentPKIStatus maps agent PKI errors to gRPC statuses without details
// that would let a caller probe credentials.
func agentPKIStatus(err error) error {
	switch {
	case status.Code(err) != codes.Unknown:
		return err
	case errors.Is(err, agentpki.ErrEnrollmentRejected):
		return status.Error(codes.Unauthenticated, agentpki.ErrEnrollmentRejected.Error())
	case errors.Is(err, agentpki.ErrCertificateRevoked):
		return status.Error(codes.Unauthenticated, agentpki.ErrCertificateRevoked.Error())
	case errors.Is(err, agentpki.ErrInvalidCertificate):
		return status.Error(codes.Unauthenticated, "the client certificate is not a valid agent certificate of this cluster")
	case errors.Is(err, modulepki.ErrInvalidRequest):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, modulepki.ErrNoAuthority):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		slog.Error("agent PKI failure", "component", "agent-pki", "error", err)
		return status.Error(codes.Internal, "agent PKI failure")
	}
}
