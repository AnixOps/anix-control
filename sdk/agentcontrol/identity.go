package agentcontrol

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Metadata keys an Agent sends on Control's agent listener.
const (
	// MetadataNodeID carries the node id with a node credential.
	MetadataNodeID = "x-node-id"
	// MetadataAPIKey carries a proxy node's API key, or a forward node's
	// token when MetadataNodeKind is NodeKindForward.
	MetadataAPIKey = "x-api-key"
	// MetadataNodeKind names the table of MetadataNodeID: NodeKindProxy (the
	// default) or NodeKindForward. Only AgentEnrollment.Enroll reads it.
	MetadataNodeKind = "x-node-kind"
	// MetadataAuthDeprecated is the response header Control sends when an
	// Agent authenticated with its legacy node credential although client
	// certificates are preferred (agent_control.mtls: preferred).
	MetadataAuthDeprecated = "x-anix-auth-deprecated"
	// MetadataAuthSunset accompanies MetadataAuthDeprecated when Control
	// announces a date (agent_control.legacy_sunset) after which legacy
	// credentials may be refused, as an HTTP-date (RFC 9110).
	MetadataAuthSunset = "x-anix-auth-sunset"
	// MetadataAuthDeprecationLink accompanies MetadataAuthDeprecated: where
	// the operator reads how to move the node to client certificates.
	MetadataAuthDeprecationLink = "x-anix-auth-deprecation-link"
	// MetadataErrorCode is the trailer of a call Control refused for a
	// machine-readable reason, such as ErrorCodeMTLSRequired.
	MetadataErrorCode = "x-anix-error-code"
	// ErrorCodeMTLSRequired refuses legacy credentials on an AnixOps Agent
	// channel under agent_control.mtls: required. HTTP answers carry it as
	// the "code" of their JSON body.
	ErrorCodeMTLSRequired = "agent_mtls_required"
)

// Node kinds of an agent identity.
const (
	// NodeKindProxy is a proxy node, a v2_node row.
	NodeKindProxy = "proxy"
	// NodeKindForward is a forward node, a v2_forward_node row.
	NodeKindForward = "forward"
)

// EnrollmentCredentialPrefix starts every one-time agent enrollment
// credential, so secret scanners can recognize them.
const EnrollmentCredentialPrefix = "anixagt_"

// spiffeTrustDomain is the SPIFFE trust domain of every AnixOps identity.
const spiffeTrustDomain = "anixops"

var (
	// ErrInvalidAgentIdentity reports a node name, SPIFFE ID or certificate
	// that is not an AnixOps agent identity.
	ErrInvalidAgentIdentity = errors.New("invalid AnixOps agent identity")

	agentClusterPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

// AgentNode names the node an agent runs on. Proxy and forward node ids
// overlap, so the kind is part of the name.
type AgentNode struct {
	Kind string
	ID   uint32
}

// String returns the node name, "proxy-<id>" or "forward-<id>".
func (n AgentNode) String() string {
	return n.Kind + "-" + strconv.FormatUint(uint64(n.ID), 10)
}

// Valid reports whether n names a node.
func (n AgentNode) Valid() bool {
	return (n.Kind == NodeKindProxy || n.Kind == NodeKindForward) && n.ID > 0
}

// ParseAgentNode parses "proxy-<id>" or "forward-<id>".
func ParseAgentNode(name string) (AgentNode, error) {
	kind, rawID, ok := strings.Cut(name, "-")
	if !ok || rawID == "" || rawID[0] == '0' || strings.TrimLeft(rawID, "0123456789") != "" {
		return AgentNode{}, fmt.Errorf("%w: node %q", ErrInvalidAgentIdentity, name)
	}
	id, err := strconv.ParseUint(rawID, 10, 32)
	node := AgentNode{Kind: kind, ID: uint32(id)}
	if err != nil || !node.Valid() {
		return AgentNode{}, fmt.Errorf("%w: node %q", ErrInvalidAgentIdentity, name)
	}
	return node, nil
}

// AgentIdentity is a parsed agent SPIFFE ID,
// spiffe://anixops/<cluster>/agent/<node>.
type AgentIdentity struct {
	Cluster string
	Node    AgentNode
}

// String returns the SPIFFE ID.
func (id AgentIdentity) String() string {
	return "spiffe://" + spiffeTrustDomain + "/" + id.Cluster + "/agent/" + id.Node.String()
}

// URL returns the SPIFFE ID as a URI SAN.
func (id AgentIdentity) URL() *url.URL {
	return &url.URL{Scheme: "spiffe", Host: spiffeTrustDomain, Path: "/" + id.Cluster + "/agent/" + id.Node.String()}
}

// NewAgentIdentity returns the identity of node in cluster.
func NewAgentIdentity(cluster string, node AgentNode) (AgentIdentity, error) {
	if !agentClusterPattern.MatchString(cluster) {
		return AgentIdentity{}, fmt.Errorf("%w: cluster %q", ErrInvalidAgentIdentity, cluster)
	}
	if !node.Valid() {
		return AgentIdentity{}, fmt.Errorf("%w: node %q", ErrInvalidAgentIdentity, node.String())
	}
	return AgentIdentity{Cluster: cluster, Node: node}, nil
}

// ParseAgentIdentity parses an agent SPIFFE ID. Module and kernel identities
// of the same trust domain are not agent identities.
func ParseAgentIdentity(raw string) (AgentIdentity, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "spiffe" || parsed.Host != spiffeTrustDomain || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Port() != "" || parsed.Opaque != "" {
		return AgentIdentity{}, fmt.Errorf("%w: %q", ErrInvalidAgentIdentity, raw)
	}
	segments := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	if len(segments) != 3 || segments[1] != "agent" {
		return AgentIdentity{}, fmt.Errorf("%w: %q", ErrInvalidAgentIdentity, raw)
	}
	node, err := ParseAgentNode(segments[2])
	if err != nil {
		return AgentIdentity{}, err
	}
	identity, err := NewAgentIdentity(segments[0], node)
	if err != nil {
		return AgentIdentity{}, err
	}
	if identity.String() != raw {
		return AgentIdentity{}, fmt.Errorf("%w: %q is not canonical", ErrInvalidAgentIdentity, raw)
	}
	return identity, nil
}

// AgentIdentityFromCertificate returns the identity of a certificate that
// carries exactly one URI SAN, an agent SPIFFE ID, and no other SAN kinds.
func AgentIdentityFromCertificate(certificate *x509.Certificate) (AgentIdentity, error) {
	if certificate == nil || len(certificate.URIs) != 1 || len(certificate.DNSNames) != 0 ||
		len(certificate.IPAddresses) != 0 || len(certificate.EmailAddresses) != 0 {
		return AgentIdentity{}, fmt.Errorf("%w: certificate must carry exactly one URI SAN", ErrInvalidAgentIdentity)
	}
	return ParseAgentIdentity(certificate.URIs[0].String())
}
