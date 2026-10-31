package grpc

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/agenttransport"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/middleware"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/nodegroup"
	"google.golang.org/grpc"
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
	// onJoin is told of every committed join, in its own goroutine (the
	// administrators' notification); nil tells no one.
	onJoin func(nodegroup.Joined)
}

// Join rate limit per peer address: joins are rare (one per machine), so a
// peer that calls more often probes tokens or loops.
const (
	joinRatePerSecond = 0.5
	joinBurst         = 20
)

var (
	joinLimiterMu   sync.Mutex
	joinLimiterOnce sync.Once
	joinLimiter     = middleware.NewRateLimiter(joinRatePerSecond, joinBurst, middleware.WithTTL(10*time.Minute))
)

// allowJoin takes a token of peer's join bucket.
func allowJoin(peer string) bool {
	joinLimiterOnce.Do(func() { joinLimiter.StartCleanup(time.Minute) })
	joinLimiterMu.Lock()
	limiter := joinLimiter
	joinLimiterMu.Unlock()
	return limiter.Allow(peer)
}

// setJoinLimiter replaces the join limiter, for tests; it returns the
// previous one.
func setJoinLimiter(limiter *middleware.RateLimiter) *middleware.RateLimiter {
	joinLimiterMu.Lock()
	defer joinLimiterMu.Unlock()
	previous := joinLimiter
	joinLimiter = limiter
	return previous
}

var _ agentv1pb.AgentEnrollmentServer = (*AgentEnrollmentGRPCServer)(nil)

// NewAgentEnrollmentGRPCServer returns the enrollment service of auth's
// agent PKI. Without one, every call answers FailedPrecondition.
func NewAgentEnrollmentGRPCServer(auth *AgentAuthenticator) *AgentEnrollmentGRPCServer {
	return &AgentEnrollmentGRPCServer{auth: auth}
}

// WithJoinNotifier sets who is told of every committed join.
func (s *AgentEnrollmentGRPCServer) WithJoinNotifier(notify func(nodegroup.Joined)) *AgentEnrollmentGRPCServer {
	s.onJoin = notify
	return s
}

func (s *AgentEnrollmentGRPCServer) service() (*agentpki.Service, error) {
	if pki := s.auth.pki(); pki != nil {
		return pki, nil
	}
	return nil, status.Error(codes.FailedPrecondition, agentpki.ErrDisabled.Error())
}

// answer sets the x-anix-error-code trailer of a refused enrollment call
// (agent_refusals.go).
func answer[T any](ctx context.Context, response T, err error) (T, error) {
	setRefusalTrailer(err, func(md metadata.MD) { _ = grpc.SetTrailer(ctx, md) })
	return response, err
}

// Enroll issues a node's first certificate for a bootstrap credential: a
// one-time enrollment credential, or the node credential in metadata
// (refused when agent_control.mtls is required).
func (s *AgentEnrollmentGRPCServer) Enroll(ctx context.Context, request *agentv1pb.EnrollAgentRequest) (*agentv1pb.EnrollAgentResponse, error) {
	response, err := s.enroll(ctx, request)
	return answer(ctx, response, err)
}

func (s *AgentEnrollmentGRPCServer) enroll(ctx context.Context, request *agentv1pb.EnrollAgentRequest) (*agentv1pb.EnrollAgentResponse, error) {
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
			return agentpki.Bootstrap{}, refuseAgent(agentcontrol.ErrorCodeEnrollmentRejected, codes.Unauthenticated, "invalid x-node-id")
		}
		node = agentcontrol.AgentNode{Kind: kind, ID: uint32(id)}
		if !node.Valid() {
			return agentpki.Bootstrap{}, refuseAgent(agentcontrol.ErrorCodeEnrollmentRejected, codes.Unauthenticated, "invalid x-node-kind")
		}
	}
	if credential != "" {
		return agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Node: node, Secret: credential}, nil
	}
	if s.auth.mode() == config.AgentMTLSRequired {
		agenttransport.CountRefused(agentv1pb.AgentEnrollment_Enroll_FullMethodName)
		return agentpki.Bootstrap{}, refuseAgent(agentcontrol.ErrorCodeMTLSRequired, codes.Unauthenticated, "an enrollment credential is required (agent_control.mtls: required)")
	}
	secret := first(agentcontrol.MetadataAPIKey)
	if secret == "" || node == (agentcontrol.AgentNode{}) {
		return agentpki.Bootstrap{}, refuseAgent(agentcontrol.ErrorCodeEnrollmentRejected, codes.Unauthenticated, "an enrollment credential or x-node-id and x-api-key are required")
	}
	method := model.AgentEnrollmentMethodNodeAPIKey
	if node.Kind == agentcontrol.NodeKindForward {
		method = model.AgentEnrollmentMethodForwardToken
	}
	return agentpki.Bootstrap{Method: method, Node: node, Secret: secret}, nil
}

// Join makes the calling machine a node of the group whose join token it
// presents, or rejoins its earlier node (nodegroup.Join), and issues the
// node's certificate. It needs no client certificate. Every unusable token
// answers PermissionDenied with join_rejected and no detail.
func (s *AgentEnrollmentGRPCServer) Join(ctx context.Context, request *agentv1pb.JoinRequest) (*agentv1pb.JoinResponse, error) {
	response, err := s.join(ctx, request)
	return answer(ctx, response, err)
}

func (s *AgentEnrollmentGRPCServer) join(ctx context.Context, request *agentv1pb.JoinRequest) (*agentv1pb.JoinResponse, error) {
	pki, err := s.service()
	if err != nil {
		return nil, err
	}
	peer := remoteHost(ctx)
	if peer == "" && isEdgeConnection(ctx) {
		// The record and the rejoin rule need the node's address, and the rate
		// limit is per address: without it the edge proxy is misconfigured.
		return nil, status.Error(codes.FailedPrecondition, "the edge proxy sent no client address (x-forwarded-for)")
	}
	if !allowJoin(peer) {
		return nil, status.Error(codes.ResourceExhausted, "too many join attempts from this address; retry in a minute")
	}
	groups := &nodegroup.Service{DB: databaseForAgentChecks(), PKI: pki, OnJoin: s.onJoin}
	joined, err := groups.Join(ctx, nodegroup.JoinRequest{
		Token: request.GetJoinToken(), CSRDER: request.GetCsrDer(), MachineFingerprint: request.GetMachineFingerprint(),
		Hostname: request.GetHostname(), RequestedName: request.GetRequestedName(), ConnectHost: request.GetConnectHost(),
		PublicAddresses: request.GetPublicAddresses(), Arch: request.GetArch(), OS: request.GetOs(),
		AgentVersion: request.GetAgentVersion(), InstanceID: request.GetInstanceId(), Reset: request.GetReset_(), PeerAddress: peer,
	})
	if err != nil {
		switch {
		case errors.Is(err, nodegroup.ErrJoinRejected):
			slog.Warn("agent join rejected", "component", "agent-pki", "addr", peer)
			return nil, refuseAgent(agentcontrol.ErrorCodeJoinRejected, codes.PermissionDenied, "the join token is not usable")
		case errors.Is(err, nodegroup.ErrInvalidJoin), errors.Is(err, kernelforward.ErrInvalidRequest):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, agentPKIStatus(err)
	}
	slog.Info("agent joined", "component", "agent-pki", "node", joined.Node.String(), "group", joined.GroupName,
		"created", joined.Created, "approval", joined.Approval, "serial", joined.Issued.Serial, "addr", peer)
	return &agentv1pb.JoinResponse{
		Certificate: agentCertificateMessage(joined.Issued), Node: joined.Node.String(), NodeName: joined.NodeName,
		Group: joined.GroupName, Created: joined.Created, Approval: joined.Approval,
	}, nil
}

// Renew issues a new certificate to the holder of a valid one.
func (s *AgentEnrollmentGRPCServer) Renew(ctx context.Context, request *agentv1pb.RenewAgentCertificateRequest) (*agentv1pb.RenewAgentCertificateResponse, error) {
	response, err := s.renew(ctx, request)
	return answer(ctx, response, err)
}

func (s *AgentEnrollmentGRPCServer) renew(ctx context.Context, request *agentv1pb.RenewAgentCertificateRequest) (*agentv1pb.RenewAgentCertificateResponse, error) {
	pki, err := s.service()
	if err != nil {
		return nil, err
	}
	chain := peerCertificateChain(ctx)
	if len(chain) == 0 {
		return nil, refuseAgent(agentcontrol.ErrorCodeCertInvalid, codes.Unauthenticated, "renewal requires the current agent client certificate")
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
func (s *AgentEnrollmentGRPCServer) GetTrustBundle(ctx context.Context, request *agentv1pb.GetAgentTrustBundleRequest) (*agentv1pb.GetAgentTrustBundleResponse, error) {
	response, err := s.getTrustBundle(ctx, request)
	return answer(ctx, response, err)
}

func (s *AgentEnrollmentGRPCServer) getTrustBundle(ctx context.Context, _ *agentv1pb.GetAgentTrustBundleRequest) (*agentv1pb.GetAgentTrustBundleResponse, error) {
	pki, err := s.service()
	if err != nil {
		return nil, err
	}
	if _, ok, err := s.auth.certificatePrincipal(ctx); err != nil {
		return nil, err
	} else if !ok {
		return nil, refuseAgent(agentcontrol.ErrorCodeCertInvalid, codes.Unauthenticated, "an agent client certificate is required")
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
// that would let a caller probe credentials. Refusals carry their error
// code (agent_refusals.go).
func agentPKIStatus(err error) error {
	switch {
	case status.Code(err) != codes.Unknown:
		return err
	case errors.Is(err, agentpki.ErrEnrollmentRejected):
		return refuseAgent(agentcontrol.ErrorCodeEnrollmentRejected, codes.Unauthenticated, agentpki.ErrEnrollmentRejected.Error())
	case errors.Is(err, agentpki.ErrCertificateRevoked), errors.Is(err, agentpki.ErrInvalidCertificate):
		return certificateRefusal(err)
	case errors.Is(err, modulepki.ErrInvalidRequest):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, modulepki.ErrNoAuthority):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		slog.Error("agent PKI failure", "component", "agent-pki", "error", err)
		return status.Error(codes.Internal, "agent PKI failure")
	}
}
