// Package moduletls holds the SPIFFE identity rules of the Control kernel CA.
//
// Every certificate the kernel CA signs carries exactly one URI SAN in the
// AnixOps trust domain, such as
//
//	spiffe://anixops/<cluster>/kernel   the kernel
//
// Agent identities live in the same trust domain (internal/agentpki).
package moduletls

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
)

// TrustDomain is the SPIFFE trust domain of every AnixOps identity.
const TrustDomain = "anixops"

// KindKernel is the kind of the kernel identity.
const KindKernel = "kernel"

var (
	// ErrInvalidIdentity reports a name that cannot form an AnixOps SPIFFE
	// identity.
	ErrInvalidIdentity = errors.New("invalid AnixOps identity")

	clusterPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

// Identity is an AnixOps SPIFFE identity.
type Identity struct {
	Cluster string
	Kind    string
}

// String returns the SPIFFE ID.
func (id Identity) String() string {
	return "spiffe://" + TrustDomain + "/" + id.Cluster + "/" + id.Kind
}

// URL returns the SPIFFE ID as a URI SAN.
func (id Identity) URL() *url.URL {
	parsed, _ := url.Parse(id.String())
	return parsed
}

// ValidCluster reports whether name can be a cluster name.
func ValidCluster(name string) bool { return clusterPattern.MatchString(name) }

// Kernel returns the kernel identity of cluster.
func Kernel(cluster string) (Identity, error) {
	if !ValidCluster(cluster) {
		return Identity{}, fmt.Errorf("%w: cluster %q", ErrInvalidIdentity, cluster)
	}
	return Identity{Cluster: cluster, Kind: KindKernel}, nil
}
