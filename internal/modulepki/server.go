package modulepki

import (
	"context"
	"crypto/x509"
	"errors"

	modulepkiv1 "github.com/AnixOps/anix-control/sdk/api/modulepki/v1"
	"github.com/AnixOps/anix-control/sdk/moduletls"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Server serves ModulePKI on the kernel's module listener. The listener
// verifies client certificates during the handshake but accepts clients
// without one, because Enroll comes before the first certificate.
type Server struct {
	modulepkiv1.UnimplementedModulePKIServer
	Authority *Authority
}

var _ modulepkiv1.ModulePKIServer = (*Server)(nil)

func (s *Server) Enroll(ctx context.Context, request *modulepkiv1.EnrollRequest) (*modulepkiv1.EnrollResponse, error) {
	if s == nil || s.Authority == nil {
		return nil, status.Error(codes.FailedPrecondition, "the kernel does not issue module certificates (external PKI)")
	}
	issued, err := s.Authority.Enroll(ctx, request.GetEnrollmentCredential(), request.GetPackageId(), request.GetCluster(), request.GetCsrDer())
	if err != nil {
		return nil, statusError(err)
	}
	return &modulepkiv1.EnrollResponse{Certificate: issuedMessage(issued)}, nil
}

func (s *Server) Renew(ctx context.Context, request *modulepkiv1.RenewRequest) (*modulepkiv1.RenewResponse, error) {
	if s == nil || s.Authority == nil {
		return nil, status.Error(codes.FailedPrecondition, "the kernel does not issue module certificates (external PKI)")
	}
	certificate, err := PeerCertificate(ctx)
	if err != nil {
		return nil, err
	}
	issued, err := s.Authority.Renew(ctx, certificate, request.GetCsrDer())
	if err != nil {
		return nil, statusError(err)
	}
	return &modulepkiv1.RenewResponse{Certificate: issuedMessage(issued)}, nil
}

func (s *Server) GetTrustBundle(ctx context.Context, _ *modulepkiv1.GetTrustBundleRequest) (*modulepkiv1.GetTrustBundleResponse, error) {
	if s == nil || s.Authority == nil {
		return nil, status.Error(codes.FailedPrecondition, "the kernel does not issue module certificates (external PKI)")
	}
	if _, err := PeerCertificate(ctx); err != nil {
		return nil, err
	}
	bundle, err := s.Authority.TrustBundle(ctx)
	if err != nil {
		return nil, statusError(err)
	}
	response := &modulepkiv1.GetTrustBundleResponse{}
	for _, certificate := range bundle {
		response.TrustBundleDer = append(response.TrustBundleDer, certificate.Raw)
	}
	return response, nil
}

// PeerCertificate returns the verified client certificate of a gRPC call, or
// Unauthenticated when the client presented none.
func PeerCertificate(ctx context.Context) (*x509.Certificate, error) {
	remote, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no transport peer")
	}
	info, ok := remote.AuthInfo.(credentials.TLSInfo)
	if !ok || len(info.State.PeerCertificates) == 0 {
		return nil, status.Error(codes.Unauthenticated, "a module client certificate is required")
	}
	return info.State.PeerCertificates[0], nil
}

func issuedMessage(issued Issued) *modulepkiv1.IssuedCertificate {
	return &modulepkiv1.IssuedCertificate{
		CertificateDer: issued.CertificateDER, TrustBundleDer: issued.TrustBundleDER,
		SpiffeId: issued.Identity.String(), NotAfterUnix: issued.NotAfter.Unix(), RenewAfterUnix: issued.RenewAfter.Unix(),
	}
}

func statusError(err error) error {
	switch {
	case errors.Is(err, ErrEnrollmentRejected):
		return status.Error(codes.PermissionDenied, ErrEnrollmentRejected.Error())
	case errors.Is(err, ErrCertificateRevoked):
		return status.Error(codes.PermissionDenied, ErrCertificateRevoked.Error())
	case errors.Is(err, moduletls.ErrUnexpectedPeer), errors.Is(err, moduletls.ErrInvalidIdentity):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, ErrInvalidRequest):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ErrNoAuthority):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, "module PKI failure")
	}
}
