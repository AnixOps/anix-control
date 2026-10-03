package grpc

import (
	"errors"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// agentRefusal is a refused Agent call with a machine-readable reason, one
// of the agentcontrol.ErrorCode* codes. The caller of the refusing RPC
// writes the code into the x-anix-error-code trailer (setRefusalTrailer),
// once, with the refusal's other trailer metadata. The status message
// starts with the code, as agent_mtls_required's always did.
type agentRefusal struct {
	code    string
	status  *status.Status
	trailer metadata.MD
}

// refuseAgent returns a refusal with code, answered as grpcCode.
func refuseAgent(code string, grpcCode codes.Code, message string) error {
	return &agentRefusal{code: code, status: status.New(grpcCode, code+": "+message)}
}

func (e *agentRefusal) Error() string { return e.status.Err().Error() }

// GRPCStatus makes the refusal the call's status.
func (e *agentRefusal) GRPCStatus() *status.Status { return e.status }

// refusalCode returns the code of a refusal, empty for any other error.
func refusalCode(err error) string {
	var refusal *agentRefusal
	if errors.As(err, &refusal) {
		return refusal.code
	}
	return ""
}

// setRefusalTrailer writes the trailer of a refusal: its metadata and
// x-anix-error-code. Other errors set nothing.
func setRefusalTrailer(err error, set func(metadata.MD)) {
	var refusal *agentRefusal
	if !errors.As(err, &refusal) {
		return
	}
	trailer := refusal.trailer.Copy()
	if trailer == nil {
		trailer = metadata.MD{}
	}
	trailer.Set(agentcontrol.MetadataErrorCode, refusal.code)
	set(trailer)
}

// certificateRefusal maps an agent PKI verification error to its refusal:
// revoked, expired, of another cluster or otherwise invalid. Other errors
// (a database failure) are returned as they are.
func certificateRefusal(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, agentpki.ErrCertificateRevoked):
		return refuseAgent(agentcontrol.ErrorCodeCertRevoked, codes.Unauthenticated, "agent client certificate revoked")
	case errors.Is(err, agentpki.ErrCertificateExpired):
		return refuseAgent(agentcontrol.ErrorCodeCertExpired, codes.Unauthenticated, "agent client certificate expired")
	case errors.Is(err, agentpki.ErrCertificateWrongCluster):
		return refuseAgent(agentcontrol.ErrorCodeCertWrongCluster, codes.Unauthenticated, "the client certificate is an agent certificate of another cluster")
	case errors.Is(err, agentpki.ErrInvalidCertificate):
		return refuseAgent(agentcontrol.ErrorCodeCertInvalid, codes.Unauthenticated, "the client certificate is not a valid agent certificate of this cluster")
	}
	return err
}
