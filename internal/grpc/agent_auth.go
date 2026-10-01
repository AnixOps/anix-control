package grpc

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// agentServicePrefix starts the full method names of the Agent services
// (AgentControlService, AgentEnrollment), which authenticate in their
// handlers.
const agentServicePrefix = "/anix.agent.v1."

// legacyAuthDeprecation is the value of the deprecation header answered to
// a legacy node credential in agent_control.mtls: preferred.
const legacyAuthDeprecation = "node API key authentication is deprecated; enroll with anix.agent.v1.AgentEnrollment and present the client certificate"

// AgentAuthenticator authenticates AnixOps Agents on the node-facing
// listener: by client certificate (agent PKI) or by the legacy node
// credential, as agent_control.mtls allows. The zero value and nil accept
// legacy credentials only and refuse client certificates, which nothing
// could verify.
type AgentAuthenticator struct {
	// PKI verifies client certificates and serves AgentEnrollment; nil when
	// the built-in module PKI is off.
	PKI *agentpki.Service
	// Mode is agent_control.mtls; empty means optional.
	Mode string
}

func (a *AgentAuthenticator) mode() string {
	if a == nil {
		return config.AgentMTLSOptional
	}
	return config.AgentControlConfig{MTLS: a.Mode}.MTLSOrDefault()
}

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
		return status.Error(codes.Unauthenticated, "agent client certificate expired")
	}
	revoked, err := pki.IsRevoked(ctx, principal.Serial, principal.Node)
	if err != nil {
		return nil // keep the stream through a transient database error
	}
	if revoked {
		return status.Error(codes.Unauthenticated, "agent client certificate revoked")
	}
	return nil
}

// peerCertificateChain returns the raw client certificate chain of the
// call's TLS connection, nil when the client presented none.
func peerCertificateChain(ctx context.Context) [][]byte {
	remote, ok := peer.FromContext(ctx)
	if !ok || remote.AuthInfo == nil {
		return nil
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
// error when the certificate does not authenticate an agent: an invalid,
// expired, revoked or foreign certificate never falls back to legacy
// credentials. A node id in the metadata must name the certificate's node.
func (a *AgentAuthenticator) certificatePrincipal(ctx context.Context) (agentPrincipal, bool, error) {
	chain := peerCertificateChain(ctx)
	if len(chain) == 0 {
		return agentPrincipal{}, false, nil
	}
	pki := a.pki()
	if pki == nil {
		return agentPrincipal{}, false, status.Error(codes.Unauthenticated, "agent client certificates are not accepted: the agent PKI is not enabled")
	}
	identity, leaf, err := pki.VerifyPeer(ctx, chain)
	switch {
	case errors.Is(err, agentpki.ErrCertificateRevoked):
		return agentPrincipal{}, false, status.Error(codes.Unauthenticated, "agent client certificate revoked")
	case errors.Is(err, agentpki.ErrInvalidCertificate):
		return agentPrincipal{}, false, status.Error(codes.Unauthenticated, "the client certificate is not a valid agent certificate of this cluster")
	case err != nil:
		return agentPrincipal{}, false, status.Error(codes.Unavailable, "agent certificate check failed")
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if ids := md.Get(agentcontrol.MetadataNodeID); len(ids) > 0 && strings.TrimSpace(ids[0]) != "" &&
			strings.TrimSpace(ids[0]) != strconv.FormatUint(uint64(identity.Node.ID), 10) {
			return agentPrincipal{}, false, status.Error(codes.Unauthenticated, "x-node-id does not match the client certificate")
		}
		if kinds := md.Get(agentcontrol.MetadataNodeKind); len(kinds) > 0 && strings.TrimSpace(kinds[0]) != "" &&
			strings.TrimSpace(kinds[0]) != identity.Node.Kind {
			return agentPrincipal{}, false, status.Error(codes.Unauthenticated, "x-node-kind does not match the client certificate")
		}
	}
	return agentPrincipal{
		Node: identity.Node, Certificate: true, Serial: modulepki.SerialString(leaf.SerialNumber), NotAfter: leaf.NotAfter,
		SPIFFEID: identity.String(),
	}, true, nil
}

// deprecationHeader is the deprecation header of a legacy node
// credential in agent_control.mtls: preferred.
func (a *AgentAuthenticator) deprecationHeader() metadata.MD {
	if a.mode() != config.AgentMTLSPreferred {
		return nil
	}
	return metadata.Pairs(agentcontrol.MetadataAuthDeprecated, legacyAuthDeprecation)
}

// authenticateControlStream authenticates an AgentControlService stream:
// a proxy or forward node by certificate, or a proxy node by its API key
// unless agent_control.mtls is required. Disabled and deleted nodes are
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
				return agentPrincipal{}, status.Error(codes.PermissionDenied, "node is disabled or no longer exists")
			}
			return agentPrincipal{}, status.Error(codes.Unavailable, "node check failed")
		}
		return principal, nil
	}
	if a.mode() == config.AgentMTLSRequired {
		return agentPrincipal{}, status.Error(codes.Unauthenticated, "an agent client certificate is required (agent_control.mtls: required)")
	}
	nodeID, err := authenticatedStreamNodeID(ctx)
	if err != nil {
		return agentPrincipal{}, err
	}
	if header := a.deprecationHeader(); header != nil {
		if err := stream.SetHeader(header); err != nil {
			return agentPrincipal{}, err
		}
	}
	return agentPrincipal{Node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: nodeID}}, nil
}

// remoteHost returns the client's address without the port.
func remoteHost(ctx context.Context) string {
	remote, ok := peer.FromContext(ctx)
	if !ok || remote.Addr == nil {
		return ""
	}
	address := remote.Addr.String()
	if host, _, err := net.SplitHostPort(address); err == nil {
		return host
	}
	return address
}
