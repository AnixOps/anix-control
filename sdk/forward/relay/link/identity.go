package link

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// TrustDomain is the SPIFFE trust domain of every AnixOps identity.
const TrustDomain = "anixops"

// ErrInvalidIdentity reports a string or certificate that is not an AnixOps
// agent identity (spiffe://anixops/<cluster>/agent/<node>).
var ErrInvalidIdentity = errors.New("link: invalid agent identity")

var (
	clusterPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	// nodePattern is an Agent identity name: the kind and the node's id,
	// without leading zeros (node-ops-service.md D9).
	nodePattern = regexp.MustCompile(`^(proxy|forward)-[1-9][0-9]{0,9}$`)
)

// ParseIdentity validates a node identity as the planner writes it into
// peer_identity and ingress_peers and returns it unchanged. Only the
// canonical spelling is accepted, so the string comparisons made during the
// handshake cannot be fooled by an alternative form of the same URI.
func ParseIdentity(raw string) (string, error) {
	if _, err := identityNode(raw); err != nil {
		return "", err
	}
	return raw, nil
}

// identityNode returns the node name of a canonical agent identity.
func identityNode(raw string) (string, error) {
	bad := func() (string, error) { return "", fmt.Errorf("%w: %q", ErrInvalidIdentity, raw) }
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "spiffe" || u.Host != TrustDomain || u.User != nil || u.RawQuery != "" ||
		u.Fragment != "" || u.Opaque != "" || u.Port() != "" || u.ForceQuery {
		return bad()
	}
	segments := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(segments) != 3 || segments[1] != "agent" || !clusterPattern.MatchString(segments[0]) || !nodePattern.MatchString(segments[2]) {
		return bad()
	}
	if id, err := strconv.ParseUint(segments[2][strings.IndexByte(segments[2], '-')+1:], 10, 32); err != nil || id == 0 {
		return bad()
	}
	if "spiffe://"+TrustDomain+"/"+segments[0]+"/agent/"+segments[2] != raw {
		return bad()
	}
	return segments[2], nil
}

// certIdentity returns the SPIFFE identity and node name of a link
// certificate: exactly one URI SAN (an agent identity) and no IP or email
// names. The DNS names are the caller's to check.
func certIdentity(cert *x509.Certificate) (identity, node string, err error) {
	if cert == nil || len(cert.URIs) != 1 || len(cert.IPAddresses) != 0 || len(cert.EmailAddresses) != 0 {
		return "", "", fmt.Errorf("%w: a link certificate carries exactly one URI name and no IP or email names", ErrInvalidIdentity)
	}
	identity = cert.URIs[0].String()
	node, err = identityNode(identity)
	return identity, node, err
}
