package grpc

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/agentstreams"
	"github.com/AnixOps/anix-control/v5/internal/agenttransport"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// AgentAuthenticator authenticates AnixOps Agents on the node-facing
// listener by client certificate (agent PKI): agent_control.mtls is
// required, built in. The zero value and nil refuse every agent stream,
// since nothing could verify a certificate.
type AgentAuthenticator struct {
	// PKI verifies client certificates and serves AgentEnrollment; nil when
	// the built-in module PKI is off.
	PKI *agentpki.Service

	// identify replaces the certificate check; tests in this package set it
	// to run Agent Control streams over a plaintext connection.
	identify func(ctx context.Context) (agentPrincipal, bool, error)
}

// pki is the agent PKI the listener verifies certificates with.
func (a *AgentAuthenticator) pki() *agentpki.Service {
	if a == nil {
		return nil
	}
	return a.PKI
}

// agentPrincipal is an authenticated agent.
type agentPrincipal struct {
	Node agentcontrol.AgentNode
	// Certificate is true when a client certificate authenticated the call;
	// Serial, NotAfter and SPIFFEID describe it.
	Certificate bool
	Serial      string
	NotAfter    time.Time
	SPIFFEID    string
}

// identity names how the agent authenticated: its SPIFFE ID, or the node
// key (agentstreams.IdentityAPIKey). Never the credential itself.
func (p agentPrincipal) identity() string {
	if p.Certificate && p.SPIFFEID != "" {
		return p.SPIFFEID
	}
	return agentstreams.IdentityAPIKey
}

// recheck tells whether a long-lived stream authenticated by certificate
// may go on: the certificate must not have expired or been revoked since
// (revocation answers are cached for at most 30 s).
func (a *AgentAuthenticator) recheck(ctx context.Context, principal agentPrincipal) error {
	pki := a.pki()
	if !principal.Certificate || pki == nil {
		return nil
	}
	if !time.Now().Before(principal.NotAfter) {
		return refuseAgent(agentcontrol.ErrorCodeCertExpired, codes.Unauthenticated, "agent client certificate expired")
	}
	revoked, err := pki.IsRevoked(ctx, principal.Serial, principal.Node)
	if err != nil {
		return nil // keep the stream through a transient database error
	}
	if revoked {
		return refuseAgent(agentcontrol.ErrorCodeCertRevoked, codes.Unauthenticated, "agent client certificate revoked")
	}
	return nil
}

// peerCertificateChain returns the raw client certificate chain of the
// call's TLS connection, or the one an edge proxy vouched for
// (EdgeClientCertHeader); nil when the client presented none.
func peerCertificateChain(ctx context.Context) [][]byte {
	remote, ok := peer.FromContext(ctx)
	if !ok || remote.AuthInfo == nil {
		return nil
	}
	if isEdgeConnection(ctx) {
		return edgeClientChain(ctx)
	}
	info, ok := remote.AuthInfo.(credentials.TLSInfo)
	if !ok || len(info.State.PeerCertificates) == 0 {
		return nil
	}
	chain := make([][]byte, 0, len(info.State.PeerCertificates))
	for _, certificate := range info.State.PeerCertificates {
		chain = append(chain, certificate.Raw)
	}
	return chain
}

// certificatePrincipal verifies the client certificate of the call. It
// answers ok=false when the client presented none, and an Unauthenticated
// refusal when the certificate does not authenticate an agent: an invalid,
// expired, revoked or foreign certificate never falls back to legacy
// credentials. A node id in the metadata must name the certificate's node.
// Each refusal carries its agentcontrol error code (agent_refusals.go); a
// failed check (Unavailable) carries none.
func (a *AgentAuthenticator) certificatePrincipal(ctx context.Context) (agentPrincipal, bool, error) {
	if a != nil && a.identify != nil {
		return a.identify(ctx)
	}
	chain := peerCertificateChain(ctx)
	if len(chain) == 0 {
		return agentPrincipal{}, false, nil
	}
	pki := a.pki()
	if pki == nil {
		return agentPrincipal{}, false, refuseAgent(agentcontrol.ErrorCodeCertInvalid, codes.Unauthenticated, "agent client certificates are not accepted: the agent PKI is not enabled")
	}
	identity, leaf, err := pki.VerifyPeer(ctx, chain)
	if refusal := certificateRefusal(err); refusal != nil {
		if refusalCode(refusal) == "" {
			return agentPrincipal{}, false, status.Error(codes.Unavailable, "agent certificate check failed")
		}
		return agentPrincipal{}, false, refusal
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if ids := md.Get(agentcontrol.MetadataNodeID); len(ids) > 0 && strings.TrimSpace(ids[0]) != "" &&
			strings.TrimSpace(ids[0]) != strconv.FormatUint(uint64(identity.Node.ID), 10) {
			return agentPrincipal{}, false, refuseAgent(agentcontrol.ErrorCodeCertWrongNode, codes.Unauthenticated, "x-node-id does not match the client certificate")
		}
		if kinds := md.Get(agentcontrol.MetadataNodeKind); len(kinds) > 0 && strings.TrimSpace(kinds[0]) != "" &&
			strings.TrimSpace(kinds[0]) != identity.Node.Kind {
			return agentPrincipal{}, false, refuseAgent(agentcontrol.ErrorCodeCertWrongNode, codes.Unauthenticated, "x-node-kind does not match the client certificate")
		}
	}
	return agentPrincipal{
		Node: identity.Node, Certificate: true, Serial: modulepki.SerialString(leaf.SerialNumber), NotAfter: leaf.NotAfter,
		SPIFFEID: identity.String(),
	}, true, nil
}

// refuseUncertified answers an agent stream without a client certificate:
// counted, and refused with the mTLS-required error code.
func (a *AgentAuthenticator) refuseUncertified(method string) error {
	agenttransport.CountRefused(method)
	return refuseAgent(agentcontrol.ErrorCodeMTLSRequired, codes.Unauthenticated,
		"an agent client certificate is required (agent_control.mtls: required)")
}

// authenticateControlStream authenticates an AgentControlService stream:
// a proxy or forward node by its certificate. Disabled and deleted nodes are
// refused.
func (a *AgentAuthenticator) authenticateControlStream(stream grpc.ServerStream) (agentPrincipal, error) {
	ctx := stream.Context()
	principal, ok, err := a.certificatePrincipal(ctx)
	if err != nil {
		return agentPrincipal{}, err
	}
	if ok {
		if err := agentpki.CheckNodeEnabled(ctx, databaseForAgentChecks(), principal.Node); err != nil {
			if errors.Is(err, agentpki.ErrInvalidNode) {
				// Disabling or deleting a node revokes its certificates;
				// this check closes the revocation cache's window.
				return agentPrincipal{}, refuseAgent(agentcontrol.ErrorCodeCertRevoked, codes.PermissionDenied, "node is disabled or no longer exists")
			}
			return agentPrincipal{}, status.Error(codes.Unavailable, "node check failed")
		}
		return principal, nil
	}
	return agentPrincipal{}, a.refuseUncertified(agentv1pb.AgentControlService_ControlStream_FullMethodName)
}

// authentication names how principal authenticated
// (agentstreams.Authentication*).
func (p agentPrincipal) authentication() string {
	if p.Certificate {
		return agentstreams.AuthenticationMTLS
	}
	return agentstreams.AuthenticationAPIKey
}

// certificate describes principal's client certificate, nil without one.
func (p agentPrincipal) certificate() *agentstreams.SessionCertificate {
	if !p.Certificate {
		return nil
	}
	return &agentstreams.SessionCertificate{Serial: p.Serial, NotAfter: p.NotAfter, SPIFFEID: p.SPIFFEID}
}

// transport names how principal reached the stream, for the transport
// inventory.
func (p agentPrincipal) transport() string {
	if p.Certificate {
		return model.AgentTransportMTLSStream
	}
	return model.AgentTransportAPIKeyStream
}

// sighting is the inventory record of principal on the stream.
func (p agentPrincipal) sighting(agentVersion string) agenttransport.Sighting {
	sighting := agenttransport.Sighting{
		Node: p.Node, Transport: p.transport(), AgentVersion: agentVersion, Identity: p.identity(),
	}
	if p.Certificate {
		notAfter := p.NotAfter
		sighting.CertSerial, sighting.CertNotAfter = p.Serial, &notAfter
	}
	return sighting
}

// remoteHost returns the client's address without the port. Through an
// edge proxy the peer is the proxy, so the node's address is the one the
// proxy vouched for (X-Forwarded-For).
func remoteHost(ctx context.Context) string {
	remote, ok := peer.FromContext(ctx)
	if !ok || remote.Addr == nil {
		return ""
	}
	if isEdgeConnection(ctx) {
		return edgeClientAddress(ctx)
	}
	address := remote.Addr.String()
	if host, _, err := net.SplitHostPort(address); err == nil {
		return host
	}
	return address
}
