package grpc

import (
	"context"
	"crypto/x509"
	"errors"
	"log/slog"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Forward link certificates (owner decision H28; PROTOCOL.md "Forward link
// certificates", internal/agentpki/link.go). Both RPCs authenticate with the
// agent client certificate only: a node credential, which anyone holding
// the node's API key or token has, never gets a link certificate.

// IssueLinkCertificate issues the certificate's node a forward link
// certificate.
func (s *AgentEnrollmentGRPCServer) IssueLinkCertificate(ctx context.Context, request *agentv1pb.IssueLinkCertificateRequest) (*agentv1pb.IssueLinkCertificateResponse, error) {
	response, err := s.issueLinkCertificate(ctx, request)
	return answer(ctx, response, err)
}

func (s *AgentEnrollmentGRPCServer) issueLinkCertificate(ctx context.Context, request *agentv1pb.IssueLinkCertificateRequest) (*agentv1pb.IssueLinkCertificateResponse, error) {
	pki, err := s.linkService()
	if err != nil {
		return nil, err
	}
	leaf, err := s.agentLeaf(ctx)
	if err != nil {
		return nil, err
	}
	issued, err := pki.IssueLinkCertificate(ctx, leaf, request.GetCsrDer(), remoteHost(ctx))
	if err != nil {
		return nil, linkPKIStatus(err)
	}
	slog.Info("forward link certificate issued", "component", "agent-pki", "node", issued.Identity.Node.String(),
		"serial", issued.Serial, "not_after", issued.NotAfter)
	return &agentv1pb.IssueLinkCertificateResponse{Certificate: &agentv1pb.LinkCertificate{
		CertificateDer: issued.CertificateDER, TrustBundleDer: issued.TrustBundleDER, SpiffeId: issued.Identity.String(),
		Node: issued.Identity.Node.String(), DnsName: issued.DNSName, Serial: issued.Serial,
		NotAfterUnix: issued.NotAfter.Unix(), RenewAfterUnix: issued.RenewAfter.Unix(),
	}}, nil
}

// GetLinkTrustBundle returns the forward link CAs to an agent that holds a
// certificate.
func (s *AgentEnrollmentGRPCServer) GetLinkTrustBundle(ctx context.Context, request *agentv1pb.GetLinkTrustBundleRequest) (*agentv1pb.GetLinkTrustBundleResponse, error) {
	response, err := s.getLinkTrustBundle(ctx, request)
	return answer(ctx, response, err)
}

func (s *AgentEnrollmentGRPCServer) getLinkTrustBundle(ctx context.Context, _ *agentv1pb.GetLinkTrustBundleRequest) (*agentv1pb.GetLinkTrustBundleResponse, error) {
	pki, err := s.linkService()
	if err != nil {
		return nil, err
	}
	if _, err := s.agentLeaf(ctx); err != nil {
		return nil, err
	}
	bundle, err := pki.LinkTrustBundle(ctx)
	if err != nil {
		return nil, linkPKIStatus(err)
	}
	response := &agentv1pb.GetLinkTrustBundleResponse{}
	for _, certificate := range bundle {
		response.TrustBundleDer = append(response.TrustBundleDer, certificate.Raw)
	}
	return response, nil
}

// linkService is the agent PKI when it serves link certificates.
func (s *AgentEnrollmentGRPCServer) linkService() (*agentpki.Service, error) {
	pki := s.auth.pki()
	if pki == nil || pki.Link() == nil {
		return nil, refuseAgent(agentcontrol.ErrorCodeLinkUnavailable, codes.FailedPrecondition, agentpki.ErrLinkDisabled.Error())
	}
	return pki, nil
}

// agentLeaf verifies the call's agent client certificate, refusing a call
// without one, and returns its leaf.
func (s *AgentEnrollmentGRPCServer) agentLeaf(ctx context.Context) (*x509.Certificate, error) {
	_, ok, err := s.auth.certificatePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	chain := peerCertificateChain(ctx)
	if !ok || len(chain) == 0 {
		return nil, refuseAgent(agentcontrol.ErrorCodeCertInvalid, codes.Unauthenticated, "an agent client certificate is required")
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return nil, refuseAgent(agentcontrol.ErrorCodeCertInvalid, codes.Unauthenticated, "the client certificate is not a valid agent certificate of this cluster")
	}
	return leaf, nil
}

// linkPKIStatus maps link issuance errors to refusals; the agent
// certificate's errors keep the agent PKI's codes.
func linkPKIStatus(err error) error {
	switch {
	case status.Code(err) != codes.Unknown:
		return err
	case errors.Is(err, agentpki.ErrLinkNotNegotiated):
		return refuseAgent(agentcontrol.ErrorCodeLinkNotNegotiated, codes.FailedPrecondition, err.Error())
	case errors.Is(err, agentpki.ErrInvalidLinkRequest):
		return refuseAgent(agentcontrol.ErrorCodeLinkRequestInvalid, codes.InvalidArgument, err.Error())
	case errors.Is(err, agentpki.ErrLinkDisabled), errors.Is(err, agentpki.ErrNoLinkAuthority):
		return refuseAgent(agentcontrol.ErrorCodeLinkUnavailable, codes.FailedPrecondition, err.Error())
	default:
		return agentPKIStatus(err)
	}
}
